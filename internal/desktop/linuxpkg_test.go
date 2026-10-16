package desktop

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentup/internal/model"
	"agentup/internal/runner"
)

func TestClaudeWindowsInstallsUseWinget(t *testing.T) {
	dir := t.TempDir()
	writePE(t, filepath.Join(dir, "claude.exe"), 0x8664)
	for _, record := range []windowsRecord{
		{Name: "Claude", Version: "2.19000.1.0", Package: "Claude", Family: claudeFamily, Publisher: "CN=Anthropic",
			Path: `C:\WindowsApps\Claude_2.19000.1.0_x64`, Scope: "user", Arch: "X64", Source: "msix"},
		{Name: "Claude", Version: "2.19000.1", Package: "AnthropicClaude", Publisher: "Anthropic PBC",
			Path: dir, Scope: "user", Source: "installer"},
	} {
		m, mr := windowsFixture(t, []windowsRecord{record})
		wingetList(mr, "Anthropic.Claude", "winget", runner.MockResult{Stdout: "Name ID Version Available\n---\nClaude Anthropic.Claude 2.19000.1 2.19675.1\n"})
		info := productInstallation(t, m.Scan(false), "claude.desktop")
		want := "winget.exe " + strings.Join(wingetUpgrade("Anthropic.Claude", "winget"), " ")
		if info.Capability != Automatic || info.Plan == nil || strings.Join(info.Plan.Command, " ") != want ||
			info.Plan.Standalone || !info.UpgradeAvailable || info.LatestVersion != "2.19675.1" {
			t.Fatalf("Claude %s install is not automatic: %+v %+v", record.Source, info, info.Plan)
		}
	}
	// A machine-wide or differently keyed installer is not the one WinGet's manifest describes.
	m, mr := windowsFixture(t, []windowsRecord{{Name: "Claude", Version: "2.19000.1", Package: "{other}", Publisher: "Anthropic PBC", Path: dir, Scope: "system", Source: "installer"}})
	wingetList(mr, "Anthropic.Claude", "winget", runner.MockResult{Stdout: "Claude Anthropic.Claude 2.19000.1 2.19675.1\n"})
	if info := productInstallation(t, m.Scan(false), "claude.desktop"); info.Capability != Guided {
		t.Fatalf("unmatched Claude installer was handed to WinGet: %+v", info)
	}
}

func TestFlatpakOpenCodeUpdatesFromItsRemote(t *testing.T) {
	m, mr := linuxFixture(t)
	mr.SetLookPath("flatpak", "flatpak", nil)
	mr.SetResult("flatpak", []string{"list", "--app", "--columns=application,version,installation,arch,origin"}, runner.MockResult{
		Stdout: "ai.opencode.opencode\t1.18.8\tuser\tx86_64\tflathub\ncom.example.ClaudeWrapper\t1.0.0\tuser\tx86_64\tflathub\n"})
	mr.SetResult("flatpak", []string{"ps", "--columns=application"}, runner.MockResult{})
	mr.SetResult("flatpak", []string{"remote-ls", "--user", "--app", "--updates", "--columns=application,version", "flathub"},
		runner.MockResult{Stdout: "ai.opencode.opencode\t1.19.2\n"})
	snapshot := m.Scan(false)
	info := productInstallation(t, snapshot, "opencode.desktop")
	want := "flatpak update --user -y --noninteractive ai.opencode.opencode"
	if info.Source != "flatpak" || info.Capability != Automatic || info.Plan == nil || strings.Join(info.Plan.Command, " ") != want ||
		info.LatestVersion != "1.19.2" || info.LatestSource != LatestFlatpak || info.Running != "stopped" ||
		info.Path != filepath.Join(m.home, ".local", "share", "flatpak", "app", "ai.opencode.opencode") {
		t.Fatalf("Flatpak plan is wrong: %+v %+v", info, info.Plan)
	}
	for _, other := range snapshot.Installations {
		if other.Installed && other.ProductID == "claude.desktop" {
			t.Fatalf("a community Flatpak was taken over: %+v", other)
		}
	}
	mr.SetResult("flatpak", []string{"ps", "--columns=application"}, runner.MockResult{Stdout: "ai.opencode.opencode\n"})
	if info := productInstallation(t, m.Scan(false), "opencode.desktop"); info.Running != "running" || info.Plan != nil {
		t.Fatalf("a running Flatpak was not blocked: %+v", info)
	}
}

func TestSnapRequiresTheVerifiedPublisher(t *testing.T) {
	m, mr := linuxFixture(t)
	mr.SetLookPath("snap", "snap", nil)
	mr.SetResult("snap", []string{"list", "antigravity"}, runner.MockResult{
		Stdout: "Name         Version  Rev  Tracking       Publisher  Notes\nantigravity  2.23.1   41   latest/stable  ubuntu✓    classic\n"})
	mr.SetResult("snap", []string{"refresh", "--list"}, runner.MockResult{
		Stdout: "Name         Version  Rev  Size   Publisher  Notes\nantigravity  2.24.0   42   210MB  ubuntu✓    classic\n"})
	info := productInstallation(t, m.Scan(false), "antigravity.desktop")
	if info.Source != "snap" || info.Capability != Automatic || info.Plan == nil || strings.Join(info.Plan.Command, " ") != "snap refresh antigravity" ||
		info.LatestVersion != "2.24.0" || info.Path != "/snap/antigravity" || info.Channel != "latest/stable" {
		t.Fatalf("snap plan is wrong: %+v %+v", info, info.Plan)
	}
	for _, publisher := range []string{"someone✓", "ubuntu"} {
		mr.SetResult("snap", []string{"list", "antigravity"}, runner.MockResult{
			Stdout: "Name Version Rev Tracking Publisher Notes\nantigravity 2.23.1 41 latest/stable " + publisher + " classic\n"})
		for _, info := range m.Scan(true).Installations {
			if info.Installed && info.Source == "snap" {
				t.Fatalf("snap from %q was accepted: %+v", publisher, info)
			}
		}
	}
}

// appImageRunner stands in for the AppImage runtime: --appimage-extract writes the
// desktop entry, with the version taken from the image's content.
type appImageRunner struct{ *runner.MockRunner }

func (r appImageRunner) Run(name string, args ...string) (string, string, int, error) {
	if name != "sh" {
		return r.MockRunner.Run(name, args...)
	}
	data, err := os.ReadFile(args[4])
	if err != nil {
		return "", err.Error(), 1, nil
	}
	version := "2.0.28"
	if strings.HasSuffix(string(data), "new") {
		version = "2.0.29"
	}
	root := filepath.Join(args[3], "squashfs-root")
	os.MkdirAll(root, 0o755)
	entry := "[Desktop Entry]\nName=OpenCode\nX-AppImage-Version=" + version + "\n[Desktop Action New]\nName=Other\n"
	return "", "", 0, os.WriteFile(filepath.Join(root, "opencode-desktop.desktop"), []byte(entry), 0o644)
}

// writeELF writes a minimal x86-64 ELF header followed by a marker.
func writeELF(t *testing.T, path, marker string) {
	t.Helper()
	header := make([]byte, 64)
	copy(header, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(header[16:], 2)
	binary.LittleEndian.PutUint16(header[18:], 62)
	binary.LittleEndian.PutUint32(header[20:], 1)
	binary.LittleEndian.PutUint16(header[52:], 64)
	if err := os.WriteFile(path, append(header, marker...), 0o755); err != nil {
		t.Fatal(err)
	}
}

const openCodeLinuxManifest = `files:
  - url: https://opencode.ai/files/bin/2.0.29/opencode-desktop-linux-x86_64.AppImage
    sha512: appimage==
    size: 159218186
  - url: https://opencode.ai/files/bin/2.0.29/opencode-desktop-linux-amd64.deb
    sha512: deb==
    size: 123057780
version: 2.0.29
`

func TestOpenCodeAppImageIsReplacedInPlace(t *testing.T) {
	m, mr := linuxFixture(t)
	mr.SetLookPath("sh", "sh", nil)
	m.runner = appImageRunner{mr}
	dir := filepath.Join(m.home, "Applications")
	os.MkdirAll(dir, 0o755)
	image := filepath.Join(dir, "OpenCode-Desktop.AppImage")
	writeELF(t, image, "old")
	writeELF(t, filepath.Join(dir, "other-app.AppImage"), "old")
	m.fetch = func(_ context.Context, url string, _ int64) ([]byte, error) {
		if url != openCodeManifest+"latest-linux.yml" {
			t.Fatalf("wrong manifest %s", url)
		}
		return []byte(openCodeLinuxManifest), nil
	}
	downloaded := filepath.Join(t.TempDir(), "opencode-desktop-linux-x86_64.AppImage")
	m.download = func(_ context.Context, asset Asset, _ string) (string, error) {
		if asset.SHA512 != "appimage==" {
			t.Fatalf("wrong asset selected: %+v", asset)
		}
		writeELF(t, downloaded, "new")
		return downloaded, nil
	}
	info := productInstallation(t, m.Scan(false), "opencode.desktop")
	if info.Source != "appimage" || info.Capability != Automatic || info.Scope != "user" || info.Arch != "amd64" ||
		info.Plan == nil || info.Plan.Action != ActionReplace || info.Plan.Asset == nil || info.Version != "2.0.28" {
		t.Fatalf("AppImage plan is wrong: %+v %+v", info, info.Plan)
	}
	result := m.Execute(*info.Plan, nil)
	if result.Outcome != "updated" || result.NewVersion != "2.0.29" {
		t.Fatalf("AppImage was not replaced: %+v", result)
	}
	if data, _ := os.ReadFile(image); !strings.HasSuffix(string(data), "new") {
		t.Fatal("the original path does not hold the new AppImage")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".*agentup*")); len(leftovers) != 0 {
		t.Fatalf("temporary files were left behind: %v", leftovers)
	}
	if after := productInstallation(t, m.Scan(true), "opencode.desktop"); after.ID != info.ID {
		t.Fatal("the replaced AppImage was reported as a different installation")
	}
}

func TestAppImageOutsideHomeIsNotReplaced(t *testing.T) {
	m := &Manager{os: "linux"}
	info := Installation{ProductID: "opencode.desktop", Version: "2.0.28", Source: "appimage", Scope: "system", Arch: "amd64", Installed: true}
	m.fetch = func(context.Context, string, int64) ([]byte, error) { return []byte(openCodeLinuxManifest), nil }
	m.ctx = context.Background()
	m.vendorCheck(&info, "latest-linux.yml")
	if info.Capability != Guided || info.Reason != "appimage_system_location" || info.LatestCheck != model.CheckOK {
		t.Fatalf("system AppImage would be replaced: %+v", info)
	}
}

func TestAptUpgradeAsksForThePasswordThroughPkexec(t *testing.T) {
	m, mr := linuxFixture(t)
	debChatGPT(mr, "26.1007.1-1", "26.1013.2-1")
	mr.SetLookPath("pkexec", "/usr/bin/pkexec", nil)
	mr.SetLookPath("env", "/usr/bin/env", nil)
	mr.SetLookPath("apt-get", "/usr/bin/apt-get", nil)
	info := productInstallation(t, m.Scan(false), "codex.desktop")
	want := "/usr/bin/pkexec /usr/bin/env DEBIAN_FRONTEND=noninteractive /usr/bin/apt-get install --only-upgrade -y chatgpt"
	if info.Capability != Automatic || info.Plan == nil || info.Plan.Action != ActionCommand || !info.Plan.RequiresAdmin ||
		strings.Join(info.Plan.Command, " ") != want {
		t.Fatalf("APT plan does not use pkexec: %+v %+v", info, info.Plan)
	}
	args := strings.Fields(want)[1:]
	for _, tc := range []struct {
		code    int
		outcome string
		message string
	}{{126, "cancelled", "cancelled or refused"}, {127, "failed", "sudo /usr/bin/env"}} {
		mr.SetResult("/usr/bin/pkexec", args, runner.MockResult{ExitCode: tc.code})
		if result := m.Execute(*info.Plan, nil); result.Outcome != tc.outcome || !strings.Contains(result.Message, tc.message) {
			t.Fatalf("pkexec exit %d was reported as %+v", tc.code, result)
		}
	}
	mr.SetResult("/usr/bin/pkexec", args, runner.MockResult{})
	m.runner = &hookRunner{MockRunner: mr,
		when: func(name string, _ []string) bool { return name == "/usr/bin/pkexec" },
		update: func() {
			mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "chatgpt"}, runner.MockResult{Stdout: "chatgpt\t26.1013.2-1\tamd64\tinstalled"})
		}}
	phases := []string{}
	result := m.Execute(*info.Plan, func(stream, text string) {
		if stream == "phase" {
			phases = append(phases, text)
		}
	})
	if result.Outcome != "updated" || result.NewVersion != "26.1013.2-1" || !strings.Contains(strings.Join(phases, ","), "needs_elevation") {
		t.Fatalf("pkexec upgrade was not verified: %+v %v", result, phases)
	}
	// The terminal CLI keeps sudo in the same terminal.
	m.runner = mr
	debChatGPT(mr, "26.1007.1-1", "26.1013.2-1")
	m.EnableTerminal()
	if info := productInstallation(t, m.Scan(false), "codex.desktop"); info.Capability != Privileged || info.Plan.Command[0] != "sudo" {
		t.Fatalf("terminal plan should keep sudo: %+v", info.Plan)
	}
}

func TestOpenCodePackagesInstallTheVerifiedDownload(t *testing.T) {
	manifest := openCodeLinuxManifest + `  - url: https://opencode.ai/files/bin/2.0.29/opencode-desktop-linux-x86_64.rpm
    sha512: rpm==
    size: 106509553
`
	t.Run("deb", func(t *testing.T) {
		m, mr := linuxFixture(t)
		mr.SetLookPath("dpkg-query", "dpkg-query", nil)
		mr.SetLookPath("dpkg", "dpkg", nil)
		mr.SetLookPath("pkexec", "pkexec", nil)
		mr.SetResultByName("dpkg-query", runner.MockResult{ExitCode: 1})
		mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "opencode-desktop"}, runner.MockResult{Stdout: "opencode-desktop\t2.0.28\tamd64\tinstalled"})
		mr.SetResult("dpkg-query", []string{"-L", "opencode-desktop"}, runner.MockResult{Stdout: "/opt/OpenCode/opencode-desktop\n/usr/share/applications/opencode-desktop.desktop\n"})
		mr.SetResult("dpkg", []string{"--compare-versions", "2.0.28", "lt", "2.0.29"}, runner.MockResult{})
		m.fetch = func(context.Context, string, int64) ([]byte, error) { return []byte(manifest), nil }
		downloaded := filepath.Join(t.TempDir(), "opencode-desktop-linux-amd64.deb")
		m.download = func(_ context.Context, asset Asset, _ string) (string, error) {
			if asset.SHA512 != "deb==" {
				t.Fatalf("wrong asset selected: %+v", asset)
			}
			return downloaded, os.WriteFile(downloaded, []byte("deb"), 0o600)
		}
		info := productInstallation(t, m.Scan(false), "opencode.desktop")
		install := "pkexec env DEBIAN_FRONTEND=noninteractive apt-get install -y"
		if info.Capability != Automatic || info.Plan == nil || info.Plan.Action != ActionPackage || !info.Plan.RequiresAdmin ||
			strings.Join(info.Plan.Command, " ") != install || info.Plan.Asset.SHA512 != "deb==" {
			t.Fatalf("deb plan is wrong: %+v %+v", info, info.Plan)
		}
		args := append(strings.Fields(install)[1:], downloaded)
		mr.SetResult("pkexec", args, runner.MockResult{ExitCode: 126})
		if result := m.Execute(*info.Plan, nil); result.Outcome != "cancelled" {
			t.Fatalf("a refused password prompt was not reported as cancelled: %+v", result)
		}
		mr.SetResult("pkexec", args, runner.MockResult{})
		m.runner = &hookRunner{MockRunner: mr,
			when: func(name string, _ []string) bool { return name == "pkexec" },
			update: func() {
				mr.SetResult("dpkg-query", []string{"-W", "-f=" + debQueryFormat, "opencode-desktop"}, runner.MockResult{Stdout: "opencode-desktop\t2.0.29\tamd64\tinstalled"})
			}}
		if result := m.Execute(*info.Plan, nil); result.Outcome != "updated" || result.NewVersion != "2.0.29" {
			t.Fatalf("deb install was not verified: %+v", result)
		}
	})
	t.Run("rpm keeps its release out of the comparison", func(t *testing.T) {
		m, mr := linuxFixture(t)
		mr.SetLookPath("rpm", "rpm", nil)
		mr.SetResultByName("rpm", runner.MockResult{ExitCode: 1})
		mr.SetResult("rpm", []string{"-q", "--qf", rpmQueryFormat, "opencode-desktop"}, runner.MockResult{Stdout: "opencode-desktop\t2.0.28-1\tx86_64"})
		mr.SetResult("rpm", []string{"-ql", "opencode-desktop"}, runner.MockResult{Stdout: "/usr/share/applications/opencode-desktop.desktop\n"})
		m.fetch = func(context.Context, string, int64) ([]byte, error) { return []byte(manifest), nil }
		// Without pkexec the Web UI cannot authorize, so it only offers the official entry.
		info := productInstallation(t, m.Scan(false), "opencode.desktop")
		if !info.UpgradeAvailable || info.Capability != Guided || info.Reason != "package_manager_authorization" {
			t.Fatalf("rpm without pkexec is wrong: %+v", info)
		}
		mr.SetLookPath("pkexec", "pkexec", nil)
		mr.SetLookPath("dnf", "dnf", nil)
		info = productInstallation(t, m.Scan(false), "opencode.desktop")
		if info.Capability != Automatic || info.Plan == nil || strings.Join(info.Plan.Command, " ") != "pkexec dnf install -y" ||
			info.Plan.Asset.SHA512 != "rpm==" || info.Version != "2.0.28-1" {
			t.Fatalf("rpm plan is wrong: %+v %+v", info, info.Plan)
		}
		m.EnableTerminal()
		if info := productInstallation(t, m.Scan(false), "opencode.desktop"); info.Capability != Privileged || info.Plan.Action != ActionPackage || info.Plan.Command[0] != "sudo" {
			t.Fatalf("terminal rpm plan should use sudo: %+v", info.Plan)
		}
	})
}
