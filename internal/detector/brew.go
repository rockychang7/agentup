package detector

import (
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// detectBrew checks if the tool was installed via Homebrew.
// Returns model.InstallMethodBrew if confirmed, otherwise model.InstallMethodUnknown.
// This is only called on macOS where brew is supported.
func (d *Detector) detectBrew(tool config.ToolConfig) model.InstallMethod {
	if tool.BrewFormula == "" {
		return model.InstallMethodUnknown
	}

	// Check if brew itself is available
	if _, _, _, err := d.runner.Run("brew", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	// Check if the formula/cask is installed
	stdout, _, exitCode, err := d.runner.Run("brew", "list", tool.BrewFormula)
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	// brew list outputs the formula name and installed files if installed
	if strings.Contains(stdout, tool.BrewFormula) {
		return model.InstallMethodBrew
	}

	return model.InstallMethodUnknown
}
