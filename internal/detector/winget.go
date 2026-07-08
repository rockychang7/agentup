package detector

import (
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// detectWinget checks if the tool was installed via winget.
// Returns model.InstallMethodWinget if confirmed, otherwise model.InstallMethodUnknown.
// This is only called on Windows where winget is supported.
func (d *Detector) detectWinget(tool config.ToolConfig) model.InstallMethod {
	if tool.WingetID == "" {
		return model.InstallMethodUnknown
	}

	// Check if winget itself is available
	if _, _, _, err := d.runner.Run("winget", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	// Check if the package is installed via winget
	stdout, _, exitCode, err := d.runner.Run("winget", "list", tool.WingetID)
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	// winget list output contains the package ID if installed
	if strings.Contains(stdout, tool.WingetID) {
		return model.InstallMethodWinget
	}

	return model.InstallMethodUnknown
}
