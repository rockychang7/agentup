package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/operation"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/selfupdate"
	"agentup/internal/upgrader"
)

func setupCommandTest(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, *runner.MockRunner) {
	t.Helper()
	isolateLocks(t)
	oldFlags := outputFlags
	oldRunner, oldPlatform, oldTools := appRunner, appPlatform, appTools
	oldDetector, oldUpgrader := appDetector, appUpgrader
	t.Cleanup(func() {
		outputFlags = oldFlags
		appRunner, appPlatform, appTools = oldRunner, oldPlatform, oldTools
		appDetector, appUpgrader = oldDetector, oldUpgrader
	})
	outputFlags.JSON, outputFlags.Offline, outputFlags.Verbose = false, false, false
	outputFlags.Color = "auto"
	mr := runner.NewMockRunner()
	appRunner, appPlatform, appTools = mr, platform.NewPlatform(), config.DefaultTools()
	appDetector = detector.New(mr, appPlatform, appTools)
	local := detector.NewWithoutLatestCheck(mr, appPlatform, appTools)
	appUpgrader = upgrader.New(mr, appPlatform, local, appTools)
	out, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	return cmd, out, stderr, mr
}

func isolateLocks(t *testing.T) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestMutationLockBlocksCLIUpgrade(t *testing.T) {
	cmd, out, stderr, mr := setupCommandTest(t)
	installMockCodex(mr)
	release, err := operation.Acquire("mutation")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, tt := range []struct {
		name, retry string
		run         func() error
	}{
		{"upgrade all", "agentup upgrade", func() error { return upgradeCmd.RunE(cmd, nil) }},
		{"upgrade codex", "agentup upgrade codex", func() error { return upgradeCmd.RunE(cmd, []string{"codex"}) }},
		{"upgrade claude", "agentup upgrade claude-code", func() error { return upgradeCmd.RunE(cmd, []string{"claude"}) }},
		{"upgrade agentup", "agentup update", func() error { return upgradeCmd.RunE(cmd, []string{"agentup"}) }},
		{"update", "agentup update", func() error { return runSelfUpdate(cmd, false) }},
		{"uninstall", "agentup uninstall", func() error { return uninstallCmd.RunE(cmd, nil) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stderr.Reset()
			err := tt.run()
			if !errors.Is(err, operation.ErrBusy) || err.Error() != "Another AgentUp operation is running." {
				t.Fatalf("busy operation must return a concise error and preserve ErrBusy: %v", err)
			}
			var actionable actionableError
			if !errors.As(err, &actionable) || !strings.Contains(actionable.hint, "Wait for it to finish") {
				t.Fatalf("busy operation must explain how to retry: %v", err)
			}
			printError(cmd, err)
			want := "Error: Another AgentUp operation is running.\n\nWait for it to finish, then retry:\n  " + tt.retry + "\n"
			if stderr.String() != want {
				t.Fatalf("unexpected busy operation output:\ngot:  %q\nwant: %q", stderr.String(), want)
			}
		})
	}
	if out.Len() != 0 {
		t.Fatalf("blocked operation wrote to stdout: %s", out.String())
	}
	if len(mr.Calls) != 0 {
		t.Fatal("busy upgrade executed commands")
	}
}

func TestUIBlocksSelfUpdateAndUninstall(t *testing.T) {
	cmd, out, stderr, mr := setupCommandTest(t)
	release, err := operation.AcquireShared("ui")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, tt := range []struct {
		name, action, retry string
		run                 func() error
	}{
		{"update", "updating", "agentup update", func() error { return runSelfUpdate(cmd, false) }},
		{"upgrade agentup", "updating", "agentup update", func() error { return upgradeCmd.RunE(cmd, []string{"agentup"}) }},
		{"uninstall", "uninstalling", "agentup uninstall", func() error { return uninstallCmd.RunE(cmd, nil) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stderr.Reset()
			err := tt.run()
			message := "Close AgentUp UI before " + tt.action + "."
			if !errors.Is(err, operation.ErrBusy) || err.Error() != message {
				t.Fatalf("UI lock must return a concise error and preserve ErrBusy: %v", err)
			}
			printError(cmd, err)
			want := "Error: " + message + "\n\nRun:\n  agentup ui stop --all\n  " + tt.retry +
				"\n\nFor older UIs, press Ctrl+C in the UI terminal.\n"
			if stderr.String() != want {
				t.Fatalf("unexpected UI conflict output:\ngot:  %q\nwant: %q", stderr.String(), want)
			}
		})
	}
	if out.Len() != 0 {
		t.Fatalf("blocked operation wrote to stdout: %s", out.String())
	}
	if len(mr.Calls) != 0 {
		t.Fatal("blocked operation executed commands")
	}
}

func TestActionableErrorDetailsOnlyAppearInVerboseOutput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		verbose bool
		json    bool
		width   int
	}{
		{name: "default"},
		{name: "verbose", verbose: true},
		{name: "json", json: true},
		{name: "json verbose", json: true, verbose: true},
		{name: "narrow", width: 40},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, out, stderr, _ := setupCommandTest(t)
			outputFlags.Verbose, outputFlags.JSON, outputFlags.Width = tt.verbose, tt.json, tt.width
			if tt.json {
				outputFlags.Color = "always"
			}
			cause := fmt.Errorf("%w: system-lock-detail", operation.ErrBusy)
			err := actionableError{
				message: "Close AgentUp UI before updating.",
				hint:    "Run:\n  agentup ui stop --all\n  agentup update",
				cause:   cause,
			}
			printError(cmd, err)
			text := stderr.String()
			if strings.Contains(text, "system-lock-detail") != tt.verbose ||
				strings.Contains(text, "Details:") != tt.verbose {
				t.Fatalf("verbose=%t produced unexpected diagnostics: %q", tt.verbose, text)
			}
			if !strings.Contains(text, "\n  agentup ui stop --all\n  agentup update\n") {
				t.Fatalf("recovery commands must stay on separate lines: %q", text)
			}
			if out.Len() != 0 || strings.Contains(text, "\x1b") {
				t.Fatalf("error output must stay on stderr without ANSI: stdout=%q, stderr=%q", out.String(), text)
			}
			if tt.width > 0 {
				for _, line := range strings.Split(text, "\n") {
					if len(line) > tt.width {
						t.Fatalf("error line exceeds width %d: %q", tt.width, line)
					}
				}
			}
		})
	}
}

func TestPrintErrorDoesNotDuplicateReportedErrors(t *testing.T) {
	cmd, _, stderr, _ := setupCommandTest(t)
	for _, err := range []error{
		reportedError{"already printed"},
		fmt.Errorf("wrapped: %w", reportedError{"already printed"}),
	} {
		printError(cmd, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("reported errors were printed again: %q", stderr.String())
	}
	printError(cmd, errors.New("network unavailable"))
	if stderr.String() != "Error: network unavailable\n" {
		t.Fatalf("ordinary errors changed: %q", stderr.String())
	}
}

func TestUILockFilesystemErrorsAreNotReportedAsBusy(t *testing.T) {
	cmd, _, _, _ := setupCommandTest(t)
	cacheFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(cacheFile, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"LOCALAPPDATA", "XDG_CACHE_HOME", "HOME"} {
		t.Setenv(name, cacheFile)
	}
	for _, tt := range []struct {
		name string
		run  func() error
	}{
		{"update", func() error { return runSelfUpdate(cmd, false) }},
		{"uninstall", func() error { return uninstallCmd.RunE(cmd, nil) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			var pathError *os.PathError
			var actionable actionableError
			if err == nil || !errors.As(err, &pathError) || errors.As(err, &actionable) ||
				errors.Is(err, operation.ErrBusy) {
				t.Fatalf("filesystem error must retain its cause, not a UI-busy hint: %v", err)
			}
		})
	}
}

func installMockCodex(mr *runner.MockRunner) {
	mr.SetLookPath("codex", "/mock/codex", nil)
	mr.SetResult("/mock/codex", []string{"--version"}, runner.MockResult{Stdout: "1.0.0"})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "11.0.0"})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{Stdout: "@openai/codex@1.0.0"})
}

func TestListOfflineJSONHasNoProgressOrRegistryCalls(t *testing.T) {
	cmd, out, stderr, mr := setupCommandTest(t)
	installMockCodex(mr)
	outputFlags.Offline, outputFlags.JSON = true, true
	if err := runList(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var infos []model.ToolInfo
	if err := json.Unmarshal(out.Bytes(), &infos); err != nil {
		t.Fatalf("stdout is not JSON: %v: %s", err, out.String())
	}
	if len(infos) != 4 || infos[0].LatestCheck != model.CheckOffline {
		t.Fatalf("unexpected offline detection: %+v", infos)
	}
	if stderr.Len() != 0 || strings.Contains(out.String(), "\x1b") {
		t.Fatal("JSON mode must suppress progress and ANSI")
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && call.Args[0] == "view" {
			t.Fatalf("offline list queried the registry: %+v", call)
		}
	}
}

func TestListCheckFailureIsNotUpToDate(t *testing.T) {
	cmd, out, _, mr := setupCommandTest(t)
	installMockCodex(mr)
	mr.SetResult("npm", []string{"view", "@openai/codex", "version"}, runner.MockResult{ExitCode: 1, Stderr: "network unavailable"})
	if err := runList(cmd, nil); err == nil {
		t.Fatal("failed lookup must return an error")
	}
	if !strings.Contains(out.String(), "Check failed") || strings.Contains(out.String(), "Up to date") {
		t.Fatalf("check failure must remain distinct: %s", out.String())
	}
}

func TestUpgradeFailureReturnsErrorWithValidJSON(t *testing.T) {
	cmd, out, stderr, mr := setupCommandTest(t)
	installMockCodex(mr)
	mr.SetResult("npm", []string{"install", "-g", "@openai/codex@latest"}, runner.MockResult{ExitCode: 1, Stderr: "EACCES"})
	outputFlags.JSON = true
	if err := upgradeCmd.RunE(cmd, []string{"codex"}); err == nil {
		t.Fatal("failed upgrade must return an error")
	}
	var results []model.UpgradeResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out.String())
	}
	if len(results) != 1 || results[0].Outcome != model.OutcomeFailed || results[0].Log != "EACCES" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON progress leaked to stderr: %s", stderr.String())
	}
}

func TestUpgradeInvalidNameReturnsError(t *testing.T) {
	cmd, out, _, _ := setupCommandTest(t)
	if err := upgradeCmd.RunE(cmd, []string{"invalid"}); err == nil {
		t.Fatal("unsupported tool must fail")
	}
	if out.Len() > 0 {
		t.Fatal("invalid name should not start an upgrade")
	}
}

func TestJSONDisablesForcedColorOnErrors(t *testing.T) {
	cmd, _, stderr, _ := setupCommandTest(t)
	outputFlags.JSON, outputFlags.Color = true, "always"
	notices(cmd).Error("invalid argument")
	if strings.Contains(stderr.String(), "\x1b") {
		t.Fatalf("JSON mode leaked styles on stderr: %q", stderr.String())
	}
}

func TestClaudeAlias(t *testing.T) {
	cmd, out, _, _ := setupCommandTest(t)
	if err := upgradeCmd.RunE(cmd, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "claude-code") || !strings.Contains(out.String(), "skipped") {
		t.Fatalf("alias did not route to claude-code: %s", out.String())
	}
}

func TestRootHasReadOnlyOverview(t *testing.T) {
	cmd, out, _, _ := setupCommandTest(t)
	outputFlags.Offline = true
	if rootCmd.RunE == nil {
		t.Fatal("root command must show status")
	}
	if err := rootCmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "AgentUp") || !strings.Contains(out.String(), "0 installed") {
		t.Fatalf("unexpected overview: %s", out.String())
	}
}

func TestUnsafeFlagCombinationsRejected(t *testing.T) {
	cmd, _, _, _ := setupCommandTest(t)
	cmd.Use = "upgrade"
	outputFlags.Offline = true
	if err := rootCmd.PersistentPreRunE(cmd, nil); err == nil {
		t.Fatal("offline flag must not allow mutations")
	}
	outputFlags.Offline, outputFlags.Color = false, "invalid"
	if err := rootCmd.PersistentPreRunE(cmd, nil); err == nil {
		t.Fatal("invalid color mode must fail")
	}
}

type unverifiedUpgradeRunner struct {
	*runner.MockRunner
	upgraded bool
}

func (r *unverifiedUpgradeRunner) Run(name string, args ...string) (string, string, int, error) {
	if name == "npm" && len(args) > 0 && args[0] == "install" {
		r.upgraded = true
		return "", "", 0, nil
	}
	if r.upgraded && name == "/mock/codex" {
		return "", "version unavailable", 1, nil
	}
	return r.MockRunner.Run(name, args...)
}

func TestUpgradeUnverifiedReturnsErrorWithValidJSON(t *testing.T) {
	cmd, out, stderr, mr := setupCommandTest(t)
	installMockCodex(mr)
	r := &unverifiedUpgradeRunner{MockRunner: mr}
	local := detector.NewWithoutLatestCheck(r, appPlatform, appTools)
	appUpgrader = upgrader.New(r, appPlatform, local, appTools)
	outputFlags.JSON = true
	err := upgradeCmd.RunE(cmd, []string{"codex"})
	var reported reportedError
	if !errors.As(err, &reported) {
		t.Fatalf("unverified upgrade must report a failure: %v", err)
	}
	var results []model.UpgradeResult
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out.String())
	}
	if len(results) != 1 || results[0].Outcome != model.OutcomeUnverified || results[0].Status != model.UpgradeStatusUnverified {
		t.Fatalf("unexpected result: %+v", results)
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON progress leaked to stderr: %s", stderr.String())
	}
}

func TestSelfUpdateJSON(t *testing.T) {
	for _, tt := range []struct {
		name   string
		result selfupdate.Result
		err    error
	}{
		{name: "updated", result: selfupdate.Result{CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Updated: true}},
		{name: "unchanged", result: selfupdate.Result{CurrentVersion: "1.1.0", LatestVersion: "1.1.0", SkippedReason: "already up to date"}},
		{name: "failed", err: errors.New("network unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, out, stderr, _ := setupCommandTest(t)
			originalUpdater := appSelfUpdater
			t.Cleanup(func() { appSelfUpdater = originalUpdater })
			fake := &fakeSelfUpdater{result: tt.result, err: tt.err}
			appSelfUpdater = fake
			outputFlags.JSON, outputFlags.Color = true, "always"
			err := runSelfUpdate(cmd, false)
			if (err != nil) != (tt.err != nil) {
				t.Fatalf("unexpected error: %v", err)
			}
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("invalid JSON: %v: %s", err, out.String())
			}
			if result["name"] != "agentup" || !fake.called {
				t.Fatalf("unexpected self-update result: %+v", result)
			}
			if tt.err != nil && (result["status"] != "failed" || result["error"] != tt.err.Error()) {
				t.Fatalf("failure was not reported in JSON: %+v", result)
			}
			if tt.err == nil && result["updated"] != tt.result.Updated {
				t.Fatalf("incorrect updated flag: %+v", result)
			}
			if stderr.Len() != 0 || strings.Contains(out.String(), "\x1b") {
				t.Fatal("JSON self-update leaked progress or ANSI")
			}
		})
	}
}

func TestDoctorCanceledJSONReturnsError(t *testing.T) {
	cmd, out, stderr, _ := setupCommandTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	outputFlags.JSON = true
	if err := doctorCmd.RunE(cmd, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled doctor must return an error: %v", err)
	}
	var result model.DoctorResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out.String())
	}
	if stderr.Len() != 0 {
		t.Fatal("JSON doctor leaked progress")
	}
}
