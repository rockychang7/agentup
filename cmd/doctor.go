package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"agentup/internal/model"
	"agentup/pkg/table"
)

// doctorCmd runs environment diagnostics.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run environment diagnostics",
	Long: `Check the current environment for upgrade capabilities.

This command checks:
  - Current operating system
  - Available package managers (npm, pnpm, yarn, brew, winget, scoop)
  - Whether supported CLI tools are found in PATH

Example:
  agentup doctor`,
	Run: func(cmd *cobra.Command, args []string) {
		result := runDoctor()
		table.PrintDoctor(result)
	},
}

// runDoctor performs all environment checks and returns a DoctorResult.
func runDoctor() model.DoctorResult {
	result := model.DoctorResult{
		OS:       appPlatform.Name(),
		Managers: checkManagers(),
		Tools:    checkTools(),
	}
	return result
}

// checkManagers checks which package managers are available on the system.
func checkManagers() []model.ManagerInfo {
	managers := appPlatform.SupportedManagers()
	result := make([]model.ManagerInfo, 0, len(managers))

	for _, name := range managers {
		info := model.ManagerInfo{
			Name:  name,
			Found: false,
		}

		stdout, _, exitCode, err := appRunner.Run(name, "--version")
		if err == nil && exitCode == 0 {
			info.Found = true
			info.Version = parseManagerVersion(stdout)
		}

		// Try to get the path
		if path, err := appRunner.LookPath(name); err == nil {
			info.Path = path
		}

		result = append(result, info)
	}

	return result
}

// checkTools checks which agent CLI tools are found in PATH.
func checkTools() []model.ToolPathInfo {
	result := make([]model.ToolPathInfo, 0, len(appTools))

	for _, tool := range appTools {
		info := model.ToolPathInfo{
			Name:  tool.Name,
			Found: false,
		}

		if path, err := appRunner.LookPath(tool.BinaryName); err == nil {
			info.Found = true
			info.Path = path
		}

		result = append(result, info)
	}

	return result
}

// parseManagerVersion extracts a version string from manager --version output.
func parseManagerVersion(output string) string {
	output = trimSpace(output)
	if output == "" {
		return "unknown"
	}
	// Return first non-empty line
	lines := splitLines(output)
	for _, line := range lines {
		line = trimSpace(line)
		if line != "" {
			return line
		}
	}
	return "unknown"
}

// trimSpace removes leading and trailing whitespace.
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}


