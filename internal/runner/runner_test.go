package runner

import (
	"errors"
	"testing"
)

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
