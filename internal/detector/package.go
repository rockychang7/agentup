package detector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"agentup/internal/config"
	"agentup/internal/model"
)

// Only use metadata belonging to the active executable, never a leftover global package.
func installedPackageVersion(tool config.ToolConfig, binary string) string {
	packages := []string{tool.NpmPackage}
	if tool.Name == model.ToolOpenCode {
		packages = append(packages, "@opencode/cli")
	}
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		resolved = binary
	}
	for dir, depth := filepath.Dir(resolved), 0; depth < 10; depth++ {
		for _, pkg := range packages {
			if version := packageVersionAt(tool, pkg, dir, resolved, ""); version != "" {
				return version
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Windows npm shims are files rather than symlinks. Bound reads so a native
	// executable is never loaded into memory just to inspect a launcher.
	stat, err := os.Stat(binary)
	if err != nil || stat.Size() > 64<<10 {
		return ""
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return ""
	}
	shim := strings.ReplaceAll(string(data), "\\", "/")
	for _, pkg := range packages {
		dir := filepath.Join(filepath.Dir(binary), "node_modules", filepath.FromSlash(pkg))
		if version := packageVersionAt(tool, pkg, dir, resolved, shim); version != "" {
			return version
		}
	}
	return ""
}

func packageVersionAt(tool config.ToolConfig, pkg, dir, binary, shim string) string {
	file := filepath.Join(dir, "package.json")
	stat, err := os.Stat(file)
	if err != nil || stat.Size() > 64<<10 {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var manifest struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Bin     map[string]string `json:"bin"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Name != pkg || !validVersion(manifest.Version) {
		return ""
	}
	resolved, err := config.ForVersion(tool, manifest.Version)
	if err != nil || resolved.NpmPackage != pkg {
		return ""
	}
	for _, name := range append([]string{tool.BinaryName}, tool.BinaryAliases...) {
		entry := manifest.Bin[name]
		if entry == "" {
			continue
		}
		target := filepath.Clean(filepath.Join(dir, filepath.FromSlash(entry)))
		match := target == filepath.Clean(binary)
		if runtime.GOOS == "windows" {
			match = strings.EqualFold(target, filepath.Clean(binary))
		}
		shimTarget := "/node_modules/" + pkg + "/" + strings.TrimPrefix(entry, "./")
		if match || (shim != "" && strings.Contains(shim, shimTarget)) {
			return manifest.Version
		}
	}
	return ""
}
