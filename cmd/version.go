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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if outputFlags.JSON {
			return writeJSON(cmd, map[string]string{"name": "agentup", "version": Version})
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "agentup version %s\n", Version)
		return err
	},
}
