package detector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"agentup/internal/config"
)

func openCodeNativePath(binaryPath string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	native := path.Join(strings.ReplaceAll(home, "\\", "/"), ".opencode", "bin", "opencode")
	binaryPath = path.Clean(strings.ReplaceAll(binaryPath, "\\", "/"))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(binaryPath, native) || strings.EqualFold(binaryPath, native+".exe")
	}
	return binaryPath == native || binaryPath == native+".exe"
}

// Native installations must not require Node or query GitHub's mixed release feed.
func (d *Detector) checkOpenCodeNativeLatest(tool config.ToolConfig) (string, error) {
	endpoint := "https://registry.npmjs.org/" + url.PathEscape(tool.NpmPackage) + "/latest"
	req, err := http.NewRequestWithContext(d.context, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	res, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OpenCode version lookup: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenCode version lookup returned HTTP %d", res.StatusCode)
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("read OpenCode version: %w", err)
	}
	if len(data) > limit {
		return "", fmt.Errorf("OpenCode version response is too large")
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", fmt.Errorf("invalid OpenCode version response: %w", err)
	}
	if !validVersion(metadata.Version) {
		return "", fmt.Errorf("OpenCode registry did not return a recognizable version")
	}
	return strings.TrimPrefix(metadata.Version, "v"), nil
}
