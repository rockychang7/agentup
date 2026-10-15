package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Runner is the interface for executing external commands.
// All external command execution should go through this interface
// instead of calling exec.Command directly.
type Runner interface {
	// Run executes a command and returns stdout, stderr, exit code, and error.
	// If the command runs but exits with non-zero code, exitCode reflects that
	// and err is nil. err is only non-nil if the command failed to start.
	Run(name string, args ...string) (stdout, stderr string, exitCode int, err error)
	// LookPath searches for the executable in PATH.
	LookPath(file string) (string, error)
}

// DefaultRunner is the production implementation of Runner using exec.Command.
type DefaultRunner struct {
	Context context.Context
}

// Run executes a command and captures stdout, stderr, and exit code.
func (r *DefaultRunner) Run(name string, args ...string) (stdout, stderr string, exitCode int, err error) {
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return r.RunContext(ctx, name, args...)
}

func (r *DefaultRunner) RunContext(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error) {
	return r.runContext(ctx, name, args, nil)
}

// RunStreaming reports output while retaining the usual command result.
func (r *DefaultRunner) RunStreaming(name string, args []string, output func(string, string)) (string, string, int, error) {
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	return r.runContext(ctx, name, args, output)
}

func (r *DefaultRunner) runContext(ctx context.Context, name string, args []string, output func(string, string)) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	configureCancellation(cmd)
	// Prevent a child that inherits the pipes from blocking cancellation indefinitely.
	cmd.WaitDelay = time.Second
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if output != nil {
		out := newStreamWriter(&stdoutBuf, "stdout", output)
		errOut := newStreamWriter(&stderrBuf, "stderr", output)
		cmd.Stdout, cmd.Stderr = out, errOut
		defer out.flush()
		defer errOut.flush()
	}

	if runErr := cmd.Run(); runErr != nil {
		if ctx.Err() != nil {
			return stdoutBuf.String(), stderrBuf.String(), -1, ctx.Err()
		}
		// Try to extract exit code from the error
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		// Command failed to start (e.g., not found)
		return "", "", -1, fmt.Errorf("failed to execute '%s': %w", name, runErr)
	}

	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

func (r *timedRunner) RunStreaming(name string, args []string, output func(string, string)) (string, string, int, error) {
	if base, ok := r.Runner.(*DefaultRunner); ok {
		ctx := base.Context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()
		return base.runContext(ctx, name, args, output)
	}
	stdout, stderr, code, err := r.Run(name, args...)
	if stdout != "" {
		output("stdout", stdout)
	}
	if stderr != "" {
		output("stderr", stderr)
	}
	return stdout, stderr, code, err
}

type timedRunner struct {
	Runner
	timeout time.Duration
}

// WithTimeout bounds production commands without changing the simple Runner test interface.
func WithTimeout(r Runner, timeout time.Duration) Runner {
	return &timedRunner{Runner: r, timeout: timeout}
}

func (r *timedRunner) Run(name string, args ...string) (string, string, int, error) {
	if contextual, ok := r.Runner.(interface {
		RunContext(context.Context, string, ...string) (string, string, int, error)
	}); ok {
		ctx := context.Background()
		if base, ok := r.Runner.(*DefaultRunner); ok && base.Context != nil {
			ctx = base.Context
		}
		ctx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()
		return contextual.RunContext(ctx, name, args...)
	}
	return r.Runner.Run(name, args...)
}

// RunInteractive hands the terminal to a command, for prompts such as sudo passwords.
// Its output goes to stderr so machine-readable stdout stays intact.
func (r *DefaultRunner) RunInteractive(name string, args []string) (int, error) {
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return -1, ctx.Err()
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("failed to execute '%s': %w", name, err)
	}
	return 0, nil
}

// LookPath searches for an executable in the system PATH.
func (r *DefaultRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// --- MockRunner for unit testing ---

// MockResult is a preset result for a mock command call.
type MockResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// MockRunner is a test double for Runner. It allows setting up
// preset results for specific commands.
type MockRunner struct {
	mu sync.Mutex

	// Results maps "name arg1 arg2..." to preset results.
	// If no exact match, falls back to results keyed by just the command name.
	Results map[string]MockResult
	// LookPathResults maps file name to path or error.
	LookPathResults map[string]struct {
		Path string
		Err  error
	}
	// Calls records all Run calls made during the test.
	Calls []MockCall
}

// MockCall records a single Run invocation.
type MockCall struct {
	Name string
	Args []string
}

// Run returns a preset result for the given command.
func (m *MockRunner) Run(name string, args ...string) (stdout, stderr string, exitCode int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, MockCall{Name: name, Args: append([]string(nil), args...)})

	// Try exact match with args first
	key := name
	if len(args) > 0 {
		key = name + " " + strings.Join(args, " ")
	}
	if result, ok := m.Results[key]; ok {
		return result.Stdout, result.Stderr, result.ExitCode, result.Err
	}

	// Fall back to command name only
	if result, ok := m.Results[name]; ok {
		return result.Stdout, result.Stderr, result.ExitCode, result.Err
	}

	// Default: command not found
	return "", "", -1, fmt.Errorf("mock: no result set for '%s'", key)
}

// RunInteractive records the call and returns the preset exit code.
func (m *MockRunner) RunInteractive(name string, args []string) (int, error) {
	_, _, code, err := m.Run(name, args...)
	return code, err
}

// LookPath returns a preset path result.
func (m *MockRunner) LookPath(file string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if result, ok := m.LookPathResults[file]; ok {
		return result.Path, result.Err
	}
	return "", fmt.Errorf("mock: executable not found: %s", file)
}

// NewMockRunner creates a new MockRunner with empty result maps.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		Results: make(map[string]MockResult),
		LookPathResults: make(map[string]struct {
			Path string
			Err  error
		}),
	}
}

// SetResult sets a preset result for a command (with args).
func (m *MockRunner) SetResult(name string, args []string, result MockResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := name
	if len(args) > 0 {
		key = name + " " + strings.Join(args, " ")
	}
	m.Results[key] = result
}

// SetResultByName sets a preset result for a command (by name only, no args).
func (m *MockRunner) SetResultByName(name string, result MockResult) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Results[name] = result
}

// SetLookPath sets a preset LookPath result for a file.
func (m *MockRunner) SetLookPath(file string, path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.LookPathResults[file] = struct {
		Path string
		Err  error
	}{Path: path, Err: err}
}
