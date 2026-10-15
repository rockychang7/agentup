package desktop

import (
	"debug/macho"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type bundleMetadata struct {
	ID         string `json:"CFBundleIdentifier"`
	Version    string `json:"CFBundleShortVersionString"`
	Build      string `json:"CFBundleVersion"`
	Executable string `json:"CFBundleExecutable"`
}

type caskMetadata struct {
	Token     string `json:"token"`
	Tap       string `json:"tap"`
	Version   string `json:"version"`
	Installed any    `json:"installed"`
	Pinned    bool   `json:"pinned"`
	Disabled  bool   `json:"disabled"`
	Artifacts []struct {
		App    []any  `json:"app"`
		Target string `json:"target"`
	} `json:"artifacts"`
}

func (m *Manager) scanMac(snapshot *Snapshot, offline bool) {
	dirs := m.appDirs
	if dirs == nil {
		dirs = []string{"/Applications", filepath.Join(m.home, "Applications")}
	}
	processes, processErr := m.run("ps", "-axo", "command=")
	var casks []caskMetadata
	brew, brewErr := m.runner.LookPath("brew")
	if !offline && brewErr == nil {
		out, err := m.run(brew, "info", "--json=v2", "--cask", "--installed")
		var data struct {
			Casks []caskMetadata `json:"casks"`
		}
		if err == nil {
			err = json.Unmarshal([]byte(out), &data)
		}
		if err != nil {
			snapshot.Issues = append(snapshot.Issues, "Cannot read Homebrew Cask ownership: "+err.Error())
		} else {
			casks = data.Casks
		}
	}
	for _, dir := range dirs {
		for _, product := range Products() {
			for _, app := range product.AppNames {
				path := filepath.Join(dir, app)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					continue
				} else if err != nil {
					snapshot.Issues = append(snapshot.Issues, err.Error())
					continue
				}
				out, err := m.run("plutil", "-convert", "json", "-o", "-", filepath.Join(path, "Contents", "Info.plist"))
				var metadata bundleMetadata
				if err == nil {
					err = json.Unmarshal([]byte(out), &metadata)
				}
				if err != nil {
					snapshot.Issues = append(snapshot.Issues, "Cannot read "+path+": "+err.Error())
					continue
				}
				if metadata.ID != product.Bundle {
					continue
				}
				version := metadata.Version
				if version == "" {
					version = metadata.Build
				}
				if product.ID == "antigravity.desktop" {
					major, err := strconv.Atoi(strings.Split(version, ".")[0])
					if err != nil || major < 2 {
						continue
					}
				}
				info := rawInstallation(product)
				info.Path, info.Version, info.PackageID = path, version, metadata.ID
				info.Source, info.Scope, info.Arch = "app-bundle", "system", "unknown"
				info.Arch = macArchitecture(path, metadata.Executable)
				if dir == filepath.Join(m.home, "Applications") {
					info.Scope = "user"
				}
				info.Running = "stopped"
				if processErr != nil {
					info.Running = "unknown"
				} else if strings.Contains(processes, path+"/Contents/") {
					info.Running = "running"
				}
				for _, cask := range casks {
					if cask.Token != product.Cask || cask.Tap != "homebrew/cask" || cask.Installed == nil {
						continue
					}
					owns := false
					for _, artifact := range cask.Artifacts {
						if len(artifact.App) != 0 && artifact.Target != "" && filepath.Clean(artifact.Target) == filepath.Clean(path) {
							owns = true
						}
					}
					if !owns {
						room, err := m.run(brew, "--caskroom", product.Cask)
						if err == nil {
							versions, _ := os.ReadDir(strings.TrimSpace(room))
							for _, entry := range versions {
								target, err := filepath.EvalSymlinks(filepath.Join(strings.TrimSpace(room), entry.Name(), app))
								if err == nil && filepath.Clean(target) == filepath.Clean(path) {
									owns = true
								}
							}
						}
					}
					if !owns {
						continue
					}
					info.Source, info.Channel = "brew-cask", "stable"
					info.Capability, info.Reason = Automatic, ""
					if info.Arch == "unknown" || (info.Arch != "universal" && info.Arch != m.arch) {
						info.Capability, info.Reason = Guided, "architecture_unverified"
					}
					if cask.Pinned || cask.Disabled {
						info.Capability, info.Reason = Blocked, "source_pinned_or_disabled"
					}
					info.LatestSource = LatestBrewCask
					candidate(&info, strings.Split(cask.Version, ",")[0])
					info.spec = planSpec{command: []string{brew, "upgrade", "--cask", product.Cask}}
				}
				if info.Source == "app-bundle" && !offline {
					m.caskReference(&info, product.Cask)
				}
				snapshot.Installations = append(snapshot.Installations, info)
			}
		}
	}
}

func macArchitecture(bundle, executable string) string {
	if executable == "" || filepath.Base(executable) != executable || strings.ContainsAny(executable, `/\`) {
		return "unknown"
	}
	path := filepath.Join(bundle, "Contents", "MacOS", executable)
	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		x64, arm := false, false
		for _, arch := range fat.Arches {
			x64 = x64 || arch.Cpu == macho.CpuAmd64
			arm = arm || arch.Cpu == macho.CpuArm64
		}
		if x64 && arm {
			return "universal"
		}
		if x64 {
			return "amd64"
		}
		if arm {
			return "arm64"
		}
	}
	file, err := macho.Open(path)
	if err != nil {
		return "unknown"
	}
	defer file.Close()
	switch file.Cpu {
	case macho.CpuAmd64:
		return "amd64"
	case macho.CpuArm64:
		return "arm64"
	default:
		return "unknown"
	}
}
