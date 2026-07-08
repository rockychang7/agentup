package cmd

import (
	"github.com/spf13/cobra"

	"agentup/pkg/table"
)

// listCmd shows all supported tools and their installation status.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all supported tools and their installation status",
	Long: `List all supported AI coding agent CLI tools and show their
installation status, current version, install method, executable path,
and whether automatic upgrade is supported.

Example:
  agentup list`,
	Run: func(cmd *cobra.Command, args []string) {
		tools := appDetector.DetectAll()
		table.PrintList(tools)
	},
}
