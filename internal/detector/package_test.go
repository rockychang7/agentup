package detector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestDetectBrokenOpenCodePackage(t *testing.T) {
	for _, tt := range []struct {
		version, pkg, shimPkg string
		recoverable           bool
	}{
		{"1.19.0", "opencode-ai", "opencode-ai", true},
		{"2.2.6", "@opencode/cli", "@opencode/cli", true},
		{"2.2.6", "@opencode/cli", "opencode-ai", false},
		{"1.19.0", "@opencode/cli", "@opencode/cli", false},
		{"3.0.0", "@opencode/cli", "@opencode/cli", false},
		{"2.3.0-beta.1", "@opencode/cli", "@opencode/cli", false},
		{"unknown", "@opencode/cli", "@opencode/cli", false},
	} {
		t.Run(tt.version+"/"+tt.pkg+"/"+tt.shimPkg, func(t *testing.T) {
			root := t.TempDir()
			binary := filepath.Join(root, "opencode.cmd")
			dir := filepath.Join(root, "node_modules", filepath.FromSlash(tt.pkg))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			manifest := fmt.Sprintf(`{"name":%q,"version":%q,"bin":{"opencode":"bin/opencode.exe"}}`, tt.pkg, tt.version)
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			shim := `"%dp0%\node_modules\` + strings.ReplaceAll(tt.shimPkg, "/", "\\") + `\bin\opencode.exe" %*`
			if err := os.WriteFile(binary, []byte(shim), 0o644); err != nil {
				t.Fatal(err)
			}
			mr := runner.NewMockRunner()
			mr.SetLookPath("opencode", binary, nil)
			mr.SetResultByName(binary, runner.MockResult{ExitCode: 1, Stderr: "native binary not installed"})
			mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "12.2.2"})
			mr.SetResult("npm", []string{"list", "-g", tt.pkg, "--depth=0"}, runner.MockResult{Stdout: tt.pkg + "@" + tt.version})
			mr.SetResult("npm", []string{"view", tt.pkg, "version"}, runner.MockResult{Stdout: tt.version})
			d := New(mr, winTestPlatform(), config.DefaultTools())
			info, err := d.Detect(model.ToolOpenCode)
			if err != nil || !info.Installed || info.Version != "unknown" || info.UpgradeSupported != tt.recoverable {
				t.Fatalf("incorrect broken launcher detection: %+v, %v", info, err)
			}
			if tt.recoverable {
				if info.PackageVersion != tt.version || info.InstallMethod != model.InstallMethodNpm ||
					info.LatestCheck != model.CheckFailed || info.UpgradeAvailable || !strings.Contains(info.CheckMessage, "reinstall") {
					t.Fatalf("package metadata hid a broken launcher: %+v", info)
				}
			} else if info.PackageVersion != "" {
				t.Fatalf("unrelated/unsupported package trusted: %+v", info)
			}
		})
	}
}

func TestInstalledPackageVersionMatchesActualBinary(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "node_modules", "@opencode", "cli")
	binary := filepath.Join(dir, "bin", "opencode.exe")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"@opencode/cli","version":"2.2.6","bin":{"opencode":"./bin/opencode.exe"}}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := *config.FindTool(model.ToolOpenCode)
	if got := installedPackageVersion(tool, binary); got != "2.2.6" {
		t.Fatalf("direct package binary not recognized: %q", got)
	}
	if got := installedPackageVersion(tool, filepath.Join(dir, "unrelated.exe")); got != "" {
		t.Fatalf("unrelated binary claimed by package: %q", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := installedPackageVersion(tool, binary); got != "" {
		t.Fatalf("malformed metadata trusted: %q", got)
	}
}
