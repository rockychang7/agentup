package webui

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"agentup/internal/model"
	"agentup/internal/service"
)

//go:embed all:assets
var assets embed.FS

type Server struct {
	service  *service.Service
	origin   string
	host     string
	token    string
	cookie   string
	static   http.Handler
	stopped  chan struct{}
	stopOnce sync.Once
}

func New(s *service.Service, origin string) (*Server, error) {
	root, _ := fs.Sub(assets, "assets")
	return newServer(s, origin, root)
}

func newServer(s *service.Service, origin string, root fs.FS) (*Server, error) {
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, fmt.Errorf("Web UI assets missing; run npm ci --prefix web and npm run build --prefix web")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	return &Server{service: s, origin: origin, host: strings.TrimPrefix(origin, "http://"), token: token,
		cookie: "agentup_" + token[:12], static: http.FileServer(http.FS(root)), stopped: make(chan struct{})}, nil
}

func (s *Server) URL() string { return s.origin + "/#token=" + s.token }

func (s *Server) Done() <-chan struct{} { return s.stopped }

func (s *Server) Stop(force bool) error {
	if err := s.service.Stop(force); err != nil {
		return err
	}
	s.stopOnce.Do(func() { close(s.stopped) })
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Host != s.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		s.fail(w, http.StatusForbidden, "forbidden")
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.fail(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		// Only the single application entry and build assets are public.
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		s.static.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/api/session" {
		if r.Method != http.MethodPost {
			s.fail(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		if r.Header.Get("Origin") != s.origin {
			s.fail(w, http.StatusForbidden, "forbidden")
			return
		}
		var input struct {
			Token string `json:"token"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		if !s.valid(input.Token) {
			s.fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookie, Value: s.token, Path: "/api/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		s.reply(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	cookie, err := r.Cookie(s.cookie)
	if err != nil || !s.valid(cookie.Value) {
		s.fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if r.Method == http.MethodPost && (r.Header.Get("Origin") != s.origin || !s.valid(r.Header.Get("X-AgentUp-Token"))) {
		s.fail(w, http.StatusForbidden, "forbidden")
		return
	}
	switch {
	case r.URL.Path == "/api/state" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.service.JSON())
	case r.URL.Path == "/api/events" && r.Method == http.MethodGet:
		s.events(w, r)
	case r.URL.Path == "/api/shutdown" && r.Method == http.MethodPost:
		var input struct {
			Force bool `json:"force"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		s.operation(w, s.Stop(input.Force))
	case r.URL.Path == "/api/refresh" && r.Method == http.MethodPost:
		var input struct {
			Offline bool `json:"offline"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		s.operation(w, s.service.Refresh(input.Offline))
	case r.URL.Path == "/api/doctor" && r.Method == http.MethodPost:
		var input struct{}
		if !s.decode(w, r, &input) {
			return
		}
		s.operation(w, s.service.Diagnose())
	case r.URL.Path == "/api/upgrades" && r.Method == http.MethodPost:
		var input struct {
			Tools    []model.ToolName `json:"tools"`
			Revision *uint64          `json:"revision"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		if input.Revision == nil {
			s.fail(w, http.StatusBadRequest, "invalid_request")
			return
		}
		id, err := s.service.Upgrade(input.Tools, *input.Revision)
		if err != nil {
			s.operation(w, err)
			return
		}
		s.reply(w, http.StatusAccepted, map[string]string{"id": id})
	case r.URL.Path == "/api/desktop/upgrades" && r.Method == http.MethodPost:
		var input struct {
			Plans    []string `json:"plans"`
			Revision *uint64  `json:"revision"`
		}
		if !s.decode(w, r, &input) {
			return
		}
		if input.Revision == nil {
			s.fail(w, http.StatusBadRequest, "invalid_request")
			return
		}
		id, err := s.service.UpgradeDesktops(input.Plans, *input.Revision)
		if err != nil {
			s.operation(w, err)
			return
		}
		s.reply(w, http.StatusAccepted, map[string]string{"id": id})
	default:
		s.fail(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (s *Server) valid(token string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, value any) bool {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		s.fail(w, http.StatusUnsupportedMediaType, "invalid_request")
		return false
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	body = bytes.TrimSpace(body)
	if err != nil || len(body) == 0 || body[0] != '{' {
		s.fail(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		s.fail(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		s.fail(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func (s *Server) operation(w http.ResponseWriter, err error) {
	if err != nil {
		code := http.StatusBadRequest
		if err == service.ErrBusy || err == service.ErrStale {
			code = http.StatusConflict
		}
		s.fail(w, code, err.Error())
		return
	}
	s.reply(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) fail(w http.ResponseWriter, status int, code string) {
	s.reply(w, status, map[string]string{"error": code})
}

func (s *Server) reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, http.StatusInternalServerError, "stream_unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	changes, unsubscribe := s.service.Subscribe()
	defer unsubscribe()
	send := func() bool {
		// Slow readers cannot hold a handler open indefinitely.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", s.service.JSON())
		flusher.Flush()
		return err == nil
	}
	if !send() {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	updates := time.NewTicker(100 * time.Millisecond)
	defer updates.Stop()
	dirty := false
	for {
		select {
		case <-s.stopped:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Second))
			_, _ = fmt.Fprint(w, "event: shutdown\ndata: {}\n\n")
			flusher.Flush()
			return
		case <-r.Context().Done():
			return
		case <-changes:
			dirty = true
		case <-updates.C:
			if dirty {
				if !send() {
					return
				}
				dirty = false
			}
		case <-heartbeat.C:
			if !send() {
				return
			}
		}
	}
}
