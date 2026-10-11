package table

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"agentup/internal/model"
)

func withoutNoColor(t *testing.T) {
	t.Helper()
	old, exists := os.LookupEnv("NO_COLOR")
	_ = os.Unsetenv("NO_COLOR")
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv("NO_COLOR", old)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})
}

func sampleTools() []model.ToolInfo {
	return []model.ToolInfo{
		{Name: model.ToolCodex, Installed: true, Version: "1.2.3", LatestVersion: "1.3.0",
			UpgradeAvailable: true, InstallMethod: model.InstallMethodNpm, UpgradeSupported: true,
			LatestCheck: model.CheckOK, Path: "/very/long/install/directory/bin/codex"},
		{Name: model.ToolClaudeCode, Installed: true, Version: "2.0.0", LatestVersion: "2.0.0",
			InstallMethod: model.InstallMethodNpm, UpgradeSupported: true, LatestCheck: model.CheckOK},
		{Name: model.ToolOpenCode},
		{Name: model.ToolAgy, Installed: true, Version: "1.0.0", InstallMethod: model.InstallMethodSelfUpdate,
			UpgradeSupported: true, LatestCheck: model.CheckUnavailable},
	}
}

func TestColorModes(t *testing.T) {
	withoutNoColor(t)
	for _, mode := range []string{"auto", "never", "always"} {
		t.Run(mode, func(t *testing.T) {
			var out bytes.Buffer
			p := New(&out, Options{Color: mode})
			p.Heading("AgentUp", "")
			p.List(sampleTools())
			hasANSI := strings.Contains(out.String(), "\x1b[")
			if hasANSI != (mode == "always") {
				t.Fatalf("color mode %s produced %q", mode, out.String())
			}
		})
	}
}

func TestNoColorOverridesForcedColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	var out bytes.Buffer
	New(&out, Options{Color: "always"}).Heading("AgentUp", "")
	if strings.Contains(out.String(), "\x1b") {
		t.Fatalf("NO_COLOR must suppress styling, got %q", out.String())
	}
}

func TestListCompactAndVerbose(t *testing.T) {
	var out bytes.Buffer
	New(&out, Options{}).List(sampleTools())
	text := out.String()
	for _, want := range []string{"Agent", "Version", "Source", "Status", "1.2.3 -> 1.3.0", "Up to date", "Latest unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "/very/long") || strings.Contains(text, "Upgrade Supported") {
		t.Fatalf("default table should hide diagnostic details: %s", text)
	}
	out.Reset()
	New(&out, Options{Verbose: true, Width: 120}).List(sampleTools())
	if !strings.Contains(out.String(), "/very/long/install/directory/bin/codex") {
		t.Fatalf("verbose output should include full path: %s", out.String())
	}
}

func TestNarrowOutputFits(t *testing.T) {
	withoutNoColor(t)
	for _, width := range []int{20, 40, 80, 120} {
		var out bytes.Buffer
		p := New(&out, Options{Width: width, Verbose: true, Color: "always"})
		p.List(sampleTools())
		p.UpgradeResults([]model.UpgradeResult{{
			Name: model.ToolCodex, Status: model.UpgradeStatusFailed, Outcome: model.OutcomeFailed,
			Message: strings.Repeat("longfailure", 20), Command: []string{"npm", "install", "-g", "@openai/codex@latest"},
		}})
		for _, line := range strings.Split(out.String(), "\n") {
			if actual := lipgloss.Width(line); actual > width {
				t.Errorf("width %d: line has %d cells: %q", width, actual, line)
			}
		}
	}
}

func TestUnchangedNeverClaimsLatest(t *testing.T) {
	var out bytes.Buffer
	New(&out, Options{}).UpgradeResults([]model.UpgradeResult{
		{Name: model.ToolCodex, Status: model.UpgradeStatusSuccess, OldVersion: "1.0.0", NewVersion: "1.0.0"},
	})
	text := out.String()
	if !strings.Contains(text, "unchanged") || strings.Contains(text, "already latest") || strings.Contains(text, "up to date") {
		t.Fatalf("unchanged version is not evidence of latest: %s", text)
	}
}

func TestExternalOutputCannotInjectTerminalEscapes(t *testing.T) {
	var out bytes.Buffer
	New(&out, Options{Verbose: true}).UpgradeResults([]model.UpgradeResult{
		{Name: model.ToolCodex, Status: model.UpgradeStatusFailed, Message: "\x1b[2Jbad",
			Command: []string{"npm"}, Log: "\x1b[31munsafe\x1b[0m\x07"},
	})
	if strings.ContainsAny(out.String(), "\x1b\x07") {
		t.Fatalf("external text leaked control codes: %q", out.String())
	}
}

func TestDoctorMissingOptionalManagersNotErrors(t *testing.T) {
	var out bytes.Buffer
	New(&out, Options{}).Doctor(model.DoctorResult{
		OS: "Windows", Managers: []model.ManagerInfo{{Name: "npm", Found: true}, {Name: "scoop"}},
	})
	if !strings.Contains(out.String(), "Missing optional managers are not errors") {
		t.Fatalf("missing optional manager should not be an error: %s", out.String())
	}
}

func TestProgressPipeIsAppendOnlyAndConcurrent(t *testing.T) {
	var out bytes.Buffer
	p := NewProgress(New(&out, Options{}), []model.ToolName{model.ToolCodex, model.ToolAgy})
	var wg sync.WaitGroup
	for _, name := range []model.ToolName{model.ToolCodex, model.ToolAgy} {
		wg.Add(1)
		go func(name model.ToolName) {
			defer wg.Done()
			p.Observe(model.UpgradeEvent{Name: name, Phase: model.PhaseUpgrading})
			result := model.UpgradeResult{Name: name, Outcome: model.OutcomeUpdated}
			p.Observe(model.UpgradeEvent{Name: name, Phase: model.PhaseFinished, Result: &result})
		}(name)
	}
	wg.Wait()
	p.Close()
	if strings.ContainsAny(out.String(), "\x1b\r") {
		t.Fatalf("pipe received cursor controls: %q", out.String())
	}
	if strings.Count(out.String(), "updated") != 2 {
		t.Fatalf("missing task completion: %s", out.String())
	}
}

func TestProgressTerminalClearsBlockAndStopsTicker(t *testing.T) {
	var out bytes.Buffer
	printer := New(&out, Options{Width: 40})
	printer.Interactive = true
	p := NewProgress(printer, []model.ToolName{model.ToolCodex})
	result := model.UpgradeResult{Name: model.ToolCodex, Outcome: model.OutcomeFailed}
	p.Observe(model.UpgradeEvent{Name: model.ToolCodex, Phase: model.PhaseFinished, Result: &result})
	p.Close()
	p.Close()
	if p.frame.lines != 0 {
		t.Fatal("progress block was not cleared")
	}
	if !strings.Contains(out.String(), "\x1b[2K") || !strings.Contains(ansi.Strip(out.String()), "failed") {
		t.Fatalf("expected a redrawn task block: %q", out.String())
	}
}

func TestScanExternalVersionCannotInjectTerminalEscapes(t *testing.T) {
	var out bytes.Buffer
	printer := New(&out, Options{Width: 80})
	printer.Interactive = true
	scan := NewScan(printer, []model.ToolName{model.ToolCodex})
	scan.Update(model.ToolInfo{
		Name: model.ToolCodex, Installed: true, Version: "\x1b[2Jbad\x07\rversion",
	})
	scan.Close()
	if strings.Contains(out.String(), "\x1b[2J") || strings.Contains(out.String(), "\x07") {
		t.Fatalf("external version leaked terminal controls: %q", out.String())
	}
}
