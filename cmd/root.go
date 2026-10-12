package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/selfupdate"
	"agentup/internal/upgrader"
	"agentup/pkg/table"
)

// Version is the current version of agentup.
// This value is injected at build time via ldflags:
//
//	-ldflags "-X agentup/cmd.Version=0.2.0"
var Version = "dev"

// Global instances shared across commands.
var (
	appRunner      runner.Runner
	appPlatform    platform.Platform
	appDetector    *detector.Detector
	appUpgrader    *upgrader.Upgrader
	appSelfUpdater selfUpdater
	appTools       []config.ToolConfig
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:           "agentup",
	Short:         "Unified CLI tool for detecting and upgrading AI coding agent CLIs",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          runList,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if outputFlags.Color != "auto" && outputFlags.Color != "always" && outputFlags.Color != "never" {
			return fmt.Errorf("invalid --color value %q; use auto, always or never", outputFlags.Color)
		}
		isUI := cmd.Name() == "ui" || (cmd.Parent() != nil && cmd.Parent().Name() == "ui")
		if outputFlags.Offline && !isUI && cmd.Name() != "agentup" && cmd.Name() != "list" &&
			cmd.Name() != "doctor" && cmd.Name() != "version" && cmd.Name() != "ui" {
			return fmt.Errorf("--offline is read-only; use agentup list --offline")
		}
		if outputFlags.JSON && (cmd.Name() == "uninstall" || cmd.Name() == "ui") {
			return fmt.Errorf("--json is not supported by %s", cmd.Name())
		}
		initGlobals(commandContext(cmd))
		return nil
	},
	Long: `agentup is a cross-platform CLI tool for detecting and upgrading
locally installed AI coding agent CLI tools.

Run agentup without a command for a read-only status overview.
Supports Codex, Claude Code, OpenCode and Agy on macOS, Linux and Windows.`,
	Example: "  agentup\n  agentup list --offline\n  agentup upgrade\n  agentup doctor --verbose",
}

// Execute runs the root command.
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	rootCmd.SetContext(ctx)
	restoreOut, _ := termenv.EnableVirtualTerminalProcessing(termenv.NewOutput(os.Stdout))
	restoreErr, _ := termenv.EnableVirtualTerminalProcessing(termenv.NewOutput(os.Stderr))
	restore := func() {
		if restoreOut != nil {
			_ = restoreOut()
		}
		if restoreErr != nil {
			_ = restoreErr()
		}
	}
	if err := rootCmd.Execute(); err != nil {
		if _, printed := err.(reportedError); !printed {
			table.New(rootCmd.ErrOrStderr(), presentationOptions()).Error(err.Error())
		}
		restore()
		os.Exit(1)
	}
	restore()
}

// initGlobals initializes the shared services used by commands.
func initGlobals(ctx context.Context) {
	appRunner = &runner.DefaultRunner{Context: ctx}
	appPlatform = platform.NewPlatform()
	appTools = config.DefaultTools()
	// appDetector checks latest versions (used by list command)
	appDetector = detector.New(appRunner, appPlatform, appTools)
	// upgradeDetector skips latest version check to avoid network overhead during upgrades
	upgradeDetector := detector.NewWithoutLatestCheck(appRunner, appPlatform, appTools)
	appUpgrader = upgrader.New(appRunner, appPlatform, upgradeDetector, appTools)
	appSelfUpdater = selfupdate.New(Version)
}

// init registers all subcommands with the root command.
func init() {
	rootCmd.PersistentFlags().StringVar(&outputFlags.Color, "color", "auto", "colors: auto, always or never")
	rootCmd.PersistentFlags().BoolVarP(&outputFlags.Verbose, "verbose", "v", false, "show paths, check details and command logs")
	rootCmd.PersistentFlags().BoolVar(&outputFlags.JSON, "json", false, "write structured JSON without progress output")
	rootCmd.PersistentFlags().BoolVar(&outputFlags.Offline, "offline", false, "skip registry checks (read-only commands)")
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		var text bytes.Buffer
		out := cmd.OutOrStdout()
		cmd.SetOut(&text)
		defaultHelp(cmd, args)
		cmd.SetOut(out)
		p := table.New(out, presentationOptions())
		for _, line := range strings.Split(strings.TrimSuffix(text.String(), "\n"), "\n") {
			if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") {
				p.Heading(line, "")
			} else {
				p.Line(line)
			}
		}
	})
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(uninstallCmd)
	rootCmd.AddCommand(uiCmd)
}
