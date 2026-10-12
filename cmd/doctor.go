package cmd

import (
	"github.com/spf13/cobra"

	"agentup/internal/diagnostic"
	"agentup/internal/model"
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
	return diagnostic.Run(appRunner, appPlatform, appDetector)
}
