package detector

import (
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// detectScoop checks if the tool was installed via scoop.
// Returns model.InstallMethodScoop if confirmed, otherwise model.InstallMethodUnknown.
// This is only called on Windows where scoop is supported.
func (d *Detector) detectScoop(tool config.ToolConfig, binaryPath string) model.InstallMethod {
	if tool.ScoopPackage == "" {
		return model.InstallMethodUnknown
	}

	// Quick check: if the binary path contains "scoop", it's likely a scoop install
	// This is the fastest and most reliable check on Windows
	normalizedPath := strings.ReplaceAll(strings.ToLower(binaryPath), "\\", "/")
	if strings.Contains(normalizedPath, "scoop") {
		return model.InstallMethodScoop
	}

	// Also check via scoop list command
	if _, _, _, err := d.runner.Run("scoop", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	stdout, _, exitCode, err := d.runner.Run("scoop", "list")
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	if strings.Contains(strings.ToLower(stdout), strings.ToLower(tool.ScoopPackage)) {
		return model.InstallMethodScoop
	}

	return model.InstallMethodUnknown
}
