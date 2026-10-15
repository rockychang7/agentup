package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"agentup/internal/model"
	"agentup/internal/operation"
	"agentup/pkg/table"
)

// upgradeCmd upgrades one or all installed tools.
var upgradeCmd = &cobra.Command{
	Use:       "upgrade [tool]",
	Short:     "Upgrade one or all installed tools",
	ValidArgs: []string{"codex", "claude-code", "claude", "opencode", "agy", "agentup"},
	Long: `Upgrade installed AI coding agent CLI tools.

If no tool name is specified, all installed tools are upgraded.
If a tool name is specified, only that tool is upgraded.

Supported tool names: codex, claude-code (or claude), opencode, agy, agentup

Examples:
  agentup upgrade              # Upgrade all installed tools
  agentup upgrade codex        # Upgrade only codex
  agentup upgrade claude-code  # Upgrade only claude-code
  agentup upgrade agentup      # Upgrade agentup itself
  agentup upgrade codex --kind desktop       # Preview Desktop plan
  agentup upgrade codex --kind desktop --yes # Confirm Desktop plan`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		kind, err := selectedKind(cmd)
		if err != nil {
			return err
		}
		if kind != "cli" {
			return runDesktopUpgrade(cmd, args)
		}
		for _, flag := range []string{"yes", "instance"} {
			if cmd.Flags().Changed(flag) {
				return fmt.Errorf("--%s is only supported with --kind desktop", flag)
			}
		}
		var target model.ToolName
		if len(args) > 0 {
			toolName := args[0]
			if toolName == "agentup" {
				return runSelfUpdate(cmd, false)
			}
			if toolName == "claude" {
				toolName = "claude-code"
			}
			if !model.IsValidToolName(toolName) {
				return fmt.Errorf("unsupported tool %q; choose codex, claude-code (claude), opencode, agy or agentup", toolName)
			}
			target = model.ToolName(toolName)
		}
		release, err := operation.Acquire("mutation")
		if err != nil {
			if errors.Is(err, operation.ErrBusy) {
				retry := "agentup upgrade"
				if target != "" {
					retry += " " + string(target)
				}
				return actionableError{
					message: "Another AgentUp operation is running.",
					hint:    "Wait for it to finish, then retry:\n  " + retry,
					cause:   err,
				}
			}
			return fmt.Errorf("cannot start upgrade: %w", err)
		}
		defer release()
		names := []model.ToolName{target}
		if target == "" {
			names = make([]model.ToolName, len(appTools))
			for i, tool := range appTools {
				names[i] = tool.Name
			}
		}
		appUpgrader.SetObserver(nil)
		var progress *table.Progress
		if !outputFlags.JSON {
			p := printer(cmd)
			if !p.Interactive {
				p = notices(cmd)
			}
			progress = table.NewProgress(p, names)
			appUpgrader.SetObserver(progress.Observe)
			defer progress.Close()
			defer appUpgrader.SetObserver(nil)
		}
		var results []model.UpgradeResult
		if target == "" {
			results = appUpgrader.UpgradeAll()
		} else {
			result, err := appUpgrader.Upgrade(target)
			if err != nil {
				return err
			}
			results = []model.UpgradeResult{result}
		}
		if progress != nil {
			progress.Close()
		}
		if outputFlags.JSON {
			if err := writeJSON(cmd, results); err != nil {
				return err
			}
		} else {
			printer(cmd).UpgradeResults(results)
		}
		for _, result := range results {
			if result.Status == model.UpgradeStatusFailed || result.Outcome == model.OutcomeUnverified {
				return reportedError{"one or more upgrades failed or could not be verified"}
			}
		}
		if err := commandContext(cmd).Err(); err != nil {
			return err
		}
		return nil
	},
}
