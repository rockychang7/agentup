package detector

import (
	"testing"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestLatestFailureAndUnavailableAreDifferent(t *testing.T) {
	mr := runner.NewMockRunner()
	d := New(mr, macTestPlatform(), config.DefaultTools())
	failed := d.CheckLatest(model.ToolInfo{Name: model.ToolCodex, Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodNpm})
	unavailable := d.CheckLatest(model.ToolInfo{Name: model.ToolCodex, Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodBinary})
	if failed.LatestCheck != model.CheckFailed || failed.CheckMessage == "" || failed.UpgradeAvailable {
		t.Fatalf("failed check must have a reason: %+v", failed)
	}
	if unavailable.LatestCheck != model.CheckUnavailable {
		t.Fatalf("missing registry lookup is not a failed check: %+v", unavailable)
	}
}

func TestMalformedRegistryResponseNotCurrent(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetResult("npm", []string{"view", "@openai/codex", "version"}, runner.MockResult{Stdout: "network proxy error"})
	d := New(mr, macTestPlatform(), config.DefaultTools())
	info := d.CheckLatest(model.ToolInfo{Name: model.ToolCodex, Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodNpm})
	if info.LatestCheck != model.CheckFailed || info.LatestVersion != "" {
		t.Fatalf("malformed registry output was treated as a version: %+v", info)
	}
}

func TestWingetSourceColumnNotUsedAsVersion(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetResult("winget", []string{"upgrade"}, runner.MockResult{Stdout: "Agent  Vendor.Agent  1.0.0  1.1.0  winget"})
	tool := config.ToolConfig{Name: "test", WingetID: "Vendor.Agent"}
	d := New(mr, winTestPlatform(), []config.ToolConfig{tool})
	info := d.CheckLatest(model.ToolInfo{Name: "test", Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodWinget})
	if info.LatestVersion != "1.1.0" || !info.UpgradeAvailable {
		t.Fatalf("wrong winget version: %+v", info)
	}
}

func TestUnrecognizedScoopStatusCannotClaimLatest(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetResult("scoop", []string{"status"}, runner.MockResult{Stdout: "unexpected response"})
	d := New(mr, winTestPlatform(), config.DefaultTools())
	info := d.CheckLatest(model.ToolInfo{Name: model.ToolOpenCode, Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodScoop})
	if info.LatestCheck != model.CheckFailed {
		t.Fatalf("unexpected response cannot establish latest: %+v", info)
	}
}
