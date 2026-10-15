package desktop

import (
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
		channel:    "winget",
		command:    []string{winget, "upgrade", "--id", target.id, "--exact", "--source", target.source, "--disable-interactivity"},
		standalone: info.ProductID == "codex.desktop",
	}
	switch {
	case available != "":
		info.LatestSource = LatestWinget
		candidate(info, available)
	case target.source == "msstore":
		// The Store source does not list available versions; the upgrade itself reports no_update.
		info.LatestSource, info.LatestCheck = LatestMSStore, model.CheckUnavailable
		info.CheckMessage = "Microsoft Store does not report an available version through WinGet."
		info.spec.unknownOK = true
	default:
		// The community source omits the Available column when nothing newer exists.
		info.LatestSource = LatestWinget
		candidate(info, installed)
	}
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
	return "failed", fmt.Sprintf("Upgrade exited %d: %.4000s", code, strings.TrimSpace(stderr))
}
