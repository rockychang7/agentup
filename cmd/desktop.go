package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"agentup/internal/desktop"
	"agentup/internal/model"
	"agentup/internal/operation"
)

func selectedKind(cmd *cobra.Command) (string, error) {
	if cmd.Flags().Lookup("kind") == nil {
		return "cli", nil
	}
	kind, _ := cmd.Flags().GetString("kind")
	if kind != "cli" && kind != "desktop" && kind != "all" {
		return "", fmt.Errorf("invalid --kind %q; use cli, desktop or all", kind)
	}
	return kind, nil
}

func runDesktopList(cmd *cobra.Command, kind string) error {
	manager := desktop.New(commandContext(cmd), appRunner, appPlatform.Name(), runtime.GOARCH)
	snapshot := manager.Scan(outputFlags.Offline)
	var cli []model.ToolInfo
	if kind == "all" {
		if outputFlags.Offline {
			cli = appDetector.DetectAllLocal(nil)
		} else {
			cli = appDetector.DetectAll()
		}
	}
	if outputFlags.JSON {
		if kind == "all" {
			if err := writeJSON(cmd, struct {
				CLI     []model.ToolInfo `json:"cli"`
				Desktop desktop.Snapshot `json:"desktop"`
			}{cli, snapshot}); err != nil {
				return err
			}
		} else if err := writeJSON(cmd, snapshot); err != nil {
			return err
		}
	} else {
		p := printer(cmd)
		if kind == "all" {
			p.Heading("CLI", "")
			p.List(cli)
			p.Line("")
		}
		p.Heading("Desktop", "Antigravity app only; IDE is excluded")
		for _, info := range snapshot.Installations {
			status := "not installed"
			if info.Reason == "discovery_incomplete" {
				status = "discovery incomplete"
			} else if info.Installed {
				status = info.Version + " | " + info.Source + " | " + string(info.Capability)
				if info.UpgradeAvailable {
					status += " -> " + info.LatestVersion
				}
			}
			p.Line(info.Name + " | " + status)
			if outputFlags.Verbose && info.Installed {
				p.Line("  Instance: " + info.ID)
				p.Line("  Path: " + info.Path)
				p.Line("  Running: " + info.Running + " | " + info.Scope + " | " + info.Arch)
				p.Line("  Check: " + string(info.LatestCheck) + " | " + info.Reason)
				if info.Plan != nil {
					p.Line("  Action: " + info.Plan.Action)
				}
			}
		}
		for _, issue := range snapshot.Issues {
			p.Notice(issue)
		}
	}
	if err := commandContext(cmd).Err(); err != nil {
		return err
	}
	if len(snapshot.Issues) != 0 {
		return reportedError{"desktop discovery was incomplete"}
	}
	for _, info := range cli {
		if info.LatestCheck == model.CheckFailed {
			return reportedError{"one or more latest-version checks failed"}
		}
	}
	return nil
}

func runDesktopUpgrade(cmd *cobra.Command, args []string) error {
	if outputFlags.Offline {
		return fmt.Errorf("--offline is read-only; desktop upgrades are disabled")
	}
	kind, err := selectedKind(cmd)
	if err != nil {
		return err
	}
	if kind != "desktop" {
		return fmt.Errorf("use --kind desktop to upgrade desktops; mixed CLI/Desktop upgrades are not supported")
	}
	yes, _ := cmd.Flags().GetBool("yes")
	instance, _ := cmd.Flags().GetString("instance")
	target := ""
	if len(args) != 0 {
		product, ok := desktop.FindProduct(args[0])
		if !ok {
			return fmt.Errorf("unsupported desktop %q; Antigravity IDE is not managed", args[0])
		}
		target = product.ID
	}
	if instance != "" && target == "" {
		return fmt.Errorf("--instance requires a desktop product name")
	}
	manager := desktop.New(commandContext(cmd), appRunner, appPlatform.Name(), runtime.GOARCH)
	snapshot := manager.Scan(false)
	selected := []desktop.Installation{}
	for _, info := range snapshot.Installations {
		if target != "" && info.ProductID != target {
			continue
		}
		if instance != "" && info.ID != instance {
			continue
		}
		selected = append(selected, info)
	}
	if len(selected) == 0 {
		return fmt.Errorf("desktop installation not found; run agentup list --kind desktop --verbose")
	}
	if target != "" && instance == "" && len(selected) > 1 {
		return fmt.Errorf("multiple desktop installations found; choose --instance using agentup list --kind desktop --verbose")
	}
	if !yes {
		plans := []desktop.Plan{}
		for _, info := range selected {
			if info.Plan != nil {
				plans = append(plans, *info.Plan)
			}
		}
		if outputFlags.JSON {
			if err := writeJSON(cmd, struct {
				ReviewOnly    bool                   `json:"review_only"`
				Installations []desktop.Installation `json:"installations"`
				Plans         []desktop.Plan         `json:"plans"`
				Issues        []string               `json:"issues"`
			}{true, selected, plans, snapshot.Issues}); err != nil {
				return err
			}
			if len(snapshot.Issues) != 0 {
				return reportedError{"desktop discovery was incomplete"}
			}
			return commandContext(cmd).Err()
		}
		p := printer(cmd)
		p.Heading("Desktop upgrade preview", "Review only; rerun with --yes to apply")
		for _, info := range selected {
			p.Line(info.Name + " | " + info.Version + " | " + string(info.Capability) + " | " + info.Reason)
			if info.Plan != nil {
				switch info.Plan.Action {
				case desktop.ActionCommand:
					p.Line("  Fully quit the app. Command: " + strings.Join(info.Plan.Command, " "))
					p.Line("  The package manager selects its available version at execution time.")
					if info.Plan.Standalone {
						p.Line("  Confirm this app on its own; batch upgrades skip it.")
					}
				case desktop.ActionPrivileged:
					p.Line("  Fully quit the app. Runs in this terminal and asks for your password: " + strings.Join(info.Plan.Command, " "))
				case desktop.ActionReplace:
					p.Line("  Fully quit the app. Downloads " + info.Plan.Asset.URL)
					p.Line("  Checks size and SHA-512, then replaces " + info.Plan.Path + " in place.")
				case desktop.ActionPackage:
					p.Line("  Fully quit the app. Downloads " + info.Plan.Asset.URL)
					p.Line("  Checks size and SHA-512, then installs it with: " + strings.Join(info.Plan.Command, " ") + " <package>")
				case desktop.ActionInstaller:
					p.Line("  Fully quit the app. Downloads and verifies " + info.Plan.Asset.URL)
					p.Line("  Checks size, SHA-512 and the publisher signature (" + info.Plan.Asset.Publisher + "), then opens the installer for you to complete.")
				default:
					p.Line("  Official update entry: " + info.Plan.URL)
				}
			}
		}
		for _, issue := range snapshot.Issues {
			p.Notice(issue)
		}
		if len(snapshot.Issues) != 0 {
			return reportedError{"desktop discovery was incomplete"}
		}
		return nil
	}
	release, err := operation.Acquire("mutation")
	if err != nil {
		return fmt.Errorf("cannot start desktop upgrade: %w", err)
	}
	defer release()
	results := []desktop.Result{}
	manager.EnableTerminal()
	for _, info := range selected {
		batchSkip := target == "" && info.Plan != nil && (info.Capability != desktop.Automatic || info.Plan.Standalone)
		if info.Plan == nil || batchSkip {
			outcome, message := "skipped", info.Reason
			if info.Reason == "discovery_incomplete" {
				outcome, message = "unverified", strings.Join(snapshot.Issues, "; ")
			} else if batchSkip {
				message = "Batch upgrades skip this app; upgrade it by name."
			}
			results = append(results, desktop.Result{
				Installation: info.ID, ProductID: info.ProductID, Name: info.Name,
				Outcome: outcome, OldVersion: info.Version,
				Message: message, URL: info.UpdateURL,
			})
			continue
		}
		result := manager.Execute(*info.Plan, func(stream, text string) {
			if outputFlags.JSON {
				return
			}
			if stream == "phase" {
				text = "[" + info.Name + "] " + text
			}
			notices(cmd).Line(strings.TrimSpace(text))
		})
		results = append(results, result)
	}
	if outputFlags.JSON {
		if err := writeJSON(cmd, results); err != nil {
			return err
		}
	} else {
		p := printer(cmd)
		p.Heading("Desktop upgrade results", "")
		for _, result := range results {
			p.Line(result.Name + " | " + result.Outcome + " | " + result.OldVersion + " -> " + result.NewVersion)
			if result.Message != "" {
				p.Line("  " + result.Message)
			}
			if result.URL != "" {
				p.Line("  " + result.URL)
			}
		}
	}
	for _, result := range results {
		if result.Outcome == "failed" || result.Outcome == "unverified" {
			return reportedError{"one or more desktop upgrades failed or could not be verified"}
		}
	}
	return commandContext(cmd).Err()
}

func init() {
	rootCmd.Flags().String("kind", "cli", "list cli, desktop or all")
	listCmd.Flags().String("kind", "cli", "list cli, desktop or all")
	upgradeCmd.Flags().String("kind", "cli", "upgrade cli or desktop (default: cli)")
	upgradeCmd.Flags().Bool("yes", false, "confirm the generated desktop update plan")
	upgradeCmd.Flags().String("instance", "", "desktop installation ID when multiple copies exist")
}
