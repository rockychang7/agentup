package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"agentup/internal/config"
	"agentup/internal/desktop"
	"agentup/internal/detector"
	"agentup/internal/diagnostic"
	"agentup/internal/model"
	"agentup/internal/operation"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/upgrader"
)

var (
	ErrBusy        = errors.New("busy")
	ErrInvalid     = errors.New("invalid_tools")
	ErrUnsupported = errors.New("unsupported_upgrade")
	ErrOffline     = errors.New("offline_read_only")
	ErrStale       = errors.New("stale_preview")
)

type Agent struct {
	model.ToolInfo
	Command []string `json:"command,omitempty"`
}

type TaskTool struct {
	Name   model.ToolName       `json:"name"`
	Phase  model.UpgradePhase   `json:"phase"`
	Result *model.UpgradeResult `json:"result,omitempty"`
}

type Log struct {
	Name   model.ToolName `json:"name"`
	Stream string         `json:"stream"`
	Text   string         `json:"text"`
}

type Task struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Tools        []TaskTool `json:"tools"`
	Logs         []Log      `json:"logs"`
	LogTruncated bool       `json:"log_truncated"`
}

type State struct {
	Version      string              `json:"version"`
	OS           string              `json:"os"`
	Arch         string              `json:"arch"`
	Agents       []Agent             `json:"agents"`
	Tasks        []Task              `json:"tasks"`
	Doctor       *model.DoctorResult `json:"doctor"`
	Scanning     bool                `json:"scanning"`
	Diagnosing   bool                `json:"diagnosing"`
	ActiveTasks  []string            `json:"active_tasks"`
	Offline      bool                `json:"offline"`
	Revision     uint64              `json:"revision"`
	Desktop      desktop.Snapshot    `json:"desktop"`
	DesktopTasks []DesktopTask       `json:"desktop_tasks"`
}

type Service struct {
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	r              runner.Runner
	p              platform.Platform
	tools          []config.ToolConfig
	state          State
	subs           map[chan struct{}]struct{}
	desktopManager desktopBackend
	// fingerprint covers what a confirmation shows; Revision only moves when it changes.
	fingerprint [32]byte
	resources   map[string]*sync.Mutex
	release     func()
}

func New(ctx context.Context, version, arch string, r runner.Runner, p platform.Platform, tools []config.ToolConfig) *Service {
	ctx, cancel := context.WithCancel(ctx)
	if _, ok := r.(*runner.DefaultRunner); ok {
		r = &runner.DefaultRunner{Context: ctx}
	}
	s := &Service{ctx: ctx, cancel: cancel, r: r, p: p, tools: tools, subs: make(map[chan struct{}]struct{}),
		desktopManager: desktop.New(ctx, r, p.Name(), arch), resources: make(map[string]*sync.Mutex)}
	s.state = State{Version: version, OS: p.Name(), Arch: arch, Agents: []Agent{}, Tasks: []Task{}, ActiveTasks: []string{}}
	s.state.Desktop = desktop.Snapshot{Installations: []desktop.Installation{}, Issues: []string{}}
	s.state.DesktopTasks = []DesktopTask{}
	for _, tool := range tools {
		s.state.Agents = append(s.state.Agents, Agent{ToolInfo: model.ToolInfo{Name: tool.Name, LatestCheck: model.CheckPending}})
	}
	return s
}

func (s *Service) Close() {
	_ = s.Stop(true)
	s.wg.Wait()
}

// Stop atomically rejects new work before cancellation without waiting on HTTP handlers.
func (s *Service) Stop(force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.state.ActiveTasks) != 0 && !force {
		return ErrBusy
	}
	s.cancel()
	return nil
}

func (s *Service) JSON() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(s.state)
	return data
}

func (s *Service) Snapshot() State {
	var result State
	_ = json.Unmarshal(s.JSON(), &result)
	return result
}

func (s *Service) Subscribe() (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan struct{}, 1)
	s.subs[ch] = struct{}{}
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

// changedLocked notifies subscribers. Log lines and phases do not invalidate an
// open confirmation, so the revision follows only versions, commands and plans.
func (s *Service) changedLocked() {
	data, _ := json.Marshal(struct {
		Offline bool
		Agents  []Agent
		Desktop desktop.Snapshot
	}{s.state.Offline, s.state.Agents, s.state.Desktop})
	if sum := sha256.Sum256(data); sum != s.fingerprint {
		s.fingerprint = sum
		s.state.Revision++
	}
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Independent upgrades may start while other rows are updating.
func (s *Service) busyLocked() bool {
	return s.state.Scanning || s.state.Diagnosing || s.ctx.Err() != nil
}

// idleLocked is required for a full refresh or diagnosis, which replace shared state.
func (s *Service) idleLocked() bool {
	return !s.busyLocked() && len(s.state.ActiveTasks) == 0
}

// startLocked starts work immediately and keeps other AgentUp instances out until
// all active operations finish. Only the same package manager waits on a lock.
func (s *Service) startLocked(id string, run func()) error {
	if s.release == nil {
		release, err := operation.Acquire("mutation")
		if err != nil {
			return ErrBusy
		}
		s.release = release
	}
	s.state.ActiveTasks = append(s.state.ActiveTasks, id)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		run()
	}()
	return nil
}

func (s *Service) resourceLock(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources[key] == nil {
		s.resources[key] = &sync.Mutex{}
	}
	return s.resources[key]
}

func (s *Service) finishLocked(id string) {
	for i, active := range s.state.ActiveTasks {
		if active == id {
			s.state.ActiveTasks = append(s.state.ActiveTasks[:i:i], s.state.ActiveTasks[i+1:]...)
			break
		}
	}
	if len(s.state.ActiveTasks) == 0 && s.release != nil {
		s.release()
		s.release = nil
	}
	s.changedLocked()
}

// activeLocked reports whether a queued or running task already covers the key.
func (s *Service) activeLocked(key string) bool {
	for _, id := range s.state.ActiveTasks {
		if task := s.taskLocked(id); task != nil {
			for _, tool := range task.Tools {
				if string(tool.Name) == key {
					return true
				}
			}
		}
		if task := s.desktopTaskLocked(id); task != nil {
			for _, item := range task.Items {
				if item.Installation == key {
					return true
				}
			}
		}
	}
	return false
}

func (s *Service) taskLocked(id string) *Task {
	for i := range s.state.Tasks {
		if s.state.Tasks[i].ID == id {
			return &s.state.Tasks[i]
		}
	}
	return nil
}

func (s *Service) isActiveLocked(id string) bool {
	return slices.Contains(s.state.ActiveTasks, id)
}

// trimHistory drops the oldest finished entry once history is full; active tasks stay.
func trimHistory[T any](items []T, id func(T) string, active func(string) bool) []T {
	if len(items) < 20 {
		return items
	}
	for i, item := range items {
		if !active(id(item)) {
			return append(items[:i:i], items[i+1:]...)
		}
	}
	return items
}

func (s *Service) Refresh(offline bool) error {
	s.mu.Lock()
	if !s.idleLocked() {
		s.mu.Unlock()
		return ErrBusy
	}
	s.state.Scanning, s.state.Offline = true, offline
	for i := range s.state.Agents {
		s.state.Agents[i].LatestCheck = model.CheckPending
	}
	s.changedLocked()
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.scan(offline)
	}()
	return nil
}

func (s *Service) scan(offline bool) {
	defer func() {
		s.mu.Lock()
		s.state.Scanning = false
		s.changedLocked()
		s.mu.Unlock()
	}()
	d := detector.New(s.r, s.p, s.tools)
	u := upgrader.New(s.r, s.p, d, s.tools)
	set := func(info model.ToolInfo) { s.setAgent(u, info) }
	local := d.DetectAllLocal(func(info model.ToolInfo) {
		if !offline && info.Installed {
			info.LatestCheck = model.CheckPending
		}
		set(info)
	})
	if s.ctx.Err() == nil {
		snapshot := s.desktopManager.Scan(offline)
		s.mu.Lock()
		s.state.Desktop = snapshot
		s.changedLocked()
		s.mu.Unlock()
	}
	if !offline && s.ctx.Err() == nil {
		d.CheckAllLatest(local, set)
	}
}

func (s *Service) Diagnose() error {
	s.mu.Lock()
	if !s.idleLocked() {
		s.mu.Unlock()
		return ErrBusy
	}
	s.state.Diagnosing = true
	s.changedLocked()
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		d := detector.NewWithoutLatestCheck(s.r, s.p, s.tools)
		result := diagnostic.Run(s.r, s.p, d)
		s.mu.Lock()
		s.state.Doctor, s.state.Diagnosing = &result, false
		s.changedLocked()
		s.mu.Unlock()
	}()
	return nil
}

func (s *Service) Upgrade(names []model.ToolName, revision uint64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busyLocked() {
		return "", ErrBusy
	}
	if s.state.Offline {
		return "", ErrOffline
	}
	if revision != s.state.Revision {
		return "", ErrStale
	}
	if len(names) == 0 || len(names) > len(s.tools) {
		return "", ErrInvalid
	}
	seen := make(map[model.ToolName]bool)
	commands := make(map[model.ToolName][]string)
	for _, name := range names {
		if seen[name] || !model.IsValidToolName(string(name)) {
			return "", ErrInvalid
		}
		seen[name] = true
		found := false
		for _, a := range s.state.Agents {
			if a.Name == name && a.Installed && a.UpgradeSupported && len(a.Command) != 0 {
				found = true
				commands[name] = append([]string(nil), a.Command...)
			}
		}
		if !found {
			return "", ErrUnsupported
		}
		if s.activeLocked(string(name)) {
			return "", ErrBusy
		}
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), s.state.Revision)
	if err := s.startLocked(id, func() { s.runUpgrade(id, names, commands) }); err != nil {
		return "", err
	}
	task := Task{ID: id, Status: "running", StartedAt: time.Now(), Tools: []TaskTool{}, Logs: []Log{}}
	for _, name := range names {
		task.Tools = append(task.Tools, TaskTool{Name: name, Phase: model.PhaseWaiting})
	}
	s.state.Tasks = trimHistory(s.state.Tasks, func(t Task) string { return t.ID }, s.isActiveLocked)
	s.state.Tasks = append(s.state.Tasks, task)
	s.changedLocked()
	return id, nil
}

func (s *Service) runUpgrade(id string, names []model.ToolName, commands map[model.ToolName][]string) {
	s.mu.Lock()
	resources := make(map[model.ToolName]string)
	for _, agent := range s.state.Agents {
		key := string(agent.InstallMethod)
		if key == "self-update" {
			key += ":" + agent.Path
		}
		resources[agent.Name] = key
	}
	if task := s.taskLocked(id); task != nil {
		task.Status, task.StartedAt = "running", time.Now()
	}
	s.changedLocked()
	s.mu.Unlock()
	d := detector.NewWithoutLatestCheck(s.r, s.p, s.tools)
	u := upgrader.New(s.r, s.p, d, s.tools)
	u.SetStreamLogs(true)
	u.SetObserver(func(event model.UpgradeEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		task := s.taskLocked(id)
		if task == nil {
			return
		}
		for i := range task.Tools {
			if task.Tools[i].Name == event.Name {
				task.Tools[i].Phase = event.Phase
				if event.Result != nil {
					result := *event.Result
					// Streamed output already lives in the bounded task tail.
					result.Log = ""
					result.Message = tail(clean(result.Message), 8<<10)
					task.Tools[i].Result = &result
				}
			}
		}
		if event.Log != "" {
			text := clean(event.Log)
			if len(text) > 16<<10 {
				task.LogTruncated = true
			}
			task.Logs = append(task.Logs, Log{Name: event.Name, Stream: event.Stream, Text: tail(text, 16<<10)})
			// A bounded tail survives browser reconnection without growing indefinitely.
			total := 0
			first := len(task.Logs)
			for first > 0 && total+len(task.Logs[first-1].Text) <= 128<<10 && len(task.Logs)-first < 1000 {
				first--
				total += len(task.Logs[first].Text)
			}
			if first > 0 {
				task.Logs = append([]Log{}, task.Logs[first:]...)
				task.LogTruncated = true
			}
		}
		s.changedLocked()
	})
	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(name model.ToolName) {
			defer wg.Done()
			lock := s.resourceLock(resources[name])
			lock.Lock()
			defer lock.Unlock()
			_, _ = u.UpgradeConfirmed(name, commands[name])
		}(name)
	}
	wg.Wait()
	s.mu.Lock()
	if task := s.taskLocked(id); task != nil {
		task.Status = "completed"
		finished := time.Now()
		task.FinishedAt = &finished
		for _, item := range task.Tools {
			if item.Result == nil || item.Result.Status == model.UpgradeStatusFailed || item.Result.Status == model.UpgradeStatusUnverified {
				task.Status = "failed"
			}
		}
	}
	offline := s.state.Offline
	s.changedLocked()
	s.mu.Unlock()
	// Re-check only the upgraded tools, so other rows keep their state and stay usable.
	if s.ctx.Err() == nil {
		s.redetect(names, offline)
	}
	s.mu.Lock()
	s.finishLocked(id)
	s.mu.Unlock()
}

func (s *Service) redetect(names []model.ToolName, offline bool) {
	d := detector.New(s.r, s.p, s.tools)
	if offline {
		d = detector.NewWithoutLatestCheck(s.r, s.p, s.tools)
	}
	u := upgrader.New(s.r, s.p, d, s.tools)
	for _, name := range names {
		info, err := d.Detect(name)
		if err != nil {
			continue
		}
		s.setAgent(u, info)
	}
}

func (s *Service) setAgent(u *upgrader.Upgrader, info model.ToolInfo) {
	a := Agent{ToolInfo: info}
	if info.Installed && info.UpgradeSupported {
		a.Command, _ = u.Preview(info)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Agents {
		if s.state.Agents[i].Name == a.Name {
			s.state.Agents[i] = a
		}
	}
	s.changedLocked()
}

func clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(text))
}

func tail(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	start := len(text) - limit
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return text[start:]
}
