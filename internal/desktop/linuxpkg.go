package desktop

import (
	"bufio"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentup/internal/model"
	"agentup/internal/runner"
)

// Only first-party or distribution-verified packages are taken over. Community
// wrappers that reuse a product name are never matched.
var flatpakApps = map[string]string{
	"ai.opencode.opencode": "opencode.desktop",
}

type snapApp struct{ product, publisher string }

var snapApps = map[string]snapApp{
	// Packaged by Canonical (publisher "ubuntu", verified) from Google's release.
	"antigravity": {"antigravity.desktop", "ubuntu"},
}

// scanFlatpak reads user and system Flatpak installations; updates come from the
// same remote they were installed from and need no password for the active user.
func (m *Manager) scanFlatpak(snapshot *Snapshot, offline bool) {
	flatpak, err := m.runner.LookPath("flatpak")
	if err != nil {
		return
	}
	out, err := m.run(flatpak, "list", "--app", "--columns=application,version,installation,arch,origin")
	if err != nil {
		snapshot.Issues = append(snapshot.Issues, "Cannot list Flatpak apps: "+err.Error())
		return
	}
	running := map[string]bool{}
	ps, psErr := m.run(flatpak, "ps", "--columns=application")
	for _, line := range strings.Split(ps, "\n") {
		running[strings.TrimSpace(line)] = true
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) != 5 {
			continue
		}
		app, version, scope, arch, origin := fields[0], fields[1], fields[2], fields[3], fields[4]
		id, ok := flatpakApps[app]
		if !ok || (scope != "user" && scope != "system") {
			continue
		}
		product, _ := FindProduct(id)
		info := rawInstallation(product)
		info.Version, info.PackageID, info.Source, info.Channel = version, app, "flatpak", origin
		info.Scope, info.Arch = scope, linuxArch(arch)
		// The deploy directory changes with every update; the app directory does not.
		info.Path = filepath.Join("/var/lib/flatpak/app", app)
		if scope == "user" {
			info.Path = filepath.Join(m.home, ".local", "share", "flatpak", "app", app)
		}
		info.Running = "stopped"
		if psErr != nil {
			info.Running = "unknown"
		} else if running[app] {
			info.Running = "running"
		}
		if !offline {
			m.flatpakCandidate(&info, flatpak)
		}
		snapshot.Installations = append(snapshot.Installations, info)
	}
}

func (m *Manager) flatpakCandidate(info *Installation, flatpak string) {
	if !numericVersion.MatchString(info.Version) {
		info.LatestCheck, info.Reason = model.CheckUnavailable, "version_uncomparable"
		return
	}
	out, err := m.run(flatpak, "remote-ls", "--"+info.Scope, "--app", "--updates", "--columns=application,version", info.Channel)
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, "Flatpak update check failed: "+err.Error()
		return
	}
	latest := info.Version
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) == 2 && fields[0] == info.PackageID && fields[1] != "" {
			latest = fields[1]
		}
	}
	info.LatestSource, info.Capability, info.Reason = LatestFlatpak, Automatic, ""
	candidate(info, latest)
	info.spec = planSpec{command: []string{flatpak, "update", "--" + info.Scope, "-y", "--noninteractive", info.PackageID}}
}

// scanSnap reads snaps by exact name and verified publisher. A refresh asks snapd,
// which shows the desktop's own authorization prompt when one is needed.
func (m *Manager) scanSnap(snapshot *Snapshot, offline bool) {
	snap, err := m.runner.LookPath("snap")
	if err != nil {
		return
	}
	for name, want := range snapApps {
		out, _, code, err := runner.WithTimeout(m.runner, 15*time.Second).Run(snap, "list", name)
		if err != nil {
			snapshot.Issues = append(snapshot.Issues, "Cannot query snap "+name+": "+err.Error())
			continue
		}
		if code != 0 {
			continue
		}
		row, ok := snapRow(out, name)
		// Columns: Name Version Rev Tracking Publisher Notes
		if !ok || len(row) < 5 || snapPublisher(row[4]) != want.publisher {
			continue
		}
		product, _ := FindProduct(want.product)
		info := rawInstallation(product)
		info.Version, info.PackageID, info.Source, info.Channel = row[1], name, "snap", row[3]
		info.Path, info.Scope, info.Arch = "/snap/"+name, "system", linuxArch(m.arch)
		prefix := info.Path + "/"
		info.Running = m.linuxRunning(func(exe string) bool { return strings.HasPrefix(exe, prefix) })
		if !offline {
			m.snapCandidate(&info, snap)
		}
		snapshot.Installations = append(snapshot.Installations, info)
	}
}

func (m *Manager) snapCandidate(info *Installation, snap string) {
	out, stderr, code, err := runner.WithTimeout(m.runner, 60*time.Second).Run(snap, "refresh", "--list")
	if err != nil || code != 0 {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, strings.TrimSpace("Snap update check failed: "+stderr)
		if err != nil {
			info.CheckMessage += " " + err.Error()
		}
		return
	}
	latest := info.Version
	if row, ok := snapRow(out, info.PackageID); ok && len(row) > 1 {
		latest = row[1]
	}
	info.LatestSource, info.Capability, info.Reason = LatestSnap, Automatic, ""
	candidate(info, latest)
	info.spec = planSpec{command: []string{snap, "refresh", info.PackageID}, admin: true}
}

func snapRow(out, name string) ([]string, bool) {
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == name {
			return fields, true
		}
	}
	return nil, false
}

// snapPublisher strips the verification mark; an unverified publisher never matches.
func snapPublisher(value string) string {
	for _, mark := range []string{"✓", "**", "✪", "*"} {
		if name, ok := strings.CutSuffix(value, mark); ok {
			return name
		}
	}
	return ""
}

func linuxArch(value string) string {
	switch value {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return "unknown"
}

// appImageDirs are the places AppImage managers and users keep AppImages. Launcher
// entries that point at an AppImage are followed as well.
func (m *Manager) appImageDirs() []string {
	dirs := []string{
		filepath.Join(m.home, "Applications"), filepath.Join(m.home, "AppImages"),
		filepath.Join(m.home, ".local", "bin"), filepath.Join(m.home, "Downloads"), "/opt",
	}
	for _, dir := range m.desktopDir {
		entries, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
		for _, entry := range entries {
			if exec := desktopExec(entry); filepath.IsAbs(exec) && strings.HasSuffix(strings.ToLower(exec), ".appimage") {
				dirs = append(dirs, exec)
			}
		}
	}
	return dirs
}

// scanAppImages finds official OpenCode AppImages. The file name only selects
// candidates; the product is confirmed from the desktop entry inside the image.
func (m *Manager) scanAppImages(snapshot *Snapshot, offline bool) {
	product, _ := FindProduct("opencode.desktop")
	seen := map[string]bool{}
	for _, root := range m.appImageDirs() {
		files := []string{root}
		if stat, err := os.Stat(root); err != nil {
			continue
		} else if stat.IsDir() {
			files, _ = filepath.Glob(filepath.Join(root, "*"))
		}
		for _, file := range files {
			name := strings.ToLower(filepath.Base(file))
			if seen[file] || !strings.HasSuffix(name, ".appimage") || !strings.Contains(name, "opencode") {
				continue
			}
			seen[file] = true
			stat, err := os.Stat(file)
			if err != nil || !stat.Mode().IsRegular() {
				continue
			}
			meta, err := m.appImageMeta(file, stat)
			if err != nil {
				snapshot.Issues = append(snapshot.Issues, "Cannot read AppImage "+file+": "+err.Error())
				continue
			}
			if meta.Name != "OpenCode" || !numericVersion.MatchString(meta.Version) {
				continue
			}
			info := rawInstallation(product)
			info.Version, info.Path, info.PackageID, info.Source = meta.Version, file, filepath.Base(file), "appimage"
			info.Scope, info.Arch, info.Channel = "system", elfArch(file), "official"
			if strings.HasPrefix(file, m.home+string(filepath.Separator)) {
				info.Scope = "user"
			}
			info.Running = m.appImageRunning(file)
			if !offline {
				m.vendorCheck(&info, linuxManifest(info.Arch))
			}
			snapshot.Installations = append(snapshot.Installations, info)
		}
	}
}

type appImageMeta struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// appImageMeta reads Name and X-AppImage-Version from the embedded desktop entry.
// Extraction runs only the AppImage runtime, never the app; results are cached by
// size and modification time so a scan does not unpack the same file again.
func (m *Manager) appImageMeta(file string, stat os.FileInfo) (appImageMeta, error) {
	cache := ""
	if m.cacheDir != "" {
		cache = filepath.Join(m.cacheDir, "appimage-"+digest(file)+".json")
	}
	var meta appImageMeta
	if data, err := os.ReadFile(cache); cache != "" && err == nil && json.Unmarshal(data, &meta) == nil &&
		meta.Size == stat.Size() && meta.ModTime.Equal(stat.ModTime()) {
		return meta, nil
	}
	dir, err := os.MkdirTemp("", "agentup-appimage-")
	if err != nil {
		return meta, err
	}
	defer os.RemoveAll(dir)
	sh, err := m.runner.LookPath("sh")
	if err != nil {
		return meta, err
	}
	if _, err := m.run(sh, "-c", `cd "$1" && exec "$2" --appimage-extract '*.desktop' >/dev/null`, "sh", dir, file); err != nil {
		return meta, err
	}
	entries, _ := filepath.Glob(filepath.Join(dir, "squashfs-root", "*.desktop"))
	if len(entries) == 0 {
		return meta, errors.New("no desktop entry inside the AppImage")
	}
	meta.Name, meta.Version = desktopValue(entries[0], "Name"), desktopValue(entries[0], "X-AppImage-Version")
	meta.Size, meta.ModTime = stat.Size(), stat.ModTime()
	if data, err := json.Marshal(meta); cache != "" && err == nil && os.MkdirAll(m.cacheDir, 0o700) == nil {
		_ = os.WriteFile(cache, data, 0o600)
	}
	return meta, nil
}

func desktopValue(path, key string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	section := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			section = line
			continue
		}
		if value, ok := strings.CutPrefix(line, key+"="); ok && section == "[Desktop Entry]" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func elfArch(path string) string {
	file, err := elf.Open(path)
	if err != nil {
		return "unknown"
	}
	defer file.Close()
	switch file.Machine {
	case elf.EM_X86_64:
		return "amd64"
	case elf.EM_AARCH64:
		return "arm64"
	}
	return "unknown"
}

// AppImages run from a temporary mount, so their processes are found through the
// APPIMAGE variable the runtime sets to the image path.
func (m *Manager) appImagePIDs(path string) ([]string, error) {
	entries, err := os.ReadDir(m.procRoot)
	if err != nil {
		return nil, err
	}
	pids := []string{}
	want := []byte("APPIMAGE=" + path + "\x00")
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.procRoot, entry.Name(), "environ"))
		if err == nil && strings.Contains(string(data)+"\x00", string(want)) {
			pids = append(pids, entry.Name())
		}
	}
	return pids, nil
}

func (m *Manager) appImageRunning(path string) string {
	pids, err := m.appImagePIDs(path)
	switch {
	case err != nil:
		return "unknown"
	case len(pids) > 0:
		return "running"
	}
	return "stopped"
}

// replaceAppImage downloads the verified official AppImage and swaps it in place,
// keeping the original file name and permissions. It returns a message on failure.
func (m *Manager) replaceAppImage(plan Plan, log func(string, string)) string {
	if m.os != "linux" || plan.Asset == nil || plan.Source != "appimage" {
		return "AppImage updates are only available for AppImage installations on Linux."
	}
	stat, err := os.Stat(plan.Path)
	if err != nil {
		return "The AppImage could not be found: " + err.Error()
	}
	log("phase", "downloading")
	dir := filepath.Join(m.cacheDir, plan.ID)
	defer os.RemoveAll(dir)
	downloaded, err := m.download(m.ctx, *plan.Asset, dir)
	if err != nil {
		if m.ctx.Err() != nil {
			return "Download cancelled. Nothing was changed."
		}
		return "Download or checksum verification failed; nothing was changed: " + err.Error()
	}
	// The user may have reopened the app while downloading.
	current, err := m.Scan(false).Find(plan.Installation)
	if err != nil || current.Plan == nil || current.Plan.ID != plan.ID || current.Running != "stopped" {
		return "The app was reopened or the installation changed while downloading. Nothing was changed; refresh and try again."
	}
	log("phase", "installing")
	if err := replaceFile(downloaded, plan.Path, stat.Mode().Perm()); err != nil {
		return "Cannot replace the AppImage; nothing was changed: " + err.Error()
	}
	log("stdout", fmt.Sprintf("Replaced %s with the verified %s release.\n", plan.Path, plan.TargetVersion))
	return ""
}

// replaceFile writes next to the target first, so a failure never leaves a broken file.
func replaceFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".agentup-*")
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr, os.Chmod(out.Name(), mode|0o100)); err != nil {
		os.Remove(out.Name())
		return err
	}
	if err := os.Rename(out.Name(), target); err != nil {
		os.Remove(out.Name())
		return err
	}
	return nil
}

// installPackage downloads the verified official .deb or .rpm and installs it with the
// planned package-manager command. It returns an outcome only when it stops early.
func (m *Manager) installPackage(plan Plan, log func(string, string)) (string, string) {
	if m.os != "linux" || plan.Asset == nil || len(plan.Command) == 0 {
		return "failed", "Package updates are only available for package installations on Linux."
	}
	sudo := plan.Command[0] == "sudo"
	interactive, ok := m.runner.(interface {
		RunInteractive(string, []string) (int, error)
	})
	if sudo && (!m.terminal || !ok) {
		return "failed", "This update needs administrator rights. Use agentup upgrade from a terminal."
	}
	log("phase", "downloading")
	dir := filepath.Join(m.cacheDir, plan.ID)
	defer os.RemoveAll(dir)
	path, err := m.download(m.ctx, *plan.Asset, dir)
	if err != nil {
		if m.ctx.Err() != nil {
			return "cancelled", "Download cancelled. Nothing was installed."
		}
		return "failed", "Download or checksum verification failed; nothing was installed: " + err.Error()
	}
	current, err := m.Scan(false).Find(plan.Installation)
	if err != nil || current.Plan == nil || current.Plan.ID != plan.ID || current.Running != "stopped" {
		return "cancelled", "The app was reopened or the installation changed while downloading. Nothing was installed; refresh and try again."
	}
	log("phase", "needs_elevation")
	args := append(append([]string(nil), plan.Command[1:]...), path)
	if sudo {
		code, err := interactive.RunInteractive(plan.Command[0], args)
		if err != nil {
			return "failed", err.Error()
		}
		if code != 0 {
			return "failed", fmt.Sprintf("The package manager exited %d; authorization may have been refused.", code)
		}
		return "", ""
	}
	streaming, ok := runner.WithTimeout(m.runner, 20*time.Minute).(interface {
		RunStreaming(string, []string, func(string, string)) (string, string, int, error)
	})
	if !ok {
		return "failed", "Streaming command output is unavailable."
	}
	_, stderr, code, err := streaming.RunStreaming(plan.Command[0], args, log)
	switch {
	case err != nil:
		return "failed", err.Error()
	case code == 126:
		return "cancelled", "The password prompt was cancelled or refused. Nothing was changed."
	case code == 127:
		return "failed", "No system password prompt could be shown. Run agentup upgrade from a terminal instead."
	case code != 0:
		return "failed", fmt.Sprintf("The package manager exited %d: %.4000s", code, strings.TrimSpace(stderr))
	}
	return "", ""
}
