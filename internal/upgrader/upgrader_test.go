package upgrader

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/runner"
)

// testPlatform is a mock Platform for testing.
type testPlatform struct {
	name     string
	managers []string
}

func (p *testPlatform) Name() string                { return p.name }
func (p *testPlatform) SupportedManagers() []string { return p.managers }
func (p *testPlatform) IsBrewSupported() bool       { return false }
func (p *testPlatform) IsWingetSupported() bool      { return false }
func (p *testPlatform) IsScoopSupported() bool       { return false }

func macPlatform() *testPlatform {
	return &testPlatform{
		name:     "macOS",
		managers: []string{"npm", "pnpm", "yarn", "brew"},
	}
}

func winPlatform() *testPlatform {
	return &testPlatform{
		name:     "Windows",
		managers: []string{"npm", "pnpm", "yarn", "winget", "scoop"},
	}
}

// setupNpmCodexInstalled configures a MockRunner where codex is installed via npm.
func setupNpmCodexInstalled(mr *runner.MockRunner) {
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.10.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "@openai/codex@0.12.3\n",
		ExitCode: 0,
	})
}

func TestUpgrade_NpmToolSuccess(t *testing.T) {
	mr := runner.NewMockRunner()
	setupNpmCodexInstalled(mr)

	// Upgrade command succeeds
	mr.SetResult("npm", []string{"install", "-g", "@openai/codex@latest"}, runner.MockResult{
		Stdout:   "added 1 package\n",
		ExitCode: 0,
	})

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	result, err := up.Upgrade(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != model.UpgradeStatusSuccess {
		t.Errorf("expected status 'success', got '%s' (msg: %s)", result.Status, result.Message)
	}
	if result.OldVersion != "0.12.3" {
		t.Errorf("expected old version '0.12.3', got '%s'", result.OldVersion)
	}

	// Verify the upgrade command was called
	found := false
	for _, call := range mr.Calls {
		if call.Name == "npm" && len(call.Args) >= 3 && call.Args[0] == "install" && call.Args[1] == "-g" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected npm install -g command to be called")
	}
}

func TestUpgrade_NotInstalledToolSkipped(t *testing.T) {
	mr := runner.NewMockRunner()
	// No LookPath result set — tool not found

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	result, err := up.Upgrade(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != model.UpgradeStatusSkipped {
		t.Errorf("expected status 'skipped', got '%s'", result.Status)
	}
	if result.Message != "not installed" {
		t.Errorf("expected message 'not installed', got '%s'", result.Message)
	}
}

func TestUpgrade_AgySelfUpdateSuccess(t *testing.T) {
	mr := runner.NewMockRunner()

	// agy is installed as binary
	mr.SetLookPath("agy", "/usr/local/bin/agy", nil)
	mr.SetResult("/usr/local/bin/agy", []string{"--version"}, runner.MockResult{
		Stdout:   "1.0.16\n",
		ExitCode: 0,
	})
	// npm available but agy has no npm package
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})
	// agy update command succeeds
	mr.SetResult("/usr/local/bin/agy", []string{"update"}, runner.MockResult{
		Stdout:   "Updated to version 1.0.17\n",
		ExitCode: 0,
	})

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	result, err := up.Upgrade(model.ToolAgy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != model.UpgradeStatusSuccess {
		t.Errorf("expected status 'success', got '%s' (msg: %s)", result.Status, result.Message)
	}
	if result.OldVersion != "1.0.16" {
		t.Errorf("expected old version '1.0.16', got '%s'", result.OldVersion)
	}

	// Verify the agy update command was called
	found := false
	for _, call := range mr.Calls {
		if call.Name == "/usr/local/bin/agy" && len(call.Args) >= 1 && call.Args[0] == "update" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'agy update' command to be called")
	}
}

func TestUpgrade_UpgradeCommandFails(t *testing.T) {
	mr := runner.NewMockRunner()
	setupNpmCodexInstalled(mr)

	// Upgrade command fails with permission error
	mr.SetResult("npm", []string{"install", "-g", "@openai/codex@latest"}, runner.MockResult{
		Stderr:   "EACCES: permission denied",
		ExitCode: 1,
	})

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	result, err := up.Upgrade(model.ToolCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != model.UpgradeStatusFailed {
		t.Errorf("expected status 'failed', got '%s'", result.Status)
	}
	if result.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestUpgrade_BinaryInstallMethodSkipped(t *testing.T) {
	mr := runner.NewMockRunner()

	// codex is installed but via manual binary (no npm/brew match)
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	// npm available but codex not in npm
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout: "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		ExitCode: 1,
	})
	// brew not available
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{
		ExitCode: -1,
	})

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	result, _ := up.Upgrade(model.ToolCodex)
	if result.Status != model.UpgradeStatusSkipped {
		t.Errorf("expected status 'skipped', got '%s'", result.Status)
	}
}

func TestUpgradeAll_OneFailureDoesNotAffectOthers(t *testing.T) {
	mr := runner.NewMockRunner()

	// codex installed via npm, upgrade succeeds
	mr.SetLookPath("codex", "/usr/local/bin/codex", nil)
	mr.SetResult("/usr/local/bin/codex", []string{"--version"}, runner.MockResult{
		Stdout:   "0.12.3\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{
		Stdout:   "11.0.0\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@openai/codex", "--depth=0"}, runner.MockResult{
		Stdout:   "@openai/codex@0.12.3\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"install", "-g", "@openai/codex@latest"}, runner.MockResult{
		ExitCode: 0,
	})

	// claude-code installed via npm, upgrade fails
	mr.SetLookPath("claude", "/usr/local/bin/claude", nil)
	mr.SetResult("/usr/local/bin/claude", []string{"--version"}, runner.MockResult{
		Stdout:   "1.0.21\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"list", "-g", "@anthropic-ai/claude-code", "--depth=0"}, runner.MockResult{
		Stdout:   "@anthropic-ai/claude-code@1.0.21\n",
		ExitCode: 0,
	})
	mr.SetResult("npm", []string{"install", "-g", "@anthropic-ai/claude-code@latest"}, runner.MockResult{
		Stderr:   "network error",
		ExitCode: 1,
	})

	// opencode and agy not installed

	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	results := up.UpgradeAll()

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// Find codex and claude-code results
	var codexResult, claudeResult *model.UpgradeResult
	for i := range results {
		switch results[i].Name {
		case model.ToolCodex:
			codexResult = &results[i]
		case model.ToolClaudeCode:
			claudeResult = &results[i]
		}
	}

	if codexResult == nil || codexResult.Status != model.UpgradeStatusSuccess {
		if codexResult != nil {
			t.Errorf("expected codex to succeed, got '%s' (msg: %s)", codexResult.Status, codexResult.Message)
		} else {
			t.Error("codex result not found")
		}
	}

	if claudeResult == nil || claudeResult.Status != model.UpgradeStatusFailed {
		if claudeResult != nil {
			t.Errorf("expected claude-code to fail, got '%s' (msg: %s)", claudeResult.Status, claudeResult.Message)
		} else {
			t.Error("claude-code result not found")
		}
	}
}

func TestUpgrade_UnsupportedToolReturnsError(t *testing.T) {
	mr := runner.NewMockRunner()
	tools := config.DefaultTools()
	p := macPlatform()
	det := detector.NewWithoutLatestCheck(mr, p, tools)
	up := New(mr, p, det, tools, io.Discard)

	_, err := up.Upgrade(model.ToolName("nonexistent"))
	if err == nil {
		t.Fatal("expected error for unsupported tool")
	}
}

func TestBuildUpgradeCommand_Npm(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		NpmPackage: "@openai/codex",
	}
	name, args, err := up.buildUpgradeCommand(tool, model.InstallMethodNpm, "/usr/local/bin/codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "npm" {
		t.Errorf("expected 'npm', got '%s'", name)
	}
	if len(args) != 3 || args[0] != "install" || args[1] != "-g" {
		t.Errorf("unexpected args: %v", args)
	}
	if args[2] != "@openai/codex@latest" {
		t.Errorf("expected '@openai/codex@latest', got '%s'", args[2])
	}
}

func TestBuildUpgradeCommand_BrewCask(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		BrewFormula: "codex",
		BrewCask:    true,
	}
	name, args, err := up.buildUpgradeCommand(tool, model.InstallMethodBrew, "/usr/local/bin/codex")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "brew" {
		t.Errorf("expected 'brew', got '%s'", name)
	}
	if len(args) != 3 || args[0] != "upgrade" || args[1] != "--cask" || args[2] != "codex" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildUpgradeCommand_BrewFormula(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		BrewFormula: "opencode",
		BrewCask:    false,
	}
	name, args, err := up.buildUpgradeCommand(tool, model.InstallMethodBrew, "/usr/local/bin/opencode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "brew" {
		t.Errorf("expected 'brew', got '%s'", name)
	}
	if len(args) != 2 || args[0] != "upgrade" || args[1] != "opencode" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildUpgradeCommand_Scoop(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		ScoopPackage: "opencode",
	}
	name, args, err := up.buildUpgradeCommand(tool, model.InstallMethodScoop, "C:/Users/test/scoop/shims/opencode.exe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "scoop" {
		t.Errorf("expected 'scoop', got '%s'", name)
	}
	if len(args) != 2 || args[0] != "update" || args[1] != "opencode" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestBuildUpgradeCommand_BinaryReturnsError(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{}
	_, _, err := up.buildUpgradeCommand(tool, model.InstallMethodBinary, "/usr/local/bin/codex")
	if err == nil {
		t.Fatal("expected error for binary install method")
	}
}

func TestBuildUpgradeCommand_SelfUpdate(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		Name:           model.ToolAgy,
		SelfUpdateArgs: []string{"update"},
	}
	name, args, err := up.buildUpgradeCommand(tool, model.InstallMethodSelfUpdate, "/usr/local/bin/agy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "/usr/local/bin/agy" {
		t.Errorf("expected '/usr/local/bin/agy', got '%s'", name)
	}
	if len(args) != 1 || args[0] != "update" {
		t.Errorf("expected args ['update'], got %v", args)
	}
}

func TestBuildUpgradeCommand_SelfUpdateNoArgsReturnsError(t *testing.T) {
	up := &Upgrader{}
	tool := config.ToolConfig{
		Name: model.ToolAgy,
		// SelfUpdateArgs is empty
	}
	_, _, err := up.buildUpgradeCommand(tool, model.InstallMethodSelfUpdate, "/usr/local/bin/agy")
	if err == nil {
		t.Fatal("expected error for self-update with no args")
	}
}

func TestClassifyError_PermissionDenied(t *testing.T) {
	msg := classifyError(
		fmt.Errorf("exec: permission denied"),
		"",
		"npm",
	)
	if !contains(msg, "permission") {
		t.Errorf("expected permission-related message, got '%s'", msg)
	}
}

func TestClassifyError_NotFound(t *testing.T) {
	msg := classifyError(
		fmt.Errorf("no such file or directory"),
		"",
		"brew",
	)
	if !contains(msg, "not found") {
		t.Errorf("expected not-found message, got '%s'", msg)
	}
}

func TestClassifyExitError_NetworkError(t *testing.T) {
	msg := classifyExitError("ETIMEDOUT: connection timed out", 1, "npm")
	if !contains(msg, "network") {
		t.Errorf("expected network-related message, got '%s'", msg)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
