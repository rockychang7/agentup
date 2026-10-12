package diagnostic

import (
	"fmt"
	"strings"
	"time"

	"agentup/internal/detector"
	"agentup/internal/model"
	"agentup/internal/platform"
	"agentup/internal/runner"
)

func Run(r runner.Runner, p platform.Platform, d *detector.Detector) model.DoctorResult {
	result := model.DoctorResult{OS: p.Name(), Managers: []model.ManagerInfo{}, Tools: []model.ToolPathInfo{}}
	for _, name := range p.SupportedManagers() {
		info := model.ManagerInfo{Name: name}
		path, err := r.LookPath(name)
		if err == nil {
			info.Path = path
			out, stderr, code, err := runner.WithTimeout(r, 10*time.Second).Run(name, "--version")
			if err == nil && code == 0 {
				info.Found = true
				info.Version = FirstLine(out)
			} else if err != nil {
				info.Error = err.Error()
			} else {
				info.Error = fmt.Sprintf("exit code %d: %s", code, strings.TrimSpace(stderr))
			}
		}
		result.Managers = append(result.Managers, info)
	}
	for _, tool := range d.DetectAllLocal(nil) {
		info := model.ToolPathInfo{Name: tool.Name, Found: tool.Installed, Path: tool.Path}
		if tool.Installed && tool.Version == "unknown" {
			info.Issue = "version could not be read; check the executable and PATH"
			info.IssueCode = "version_unreadable"
		} else if tool.Installed && !tool.UpgradeSupported {
			info.Issue = "automatic upgrade is unavailable for this install source; upgrade manually"
			info.IssueCode = "manual_upgrade"
		}
		result.Tools = append(result.Tools, info)
	}
	return result
}

func FirstLine(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "unknown"
}
