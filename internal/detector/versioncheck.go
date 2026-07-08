package detector

import (
	"encoding/json"
	"strconv"
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// checkLatestVersion queries the appropriate registry to find the latest
// available version for the tool based on its install method.
// Returns the latest version string, or "" if it could not be determined.
// For package managers that only report "outdated" status (scoop, winget),
// if the tool is NOT listed as outdated, the currentVersion is returned
// (meaning it's already up to date).
func (d *Detector) checkLatestVersion(tool config.ToolConfig, method model.InstallMethod, currentVersion string) string {
	switch method {
	case model.InstallMethodNpm, model.InstallMethodPnpm, model.InstallMethodYarn:
		return d.checkNpmLatestVersion(tool)
	case model.InstallMethodBrew:
		return d.checkBrewLatestVersion(tool)
	case model.InstallMethodScoop:
		return d.checkScoopLatestVersion(tool, currentVersion)
	case model.InstallMethodWinget:
		return d.checkWingetLatestVersion(tool, currentVersion)
	case model.InstallMethodSelfUpdate:
		// For self-update tools, try npm registry if the package is on npm
		if tool.NpmPackage != "" {
			return d.checkNpmLatestVersion(tool)
		}
		return ""
	default:
		return ""
	}
}

// checkNpmLatestVersion queries the npm registry for the latest version
// of the package using `npm view <package> version`.
func (d *Detector) checkNpmLatestVersion(tool config.ToolConfig) string {
	if tool.NpmPackage == "" {
		return ""
	}
	stdout, _, exitCode, err := d.runner.Run("npm", "view", tool.NpmPackage, "version")
	if err != nil || exitCode != 0 {
		return ""
	}
	return parseVersion(strings.TrimSpace(stdout))
}

// brewInfoJSON represents the JSON output of `brew info --json=v2`.
type brewInfoJSON struct {
	Formulae []brewFormula `json:"formulae"`
	Casks    []brewCask    `json:"casks"`
}

type brewFormula struct {
	Name     string `json:"name"`
	Versions struct {
		Stable string `json:"stable"`
	} `json:"versions"`
}

type brewCask struct {
	Token   string `json:"token"`
	Version string `json:"version"`
}

// checkBrewLatestVersion queries Homebrew for the latest version of the formula/cask.
func (d *Detector) checkBrewLatestVersion(tool config.ToolConfig) string {
	if tool.BrewFormula == "" {
		return ""
	}
	stdout, _, exitCode, err := d.runner.Run("brew", "info", "--json=v2", tool.BrewFormula)
	if err != nil || exitCode != 0 {
		return ""
	}

	var info brewInfoJSON
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		return ""
	}

	if tool.BrewCask {
		for _, cask := range info.Casks {
			if cask.Token == tool.BrewFormula {
				return cask.Version
			}
		}
	} else {
		for _, formula := range info.Formulae {
			if formula.Name == tool.BrewFormula {
				return formula.Versions.Stable
			}
		}
	}
	return ""
}

// checkScoopLatestVersion checks scoop status for available updates.
// If the package is listed as outdated, the latest version is extracted.
// If the package is NOT listed, it means it's up to date, so currentVersion is returned.
func (d *Detector) checkScoopLatestVersion(tool config.ToolConfig, currentVersion string) string {
	if tool.ScoopPackage == "" {
		return ""
	}
	stdout, _, exitCode, err := d.runner.Run("scoop", "status")
	if err != nil || exitCode != 0 {
		// If scoop status fails, we can't determine the latest version
		return ""
	}

	// If "up to date" message is in output, no updates available
	lowerOutput := strings.ToLower(stdout)
	if strings.Contains(lowerOutput, "up to date") || strings.Contains(lowerOutput, "everything is ok") {
		return currentVersion
	}

	// Search for the package name in the output
	// Output format varies, but typically includes lines like:
	// "opencode   1.17.15 -> 1.18.0"
	lowerPkg := strings.ToLower(tool.ScoopPackage)
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerPkg) {
			// Try to extract the version after "->"
			if idx := strings.Index(line, "->"); idx >= 0 {
				afterArrow := strings.TrimSpace(line[idx+2:])
				// Take the first token after "->"
				parts := strings.Fields(afterArrow)
				if len(parts) > 0 {
					return parseVersion(parts[0])
				}
			}
			// If no "->" found, try to extract the last version-like token
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				lastField := fields[len(fields)-1]
				if v := parseVersion(lastField); v != "" {
					return v
				}
			}
		}
	}

	// Package not found in outdated list — it's up to date
	return currentVersion
}

// checkWingetLatestVersion checks winget upgrade list for available updates.
// If the package is listed, the available version is extracted.
// If not listed, it means it's up to date, so currentVersion is returned.
func (d *Detector) checkWingetLatestVersion(tool config.ToolConfig, currentVersion string) string {
	if tool.WingetID == "" {
		return ""
	}
	stdout, _, exitCode, err := d.runner.Run("winget", "upgrade")
	if err != nil || exitCode != 0 {
		return ""
	}

	// If "No updates" or "No installed package" message is in output
	lowerOutput := strings.ToLower(stdout)
	if strings.Contains(lowerOutput, "no updates") || strings.Contains(lowerOutput, "no package") {
		return currentVersion
	}

	// Search for the package ID in the output
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(tool.WingetID)) {
			// winget upgrade output has columns: Name, Id, Version, Available
			// Try to extract the last field as the available version
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				lastField := fields[len(fields)-1]
				if v := parseVersion(lastField); v != "" {
					return v
				}
			}
		}
	}

	// Package not found in upgrade list — it's up to date
	return currentVersion
}

// compareVersions compares two semantic version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2.
// Pre-release and build metadata are stripped before comparison.
func compareVersions(v1, v2 string) int {
	// Strip pre-release and build metadata
	v1 = strings.Split(strings.Split(v1, "+")[0], "-")[0]
	v2 = strings.Split(strings.Split(v2, "+")[0], "-")[0]

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}
