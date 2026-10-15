package upgrader

import (
	"reflect"
	"strings"
	"testing"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestPreviewOpenCodeReleaseLines(t *testing.T) {
	for _, tt := range []struct {
		version string
		method  model.InstallMethod
		want    []string
	}{
		{"1.19.0", model.InstallMethodNpm, []string{"npm", "install", "-g", "opencode-ai@latest"}},
		{"2.0.29", model.InstallMethodNpm, []string{"npm", "install", "-g", "@opencode/cli@latest"}},
		{"1.19.0", model.InstallMethodPnpm, []string{"pnpm", "add", "-g", "opencode-ai"}},
		{"2.0.29", model.InstallMethodPnpm, []string{"pnpm", "add", "-g", "--allow-build=@opencode/cli", "@opencode/cli@latest"}},
		{"1.19.0", model.InstallMethodYarn, []string{"yarn", "global", "upgrade", "opencode-ai"}},
		{"2.0.29", model.InstallMethodYarn, []string{"yarn", "global", "add", "@opencode/cli@latest"}},
		{"1.19.0", model.InstallMethodBrew, []string{"brew", "upgrade", "opencode"}},
		{"2.0.29", model.InstallMethodBrew, []string{"brew", "upgrade", "anomalyco/tap/opencode-v2"}},
		{"1.19.0", model.InstallMethodScoop, []string{"scoop", "update", "opencode"}},
		{"2.0.29", model.InstallMethodScoop, nil},
		{"2.0.29", model.InstallMethodWinget, nil},
		{"1.19.0", model.InstallMethodSelfUpdate, []string{"/mock/opencode", "upgrade", "--method", "curl"}},
		{"2.0.29", model.InstallMethodSelfUpdate, []string{"/mock/opencode", "upgrade", "--method", "curl"}},
		{"unknown", model.InstallMethodNpm, nil},
		{"3.0.0", model.InstallMethodNpm, nil},
		{"2.1.0-beta.1", model.InstallMethodNpm, nil},
	} {
		t.Run(tt.version+"/"+string(tt.method), func(t *testing.T) {
			up := &Upgrader{tools: config.DefaultTools()}
			got, err := up.Preview(model.ToolInfo{Name: model.ToolOpenCode, Version: tt.version,
				InstallMethod: tt.method, Path: "/mock/opencode"})
			if tt.want == nil {
				if err == nil {
					t.Fatalf("unsafe command generated: %v", got)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestUpgradeOpenCodeUsesResolvedPackage(t *testing.T) {
	for _, tt := range []struct{ version, pkg string }{
		{"1.19.0", "opencode-ai"},
		{"2.0.29", "@opencode/cli"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			mr := runner.NewMockRunner()
			mr.SetLookPath("opencode", "/mock/opencode", nil)
			mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: tt.version})
			mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
			mr.SetResult("npm", []string{"list", "-g", tt.pkg, "--depth=0"}, runner.MockResult{Stdout: tt.pkg + "@" + tt.version})
			mr.SetResult("npm", []string{"install", "-g", tt.pkg + "@latest"}, runner.MockResult{})
			tools := config.DefaultTools()
			up := New(mr, macPlatform(), detector.NewWithoutLatestCheck(mr, macPlatform(), tools), tools)
			result, err := up.Upgrade(model.ToolOpenCode)
			if err != nil || result.Status != model.UpgradeStatusSuccess ||
				!reflect.DeepEqual(result.Command, []string{"npm", "install", "-g", tt.pkg + "@latest"}) {
				t.Fatalf("wrong upgrade command: %+v, %v", result, err)
			}
		})
	}
}

func TestUpgradeConfirmedOpenCodeRejectsChangedReleaseLine(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetLookPath("opencode", "/mock/opencode", nil)
	mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: "2.0.29"})
	mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
	mr.SetResult("npm", []string{"list", "-g", "@opencode/cli", "--depth=0"}, runner.MockResult{Stdout: "@opencode/cli@2.0.29"})
	tools := config.DefaultTools()
	up := New(mr, macPlatform(), detector.NewWithoutLatestCheck(mr, macPlatform(), tools), tools)
	result, err := up.UpgradeConfirmed(model.ToolOpenCode, []string{"npm", "install", "-g", "opencode-ai@latest"})
	if err != nil || result.Outcome != model.OutcomeFailed {
		t.Fatalf("stale V1 command was accepted: %+v, %v", result, err)
	}
	for _, call := range mr.Calls {
		if len(call.Args) > 0 && call.Args[0] == "install" {
			t.Fatalf("mutation executed with stale confirmation: %+v", call)
		}
	}
}

func TestUpgradeOpenCodeNative(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("USERPROFILE", "/home/test")
	mr := runner.NewMockRunner()
	binary := "/home/test/.opencode/bin/opencode"
	mr.SetLookPath("opencode", binary, nil)
	mr.SetResult(binary, []string{"--version"}, runner.MockResult{Stdout: "2.0.29"})
	mr.SetResult(binary, []string{"upgrade", "--method", "curl"}, runner.MockResult{})
	tools := config.DefaultTools()
	up := New(mr, macPlatform(), detector.NewWithoutLatestCheck(mr, macPlatform(), tools), tools)
	result, err := up.Upgrade(model.ToolOpenCode)
	if err != nil || result.Status != model.UpgradeStatusSuccess ||
		!reflect.DeepEqual(result.Command, []string{binary, "upgrade", "--method", "curl"}) {
		t.Fatalf("native self-update failed: %+v, %v", result, err)
	}
}

type openCodeUpgradeRunner struct {
	*runner.MockRunner
	after string
}

func (r *openCodeUpgradeRunner) Run(name string, args ...string) (string, string, int, error) {
	stdout, stderr, code, err := r.MockRunner.Run(name, args...)
	if name == "npm" && len(args) > 0 && args[0] == "install" && code == 0 && err == nil {
		r.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: r.after})
	}
	return stdout, stderr, code, err
}

func TestUpgradeOpenCodeVerifiesReleaseLine(t *testing.T) {
	for _, tt := range []struct {
		before, after string
		want          model.UpgradeOutcome
	}{
		{"1.18.0", "1.19.0", model.OutcomeUpdated},
		{"2.0.28", "2.0.29", model.OutcomeUpdated},
		{"2.0.29", "2.0.29", model.OutcomeUnchanged},
		{"1.19.0", "2.0.29", model.OutcomeFailed},
		{"2.0.29", "1.19.0", model.OutcomeFailed},
		{"2.0.29", "2.1.0-beta.1", model.OutcomeFailed},
		{"2.0.29", "3.0.0", model.OutcomeFailed},
		{"2.0.29", "unknown", model.OutcomeUnverified},
	} {
		t.Run(tt.before+"/"+tt.after, func(t *testing.T) {
			mr := runner.NewMockRunner()
			mr.SetLookPath("opencode", "/mock/opencode", nil)
			mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: tt.before})
			mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
			pkg := "opencode-ai"
			if strings.HasPrefix(tt.before, "2.") {
				pkg = "@opencode/cli"
			}
			mr.SetResult("npm", []string{"list", "-g", pkg, "--depth=0"}, runner.MockResult{Stdout: pkg + "@" + tt.before})
			mr.SetResult("npm", []string{"install", "-g", pkg + "@latest"}, runner.MockResult{})
			r := &openCodeUpgradeRunner{MockRunner: mr, after: tt.after}
			tools := config.DefaultTools()
			up := New(r, macPlatform(), detector.NewWithoutLatestCheck(r, macPlatform(), tools), tools)
			result, err := up.Upgrade(model.ToolOpenCode)
			if err != nil || result.Outcome != tt.want || result.OldVersion != tt.before || result.NewVersion != tt.after {
				t.Fatalf("unexpected verification: %+v, %v", result, err)
			}
			if tt.want == model.OutcomeFailed && (result.Status != model.UpgradeStatusFailed || !strings.Contains(result.Message, "verification failed")) {
				t.Fatalf("unsafe release was reported as successful: %+v", result)
			}
		})
	}
}
