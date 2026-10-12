package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"agentup/internal/config"
	"agentup/internal/model"
	"agentup/internal/platform"
	"agentup/internal/runner"
	"agentup/internal/service"
)

func fixture(t *testing.T) *Server {
	t.Helper()
	s := service.New(context.Background(), "test", "test", runner.NewMockRunner(), platform.NewPlatform(), config.DefaultTools())
	t.Cleanup(s.Close)
	server, err := newServer(s, "http://127.0.0.1:43210", fstest.MapFS{"index.html": {Data: []byte("<html>AgentUp</html>")}})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func perform(s *Server, method, path, body string, authenticated, token bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	request.Header.Set("Origin", s.origin)
	request.Header.Set("Content-Type", "application/json")
	if authenticated {
		request.AddCookie(&http.Cookie{Name: s.cookie, Value: s.token})
	}
	if token {
		request.Header.Set("X-AgentUp-Token", s.token)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, request)
	return w
}

func TestSessionAndSecurityBoundaries(t *testing.T) {
	s := fixture(t)
	w := perform(s, "POST", "/api/session", `{"token":"`+s.token+`"}`, false, false)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/" {
		t.Fatalf("unsafe cookie: %+v", cookie)
	}
	if !strings.Contains(s.URL(), "#token=") || strings.Contains(strings.Split(s.URL(), "#")[0], s.token) {
		t.Fatal("token should only be in URL fragment")
	}
	for _, test := range []struct {
		method, path, body string
		auth, token        bool
		code               int
	}{
		{"GET", "/api/state", "", false, false, 401},
		{"GET", "/api/state", "", true, false, 200},
		{"POST", "/api/refresh", `{}`, true, false, 403},
		{"POST", "/api/shutdown", `{}`, false, true, 401},
		{"POST", "/api/shutdown", `{}`, true, false, 403},
		{"GET", "/api/shutdown", "", true, true, 405},
		{"POST", "/api/session", `{"token":"wrong"}`, false, false, 401},
		{"GET", "/api/session", "", false, false, 405},
		{"GET", "/server.go", "", false, false, 404},
		{"POST", "/", "", false, false, 405},
		{"GET", "/", "", false, false, 200},
	} {
		w := perform(s, test.method, test.path, test.body, test.auth, test.token)
		if w.Code != test.code {
			t.Errorf("%s %s: got %d want %d", test.method, test.path, w.Code, test.code)
		}
		if w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing security headers")
		}
	}
	for _, header := range []string{"Host", "Origin", "Sec-Fetch-Site"} {
		r := httptest.NewRequest("GET", s.origin+"/api/state", nil)
		r.AddCookie(cookie)
		if header == "Host" {
			r.Host = "evil.example"
		} else if header == "Origin" {
			r.Header.Set(header, "https://evil.example")
		} else {
			r.Header.Set(header, "cross-site")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("%s bypassed protection: %d", header, w.Code)
		}
	}
	r := httptest.NewRequest("POST", s.origin+"/api/session", strings.NewReader(`{"token":"`+s.token+`"}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("session accepted missing origin")
	}
}

func TestStrictRequestValidation(t *testing.T) {
	s := fixture(t)
	for _, body := range []string{`{"unknown":true}`, `{} {}`, `{"offline":"yes"}`, strings.Repeat("x", 5000), ``, `null`} {
		// null is not an operation object.
		w := perform(s, "POST", "/api/refresh", body, true, true)
		if w.Code != 400 {
			t.Errorf("%q: expected 400, got %d", body[:min(len(body), 30)], w.Code)
		}
	}
	w := perform(s, "POST", "/api/upgrades", `{"tools":["codex;whoami"],"revision":0}`, true, true)
	if w.Code != 400 {
		t.Fatal("arbitrary tool accepted")
	}
	for _, body := range []string{`{"tools":["agy"]}`, `{"tools":["agy"],"revision":null}`, `{"tools":["agy"],"revision":-1}`} {
		if w := perform(s, "POST", "/api/upgrades", body, true, true); w.Code != 400 {
			t.Fatal("upgrade accepted missing or invalid revision", body, w.Code)
		}
	}
	if w := perform(s, "POST", "/api/upgrades", `{"tools":["agy"],"revision":100}`, true, true); w.Code != 409 || !strings.Contains(w.Body.String(), "stale_preview") {
		t.Fatal("stale upgrade accepted", w.Code, w.Body)
	}
	r := httptest.NewRequest("POST", s.origin+"/api/doctor", strings.NewReader("{}"))
	r.Header.Set("Origin", s.origin)
	r.Header.Set("X-AgentUp-Token", s.token)
	r.AddCookie(&http.Cookie{Name: s.cookie, Value: s.token})
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, r)
	if recorder.Code != 415 {
		t.Fatal("missing JSON content type accepted")
	}
}

func TestSSEReconnectReturnsCurrentSnapshot(t *testing.T) {
	s := fixture(t)
	httpServer := httptest.NewServer(s)
	defer httpServer.Close()
	s.origin = httpServer.URL
	s.host = strings.TrimPrefix(httpServer.URL, "http://")
	client := &http.Client{Timeout: 3 * time.Second}
	snapshot := func() service.State {
		req, _ := http.NewRequest("GET", httpServer.URL+"/api/events", nil)
		req.AddCookie(&http.Cookie{Name: s.cookie, Value: s.token})
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal(response.StatusCode)
		}
		reader := bufio.NewReader(response.Body)
		event, _ := reader.ReadString('\n')
		data, err := reader.ReadString('\n')
		if event != "event: state\n" || err != nil {
			t.Fatal("missing snapshot", event, data, err)
		}
		var state service.State
		if err = json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	first := snapshot()
	if err := s.service.Refresh(true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for s.service.Snapshot().Scanning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	next := snapshot()
	if !next.Offline || next.Revision <= first.Revision {
		t.Fatal("reconnect replayed a stale state")
	}
}

func TestMissingAssetsReportsBuildInstructions(t *testing.T) {
	s := fixture(t)
	_, err := newServer(s.service, s.origin, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "npm ci") {
		t.Fatal("missing build should explain recovery")
	}
}

func TestShutdownIsAuthenticatedStrictAndStopsOnlyThisService(t *testing.T) {
	s := fixture(t)
	other := fixture(t)
	for _, body := range []string{`null`, `{"force":"yes"}`, `{"unknown":1}`, `{} {}`} {
		if w := perform(s, "POST", "/api/shutdown", body, true, true); w.Code != 400 {
			t.Fatal("invalid shutdown request accepted", body, w.Code)
		}
	}
	if w := perform(s, "POST", "/api/shutdown", `{}`, true, true); w.Code != 202 {
		t.Fatal("shutdown failed", w.Code, w.Body)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("shutdown signal was not sent")
	}
	select {
	case <-other.Done():
		t.Fatal("shutdown affected another instance")
	default:
	}
	if err := s.service.Refresh(false); err != service.ErrBusy {
		t.Fatal("shutdown service accepted new work", err)
	}
	if w := perform(s, "POST", "/api/shutdown", `{}`, true, true); w.Code != 202 {
		t.Fatal("repeated shutdown was not idempotent", w.Code)
	}
}

func TestShutdownSSEStopsAutomaticReconnect(t *testing.T) {
	s := fixture(t)
	if err := s.Stop(false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req := httptest.NewRequest("GET", s.origin+"/api/events", nil).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: s.cookie, Value: s.token})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "event: shutdown\ndata: {}\n\n") {
		t.Fatal("SSE did not explain intentional shutdown", w.Body)
	}
}

type shutdownRunner struct {
	*runner.MockRunner
	started chan struct{}
	resume  chan struct{}
}

func (r *shutdownRunner) Run(name string, args ...string) (string, string, int, error) {
	if name == "/mock/agy" && len(args) == 1 && args[0] == "update" {
		close(r.started)
		<-r.resume
	}
	return r.MockRunner.Run(name, args...)
}

func TestShutdownProtectsActiveUpgradeUnlessForced(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	mock := runner.NewMockRunner()
	mock.SetLookPath("agy", "/mock/agy", nil)
	mock.SetResult("/mock/agy", []string{"--version"}, runner.MockResult{Stdout: "1.0.0"})
	mock.SetResult("/mock/agy", []string{"update"}, runner.MockResult{Stdout: "done"})
	r := &shutdownRunner{MockRunner: mock, started: make(chan struct{}), resume: make(chan struct{})}
	svc := service.New(context.Background(), "test", "test", r, platform.NewPlatform(), config.DefaultTools())
	t.Cleanup(svc.Close)
	t.Cleanup(func() { close(r.resume) })
	s, err := newServer(svc, "http://127.0.0.1:43210", fstest.MapFS{"index.html": {Data: []byte("<html>AgentUp</html>")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Refresh(false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for svc.Snapshot().Scanning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := svc.Upgrade([]model.ToolName{model.ToolAgy}, svc.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade did not start")
	}
	if w := perform(s, "POST", "/api/shutdown", `{}`, true, true); w.Code != http.StatusConflict {
		t.Fatal("normal shutdown interrupted an active upgrade", w.Code, w.Body)
	}
	select {
	case <-s.Done():
		t.Fatal("rejected shutdown still stopped the server")
	default:
	}
	if w := perform(s, "POST", "/api/shutdown", `{"force":true}`, true, true); w.Code != http.StatusAccepted {
		t.Fatal("forced shutdown failed", w.Code, w.Body)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("forced shutdown did not stop the server")
	}
	if err := svc.Refresh(false); err != service.ErrBusy {
		t.Fatal("forced shutdown accepted new work", err)
	}
}
