package desktop

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agentup/internal/model"
	"agentup/internal/runner"
)

// antigravityProductCode is both the uninstall key and the WinGet ProductCode.
const antigravityProductCode = "121a0be4-63bd-531e-acf8-fc3924c7e984"

type wingetTarget struct {
	id, source string
}

// wingetTargetFor maps only identities verified against local install records.
// A display-name match is never enough to hand an installation to WinGet.
func wingetTargetFor(info Installation) (wingetTarget, bool) {
	switch {
	case info.ProductID == "codex.desktop" && info.Source == "msix":
		return wingetTarget{"9PLM9XGG6VKS", "msstore"}, true
	// Anthropic's WinGet manifest lists the MSIX package family (matched during discovery)
	// and the per-user installer by its uninstall key.
	case info.ProductID == "claude.desktop" && info.Source == "msix":
		return wingetTarget{"Anthropic.Claude", "winget"}, true
	case info.ProductID == "claude.desktop" && info.Source == "installer" && info.Scope == "user" && info.PackageID == "AnthropicClaude":
		return wingetTarget{"Anthropic.Claude", "winget"}, true
	case info.ProductID == "antigravity.desktop" && info.Source == "installer" &&
		strings.EqualFold(strings.Trim(info.PackageID, "{}"), antigravityProductCode):
		return wingetTarget{"Google.Antigravity", "winget"}, true
	case info.ProductID == "opencode.desktop" && info.Source == "installer" && appMajor(info.Version) == 1:
		// The WinGet entry only carries 1.x. It also matches 2.x installs, which it would downgrade.
		return wingetTarget{"SST.OpenCodeDesktop", "winget"}, true
	}
	return wingetTarget{}, false
}

// parseWingetList reads the row of an exact ID. Column headers are localized, so only
// the tokens after the ID are used: the installed version and, if listed, the available one.
func parseWingetList(out, id string) (installed, available string, found bool) {
	for _, line := range strings.FieldsFunc(out, func(r rune) bool { return r == '\n' || r == '\r' }) {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field != id || i+1 >= len(fields) {
				continue
			}
			installed = fields[i+1]
			if i+2 < len(fields) && numericVersion.MatchString(fields[i+2]) {
				available = fields[i+2]
			}
			return installed, available, true
		}
	}
	return "", "", false
}

func (m *Manager) wingetCheck(info *Installation, winget string) {
	target, ok := wingetTargetFor(*info)
	if !ok {
		return
	}
	out, _, code, err := runner.WithTimeout(m.runner, 90*time.Second).Run(winget,
		"list", "--id", target.id, "--exact", "--source", target.source, "--disable-interactivity")
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, "WinGet check failed: "+err.Error()
		return
	}
	if code != 0 {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, wingetMessage(code)
		switch uint32(code) {
		case 0x8A150014:
			info.Reason = "winget_unmatched"
		case 0x8A150046, 0x8A150041:
			info.Reason = "winget_agreement_required"
		}
		return
	}
	installed, available, found := parseWingetList(out, target.id)
	if cmp, ok := compareVersion(installed, info.Version); !found || !ok || cmp != 0 {
		info.Reason = "winget_unmatched"
		info.CheckMessage = fmt.Sprintf("WinGet %s does not report this installation and version.", target.id)
		return
	}
	info.Channel, info.Capability, info.Reason = target.source, Automatic, ""
	info.spec = planSpec{
		command:    []string{winget, "upgrade", "--id", target.id, "--exact", "--source", target.source, "--disable-interactivity"},
		standalone: info.ProductID == "codex.desktop",
	}
	switch {
	case available != "":
		info.LatestSource = LatestWinget
		candidate(info, available)
	case target.source == "msstore":
		// WinGet reports Store versions as "Unknown", so `winget upgrade` always ends with
		// "no applicable upgrade". The Store catalog gives the version, and an install with
		// --force hands the update to the Store, which keeps the app's data. Unlike
		// `upgrade`, `install` asks for the Store package agreement on every run; the
		// user accepts it by confirming this plan, so it is passed on their behalf.
		info.spec.command = []string{winget, "install", "--id", target.id, "--exact", "--source", target.source, "--force", "--accept-package-agreements", "--disable-interactivity"}
		info.LatestSource = LatestMSStore
		if latest, err := m.storeVersion(target.id, info.PackageID, info.Arch); err == nil {
			candidate(info, latest)
		} else {
			info.LatestCheck, info.CheckMessage = model.CheckUnavailable, "Microsoft Store catalog: "+err.Error()
			info.spec.unknownOK = true
		}
	default:
		// The community source omits the Available column when nothing newer exists.
		info.LatestSource = LatestWinget
		candidate(info, installed)
	}
}

// storeVersion reads the newest package version for this package name and
// architecture from the Microsoft Store catalog.
func (m *Manager) storeVersion(productID, name, arch string) (string, error) {
	data, err := m.fetch(m.ctx, "https://displaycatalog.mp.microsoft.com/v7.0/products?bigIds="+productID+"&market=US&languages=en-us", 2<<20)
	if err != nil {
		return "", err
	}
	var catalog struct {
		Products []struct {
			DisplaySkuAvailabilities []struct {
				Sku struct {
					Properties struct {
						Packages []struct {
							PackageFullName string
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return "", err
	}
	storeArch := map[string]string{"amd64": "x64", "arm64": "arm64"}[arch]
	latest := ""
	for _, product := range catalog.Products {
		for _, sku := range product.DisplaySkuAvailabilities {
			for _, pkg := range sku.Sku.Properties.Packages {
				// PackageFullName: Name_Version_Arch_ResourceId_PublisherId
				parts := strings.Split(pkg.PackageFullName, "_")
				if len(parts) != 5 || parts[0] != name || (parts[2] != storeArch && parts[2] != "neutral") {
					continue
				}
				if cmp, ok := compareVersion(latest, parts[1]); latest == "" || (ok && cmp < 0) {
					latest = parts[1]
				}
			}
		}
	}
	if !numericVersion.MatchString(latest) {
		return "", fmt.Errorf("no %s package for %s", arch, name)
	}
	return latest, nil
}

type wingetCode struct{ outcome, message string }

// Return codes from the WinGet documentation; text output is localized and never parsed.
var wingetCodes = map[uint32]wingetCode{
	0x8A15002B: {"no_update", "WinGet found no applicable update for this installation."},
	0x8A150014: {"failed", "WinGet no longer matches this installation. Refresh and check the install source."},
	0x8A150046: {"failed", "WinGet source agreements are not accepted. Run winget once in a terminal to review them, then retry."},
	0x8A150041: {"failed", "Package agreements are not accepted. Review them with winget in a terminal, then retry."},
	0x8A150101: {"failed", "The app is still running. Fully quit it and retry."},
	0x8A150103: {"failed", "App files are in use. Fully quit the app and retry."},
	0x8A150111: {"failed", "The app is in use by another application. Close it and retry."},
	0x8A150102: {"failed", "Another installation is in progress. Retry later."},
	0x8A150109: {"unverified", "Restart Windows to finish the installation, then refresh."},
	0x8A15010A: {"failed", "Restart Windows, then retry the update."},
	0x8A15010B: {"unverified", "Windows will restart to finish the installation. Refresh afterwards."},
	0x8A150107: {"failed", "No network connection. Connect and retry."},
	0x8A15010C: {"cancelled", "The installation was cancelled."},
	0x8A150077: {"cancelled", "Authentication was cancelled."},
	0x8A15003A: {"failed", "Blocked by organization policy."},
	0x8A15010F: {"failed", "Organization policy prevents this installation."},
	0x8A15001B: {"failed", "The Microsoft Store client is blocked by policy."},
	0x8A15001C: {"failed", "This Store app is blocked by policy."},
	0x8A150019: {"failed", "Administrator rights are required."},
	0x8A150011: {"failed", "The installer hash does not match the WinGet manifest. Nothing was installed."},
	0x8A150050: {"failed", "WinGet cannot determine the upgrade version."},
}

func wingetMessage(code int) string {
	if known, ok := wingetCodes[uint32(code)]; ok {
		return known.message
	}
	return fmt.Sprintf("WinGet exited 0x%08X.", uint32(code))
}

func commandFailure(plan Plan, code int, stderr string) (string, string) {
	name := strings.ToLower(filepath.Base(strings.ReplaceAll(plan.Command[0], `\`, "/")))
	if name == "winget" || name == "winget.exe" {
		if known, ok := wingetCodes[uint32(code)]; ok {
			return known.outcome, known.message
		}
		return "failed", wingetMessage(code)
	}
	if name == "pkexec" {
		switch code {
		case 126:
			return "cancelled", "The password prompt was cancelled or refused. Nothing was changed."
		case 127:
			return "failed", "No system password prompt could be shown. Run this in a terminal: sudo " + strings.Join(plan.Command[1:], " ")
		}
	}
	if name == "snap" && strings.Contains(stderr, "access denied") {
		// snapd asks the desktop's authorization agent; without one the refresh is refused.
		return "failed", "Snap needs administrator authorization and no password prompt appeared. Run this in a terminal: sudo snap " + strings.Join(plan.Command[1:], " ")
	}
	return "failed", fmt.Sprintf("Upgrade exited %d: %.4000s", code, strings.TrimSpace(stderr))
}
