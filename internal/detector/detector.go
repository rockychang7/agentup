package detector

import (
	"fmt"
	"regexp"
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/platform"
	"agentup/internal/runner"
)

// Detector orchestrates the detection of AI coding agent CLI tools.
// It checks if each tool is installed, retrieves its version,
// and determines the installation method.
type Detector struct {
	runner            runner.Runner
	platform          platform.Platform
	tools             []config.ToolConfig
	enableLatestCheck bool // when true, detectTool queries registry for latest version
}

// New creates a new Detector with the given runner, platform, and tool configs.
// Latest version checking is enabled by default.
func New(r runner.Runner, p platform.Platform, tools []config.ToolConfig) *Detector {
	return &Detector{
		runner:            r,
		platform:          p,
		tools:             tools,
		enableLatestCheck: true,
	}
}

// NewWithoutLatestCheck creates a Detector that skips latest version checking.
// Use this when you only need the installed version (e.g., during upgrades)
// to avoid unnecessary network requests.
func NewWithoutLatestCheck(r runner.Runner, p platform.Platform, tools []config.ToolConfig) *Detector {
	return &Detector{
		runner:            r,
		platform:          p,
		tools:             tools,
		enableLatestCheck: false,
	}
}

// DetectAll detects all configured tools and returns their info.
func (d *Detector) DetectAll() []model.ToolInfo {
	results := make([]model.ToolInfo, 0, len(d.tools))
	for _, tool := range d.tools {
		info := d.detectTool(tool)
		results = append(results, info)
	}
	return results
}

// Detect detects a single tool by name.
// Returns an error if the tool name is not recognized.
func (d *Detector) Detect(name model.ToolName) (model.ToolInfo, error) {
	for _, tool := range d.tools {
		if tool.Name == name {
			return d.detectTool(tool), nil
		}
	}
	return model.ToolInfo{}, fmt.Errorf("unsupported tool: %s", name)
}

// detectTool performs the full detection for a single tool config.
func (d *Detector) detectTool(tool config.ToolConfig) model.ToolInfo {
	info := model.ToolInfo{
		Name:             tool.Name,
		Installed:        false,
		Version:          "",
		InstallMethod:    model.InstallMethodUnknown,
		Path:             "",
		UpgradeSupported: tool.UpgradeSupported,
	}

	// Step 1: Check if the binary exists in PATH
	path, err := d.runner.LookPath(tool.BinaryName)
	if err != nil {
		// Binary not found — tool is not installed
		return info
	}

	info.Installed = true
	info.Path = path

	// Step 2: Get the version
	info.Version = d.getVersion(tool, path)

	// Step 3: Determine the install method
	info.InstallMethod = d.detectInstallMethod(tool, path)

	// If install method is unknown but tool is binary-only, mark as binary
	if info.InstallMethod == model.InstallMethodUnknown && tool.BinaryOnly {
		info.InstallMethod = model.InstallMethodBinary
	}

	// If tool has a self-update command and was installed as binary,
	// mark it as self-update to enable automatic upgrades
	if info.InstallMethod == model.InstallMethodBinary && len(tool.SelfUpdateArgs) > 0 {
		info.InstallMethod = model.InstallMethodSelfUpdate
	}

	// Step 4: Determine if upgrade is supported based on install method
	// Can upgrade if:
	// 1. Install method is a package manager (npm/pnpm/yarn/brew/winget/scoop)
	// 2. Or tool has a self-update command (self-update method)
	if info.InstallMethod == model.InstallMethodSelfUpdate {
		info.UpgradeSupported = true
	} else if info.InstallMethod == model.InstallMethodBinary ||
		info.InstallMethod == model.InstallMethodUnknown {
		info.UpgradeSupported = false
	}

	// Step 5: Check latest version from registry (optional)
	if d.enableLatestCheck {
		info.LatestVersion = d.checkLatestVersion(tool, info.InstallMethod, info.Version)

		// Step 6: Determine if an upgrade is available
		if info.LatestVersion != "" && info.Version != "" && info.Version != "unknown" {
			info.UpgradeAvailable = compareVersions(info.Version, info.LatestVersion) < 0
		}
	}

	return info
}

// getVersion tries to get the tool's version by running version commands.
func (d *Detector) getVersion(tool config.ToolConfig, path string) string {
	// Try each version argument combination
	for _, args := range [][]string{tool.VersionArgs, {"-v"}, {"version"}} {
		if len(args) == 0 {
			continue
		}
		stdout, _, exitCode, err := d.runner.Run(path, args...)
		if err != nil || exitCode != 0 {
			continue
		}
		version := parseVersion(stdout)
		if version != "" {
			return version
		}
	}
	return "unknown"
}

// detectInstallMethod determines how the tool was installed.
// It checks package managers in priority order based on the platform.
func (d *Detector) detectInstallMethod(tool config.ToolConfig, binaryPath string) model.InstallMethod {
	managers := d.platform.SupportedManagers()

	for _, mgr := range managers {
		var method model.InstallMethod
		switch mgr {
		case "npm":
			method = d.detectNpm(tool)
		case "pnpm":
			method = d.detectPnpm(tool)
		case "yarn":
			method = d.detectYarn(tool)
		case "brew":
			method = d.detectBrew(tool)
		case "winget":
			method = d.detectWinget(tool)
		case "scoop":
			method = d.detectScoop(tool, binaryPath)
		default:
			continue
		}
		if method != model.InstallMethodUnknown {
			return method
		}
	}

	// Binary exists but no package manager claims it — manual binary install
	return model.InstallMethodBinary
}

// versionRegex matches semantic version patterns like 1.0.0, 0.12.3, v2.1.0-beta
var versionRegex = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)`)

// parseVersion extracts a version string from command output.
func parseVersion(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	// Try to find a version pattern in the output
	matches := versionRegex.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}

	// If no version pattern found, return the first non-empty line
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}

	return ""
}
