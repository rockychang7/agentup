package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Stop re-discovers the confirmed installation, never accepting process IDs or
// executable paths from a client. Stopping does not authorize an upgrade.
func (m *Manager) Stop(confirmed Installation) error {
	current, err := m.Scan(true).Find(confirmed.ID)
	if err != nil {
		return err
	}
	if !sameInstallation(confirmed, current) {
		return errors.New("installation changed; refresh before quitting the app")
	}
	return m.stopApp(current)
}

func sameInstallation(a, b Installation) bool {
	return b.Installed && a.ID == b.ID && a.ProductID == b.ProductID &&
		a.Path == b.Path && a.PackageID == b.PackageID && a.Scope == b.Scope
}

func (m *Manager) stopApp(info Installation) error {
	if info.Running == "stopped" {
		return nil
	}
	if info.Running != "running" || info.Path == "" {
		return errors.New("cannot verify which app processes to stop")
	}
	switch m.os {
	case "windows":
		shell, err := m.runner.LookPath("powershell.exe")
		if err != nil {
			return err
		}
		// Match the exact executable or installation directory, not a process name.
		target := strings.ReplaceAll(info.Path, "'", "''")
		script := "$ErrorActionPreference = 'Stop'; $target = '" + target + "'.TrimEnd('\\'); " +
			"$session = (Get-Process -Id $PID).SessionId; " +
			"Get-Process | Where-Object { $_.SessionId -eq $session -and $_.Path -and " +
			"($_.Path.Equals($target, [StringComparison]::OrdinalIgnoreCase) -or " +
			"$_.Path.StartsWith($target + '\\', [StringComparison]::OrdinalIgnoreCase)) } | " +
			"Stop-Process -Force -ErrorAction Stop"
		if _, err := m.run(shell, "-NoProfile", "-NonInteractive", "-Command", script); err != nil {
			return err
		}
	case "darwin":
		out, err := m.run("ps", "-axo", "pid=,comm=")
		if err != nil {
			return err
		}
		prefix := filepath.Clean(info.Path) + "/Contents/"
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			exe := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
			if strings.HasPrefix(exe, prefix) {
				if err := m.terminate(fields[0]); err != nil {
					return err
				}
			}
		}
	case "linux":
		owned := map[string]bool{}
		switch info.Source {
		case "flatpak":
			flatpak, err := m.runner.LookPath("flatpak")
			if err != nil {
				return err
			}
			if _, err := m.run(flatpak, "kill", info.PackageID); err != nil {
				return err
			}
		case "snap":
			if err := m.terminateWhere(func(exe string) bool { return strings.HasPrefix(exe, info.Path+"/") }); err != nil {
				return err
			}
		case "appimage":
			pids, err := m.appImagePIDs(info.Path)
			if err != nil {
				return err
			}
			for _, pid := range pids {
				if err := m.terminate(pid); err != nil {
					return err
				}
			}
		case "apt":
			list, err := m.run("dpkg-query", "-L", info.PackageID)
			if err != nil {
				return err
			}
			owned, _ = packageFiles(list)
		case "rpm":
			list, err := m.run("rpm", "-ql", info.PackageID)
			if err != nil {
				return err
			}
			owned, _ = packageFiles(list)
		default:
			return errors.New("this installation does not support quitting from AgentUp")
		}
		if len(owned) != 0 {
			if err := m.terminateWhere(func(exe string) bool { return owned[exe] }); err != nil {
				return err
			}
		}
	default:
		return errors.New("quitting apps is not supported on this operating system")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		current, err := m.Scan(true).Find(info.ID)
		if err == nil && sameInstallation(info, current) && current.Running == "stopped" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the app has not exited; quit it manually and refresh")
		}
		select {
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-time.After(m.poll):
		}
	}
}

// terminateWhere stops the current user's processes whose executable matches.
func (m *Manager) terminateWhere(match func(string) bool) error {
	entries, err := os.ReadDir(m.procRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		exe, err := os.Readlink(filepath.Join(m.procRoot, entry.Name(), "exe"))
		if err == nil && match(strings.TrimSuffix(exe, " (deleted)")) {
			if err := m.terminate(entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) terminate(value string) error {
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 1 || pid == os.Getpid() {
		return fmt.Errorf("refusing invalid app process ID %q", value)
	}
	_, err = m.run("kill", "-TERM", strconv.Itoa(pid))
	return err
}
