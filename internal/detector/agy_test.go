package detector

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/runner"
	"agentup/internal/testutil"
)

func TestAgyManifestPlatform(t *testing.T) {
	for _, tt := range []struct {
		os, arch, want string
		musl           bool
	}{
		{"windows", "amd64", "windows_amd64", false},
		{"windows", "arm64", "windows_arm64", false},
		{"darwin", "amd64", "darwin_amd64", false},
		{"darwin", "arm64", "darwin_arm64", false},
		{"linux", "amd64", "linux_amd64", false},
		{"linux", "arm64", "linux_arm64", false},
		{"linux", "amd64", "linux_amd64_musl", true},
		{"linux", "arm64", "linux_arm64_musl", true},
		{"darwin", "arm64", "darwin_arm64", true},
		{"windows", "386", "", false},
		{"linux", "riscv64", "", false},
		{"freebsd", "amd64", "", false},
	} {
		t.Run(tt.os+"/"+tt.arch+"/"+tt.want, func(t *testing.T) {
			got, err := agyManifestPlatform(tt.os, tt.arch, tt.musl)
			if got != tt.want || (err != nil) != (tt.want == "") {
				t.Fatalf("got platform %q, error %v; want %q", got, err, tt.want)
			}
		})
	}
}

type agyTransport func(*http.Request) (*http.Response, error)

func (f agyTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCheckLatestAgyManifest(t *testing.T) {
	for _, tt := range []struct {
		name, current, body, latest, message string
		status                               int
		upgrade                              bool
	}{
		{"update", "1.3.4", `{"version":"1.3.5","url":"https://example.invalid/agy.exe","sha512":"unused"}`, "1.3.5", "", 200, true},
		{"current", "1.3.5", `{"version":"1.3.5"}`, "1.3.5", "", 200, false},
		{"newer local version", "1.3.6", `{"version":"1.3.5"}`, "1.3.5", "", 200, false},
		{"v prefix", "v1.3.4", `{"version":"v1.3.5"}`, "1.3.5", "", 200, true},
		{"unknown local version", "unknown", `{"version":"1.3.5"}`, "1.3.5", "", 200, false},
		{"missing version", "1.3.4", `{"url":"https://example.invalid/1.3.5/agy.exe"}`, "", "recognizable version", 200, false},
		{"empty version", "1.3.4", `{"version":""}`, "", "recognizable version", 200, false},
		{"invalid version", "1.3.4", `{"version":"latest"}`, "", "recognizable version", 200, false},
		{"embedded version", "1.3.4", `{"version":"release 1.3.5"}`, "", "recognizable version", 200, false},
		{"wrong version type", "1.3.4", `{"version":123}`, "", "invalid Agy version response", 200, false},
		{"null manifest", "1.3.4", `null`, "", "recognizable version", 200, false},
		{"malformed JSON", "1.3.4", `<html>proxy error</html>`, "", "invalid Agy version response", 200, false},
		{"trailing JSON", "1.3.4", `{"version":"1.3.5"} {}`, "", "invalid Agy version response", 200, false},
		{"not found", "1.3.4", `{"version":"1.3.5"}`, "", "HTTP 404", 404, false},
		{"server error", "1.3.4", `unavailable`, "", "HTTP 503", 503, false},
		{"oversized response", "1.3.4", `{"version":"1.3.5"}` + strings.Repeat(" ", 1<<20), "", "too large", 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mr := runner.NewMockRunner()
			d := New(mr, winTestPlatform(), config.DefaultTools())
			requests := 0
			d.httpClient = &http.Client{Transport: agyTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				platform, err := agyManifestPlatform(runtime.GOOS, runtime.GOARCH, false)
				if err != nil {
					t.Fatal(err)
				}
				// musl uses a separate manifest, but never a different OS or architecture.
				want := "/manifests/" + platform
				if req.URL.Scheme != "https" ||
					req.URL.Host != "antigravity-cli-auto-updater-974169037036.us-central1.run.app" ||
					(req.URL.Path != want+".json" && (runtime.GOOS != "linux" || req.URL.Path != want+"_musl.json")) ||
					req.Method != http.MethodGet || req.Header.Get("Accept") != "application/json" {
					t.Fatalf("unexpected manifest request: %s %s, headers %v", req.Method, req.URL, req.Header)
				}
				return &http.Response{
					StatusCode: tt.status,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			})}
			info := d.CheckLatest(model.ToolInfo{
				Name: model.ToolAgy, Installed: true, Version: tt.current, InstallMethod: model.InstallMethodSelfUpdate,
				LatestVersion: "stale", UpgradeAvailable: true, LatestCheck: model.CheckOK, CheckMessage: "stale",
			})
			want := model.CheckOK
			if tt.message != "" {
				want = model.CheckFailed
			}
			if info.LatestCheck != want || info.LatestVersion != tt.latest ||
				info.UpgradeAvailable != tt.upgrade || !strings.Contains(info.CheckMessage, tt.message) {
				t.Fatalf("wrong manifest result: %+v", info)
			}
			if tt.message == "" && info.CheckMessage != "" {
				t.Fatalf("successful lookup kept a stale error: %+v", info)
			}
			if requests != 1 || len(mr.Calls) != 0 {
				t.Fatalf("lookup must be one HTTP request with no external commands: requests=%d, commands=%v", requests, mr.Calls)
			}
		})
	}
}

type agyBody struct {
	io.Reader
	closed bool
}

func (b *agyBody) Close() error {
	b.closed = true
	return nil
}

func TestAgyManifestReadAndClose(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		reader io.Reader
		want   string
	}{
		{"success", 200, strings.NewReader(`{"version":"1.3.5"}`), ""},
		{"HTTP error", 503, strings.NewReader("unavailable"), "HTTP 503"},
		{"read error", 200, iotest.ErrReader(io.ErrUnexpectedEOF), "read Agy version"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := &agyBody{Reader: tt.reader}
			d := New(runner.NewMockRunner(), winTestPlatform(), config.DefaultTools())
			d.httpClient = &http.Client{Transport: agyTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Body: body, Header: http.Header{}}, nil
			})}
			_, err := d.checkAgyLatestVersion()
			if (err == nil) != (tt.want == "") || (err != nil && !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("unexpected lookup error: %v", err)
			}
			if !body.closed {
				t.Fatal("manifest response body was not closed")
			}
		})
	}
}

func TestAgyManifestTransportFailure(t *testing.T) {
	d := New(runner.NewMockRunner(), winTestPlatform(), config.DefaultTools())
	d.httpClient = &http.Client{Transport: agyTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unreachable")
	})}
	info := d.CheckLatest(model.ToolInfo{
		Name: model.ToolAgy, Installed: true, Version: "1.3.4", InstallMethod: model.InstallMethodSelfUpdate,
	})
	if info.LatestCheck != model.CheckFailed || !strings.Contains(info.CheckMessage, "network unreachable") ||
		info.LatestVersion != "" || info.UpgradeAvailable {
		t.Fatalf("network failure was not reported: %+v", info)
	}
}

func TestAgyManifestCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		name := "cancellation"
		if timeout {
			name = "timeout"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d := New(&runner.DefaultRunner{Context: ctx}, winTestPlatform(), config.DefaultTools())
			d.httpClient.Transport = agyTransport(func(req *http.Request) (*http.Response, error) {
				<-req.Context().Done()
				return nil, req.Context().Err()
			})
			want := context.Canceled
			if timeout {
				d.httpClient.Timeout = 10 * time.Millisecond
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			_, err := d.checkAgyLatestVersion()
			if !errors.Is(err, want) {
				t.Fatalf("lookup did not honor %s: %v", name, err)
			}
		})
	}
}

func TestAgyManifestSkippedOfflineOrNotInstalled(t *testing.T) {
	calls := testutil.StubAgyManifest(t, "1.3.5")
	mr := runner.NewMockRunner()
	mr.SetLookPath("agy", "/mock/agy", nil)
	mr.SetResult("/mock/agy", []string{"--version"}, runner.MockResult{Stdout: "1.3.4"})
	d := NewWithoutLatestCheck(mr, winTestPlatform(), config.DefaultTools())
	info, err := d.Detect(model.ToolAgy)
	if err != nil || info.LatestCheck != model.CheckOffline || info.LatestVersion != "" || info.UpgradeAvailable {
		t.Fatalf("offline detection queried latest: %+v, %v", info, err)
	}
	info = d.CheckLatest(model.ToolInfo{Name: model.ToolAgy})
	if info.LatestCheck != model.CheckUnavailable || info.LatestVersion != "" || info.UpgradeAvailable {
		t.Fatalf("uninstalled Agy queried latest: %+v", info)
	}
	if calls.Load() != 0 {
		t.Fatalf("offline or uninstalled Agy made %d HTTP requests", calls.Load())
	}
}
