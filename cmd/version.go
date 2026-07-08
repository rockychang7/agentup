package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd shows the agentup version.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show agentup version",
	Long:  `Display the current version of agentup.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("agentup version %s\n", Version)
	},
}
