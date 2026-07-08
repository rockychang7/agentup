package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/upgrader"
)

// Version is the current version of agentup.
// This value is injected at build time via ldflags:
//   -ldflags "-X agentup/cmd.Version=v0.1.0"
var Version = "dev"

// Global instances shared across commands.
var (
	appRunner   runner.Runner
	appPlatform platform.Platform
	appDetector *detector.Detector
	appUpgrader *upgrader.Upgrader
	appTools    []config.ToolConfig
)

// rootCmd is the base command.
var rootCmd = &cobra.Command{
	Use:   "agentup",
	Short: "Unified CLI tool for detecting and upgrading AI coding agent CLIs",
	Long: `agentup is a cross-platform CLI tool for detecting and upgrading
locally installed AI coding agent CLI tools.

Supported tools:
  - Codex CLI
  - Claude Code CLI
  - OpenCode CLI
  - Agy CLI

Supported platforms:
  - macOS
  - Windows

Usage:
  agentup [command]

Available Commands:
  list      List all supported tools and their installation status
  upgrade   Upgrade one or all installed tools
  doctor    Run environment diagnostics
  version   Show agentup version

Use "agentup [command] --help" for more information about a command.`,
}

// Execute runs the root command.
func Execute() {
	initGlobals()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// initGlobals initializes the shared runner, platform, detector, and upgrader.
func initGlobals() {
	appRunner = &runner.DefaultRunner{}
	appPlatform = platform.NewPlatform()
	appTools = config.DefaultTools()
	// appDetector checks latest versions (used by list command)
	appDetector = detector.New(appRunner, appPlatform, appTools)
	// upgradeDetector skips latest version check to avoid network overhead during upgrades
	upgradeDetector := detector.NewWithoutLatestCheck(appRunner, appPlatform, appTools)
	appUpgrader = upgrader.New(appRunner, appPlatform, upgradeDetector, appTools, os.Stdout)
}

// init registers all subcommands with the root command.
func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
}
