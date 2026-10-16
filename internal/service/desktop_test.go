package service

import (
	"context"
	"strings"
	"sync"
	"testing"

	"agentup/internal/desktop"
	"agentup/internal/operation"
	"agentup/internal/platform"
	"agentup/internal/runner"
)

type fakeDesktop struct {
	mu       sync.Mutex
	snapshot desktop.Snapshot
	executed []desktop.Plan
	log      string
	gate     <-chan struct{}
	stopped  []string
}

func (f *fakeDesktop) Scan(bool) desktop.Snapshot { return f.snapshot }

func (f *fakeDesktop) Execute(plan desktop.Plan, log func(string, string)) desktop.Result {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	f.executed = append(f.executed, plan)
	f.mu.Unlock()
	log("phase", "upgrading")
	if f.log != "" {
		log("stdout", f.log)
	}
	return desktop.Result{
		Installation: plan.Installation, ProductID: plan.ProductID, Outcome: "handed_off",
		OldVersion: plan.BeforeVersion, URL: plan.URL,
	}
}

func (f *fakeDesktop) Stop(info desktop.Installation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, info.ID)
	for i := range f.snapshot.Installations {
		if f.snapshot.Installations[i].ID == info.ID {
			f.snapshot.Installations[i].Running = "stopped"
		}
	}
	return nil
}

func desktopServiceFixture(t *testing.T) (*Service, *fakeDesktop) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	info := desktop.Installation{
		ID: "sample-installation", ProductID: "antigravity.desktop", Name: "Antigravity app",
		Installed: true, Capability: desktop.Guided, Version: "2.18.1",
		Plan: &desktop.Plan{
			ID: "trusted-plan", Installation: "sample-installation",
			ProductID: "antigravity.desktop", Action: "guide", BeforeVersion: "2.18.1",
			URL: "https://antigravity.google/download",
		},
	}
	backend := &fakeDesktop{snapshot: desktop.Snapshot{
		Installations: []desktop.Installation{info}, Issues: []string{},
	}}
	s := New(context.Background(), "test", "amd64", runner.NewMockRunner(), platform.NewPlatform(), nil)
	s.desktopManager = backend
	s.state.Desktop, s.state.Revision = backend.snapshot, 42
	t.Cleanup(s.Close)
	return s, backend
}

func TestDesktopHandoffTaskUsesBoundedLogsAndNeverClaimsUpdated(t *testing.T) {
	s, backend := desktopServiceFixture(t)
	backend.log = "\x1b[31m" + strings.Repeat("x", 140<<10) + "\x1b[0m"
	if _, err := s.UpgradeDesktops([]string{"trusted-plan"}, 42); err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 && !state.Scanning })
	task := state.DesktopTasks[0]
	if task.Status != "handed_off" || task.Items[0].Result.Outcome != "handed_off" {
		t.Fatalf("handoff was displayed as a verified upgrade: %+v", task)
	}
	if !task.LogTruncated || len(task.Logs) != 1 || len(task.Logs[0].Text) > 16<<10 ||
		strings.Contains(task.Logs[0].Text, "\x1b") {
		t.Fatalf("desktop logs were not bounded and cleaned: %+v", task.Logs)
	}
}

func TestDesktopPlanSelectionValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		plans    []string
		revision uint64
		want     error
		setup    func(*Service)
	}{
		{name: "stale revision", plans: []string{"trusted-plan"}, revision: 41, want: ErrStale},
		{name: "unknown plan", plans: []string{"arbitrary command"}, revision: 42, want: ErrStale},
		{name: "empty", revision: 42, want: ErrInvalid},
		{name: "duplicate", plans: []string{"trusted-plan", "trusted-plan"}, revision: 42, want: ErrInvalid},
		{name: "offline", plans: []string{"trusted-plan"}, revision: 42, want: ErrOffline, setup: func(s *Service) { s.state.Offline = true }},
		{name: "busy", plans: []string{"trusted-plan"}, revision: 42, want: ErrBusy, setup: func(s *Service) { s.state.Scanning = true }},
		{name: "blocked", plans: []string{"trusted-plan"}, revision: 42, want: ErrUnsupported, setup: func(s *Service) { s.state.Desktop.Installations[0].Capability = desktop.Blocked }},
		{name: "privileged needs a terminal", plans: []string{"trusted-plan"}, revision: 42, want: ErrUnsupported, setup: func(s *Service) {
			s.state.Desktop.Installations[0].Capability = desktop.Privileged
			s.state.Desktop.Installations[0].Plan.Action = desktop.ActionPrivileged
		}},
		{name: "standalone excluded from batch", plans: []string{"trusted-plan", "second-plan"}, revision: 42, want: ErrUnsupported, setup: func(s *Service) {
			first := &s.state.Desktop.Installations[0]
			first.Capability, first.Plan.Action, first.Plan.Standalone = desktop.Automatic, desktop.ActionCommand, true
			second := *first
			second.ID, second.Plan = "second", &desktop.Plan{ID: "second-plan", Installation: "second", Action: desktop.ActionCommand}
			s.state.Desktop.Installations = append(s.state.Desktop.Installations, second)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, backend := desktopServiceFixture(t)
			if tc.setup != nil {
				tc.setup(s)
			}
			if _, err := s.UpgradeDesktops(tc.plans, tc.revision); err != tc.want {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if len(backend.executed) != 0 {
				t.Fatal("rejected request reached the executor")
			}
		})
	}
}

func TestDesktopUpgradeSharesMutationLock(t *testing.T) {
	s, _ := desktopServiceFixture(t)
	release, err := operation.Acquire("mutation")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := s.UpgradeDesktops([]string{"trusted-plan"}, 42); err != ErrBusy {
		t.Fatalf("desktop operation bypassed mutation lock: %v", err)
	}
}

func TestDesktopTaskProtectsShutdownAndOtherOperations(t *testing.T) {
	s, backend := desktopServiceFixture(t)
	gate := make(chan struct{})
	backend.gate = gate
	if _, err := s.UpgradeDesktops([]string{"trusted-plan"}, 42); err != nil {
		close(gate)
		t.Fatal(err)
	}
	stopErr := s.Stop(false)
	refreshErr := s.Refresh(false)
	close(gate)
	if stopErr != ErrBusy || refreshErr != ErrBusy {
		t.Fatalf("active desktop operation was not protected: stop=%v refresh=%v", stopErr, refreshErr)
	}
	waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 && !state.Scanning })
}

func TestIndependentDesktopUpgradesStartImmediately(t *testing.T) {
	s, backend := desktopServiceFixture(t)
	second := backend.snapshot.Installations[0]
	second.ID, second.Name = "second-installation", "Second app"
	plan := *second.Plan
	plan.ID, plan.Installation = "second-plan", second.ID
	second.Plan = &plan
	backend.snapshot.Installations = append(backend.snapshot.Installations, second)
	s.state.Desktop = backend.snapshot
	gate := make(chan struct{})
	backend.gate = gate
	if _, err := s.UpgradeDesktops([]string{"trusted-plan"}, 42); err != nil {
		close(gate)
		t.Fatal(err)
	}
	// Log lines and phases from the running task must not invalidate the next click.
	revision := s.Snapshot().Revision
	if _, err := s.UpgradeDesktops([]string{"trusted-plan"}, revision); err != ErrBusy {
		close(gate)
		t.Fatalf("the same installation was started twice: %v", err)
	}
	if _, err := s.UpgradeDesktops([]string{"second-plan"}, revision); err != nil {
		close(gate)
		t.Fatal("a running task blocked another installation:", err)
	}
	state := s.Snapshot()
	if len(state.ActiveTasks) != 2 || state.DesktopTasks[0].Status != "running" || state.DesktopTasks[1].Status != "running" {
		close(gate)
		t.Fatalf("upgrades did not start right away: %+v", state)
	}
	close(gate)
	state = waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 })
	if len(backend.executed) != 2 {
		t.Fatalf("both plans should run: %+v", backend.executed)
	}
	for _, task := range state.DesktopTasks {
		if task.Status != "handed_off" {
			t.Fatalf("task did not finish: %+v", task)
		}
	}
}

func TestStopDesktopQuitsOnlyARunningApp(t *testing.T) {
	s, backend := desktopServiceFixture(t)
	if _, err := s.StopDesktop("sample-installation", 42); err != ErrUnsupported {
		t.Fatalf("an app that is not running was stopped: %v", err)
	}
	backend.snapshot.Installations[0].Running = "running"
	s.state.Desktop = backend.snapshot
	if _, err := s.StopDesktop("sample-installation", 41); err != ErrStale {
		t.Fatalf("stale revision was accepted: %v", err)
	}
	id, err := s.StopDesktop("sample-installation", 42)
	if err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 })
	task := state.DesktopTasks[0]
	if task.ID != id || task.Action != "stop" || task.Status != "completed" || task.Items[0].Result.Outcome != "stopped" {
		t.Fatalf("stop was not recorded: %+v", task)
	}
	if len(backend.stopped) != 1 || state.Desktop.Installations[0].Running != "stopped" {
		t.Fatalf("app was not stopped and rescanned: %v %+v", backend.stopped, state.Desktop.Installations[0])
	}
}
