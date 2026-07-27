package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateDownloadsVerifiesAndStagesRelease(t *testing.T) {
	assetName := "agentup_0.2.0_windows_amd64.zip"
	archiveData := makeZipArchive(t, "agentup.exe", []byte("new-agentup"))
	checksumData := checksumFile(assetName, archiveData)
	server := newReleaseServer(t, "v0.2.0", assetName, archiveData, checksumData)
	defer server.Close()

	targetPath := filepath.Join(t.TempDir(), "agentup.exe")
	if err := os.WriteFile(targetPath, []byte("old-agentup"), 0o755); err != nil {
		t.Fatalf("write target executable: %v", err)
	}
	resolvedTargetPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		t.Fatalf("resolve target executable: %v", err)
	}

	replaced := false
	updater := New("0.1.0")
	updater.apiURL = server.URL + "/latest"
	updater.client = server.Client()
	updater.goos = "windows"
	updater.goarch = "amd64"
	updater.executablePath = func() (string, error) {
		return targetPath, nil
	}
	updater.replace = func(stagedPath, actualTargetPath string) (bool, error) {
		replaced = true
		if actualTargetPath != resolvedTargetPath {
			t.Fatalf("expected target %q, got %q", resolvedTargetPath, actualTargetPath)
		}
		data, err := os.ReadFile(stagedPath)
		if err != nil {
			t.Fatalf("read staged executable: %v", err)
		}
		if string(data) != "new-agentup" {
			t.Fatalf("expected staged binary contents, got %q", data)
		}
		if err := os.Remove(stagedPath); err != nil {
			t.Fatalf("remove staged executable: %v", err)
		}
		return true, nil
	}

	result, err := updater.Update(context.Background(), false)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !replaced {
		t.Fatal("expected replacement to run")
	}
	if !result.Updated {
		t.Fatal("expected update result to report success")
	}
	if !result.PendingRestart {
		t.Fatal("expected pending restart result from replacement")
	}
	if result.LatestVersion != "v0.2.0" {
		t.Fatalf("expected latest version v0.2.0, got %q", result.LatestVersion)
	}
	if result.AssetName != assetName {
		t.Fatalf("expected asset %q, got %q", assetName, result.AssetName)
	}
}

func TestUpdateSkipsWhenAlreadyCurrent(t *testing.T) {
	assetRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			_ = json.NewEncoder(response).Encode(githubRelease{
				TagName: "v0.2.0",
				Assets: []releaseAsset{
					{Name: "agentup_0.2.0_windows_amd64.zip", BrowserDownloadURL: serverURL(request, "/asset")},
				},
			})
		default:
			assetRequests++
			http.Error(response, "asset should not be downloaded", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	updater := New("0.2.0")
	updater.apiURL = server.URL + "/latest"
	updater.client = server.Client()
	updater.goos = "windows"
	updater.goarch = "amd64"
	updater.replace = func(string, string) (bool, error) {
		t.Fatal("replacement should not run for the current version")
		return false, nil
	}

	result, err := updater.Update(context.Background(), false)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if result.Updated {
		t.Fatal("expected update to be skipped")
	}
	if result.SkippedReason != "already up to date" {
		t.Fatalf("unexpected skip reason %q", result.SkippedReason)
	}
	if assetRequests != 0 {
		t.Fatalf("expected no asset downloads, got %d", assetRequests)
	}
}

func TestUpdateRejectsDevelopmentBuildWithoutForce(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		http.Error(response, "request should not be made", http.StatusInternalServerError)
	}))
	defer server.Close()

	updater := New("dev")
	updater.apiURL = server.URL + "/latest"
	updater.client = server.Client()

	_, err := updater.Update(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "development builds") {
		t.Fatalf("expected development build error, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no release requests, got %d", requests)
	}
}

func TestUpdateRejectsChecksumMismatch(t *testing.T) {
	assetName := "agentup_0.2.0_windows_amd64.zip"
	archiveData := makeZipArchive(t, "agentup.exe", []byte("new-agentup"))
	checksumData := []byte(strings.Repeat("0", 64) + "  " + assetName + "\n")
	server := newReleaseServer(t, "v0.2.0", assetName, archiveData, checksumData)
	defer server.Close()

	updater := New("0.1.0")
	updater.apiURL = server.URL + "/latest"
	updater.client = server.Client()
	updater.goos = "windows"
	updater.goarch = "amd64"
	updater.replace = func(string, string) (bool, error) {
		t.Fatal("replacement should not run after checksum failure")
		return false, nil
	}

	_, err := updater.Update(context.Background(), false)
	if err == nil || !strings.Contains(err.Error(), "checksum verification failed") {
		t.Fatalf("expected checksum verification error, got %v", err)
	}
}

func TestExtractExecutableFromTarGz(t *testing.T) {
	archiveData := makeTarGzArchive(t, "nested/agentup", []byte("unix-agentup"))

	binaryData, err := extractExecutable("agentup_0.2.0_linux_amd64.tar.gz", archiveData, "linux")
	if err != nil {
		t.Fatalf("extract executable: %v", err)
	}
	if string(binaryData) != "unix-agentup" {
		t.Fatalf("unexpected executable contents %q", binaryData)
	}
}

func TestExpectedAssetName(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{
			name:   "windows amd64",
			tag:    "v1.2.3",
			goos:   "windows",
			goarch: "amd64",
			want:   "agentup_1.2.3_windows_amd64.zip",
		},
		{
			name:   "darwin arm64",
			tag:    "1.2.3",
			goos:   "darwin",
			goarch: "arm64",
			want:   "agentup_1.2.3_darwin_arm64.tar.gz",
		},
		{
			name:    "unsupported Windows arm64",
			tag:     "v1.2.3",
			goos:    "windows",
			goarch:  "arm64",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := expectedAssetName(test.tag, test.goos, test.goarch)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected asset name: %v", err)
			}
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestCompareVersionStrings(t *testing.T) {
	tests := []struct {
		left       string
		right      string
		want       int
		comparable bool
	}{
		{left: "v1.2.3", right: "1.2.3", want: 0, comparable: true},
		{left: "1.2.3", right: "1.3.0", want: -1, comparable: true},
		{left: "2.0.0", right: "1.9.9", want: 1, comparable: true},
		{left: "1.0.0-beta.1", right: "1.0.0-beta.2", want: -1, comparable: true},
		{left: "1.0.0-rc.1", right: "1.0.0", want: -1, comparable: true},
		{left: "dev", right: "1.0.0", want: 0, comparable: false},
	}

	for _, test := range tests {
		got, comparable := compareVersionStrings(test.left, test.right)
		if comparable != test.comparable {
			t.Fatalf("compare %q and %q: expected comparable=%v, got %v", test.left, test.right, test.comparable, comparable)
		}
		if got != test.want {
			t.Fatalf("compare %q and %q: expected %d, got %d", test.left, test.right, test.want, got)
		}
	}
}

func newReleaseServer(
	t *testing.T,
	tag string,
	assetName string,
	archiveData []byte,
	checksumData []byte,
) *httptest.Server {
	t.Helper()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/latest":
			response.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(response).Encode(githubRelease{
				TagName: tag,
				Assets: []releaseAsset{
					{Name: assetName, BrowserDownloadURL: server.URL + "/asset"},
					{Name: "checksums.txt", BrowserDownloadURL: server.URL + "/checksums"},
				},
			})
		case "/asset":
			_, _ = response.Write(archiveData)
		case "/checksums":
			_, _ = response.Write(checksumData)
		default:
			http.NotFound(response, request)
		}
	}))
	return server
}

func serverURL(request *http.Request, path string) string {
	return "http://" + request.Host + path
}

func checksumFile(assetName string, archiveData []byte) []byte {
	sum := sha256.Sum256(archiveData)
	return []byte(fmt.Sprintf("%x  %s\n", sum, assetName))
}

func makeZipArchive(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buffer.Bytes()
}

func makeTarGzArchive(t *testing.T, name string, data []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(data)),
	}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buffer.Bytes()
}
