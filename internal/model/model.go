package model

// ToolName represents a supported AI coding agent tool name.
type ToolName string

const (
	ToolCodex      ToolName = "codex"
	ToolClaudeCode ToolName = "claude-code"
	ToolOpenCode   ToolName = "opencode"
	ToolAgy        ToolName = "agy"
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
	InstallMethodNpm        InstallMethod = "npm"
	InstallMethodPnpm       InstallMethod = "pnpm"
	InstallMethodYarn       InstallMethod = "yarn"
	InstallMethodBrew       InstallMethod = "brew"
	InstallMethodWinget     InstallMethod = "winget"
	InstallMethodScoop      InstallMethod = "scoop"
	InstallMethodBinary     InstallMethod = "binary"
	InstallMethodSelfUpdate InstallMethod = "self-update"
	InstallMethodUnknown    InstallMethod = "unknown"
)

// UpgradeStatus represents the result of an upgrade operation.
type UpgradeStatus string

const (
	UpgradeStatusSuccess    UpgradeStatus = "success"
	UpgradeStatusFailed     UpgradeStatus = "failed"
	UpgradeStatusSkipped    UpgradeStatus = "skipped"
	UpgradeStatusUnverified UpgradeStatus = "unverified"
)

type UpgradeOutcome string

const (
	OutcomeUpdated    UpgradeOutcome = "updated"
	OutcomeUnchanged  UpgradeOutcome = "unchanged"
	OutcomeUnverified UpgradeOutcome = "unverified"
	OutcomeFailed     UpgradeOutcome = "failed"
	OutcomeSkipped    UpgradeOutcome = "skipped"
)

type CheckStatus string

const (
	CheckPending     CheckStatus = "pending"
	CheckOK          CheckStatus = "checked"
	CheckOffline     CheckStatus = "offline"
	CheckUnavailable CheckStatus = "unavailable"
	CheckFailed      CheckStatus = "failed"
)

// ToolInfo holds the detection result for a single tool.
type ToolInfo struct {
	Name             ToolName      `json:"name"`
	Installed        bool          `json:"installed"`
	Version          string        `json:"version"`
	LatestVersion    string        `json:"latest_version"`
	UpgradeAvailable bool          `json:"upgrade_available"`
	InstallMethod    InstallMethod `json:"install_method"`
	Path             string        `json:"path"`
	UpgradeSupported bool          `json:"upgrade_supported"`
	LatestCheck      CheckStatus   `json:"latest_check"`
	CheckMessage     string        `json:"check_message,omitempty"`
}

// UpgradeResult holds the result of an upgrade operation for a single tool.
type UpgradeResult struct {
	Name           ToolName       `json:"name"`
	Status         UpgradeStatus  `json:"status"`
	Outcome        UpgradeOutcome `json:"outcome"`
	OldVersion     string         `json:"old_version"`
	NewVersion     string         `json:"new_version"`
	Message        string         `json:"message,omitempty"`
	DurationMillis int64          `json:"duration_ms"`
	Command        []string       `json:"command,omitempty"`
	Log            string         `json:"log,omitempty"`
}

type UpgradePhase string

const (
	PhaseDetecting UpgradePhase = "detecting"
	PhaseWaiting   UpgradePhase = "waiting"
	PhaseUpgrading UpgradePhase = "upgrading"
	PhaseVerifying UpgradePhase = "verifying"
	PhaseFinished  UpgradePhase = "finished"
)

// UpgradeEvent is a presentation-neutral task update. Observers may be called concurrently.
type UpgradeEvent struct {
	Name   ToolName
	Phase  UpgradePhase
	Source InstallMethod
	Result *UpgradeResult
	Log    string
	Stream string
}

// ManagerInfo holds information about a package manager on the system.
type ManagerInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
	Found   bool   `json:"found"`
	Error   string `json:"error,omitempty"`
}

// ToolPathInfo holds path information for a tool binary.
type ToolPathInfo struct {
	Name      ToolName `json:"name"`
	Found     bool     `json:"found"`
	Path      string   `json:"path"`
	Issue     string   `json:"issue,omitempty"`
	IssueCode string   `json:"issue_code,omitempty"`
}

// DoctorResult holds the complete environment diagnosis result.
type DoctorResult struct {
	OS       string         `json:"os"`
	Managers []ManagerInfo  `json:"managers"`
	Tools    []ToolPathInfo `json:"tools"`
}
