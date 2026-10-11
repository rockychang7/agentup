package table

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/mod/semver"
	"golang.org/x/term"

	"agentup/internal/model"
)

type Options struct {
	Color   string
	Verbose bool
	Width   int
}

// Printer owns all formatting, including terminal capability and width decisions.
type Printer struct {
	Output      io.Writer
	Options     Options
	Interactive bool
	Width       int
	heading     lipgloss.Style
	success     lipgloss.Style
	warning     lipgloss.Style
	failure     lipgloss.Style
	muted       lipgloss.Style
}

func New(w io.Writer, opts Options) *Printer {
	if w == nil {
		w = io.Discard
	}
	width, interactive := terminalSize(w)
	if opts.Width > 0 {
		width = opts.Width
	}
	width = max(10, width)
	r := lipgloss.NewRenderer(w)
	_, noColor := os.LookupEnv("NO_COLOR")
	colored := !noColor && opts.Color != "never" &&
		(opts.Color == "always" || (interactive && os.Getenv("TERM") != "dumb"))
	r.SetColorProfile(termenv.Ascii)
	if colored {
		r.SetColorProfile(termenv.ANSI)
	}
	return &Printer{
		Output: w, Options: opts, Interactive: interactive, Width: width,
		heading: r.NewStyle().Bold(true).Foreground(lipgloss.Color("6")),
		success: r.NewStyle().Foreground(lipgloss.Color("2")),
		warning: r.NewStyle().Foreground(lipgloss.Color("3")),
		failure: r.NewStyle().Foreground(lipgloss.Color("1")),
		muted:   r.NewStyle().Foreground(lipgloss.Color("8")),
	}
}

func terminalSize(w io.Writer) (int, bool) {
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) || os.Getenv("TERM") == "dumb" {
		return 80, false
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		width = 80
	}
	return width, true
}

func clean(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(clean(s)), " ")
}

func wrap(s string, width int) string {
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func (p *Printer) Line(text string) {
	fmt.Fprintln(p.Output, wrap(clean(text), p.Width))
}

func (p *Printer) Heading(title, detail string) {
	fmt.Fprintln(p.Output, p.heading.Render(wrap(singleLine(title), p.Width)))
	if detail != "" {
		fmt.Fprintln(p.Output, p.muted.Render(wrap(singleLine(detail), p.Width)))
	}
	fmt.Fprintln(p.Output)
}

func (p *Printer) Notice(text string) {
	fmt.Fprintln(p.Output, p.muted.Render(wrap(singleLine(text), p.Width)))
}

func (p *Printer) Error(text string) {
	fmt.Fprintln(p.Output, p.failure.Render(wrap("Error: "+singleLine(text), p.Width)))
}

type row struct {
	cells []string
	tone  string
}

func (p *Printer) tone(text, tone string) string {
	switch tone {
	case "success":
		return p.success.Render(text)
	case "warning":
		return p.warning.Render(text)
	case "failure":
		return p.failure.Render(text)
	default:
		return p.muted.Render(text)
	}
}

func (p *Printer) rows(headers []string, rows []row) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = lipgloss.Width(header)
	}
	for _, row := range rows {
		for i, cell := range row.cells {
			widths[i] = max(widths[i], lipgloss.Width(singleLine(cell)))
		}
	}
	total := (len(headers) - 1) * 2
	for _, width := range widths {
		total += width
	}
	if total > p.Width {
		for _, row := range rows {
			fmt.Fprintln(p.Output, p.heading.Render(wrap(singleLine(row.cells[0]), p.Width)))
			for i, cell := range row.cells[1:] {
				text := wrap("  "+headers[i+1]+": "+singleLine(cell), p.Width)
				if i == len(row.cells)-2 {
					text = p.tone(text, row.tone)
				}
				fmt.Fprintln(p.Output, text)
			}
			fmt.Fprintln(p.Output)
		}
		return
	}
	format := func(cells []string, tone string, header bool) {
		for i, cell := range cells {
			cell = singleLine(cell)
			text := cell
			if header {
				text = p.heading.Render(cell)
			} else if i == len(cells)-1 {
				text = p.tone(cell, tone)
			}
			fmt.Fprint(p.Output, text)
			if i < len(cells)-1 {
				fmt.Fprint(p.Output, strings.Repeat(" ", widths[i]-lipgloss.Width(cell)+2))
			}
		}
		fmt.Fprintln(p.Output)
	}
	format(headers, "", true)
	fmt.Fprintln(p.Output, p.muted.Render(strings.Repeat("-", total)))
	for _, row := range rows {
		format(row.cells, row.tone, false)
	}
}

func version(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func toolStatus(t model.ToolInfo) (string, string) {
	switch {
	case !t.Installed:
		return "Not installed", ""
	case t.LatestCheck == model.CheckPending:
		return "Checking latest...", ""
	case t.Version == "" || t.Version == "unknown" || !semver.IsValid("v"+strings.TrimPrefix(t.Version, "v")):
		return "Version unreadable", "warning"
	case t.LatestCheck == model.CheckFailed:
		return "Check failed", "warning"
	case t.UpgradeAvailable && !t.UpgradeSupported:
		return "Update available (manual)", "warning"
	case t.UpgradeAvailable:
		return "Update available", "warning"
	case !t.UpgradeSupported:
		return "Manual upgrade", "warning"
	case t.LatestCheck == model.CheckOffline:
		return "Offline / not checked", ""
	case t.LatestCheck == model.CheckOK && t.LatestVersion != "":
		return "Up to date", "success"
	default:
		return "Latest unavailable", "warning"
	}
}

func (p *Printer) List(tools []model.ToolInfo) {
	rows := make([]row, 0, len(tools))
	installed, updates, failed := 0, 0, 0
	for _, t := range tools {
		v, source := "-", "-"
		if t.Installed {
			installed++
			v, source = version(t.Version), string(t.InstallMethod)
			if t.UpgradeAvailable {
				updates++
				v += " -> " + t.LatestVersion
			}
		}
		if t.LatestCheck == model.CheckFailed {
			failed++
		}
		status, tone := toolStatus(t)
		rows = append(rows, row{[]string{string(t.Name), v, source, status}, tone})
	}
	p.rows([]string{"Agent", "Version", "Source", "Status"}, rows)
	fmt.Fprintln(p.Output)
	p.Notice(fmt.Sprintf("%d installed / %d supported | %d updates | %d checks failed", installed, len(tools), updates, failed))
	if p.Options.Verbose {
		for _, t := range tools {
			if !t.Installed {
				continue
			}
			fmt.Fprintln(p.Output)
			p.Line(fmt.Sprintf("%s: %s", t.Name, t.Path))
			p.Notice(fmt.Sprintf("  Auto-upgrade: %t | Latest: %s | Check: %s", t.UpgradeSupported, version(t.LatestVersion), t.LatestCheck))
			if t.CheckMessage != "" {
				p.Notice("  " + t.CheckMessage)
			}
		}
	}
	if updates > 0 {
		fmt.Fprintln(p.Output)
		p.Line("Next: agentup upgrade")
	}
	if failed > 0 {
		p.Line("Retry: agentup list --verbose | Skip registry checks: agentup list --offline")
	}
}

func Duration(millis int64) string {
	d := time.Duration(millis) * time.Millisecond
	if d < time.Second {
		return "<1s"
	}
	return d.Round(time.Second).String()
}

func Outcome(r model.UpgradeResult) model.UpgradeOutcome {
	if r.Outcome != "" {
		return r.Outcome
	}
	if r.Status == model.UpgradeStatusFailed {
		return model.OutcomeFailed
	}
	if r.Status == model.UpgradeStatusSkipped {
		return model.OutcomeSkipped
	}
	if r.Status == model.UpgradeStatusUnverified {
		return model.OutcomeUnverified
	}
	if r.NewVersion == "" || r.NewVersion == "unknown" {
		return model.OutcomeUnverified
	}
	if r.NewVersion == r.OldVersion {
		return model.OutcomeUnchanged
	}
	return model.OutcomeUpdated
}

func (p *Printer) UpgradeResults(results []model.UpgradeResult) {
	counts := make(map[model.UpgradeOutcome]int)
	rows := make([]row, 0, len(results))
	for _, result := range results {
		outcome := Outcome(result)
		counts[outcome]++
		v := version(result.OldVersion)
		if result.NewVersion != "" && result.OldVersion != result.NewVersion {
			v += " -> " + result.NewVersion
		}
		tone := "success"
		if outcome == model.OutcomeFailed {
			tone = "failure"
		} else if outcome == model.OutcomeUnverified || outcome == model.OutcomeUnchanged {
			tone = "warning"
		} else if outcome == model.OutcomeSkipped {
			tone = ""
		}
		rows = append(rows, row{[]string{string(result.Name), v, Duration(result.DurationMillis), string(outcome)}, tone})
	}
	p.Heading("Upgrade results", "")
	p.rows([]string{"Agent", "Version", "Time", "Result"}, rows)
	fmt.Fprintln(p.Output)
	p.Notice(fmt.Sprintf("%d updated | %d unchanged | %d unverified | %d failed | %d skipped",
		counts[model.OutcomeUpdated], counts[model.OutcomeUnchanged], counts[model.OutcomeUnverified],
		counts[model.OutcomeFailed], counts[model.OutcomeSkipped]))
	for _, result := range results {
		if result.Message != "" {
			fmt.Fprintln(p.Output)
			message := singleLine(result.Message)
			if !p.Options.Verbose {
				message = ansi.Truncate(message, max(20, p.Width*2-len(result.Name)-2), "...")
			}
			p.Line(fmt.Sprintf("%s: %s", result.Name, message))
		}
		if result.Status == model.UpgradeStatusFailed || Outcome(result) == model.OutcomeUnverified {
			p.Line(fmt.Sprintf("Retry: agentup upgrade %s --verbose", result.Name))
		}
		if p.Options.Verbose && len(result.Command) > 0 {
			p.Notice("  Command: " + strings.Join(result.Command, " "))
			if result.Log != "" {
				p.Line("  Output:\n" + clean(result.Log))
			}
		}
	}
}

func (p *Printer) Doctor(result model.DoctorResult) {
	p.Heading("Environment", result.OS)
	found, missing, issues := []string{}, []string{}, []string{}
	for _, m := range result.Managers {
		if m.Found {
			found = append(found, m.Name)
		} else {
			missing = append(missing, m.Name)
			if m.Path != "" {
				issues = append(issues, fmt.Sprintf("%s is in PATH but failed its version check: %s", m.Name, m.Error))
			}
		}
	}
	installed := 0
	for _, t := range result.Tools {
		if t.Found {
			installed++
		}
		if t.Issue != "" {
			issues = append(issues, fmt.Sprintf("%s: %s", t.Name, t.Issue))
		}
	}
	p.Line(fmt.Sprintf("%d agents in PATH | %d package managers available", installed, len(found)))
	if len(found) > 0 {
		p.Line("Available: " + strings.Join(found, ", "))
	}
	fmt.Fprintln(p.Output)
	if len(issues) == 0 {
		p.Line("No detected blockers. Missing optional managers are not errors.")
	} else {
		for _, issue := range issues {
			fmt.Fprintln(p.Output, p.warning.Render(wrap(singleLine(issue), p.Width)))
		}
		p.Line("Next: agentup list --verbose")
	}
	if installed == 0 {
		p.Line("No supported agents found. Install an agent or check your PATH.")
	}
	if p.Options.Verbose {
		fmt.Fprintln(p.Output)
		p.Heading("Package managers", "Optional / not found: "+strings.Join(missing, ", "))
		for _, m := range result.Managers {
			if m.Path != "" {
				p.Line(fmt.Sprintf("%s %s | %s", m.Name, version(m.Version), m.Path))
			}
		}
		fmt.Fprintln(p.Output)
		p.Heading("Agent paths", "")
		for _, t := range result.Tools {
			p.Line(fmt.Sprintf("%s: %s", t.Name, version(t.Path)))
		}
	}
}

func PrintList(tools []model.ToolInfo)                  { PrintListTo(os.Stdout, tools) }
func PrintListTo(w io.Writer, tools []model.ToolInfo)   { New(w, Options{}).List(tools) }
func PrintUpgradeResults(results []model.UpgradeResult) { PrintUpgradeResultsTo(os.Stdout, results) }
func PrintUpgradeResultsTo(w io.Writer, results []model.UpgradeResult) {
	New(w, Options{}).UpgradeResults(results)
}
func PrintDoctor(result model.DoctorResult)                { PrintDoctorTo(os.Stdout, result) }
func PrintDoctorTo(w io.Writer, result model.DoctorResult) { New(w, Options{}).Doctor(result) }
