package detector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestDetectOpenCodePackageManagers(t *testing.T) {
	for _, version := range []string{"1.19.0", "2.0.29"} {
		for _, manager := range []string{"npm", "pnpm", "yarn"} {
			t.Run(version+"/"+manager, func(t *testing.T) {
				mr := runner.NewMockRunner()
				mr.SetLookPath("opencode", "/mock/opencode", nil)
				mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: version})
				mr.SetResult(manager, []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
				pkg := "opencode-ai"
				if strings.HasPrefix(version, "2.") {
					pkg = "@opencode/cli"
					// A leftover V1 package must not claim the active V2 executable.
					mr.SetResult("npm", []string{"--version"}, runner.MockResult{Stdout: "10.0.0"})
					mr.SetResult("npm", []string{"list", "-g", "opencode-ai", "--depth=0"}, runner.MockResult{Stdout: "opencode-ai@1.19.0"})
				}
				args := []string{"list", "-g", pkg, "--depth=0"}
				if manager == "yarn" {
					args = []string{"global", "list", "--pattern", pkg}
				}
				mr.SetResult(manager, args, runner.MockResult{Stdout: pkg + "@" + version})
				d := NewWithoutLatestCheck(mr, macTestPlatform(), config.DefaultTools())
				info, err := d.Detect(model.ToolOpenCode)
				if err != nil || info.InstallMethod != model.InstallMethod(manager) || !info.UpgradeSupported {
					t.Fatalf("wrong installation: %+v, %v", info, err)
				}
			})
		}
	}
}

func TestDetectOpenCodeV2Brew(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetLookPath("opencode", "/opt/homebrew/bin/opencode", nil)
	mr.SetResult("/opt/homebrew/bin/opencode", []string{"--version"}, runner.MockResult{Stdout: "2.0.29"})
	mr.SetResult("brew", []string{"--version"}, runner.MockResult{Stdout: "Homebrew 4.0.0"})
	mr.SetResult("brew", []string{"list", "anomalyco/tap/opencode-v2"}, runner.MockResult{
		Stdout: "/opt/homebrew/Cellar/opencode-v2/2.0.29/bin/opencode",
	})
	info, err := NewWithoutLatestCheck(mr, macTestPlatform(), config.DefaultTools()).Detect(model.ToolOpenCode)
	if err != nil || info.InstallMethod != model.InstallMethodBrew || !info.UpgradeSupported {
		t.Fatalf("tap formula was not detected: %+v, %v", info, err)
	}
}

func TestDetectOpenCodeNativeAndUnsupported(t *testing.T) {
	for _, tt := range []struct {
		version, path string
		method        model.InstallMethod
		supported     bool
	}{
		{"1.19.0", "/home/test/.opencode/bin/opencode", model.InstallMethodSelfUpdate, true},
		{"2.0.29", "/home/test/.opencode/bin/opencode", model.InstallMethodSelfUpdate, true},
		{"2.0.29", `C:\Users\test\.opencode\bin\opencode.exe`, model.InstallMethodSelfUpdate, true},
		{"2.0.29", "/usr/local/bin/opencode", model.InstallMethodBinary, false},
		{"2.0.29", "/home/other/.opencode/bin/opencode", model.InstallMethodBinary, false},
		{"2.0.29", "C:/Users/test/scoop/shims/opencode.exe", model.InstallMethodBinary, false},
		{"unknown", "/home/test/.opencode/bin/opencode", model.InstallMethodUnknown, false},
		{"3.0.0", "/home/test/.opencode/bin/opencode", model.InstallMethodUnknown, false},
		{"2.1.0-beta.1", "/home/test/.opencode/bin/opencode", model.InstallMethodUnknown, false},
	} {
		t.Run(tt.version+"/"+tt.path, func(t *testing.T) {
			home := "/home/test"
			if strings.HasPrefix(tt.path, "C:") {
				home = `C:\Users\test`
			}
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			mr := runner.NewMockRunner()
			mr.SetLookPath("opencode", tt.path, nil)
			mr.SetResult(tt.path, []string{"--version"}, runner.MockResult{Stdout: tt.version})
			mr.SetResult("scoop", []string{"--version"}, runner.MockResult{Stdout: "0.5.0"})
			mr.SetResult("scoop", []string{"list"}, runner.MockResult{Stdout: "opencode 1.19.0"})
			info, err := NewWithoutLatestCheck(mr, winTestPlatform(), config.DefaultTools()).Detect(model.ToolOpenCode)
			if err != nil || info.InstallMethod != tt.method || info.UpgradeSupported != tt.supported {
				t.Fatalf("wrong native classification: %+v, %v", info, err)
			}
			if tt.method == model.InstallMethodSelfUpdate && len(mr.Calls) != 1 {
				t.Fatalf("native detection queried package managers: %v", mr.Calls)
			}
		})
	}
}

func TestOpenCodeNativePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	native := filepath.Join(home, ".opencode", "bin", "opencode")
	for _, tt := range []struct {
		path string
		want bool
	}{
		{native, true},
		{native + ".exe", true},
		{filepath.Join(home, "other", ".opencode", "bin", "opencode"), false},
		{filepath.Join(home, ".opencode", "bin", "opencode2"), false},
		{strings.ToUpper(native), runtime.GOOS == "windows"},
	} {
		if got := openCodeNativePath(tt.path); got != tt.want {
			t.Errorf("openCodeNativePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if openCodeNativePath(native) {
		t.Fatal("missing home directory was treated as an official installation")
	}
}

func TestDetectOpenCodeAlias(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetLookPath("opencode2", "/mock/opencode2", nil)
	mr.SetResult("/mock/opencode2", []string{"--version"}, runner.MockResult{Stdout: "2.0.29"})
	d := NewWithoutLatestCheck(mr, macTestPlatform(), config.DefaultTools())
	info, _ := d.Detect(model.ToolOpenCode)
	if !info.Installed || info.Path != "/mock/opencode2" || info.Version != "2.0.29" {
		t.Fatalf("V2 alias not found: %+v", info)
	}
	mr.SetLookPath("opencode", "/mock/opencode", nil)
	mr.SetResult("/mock/opencode", []string{"--version"}, runner.MockResult{Stdout: "1.19.0"})
	info, _ = d.Detect(model.ToolOpenCode)
	if info.Path != "/mock/opencode" || info.Version != "1.19.0" {
		t.Fatalf("alias replaced the active command: %+v", info)
	}
}

func TestCheckLatestOpenCodeReleaseLines(t *testing.T) {
	for _, tt := range []struct {
		version, pkg, latest string
		want                 model.CheckStatus
	}{
		{"1.18.0", "opencode-ai", "1.19.0", model.CheckOK},
		{"2.0.28", "@opencode/cli", "2.0.29", model.CheckOK},
		{"1.19.0", "opencode-ai", "2.0.29", model.CheckFailed},
		{"2.0.28", "@opencode/cli", "1.19.0", model.CheckFailed},
		{"2.0.28", "@opencode/cli", "2.1.0-beta.1", model.CheckFailed},
		{"unknown", "", "", model.CheckUnavailable},
	} {
		t.Run(tt.version+"/"+tt.latest, func(t *testing.T) {
			mr := runner.NewMockRunner()
			mr.SetResult("npm", []string{"view", tt.pkg, "version"}, runner.MockResult{Stdout: tt.latest})
			d := New(mr, macTestPlatform(), config.DefaultTools())
			info := d.CheckLatest(model.ToolInfo{Name: model.ToolOpenCode, Installed: true,
				Version: tt.version, InstallMethod: model.InstallMethodNpm})
			if info.LatestCheck != tt.want {
				t.Fatalf("wrong lookup result: %+v", info)
			}
			if tt.want == model.CheckOK && (info.LatestVersion != tt.latest || !info.UpgradeAvailable) {
				t.Fatalf("wrong latest version: %+v", info)
			}
			if tt.want != model.CheckOK && (info.LatestVersion != "" || info.UpgradeAvailable) {
				t.Fatalf("unsafe release-line result: %+v", info)
			}
			if tt.pkg == "" && len(mr.Calls) != 0 {
				t.Fatal("unknown version queried a guessed package")
			}
		})
	}
}

func TestCheckLatestOpenCodeBrewTap(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetResult("brew", []string{"info", "--json=v2", "anomalyco/tap/opencode-v2"}, runner.MockResult{
		Stdout: `{"formulae":[{"name":"opencode-v2","full_name":"anomalyco/tap/opencode-v2","versions":{"stable":"2.0.29"}}]}`,
	})
	info := New(mr, macTestPlatform(), config.DefaultTools()).CheckLatest(model.ToolInfo{
		Name: model.ToolOpenCode, Installed: true, Version: "2.0.28", InstallMethod: model.InstallMethodBrew,
	})
	if info.LatestCheck != model.CheckOK || info.LatestVersion != "2.0.29" || !info.UpgradeAvailable {
		t.Fatalf("tap lookup failed: %+v", info)
	}
}

type openCodeTransport func(*http.Request) (*http.Response, error)

func (f openCodeTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCheckLatestOpenCodeNative(t *testing.T) {
	for _, tt := range []struct {
		version, pkg, body string
		status             int
		want               model.CheckStatus
	}{
		{"1.18.0", "opencode-ai", `{"version":"1.19.0"}`, 200, model.CheckOK},
		{"2.0.28", "@opencode/cli", `{"version":"2.0.29"}`, 200, model.CheckOK},
		{"2.0.28", "@opencode/cli", `{"version":"1.19.0"}`, 200, model.CheckFailed},
		{"2.0.28", "@opencode/cli", `{"version":"unknown"}`, 200, model.CheckFailed},
		{"2.0.28", "@opencode/cli", `<html>proxy error</html>`, 200, model.CheckFailed},
		{"2.0.28", "@opencode/cli", `{"error":"not found"}`, 404, model.CheckFailed},
		{"2.0.28", "@opencode/cli", strings.Repeat(" ", (1<<20)+1), 200, model.CheckFailed},
	} {
		t.Run(fmt.Sprintf("%s/%d/%s", tt.version, tt.status, tt.want), func(t *testing.T) {
			mr := runner.NewMockRunner()
			d := New(mr, macTestPlatform(), config.DefaultTools())
			var requested []string
			d.httpClient = &http.Client{Transport: openCodeTransport(func(req *http.Request) (*http.Response, error) {
				requested = append(requested, req.URL.Path)
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body)), Header: http.Header{}}, nil
			})}
			info := d.CheckLatest(model.ToolInfo{Name: model.ToolOpenCode, Installed: true,
				Version: tt.version, InstallMethod: model.InstallMethodSelfUpdate})
			if info.LatestCheck != tt.want || !reflect.DeepEqual(requested, []string{"/" + tt.pkg + "/latest"}) {
				t.Fatalf("wrong native lookup: %+v, paths %v", info, requested)
			}
			if len(mr.Calls) != 0 {
				t.Fatal("native lookup requires an external package manager")
			}
		})
	}
}

func TestOpenCodeNativeLookupCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := New(&runner.DefaultRunner{Context: ctx}, macTestPlatform(), config.DefaultTools())
	_, err := d.checkOpenCodeNativeLatest(*config.FindTool(model.ToolOpenCode))
	if err == nil {
		t.Fatal("canceled version lookup succeeded")
	}
}
