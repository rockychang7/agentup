package upgrader

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"agentup/internal/model"
)

const progressBarWidth = 24

// upgradeProgress serializes concurrent task messages and tracks completion.
type upgradeProgress struct {
	output      io.Writer
	outputMu    *sync.Mutex
	total       int
	next        int
	completed   int
	interactive bool
	lineVisible bool
	lineWidth   int
}

func newUpgradeProgress(output io.Writer, outputMu *sync.Mutex, total int) *upgradeProgress {
	return &upgradeProgress{
		output:      output,
		outputMu:    outputMu,
		total:       total,
		interactive: isInteractiveOutput(output),
	}
}

func (p *upgradeProgress) begin() {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()

	p.renderLocked("preparing")
}

// announce assigns the next display number when a task first becomes visible.
func (p *upgradeProgress) announce(format string, args ...any) {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()

	p.clearInteractiveLineLocked()
	p.next++
	fmt.Fprintf(p.output, "[%d/%d] %s\n", p.next, p.total, fmt.Sprintf(format, args...))
	if p.interactive {
		p.renderLocked("upgrades running")
	}
}

func (p *upgradeProgress) complete(result model.UpgradeResult) {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()

	p.completed++
	p.clearInteractiveLineLocked()
	p.renderLocked(fmt.Sprintf("%s: %s", result.Name, result.Status))
}

func (p *upgradeProgress) finish() {
	p.outputMu.Lock()
	defer p.outputMu.Unlock()

	if p.interactive && p.lineVisible {
		fmt.Fprintln(p.output)
		p.lineVisible = false
	}
}

func (p *upgradeProgress) renderLocked(message string) {
	percentage := 0
	if p.total > 0 {
		percentage = p.completed * 100 / p.total
	}
	filled := 0
	if p.total > 0 {
		filled = p.completed * progressBarWidth / p.total
	}
	bar := "[" + strings.Repeat("#", filled) + strings.Repeat("-", progressBarWidth-filled) + "]"
	line := fmt.Sprintf(
		"Progress %s %3d%% (%d/%d complete) | %s",
		bar,
		percentage,
		p.completed,
		p.total,
		message,
	)

	if p.interactive {
		padding := ""
		if p.lineWidth > len(line) {
			padding = strings.Repeat(" ", p.lineWidth-len(line))
		}
		fmt.Fprintf(p.output, "\r%s%s", line, padding)
		p.lineVisible = true
		p.lineWidth = len(line)
		return
	}
	fmt.Fprintln(p.output, line)
}

func (p *upgradeProgress) clearInteractiveLineLocked() {
	if !p.interactive || !p.lineVisible {
		return
	}
	fmt.Fprintf(p.output, "\r%s\r", strings.Repeat(" ", p.lineWidth))
	p.lineVisible = false
	p.lineWidth = 0
}

func isInteractiveOutput(output io.Writer) bool {
	file, ok := output.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
