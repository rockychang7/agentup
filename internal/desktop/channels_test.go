package desktop

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"agentup/internal/model"
	"agentup/internal/runner"
)

// hookRunner changes fixture state when a matching command runs, like a real upgrade would.
type hookRunner struct {
	*runner.MockRunner
	mu     sync.Mutex
	when   func(name string, args []string) bool
	update func()
}

func (r *hookRunner) Run(name string, args ...string) (string, string, int, error) {
	out, stderr, code, err := r.MockRunner.Run(name, args...)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil && code == 0 && r.when(name, args) && r.update != nil {
		r.update()
		r.update = nil
	}
	return out, stderr, code, err
}

func (r *hookRunner) RunInteractive(name string, args []string) (int, error) {
	_, _, code, err := r.Run(name, args...)
	return code, err
}

func exitCode(value uint32) int { return int(int32(value)) }

func setWindowsRecords(t *testing.T, mr *runner.MockRunner, records []windowsRecord) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"items": records, "processes": []string{}, "issues": []string{}})
	mr.SetResult("powershell.exe", []string{"-NoProfile", "-NonInteractive", "-Command", windowsProbe}, runner.MockResult{Stdout: string(data)})
}

func wingetList(mr *runner.MockRunner, id, source string, result runner.MockResult) {
	mr.SetLookPath("winget.exe", "winget.exe", nil)
	mr.SetResult("winget.exe", []string{"list", "--id", id, "--exact", "--source", source, "--disable-interactivity"}, result)
}

func wingetUpgrade(id, source string) []string {
	return []string{"upgrade", "--id", id, "--exact", "--source", source, "--disable-interactivity"}
}

var codexRecord = windowsRecord{
	Name: "OpenAI.Codex", Version: "26.1007.9446.0", Package: "OpenAI.Codex", Family: "OpenAI.Codex_2p2nqsd0c76g0",
	Publisher: "CN=store", Path: `C:\WindowsApps\OpenAI.Codex_26.1007.9446.0_x64`, Scope: "user", Arch: "X64", Source: "msix",
}

func antigravityRecord(version string) windowsRecord {
	return windowsRecord{
		Name: "Antigravity " + version, Version: version, Publisher: "Google", Package: antigravityProductCode,
		Path: `C:\Users\u\AppData\Local\Programs\antigravity\Antigravity.exe`, Scope: "user", Source: "installer",
	}
}

func TestWingetKeepsCodexStandalone(t *testing.T) {
	m, mr := windowsFixture(t, []windowsRecord{codexRecord})
	wingetList(mr, "9PLM9XGG6VKS", "msstore", runner.MockResult{Stdout: "名称    ID           版本\n-----\nChatGPT 9PLM9XGG6VKS 26.1007.9446.0\n"})
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	want := "winget.exe " + strings.Join(wingetUpgrade("9PLM9XGG6VKS", "msstore"), " ")
	if info.Capability != Automatic || info.Plan == nil || strings.Join(info.Plan.Command, " ") != want ||
		!info.Plan.Standalone || info.LatestCheck != model.CheckUnavailable || info.Channel != "msstore" {
		t.Fatalf("Codex Store plan is wrong: %+v %+v", info, info.Plan)
	}
}

func TestWingetRequiresExactProductCodeAndVersion(t *testing.T) {
	m, mr := windowsFixture(t, []windowsRecord{antigravityRecord("2.18.1")})
	wingetList(mr, "Google.Antigravity", "winget", runner.MockResult{Stdout: "Name ID Version Available\n---\nAntigravity 2.18.1 Google.Antigravity 2.18.1 2.23.1\n"})
	info := productInstallation(t, m.Scan(false), "antigravity.desktop")
	if info.Capability != Automatic || !info.UpgradeAvailable || info.LatestVersion != "2.23.1" ||
		info.Plan == nil || info.Plan.Action != ActionCommand || info.Plan.Standalone {
		t.Fatalf("matched Antigravity was not upgradeable: %+v", info)
	}

	wingetList(mr, "Google.Antigravity", "winget", runner.MockResult{Stdout: "Antigravity 2.17.0 Google.Antigravity 2.17.0 2.23.1\n"})
	info = productInstallation(t, m.Scan(false), "antigravity.desktop")
	if info.Reason != "winget_unmatched" || info.Plan.Action != ActionGuide {
		t.Fatalf("a WinGet row for another version was trusted: %+v", info)
	}

	other := antigravityRecord("2.18.1")
	other.Package = "unrelated-key"
	m, mr = windowsFixture(t, []windowsRecord{other})
	mr.SetLookPath("winget.exe", "winget.exe", nil)
	info = productInstallation(t, m.Scan(false), "antigravity.desktop")
	if info.Capability != Guided || info.Plan.Action != ActionGuide {
		t.Fatalf("an unrelated uninstall key was handed to WinGet: %+v", info)
	}
	for _, call := range mr.Calls {
		if call.Name == "winget.exe" {
			t.Fatalf("WinGet was queried without a verified identity: %+v", call)
		}
	}

	wingetList(mr, "Google.Antigravity", "winget", runner.MockResult{ExitCode: exitCode(0x8A150046)})
	m, _ = windowsFixture(t, []windowsRecord{antigravityRecord("2.18.1")})
	m.runner = mr
	setWindowsRecords(t, mr, []windowsRecord{antigravityRecord("2.18.1")})
	info = productInstallation(t, m.Scan(false), "antigravity.desktop")
	if info.Reason != "winget_agreement_required" || info.LatestCheck != model.CheckFailed {
		t.Fatalf("source agreement prompt was not reported: %+v", info)
	}
}

func TestOpenCodeUsesWingetOnlyWithinVersionOne(t *testing.T) {
	v1 := windowsRecord{Name: "OpenCode 1.18.0", Version: "1.18.0", Publisher: "OpenCode", Package: "oc", Path: `C:\OpenCode\OpenCode.exe`, Scope: "user", Source: "installer"}
	m, mr := windowsFixture(t, []windowsRecord{v1})
	wingetList(mr, "SST.OpenCodeDesktop", "winget", runner.MockResult{Stdout: "OpenCode 1.18.0 SST.OpenCodeDesktop 1.18.0 1.19.0\n"})
	info := productInstallation(t, m.Scan(false), "opencode.desktop")
	if info.Capability != Automatic || info.LatestVersion != "1.19.0" || info.Plan == nil || info.Plan.Action != ActionCommand {
		t.Fatalf("OpenCode 1.x WinGet plan missing: %+v", info)
	}
	wingetList(mr, "SST.OpenCodeDesktop", "winget", runner.MockResult{Stdout: "OpenCode 1.18.0 SST.OpenCodeDesktop 1.18.0 2.0.29\n"})
	if info = productInstallation(t, m.Scan(false), "opencode.desktop"); info.Reason != "release_line_change" || info.Plan.Action != ActionGuide {
		t.Fatalf("WinGet crossed the OpenCode release line: %+v", info)
	}

	v2 := v1
	v2.Name, v2.Version = "OpenCode 2.0.29", "2.0.29"
	m, mr = windowsFixture(t, []windowsRecord{v2})
	wingetList(mr, "SST.OpenCodeDesktop", "winget", runner.MockResult{Stdout: "OpenCode 2.0.29 SST.OpenCodeDesktop 2.0.29\n"})
	m.Scan(false)
	for _, call := range mr.Calls {
		if call.Name == "winget.exe" {
			t.Fatalf("OpenCode 2.x was matched to the 1.x WinGet entry: %+v", call)
		}
	}
}

func TestWingetExitCodesAreMappedAndVersionIsVerified(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		outcome string
	}{
		{"updated", 0, "updated"},
		{"no update", exitCode(0x8A15002B), "no_update"},
		{"in use", exitCode(0x8A150101), "failed"},
		{"cancelled", exitCode(0x8A15010C), "cancelled"},
		{"reboot", exitCode(0x8A150109), "unverified"},
		{"unknown", 7, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mr := windowsFixture(t, []windowsRecord{antigravityRecord("2.18.1")})
			wingetList(mr, "Google.Antigravity", "winget", runner.MockResult{Stdout: "Antigravity 2.18.1 Google.Antigravity 2.18.1 2.23.1\n"})
			mr.SetResult("winget.exe", wingetUpgrade("Google.Antigravity", "winget"), runner.MockResult{ExitCode: tc.code, Stdout: "progress\n"})
			info := productInstallation(t, m.Scan(false), "antigravity.desktop")
			m.runner = &hookRunner{MockRunner: mr,
				when:   func(name string, args []string) bool { return name == "winget.exe" && args[0] == "upgrade" },
				update: func() { setWindowsRecords(t, mr, []windowsRecord{antigravityRecord("2.23.1")}) }}
			phases := []string{}
			result := m.Execute(*info.Plan, func(stream, text string) {
				if stream == "phase" {
					phases = append(phases, text)
				}
			})
			if result.Outcome != tc.outcome || (tc.outcome == "updated" && result.NewVersion != "2.23.1") {
				t.Fatalf("unexpected result: %+v", result)
			}
			if tc.code != 0 && tc.code != 7 && strings.Contains(result.Message, "exited") {
				t.Fatalf("known WinGet code lacked a readable message: %+v", result)
			}
			if phases[0] != "checking" || phases[1] != "upgrading" {
				t.Fatalf("phases were not reported: %v", phases)
			}
		})
	}
}

func TestParseWingetListIgnoresLocalizedHeaders(t *testing.T) {
	for _, tc := range []struct{ out, installed, available string }{
		{"名称 ID 版本 可用\n-----\nAntigravity 2.18.1 Google.Antigravity 2.18.1 2.23.1\r\n", "2.18.1", "2.23.1"},
		{"Name Id Version Source\nAntigravity Google.Antigravity 2.18.1 winget\n", "2.18.1", ""},
		{"\r   \\ \r ChatGPT 9PLM9XGG6VKS 26.1007.9446.0\n", "26.1007.9446.0", ""},
	} {
		installed, available, found := parseWingetList(tc.out, strings.Fields(tc.out)[0])
		if strings.Contains(tc.out, "Google.Antigravity") {
			installed, available, found = parseWingetList(tc.out, "Google.Antigravity")
		} else {
			installed, available, found = parseWingetList(tc.out, "9PLM9XGG6VKS")
		}
		if !found || installed != tc.installed || available != tc.available {
			t.Fatalf("%q: %q %q %t", tc.out, installed, available, found)
		}
	}
	if _, _, found := parseWingetList("Google.Antigravity.Beta 1.0", "Google.Antigravity"); found {
		t.Fatal("a different ID matched")
	}
}

func writePE(t *testing.T, path string, machine uint16) {
	t.Helper()
	data := make([]byte, 1024)
	copy(data, "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:], 64)
	copy(data[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(data[68:], machine)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

const openCodeManifestSample = `files:
  - url: https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-arm64.exe
    sha512: arm==
    size: 226188816
  - url: https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-x64.exe
    sha512: x64==
    size: 236673488
releaseDate: 2026-10-14T08:51:35.740Z
version: 2.0.29
`

func TestElectronManifestParsing(t *testing.T) {
	manifest, err := parseElectronManifest([]byte(openCodeManifestSample))
	if err != nil || manifest.Version != "2.0.29" || len(manifest.Files) != 2 ||
		manifest.Files[1].URL != "https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-x64.exe" ||
		manifest.Files[1].SHA512 != "x64==" || manifest.Files[1].Size != 236673488 {
		t.Fatalf("manifest parsed incorrectly: %+v %v", manifest, err)
	}
	if _, err := parseElectronManifest([]byte("version: latest\n")); err == nil {
		t.Fatal("non-numeric manifest version was accepted")
	}
}

type installerFixture struct {
	m         *Manager
	mock      *runner.MockRunner
	installer string
	record    windowsRecord
}

func newInstallerFixture(t *testing.T, signature string) installerFixture {
	t.Helper()
	dir := t.TempDir()
	writePE(t, filepath.Join(dir, "OpenCode.exe"), 0x8664)
	record := windowsRecord{Name: "OpenCode 2.0.28", Version: "2.0.28", Publisher: "OpenCode", Package: "oc", Path: dir, Scope: "user", Source: "installer"}
	m, mr := windowsFixture(t, []windowsRecord{record})
	m.fetch = func(_ context.Context, url string, _ int64) ([]byte, error) {
		if url != openCodeManifest+"latest.yml" {
			t.Fatalf("unexpected metadata URL %s", url)
		}
		return []byte(openCodeManifestSample), nil
	}
	installer := filepath.Join(t.TempDir(), "opencode-desktop-win-x64.exe")
	m.download = func(_ context.Context, asset Asset, _ string) (string, error) {
		if asset.SHA512 != "x64==" || asset.Publisher != openCodePublisher {
			t.Fatalf("wrong asset selected: %+v", asset)
		}
		return installer, os.WriteFile(installer, []byte("installer"), 0o600)
	}
	mr.SetResultByName("powershell.exe", runner.MockResult{Stdout: signature})
	mr.SetResultByName(installer, runner.MockResult{})
	return installerFixture{m, mr, installer, record}
}

const validSignature = `{"status":"Valid","subject":"CN=\"Anomaly Innovations, Inc https://anoma.ly/\", O=\"Anomaly Innovations, Inc https://anoma.ly/\", C=US"}`

func TestOpenCodeInstallerIsVerifiedBeforeItOpens(t *testing.T) {
	f := newInstallerFixture(t, validSignature)
	info := productInstallation(t, f.m.Scan(false), "opencode.desktop")
	if info.Capability != Interactive || info.Arch != "amd64" || info.Plan == nil || info.Plan.Action != ActionInstaller ||
		info.Plan.Asset.URL != "https://opencode.ai/files/bin/2.0.29/opencode-desktop-win-x64.exe" || info.LatestSource != LatestVendor {
		t.Fatalf("installer plan missing: %+v %+v", info, info.Plan)
	}
	updated := f.record
	updated.Version = "2.0.29"
	f.m.runner = &hookRunner{MockRunner: f.mock,
		when:   func(name string, _ []string) bool { return name == f.installer },
		update: func() { setWindowsRecords(t, f.mock, []windowsRecord{updated}) }}
	phases := []string{}
	result := f.m.Execute(*info.Plan, func(stream, text string) {
		if stream == "phase" {
			phases = append(phases, text)
		}
	})
	if result.Outcome != "updated" || result.NewVersion != "2.0.29" {
		t.Fatalf("installer flow did not verify the new version: %+v", result)
	}
	if strings.Join(phases, ",") != "checking,downloading,verifying_package,installing,verifying,stopping" {
		t.Fatalf("unexpected phases: %v", phases)
	}
	if entries, _ := os.ReadDir(f.m.receiptDir); len(entries) != 0 {
		t.Fatal("receipt was left after a verified install")
	}
}

func TestInstallerSignatureAndPublisherAreEnforced(t *testing.T) {
	for _, signature := range []string{
		`{"status":"NotSigned","subject":""}`,
		`{"status":"Valid","subject":"CN=Someone Else, O=Someone Else"}`,
		`not json`,
	} {
		f := newInstallerFixture(t, signature)
		info := productInstallation(t, f.m.Scan(false), "opencode.desktop")
		result := f.m.Execute(*info.Plan, nil)
		if result.Outcome != "failed" || !strings.Contains(result.Message, "signature") {
			t.Fatalf("%s: unsigned installer accepted: %+v", signature, result)
		}
		for _, call := range f.mock.Calls {
			if call.Name == f.installer {
				t.Fatalf("installer opened despite %s", signature)
			}
		}
	}
}

func TestUnfinishedInstallIsNotReopened(t *testing.T) {
	f := newInstallerFixture(t, validSignature)
	info := productInstallation(t, f.m.Scan(false), "opencode.desktop")
	result := f.m.Execute(*info.Plan, nil)
	if result.Outcome != "unchanged" {
		t.Fatalf("an installer that changed nothing was reported as %+v", result)
	}
	plan := *info.Plan
	if err := f.m.writeReceipt(plan); err != nil {
		t.Fatal(err)
	}
	info = productInstallation(t, f.m.Scan(false), "opencode.desktop")
	if info.Capability != Blocked || info.Reason != "install_pending_verification" || info.Plan != nil {
		t.Fatalf("pending install could start again: %+v", info)
	}
	updated := f.record
	updated.Version = "2.0.29"
	setWindowsRecords(t, f.mock, []windowsRecord{updated})
	if info = productInstallation(t, f.m.Scan(false), "opencode.desktop"); info.Reason == "install_pending_verification" {
		t.Fatal("receipt survived a completed install")
	}
}

func TestDownloadAcceptsOnlyTheManifestFile(t *testing.T) {
	body := []byte("signed installer bytes")
	sum := sha512.Sum512(body)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	host, _ := url.Parse(server.URL)
	trustedHosts[host.Hostname()] = true
	transport = server.Client().Transport
	defer func() { delete(trustedHosts, host.Hostname()); transport = nil }()
	asset := Asset{URL: server.URL + "/opencode-desktop-win-x64.exe", SHA512: base64.StdEncoding.EncodeToString(sum[:]), Size: int64(len(body))}
	path, err := downloadAsset(context.Background(), asset, t.TempDir())
	if err != nil || filepath.Base(path) != "opencode-desktop-win-x64.exe" {
		t.Fatalf("valid download rejected: %v", err)
	}
	asset.SHA512 = base64.StdEncoding.EncodeToString(make([]byte, 64))
	if _, err := downloadAsset(context.Background(), asset, t.TempDir()); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, err := downloadAsset(context.Background(), Asset{URL: "https://evil.example/x.exe", SHA512: "x", Size: 1}, t.TempDir()); err == nil {
		t.Fatal("untrusted host accepted")
	}
}

func linuxFixture(t *testing.T) (*Manager, *runner.MockRunner) {
	t.Helper()
	mr := runner.NewMockRunner()
	m := isolated(t, New(context.Background(), mr, "Linux", "amd64"))
	m.home = t.TempDir()
	return m, mr
}

func debChatGPT(mr *runner.MockRunner, version, candidate string) {
	mr.SetLookPath("dpkg-query", "dpkg-query", nil)
	mr.SetLookPath("apt-cache", "apt-cache", nil)
	mr.SetLookPath("dpkg", "dpkg", nil)
	mr.SetResultByName("dpkg-query", runner.MockResult{ExitCode: 1})
	mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "chatgpt"}, runner.MockResult{Stdout: "chatgpt\t" + version + "\tamd64\tinstalled"})
	mr.SetResult("dpkg-query", []string{"-L", "chatgpt"}, runner.MockResult{Stdout: "/opt/ChatGPT/chatgpt\n/usr/share/applications/chatgpt.desktop\n"})
	mr.SetResult("env", []string{"LC_ALL=C", "apt-cache", "policy", "chatgpt"}, runner.MockResult{Stdout: "chatgpt:\n  Candidate: " + candidate + "\n"})
	mr.SetResult("dpkg", []string{"--compare-versions", version, "lt", candidate}, runner.MockResult{})
}

func TestAptUpdateRunsOnlyFromTerminal(t *testing.T) {
	m, mr := linuxFixture(t)
	debChatGPT(mr, "26.1007.1-1", "26.1013.2-1")
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	if info.Capability != Privileged || info.Plan == nil || info.Plan.Action != ActionPrivileged || !info.Plan.RequiresAdmin ||
		strings.Join(info.Plan.Command, " ") != "sudo apt-get install --only-upgrade chatgpt" || info.LatestSource != LatestAPT {
		t.Fatalf("APT plan is wrong: %+v %+v", info, info.Plan)
	}
	if result := m.Execute(*info.Plan, nil); result.Outcome != "failed" {
		t.Fatalf("privileged plan ran without a terminal: %+v", result)
	}
	for _, call := range mr.Calls {
		if call.Name == "sudo" {
			t.Fatal("sudo ran outside a terminal")
		}
	}
	mr.SetResult("sudo", []string{"apt-get", "install", "--only-upgrade", "chatgpt"}, runner.MockResult{})
	mr.SetResult("dpkg", []string{"--compare-versions", "26.1007.1-1", "lt", "26.1013.2-1"}, runner.MockResult{})
	m.runner = &hookRunner{MockRunner: mr,
		when: func(name string, _ []string) bool { return name == "sudo" },
		update: func() {
			mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "chatgpt"}, runner.MockResult{Stdout: "chatgpt\t26.1013.2-1\tamd64\tinstalled"})
		}}
	m.EnableTerminal()
	if result := m.Execute(*info.Plan, nil); result.Outcome != "updated" || result.NewVersion != "26.1013.2-1" {
		t.Fatalf("terminal APT update was not verified: %+v", result)
	}
}

func TestLinuxRunningAppBlocksPackageUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process links use Linux paths")
	}
	m, mr := linuxFixture(t)
	debChatGPT(mr, "26.1007.1-1", "26.1013.2-1")
	if err := os.MkdirAll(filepath.Join(m.procRoot, "4242"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/opt/ChatGPT/chatgpt", filepath.Join(m.procRoot, "4242", "exe")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	if info.Running != "running" || info.Capability != Blocked || info.Plan != nil {
		t.Fatalf("running app was not protected: %+v", info)
	}
	m.procRoot = filepath.Join(m.procRoot, "missing")
	if info = productInstallation(t, m.Scan(false), "codex.desktop"); info.Running != "unknown" || info.Capability != Blocked {
		t.Fatalf("unreadable process table was treated as stopped: %+v", info)
	}
}

func TestDnfCandidateComesFromCacheOnly(t *testing.T) {
	m, mr := linuxFixture(t)
	mr.SetLookPath("rpm", "rpm", nil)
	mr.SetLookPath("dnf", "dnf", nil)
	mr.SetResultByName("rpm", runner.MockResult{ExitCode: 1})
	mr.SetResult("rpm", []string{"-q", "--qf", rpmQueryFormat, "chatgpt"}, runner.MockResult{Stdout: "chatgpt\t26.1007.1-1\tx86_64"})
	mr.SetResult("rpm", []string{"-ql", "chatgpt"}, runner.MockResult{Stdout: "/opt/ChatGPT/chatgpt\n/usr/share/applications/chatgpt.desktop\n"})
	mr.SetResult("dnf", []string{"repoquery", "--cacheonly", "--quiet", "--latest-limit=1", "--queryformat", "%|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\n", "chatgpt"}, runner.MockResult{Stdout: "26.1013.2-1\n"})
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	if info.Source != "rpm" || info.Capability != Privileged || info.LatestVersion != "26.1013.2-1" ||
		strings.Join(info.Plan.Command, " ") != "sudo dnf upgrade chatgpt" || info.LatestSource != LatestDNF {
		t.Fatalf("DNF plan is wrong: %+v", info)
	}
}

func TestRPMVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.0-1", "1.0-2", -1}, {"1.0.10-1", "1.0.9-1", 1}, {"1.0~rc1-1", "1.0-1", -1},
		{"1.0^git1-1", "1.0-1", 1}, {"1:1.0-1", "2.0-1", 1}, {"1.0a-1", "1.0-1", 1},
		{"1.0-1", "1.0", 0}, {"2.0.29-1", "2.0.29-1", 0}, {"1.01", "1.1", 0},
	} {
		if got := rpmCompare(tc.a, tc.b); got != tc.want {
			t.Fatalf("%s vs %s: got %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLinuxOpenCodeUsesVendorManifest(t *testing.T) {
	m, mr := linuxFixture(t)
	mr.SetLookPath("dpkg-query", "dpkg-query", nil)
	mr.SetResultByName("dpkg-query", runner.MockResult{ExitCode: 1})
	mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "opencode-desktop"}, runner.MockResult{Stdout: "opencode-desktop\t2.0.28\tarm64\tinstalled"})
	mr.SetResult("dpkg-query", []string{"-L", "opencode-desktop"}, runner.MockResult{Stdout: "/usr/share/applications/opencode-desktop.desktop\n"})
	m.fetch = func(_ context.Context, url string, _ int64) ([]byte, error) {
		if url != openCodeManifest+"latest-linux-arm64.yml" {
			t.Fatalf("wrong manifest %s", url)
		}
		return []byte("files:\nversion: 2.0.29\n"), nil
	}
	info := productInstallation(t, m.Scan(false), "opencode.desktop")
	if !info.UpgradeAvailable || info.LatestSource != LatestVendor || info.Capability != Guided || info.Plan.Action != ActionGuide {
		t.Fatalf("Linux OpenCode candidate missing: %+v", info)
	}
	for _, call := range mr.Calls {
		if call.Name == "env" {
			t.Fatal("OpenCode was checked against the APT cache")
		}
	}
}

func TestAntigravityFoundThroughLauncherEntry(t *testing.T) {
	m, _ := linuxFixture(t)
	root := filepath.Join(t.TempDir(), "Antigravity")
	if err := os.MkdirAll(filepath.Join(root, "resources", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "resources", "app", "package.json"), []byte(`{"name":"antigravity","version":"2.18.1"}`), 0o600)
	entries := t.TempDir()
	os.WriteFile(filepath.Join(entries, "antigravity.desktop"), []byte("[Desktop Entry]\nName=Antigravity\nExec="+filepath.Join(root, "antigravity")+" %U\n"), 0o600)
	os.WriteFile(filepath.Join(entries, "ide.desktop"), []byte("[Desktop Entry]\nExec=/opt/ide/antigravity-ide\n"), 0o600)
	m.desktopDir = []string{entries}
	info := productInstallation(t, m.Scan(true), "antigravity.desktop")
	if info.Path != root || info.Version != "2.18.1" || info.Source != "portable" {
		t.Fatalf("launcher-based install not found: %+v", info)
	}
}

func TestMacDirectInstallShowsReferenceVersion(t *testing.T) {
	f := newMacFixture(t, "1.18.0", "1.19.0")
	f.mock.SetResult("brew", []string{"info", "--json=v2", "--cask", "--installed"}, runner.MockResult{Stdout: `{"casks":[]}`})
	f.manager.fetch = func(_ context.Context, url string, _ int64) ([]byte, error) {
		if url != "https://formulae.brew.sh/api/cask/opencode-desktop.json" {
			t.Fatalf("unexpected reference URL %s", url)
		}
		return []byte(`{"version":"1.19.0,abc"}`), nil
	}
	info := productInstallation(t, f.manager.Scan(false), "opencode.desktop")
	if info.Source != "app-bundle" || info.LatestSource != LatestReference || info.LatestVersion != "1.19.0" ||
		!info.UpgradeAvailable || info.Capability != Guided || info.Plan.Action != ActionGuide {
		t.Fatalf("direct install reference is wrong: %+v", info)
	}
	calls := len(f.mock.Calls)
	f.manager.fetch = func(context.Context, string, int64) ([]byte, error) {
		t.Fatal("offline scan fetched a reference version")
		return nil, nil
	}
	f.manager.Scan(true)
	_ = calls
}
