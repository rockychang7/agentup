package config

import "agentup/internal/model"

// ToolConfig holds the configuration for a single AI coding agent tool.
// All package names, formula names, and identifiers are centralized here
// for easy maintenance.
type ToolConfig struct {
	Name             model.ToolName
	DisplayName      string
	BinaryName       string // executable name: "codex", "claude", "opencode", "agy"
	VersionArgs      []string
	NpmPackage       string // npm global package name
	BrewFormula      string // brew formula or cask name
	BrewCask         bool   // whether it's a cask
	ScoopPackage     string // scoop package name
	WingetID         string // winget package ID
	BinaryOnly       bool     // only supports binary installation (agy)
	SelfUpdateArgs   []string // args for self-update command (e.g., agy update)
	ManualUpgradeURL string   // URL for manual upgrade instructions
	UpgradeSupported bool
}

// DefaultTools returns the configuration for all four supported tools.
func DefaultTools() []ToolConfig {
	return []ToolConfig{
		{
			Name:             model.ToolCodex,
			DisplayName:      "Codex CLI",
			BinaryName:       "codex",
			VersionArgs:      []string{"--version"},
			NpmPackage:       "@openai/codex",
			BrewFormula:      "codex",
			BrewCask:         true,
			ScoopPackage:     "",
			WingetID:         "",
			BinaryOnly:       false,
			ManualUpgradeURL: "https://github.com/openai/codex/releases",
			UpgradeSupported: true,
		},
		{
			Name:             model.ToolClaudeCode,
			DisplayName:      "Claude Code CLI",
			BinaryName:       "claude",
			VersionArgs:      []string{"--version"},
			NpmPackage:       "@anthropic-ai/claude-code",
			BrewFormula:      "claude-code",
			BrewCask:         true,
			ScoopPackage:     "",
			WingetID:         "",
			BinaryOnly:       false,
			ManualUpgradeURL: "https://docs.anthropic.com/en/docs/claude-code/overview",
			UpgradeSupported: true,
		},
		{
			Name:             model.ToolOpenCode,
			DisplayName:      "OpenCode CLI",
			BinaryName:       "opencode",
			VersionArgs:      []string{"--version"},
			NpmPackage:       "opencode-ai",
			BrewFormula:      "opencode",
			BrewCask:         false,
			ScoopPackage:     "opencode",
			WingetID:         "",
			BinaryOnly:       false,
			ManualUpgradeURL: "https://opencode.ai/docs/",
			UpgradeSupported: true,
		},
		{
			Name:             model.ToolAgy,
			DisplayName:      "Agy CLI",
			BinaryName:       "agy",
			VersionArgs:      []string{"--version"},
			NpmPackage:       "",
			BrewFormula:      "",
			BrewCask:         false,
			ScoopPackage:     "",
			WingetID:         "",
			BinaryOnly:       true,
			SelfUpdateArgs:   []string{"update"},
			ManualUpgradeURL: "https://antigravity.google/docs/cli-overview",
			UpgradeSupported: true,
		},
	}
}

// FindTool returns the config for the given tool name, or nil if not found.
func FindTool(name model.ToolName) *ToolConfig {
	for _, t := range DefaultTools() {
		if t.Name == name {
			return &t
		}
	}
	return nil
}
