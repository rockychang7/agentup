package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/operation"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/testutil"
)

func newTestService(t *testing.T) (*Service, *runner.MockRunner) {
	t.Helper()
	testutil.StubAgyManifest(t, "1.1.0")
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	mr := runner.NewMockRunner()
	mr.SetLookPath("agy", "/mock/agy", nil)
	mr.SetResult("/mock/agy", []string{"--version"}, runner.MockResult{Stdout: "1.0.0"})
	mr.SetResult("/mock/agy", []string{"update"}, runner.MockResult{Stdout: "\x1b[32mdone\x1b[0m\n"})
	s := New(context.Background(), "test", "test", mr, platform.NewPlatform(), config.DefaultTools())
	t.Cleanup(s.Close)
	return s, mr
}

func TestAgyRefreshReportsLatestAndOfflineSkipsHTTP(t *testing.T) {
	s, mr := newTestService(t)
	calls := testutil.StubAgyManifest(t, "1.1.0")
	if err := s.Refresh(false); err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return !state.Scanning })
	agy := state.Agents[3]
	if agy.LatestCheck != model.CheckOK || agy.LatestVersion != "1.1.0" || !agy.UpgradeAvailable {
		t.Fatalf("Agy manifest was not exposed to the UI: %+v", agy)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one online lookup, got %d", calls.Load())
	}
	if err := s.Refresh(true); err != nil {
		t.Fatal(err)
	}
	state = waitState(t, s, func(state State) bool { return !state.Scanning })
	agy = state.Agents[3]
	if agy.LatestCheck != model.CheckOffline || agy.LatestVersion != "" || agy.UpgradeAvailable || calls.Load() != 1 {
		t.Fatalf("offline refresh kept stale latest data or queried HTTP: %+v, requests=%d", agy, calls.Load())
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && call.Args[0] == "update" {
			t.Fatal("version lookup ran the Agy updater")
		}
	}
}

func waitState(t *testing.T, s *Service, predicate func(State) bool) State {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		state := s.Snapshot()
		if predicate(state) {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("service did not reach expected state")
	return State{}
}

func TestRefreshIsReadOnlyAndOfflineBlocksUpgrade(t *testing.T) {
	s, mr := newTestService(t)
	if err := s.Refresh(true); err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return !state.Scanning })
	if !state.Offline || len(state.Agents[3].Command) != 2 {
		t.Fatalf("missing preview: %+v", state)
	}
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, state.Revision); err != ErrOffline {
		t.Fatalf("offline allowed upgrade: %v", err)
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && (call.Args[0] == "update" || call.Args[0] == "view") {
			t.Fatalf("scan mutated or queried remote: %+v", call)
		}
	}
}

func TestSelectionValidationAndCompletedTask(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.Refresh(false); err != nil {
		t.Fatal(err)
	}
	scanned := waitState(t, s, func(state State) bool { return !state.Scanning })
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, scanned.Revision-1); err != ErrStale {
		t.Fatal("stale preview accepted", err)
	}
	for _, names := range [][]model.ToolName{nil, {model.ToolAgy, model.ToolAgy}, {"shell;whoami"}} {
		if _, err := s.Upgrade(names, scanned.Revision); err != ErrInvalid {
			t.Fatalf("invalid selection accepted: %v", err)
		}
	}
	if _, err := s.Upgrade([]model.ToolName{model.ToolCodex}, scanned.Revision); err != ErrUnsupported {
		t.Fatalf("uninstalled tool accepted: %v", err)
	}
	release, err := operation.Acquire("mutation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, scanned.Revision); err != ErrBusy {
		t.Fatalf("external lock ignored: %v", err)
	}
	release()
	id, err := s.Upgrade([]model.ToolName{model.ToolAgy}, scanned.Revision)
	if err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 && !state.Scanning && len(state.Tasks) > 0 })
	task := state.Tasks[0]
	if task.ID != id || task.Status != "completed" || task.Tools[0].Result.Outcome != model.OutcomeUnchanged {
		t.Fatalf("invalid result: %+v", task)
	}
	if task.Tools[0].Result.Log != "" || strings.Contains(task.Logs[0].Text, "\x1b") {
		t.Fatal("logs were duplicated or not sanitized")
	}
	// Snapshots cannot mutate service-owned data.
	state.Tasks[0].Tools[0].Name = "evil"
	if s.Snapshot().Tasks[0].Tools[0].Name != model.ToolAgy {
		t.Fatal("snapshot aliases state")
	}
}

func TestFailureDoesNotLeaveServiceBusy(t *testing.T) {
	s, mr := newTestService(t)
	mr.SetResult("/mock/agy", []string{"update"}, runner.MockResult{Stderr: "network failed", ExitCode: 1})
	_ = s.Refresh(false)
	scanned := waitState(t, s, func(state State) bool { return !state.Scanning })
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, scanned.Revision); err != nil {
		t.Fatal(err)
	}
	state := waitState(t, s, func(state State) bool { return len(state.ActiveTasks) == 0 && !state.Scanning && len(state.Tasks) > 0 })
	if state.Tasks[0].Status != "failed" {
		t.Fatal(state.Tasks[0].Status)
	}
	if err := s.Diagnose(); err != nil {
		t.Fatal("failed task blocked diagnostics", err)
	}
	waitState(t, s, func(state State) bool { return !state.Diagnosing && state.Doctor != nil })
}

func TestCloseCancelsProductionRunnerAndRejectsNewWork(t *testing.T) {
	s := New(context.Background(), "test", "test", &runner.DefaultRunner{}, platform.NewPlatform(), nil)
	base := s.r.(*runner.DefaultRunner)
	s.Close()
	if base.Context.Err() != context.Canceled {
		t.Fatal("runner was not bound to service lifetime")
	}
	if err := s.Refresh(false); err != ErrBusy {
		t.Fatal("closed service accepted work")
	}
}

func TestCleanAndTailPreserveUnicode(t *testing.T) {
	if clean("\x1b[31mhello\x1b[0m\x00\n中文") != "hello\n中文" {
		t.Fatal("control characters survived")
	}
	result := tail(strings.Repeat("中文", 100), 101)
	if !utf8.ValidString(result) || len(result) > 101 {
		t.Fatal("tail split a Unicode rune")
	}
}

type blockingRunner struct {
	*runner.MockRunner
	started chan struct{}
	resume  chan struct{}
	once    sync.Once
	args    string
}

func (r *blockingRunner) Run(name string, args ...string) (string, string, int, error) {
	if name == "/mock/agy" && strings.Join(args, " ") == r.args {
		r.once.Do(func() {
			close(r.started)
			<-r.resume
		})
	}
	return r.MockRunner.Run(name, args...)
}

func TestBusyGuardsDuringScanAndUpgrade(t *testing.T) {
	for _, args := range []string{"--version", "update"} {
		t.Run(args, func(t *testing.T) {
			s, mr := newTestService(t)
			r := &blockingRunner{MockRunner: mr, args: args, started: make(chan struct{}), resume: make(chan struct{})}
			var once sync.Once
			resume := func() { once.Do(func() { close(r.resume) }) }
			t.Cleanup(resume)
			s.r = r
			if err := s.Refresh(false); err != nil {
				t.Fatal(err)
			}
			if args == "update" {
				state := waitState(t, s, func(state State) bool { return !state.Scanning })
				if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, state.Revision); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-r.started:
			case <-time.After(3 * time.Second):
				t.Fatal("operation did not start")
			}
			if err := s.Refresh(true); err != ErrBusy {
				t.Fatal("refresh bypassed busy state", err)
			}
			if err := s.Diagnose(); err != ErrBusy {
				t.Fatal("diagnostics bypassed busy state", err)
			}
			if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, s.Snapshot().Revision); err != ErrBusy {
				t.Fatal("upgrade bypassed busy state", err)
			}
			resume()
			waitState(t, s, func(state State) bool { return !state.Scanning && len(state.ActiveTasks) == 0 })
		})
	}
}

func TestHistoryAndOutputRemainBounded(t *testing.T) {
	s, mr := newTestService(t)
	mr.SetResult("/mock/agy", []string{"update"}, runner.MockResult{Stdout: strings.Repeat("long output\n", 10000)})
	changes, unsubscribe := s.Subscribe()
	defer unsubscribe()
	if err := s.Refresh(false); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, func(state State) bool { return !state.Scanning })
	for range 21 {
		if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, s.Snapshot().Revision); err != nil {
			t.Fatal(err)
		}
		waitState(t, s, func(state State) bool { return !state.Scanning && len(state.ActiveTasks) == 0 })
	}
	state := s.Snapshot()
	if len(state.Tasks) != 20 || len(changes) != 1 {
		t.Fatal("history or subscription is unbounded")
	}
	for _, task := range state.Tasks {
		if !task.LogTruncated || len(task.Logs) != 1 || len(task.Logs[0].Text) > 16<<10 {
			t.Fatalf("unbounded task output: %+v", task)
		}
	}
}

func TestInstallationChangeStopsBeforeMutation(t *testing.T) {
	s, mr := newTestService(t)
	_ = s.Refresh(false)
	state := waitState(t, s, func(state State) bool { return !state.Scanning })
	mr.SetLookPath("agy", "/different/agy", nil)
	mr.SetResult("/different/agy", []string{"--version"}, runner.MockResult{Stdout: "1.0.0"})
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, state.Revision); err != nil {
		t.Fatal(err)
	}
	state = waitState(t, s, func(state State) bool { return !state.Scanning && len(state.ActiveTasks) == 0 })
	if state.Tasks[0].Status != "failed" {
		t.Fatal("changed installation was accepted")
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && call.Args[0] == "update" {
			t.Fatal("a command was executed without matching confirmation")
		}
	}
}

func TestMultipleServicesSerializeUpgradesAndProtectActiveTaskOnStop(t *testing.T) {
	s, mr := newTestService(t)
	r := &blockingRunner{MockRunner: mr, args: "update", started: make(chan struct{}), resume: make(chan struct{})}
	var once sync.Once
	resume := func() { once.Do(func() { close(r.resume) }) }
	t.Cleanup(resume)
	s.r = r
	other := New(context.Background(), "test", "test", mr, platform.NewPlatform(), config.DefaultTools())
	t.Cleanup(other.Close)
	for _, current := range []*Service{s, other} {
		if err := current.Refresh(false); err != nil {
			t.Fatal(err)
		}
		waitState(t, current, func(state State) bool { return !state.Scanning })
	}
	if _, err := s.Upgrade([]model.ToolName{model.ToolAgy}, s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade did not start")
	}
	if _, err := other.Upgrade([]model.ToolName{model.ToolAgy}, other.Snapshot().Revision); err != ErrBusy {
		t.Fatal("another instance bypassed the mutation lock", err)
	}
	if err := s.Stop(false); err != ErrBusy || s.ctx.Err() != nil {
		t.Fatal("normal shutdown canceled an active upgrade", err)
	}
	if err := s.Stop(true); err != nil || s.ctx.Err() != context.Canceled {
		t.Fatal("forced shutdown did not cancel the instance", err)
	}
	if other.ctx.Err() != nil {
		t.Fatal("stopping one service canceled another")
	}
	resume()
	s.Close()
	if _, err := other.Upgrade([]model.ToolName{model.ToolAgy}, other.Snapshot().Revision); err != nil {
		t.Fatal("stopped instance retained the mutation lock", err)
	}
	waitState(t, other, func(state State) bool { return !state.Scanning && len(state.ActiveTasks) == 0 })
}
