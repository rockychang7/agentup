package desktop

import (
	"context"
	"debug/macho"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentup/internal/model"
	"agentup/internal/runner"
)

func windowsFixture(t *testing.T, records []windowsRecord) (*Manager, *runner.MockRunner) {
	t.Helper()
	mr := runner.NewMockRunner()
	mr.SetLookPath("powershell.exe", "powershell.exe", nil)
	data, err := json.Marshal(map[string]any{"items": records, "processes": []string{}, "issues": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	mr.SetResult("powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", windowsProbe}, runner.MockResult{Stdout: string(data)})
	return isolated(t, New(context.Background(), mr, "Windows", "amd64")), mr
}

// isolated keeps tests away from the network, the real process table and user directories.
func isolated(t *testing.T, m *Manager) *Manager {
	t.Helper()
	m.fetch = func(context.Context, string, int64) ([]byte, error) {
		return nil, errors.New("network is disabled in tests")
	}
	m.download = func(context.Context, Asset, string) (string, error) {
		return "", errors.New("network is disabled in tests")
	}
	m.receiptDir, m.cacheDir, m.procRoot = t.TempDir(), t.TempDir(), t.TempDir()
	m.desktopDir, m.channels = nil, map[string]bool{"brew-cask": true}
	m.poll, m.verifyWindow = time.Millisecond, 50*time.Millisecond
	return m
}

func productInstallation(t *testing.T, snapshot Snapshot, id string) Installation {
	t.Helper()
	for _, info := range snapshot.Installations {
		if info.ProductID == id && info.Installed {
			return info
		}
	}
	t.Fatalf("installed product %q not found in %+v", id, snapshot)
	return Installation{}
}

func TestWindowsDiscoverySeparatesCopiesAndExcludesIDE(t *testing.T) {
	m, mr := windowsFixture(t, []windowsRecord{
		{Name: "Antigravity 2.18.1", Version: "2.18.1", Publisher: "Google", Path: `C:\Apps\Antigravity.exe`, Scope: "user", Package: "app-user", Source: "installer"},
		{Name: "Antigravity 2.18.1", Version: "2.18.1", Publisher: "Google", Path: `C:\Program Files\Antigravity.exe`, Scope: "system", Package: "app-system", Source: "installer"},
		{Name: "Antigravity 1.25.0", Version: "1.25.0", Publisher: "Google", Path: `C:\IDE\Antigravity.exe`, Source: "installer"},
		{Name: "Antigravity IDE", Version: "2.3.0", Publisher: "Google", Path: `C:\IDE2\Antigravity.exe`, Source: "installer"},
		{Name: "Antigravity", Version: "2.3.0", Publisher: "Unknown", Path: `C:\Other\Antigravity.exe`, Source: "installer"},
		{Name: "OpenAI.Codex", Version: "26.1007.1.0", Package: "OpenAI.Codex", Family: "OpenAI.Codex_2p2nqsd0c76g0", Publisher: "CN=store-publisher", Path: `C:\WindowsApps\Codex`, Scope: "user", Source: "msix"},
		{Name: "OpenAI.Codex", Version: "26.1007.1.0", Package: "OpenAI.Codex", Family: "untrusted", Publisher: "OpenAI", Path: `C:\Fake`, Source: "msix"},
		{Name: "OpenCode CLI", Version: "2.0.29", Publisher: "OpenCode", Path: `C:\CLI\opencode.exe`, Source: "installer"},
	})
	snapshot := m.Scan(false)
	if len(snapshot.Issues) != 0 || len(snapshot.Installations) != 5 {
		t.Fatalf("unexpected discovery: %+v", snapshot)
	}
	installed := 0
	ids := map[string]bool{}
	for _, info := range snapshot.Installations {
		if !info.Installed {
			continue
		}
		installed++
		if ids[info.ID] || strings.HasPrefix(info.ID, "missing:") {
			t.Fatalf("installation identity collapsed copies: %+v", info)
		}
		ids[info.ID] = true
		if strings.Contains(info.Path, "IDE") || strings.Contains(info.Path, "Fake") {
			t.Fatal("IDE or unrecognized package was managed")
		}
		if info.Plan == nil || info.Plan.Action != "guide" || info.UpgradeAvailable {
			t.Fatalf("unverified Windows channel became automatic: %+v", info)
		}
	}
	if installed != 3 {
		t.Fatalf("expected three distinct installations, got %d", installed)
	}
	for _, call := range mr.Calls {
		if call.Name != "powershell.exe" {
			t.Fatalf("discovery executed an updater: %+v", call)
		}
	}
}

func TestWindowsOfflineIsReadOnlyAndHasNoPlans(t *testing.T) {
	m, mr := windowsFixture(t, []windowsRecord{
		{Name: "OpenCode 2.0.29", Version: "2.0.29", Publisher: "OpenCode", Path: `C:\OpenCode.exe`, Source: "installer"},
	})
	info := productInstallation(t, m.Scan(true), "opencode.desktop")
	if info.Plan != nil || info.LatestCheck != model.CheckOffline || info.LatestVersion != "" {
		t.Fatalf("offline mode exposed a plan or latest version: %+v", info)
	}
	if len(mr.Calls) != 1 {
		t.Fatal("offline discovery queried update services")
	}
}

func TestGuidedActionIsHandoffNotUpgrade(t *testing.T) {
	m, mr := windowsFixture(t, []windowsRecord{
		{Name: "Antigravity 2.18.1", Version: "2.18.1", Publisher: "Google", Path: `C:\Antigravity.exe`, Source: "installer"},
	})
	info := productInstallation(t, m.Scan(false), "antigravity.desktop")
	result := m.Execute(*info.Plan, nil)
	if result.Outcome != "handed_off" || result.URL != "https://antigravity.google/download" ||
		result.NewVersion != "" || len(result.Command) != 0 {
		t.Fatalf("handoff claimed installation success: %+v", result)
	}
	if len(mr.Calls) != 2 {
		t.Fatal("guided action executed an installer")
	}
	plan := *info.Plan
	plan.URL = "https://evil.example/installer"
	plan.ID = digestPlan(plan)
	if result := m.Execute(plan, nil); result.Outcome != "failed" {
		t.Fatalf("tampered plan was accepted: %+v", result)
	}
}

func TestDiscoveryFailureIsNotReportedAsAbsent(t *testing.T) {
	m := isolated(t, New(context.Background(), runner.NewMockRunner(), "Windows", "amd64"))
	snapshot := m.Scan(false)
	if len(snapshot.Issues) == 0 || len(snapshot.Installations) != 4 {
		t.Fatalf("missing discovery issue: %+v", snapshot)
	}
	for _, info := range snapshot.Installations {
		if info.Reason != "discovery_incomplete" || info.LatestCheck != model.CheckFailed || info.Plan != nil {
			t.Fatalf("failed discovery became a reliable absence: %+v", info)
		}
	}
}

type macFixture struct {
	manager *Manager
	mock    *runner.MockRunner
	path    string
	plist   string
}

func newMacFixture(t *testing.T, version, candidateVersion string) macFixture {
	t.Helper()
	mr := runner.NewMockRunner()
	dir := t.TempDir()
	path := filepath.Join(dir, "OpenCode.app")
	if err := os.MkdirAll(filepath.Join(path, "Contents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path, "Contents", "MacOS"), 0755); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 32)
	binary.LittleEndian.PutUint32(header[0:4], macho.Magic64)
	binary.LittleEndian.PutUint32(header[4:8], uint32(macho.CpuArm64))
	binary.LittleEndian.PutUint32(header[12:16], uint32(macho.TypeExec))
	if err := os.WriteFile(filepath.Join(path, "Contents", "MacOS", "OpenCode"), header, 0600); err != nil {
		t.Fatal(err)
	}
	plist := filepath.Join(path, "Contents", "Info.plist")
	mr.SetLookPath("brew", "brew", nil)
	mr.SetResult("ps", []string{"-axo", "command="}, runner.MockResult{Stdout: "/usr/bin/something-else"})
	setMacVersion(t, mr, plist, version)
	casks, _ := json.Marshal(map[string]any{"casks": []any{
		map[string]any{
			"token": "opencode-desktop", "tap": "homebrew/cask",
			"version": candidateVersion, "installed": version,
			"artifacts": []any{map[string]any{"app": []string{"OpenCode.app"}, "target": path}},
		},
	}})
	mr.SetResult("brew", []string{"info", "--json=v2", "--cask", "--installed"}, runner.MockResult{Stdout: string(casks)})
	mr.SetResult("brew", []string{"upgrade", "--cask", "opencode-desktop"}, runner.MockResult{Stdout: "Synthetic upgrade output\n"})
	m := isolated(t, New(context.Background(), mr, "macOS", "arm64"))
	m.appDirs = []string{dir}
	return macFixture{m, mr, path, plist}
}

func setMacVersion(t *testing.T, mr *runner.MockRunner, plist, version string) {
	t.Helper()
	data, _ := json.Marshal(bundleMetadata{ID: "ai.opencode.desktop", Version: version, Executable: "OpenCode"})
	mr.SetResult("plutil", []string{"-convert", "json", "-o", "-", plist}, runner.MockResult{Stdout: string(data)})
}

func TestMacVerifiedCaskPlanAndOfflineDiscovery(t *testing.T) {
	f := newMacFixture(t, "1.18.0", "1.19.0")
	info := productInstallation(t, f.manager.Scan(false), "opencode.desktop")
	if info.Source != "brew-cask" || info.Capability != Automatic || !info.UpgradeAvailable ||
		info.Plan == nil || strings.Join(info.Plan.Command, " ") != "brew upgrade --cask opencode-desktop" {
		t.Fatalf("missing verified Cask plan: %+v", info)
	}
	calls := len(f.mock.Calls)
	offline := productInstallation(t, f.manager.Scan(true), "opencode.desktop")
	if offline.ID != info.ID || offline.Plan != nil || offline.LatestCheck != model.CheckOffline {
		t.Fatalf("local identity changed or offline plan leaked: %+v", offline)
	}
	for _, call := range f.mock.Calls[calls:] {
		if call.Name == "brew" {
			t.Fatalf("offline discovery queried Brew metadata: %+v", call)
		}
	}
}

func TestCaskNeedsOwnershipAndPreservesVersionLine(t *testing.T) {
	for _, tc := range []struct {
		name, version, next, process, metadata string
		want                                   Capability
	}{
		{name: "running", version: "1.18.0", next: "1.19.0", process: "running", want: Blocked},
		{name: "unknown process state", version: "1.18.0", next: "1.19.0", process: "unknown", want: Blocked},
		{name: "major migration", version: "1.18.0", next: "2.0.29", want: Guided},
		{name: "unowned cask", version: "1.18.0", next: "1.19.0", metadata: `{"casks":[]}`, want: Guided},
		{name: "untrusted tap", version: "1.18.0", next: "1.19.0", metadata: `{"casks":[{"token":"opencode-desktop","tap":"other/tap","installed":"1.18.0","version":"1.19.0"}]}`, want: Guided},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMacFixture(t, tc.version, tc.next)
			if tc.process == "running" {
				f.mock.SetResult("ps", []string{"-axo", "command="}, runner.MockResult{Stdout: f.path + "/Contents/MacOS/OpenCode"})
			} else if tc.process == "unknown" {
				f.mock.SetResult("ps", []string{"-axo", "command="}, runner.MockResult{ExitCode: 1})
			}
			if tc.metadata != "" {
				f.mock.SetResult("brew", []string{"info", "--json=v2", "--cask", "--installed"}, runner.MockResult{Stdout: tc.metadata})
			}
			info := productInstallation(t, f.manager.Scan(false), "opencode.desktop")
			if info.Capability != tc.want || (info.Plan != nil && info.Plan.Action == "command") {
				t.Fatalf("unsafe Cask plan: %+v", info)
			}
			if tc.name == "major migration" && info.LatestCheck == model.CheckOK {
				t.Fatal("major migration must not imply up-to-date status")
			}
		})
	}
}

type versionChangingRunner struct {
	*runner.MockRunner
	update func()
}

func (r *versionChangingRunner) Run(name string, args ...string) (string, string, int, error) {
	out, stderr, code, err := r.MockRunner.Run(name, args...)
	if name == "brew" && len(args) > 0 && args[0] == "upgrade" && err == nil && code == 0 {
		r.update()
	}
	return out, stderr, code, err
}

func TestExecutionVerifiesActualVersion(t *testing.T) {
	for _, tc := range []struct{ after, outcome string }{
		{"1.19.0", "updated"}, {"1.18.0", "unchanged"}, {"1.17.0", "failed"},
		{"2.0.29", "failed"}, {"development", "unverified"},
	} {
		t.Run(tc.after, func(t *testing.T) {
			f := newMacFixture(t, "1.18.0", "1.19.0")
			info := productInstallation(t, f.manager.Scan(false), "opencode.desktop")
			f.manager.runner = &versionChangingRunner{f.mock, func() { setMacVersion(t, f.mock, f.plist, tc.after) }}
			log := ""
			result := f.manager.Execute(*info.Plan, func(_, text string) { log += text })
			if result.Outcome != tc.outcome || result.NewVersion != tc.after {
				t.Fatalf("unexpected result: %+v", result)
			}
			if !strings.Contains(log, "Synthetic upgrade") {
				t.Fatal("upgrade log was not forwarded")
			}
		})
	}
}

func TestExecutionRejectsChangesAfterConfirmation(t *testing.T) {
	for _, changed := range []string{"version", "process", "command"} {
		t.Run(changed, func(t *testing.T) {
			f := newMacFixture(t, "1.18.0", "1.19.0")
			plan := *productInstallation(t, f.manager.Scan(false), "opencode.desktop").Plan
			switch changed {
			case "version":
				setMacVersion(t, f.mock, f.plist, "1.19.0")
			case "process":
				f.mock.SetResult("ps", []string{"-axo", "command="}, runner.MockResult{Stdout: f.path + "/Contents/MacOS/OpenCode"})
			case "command":
				plan.Command = []string{"arbitrary", "command"}
				plan.ID = digestPlan(plan)
			}
			if result := f.manager.Execute(plan, nil); result.Outcome != "failed" {
				t.Fatalf("changed plan was executed: %+v", result)
			}
			for _, call := range f.mock.Calls {
				if call.Name == "arbitrary" || (call.Name == "brew" && len(call.Args) > 0 && call.Args[0] == "upgrade") {
					t.Fatalf("mutation ran after state changed: %+v", call)
				}
			}
		})
	}
}

func TestDebDiscoveryRequiresDesktopEvidenceAndStaysGuided(t *testing.T) {
	mr := runner.NewMockRunner()
	mr.SetLookPath("dpkg-query", "dpkg-query", nil)
	mr.SetLookPath("apt-cache", "apt-cache", nil)
	mr.SetLookPath("dpkg", "dpkg", nil)
	mr.SetResultByName("dpkg-query", runner.MockResult{ExitCode: 1})
	mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "chatgpt"}, runner.MockResult{Stdout: "chatgpt\t26.1007.1-1\tamd64\tinstalled"})
	mr.SetResult("dpkg-query", []string{"-L", "chatgpt"}, runner.MockResult{Stdout: "/usr/bin/chatgpt\n/usr/share/applications/chatgpt.desktop\n"})
	mr.SetResult("env", []string{"LC_ALL=C", "apt-cache", "policy", "chatgpt"}, runner.MockResult{Stdout: "chatgpt:\n  Installed: 26.1007.1-1\n  Candidate: 26.1013.2-1\n"})
	mr.SetResult("dpkg", []string{"--compare-versions", "26.1007.1-1", "lt", "26.1013.2-1"}, runner.MockResult{})
	m := isolated(t, New(context.Background(), mr, "Linux", "amd64"))
	m.home = t.TempDir()
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	if info.Capability != Guided || !info.UpgradeAvailable || info.Version != "26.1007.1-1" ||
		info.Plan == nil || info.Plan.Action != "guide" {
		t.Fatalf("Debian channel was incorrectly classified: %+v", info)
	}
	mr.SetResult("env", []string{"LC_ALL=C", "apt-cache", "policy", "chatgpt"}, runner.MockResult{Stdout: "chatgpt:\n  Candidate: 26.1007.1-2\n"})
	mr.SetResult("dpkg", []string{"--compare-versions", "26.1007.1-1", "lt", "26.1007.1-2"}, runner.MockResult{})
	info = productInstallation(t, m.Scan(false), "codex.desktop")
	if !info.UpgradeAvailable || info.LatestVersion != "26.1007.1-2" {
		t.Fatalf("Debian package revision update was lost: %+v", info)
	}
	mr.SetResult("dpkg-query", []string{"-L", "chatgpt"}, runner.MockResult{Stdout: "/usr/bin/chatgpt\n"})
	snapshot := m.Scan(true)
	for _, info := range snapshot.Installations {
		if info.Installed {
			t.Fatal("a CLI-only package became a Desktop installation")
		}
	}
}

func TestAntigravityPortableIdentityRejectsIDEAndUnknownVersions(t *testing.T) {
	for _, tc := range []struct {
		data string
		want bool
	}{
		{`{"name":"antigravity","version":"2.18.1"}`, true},
		{`{"name":"antigravity","version":"1.25.0"}`, false},
		{`{"name":"antigravity-ide","version":"2.25.0"}`, false},
		{`{"name":"antigravity","version":"development"}`, false},
		{`not-json`, false},
	} {
		if _, ok := portableAntigravity([]byte(tc.data)); ok != tc.want {
			t.Fatalf("portable identity %s: got %t", tc.data, ok)
		}
	}
	for _, product := range Products() {
		if strings.Contains(strings.ToLower(product.ID), "ide") || strings.Contains(product.Name, "IDE") {
			t.Fatal("IDE was added to managed products")
		}
	}
}

func TestNumericVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
		ok   bool
	}{
		{"26.1007.1.0", "26.1007.2.0", -1, true},
		{"2.0.29", "2.0.29.0", 0, true},
		{"2.0.30", "2.0.29", 1, true},
		{"2.0.29-beta", "2.0.29", 0, false},
		{"unknown", "2.0.29", 0, false},
		{"9999999999999999999999.1", "2.0", 0, false},
	} {
		got, ok := compareVersion(tc.a, tc.b)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%s vs %s: %d, %t", tc.a, tc.b, got, ok)
		}
	}
}

func TestMacArchitectureMustBeVerifiedBeforeAutomaticUpgrade(t *testing.T) {
	f := newMacFixture(t, "1.18.0", "1.19.0")
	f.manager.arch = "amd64"
	info := productInstallation(t, f.manager.Scan(false), "opencode.desktop")
	if info.Arch != "arm64" || info.Capability != Guided || info.Plan == nil || info.Plan.Action != "guide" {
		t.Fatalf("architecture mismatch became an automatic replacement: %+v", info)
	}
	if macArchitecture(f.path, "../outside") != "unknown" {
		t.Fatal("bundle executable escaped its declared directory")
	}
}

func TestCancelledExecutionDoesNotRunCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mr := runner.NewMockRunner()
	m := New(ctx, mr, "Windows", "amd64")
	result := m.Execute(Plan{}, nil)
	if result.Outcome != "failed" || len(mr.Calls) != 0 {
		t.Fatalf("cancelled execution ran commands: %+v", result)
	}
}
