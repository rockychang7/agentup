package table

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"agentup/internal/model"
)

// liveFrame redraws a bounded block only on a real terminal; pipes never receive cursor codes.
type liveFrame struct {
	printer *Printer
	lines   int
}

func (f *liveFrame) draw(lines []string) {
	if !f.printer.Interactive {
		return
	}
	if f.lines > 0 {
		fmt.Fprintf(f.printer.Output, "\x1b[%dA", f.lines)
	}
	for _, line := range lines {
		fmt.Fprintf(f.printer.Output, "\r\x1b[2K%s\n", ansi.Truncate(line, f.printer.Width-1, "..."))
	}
	for i := len(lines); i < f.lines; i++ {
		fmt.Fprint(f.printer.Output, "\r\x1b[2K\n")
	}
	if f.lines > len(lines) {
		fmt.Fprintf(f.printer.Output, "\x1b[%dA", f.lines-len(lines))
	}
	f.lines = len(lines)
}

func (f *liveFrame) clear() {
	f.draw(nil)
	f.lines = 0
}

type taskState struct {
	event   model.UpgradeEvent
	started time.Time
}

type Progress struct {
	mu        sync.Mutex
	printer   *Printer
	frame     liveFrame
	names     []model.ToolName
	tasks     map[model.ToolName]taskState
	started   time.Time
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewProgress(p *Printer, names []model.ToolName) *Progress {
	progress := &Progress{
		printer: p, frame: liveFrame{printer: p}, names: names,
		tasks: make(map[model.ToolName]taskState), started: time.Now(),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	for _, name := range names {
		progress.tasks[name] = taskState{event: model.UpgradeEvent{Name: name, Phase: model.PhaseWaiting}}
	}
	if p.Interactive {
		progress.renderLocked()
		go func() {
			defer close(progress.done)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					progress.mu.Lock()
					progress.renderLocked()
					progress.mu.Unlock()
				case <-progress.stop:
					return
				}
			}
		}()
	} else {
		p.Notice(fmt.Sprintf("Upgrading %d agents...", len(names)))
		close(progress.done)
	}
	return progress
}

func (p *Progress) Observe(event model.UpgradeEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.tasks[event.Name]
	if event.Source == "" {
		event.Source = state.event.Source
	}
	if state.started.IsZero() {
		state.started = time.Now()
	}
	state.event = event
	p.tasks[event.Name] = state
	if p.printer.Interactive {
		p.renderLocked()
	} else {
		text := fmt.Sprintf("%s: %s", event.Name, event.Phase)
		if event.Result != nil {
			text = fmt.Sprintf("%s: %s (%s)", event.Name, Outcome(*event.Result), Duration(event.Result.DurationMillis))
		} else if event.Source != "" {
			text += " (" + string(event.Source) + ")"
		}
		p.printer.Notice(text)
	}
}

func (p *Progress) renderLocked() {
	completed := 0
	lines := make([]string, 1, len(p.names)+1)
	for _, name := range p.names {
		state := p.tasks[name]
		phase, tone := string(state.event.Phase), ""
		elapsed := ""
		if !state.started.IsZero() {
			elapsed = Duration(time.Since(state.started).Milliseconds())
		}
		if state.event.Result != nil {
			completed++
			outcome := Outcome(*state.event.Result)
			phase = string(outcome)
			if outcome == model.OutcomeFailed {
				tone = "failure"
			} else if outcome == model.OutcomeUpdated {
				tone = "success"
			} else if outcome != model.OutcomeSkipped {
				tone = "warning"
			}
			elapsed = Duration(state.event.Result.DurationMillis)
		}
		text := fmt.Sprintf("  %-11s  %-11s  %s  %s", name, phase, state.event.Source, elapsed)
		lines = append(lines, p.printer.tone(text, tone))
	}
	filled := 0
	if len(p.names) > 0 {
		filled = completed * 12 / len(p.names)
	}
	lines[0] = p.printer.heading.Render(fmt.Sprintf("Upgrade [%s%s] %d/%d  %s",
		strings.Repeat("#", filled), strings.Repeat("-", 12-filled), completed, len(p.names),
		Duration(time.Since(p.started).Milliseconds())))
	p.frame.draw(lines)
}

func (p *Progress) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
		p.mu.Lock()
		defer p.mu.Unlock()
		p.frame.clear()
	})
}

// Scan shows local versions before any registry lookups, keeping one row per configured agent.
type Scan struct {
	mu      sync.Mutex
	printer *Printer
	frame   liveFrame
	names   []model.ToolName
	tools   map[model.ToolName]model.ToolInfo
	phase   string
}

func NewScan(p *Printer, names []model.ToolName) *Scan {
	s := &Scan{printer: p, frame: liveFrame{printer: p}, names: names,
		tools: make(map[model.ToolName]model.ToolInfo), phase: "Detecting local agents..."}
	s.renderLocked()
	return s
}

func (s *Scan) Update(info model.ToolInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[info.Name] = info
	s.renderLocked()
}

func (s *Scan) SetPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
	s.renderLocked()
}

func (s *Scan) renderLocked() {
	lines := []string{s.printer.heading.Render(s.phase)}
	for _, name := range s.names {
		text := fmt.Sprintf("  %-11s  detecting...", name)
		if tool, ok := s.tools[name]; ok {
			status, tone := toolStatus(tool)
			text = s.printer.tone(fmt.Sprintf("  %-11s  %s  %s  %s", name, singleLine(version(tool.Version)), tool.InstallMethod, status), tone)
		}
		lines = append(lines, text)
	}
	s.frame.draw(lines)
}

func (s *Scan) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frame.clear()
}
