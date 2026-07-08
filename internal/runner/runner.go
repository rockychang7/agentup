package runner

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
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
type DefaultRunner struct{}

// Run executes a command and captures stdout, stderr, and exit code.
func (r *DefaultRunner) Run(name string, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command(name, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if runErr := cmd.Run(); runErr != nil {
		// Try to extract exit code from the error
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		// Command failed to start (e.g., not found)
		return "", "", -1, fmt.Errorf("failed to execute '%s': %w", name, runErr)
	}

	return stdoutBuf.String(), stderrBuf.String(), 0, nil
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
	m.Calls = append(m.Calls, MockCall{Name: name, Args: args})

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

// LookPath returns a preset path result.
func (m *MockRunner) LookPath(file string) (string, error) {
	if result, ok := m.LookPathResults[file]; ok {
		return result.Path, result.Err
	}
	return "", fmt.Errorf("mock: executable not found: %s", file)
}

// NewMockRunner creates a new MockRunner with empty result maps.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		Results:         make(map[string]MockResult),
		LookPathResults: make(map[string]struct {
			Path string
			Err  error
		}),
	}
}

// SetResult sets a preset result for a command (with args).
func (m *MockRunner) SetResult(name string, args []string, result MockResult) {
	key := name
	if len(args) > 0 {
		key = name + " " + strings.Join(args, " ")
	}
	m.Results[key] = result
}

// SetResultByName sets a preset result for a command (by name only, no args).
func (m *MockRunner) SetResultByName(name string, result MockResult) {
	m.Results[name] = result
}

// SetLookPath sets a preset LookPath result for a file.
func (m *MockRunner) SetLookPath(file string, path string, err error) {
	m.LookPathResults[file] = struct {
		Path string
		Err  error
	}{Path: path, Err: err}
}
