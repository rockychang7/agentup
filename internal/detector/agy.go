package detector

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// The official installers at https://antigravity.google/cli/install.{sh,ps1}
// use these platform-specific manifests for both versions and downloads.
const agyUpdateBaseURL = "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app"

func agyManifestPlatform(goos, goarch string, musl bool) (string, error) {
	switch goos {
	case "windows", "darwin", "linux":
	default:
		return "", fmt.Errorf("Agy version lookup does not support operating system %s", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("Agy version lookup does not support architecture %s", goarch)
	}
	platform := goos + "_" + goarch
	if goos == "linux" && musl {
		platform += "_musl"
	}
	return platform, nil
}

func (d *Detector) checkAgyLatestVersion() (string, error) {
	musl := false
	if runtime.GOOS == "linux" {
		for _, library := range []string{"/lib/libc.musl-x86_64.so.1", "/lib/libc.musl-aarch64.so.1"} {
			if _, err := os.Stat(library); err == nil {
				musl = true
				break
			}
		}
	}
	platform, err := agyManifestPlatform(runtime.GOOS, runtime.GOARCH, musl)
	if err != nil {
		return "", err
	}
	endpoint := agyUpdateBaseURL + "/manifests/" + platform + ".json"
	req, err := http.NewRequestWithContext(d.context, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("Agy version lookup: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Agy version lookup: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Agy version lookup returned HTTP %d", res.StatusCode)
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("read Agy version: %w", err)
	}
	if len(data) > limit {
		return "", fmt.Errorf("Agy version response is too large")
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("invalid Agy version response: %w", err)
	}
	if !validVersion(manifest.Version) {
		return "", fmt.Errorf("Agy manifest did not return a recognizable version")
	}
	return strings.TrimPrefix(manifest.Version, "v"), nil
}
