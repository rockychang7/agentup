package desktop

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"agentup/internal/model"
	"agentup/internal/runner"
)

const debQueryFormat = "${binary:Package}\t${Version}\t${Architecture}\t${db:Status-Status}"

// Epoch is printed only when set, matching how rpm and dnf display versions.
const rpmQueryFormat = "%{NAME}\t%|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\t%{ARCH}"

var aptCandidateLine = regexp.MustCompile(`(?m)^\s*Candidate:\s+(\S+)\s*$`)

func (m *Manager) scanLinux(snapshot *Snapshot, offline bool) {
	// A system can carry both package databases; each package is reported by its own.
	if dpkg, err := m.runner.LookPath("dpkg-query"); err == nil {
		m.scanDeb(snapshot, dpkg, offline)
	}
	if _, err := m.runner.LookPath("rpm"); err == nil {
		m.scanRPM(snapshot, offline)
	}
	// Portable Antigravity is intentionally read-only. No old IDE repositories
	// or arbitrary directories are taken over by the updater.
	m.scanLinuxAntigravity(snapshot)
}

// packageFiles returns the files owned by a package and its desktop entry.
func packageFiles(list string) (map[string]bool, string) {
	files, entry := map[string]bool{}, ""
	for _, file := range strings.Split(list, "\n") {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		files[file] = true
		if entry == "" && strings.HasPrefix(file, "/usr/share/applications/") && strings.HasSuffix(file, ".desktop") {
			entry = file
		}
	}
	return files, entry
}

// linuxRunning compares each process executable with the installation's files.
func (m *Manager) linuxRunning(owned func(string) bool) string {
	entries, err := os.ReadDir(m.procRoot)
	if err != nil {
		return "unknown"
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		exe, err := os.Readlink(filepath.Join(m.procRoot, entry.Name(), "exe"))
		if err != nil {
			// Other users' processes are unreadable; desktop apps run as the current user.
			continue
		}
		if owned(strings.TrimSuffix(exe, " (deleted)")) {
			return "running"
		}
	}
	return "stopped"
}

func (m *Manager) scanDeb(snapshot *Snapshot, dpkg string, offline bool) {
	for _, product := range Products() {
		for _, pkg := range product.DebPackages {
			out, _, code, err := runner.WithTimeout(m.runner, 15*time.Second).Run(dpkg, "-W", "-f="+debQueryFormat, pkg)
			if err != nil {
				snapshot.Issues = append(snapshot.Issues, "Cannot query Debian package "+pkg+": "+err.Error())
				continue
			}
			if code != 0 {
				if code != 1 {
					snapshot.Issues = append(snapshot.Issues, "Debian package discovery failed for "+pkg)
				}
				continue
			}
			fields := strings.Split(strings.TrimSpace(out), "\t")
			if len(fields) != 4 || strings.Split(fields[0], ":")[0] != pkg {
				snapshot.Issues = append(snapshot.Issues, "Invalid Debian package metadata for "+pkg)
				continue
			}
			if fields[3] != "installed" {
				continue
			}
			list, err := m.run(dpkg, "-L", pkg)
			if err != nil {
				snapshot.Issues = append(snapshot.Issues, err.Error())
				continue
			}
			files, path := packageFiles(list)
			if path == "" {
				continue
			}
			info := rawInstallation(product)
			info.Version, info.Path, info.PackageID = fields[1], path, pkg
			info.Source, info.Scope, info.Arch, info.Channel = "apt", "system", fields[2], "unknown"
			info.Running = m.linuxRunning(func(exe string) bool { return files[exe] })
			if !offline {
				if product.ID == "opencode.desktop" {
					// OpenCode packages are downloaded, not served from an APT repository.
					m.vendorCheck(&info, linuxManifest(fields[2]))
				} else {
					m.aptCandidate(&info, pkg)
				}
			}
			snapshot.Installations = append(snapshot.Installations, info)
		}
	}
}

func linuxManifest(arch string) string {
	if arch == "arm64" || arch == "aarch64" {
		return "latest-linux-arm64.yml"
	}
	return "latest-linux.yml"
}

func (m *Manager) aptCandidate(info *Installation, pkg string) {
	apt, err := m.runner.LookPath("apt-cache")
	if err != nil {
		return
	}
	policy, err := m.run("env", "LC_ALL=C", apt, "policy", pkg)
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, err.Error()
		return
	}
	match := aptCandidateLine.FindStringSubmatch(policy)
	if len(match) != 2 || match[1] == "(none)" {
		return
	}
	info.LatestVersion, info.LatestSource = match[1], LatestAPT
	info.Capability, info.Reason = Guided, "package_manager_authorization"
	cmp, ok := m.compare("apt", info.Version, match[1])
	if !ok {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, "Cannot compare Debian package versions."
		return
	}
	info.LatestCheck, info.UpgradeAvailable = model.CheckOK, cmp < 0
	if info.UpgradeAvailable {
		info.Capability, info.Reason = Privileged, ""
		info.spec = planSpec{command: []string{"sudo", "apt-get", "install", "--only-upgrade", pkg}}
	}
}

func (m *Manager) scanRPM(snapshot *Snapshot, offline bool) {
	for _, product := range Products() {
		for _, pkg := range product.DebPackages {
			out, _, code, err := runner.WithTimeout(m.runner, 15*time.Second).Run("rpm", "-q", "--qf", rpmQueryFormat, pkg)
			if err != nil {
				snapshot.Issues = append(snapshot.Issues, err.Error())
				continue
			}
			if code != 0 {
				if code != 1 {
					snapshot.Issues = append(snapshot.Issues, "RPM package discovery failed for "+pkg)
				}
				continue
			}
			fields := strings.Split(strings.TrimSpace(out), "\t")
			if len(fields) != 3 || fields[0] != pkg {
				snapshot.Issues = append(snapshot.Issues, "Invalid RPM package metadata for "+pkg)
				continue
			}
			list, err := m.run("rpm", "-ql", pkg)
			if err != nil {
				snapshot.Issues = append(snapshot.Issues, err.Error())
				continue
			}
			files, path := packageFiles(list)
			if path == "" {
				continue
			}
			info := rawInstallation(product)
			info.Version, info.Path, info.PackageID = fields[1], path, pkg
			info.Source, info.Scope, info.Arch, info.Channel = "rpm", "system", fields[2], "unknown"
			info.Running = m.linuxRunning(func(exe string) bool { return files[exe] })
			if !offline {
				if product.ID == "opencode.desktop" {
					m.vendorCheck(&info, linuxManifest(fields[2]))
				} else {
					m.dnfCandidate(&info, pkg)
				}
			}
			snapshot.Installations = append(snapshot.Installations, info)
		}
	}
}

// dnfCandidate reads the local repository cache only; it never refreshes metadata.
func (m *Manager) dnfCandidate(info *Installation, pkg string) {
	dnf, err := m.runner.LookPath("dnf")
	if err != nil {
		return
	}
	out, err := m.run(dnf, "repoquery", "--cacheonly", "--quiet", "--latest-limit=1",
		"--queryformat", "%|EPOCH?{%{EPOCH}:}:{}|%{VERSION}-%{RELEASE}\n", pkg)
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, err.Error()
		return
	}
	latest := ""
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimPrefix(strings.TrimSpace(line), "0:"); line != "" {
			latest = line
			break
		}
	}
	if latest == "" {
		return
	}
	info.LatestVersion, info.LatestSource, info.LatestCheck = latest, LatestDNF, model.CheckOK
	info.Capability, info.Reason = Guided, "package_manager_authorization"
	info.UpgradeAvailable = rpmCompare(info.Version, latest) < 0
	if info.UpgradeAvailable {
		info.Capability, info.Reason = Privileged, ""
		info.spec = planSpec{command: []string{"sudo", "dnf", "upgrade", pkg}}
	}
}

func (m *Manager) scanLinuxAntigravity(snapshot *Snapshot) {
	product, _ := FindProduct("antigravity.desktop")
	roots := []string{filepath.Join(m.home, ".local", "share", "antigravity"), "/opt/Antigravity"}
	roots = append(roots, m.desktopEntryRoots()...)
	seen := map[string]bool{}
	for _, root := range roots {
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		// An unpacked Electron app exposes package.json; ASAR-only installations
		// remain unsupported rather than launching a GUI with --version.
		path := filepath.Join(root, "resources", "app", "package.json")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			snapshot.Issues = append(snapshot.Issues, err.Error())
			continue
		}
		metadata, ok := portableAntigravity(data)
		if !ok {
			continue
		}
		info := rawInstallation(product)
		info.Path, info.Version, info.PackageID = root, metadata.Version, metadata.Name
		info.Source, info.Scope, info.Arch = "portable", "system", "unknown"
		if strings.HasPrefix(root, m.home+string(filepath.Separator)) {
			info.Scope = "user"
		}
		prefix := root + string(filepath.Separator)
		info.Running = m.linuxRunning(func(exe string) bool { return strings.HasPrefix(exe, prefix) })
		snapshot.Installations = append(snapshot.Installations, info)
	}
}

// desktopEntryRoots finds unpacked Antigravity directories through launcher entries,
// since the app is only distributed as a tarball that users extract anywhere.
func (m *Manager) desktopEntryRoots() []string {
	roots := []string{}
	for _, dir := range m.desktopDir {
		entries, _ := filepath.Glob(filepath.Join(dir, "*.desktop"))
		for _, entry := range entries {
			exec := desktopExec(entry)
			if !filepath.IsAbs(exec) || !strings.EqualFold(strings.TrimSuffix(filepath.Base(exec), ".sh"), "antigravity") {
				continue
			}
			parent := filepath.Dir(exec)
			roots = append(roots, parent, filepath.Dir(parent))
		}
	}
	return roots
}

func desktopExec(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "Exec="); ok {
			fields := strings.Fields(value)
			if len(fields) == 0 {
				return ""
			}
			return strings.Trim(fields[0], `"`)
		}
	}
	return ""
}

type electronPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func appMajor(version string) int {
	major, _ := strconv.Atoi(strings.Split(version, ".")[0])
	return major
}

func portableAntigravity(data []byte) (electronPackage, bool) {
	var metadata electronPackage
	err := json.Unmarshal(data, &metadata)
	return metadata, err == nil && metadata.Name == "antigravity" &&
		numericVersion.MatchString(metadata.Version) && appMajor(metadata.Version) >= 2
}
