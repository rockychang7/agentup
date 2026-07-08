package upgrader

import (
	"fmt"
	"io"
	"strings"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/platform"
	"agentup/internal/runner"
)

// Upgrader handles upgrading AI coding agent CLI tools
// based on their detected installation method.
type Upgrader struct {
	runner   runner.Runner
	platform platform.Platform
	detector *detector.Detector
	tools    []config.ToolConfig
	output   io.Writer // progress messages are written here
}

// New creates a new Upgrader with the given runner, platform, detector, tools,
// and output writer for progress messages. If output is nil, progress
// messages are discarded.
func New(r runner.Runner, p platform.Platform, det *detector.Detector, tools []config.ToolConfig, output io.Writer) *Upgrader {
	if output == nil {
		output = io.Discard
	}
	return &Upgrader{
		runner:   r,
		platform: p,
		detector: det,
		tools:    tools,
		output:   output,
	}
}

// UpgradeAll upgrades all installed tools.
// Tools that are not installed are skipped.
// If one tool fails to upgrade, it does not affect the others.
// Progress messages are written to the output writer.
func (u *Upgrader) UpgradeAll() []model.UpgradeResult {
	results := make([]model.UpgradeResult, 0, len(u.tools))
	total := len(u.tools)

	for i, tool := range u.tools {
		index := i + 1
		result := u.upgradeToolWithProgress(tool, index, total)
		results = append(results, result)
	}

	return results
}

// Upgrade upgrades a single tool by name.
// Returns an error if the tool name is not recognized.
func (u *Upgrader) Upgrade(name model.ToolName) (model.UpgradeResult, error) {
	for _, tool := range u.tools {
		if tool.Name == name {
			return u.upgradeToolWithProgress(tool, 1, 1), nil
		}
	}
	return model.UpgradeResult{}, fmt.Errorf("unsupported tool: %s", name)
}

// upgradeToolWithProgress wraps upgradeTool with progress output.
func (u *Upgrader) upgradeToolWithProgress(tool config.ToolConfig, index, total int) model.UpgradeResult {
	// Pre-detect to get info for progress message
	info, _ := u.detector.Detect(tool.Name)

	if !info.Installed {
		fmt.Fprintf(u.output, "[%d/%d] %s: not installed, skipping\n", index, total, tool.Name)
		return model.UpgradeResult{
			Name:    tool.Name,
			Status:  model.UpgradeStatusSkipped,
			Message: "not installed",
		}
	}

	// Print progress before upgrade starts
	methodLabel := string(info.InstallMethod)
	fmt.Fprintf(u.output, "[%d/%d] Upgrading %s (%s)...\n", index, total, tool.Name, methodLabel)

	return u.upgradeTool(tool)
}

// upgradeTool performs the upgrade for a single tool.
func (u *Upgrader) upgradeTool(tool config.ToolConfig) model.UpgradeResult {
	result := model.UpgradeResult{
		Name:       tool.Name,
		Status:     model.UpgradeStatusSkipped,
		OldVersion: "",
		NewVersion: "",
		Message:    "",
	}

	// Step 1: Detect current state
	info, err := u.detector.Detect(tool.Name)
	if err != nil {
		result.Message = fmt.Sprintf("detection failed: %v", err)
		return result
	}

	if !info.Installed {
		result.Message = "not installed"
		return result
	}

	result.OldVersion = info.Version

	// Step 2: Check if upgrade is supported
	if !tool.UpgradeSupported {
		result.Message = fmt.Sprintf("automatic upgrade not supported, please upgrade manually: %s", tool.ManualUpgradeURL)
		return result
	}

	// Step 3: Check if install method supports upgrade
	if info.InstallMethod == model.InstallMethodBinary || info.InstallMethod == model.InstallMethodUnknown {
		result.Message = fmt.Sprintf("install method is '%s', cannot auto-upgrade. Please upgrade manually: %s",
			info.InstallMethod, tool.ManualUpgradeURL)
		return result
	}

	// Step 4: Build and execute the upgrade command
	cmdName, cmdArgs, err := u.buildUpgradeCommand(tool, info.InstallMethod, info.Path)
	if err != nil {
		result.Status = model.UpgradeStatusFailed
		result.Message = err.Error()
		return result
	}

	_, stderr, exitCode, runErr := u.runner.Run(cmdName, cmdArgs...)
	if runErr != nil {
		result.Status = model.UpgradeStatusFailed
		result.Message = classifyError(runErr, stderr, cmdName)
		return result
	}
	if exitCode != 0 {
		result.Status = model.UpgradeStatusFailed
		result.Message = classifyExitError(stderr, exitCode, cmdName)
		return result
	}

	// Step 5: Re-detect to get the new version
	newInfo, _ := u.detector.Detect(tool.Name)
	result.NewVersion = newInfo.Version

	// Step 6: Determine success
	if result.NewVersion != result.OldVersion && result.NewVersion != "unknown" {
		result.Status = model.UpgradeStatusSuccess
	} else if result.NewVersion == "unknown" {
		// Version couldn't be read after upgrade, but command succeeded
		result.Status = model.UpgradeStatusSuccess
		result.Message = "upgrade command succeeded, but could not verify new version"
	} else {
		// Version didn't change — might already be latest
		result.Status = model.UpgradeStatusSuccess
		result.Message = "already up to date"
	}

	return result
}

// buildUpgradeCommand constructs the upgrade command based on install method.
// For self-update tools, it uses the tool's own binary with SelfUpdateArgs.
func (u *Upgrader) buildUpgradeCommand(tool config.ToolConfig, method model.InstallMethod, binaryPath string) (string, []string, error) {
	switch method {
	case model.InstallMethodSelfUpdate:
		if len(tool.SelfUpdateArgs) == 0 {
			return "", nil, fmt.Errorf("self-update command not configured for %s", tool.Name)
		}
		return binaryPath, tool.SelfUpdateArgs, nil
	case model.InstallMethodNpm:
		return "npm", []string{"install", "-g", tool.NpmPackage + "@latest"}, nil
	case model.InstallMethodPnpm:
		return "pnpm", []string{"add", "-g", tool.NpmPackage}, nil
	case model.InstallMethodYarn:
		return "yarn", []string{"global", "upgrade", tool.NpmPackage}, nil
	case model.InstallMethodBrew:
		if tool.BrewCask {
			return "brew", []string{"upgrade", "--cask", tool.BrewFormula}, nil
		}
		return "brew", []string{"upgrade", tool.BrewFormula}, nil
	case model.InstallMethodWinget:
		return "winget", []string{"upgrade", tool.WingetID}, nil
	case model.InstallMethodScoop:
		return "scoop", []string{"update", tool.ScoopPackage}, nil
	default:
		return "", nil, fmt.Errorf("install method '%s' does not support automatic upgrade", method)
	}
}

// classifyError converts a raw execution error into a user-friendly message.
func classifyError(err error, stderr, cmdName string) string {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "no such file") || strings.Contains(errStr, "not found"):
		return fmt.Sprintf("package manager '%s' not found. Please install it or upgrade manually.", cmdName)
	case strings.Contains(errStr, "permission denied"):
		return fmt.Sprintf("permission denied while running '%s'. Try running with appropriate permissions.", cmdName)
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline"):
		return fmt.Sprintf("'%s' timed out. Please check your network connection and try again.", cmdName)
	default:
		if stderr != "" {
			return fmt.Sprintf("upgrade failed: %s", strings.TrimSpace(stderr))
		}
		return fmt.Sprintf("upgrade failed: %v", err)
	}
}

// classifyExitError converts a non-zero exit code into a user-friendly message.
func classifyExitError(stderr string, exitCode int, cmdName string) string {
	switch {
	case strings.Contains(stderr, "EACCES") || strings.Contains(stderr, "permission"):
		return fmt.Sprintf("permission denied while running '%s'. Try running with appropriate permissions.", cmdName)
	case strings.Contains(stderr, "ETIMEDOUT") || strings.Contains(stderr, "network"):
		return fmt.Sprintf("network error during '%s'. Please check your connection and try again.", cmdName)
	case strings.Contains(stderr, "ECONNREFUSED") || strings.Contains(stderr, "ENOTFOUND"):
		return fmt.Sprintf("network error during '%s'. Please check your connection and try again.", cmdName)
	default:
		errMsg := strings.TrimSpace(stderr)
		if errMsg == "" {
			errMsg = fmt.Sprintf("'%s' exited with code %d", cmdName, exitCode)
		}
		return fmt.Sprintf("upgrade failed: %s", errMsg)
	}
}
