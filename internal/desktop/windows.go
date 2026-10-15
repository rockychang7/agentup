package desktop

import (
	_ "embed"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

//go:embed windows_probe.ps1
var windowsProbe string

type windowsRecord struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
	Package   string `json:"package"`
	Family    string `json:"family"`
	Path      string `json:"path"`
	Scope     string `json:"scope"`
	Arch      string `json:"arch"`
	Source    string `json:"source"`
}

var displayVersionSuffix = regexp.MustCompile(` \d[\d.]*$`)

func windowsProduct(record windowsRecord) (Product, bool) {
	name := strings.ToLower(displayVersionSuffix.ReplaceAllString(record.Name, ""))
	publisher := strings.ToLower(record.Publisher)
	id := ""
	switch {
	case record.Source == "msix" && record.Package == "OpenAI.Codex" &&
		record.Family == "OpenAI.Codex_2p2nqsd0c76g0":
		id = "codex.desktop"
	case record.Source == "msix" && record.Family == claudeFamily:
		id = "claude.desktop"
	case record.Source != "msix" && name == "claude" && strings.Contains(publisher, "anthropic"):
		id = "claude.desktop"
	case name == "opencode" && (publisher == "opencode" || publisher == "anomaly"):
		id = "opencode.desktop"
	case name == "antigravity" && (publisher == "google" || publisher == "google llc"):
		major, err := strconv.Atoi(strings.Split(record.Version, ".")[0])
		if err == nil && major >= 2 {
			id = "antigravity.desktop"
		}
	}
	if id == "" || record.Path == "" {
		return Product{}, false
	}
	return FindProduct(id)
}

// claudeFamily is the MSIX package family published in Anthropic's WinGet manifest.
const claudeFamily = "Claude_pzs8sxrjxfjjc"

// Main executables of installer-based apps, read for their real architecture.
var windowsExecutables = map[string]string{
	"claude.desktop": "claude.exe", "opencode.desktop": "OpenCode.exe", "antigravity.desktop": "Antigravity.exe",
}

func windowsArch(arch string) string {
	switch strings.ToLower(arch) {
	case "x64":
		return "amd64"
	case "arm64":
		return "arm64"
	case "x86":
		return "386"
	case "neutral":
		return "neutral"
	}
	return "unknown"
}

func (m *Manager) scanWindows(snapshot *Snapshot, offline bool) {
	// Appx cannot be imported by PowerShell 7 on some Windows installations.
	shell, err := m.runner.LookPath("powershell.exe")
	if err != nil {
		snapshot.Issues = append(snapshot.Issues, "Windows PowerShell is required for desktop discovery.")
		return
	}
	out, err := m.run(shell, "-NoProfile", "-NonInteractive", "-Command", windowsProbe)
	if err != nil {
		snapshot.Issues = append(snapshot.Issues, err.Error())
		return
	}
	var data struct {
		Items     []windowsRecord `json:"items"`
		Processes []string        `json:"processes"`
		Issues    []string        `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		snapshot.Issues = append(snapshot.Issues, "Invalid Windows desktop discovery JSON: "+err.Error())
		return
	}
	if data.Items == nil || data.Processes == nil || data.Issues == nil {
		snapshot.Issues = append(snapshot.Issues, "Incomplete Windows desktop discovery JSON.")
		return
	}
	snapshot.Issues = append(snapshot.Issues, data.Issues...)
	for _, record := range data.Items {
		product, ok := windowsProduct(record)
		if !ok {
			continue
		}
		info := rawInstallation(product)
		info.Version, info.Path, info.PackageID = record.Version, record.Path, record.Package
		info.Source, info.Scope, info.Arch = record.Source, record.Scope, windowsArch(record.Arch)
		if record.Source == "installer" {
			exe := record.Path
			if !strings.EqualFold(filepath.Ext(exe), ".exe") {
				exe = filepath.Join(exe, windowsExecutables[product.ID])
			}
			info.Arch = peArchitecture(exe)
		}
		info.Running = "stopped"
		if len(data.Issues) != 0 {
			info.Running = "unknown"
		}
		path := strings.ToLower(strings.TrimRight(info.Path, `\`))
		for _, process := range data.Processes {
			process = strings.ToLower(process)
			if process == path || strings.HasPrefix(process, path+`\`) {
				info.Running = "running"
			}
		}
		if !offline {
			if winget, err := m.runner.LookPath("winget.exe"); err == nil {
				m.wingetCheck(&info, winget)
			}
			if info.ProductID == "opencode.desktop" && appMajor(info.Version) == 2 {
				m.vendorCheck(&info, "latest.yml")
			}
		}
		snapshot.Installations = append(snapshot.Installations, info)
	}
}
