package upgrader

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestNativeNpmScriptApproval(t *testing.T) {
	for _, name := range []model.ToolName{model.ToolClaudeCode, model.ToolOpenCode} {
		for _, tt := range []struct {
			npm, policy string
			approved    bool
		}{
			{"10.9.9", "undefined", false},
			{"11.10.0", "undefined", false},
			{"11.21.1", "", true},
			{"12.2.2", "", true},
			{"13.0.0", "", true},
		} {
			t.Run(string(name)+"/"+tt.npm, func(t *testing.T) {
				mr := runner.NewMockRunner()
				mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: tt.npm})
				mr.SetResult("npm", []string{"config", "get", "allow-scripts"}, runner.MockResult{Stdout: tt.policy})
				up := &Upgrader{runner: mr, tools: config.DefaultTools()}
				got, err := up.Preview(model.ToolInfo{Name: name, Version: "2.2.6", InstallMethod: model.InstallMethodNpm})
				pkg := "@anthropic-ai/claude-code"
				if name == model.ToolOpenCode {
					pkg = "@opencode/cli"
				}
				want := []string{"npm", "install", "-g", "--include=optional", "--ignore-scripts=false"}
				if tt.approved {
					want = append(want, "--allow-scripts="+pkg)
				}
				want = append(want, pkg+"@latest")
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("got %v, %v; want %v", got, err, want)
				}
			})
		}
	}
}

func TestNativeNpmRejectsUnknownManagerVersion(t *testing.T) {
	for _, result := range []runner.MockResult{
		{Stdout: "not a version"},
		{ExitCode: 1},
		{Err: fmt.Errorf("timeout")},
	} {
		mr := runner.NewMockRunner()
		mr.SetResult("npm", []string{"--version"}, result)
		up := &Upgrader{runner: mr, tools: config.DefaultTools()}
		if cmd, err := up.Preview(model.ToolInfo{Name: model.ToolClaudeCode, Version: "2.1.322", InstallMethod: model.InstallMethodNpm}); err == nil {
			t.Fatalf("unsafe command generated: %v", cmd)
		}
	}
}

func TestNativePnpmApprovesOnlyOfficialPackage(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetResult("pnpm", []string{"--version"}, runner.MockResult{Stdout: "10.28.1"})
	mr.SetResult("pnpm", []string{"add", "--help"}, runner.MockResult{Stdout: "--allow-build"})
	up := &Upgrader{runner: mr, tools: config.DefaultTools()}
	for _, name := range []model.ToolName{model.ToolClaudeCode, model.ToolOpenCode} {
		command, err := up.Preview(model.ToolInfo{Name: name, Version: "2.2.6", InstallMethod: model.InstallMethodPnpm})
		pkg := "@anthropic-ai/claude-code"
		if name == model.ToolOpenCode {
			pkg = "@opencode/cli"
		}
		want := []string{"pnpm", "add", "-g", "--ignore-scripts=false", "--optional=true", "--allow-build=" + pkg, pkg + "@latest"}
		if err != nil || !reflect.DeepEqual(command, want) {
			t.Fatalf("got %v, %v; want %v", command, err, want)
		}
	}
}

func TestNativePnpmOlderVersions(t *testing.T) {
	for _, tt := range []struct {
		version string
		wantErr bool
	}{
		{"9.15.9", false},
		{"10.0.0", true},
		{"unknown", true},
	} {
		t.Run(tt.version, func(t *testing.T) {
			mr := runner.NewMockRunner()
			mr.SetResult("pnpm", []string{"--version"}, runner.MockResult{Stdout: tt.version})
			mr.SetResult("pnpm", []string{"add", "--help"}, runner.MockResult{Stdout: "--ignore-scripts"})
			up := &Upgrader{runner: mr, tools: config.DefaultTools()}
			command, err := up.Preview(model.ToolInfo{Name: model.ToolClaudeCode, Version: "2.1.322", InstallMethod: model.InstallMethodPnpm})
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected pnpm compatibility: %v, %v", command, err)
			}
			if !tt.wantErr {
				want := []string{"pnpm", "add", "-g", "--ignore-scripts=false", "--optional=true", "@anthropic-ai/claude-code@latest"}
				if !reflect.DeepEqual(command, want) {
					t.Fatalf("old pnpm received unsupported flags: %v", command)
				}
			}
		})
	}
}
func TestNativeNpmConfirmationRejectsManagerPolicyChange(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetLookPath("claude", "/mock/claude", nil)
	mr.SetResult("/mock/claude", []string{"--version"}, runner.MockResult{Stdout: "2.1.322"})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "11.10.0"})
	mr.SetResult("npm", []string{"list", "-g", "@anthropic-ai/claude-code", "--depth=0"}, runner.MockResult{Stdout: "@anthropic-ai/claude-code@2.1.322"})
	tools := config.DefaultTools()
	up := New(mr, macPlatform(), detector.NewWithoutLatestCheck(mr, macPlatform(), tools), tools)
	command, err := up.Preview(model.ToolInfo{Name: model.ToolClaudeCode, Version: "2.1.322", InstallMethod: model.InstallMethodNpm})
	if err != nil {
		t.Fatal(err)
	}
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "12.2.2"})
	result, err := up.UpgradeConfirmed(model.ToolClaudeCode, command)
	if err != nil || result.Outcome != model.OutcomeFailed || !strings.Contains(result.Message, "confirmation") {
		t.Fatalf("stale script approval accepted: %+v, %v", result, err)
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && call.Args[0] == "install" {
			t.Fatalf("mutation executed: %+v", call)
		}
	}
}

func TestUpgradeRestoresBrokenNativeLauncher(t *testing.T) {
	for _, name := range []model.ToolName{model.ToolClaudeCode, model.ToolOpenCode} {
		for _, after := range []string{"2.2.6", "unknown", "1.19.0"} {
			t.Run(string(name)+"/"+after, func(t *testing.T) {
				pkg, binaryName := "@anthropic-ai/claude-code", "claude"
				if name == model.ToolOpenCode {
					pkg, binaryName = "@opencode/cli", "opencode"
				}
				root := t.TempDir()
				binary := filepath.Join(root, binaryName+".cmd")
				dir := filepath.Join(root, "node_modules", filepath.FromSlash(pkg))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				manifest := fmt.Sprintf(`{"name":%q,"version":"2.2.6","bin":{%q:"bin/cli.exe"}}`, pkg, binaryName)
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(binary, []byte(`"%dp0%\node_modules\`+strings.ReplaceAll(pkg, "/", "\\")+`\bin\cli.exe" %*`), 0o644); err != nil {
					t.Fatal(err)
				}
				mr := runner.NewMockRunner()
				mr.SetLookPath(binaryName, binary, nil)
				mr.SetResultByName(binary, runner.MockResult{ExitCode: 1, Stderr: "native binary not installed"})
				mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "12.2.2"})
				mr.SetResult("npm", []string{"list", "-g", pkg, "--depth=0"}, runner.MockResult{Stdout: pkg + "@2.2.6"})
				args := []string{"install", "-g", "--include=optional", "--ignore-scripts=false", "--allow-scripts=" + pkg, pkg + "@latest"}
				mr.SetResult("npm", args, runner.MockResult{})
				r := &nativeRepairRunner{MockRunner: mr, binary: binary, after: after}
				tools := config.DefaultTools()
				up := New(r, macPlatform(), detector.NewWithoutLatestCheck(r, macPlatform(), tools), tools)
				result, err := up.Upgrade(name)
				want := model.OutcomeUpdated
				if after == "unknown" {
					want = model.OutcomeUnverified
				} else if after == "1.19.0" {
					want = model.OutcomeFailed
				}
				if err != nil || result.Outcome != want || result.OldVersion != "unknown" || result.NewVersion != after {
					t.Fatalf("wrong recovery outcome: %+v, %v", result, err)
				}
			})
		}
	}
}

type nativeRepairRunner struct {
	*runner.MockRunner
	binary, after string
}

func (r *nativeRepairRunner) Run(name string, args ...string) (string, string, int, error) {
	out, stderr, code, err := r.MockRunner.Run(name, args...)
	if name == "npm" && len(args) > 0 && args[0] == "install" && code == 0 && err == nil && r.after != "unknown" {
		r.SetResult(r.binary, []string{"--version"}, runner.MockResult{Stdout: r.after})
	}
	return out, stderr, code, err
}
