package model

// ToolName represents a supported AI coding agent tool name.
type ToolName string

const (
	ToolCodex       ToolName = "codex"
	ToolClaudeCode  ToolName = "claude-code"
	ToolOpenCode    ToolName = "opencode"
	ToolAgy         ToolName = "agy"
)

// SupportedTools returns all supported tool names.
func SupportedTools() []ToolName {
	return []ToolName{
		ToolCodex,
		ToolClaudeCode,
		ToolOpenCode,
		ToolAgy,
	}
}

// IsValidToolName checks if the given name is a supported tool.
func IsValidToolName(name string) bool {
	for _, t := range SupportedTools() {
		if string(t) == name {
			return true
		}
	}
	return false
}

// InstallMethod represents how a tool was installed.
type InstallMethod string

const (
	InstallMethodNpm    InstallMethod = "npm"
	InstallMethodPnpm   InstallMethod = "pnpm"
	InstallMethodYarn   InstallMethod = "yarn"
	InstallMethodBrew   InstallMethod = "brew"
	InstallMethodWinget InstallMethod = "winget"
	InstallMethodScoop  InstallMethod = "scoop"
	InstallMethodBinary    InstallMethod = "binary"
	InstallMethodSelfUpdate InstallMethod = "self-update"
	InstallMethodUnknown   InstallMethod = "unknown"
)

// UpgradeStatus represents the result of an upgrade operation.
type UpgradeStatus string

const (
	UpgradeStatusSuccess UpgradeStatus = "success"
	UpgradeStatusFailed  UpgradeStatus = "failed"
	UpgradeStatusSkipped UpgradeStatus = "skipped"
)

// ToolInfo holds the detection result for a single tool.
type ToolInfo struct {
	Name             ToolName
	Installed        bool
	Version          string // currently installed version
	LatestVersion    string // latest available version from registry
	UpgradeAvailable bool   // true if current version is older than latest
	InstallMethod    InstallMethod
	Path             string
	UpgradeSupported bool
}

// UpgradeResult holds the result of an upgrade operation for a single tool.
type UpgradeResult struct {
	Name       ToolName
	Status     UpgradeStatus
	OldVersion string
	NewVersion string
	Message    string
}

// ManagerInfo holds information about a package manager on the system.
type ManagerInfo struct {
	Name    string
	Path    string
	Version string
	Found   bool
}

// ToolPathInfo holds path information for a tool binary.
type ToolPathInfo struct {
	Name  ToolName
	Found bool
	Path  string
}

// DoctorResult holds the complete environment diagnosis result.
type DoctorResult struct {
	OS       string
	Managers []ManagerInfo
	Tools    []ToolPathInfo
}
