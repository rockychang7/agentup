package upgrader

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"agentup/internal/config"
	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/platform"
	"agentup/internal/runner"
)

// Upgrader handles upgrading AI coding agent CLI tools
// based on their detected installation method.
type Upgrader struct {
	runner   runner.Runner
	platform platform.Platform
	detector *detector.Detector
	tools    []config.ToolConfig
	observer func(model.UpgradeEvent)
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
}

const maxConcurrentUpgrades = 4

// New creates an upgrader without coupling business logic to terminal output.
func New(r runner.Runner, p platform.Platform, det *detector.Detector, tools []config.ToolConfig) *Upgrader {
	return &Upgrader{
		runner:   runner.WithTimeout(r, 10*time.Minute),
		platform: p,
		detector: det,
		tools:    tools,
		locks:    make(map[string]*sync.Mutex),
	}
}

// SetObserver must be called before starting an upgrade.
func (u *Upgrader) SetObserver(observer func(model.UpgradeEvent)) {
	u.observer = observer
}

func (u *Upgrader) emit(event model.UpgradeEvent) {
	if u.observer != nil {
		u.observer(event)
	}
}

func (u *Upgrader) managerLock(info model.ToolInfo) *sync.Mutex {
	key := string(info.InstallMethod)
	if info.InstallMethod == model.InstallMethodSelfUpdate {
		key += ":" + info.Path
	}
	u.locksMu.Lock()
	defer u.locksMu.Unlock()
	if u.locks[key] == nil {
		u.locks[key] = &sync.Mutex{}
	}
	return u.locks[key]
}

// UpgradeAll upgrades all installed tools.
// Tools that are not installed are skipped.
// If one tool fails to upgrade, it does not affect the others.
// Events describe progress while returned results preserve configured order.
func (u *Upgrader) UpgradeAll() []model.UpgradeResult {
	results := make([]model.UpgradeResult, len(u.tools))
	total := len(u.tools)
	if total == 0 {
		return results
	}
	type upgradeJob struct {
		index int
		tool  config.ToolConfig
	}

	workerCount := min(total, maxConcurrentUpgrades)
	jobs := make(chan upgradeJob, total)
	var wg sync.WaitGroup

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results[job.index] = u.upgradeToolWithProgress(job.tool)
			}
		}()
	}

	for i, tool := range u.tools {
		jobs <- upgradeJob{index: i, tool: tool}
	}
	close(jobs)
	wg.Wait()
	return results
}

// Upgrade upgrades a single tool by name.
// Returns an error if the tool name is not recognized.
func (u *Upgrader) Upgrade(name model.ToolName) (model.UpgradeResult, error) {
	for _, tool := range u.tools {
		if tool.Name == name {
			return u.upgradeToolWithProgress(tool), nil
		}
	}
	return model.UpgradeResult{}, fmt.Errorf("unsupported tool: %s", name)
}

// upgradeToolWithProgress records duration and emits a completion even for skipped tasks.
func (u *Upgrader) upgradeToolWithProgress(tool config.ToolConfig) (result model.UpgradeResult) {
	started := time.Now()
	defer func() {
		result.DurationMillis = time.Since(started).Milliseconds()
		u.emit(model.UpgradeEvent{Name: tool.Name, Phase: model.PhaseFinished, Result: &result})
	}()
	u.emit(model.UpgradeEvent{Name: tool.Name, Phase: model.PhaseDetecting})
	info, err := u.detector.Detect(tool.Name)
	if err != nil {
		return model.UpgradeResult{
			Name:    tool.Name,
			Status:  model.UpgradeStatusFailed,
			Outcome: model.OutcomeFailed,
			Message: fmt.Sprintf("detection failed: %v", err),
		}
	}

	if !info.Installed {
		return model.UpgradeResult{
			Name:    tool.Name,
			Status:  model.UpgradeStatusSkipped,
			Outcome: model.OutcomeSkipped,
			Message: "not installed",
		}
	}

	return u.upgradeTool(tool, info)
}

// upgradeTool performs the upgrade for a single tool.
func (u *Upgrader) upgradeTool(tool config.ToolConfig, info model.ToolInfo) model.UpgradeResult {
	result := model.UpgradeResult{
		Name:       tool.Name,
		Status:     model.UpgradeStatusSkipped,
		Outcome:    model.OutcomeSkipped,
		OldVersion: "",
		NewVersion: "",
		Message:    "",
	}

	result.OldVersion = info.Version

	// Step 1: Check if upgrade is supported
	if !tool.UpgradeSupported {
		result.Message = fmt.Sprintf("automatic upgrade not supported, please upgrade manually: %s", tool.ManualUpgradeURL)
		return result
	}

	// Step 2: Check if install method supports upgrade
	if info.InstallMethod == model.InstallMethodBinary || info.InstallMethod == model.InstallMethodUnknown {
		result.Message = fmt.Sprintf("install method is '%s', cannot auto-upgrade. Please upgrade manually: %s",
			info.InstallMethod, tool.ManualUpgradeURL)
		return result
	}

	// Step 3: Build and execute the upgrade command
	cmdName, cmdArgs, err := u.buildUpgradeCommand(tool, info.InstallMethod, info.Path)
	if err != nil {
		result.Status = model.UpgradeStatusFailed
		result.Outcome = model.OutcomeFailed
		result.Message = err.Error()
		return result
	}

	result.Command = append([]string{cmdName}, cmdArgs...)
	u.emit(model.UpgradeEvent{Name: tool.Name, Phase: model.PhaseWaiting, Source: info.InstallMethod})
	lock := u.managerLock(info)
	lock.Lock()
	defer lock.Unlock()
	u.emit(model.UpgradeEvent{Name: tool.Name, Phase: model.PhaseUpgrading, Source: info.InstallMethod})
	stdout, stderr, exitCode, runErr := u.runner.Run(cmdName, cmdArgs...)
	result.Log = strings.TrimSpace(stdout + "\n" + stderr)
	if runErr != nil {
		result.Status = model.UpgradeStatusFailed
		result.Outcome = model.OutcomeFailed
		result.Message = classifyError(runErr, stderr, cmdName)
		return result
	}
	if exitCode != 0 {
		result.Status = model.UpgradeStatusFailed
		result.Outcome = model.OutcomeFailed
		result.Message = classifyExitError(stderr, exitCode, cmdName)
		return result
	}

	// Step 4: Re-detect to get the new version
	u.emit(model.UpgradeEvent{Name: tool.Name, Phase: model.PhaseVerifying, Source: info.InstallMethod})
	newInfo, detectErr := u.detector.Detect(tool.Name)
	result.NewVersion = newInfo.Version

	// Step 5: Determine success
	result.Status = model.UpgradeStatusSuccess
	if detectErr != nil || !newInfo.Installed || !knownVersion(result.NewVersion) || !knownVersion(result.OldVersion) {
		result.Status = model.UpgradeStatusUnverified
		result.Outcome = model.OutcomeUnverified
		result.Message = "upgrade command succeeded, but the version change could not be verified"
	} else if versionDecreased(result.OldVersion, result.NewVersion) {
		result.Status = model.UpgradeStatusFailed
		result.Outcome = model.OutcomeFailed
		result.Message = "installed version decreased; check the package source and PATH"
	} else if result.NewVersion != result.OldVersion {
		result.Outcome = model.OutcomeUpdated
	} else {
		result.Outcome = model.OutcomeUnchanged
		result.Message = "version unchanged; this does not confirm it is the latest"
	}

	return result
}

// buildUpgradeCommand constructs the upgrade command based on install method.
// For self-update tools, it uses the tool's own binary with SelfUpdateArgs.
func (u *Upgrader) buildUpgradeCommand(tool config.ToolConfig, method model.InstallMethod, binaryPath string) (string, []string, error) {
	switch method {
	case model.InstallMethodSelfUpdate:
		if len(tool.SelfUpdateArgs) == 0 {
			return "", nil, fmt.Errorf("self-update command not configured for %s", tool.Name)
		}
		return binaryPath, tool.SelfUpdateArgs, nil
	case model.InstallMethodNpm:
		return "npm", []string{"install", "-g", tool.NpmPackage + "@latest"}, nil
	case model.InstallMethodPnpm:
		return "pnpm", []string{"add", "-g", tool.NpmPackage}, nil
	case model.InstallMethodYarn:
		return "yarn", []string{"global", "upgrade", tool.NpmPackage}, nil
	case model.InstallMethodBrew:
		if tool.BrewCask {
			return "brew", []string{"upgrade", "--cask", tool.BrewFormula}, nil
		}
		return "brew", []string{"upgrade", tool.BrewFormula}, nil
	case model.InstallMethodWinget:
		return "winget", []string{"upgrade", tool.WingetID}, nil
	case model.InstallMethodScoop:
		return "scoop", []string{"update", tool.ScoopPackage}, nil
	default:
		return "", nil, fmt.Errorf("install method '%s' does not support automatic upgrade", method)
	}
}

// classifyError converts a raw execution error into a user-friendly message.
func classifyError(err error, stderr, cmdName string) string {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "no such file") || strings.Contains(errStr, "not found"):
		return fmt.Sprintf("package manager '%s' not found. Please install it or upgrade manually.", cmdName)
	case strings.Contains(errStr, "permission denied"):
		return fmt.Sprintf("permission denied while running '%s'. Try running with appropriate permissions.", cmdName)
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline"):
		return fmt.Sprintf("'%s' timed out. Please check your network connection and try again.", cmdName)
	default:
		if stderr != "" {
			return fmt.Sprintf("upgrade failed: %s", strings.TrimSpace(stderr))
		}
		return fmt.Sprintf("upgrade failed: %v", err)
	}
}

// classifyExitError converts a non-zero exit code into a user-friendly message.
func classifyExitError(stderr string, exitCode int, cmdName string) string {
	switch {
	case strings.Contains(stderr, "EACCES") || strings.Contains(stderr, "permission"):
		return fmt.Sprintf("permission denied while running '%s'. Try running with appropriate permissions.", cmdName)
	case strings.Contains(stderr, "ETIMEDOUT") || strings.Contains(stderr, "network"):
		return fmt.Sprintf("network error during '%s'. Please check your connection and try again.", cmdName)
	case strings.Contains(stderr, "ECONNREFUSED") || strings.Contains(stderr, "ENOTFOUND"):
		return fmt.Sprintf("network error during '%s'. Please check your connection and try again.", cmdName)
	default:
		errMsg := strings.TrimSpace(stderr)
		if errMsg == "" {
			errMsg = fmt.Sprintf("'%s' exited with code %d", cmdName, exitCode)
		}
		return fmt.Sprintf("upgrade failed: %s", errMsg)
	}
}
