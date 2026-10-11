package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"agentup/internal/model"
	"agentup/internal/runner"
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
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !outputFlags.JSON {
			notices(cmd).Notice("Checking environment...")
		}
		result := runDoctor()
		if outputFlags.JSON {
			if err := writeJSON(cmd, result); err != nil {
				return err
			}
		} else {
			printer(cmd).Doctor(result)
		}
		return commandContext(cmd).Err()
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

		path, pathErr := appRunner.LookPath(name)
		if pathErr != nil {
			result = append(result, info)
			continue
		}
		info.Path = path
		stdout, stderr, exitCode, err := runner.WithTimeout(appRunner, 10*time.Second).Run(name, "--version")
		if err == nil && exitCode == 0 {
			info.Found = true
			info.Version = parseManagerVersion(stdout)
		} else if err != nil {
			info.Error = err.Error()
		} else {
			info.Error = fmt.Sprintf("exit code %d: %s", exitCode, strings.TrimSpace(stderr))
		}

		result = append(result, info)
	}

	return result
}

// checkTools checks which agent CLI tools are found in PATH.
func checkTools() []model.ToolPathInfo {
	result := make([]model.ToolPathInfo, 0, len(appTools))
	for _, tool := range appDetector.DetectAllLocal(nil) {
		info := model.ToolPathInfo{
			Name:  tool.Name,
			Found: tool.Installed,
			Path:  tool.Path,
		}
		if tool.Installed && tool.Version == "unknown" {
			info.Issue = "version could not be read; check the executable and PATH"
		} else if tool.Installed && !tool.UpgradeSupported {
			info.Issue = "automatic upgrade is unavailable for this install source; upgrade manually"
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
