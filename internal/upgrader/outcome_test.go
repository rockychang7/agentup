package upgrader

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/runner"
)

type versionSequenceRunner struct {
	before, after string
	updated       bool
}

func (r *versionSequenceRunner) LookPath(name string) (string, error) {
	if r.updated && r.after == "missing" {
		return "", fmt.Errorf("not found")
	}
	return "/mock/tool", nil
}

func (r *versionSequenceRunner) Run(name string, args ...string) (string, string, int, error) {
	if len(args) == 1 && args[0] == "update" {
		r.updated = true
		return "updated", "", 0, nil
	}
	v := r.before
	if r.updated {
		v = r.after
	}
	if v == "unknown" {
		return "", "", 1, nil
	}
	return v, "", 0, nil
}

func TestUpgradeVerificationOutcomes(t *testing.T) {
	for _, tt := range []struct {
		before, after string
		want          model.UpgradeOutcome
	}{
		{"1.0.0", "1.1.0", model.OutcomeUpdated},
		{"1.1.0", "1.0.0", model.OutcomeFailed},
		{"1.0.0", "1.0.0", model.OutcomeUnchanged},
		{"1.0.0", "unknown", model.OutcomeUnverified},
		{"unknown", "1.1.0", model.OutcomeUnverified},
		{"1.0.0", "missing", model.OutcomeUnverified},
	} {
		t.Run(tt.before+"-"+tt.after, func(t *testing.T) {
			r := &versionSequenceRunner{before: tt.before, after: tt.after}
			tools := []config.ToolConfig{{Name: "test", BinaryName: "test", BinaryOnly: true,
				VersionArgs: []string{"--version"}, SelfUpdateArgs: []string{"update"}, UpgradeSupported: true}}
			p := &testPlatform{name: "test"}
			up := New(r, p, detector.NewWithoutLatestCheck(r, p, tools), tools)
			var phases []model.UpgradePhase
			up.SetObserver(func(e model.UpgradeEvent) { phases = append(phases, e.Phase) })
			result, err := up.Upgrade("test")
			if err != nil || result.Outcome != tt.want {
				t.Fatalf("got %+v, %v; want %s", result, err, tt.want)
			}
			if strings.Contains(result.Message, "already up to date") {
				t.Fatal("upgrade cannot claim latest without a registry comparison")
			}
			want := []model.UpgradePhase{model.PhaseDetecting, model.PhaseWaiting, model.PhaseUpgrading, model.PhaseVerifying, model.PhaseFinished}
			if fmt.Sprint(phases) != fmt.Sprint(want) {
				t.Fatalf("unexpected phases: %v", phases)
			}
		})
	}
}

type managerBlockingRunner struct {
	*runner.MockRunner
	mu           sync.Mutex
	active, peak int
	started      chan struct{}
	release      chan struct{}
}

func (r *managerBlockingRunner) Run(name string, args ...string) (string, string, int, error) {
	if name == "npm" && len(args) > 0 && args[0] == "install" {
		r.mu.Lock()
		r.active++
		r.peak = max(r.peak, r.active)
		r.mu.Unlock()
		r.started <- struct{}{}
		<-r.release
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
		return "", "", 0, nil
	}
	return r.MockRunner.Run(name, args...)
}

func TestUpgradeSerializesSamePackageManager(t *testing.T) {
	mr := runner.NewMockRunner()
	setupNpmCodexInstalled(mr)
	mr.SetLookPath("claude", "/mock/claude", nil)
	mr.SetResult("/mock/claude", []string{"--version"}, runner.MockResult{Stdout: "1.0.0"})
	mr.SetResult("npm", []string{"list", "-g", "@anthropic-ai/claude-code", "--depth=0"}, runner.MockResult{Stdout: "@anthropic-ai/claude-code@1.0.0"})
	r := &managerBlockingRunner{MockRunner: mr, started: make(chan struct{}, 2), release: make(chan struct{})}
	tools := config.DefaultTools()[:2]
	p := macPlatform()
	up := New(r, p, detector.NewWithoutLatestCheck(r, p, tools), tools)
	waiting := make(chan struct{}, 2)
	up.SetObserver(func(e model.UpgradeEvent) {
		if e.Phase == model.PhaseWaiting {
			waiting <- struct{}{}
		}
	})
	done := make(chan []model.UpgradeResult, 1)
	go func() { done <- up.UpgradeAll() }()
	defer func() {
		// Release workers even if a test assertion fails.
		select {
		case <-r.release:
		default:
			close(r.release)
		}
	}()
	for range 2 {
		select {
		case <-waiting:
		case <-time.After(2 * time.Second):
			t.Fatal("both tasks should reach the manager queue")
		}
	}
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first mutation did not start")
	}
	select {
	case <-r.started:
		t.Fatal("same manager started overlapping mutations")
	case <-time.After(50 * time.Millisecond):
	}
	close(r.release)
	select {
	case results := <-done:
		if len(results) != 2 || r.peak != 1 {
			t.Fatalf("expected two results with one active npm mutation: %v, peak %d", results, r.peak)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upgrades did not finish")
	}
}
