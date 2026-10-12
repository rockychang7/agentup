package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"agentup/internal/config"
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
	Version    string              `json:"version"`
	OS         string              `json:"os"`
	Arch       string              `json:"arch"`
	Agents     []Agent             `json:"agents"`
	Tasks      []Task              `json:"tasks"`
	Doctor     *model.DoctorResult `json:"doctor"`
	Scanning   bool                `json:"scanning"`
	Diagnosing bool                `json:"diagnosing"`
	ActiveTask string              `json:"active_task"`
	Offline    bool                `json:"offline"`
	Revision   uint64              `json:"revision"`
}

type Service struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	r      runner.Runner
	p      platform.Platform
	tools  []config.ToolConfig
	state  State
	subs   map[chan struct{}]struct{}
}

func New(ctx context.Context, version, arch string, r runner.Runner, p platform.Platform, tools []config.ToolConfig) *Service {
	ctx, cancel := context.WithCancel(ctx)
	if _, ok := r.(*runner.DefaultRunner); ok {
		r = &runner.DefaultRunner{Context: ctx}
	}
	s := &Service{ctx: ctx, cancel: cancel, r: r, p: p, tools: tools, subs: make(map[chan struct{}]struct{})}
	s.state = State{Version: version, OS: p.Name(), Arch: arch, Agents: []Agent{}, Tasks: []Task{}}
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
	if s.state.ActiveTask != "" && !force {
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

func (s *Service) changedLocked() {
	s.state.Revision++
	for ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s *Service) busyLocked() bool {
	return s.state.Scanning || s.state.Diagnosing || s.state.ActiveTask != "" || s.ctx.Err() != nil
}

func (s *Service) Refresh(offline bool) error {
	s.mu.Lock()
	if s.busyLocked() {
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
	set := func(info model.ToolInfo) {
		a := Agent{ToolInfo: info}
		if info.Installed && info.UpgradeSupported {
			a.Command, _ = u.Preview(info)
		}
		s.mu.Lock()
		for i := range s.state.Agents {
			if s.state.Agents[i].Name == a.Name {
				s.state.Agents[i] = a
			}
		}
		s.changedLocked()
		s.mu.Unlock()
	}
	local := d.DetectAllLocal(func(info model.ToolInfo) {
		if !offline && info.Installed {
			info.LatestCheck = model.CheckPending
		}
		set(info)
	})
	if !offline && s.ctx.Err() == nil {
		d.CheckAllLatest(local, set)
	}
}

func (s *Service) Diagnose() error {
	s.mu.Lock()
	if s.busyLocked() {
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
	}
	release, err := operation.Acquire("mutation")
	if err != nil {
		return "", ErrBusy
	}
	id := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), s.state.Revision)
	task := Task{ID: id, Status: "running", StartedAt: time.Now(), Tools: []TaskTool{}, Logs: []Log{}}
	for _, name := range names {
		task.Tools = append(task.Tools, TaskTool{Name: name, Phase: model.PhaseWaiting})
	}
	if len(s.state.Tasks) >= 20 {
		s.state.Tasks = s.state.Tasks[1:]
	}
	s.state.Tasks = append(s.state.Tasks, task)
	s.state.ActiveTask = id
	s.changedLocked()
	s.wg.Add(1)
	go s.runUpgrade(names, commands, release)
	return id, nil
}

func (s *Service) runUpgrade(names []model.ToolName, commands map[model.ToolName][]string, release func()) {
	defer s.wg.Done()
	defer release()
	d := detector.NewWithoutLatestCheck(s.r, s.p, s.tools)
	u := upgrader.New(s.r, s.p, d, s.tools)
	u.SetStreamLogs(true)
	u.SetObserver(func(event model.UpgradeEvent) {
		s.mu.Lock()
		defer s.mu.Unlock()
		task := &s.state.Tasks[len(s.state.Tasks)-1]
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
			_, _ = u.UpgradeConfirmed(name, commands[name])
		}(name)
	}
	wg.Wait()
	s.mu.Lock()
	task := &s.state.Tasks[len(s.state.Tasks)-1]
	task.Status = "completed"
	finished := time.Now()
	task.FinishedAt = &finished
	for _, item := range task.Tools {
		if item.Result == nil || item.Result.Status == model.UpgradeStatusFailed || item.Result.Status == model.UpgradeStatusUnverified {
			task.Status = "failed"
		}
	}
	s.state.ActiveTask = ""
	offline := s.state.Offline
	scan := s.ctx.Err() == nil
	s.state.Scanning = scan
	s.changedLocked()
	s.mu.Unlock()
	// Re-check actual installed versions before releasing the operation lock.
	if scan {
		s.scan(offline)
	}
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
