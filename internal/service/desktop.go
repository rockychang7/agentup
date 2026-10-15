package service

import (
	"fmt"
	"time"

	"agentup/internal/desktop"
	"agentup/internal/operation"
)

type desktopBackend interface {
	Scan(bool) desktop.Snapshot
	Execute(desktop.Plan, func(string, string)) desktop.Result
}

type DesktopTaskItem struct {
	Installation string          `json:"installation_id"`
	Name         string          `json:"name"`
	Phase        string          `json:"phase"`
	Result       *desktop.Result `json:"result,omitempty"`
}

type DesktopLog struct {
	Installation string `json:"installation_id"`
	Stream       string `json:"stream"`
	Text         string `json:"text"`
}

type DesktopTask struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	StartedAt    time.Time         `json:"started_at"`
	FinishedAt   *time.Time        `json:"finished_at,omitempty"`
	Items        []DesktopTaskItem `json:"items"`
	Logs         []DesktopLog      `json:"logs"`
	LogTruncated bool              `json:"log_truncated"`
}

func (s *Service) UpgradeDesktops(planIDs []string, revision uint64) (string, error) {
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
	if len(planIDs) == 0 || len(planIDs) > 16 {
		return "", ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range planIDs {
		if id == "" || seen[id] {
			return "", ErrInvalid
		}
		seen[id] = true
	}
	plans := []desktop.Plan{}
	task := DesktopTask{
		ID:     fmt.Sprintf("desktop-%d-%d", time.Now().UnixMilli(), revision),
		Status: "running", StartedAt: time.Now(),
		Items: []DesktopTaskItem{}, Logs: []DesktopLog{},
	}
	for _, id := range planIDs {
		found := false
		for _, info := range s.state.Desktop.Installations {
			if info.Plan == nil || info.Plan.ID != id || !info.Installed {
				continue
			}
			// Privileged plans prompt for a password and only run from a terminal.
			// Batches contain only automatic plans that may run without a separate confirmation.
			if info.Capability == desktop.Blocked || info.Capability == desktop.Privileged ||
				(len(planIDs) > 1 && (info.Capability != desktop.Automatic || info.Plan.Standalone)) {
				return "", ErrUnsupported
			}
			plans = append(plans, *info.Plan)
			task.Items = append(task.Items, DesktopTaskItem{
				Installation: info.ID, Name: info.Name, Phase: "waiting",
			})
			found = true
			break
		}
		if !found {
			return "", ErrStale
		}
	}
	release, err := operation.Acquire("mutation")
	if err != nil {
		return "", ErrBusy
	}
	if len(s.state.DesktopTasks) >= 20 {
		s.state.DesktopTasks = s.state.DesktopTasks[1:]
	}
	s.state.DesktopTasks = append(s.state.DesktopTasks, task)
	s.state.ActiveTask = task.ID
	s.changedLocked()
	s.wg.Add(1)
	go s.runDesktopUpgrade(plans, release)
	return task.ID, nil
}

func (s *Service) runDesktopUpgrade(plans []desktop.Plan, release func()) {
	defer s.wg.Done()
	defer release()
	for index, plan := range plans {
		s.mu.Lock()
		task := &s.state.DesktopTasks[len(s.state.DesktopTasks)-1]
		task.Items[index].Phase = "upgrading"
		s.changedLocked()
		s.mu.Unlock()
		result := s.desktopManager.Execute(plan, func(stream, text string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			task := &s.state.DesktopTasks[len(s.state.DesktopTasks)-1]
			if stream == "phase" {
				task.Items[index].Phase = text
				s.changedLocked()
				return
			}
			text = clean(text)
			if len(text) > 16<<10 {
				task.LogTruncated = true
			}
			task.Logs = append(task.Logs, DesktopLog{
				Installation: plan.Installation, Stream: stream, Text: tail(text, 16<<10),
			})
			total, first := 0, len(task.Logs)
			for first > 0 && total+len(task.Logs[first-1].Text) <= 128<<10 && len(task.Logs)-first < 1000 {
				first--
				total += len(task.Logs[first].Text)
			}
			if first > 0 {
				task.Logs = append([]DesktopLog{}, task.Logs[first:]...)
				task.LogTruncated = true
			}
			s.changedLocked()
		})
		result.Message = tail(clean(result.Message), 8<<10)
		s.mu.Lock()
		task = &s.state.DesktopTasks[len(s.state.DesktopTasks)-1]
		task.Items[index].Phase, task.Items[index].Result = "finished", &result
		s.changedLocked()
		s.mu.Unlock()
	}
	s.mu.Lock()
	task := &s.state.DesktopTasks[len(s.state.DesktopTasks)-1]
	task.Status = "completed"
	finished := time.Now()
	task.FinishedAt = &finished
	for _, item := range task.Items {
		if item.Result == nil || item.Result.Outcome == "failed" || item.Result.Outcome == "unverified" {
			task.Status = "failed"
			break
		}
		if item.Result.Outcome == "handed_off" {
			task.Status = "handed_off"
		}
	}
	s.state.ActiveTask = ""
	scan, offline := s.ctx.Err() == nil, s.state.Offline
	s.state.Scanning = scan
	s.changedLocked()
	s.mu.Unlock()
	if scan {
		s.scan(offline)
	}
}
