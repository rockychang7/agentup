package detector

import (
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// detectNpm checks if the tool was installed via npm global.
// Returns model.InstallMethodNpm if confirmed, otherwise model.InstallMethodUnknown.
func (d *Detector) detectNpm(tool config.ToolConfig) model.InstallMethod {
	if tool.NpmPackage == "" {
		return model.InstallMethodUnknown
	}

	// First check if npm itself is available
	if _, _, _, err := d.runner.Run("npm", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	// Check if the package is in npm global list
	stdout, _, exitCode, err := d.runner.Run("npm", "list", "-g", tool.NpmPackage, "--depth=0")
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	// npm list output contains the package name if installed
	if strings.Contains(stdout, tool.NpmPackage) {
		return model.InstallMethodNpm
	}

	return model.InstallMethodUnknown
}

// detectPnpm checks if the tool was installed via pnpm global.
// Returns model.InstallMethodPnpm if confirmed, otherwise model.InstallMethodUnknown.
func (d *Detector) detectPnpm(tool config.ToolConfig) model.InstallMethod {
	if tool.NpmPackage == "" {
		return model.InstallMethodUnknown
	}

	// Check if pnpm itself is available
	if _, _, _, err := d.runner.Run("pnpm", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	// Check if the package is in pnpm global list
	stdout, _, exitCode, err := d.runner.Run("pnpm", "list", "-g", tool.NpmPackage, "--depth=0")
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	if strings.Contains(stdout, tool.NpmPackage) {
		return model.InstallMethodPnpm
	}

	return model.InstallMethodUnknown
}

// detectYarn checks if the tool was installed via yarn global.
// Returns model.InstallMethodYarn if confirmed, otherwise model.InstallMethodUnknown.
func (d *Detector) detectYarn(tool config.ToolConfig) model.InstallMethod {
	if tool.NpmPackage == "" {
		return model.InstallMethodUnknown
	}

	// Check if yarn itself is available
	if _, _, _, err := d.runner.Run("yarn", "--version"); err != nil {
		return model.InstallMethodUnknown
	}

	// Check if the package is in yarn global list
	stdout, _, exitCode, err := d.runner.Run("yarn", "global", "list", "--pattern", tool.NpmPackage)
	if err != nil || exitCode != 0 {
		return model.InstallMethodUnknown
	}

	if strings.Contains(stdout, tool.NpmPackage) {
		return model.InstallMethodYarn
	}

	return model.InstallMethodUnknown
}
