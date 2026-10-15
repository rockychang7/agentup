package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"agentup/internal/model"
	"agentup/internal/runner"
)

type Manager struct {
	ctx     context.Context
	runner  runner.Runner
	os      string
	arch    string
	home    string
	appDirs []string
	// fetch reads small official metadata documents from allow-listed hosts.
	fetch func(ctx context.Context, url string, limit int64) ([]byte, error)
	// download stores an installer from an allow-listed host and returns its path.
	download   func(ctx context.Context, asset Asset, dir string) (string, error)
	procRoot   string
	desktopDir []string
	receiptDir string
	cacheDir   string
	terminal   bool
	poll       time.Duration
	// verifyWindow bounds how long a finished installer is polled for a new version.
	verifyWindow time.Duration
}

func New(ctx context.Context, r runner.Runner, osName, arch string) *Manager {
	home, _ := os.UserHomeDir()
	switch osName {
	case "macOS":
		osName = "darwin"
	case "Windows":
		osName = "windows"
	case "Linux":
		osName = "linux"
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	config, _ := os.UserConfigDir()
	cache, _ := os.UserCacheDir()
	return &Manager{
		ctx: ctx, runner: r, os: osName, arch: arch, home: home,
		fetch: fetchMetadata, download: downloadAsset, procRoot: "/proc",
		desktopDir: []string{filepath.Join(home, ".local", "share", "applications"), "/usr/share/applications"},
		receiptDir: filepath.Join(config, "agentup", "desktop-receipts"),
		cacheDir:   filepath.Join(cache, "agentup", "desktop-downloads"),
		poll:       3 * time.Second, verifyWindow: 2 * time.Minute,
	}
}

// EnableTerminal allows privileged commands that prompt in the current terminal.
// Only the CLI calls this; the Web UI never runs privileged plans.
func (m *Manager) EnableTerminal() { m.terminal = true }

func (m *Manager) Scan(offline bool) Snapshot {
	snapshot := Snapshot{Installations: []Installation{}, Issues: []string{}}
	switch m.os {
	case "windows":
		m.scanWindows(&snapshot, offline)
	case "darwin":
		m.scanMac(&snapshot, offline)
	case "linux":
		m.scanLinux(&snapshot, offline)
	default:
		snapshot.Issues = append(snapshot.Issues, "Desktop discovery is unavailable on this operating system.")
	}
	seen := make(map[string]bool)
	present := make(map[string]bool)
	items := []Installation{}
	for _, info := range snapshot.Installations {
		if info.ID == "" {
			info.ID = installationID(info)
		}
		if seen[info.ID] {
			continue
		}
		seen[info.ID], present[info.ProductID] = true, true
		m.finalize(&info, offline)
		items = append(items, info)
	}
	for _, product := range Products() {
		if present[product.ID] {
			continue
		}
		info := missing(product)
		if len(snapshot.Issues) != 0 {
			info.LatestCheck, info.Reason = model.CheckFailed, "discovery_incomplete"
		}
		items = append(items, info)
	}
	order := map[string]int{}
	for i, product := range Products() {
		order[product.ID] = i
	}
	sort.SliceStable(items, func(i, j int) bool {
		if order[items[i].ProductID] != order[items[j].ProductID] {
			return order[items[i].ProductID] < order[items[j].ProductID]
		}
		return items[i].Path < items[j].Path
	})
	snapshot.Installations = items
	return snapshot
}

// finalize applies the shared safety rules in one place, after platform discovery.
func (m *Manager) finalize(info *Installation, offline bool) {
	if offline {
		info.LatestCheck, info.LatestVersion, info.UpgradeAvailable = model.CheckOffline, "", false
		info.Plan = nil
		return
	}
	if info.LatestCheck == model.CheckOffline {
		info.LatestCheck = model.CheckUnavailable
	}
	if executable(info.Capability) {
		switch info.Running {
		case "running":
			info.Capability, info.Reason = Blocked, "app_running"
		case "unknown":
			info.Capability, info.Reason = Blocked, "process_check_failed"
		}
	}
	if m.pendingReceipt(info) {
		info.Capability, info.Reason = Blocked, "install_pending_verification"
	}
	info.Plan = makePlan(*info, info.spec)
}

func (m *Manager) run(name string, args ...string) (string, error) {
	if err := m.ctx.Err(); err != nil {
		return "", err
	}
	out, stderr, code, err := runner.WithTimeout(m.runner, 30*time.Second).Run(name, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("%s exited %d: %.1000s", name, code, strings.TrimSpace(stderr))
	}
	if len(out) > 8<<20 {
		return "", fmt.Errorf("%s returned too much discovery data", name)
	}
	return strings.TrimPrefix(out, "\ufeff"), nil
}

// Execute re-derives the plan from current local state before any mutation.
// Callers hold the shared AgentUp mutation lock for the entire operation.
// log receives command output; the special stream "phase" reports task phases.
func (m *Manager) Execute(confirmed Plan, log func(string, string)) (result Result) {
	started := time.Now()
	result = Result{
		Installation: confirmed.Installation, ProductID: confirmed.ProductID,
		Outcome: "failed", OldVersion: confirmed.BeforeVersion,
	}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if log == nil {
		log = func(string, string) {}
	}
	if err := m.ctx.Err(); err != nil {
		result.Message = err.Error()
		return
	}
	log("phase", "checking")
	before, err := m.Scan(false).Find(confirmed.Installation)
	if err != nil {
		result.Message = err.Error()
		return
	}
	result.Name = before.Name
	if before.Plan == nil || before.Plan.ID != confirmed.ID || digestPlan(confirmed) != confirmed.ID {
		result.Message = "Installation, version, channel or update plan changed; refresh and confirm again."
		return
	}
	if confirmed.Action == ActionGuide {
		result.Outcome, result.URL = "handed_off", before.UpdateURL
		result.Message = "Use the official update entry, then refresh to verify the installed version."
		return
	}
	if before.Running != "stopped" || !executable(before.Capability) {
		result.Message = "Fully quit the desktop app before upgrading; processes are never terminated automatically."
		return
	}
	switch confirmed.Action {
	case ActionCommand:
		result.Command = append([]string(nil), confirmed.Command...)
		log("phase", "upgrading")
		r := runner.WithTimeout(m.runner, 20*time.Minute)
		streaming := r.(interface {
			RunStreaming(string, []string, func(string, string)) (string, string, int, error)
		})
		_, stderr, code, err := streaming.RunStreaming(confirmed.Command[0], confirmed.Command[1:], log)
		if err != nil {
			result.Message = err.Error()
			return
		}
		if code != 0 {
			result.Outcome, result.Message = commandFailure(confirmed, code, stderr)
			return
		}
	case ActionPrivileged:
		result.Command = append([]string(nil), confirmed.Command...)
		interactive, ok := m.runner.(interface {
			RunInteractive(string, []string) (int, error)
		})
		if !m.terminal || !ok {
			result.Message = "This update needs administrator rights. Run the shown command in a terminal, or use agentup upgrade from a terminal."
			return
		}
		log("phase", "needs_elevation")
		code, err := interactive.RunInteractive(confirmed.Command[0], confirmed.Command[1:])
		if err != nil {
			result.Message = err.Error()
			return
		}
		if code != 0 {
			result.Message = fmt.Sprintf("The package manager exited %d; authorization may have been refused.", code)
			return
		}
	case ActionInstaller:
		// The installer flow downloads, verifies and polls the same installation itself.
		result.Outcome, result.Message, result.NewVersion = m.runInstaller(confirmed, log)
		result.URL = confirmed.Asset.URL
		return
	default:
		result.Message = "Unsupported update action."
		return
	}
	log("phase", "verifying")
	after, err := m.Scan(true).Find(confirmed.Installation)
	if err != nil || !after.Installed {
		result.Outcome, result.Message = "unverified", "Command completed, but the same installation could not be found."
		return
	}
	result.NewVersion = after.Version
	result.Outcome, result.Message = m.verdict(confirmed, result.OldVersion, result.NewVersion)
	if result.Outcome == "updated" {
		m.clearReceipt(confirmed.Installation)
	}
	return
}

func (m *Manager) verdict(plan Plan, before, after string) (string, string) {
	cmp, ok := m.compare(plan.Source, before, after)
	switch {
	case !ok:
		return "unverified", "Command completed, but the version change could not be verified."
	case cmp > 0 || !supportedLine(plan.ProductID, before, after):
		return "failed", "Version decreased or release line changed; inspect the package channel."
	case cmp == 0:
		return "unchanged", "Version unchanged; this does not confirm it is the latest."
	}
	return "updated", ""
}

// compare uses the comparator of the package format that produced the versions.
func (m *Manager) compare(source, a, b string) (int, bool) {
	switch source {
	case "apt":
		dpkg, err := m.runner.LookPath("dpkg")
		if err != nil {
			return 0, false
		}
		if a == b {
			return 0, true
		}
		_, _, code, err := runner.WithTimeout(m.runner, 15*time.Second).Run(dpkg, "--compare-versions", a, "lt", b)
		if err != nil || (code != 0 && code != 1) {
			return 0, false
		}
		if code == 0 {
			return -1, true
		}
		return 1, true
	case "rpm":
		return rpmCompare(a, b), a != "" && b != ""
	}
	return compareVersion(a, b)
}

func digestPlan(plan Plan) string {
	plan.ID = ""
	return digest(plan)
}
