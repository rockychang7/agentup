package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type httpTransport func(*http.Request) (*http.Response, error)

func (f httpTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// StubAgyManifest isolates HTTP lookups in tests that do not run in parallel.
func StubAgyManifest(t testing.TB, version string) *atomic.Int32 {
	t.Helper()
	data, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: version})
	if err != nil {
		t.Fatal(err)
	}
	calls := new(atomic.Int32)
	previous := http.DefaultTransport
	http.DefaultTransport = httpTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.URL.Scheme != "https" ||
			req.URL.Host != "antigravity-cli-auto-updater-974169037036.us-central1.run.app" ||
			!strings.HasPrefix(req.URL.Path, "/manifests/") {
			return nil, fmt.Errorf("unexpected HTTP request: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(data))),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	return calls
}
