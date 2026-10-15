package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"agentup/internal/operation"
)

func TestCommandErrorExit(t *testing.T) {
	if os.Getenv("AGENTUP_TEST_ERROR_PROCESS") == "1" {
		rootCmd.SetArgs(os.Args[3:])
		Execute()
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		args    []string
		ui      bool
		verbose bool
	}{
		{name: "update", args: []string{"update"}, ui: true},
		{name: "self-update alias", args: []string{"self-update"}, ui: true},
		{name: "upgrade agentup", args: []string{"upgrade", "agentup"}, ui: true},
		{name: "uninstall", args: []string{"uninstall"}, ui: true},
		{name: "verbose", args: []string{"update", "--verbose"}, ui: true, verbose: true},
		{name: "json", args: []string{"update", "--json", "--color=always"}, ui: true},
		{name: "mutation", args: []string{"upgrade", "codex"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateLocks(t)
			var release func()
			if tt.ui {
				release, err = operation.AcquireShared("ui")
			} else {
				release, err = operation.Acquire("mutation")
			}
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestCommandErrorExit$", "--"}, tt.args...)
			child := exec.CommandContext(ctx, executable, args...)
			child.Env = append(os.Environ(), "AGENTUP_TEST_ERROR_PROCESS=1")
			var stdout, stderr bytes.Buffer
			child.Stdout, child.Stderr = &stdout, &stderr
			var exitError *exec.ExitError
			if runErr := child.Run(); !errors.As(runErr, &exitError) || exitError.ExitCode() != 1 {
				t.Fatalf("blocked command must exit with code 1: %v\n%s", runErr, stderr.String())
			}

			want := "Error: Another AgentUp operation is running.\n\nWait for it to finish, then retry:\n  agentup upgrade codex\n"
			if tt.ui {
				action, retry := "updating", "agentup update"
				if tt.name == "uninstall" {
					action, retry = "uninstalling", "agentup uninstall"
				}
				want = "Error: Close AgentUp UI before " + action + ".\n\nRun:\n  agentup ui stop --all\n  " +
					retry + "\n\nFor older UIs, press Ctrl+C in the UI terminal.\n"
			}
			text := stderr.String()
			if tt.verbose {
				want += "\nDetails: " + operation.ErrBusy.Error()
				if !strings.HasPrefix(text, want) {
					t.Fatalf("verbose command lost its message or diagnostic cause: %q", text)
				}
			} else if text != want {
				t.Fatalf("unexpected command error:\ngot:  %q\nwant: %q", text, want)
			}
			if stdout.Len() != 0 || strings.Contains(text, "\x1b") {
				t.Fatalf("blocked command must use stderr without ANSI: stdout=%q, stderr=%q", stdout.String(), text)
			}
		})
	}
}
