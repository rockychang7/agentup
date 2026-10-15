package detector

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"golang.org/x/mod/semver"

	"agentup/internal/config"
	"agentup/internal/model"
)

// CheckLatest records failures separately from tools that have no registry lookup.
func (d *Detector) CheckLatest(info model.ToolInfo) model.ToolInfo {
	info.LatestVersion = ""
	info.UpgradeAvailable = false
	info.CheckMessage = ""
	info.LatestCheck = model.CheckUnavailable
	if !info.Installed {
		return info
	}
	for _, tool := range d.tools {
		if tool.Name != info.Name {
			continue
		}
		resolved, err := config.ForVersion(tool, info.Version)
		if err != nil {
			info.CheckMessage = err.Error()
			return info
		}
		tool = resolved
		latest, err := d.checkLatestVersion(tool, info.InstallMethod, info.Version)
		if err == nil && latest != "" {
			err = config.ValidateUpgradeVersion(tool, info.Version, latest)
		}
		if err != nil {
			info.LatestCheck = model.CheckFailed
			info.CheckMessage = err.Error()
		} else if latest != "" {
			info.LatestCheck = model.CheckOK
			info.LatestVersion = latest
			if validVersion(info.Version) {
				info.UpgradeAvailable = compareVersions(info.Version, latest) < 0
			}
		} else {
			info.CheckMessage = "no latest-version lookup for this installation source"
		}
		return info
	}
	return info
}

func (d *Detector) checkLatestVersion(tool config.ToolConfig, method model.InstallMethod, current string) (string, error) {
	switch method {
	case model.InstallMethodNpm, model.InstallMethodPnpm, model.InstallMethodYarn:
		return d.checkNpmLatestVersion(tool)
	case model.InstallMethodBrew:
		return d.checkBrewLatestVersion(tool)
	case model.InstallMethodScoop:
		return d.checkScoopLatestVersion(tool, current)
	case model.InstallMethodWinget:
		return d.checkWingetLatestVersion(tool, current)
	case model.InstallMethodSelfUpdate:
		if tool.Name == model.ToolOpenCode {
			return d.checkOpenCodeNativeLatest(tool)
		}
		if tool.NpmPackage != "" {
			return d.checkNpmLatestVersion(tool)
		}
	}
	return "", nil
}

func (d *Detector) query(name string, args ...string) (string, error) {
	stdout, stderr, code, err := d.runner.Run(name, args...)
	if err != nil {
		return "", fmt.Errorf("%s lookup: %w", name, err)
	}
	if code != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = fmt.Sprintf("exit code %d", code)
		}
		return "", fmt.Errorf("%s lookup failed: %s", name, detail)
	}
	return stdout, nil
}

func registryVersion(output string) (string, error) {
	match := versionRegex.FindStringSubmatch(output)
	if len(match) < 2 || !validVersion(match[1]) {
		return "", fmt.Errorf("registry did not return a recognizable version")
	}
	return match[1], nil
}

func (d *Detector) checkNpmLatestVersion(tool config.ToolConfig) (string, error) {
	if tool.NpmPackage == "" {
		return "", nil
	}
	out, err := d.query("npm", "view", tool.NpmPackage, "version")
	if err != nil {
		return "", err
	}
	return registryVersion(out)
}

type brewInfoJSON struct {
	Formulae []struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Versions struct {
			Stable string `json:"stable"`
		} `json:"versions"`
	} `json:"formulae"`
	Casks []struct {
		Token   string `json:"token"`
		Version string `json:"version"`
	} `json:"casks"`
}

func (d *Detector) checkBrewLatestVersion(tool config.ToolConfig) (string, error) {
	if tool.BrewFormula == "" {
		return "", nil
	}
	out, err := d.query("brew", "info", "--json=v2", tool.BrewFormula)
	if err != nil {
		return "", err
	}
	var info brewInfoJSON
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", fmt.Errorf("invalid Homebrew response: %w", err)
	}
	if tool.BrewCask {
		for _, cask := range info.Casks {
			if cask.Token == tool.BrewFormula {
				return registryVersion(cask.Version)
			}
		}
	} else {
		for _, formula := range info.Formulae {
			if formula.Name == tool.BrewFormula || formula.FullName == tool.BrewFormula ||
				formula.Name == path.Base(tool.BrewFormula) {
				return registryVersion(formula.Versions.Stable)
			}
		}
	}
	return "", fmt.Errorf("package missing from Homebrew response")
}

func (d *Detector) checkScoopLatestVersion(tool config.ToolConfig, current string) (string, error) {
	if tool.ScoopPackage == "" {
		return "", nil
	}
	out, err := d.query("scoop", "status")
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "up to date") || strings.Contains(lower, "everything is ok") {
		return registryVersion(current)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.EqualFold(fields[0], tool.ScoopPackage) {
			if idx := strings.Index(line, "->"); idx >= 0 {
				return registryVersion(line[idx+2:])
			}
			return "", fmt.Errorf("could not parse scoop update for %s", tool.ScoopPackage)
		}
	}
	if strings.Contains(lower, "installed") && strings.Contains(lower, "latest") {
		return registryVersion(current)
	}
	return "", fmt.Errorf("unrecognized scoop status response")
}

func (d *Detector) checkWingetLatestVersion(tool config.ToolConfig, current string) (string, error) {
	if tool.WingetID == "" {
		return "", nil
	}
	out, err := d.query("winget", "upgrade")
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "no updates") || strings.Contains(lower, "no available upgrade") {
		return registryVersion(current)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if strings.EqualFold(field, tool.WingetID) && len(fields) > i+2 {
				// Source may follow Available; do not mistake it for a version.
				return registryVersion(fields[i+2])
			}
		}
	}
	if strings.Contains(lower, "available") && strings.Contains(lower, "version") && strings.Contains(out, "---") {
		return registryVersion(current)
	}
	return "", fmt.Errorf("unrecognized winget upgrade response")
}

func validVersion(v string) bool {
	return semver.IsValid("v" + strings.TrimPrefix(v, "v"))
}

func compareVersions(v1, v2 string) int {
	return semver.Compare("v"+strings.TrimPrefix(v1, "v"), "v"+strings.TrimPrefix(v2, "v"))
}
