package desktop

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"agentup/internal/model"
)

// Only these hosts are contacted. Local app update configs are hints, never sources.
var trustedHosts = map[string]bool{"opencode.ai": true, "formulae.brew.sh": true}

const (
	openCodeManifest  = "https://opencode.ai/update/api/latest/desktop/opencode/"
	openCodePublisher = "Anomaly Innovations, Inc"
	maxInstallerSize  = 1 << 30
)

func trustedURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !trustedHosts[parsed.Hostname()] || parsed.User != nil {
		return nil, fmt.Errorf("untrusted download location: %s", raw)
	}
	return parsed, nil
}

// transport is replaced only by tests.
var transport http.RoundTripper

func trustedClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		_, err := trustedURL(req.URL.String())
		return err
	}}
}

func get(ctx context.Context, raw string, timeout time.Duration) (*http.Response, error) {
	if _, err := trustedURL(raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "agentup")
	resp, err := trustedClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s returned HTTP %d", raw, resp.StatusCode)
	}
	return resp, nil
}

func fetchMetadata(ctx context.Context, raw string, limit int64) ([]byte, error) {
	resp, err := get(ctx, raw, 20*time.Second)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s returned more metadata than expected", raw)
	}
	return data, nil
}

var safeFileName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// downloadAsset writes the installer only after its size and SHA-512 match the plan.
func downloadAsset(ctx context.Context, asset Asset, dir string) (string, error) {
	parsed, err := trustedURL(asset.URL)
	if err != nil {
		return "", err
	}
	name := path.Base(parsed.Path)
	if !safeFileName.MatchString(name) || asset.Size <= 0 || asset.Size > maxInstallerSize || asset.SHA512 == "" {
		return "", errors.New("installer metadata is incomplete")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	resp, err := get(ctx, asset.URL, 60*time.Minute)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	target := filepath.Join(dir, name)
	partial, err := os.OpenFile(target+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hash := sha512.New()
	written, copyErr := io.Copy(io.MultiWriter(partial, hash), io.LimitReader(resp.Body, asset.Size+1))
	closeErr := partial.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(target + ".part")
		return "", errors.Join(copyErr, closeErr)
	}
	if written != asset.Size || base64.StdEncoding.EncodeToString(hash.Sum(nil)) != asset.SHA512 {
		os.Remove(target + ".part")
		return "", errors.New("downloaded installer does not match the size and SHA-512 in the official manifest")
	}
	return target, os.Rename(target+".part", target)
}

type electronManifest struct {
	Version string
	Files   []Asset
}

// parseElectronManifest reads the flat electron-builder latest*.yml format.
func parseElectronManifest(data []byte) (electronManifest, error) {
	var manifest electronManifest
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		key, value, ok := strings.Cut(strings.TrimPrefix(trimmed, "- "), ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `'"`)
		if strings.HasPrefix(trimmed, "- ") {
			manifest.Files = append(manifest.Files, Asset{})
		}
		indented := line != trimmed
		switch {
		case !indented && key == "version":
			manifest.Version = value
		case indented && len(manifest.Files) > 0:
			file := &manifest.Files[len(manifest.Files)-1]
			switch key {
			case "url":
				file.URL = value
			case "sha512":
				file.SHA512 = value
			case "size":
				file.Size, _ = strconv.ParseInt(value, 10, 64)
			}
		}
	}
	if !numericVersion.MatchString(manifest.Version) {
		return manifest, errors.New("update manifest has no valid version")
	}
	return manifest, scanner.Err()
}

var openCodeAssets = map[string]string{
	"windows/amd64": "-win-x64.exe", "windows/arm64": "-win-arm64.exe",
}

// vendorCheck reads OpenCode's official update manifest. On Windows it also offers
// the matching signed installer; elsewhere it only reports the candidate version.
func (m *Manager) vendorCheck(info *Installation, file string) {
	data, err := m.fetch(m.ctx, openCodeManifest+file, 256<<10)
	var manifest electronManifest
	if err == nil {
		manifest, err = parseElectronManifest(data)
	}
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, "OpenCode update manifest: "+err.Error()
		return
	}
	info.LatestSource = LatestVendor
	candidate(info, manifest.Version)
	if m.os != "windows" || info.LatestCheck != model.CheckOK || info.Reason == "release_line_change" {
		return
	}
	suffix := openCodeAssets[m.os+"/"+info.Arch]
	for _, asset := range manifest.Files {
		if suffix == "" || !strings.HasSuffix(asset.URL, suffix) {
			continue
		}
		if _, err := trustedURL(asset.URL); err != nil || asset.Size <= 0 || asset.SHA512 == "" {
			break
		}
		asset.Publisher = openCodePublisher
		info.Capability, info.Reason, info.Channel = Interactive, "", "official"
		info.spec = planSpec{asset: &asset}
		return
	}
	info.Reason = "architecture_unverified"
}

// caskReference shows Homebrew's cask version for apps installed outside Homebrew.
// It is a reference, not the vendor's own latest release.
func (m *Manager) caskReference(info *Installation, token string) {
	data, err := m.fetch(m.ctx, "https://formulae.brew.sh/api/cask/"+token+".json", 1<<20)
	var cask struct {
		Version string `json:"version"`
	}
	if err == nil {
		err = json.Unmarshal(data, &cask)
	}
	if err != nil {
		info.LatestCheck, info.CheckMessage = model.CheckFailed, "Homebrew reference version: "+err.Error()
		return
	}
	info.LatestSource = LatestReference
	candidate(info, strings.Split(cask.Version, ",")[0])
}
