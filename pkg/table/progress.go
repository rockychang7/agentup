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
	printer      *Printer
	lines        int
	content      string
	cursorHidden bool
}

func (f *liveFrame) draw(lines []string) {
	if !f.printer.Interactive {
		return
	}
	rendered := make([]string, len(lines))
	for i, line := range lines {
		rendered[i] = ansi.Truncate(line, f.printer.Width-1, "...")
	}
	content := strings.Join(rendered, "\n")
	if f.lines == len(lines) && f.content == content {
		return
	}

	var frame strings.Builder
	if len(lines) > 0 && !f.cursorHidden {
		frame.WriteString("\x1b[?25l")
		f.cursorHidden = true
	}
	if f.lines > 0 {
		fmt.Fprintf(&frame, "\x1b[%dA", f.lines)
	}
	for _, line := range rendered {
		fmt.Fprintf(&frame, "\r\x1b[2K%s\n", line)
	}
	for i := len(lines); i < f.lines; i++ {
		frame.WriteString("\r\x1b[2K\n")
	}
	if f.lines > len(lines) {
		fmt.Fprintf(&frame, "\x1b[%dA", f.lines-len(lines))
	}
	if len(lines) == 0 && f.cursorHidden {
		frame.WriteString("\x1b[?25h")
		f.cursorHidden = false
	}
	fmt.Fprint(f.printer.Output, frame.String())
	f.lines = len(lines)
	f.content = content
}

func (f *liveFrame) clear() {
	f.draw(nil)
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
	closed    bool
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
	if p.closed {
		return
	}
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
		p.closed = true
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
	closed  bool
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
	if s.closed {
		return
	}
	s.tools[info.Name] = info
	s.renderLocked()
}

func (s *Scan) SetPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
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
	s.closed = true
	s.frame.clear()
}
