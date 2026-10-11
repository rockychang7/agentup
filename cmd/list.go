package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"agentup/internal/model"
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
	Args: cobra.NoArgs,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	p := printer(cmd)
	names := make([]model.ToolName, len(appTools))
	for i, tool := range appTools {
		names[i] = tool.Name
	}
	var scan *table.Scan
	if !outputFlags.JSON {
		if p.Interactive {
			scan = table.NewScan(p, names)
			defer scan.Close()
		} else {
			notices(cmd).Notice("Detecting local agents...")
		}
	}
	onLocal := func(info model.ToolInfo) {
		if !outputFlags.Offline && info.Installed {
			info.LatestCheck = model.CheckPending
		}
		if scan != nil {
			scan.Update(info)
		}
	}
	tools := appDetector.DetectAllLocal(onLocal)
	if !outputFlags.Offline {
		if scan != nil {
			scan.SetPhase("Checking latest versions...")
		} else if !outputFlags.JSON {
			notices(cmd).Notice("Local detection complete. Checking latest versions...")
		}
		tools = appDetector.CheckAllLatest(tools, func(info model.ToolInfo) {
			if scan != nil {
				scan.Update(info)
			}
		})
	}
	if scan != nil {
		scan.Close()
	}
	if outputFlags.JSON {
		if err := writeJSON(cmd, tools); err != nil {
			return err
		}
	} else {
		p.Heading("AgentUp", fmt.Sprintf("%s | Agent status", Version))
		p.List(tools)
	}
	if err := commandContext(cmd).Err(); err != nil {
		return err
	}
	for _, info := range tools {
		if info.LatestCheck == model.CheckFailed {
			return reportedError{"one or more latest-version checks failed"}
		}
	}
	return nil
}
