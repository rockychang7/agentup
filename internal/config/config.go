package config

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"agentup/internal/model"
)

// ToolConfig holds the configuration for a single AI coding agent tool.
// All package names, formula names, and identifiers are centralized here
// for easy maintenance.
type ToolConfig struct {
	Name             model.ToolName
	DisplayName      string
	BinaryName       string // executable name: "codex", "claude", "opencode", "agy"
	BinaryAliases    []string
	VersionArgs      []string
	NpmPackage       string   // npm global package name
	BrewFormula      string   // brew formula or cask name
	BrewCask         bool     // whether it's a cask
	ScoopPackage     string   // scoop package name
	WingetID         string   // winget package ID
	BinaryOnly       bool     // only supports binary installation (agy)
	SelfUpdateArgs   []string // args for self-update command (e.g., agy update)
	ManualUpgradeURL string   // URL for manual upgrade instructions
	UpgradeSupported bool
}

// ForVersion selects distribution names without treating a major migration as an update.
func ForVersion(tool ToolConfig, version string) (ToolConfig, error) {
	if tool.Name != model.ToolOpenCode {
		return tool, nil
	}
	version = "v" + strings.TrimPrefix(version, "v")
	if !semver.IsValid(version) {
		return tool, fmt.Errorf("cannot determine OpenCode release line; check the executable and PATH")
	}
	if semver.Prerelease(version) != "" {
		return tool, fmt.Errorf("OpenCode prerelease channels require manual upgrades")
	}
	switch semver.Major(version) {
	case "v0", "v1":
		return tool, nil
	case "v2":
		tool.NpmPackage = "@opencode/cli"
		tool.BrewFormula = "anomalyco/tap/opencode-v2"
		tool.ScoopPackage = ""
		tool.WingetID = ""
		tool.ManualUpgradeURL = "https://opencode.ai/v2/docs/"
		return tool, nil
	default:
		return tool, fmt.Errorf("unsupported OpenCode release line %s; upgrade manually", semver.Major(version))
	}
}

// ValidateUpgradeVersion keeps OpenCode updates on the installed stable release line.
func ValidateUpgradeVersion(tool ToolConfig, current, next string) error {
	if tool.Name != model.ToolOpenCode {
		return nil
	}
	if _, err := ForVersion(tool, current); err != nil {
		return err
	}
	if _, err := ForVersion(tool, next); err != nil {
		return err
	}
	currentMajor := semver.Major("v" + strings.TrimPrefix(current, "v"))
	nextMajor := semver.Major("v" + strings.TrimPrefix(next, "v"))
	if currentMajor != nextMajor {
		return fmt.Errorf("OpenCode release line changed from %s to %s; migrate major versions manually", currentMajor, nextMajor)
	}
	return nil
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
			BinaryAliases:    []string{"opencode2"},
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
