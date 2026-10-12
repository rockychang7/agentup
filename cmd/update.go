package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"agentup/internal/model"
	"agentup/internal/operation"
	"agentup/internal/selfupdate"
	"agentup/pkg/table"
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
	releaseUI, err := operation.Acquire("ui")
	if err != nil {
		return fmt.Errorf("close agentup ui before updating AgentUp: %w", err)
	}
	defer releaseUI()
	release, err := operation.Acquire("mutation")
	if err != nil {
		return fmt.Errorf("cannot update AgentUp: %w", err)
	}
	defer release()
	p := printer(cmd)
	var progress *table.Progress
	started := time.Now()
	if !outputFlags.JSON {
		progressPrinter := p
		if !p.Interactive {
			progressPrinter = notices(cmd)
		}
		progress = table.NewProgress(progressPrinter, []model.ToolName{"agentup"})
		progress.Observe(model.UpgradeEvent{Name: "agentup", Phase: model.PhaseUpgrading, Source: "GitHub"})
		defer progress.Close()
	}

	result, err := appSelfUpdater.Update(commandContext(cmd), force)
	if progress != nil {
		progress.Close()
	}
	if err != nil {
		if outputFlags.JSON {
			if writeErr := writeJSON(cmd, map[string]any{"name": "agentup", "status": "failed", "error": err.Error()}); writeErr != nil {
				return writeErr
			}
			return reportedError{err.Error()}
		}
		return fmt.Errorf("update agentup: %w", err)
	}
	if outputFlags.JSON {
		return writeJSON(cmd, struct {
			selfupdate.Result
			Name           string `json:"name"`
			DurationMillis int64  `json:"duration_ms"`
		}{result, "agentup", time.Since(started).Milliseconds()})
	}
	p.Heading("AgentUp update", fmt.Sprintf("%s elapsed", table.Duration(time.Since(started).Milliseconds())))

	if !result.Updated {
		p.Line(fmt.Sprintf(
			"agentup %s: %s (latest: %s).",
			result.CurrentVersion,
			result.SkippedReason,
			result.LatestVersion,
		))
		return nil
	}

	if result.PendingRestart {
		p.Line(fmt.Sprintf(
			"Downloaded agentup %s. Installation will complete after this command exits.",
			result.LatestVersion,
		))
		return nil
	}

	p.Line(fmt.Sprintf(
		"Updated agentup from %s to %s.",
		result.CurrentVersion,
		result.LatestVersion,
	))
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
