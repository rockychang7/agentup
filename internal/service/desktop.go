package service

import (
	"fmt"
	"time"

	"agentup/internal/desktop"
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
		ID:     fmt.Sprintf("desktop-%d-%d", time.Now().UnixNano(), revision),
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
	for _, plan := range plans {
		if s.activeLocked(plan.Installation) {
			return "", ErrBusy
		}
	}
	task.Status = "queued"
	if err := s.enqueueLocked(task.ID, func() { s.runDesktopUpgrade(task.ID, plans) }); err != nil {
		return "", err
	}
	s.state.DesktopTasks = trimHistory(s.state.DesktopTasks, func(t DesktopTask) string { return t.ID }, s.isActiveLocked)
	s.state.DesktopTasks = append(s.state.DesktopTasks, task)
	s.changedLocked()
	return task.ID, nil
}

func (s *Service) desktopTaskLocked(id string) *DesktopTask {
	for i := range s.state.DesktopTasks {
		if s.state.DesktopTasks[i].ID == id {
			return &s.state.DesktopTasks[i]
		}
	}
	return nil
}

func (s *Service) runDesktopUpgrade(id string, plans []desktop.Plan) {
	// update edits the task if it is still in history; trimming never removes active tasks.
	update := func(change func(*DesktopTask)) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if task := s.desktopTaskLocked(id); task != nil {
			change(task)
			s.changedLocked()
		}
	}
	update(func(task *DesktopTask) { task.Status, task.StartedAt = "running", time.Now() })
	for index, plan := range plans {
		update(func(task *DesktopTask) { task.Items[index].Phase = "upgrading" })
		result := s.desktopManager.Execute(plan, func(stream, text string) {
			update(func(task *DesktopTask) {
				if stream == "phase" {
					task.Items[index].Phase = text
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
			})
		})
		result.Message = tail(clean(result.Message), 8<<10)
		update(func(task *DesktopTask) { task.Items[index].Phase, task.Items[index].Result = "finished", &result })
	}
	update(func(task *DesktopTask) {
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
	})
	s.mu.Lock()
	offline := s.state.Offline
	s.mu.Unlock()
	// Desktop discovery replaces only the desktop snapshot; CLI rows are untouched.
	if s.ctx.Err() == nil {
		snapshot := s.desktopManager.Scan(offline)
		s.mu.Lock()
		s.state.Desktop = snapshot
		s.changedLocked()
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.finishLocked(id)
	s.mu.Unlock()
}
