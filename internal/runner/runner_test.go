package runner

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultRunnerTimeout(t *testing.T) {
	t.Setenv("AGENTUP_TEST_SLEEP", "1")
	r := WithTimeout(&DefaultRunner{}, 50*time.Millisecond)
	started := time.Now()
	_, _, code, err := r.Run(os.Args[0], "-test.run=^TestRunnerHelperProcess$")
	if !errors.Is(err, context.DeadlineExceeded) || code != -1 {
		t.Fatalf("expected timeout, got code %d, %v", code, err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("cancellation did not stop the child promptly")
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	switch os.Getenv("AGENTUP_TEST_TREE") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestRunnerHelperProcess$")
		child.Env = append(os.Environ(), "AGENTUP_TEST_TREE=child")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Second)
		return
	case "child":
		dir := os.Getenv("AGENTUP_TEST_TREE_DIR")
		if err := os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		_ = os.WriteFile(filepath.Join(dir, "survived"), []byte("alive"), 0600)
		time.Sleep(10 * time.Second)
		return
	}
	if os.Getenv("AGENTUP_TEST_SLEEP") != "1" {
		return
	}
	time.Sleep(10 * time.Second)
}

func TestDefaultRunnerCancellationKillsDescendants(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTUP_TEST_TREE", "parent")
	t.Setenv("AGENTUP_TEST_TREE_DIR", dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, _, err := (&DefaultRunner{}).RunContext(ctx, os.Args[0], "-test.run=^TestRunnerHelperProcess$")
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("descendant did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "survived")); err == nil {
		t.Fatal("descendant survived cancellation")
	}
}

var errNotFound = errors.New("executable not found")

func TestMockRunner_RunWithExactMatch(t *testing.T) {
	mr := NewMockRunner()
	mr.SetResult("npm", []string{"--version"}, MockResult{
		Stdout:   "11.10.0\n",
		ExitCode: 0,
	})

	stdout, _, exitCode, err := mr.Run("npm", "--version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if stdout != "11.10.0\n" {
		t.Errorf("expected stdout '11.10.0\\n', got '%s'", stdout)
	}
}

func TestMockRunner_RunWithNameFallback(t *testing.T) {
	mr := NewMockRunner()
	mr.SetResultByName("npm", MockResult{
		Stdout:   "11.10.0\n",
		ExitCode: 0,
	})

	// Should fall back to name-only match even with different args
	stdout, _, exitCode, err := mr.Run("npm", "list", "-g")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if stdout != "11.10.0\n" {
		t.Errorf("expected stdout '11.10.0\\n', got '%s'", stdout)
	}
}

func TestMockRunner_RunNoResultSet(t *testing.T) {
	mr := NewMockRunner()
	_, _, _, err := mr.Run("nonexistent", "arg1")
	if err == nil {
		t.Fatal("expected error for unset command")
	}
}

func TestMockRunner_LookPath(t *testing.T) {
	mr := NewMockRunner()
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)

	path, err := mr.LookPath("codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/usr/local/bin/codex" {
		t.Errorf("expected '/usr/local/bin/codex', got '%s'", path)
	}
}

func TestMockRunner_LookPathNotFound(t *testing.T) {
	mr := NewMockRunner()
	mr.SetLookPath("nonexistent", "", errNotFound)

	_, err := mr.LookPath("nonexistent")
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestMockRunner_RecordsCalls(t *testing.T) {
	mr := NewMockRunner()
	mr.SetResultByName("npm", MockResult{ExitCode: 0})
	mr.SetResultByName("brew", MockResult{ExitCode: 0})

	mr.Run("npm", "--version")
	mr.Run("brew", "list", "codex")

	if len(mr.Calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(mr.Calls))
	}
	if mr.Calls[0].Name != "npm" {
		t.Errorf("expected first call 'npm', got '%s'", mr.Calls[0].Name)
	}
	if mr.Calls[1].Name != "brew" {
		t.Errorf("expected second call 'brew', got '%s'", mr.Calls[1].Name)
	}
	if len(mr.Calls[1].Args) != 2 {
		t.Errorf("expected 2 args for second call, got %d", len(mr.Calls[1].Args))
	}
}

func TestMockRunner_RunReturnsError(t *testing.T) {
	mr := NewMockRunner()
	mr.SetResultByName("failing", MockResult{
		ExitCode: -1,
		Err:      errNotFound,
	})

	_, _, _, err := mr.Run("failing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockRunner_RunReturnsNonZeroExit(t *testing.T) {
	mr := NewMockRunner()
	mr.SetResultByName("failing", MockResult{
		Stderr:   "permission denied",
		ExitCode: 1,
	})

	_, stderr, exitCode, err := mr.Run("failing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}
	if stderr != "permission denied" {
		t.Errorf("expected stderr 'permission denied', got '%s'", stderr)
	}
}
