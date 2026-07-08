package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"agentup/internal/model"
	"agentup/pkg/table"
)

// upgradeCmd upgrades one or all installed tools.
var upgradeCmd = &cobra.Command{
	Use:   "upgrade [tool]",
	Short: "Upgrade one or all installed tools",
	Long: `Upgrade installed AI coding agent CLI tools.

If no tool name is specified, all installed tools are upgraded.
If a tool name is specified, only that tool is upgraded.

Supported tool names: codex, claude-code, opencode, agy

Examples:
  agentup upgrade              # Upgrade all installed tools
  agentup upgrade codex        # Upgrade only codex
  agentup upgrade claude-code  # Upgrade only claude-code`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			// Upgrade all installed tools
			fmt.Println("Upgrading installed agents...")
			results := appUpgrader.UpgradeAll()
			table.PrintUpgradeResults(results)
			fmt.Println("\nDone.")
			return
		}

		// Upgrade a specific tool
		toolName := args[0]

		// Validate tool name
		if !model.IsValidToolName(toolName) {
			fmt.Printf("Error: '%s' is not a supported tool.\n\n", toolName)
			fmt.Println("Supported tools:")
			for _, t := range model.SupportedTools() {
				fmt.Printf("  - %s\n", t)
			}
			return
		}

		result, err := appUpgrader.Upgrade(model.ToolName(toolName))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		// Print single result
		table.PrintUpgradeResults([]model.UpgradeResult{result})
		fmt.Println("\nDone.")

		// Print additional hint for skipped tools
		if result.Status == model.UpgradeStatusSkipped {
			if strings.Contains(result.Message, "not installed") {
				// Already covered by the message
			} else if strings.Contains(result.Message, "manual") {
				fmt.Printf("\nHint: %s\n", result.Message)
			}
		}
	},
}
