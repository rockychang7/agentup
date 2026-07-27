package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"agentup/internal/selfupdate"
)

type selfUpdater interface {
	Update(context.Context, bool) (selfupdate.Result, error)
}

var forceSelfUpdate bool

// updateCmd upgrades agentup itself from the latest GitHub release.
var updateCmd = &cobra.Command{
	Use:     "update",
	Aliases: []string{"self-update"},
	Short:   "Update agentup to the latest release",
	Long: `Download and install the latest published agentup release.

The release archive is selected for the current operating system and
architecture, then verified against the published SHA-256 checksums.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSelfUpdate(cmd, forceSelfUpdate)
	},
}

func runSelfUpdate(cmd *cobra.Command, force bool) error {
	output := cmd.OutOrStdout()
	fmt.Fprintf(output, "Checking for agentup updates (current: %s)...\n", Version)

	result, err := appSelfUpdater.Update(cmd.Context(), force)
	if err != nil {
		return fmt.Errorf("update agentup: %w", err)
	}

	if !result.Updated {
		fmt.Fprintf(
			output,
			"agentup %s: %s (latest: %s).\n",
			result.CurrentVersion,
			result.SkippedReason,
			result.LatestVersion,
		)
		return nil
	}

	if result.PendingRestart {
		fmt.Fprintf(
			output,
			"Downloaded agentup %s. Installation will complete after this command exits.\n",
			result.LatestVersion,
		)
		return nil
	}

	fmt.Fprintf(
		output,
		"Updated agentup from %s to %s.\n",
		result.CurrentVersion,
		result.LatestVersion,
	)
	return nil
}

func init() {
	updateCmd.Flags().BoolVar(
		&forceSelfUpdate,
		"force",
		false,
		"install the latest release even when the current version is equal or newer",
	)
}
