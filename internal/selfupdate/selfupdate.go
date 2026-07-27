package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL     = "https://api.github.com/repos/rockychang7/agentup/releases/latest"
	maxAPIResponse    = 5 << 20
	maxArchiveSize    = 100 << 20
	maxChecksumSize   = 1 << 20
	maxExecutableSize = 100 << 20
)

// Result describes the outcome of an agentup self-update.
type Result struct {
	CurrentVersion string
	LatestVersion  string
	AssetName      string
	Updated        bool
	PendingRestart bool
	SkippedReason  string
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type replaceFunc func(stagedPath, targetPath string) (pendingRestart bool, err error)

// Updater downloads and installs the latest published agentup release.
type Updater struct {
	currentVersion string
	apiURL         string
	client         *http.Client
	goos           string
	goarch         string
	executablePath func() (string, error)
	replace        replaceFunc
}

// New creates an updater for the current platform.
func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		apiURL:         defaultAPIURL,
		client: &http.Client{
			Timeout: 2 * time.Minute,
		},
		goos:           runtime.GOOS,
		goarch:         runtime.GOARCH,
		executablePath: os.Executable,
		replace:        replaceExecutable,
	}
}

// Update installs the latest release unless the current version is already
// current or newer. Force bypasses that version check.
func (u *Updater) Update(ctx context.Context, force bool) (Result, error) {
	result := Result{CurrentVersion: u.currentVersion}

	if strings.EqualFold(strings.TrimSpace(u.currentVersion), "dev") && !force {
		return result, fmt.Errorf("self-update is unavailable for development builds; run `agentup update --force` to install the latest release")
	}

	release, err := u.fetchLatestRelease(ctx)
	if err != nil {
		return result, err
	}
	result.LatestVersion = release.TagName

	if _, ok := parseSemanticVersion(release.TagName); !ok {
		return result, fmt.Errorf("latest release has invalid version tag %q", release.TagName)
	}

	if comparison, comparable := compareVersionStrings(u.currentVersion, release.TagName); comparable && !force {
		switch {
		case comparison == 0:
			result.SkippedReason = "already up to date"
			return result, nil
		case comparison > 0:
			result.SkippedReason = "installed version is newer than the latest release"
			return result, nil
		}
	}

	assetName, err := expectedAssetName(release.TagName, u.goos, u.goarch)
	if err != nil {
		return result, err
	}
	result.AssetName = assetName

	archiveAsset, ok := findAsset(release.Assets, assetName)
	if !ok {
		return result, fmt.Errorf("release %s does not contain %s", release.TagName, assetName)
	}
	checksumAsset, ok := findAsset(release.Assets, "checksums.txt")
	if !ok {
		return result, fmt.Errorf("release %s does not contain checksums.txt", release.TagName)
	}

	archiveData, err := u.download(ctx, archiveAsset.BrowserDownloadURL, maxArchiveSize)
	if err != nil {
		return result, fmt.Errorf("download %s: %w", assetName, err)
	}
	checksumData, err := u.download(ctx, checksumAsset.BrowserDownloadURL, maxChecksumSize)
	if err != nil {
		return result, fmt.Errorf("download checksums.txt: %w", err)
	}
	if err := verifyChecksum(checksumData, assetName, archiveData); err != nil {
		return result, err
	}

	binaryData, err := extractExecutable(assetName, archiveData, u.goos)
	if err != nil {
		return result, err
	}

	targetPath, err := u.resolveExecutablePath()
	if err != nil {
		return result, err
	}
	stagedPath, err := stageExecutable(targetPath, binaryData)
	if err != nil {
		return result, err
	}

	pendingRestart, err := u.replace(stagedPath, targetPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		return result, fmt.Errorf("replace %s: %w", targetPath, err)
	}

	result.Updated = true
	result.PendingRestart = pendingRestart
	return result, nil
}

func (u *Updater) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	data, err := u.download(ctx, u.apiURL, maxAPIResponse)
	if err != nil {
		return githubRelease{}, fmt.Errorf("query latest GitHub release: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return githubRelease{}, fmt.Errorf("decode latest GitHub release: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubRelease{}, fmt.Errorf("latest GitHub release does not contain a version tag")
	}
	return release, nil
}

func (u *Updater) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("download URL is empty")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "agentup/"+strings.TrimSpace(u.currentVersion))

	response, err := u.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = http.StatusText(response.StatusCode)
		}
		return nil, fmt.Errorf("server returned HTTP %d: %s", response.StatusCode, detail)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func (u *Updater) resolveExecutablePath() (string, error) {
	targetPath, err := u.executablePath()
	if err != nil {
		return "", fmt.Errorf("locate current executable: %w", err)
	}
	targetPath, err = filepath.Abs(targetPath)
	if err != nil {
		return "", fmt.Errorf("resolve current executable path: %w", err)
	}
	if resolvedPath, resolveErr := filepath.EvalSymlinks(targetPath); resolveErr == nil {
		targetPath = resolvedPath
	}
	return targetPath, nil
}

func expectedAssetName(tag, goos, goarch string) (string, error) {
	switch goos {
	case "windows":
		if goarch != "amd64" {
			return "", fmt.Errorf("self-update is not supported on windows/%s", goarch)
		}
	case "darwin", "linux":
		if goarch != "amd64" && goarch != "arm64" {
			return "", fmt.Errorf("self-update is not supported on %s/%s", goos, goarch)
		}
	default:
		return "", fmt.Errorf("self-update is not supported on %s/%s", goos, goarch)
	}

	version := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "V")
	extension := "tar.gz"
	if goos == "windows" {
		extension = "zip"
	}
	return fmt.Sprintf("agentup_%s_%s_%s.%s", version, goos, goarch, extension), nil
}

func findAsset(assets []releaseAsset, name string) (releaseAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return releaseAsset{}, false
}

func verifyChecksum(checksumData []byte, assetName string, archiveData []byte) error {
	var expected string
	for _, line := range strings.Split(string(checksumData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if pathpkg.Base(name) == assetName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt does not contain %s", assetName)
	}
	if _, err := hex.DecodeString(expected); err != nil || len(expected) != sha256.Size*2 {
		return fmt.Errorf("checksums.txt contains an invalid SHA-256 value for %s", assetName)
	}

	actual := sha256.Sum256(archiveData)
	if !strings.EqualFold(expected, hex.EncodeToString(actual[:])) {
		return fmt.Errorf("checksum verification failed for %s", assetName)
	}
	return nil
}

func extractExecutable(archiveName string, archiveData []byte, goos string) ([]byte, error) {
	binaryName := "agentup"
	if goos == "windows" {
		binaryName = "agentup.exe"
	}

	switch {
	case strings.HasSuffix(archiveName, ".zip"):
		return extractFromZip(archiveData, binaryName)
	case strings.HasSuffix(archiveName, ".tar.gz"):
		return extractFromTarGz(archiveData, binaryName)
	default:
		return nil, fmt.Errorf("unsupported release archive %s", archiveName)
	}
}

func extractFromZip(archiveData []byte, binaryName string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		return nil, fmt.Errorf("open release zip: %w", err)
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || pathpkg.Base(file.Name) != binaryName {
			continue
		}
		if file.UncompressedSize64 > maxExecutableSize {
			return nil, fmt.Errorf("%s exceeds the executable size limit", binaryName)
		}

		entry, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s in release zip: %w", binaryName, err)
		}
		data, readErr := readLimited(entry, maxExecutableSize)
		closeErr := entry.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s from release zip: %w", binaryName, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close %s in release zip: %w", binaryName, closeErr)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s was not found in the release zip", binaryName)
}

func extractFromTarGz(archiveData []byte, binaryName string) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archiveData))
	if err != nil {
		return nil, fmt.Errorf("open release tar.gz: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read release tar.gz: %w", err)
		}
		if header.Typeflag != tar.TypeReg || pathpkg.Base(header.Name) != binaryName {
			continue
		}
		if header.Size > maxExecutableSize {
			return nil, fmt.Errorf("%s exceeds the executable size limit", binaryName)
		}

		data, err := readLimited(tarReader, maxExecutableSize)
		if err != nil {
			return nil, fmt.Errorf("read %s from release tar.gz: %w", binaryName, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s was not found in the release tar.gz", binaryName)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("content exceeds %d bytes", limit)
	}
	return data, nil
}

func stageExecutable(targetPath string, binaryData []byte) (string, error) {
	targetDir := filepath.Dir(targetPath)
	pattern := ".agentup-update-*"
	if strings.EqualFold(filepath.Ext(targetPath), ".exe") {
		pattern += ".exe"
	}

	staged, err := os.CreateTemp(targetDir, pattern)
	if err != nil {
		return "", fmt.Errorf("create update file next to %s: %w", targetPath, err)
	}
	stagedPath := staged.Name()
	cleanup := func() {
		_ = staged.Close()
		_ = os.Remove(stagedPath)
	}

	if _, err := staged.Write(binaryData); err != nil {
		cleanup()
		return "", fmt.Errorf("write staged executable: %w", err)
	}

	mode := os.FileMode(0o755)
	if info, err := os.Stat(targetPath); err == nil && info.Mode().Perm() != 0 {
		mode = info.Mode().Perm()
	}
	if err := staged.Chmod(mode); err != nil {
		cleanup()
		return "", fmt.Errorf("set staged executable permissions: %w", err)
	}
	if err := staged.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("flush staged executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		_ = os.Remove(stagedPath)
		return "", fmt.Errorf("close staged executable: %w", err)
	}
	return stagedPath, nil
}

var semanticVersionPattern = regexp.MustCompile(
	`^[vV]?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`,
)

type semanticVersion struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

func parseSemanticVersion(value string) (semanticVersion, bool) {
	matches := semanticVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 5 {
		return semanticVersion{}, false
	}

	major, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return semanticVersion{}, false
	}
	minor, err := strconv.ParseUint(matches[2], 10, 64)
	if err != nil {
		return semanticVersion{}, false
	}
	patch, err := strconv.ParseUint(matches[3], 10, 64)
	if err != nil {
		return semanticVersion{}, false
	}

	var prerelease []string
	if matches[4] != "" {
		prerelease = strings.Split(matches[4], ".")
	}
	return semanticVersion{
		major:      major,
		minor:      minor,
		patch:      patch,
		prerelease: prerelease,
	}, true
}

func compareVersionStrings(left, right string) (int, bool) {
	leftVersion, leftOK := parseSemanticVersion(left)
	rightVersion, rightOK := parseSemanticVersion(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	return compareSemanticVersions(leftVersion, rightVersion), true
}

func compareSemanticVersions(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{
		{left.major, right.major},
		{left.minor, right.minor},
		{left.patch, right.patch},
	} {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}

	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}

	maxLength := max(len(left.prerelease), len(right.prerelease))
	for i := 0; i < maxLength; i++ {
		if i >= len(left.prerelease) {
			return -1
		}
		if i >= len(right.prerelease) {
			return 1
		}
		if comparison := comparePrereleaseIdentifier(left.prerelease[i], right.prerelease[i]); comparison != 0 {
			return comparison
		}
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftNumeric := numericIdentifier(left)
	rightNumber, rightNumeric := numericIdentifier(right)

	switch {
	case leftNumeric && rightNumeric:
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func numericIdentifier(value string) (uint64, bool) {
	if value == "" {
		return 0, false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseUint(value, 10, 64)
	return number, err == nil
}
