package config

import (
	"testing"

	"agentup/internal/model"
)

func TestDefaultTools_ReturnsFourTools(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}
}

func TestDefaultTools_AllHaveRequiredFields(t *testing.T) {
	tools := DefaultTools()
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool Name should not be empty")
		}
		if tool.DisplayName == "" {
			t.Error("tool DisplayName should not be empty")
		}
		if tool.BinaryName == "" {
			t.Error("tool BinaryName should not be empty")
		}
		if len(tool.VersionArgs) == 0 {
			t.Error("tool VersionArgs should not be empty")
		}
		if tool.ManualUpgradeURL == "" {
			t.Error("tool ManualUpgradeURL should not be empty")
		}
	}
}

func TestDefaultTools_CodexConfig(t *testing.T) {
	tools := DefaultTools()
	var codex *ToolConfig
	for i := range tools {
		if tools[i].Name == model.ToolCodex {
			codex = &tools[i]
			break
		}
	}
	if codex == nil {
		t.Fatal("codex tool not found")
	}
	if codex.BinaryName != "codex" {
		t.Errorf("expected BinaryName 'codex', got '%s'", codex.BinaryName)
	}
	if codex.NpmPackage != "@openai/codex" {
		t.Errorf("expected NpmPackage '@openai/codex', got '%s'", codex.NpmPackage)
	}
	if codex.BrewFormula != "codex" {
		t.Errorf("expected BrewFormula 'codex', got '%s'", codex.BrewFormula)
	}
	if !codex.BrewCask {
		t.Error("expected BrewCask to be true for codex")
	}
	if codex.BinaryOnly {
		t.Error("expected BinaryOnly to be false for codex")
	}
	if !codex.UpgradeSupported {
		t.Error("expected UpgradeSupported to be true for codex")
	}
}

func TestDefaultTools_ClaudeCodeConfig(t *testing.T) {
	tools := DefaultTools()
	var claude *ToolConfig
	for i := range tools {
		if tools[i].Name == model.ToolClaudeCode {
			claude = &tools[i]
			break
		}
	}
	if claude == nil {
		t.Fatal("claude-code tool not found")
	}
	if claude.BinaryName != "claude" {
		t.Errorf("expected BinaryName 'claude', got '%s'", claude.BinaryName)
	}
	if claude.NpmPackage != "@anthropic-ai/claude-code" {
		t.Errorf("expected NpmPackage '@anthropic-ai/claude-code', got '%s'", claude.NpmPackage)
	}
	if claude.BrewFormula != "claude-code" {
		t.Errorf("expected BrewFormula 'claude-code', got '%s'", claude.BrewFormula)
	}
	if !claude.BrewCask {
		t.Error("expected BrewCask to be true for claude-code")
	}
}

func TestDefaultTools_OpenCodeConfig(t *testing.T) {
	tools := DefaultTools()
	var opencode *ToolConfig
	for i := range tools {
		if tools[i].Name == model.ToolOpenCode {
			opencode = &tools[i]
			break
		}
	}
	if opencode == nil {
		t.Fatal("opencode tool not found")
	}
	if opencode.BinaryName != "opencode" {
		t.Errorf("expected BinaryName 'opencode', got '%s'", opencode.BinaryName)
	}
	if opencode.NpmPackage != "opencode-ai" {
		t.Errorf("expected NpmPackage 'opencode-ai', got '%s'", opencode.NpmPackage)
	}
	if opencode.ScoopPackage != "opencode" {
		t.Errorf("expected ScoopPackage 'opencode', got '%s'", opencode.ScoopPackage)
	}
	if opencode.BrewCask {
		t.Error("expected BrewCask to be false for opencode")
	}
}

func TestDefaultTools_AgyConfig(t *testing.T) {
	tools := DefaultTools()
	var agy *ToolConfig
	for i := range tools {
		if tools[i].Name == model.ToolAgy {
			agy = &tools[i]
			break
		}
	}
	if agy == nil {
		t.Fatal("agy tool not found")
	}
	if agy.BinaryName != "agy" {
		t.Errorf("expected BinaryName 'agy', got '%s'", agy.BinaryName)
	}
	if !agy.BinaryOnly {
		t.Error("expected BinaryOnly to be true for agy")
	}
	if !agy.UpgradeSupported {
		t.Error("expected UpgradeSupported to be true for agy (self-update)")
	}
	if len(agy.SelfUpdateArgs) == 0 || agy.SelfUpdateArgs[0] != "update" {
		t.Errorf("expected SelfUpdateArgs ['update'], got %v", agy.SelfUpdateArgs)
	}
	if agy.NpmPackage != "" {
		t.Errorf("expected NpmPackage to be empty for agy, got '%s'", agy.NpmPackage)
	}
	if agy.BrewFormula != "" {
		t.Errorf("expected BrewFormula to be empty for agy, got '%s'", agy.BrewFormula)
	}
}

func TestFindTool_ExistingTool(t *testing.T) {
	tool := FindTool(model.ToolCodex)
	if tool == nil {
		t.Fatal("expected tool to be found")
	}
	if tool.Name != model.ToolCodex {
		t.Errorf("expected ToolCodex, got %s", tool.Name)
	}
}

func TestFindTool_NonExistingTool(t *testing.T) {
	tool := FindTool(model.ToolName("nonexistent"))
	if tool != nil {
		t.Fatal("expected nil for non-existing tool")
	}
}
