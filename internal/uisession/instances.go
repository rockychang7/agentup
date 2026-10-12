package uisession

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"agentup/internal/operation"
)

type Instance struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Version   string    `json:"version"`
	StartedAt time.Time `json:"started_at"`
	Token     string    `json:"-"`
}

type record struct {
	Instance
	Token string `json:"token"`
}

func (i Instance) Origin() string { return fmt.Sprintf("http://127.0.0.1:%d", i.Port) }
func (i Instance) URL() string    { return i.Origin() + "/#token=" + i.Token }
func lockName(port int) string    { return fmt.Sprintf("ui-port-%d", port) }

func directory() (string, error) {
	root, err := operation.Directory()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "ui")
	return dir, os.MkdirAll(dir, 0700)
}

// Register holds a port-specific OS lock; stale records are never trusted by PID alone.
func Register(launchURL, version string) (Instance, func(), error) {
	u, err := url.Parse(launchURL)
	if err != nil {
		return Instance{}, nil, err
	}
	port, err := strconv.Atoi(u.Port())
	token := strings.TrimPrefix(u.Fragment, "token=")
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || port < 1 || port > 65535 || !validToken(token) {
		return Instance{}, nil, fmt.Errorf("invalid local UI launch URL")
	}
	dir, err := directory()
	if err != nil {
		return Instance{}, nil, err
	}
	release, err := operation.Acquire(lockName(port))
	if err != nil {
		return Instance{}, nil, err
	}
	instance := Instance{PID: os.Getpid(), Port: port, Version: version, StartedAt: time.Now(), Token: token}
	path := filepath.Join(dir, strconv.Itoa(port)+".json")
	temp, err := os.CreateTemp(dir, ".instance-*")
	if err != nil {
		release()
		return Instance{}, nil, err
	}
	defer os.Remove(temp.Name())
	err = json.NewEncoder(temp).Encode(record{Instance: instance, Token: token})
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	if err != nil {
		release()
		return Instance{}, nil, err
	}
	return instance, func() {
		_ = os.Remove(path)
		release()
	}, nil
}

func validToken(token string) bool {
	value, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil && len(value) == 32
}

func List() ([]Instance, error) {
	dir, err := directory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	instances := []Instance{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		release, err := operation.Acquire(lockName(port))
		if err == nil {
			err = os.Remove(path)
			release()
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			continue
		}
		if !errors.Is(err, operation.ErrBusy) {
			return nil, err
		}
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var r record
		err = json.NewDecoder(io.LimitReader(file, 4096)).Decode(&r)
		_ = file.Close()
		if err != nil || r.Port != port || r.PID < 1 || !validToken(r.Token) {
			return nil, fmt.Errorf("invalid UI instance record: %s", path)
		}
		r.Instance.Token = r.Token
		instances = append(instances, r.Instance)
	}
	sort.Slice(instances, func(a, b int) bool { return instances[a].Port < instances[b].Port })
	return instances, nil
}

// Stop uses the instance's authenticated loopback API, never an unverified process ID.
func Stop(ctx context.Context, instance Instance, force bool) error {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:           jar,
		Transport:     &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext},
		Timeout:       5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	post := func(path string, body any) error {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, instance.Origin()+path, bytes.NewReader(data))
		if err != nil {
			return err
		}
		request.Header.Set("Origin", instance.Origin())
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-AgentUp-Token", instance.Token)
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if response.StatusCode == http.StatusConflict {
				return fmt.Errorf("an upgrade is running; wait for it to finish, or use --force to cancel it")
			}
			return fmt.Errorf("UI request rejected (HTTP %d)", response.StatusCode)
		}
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return err
	}
	if err := post("/api/session", map[string]string{"token": instance.Token}); err != nil {
		return err
	}
	if err := post("/api/shutdown", map[string]bool{"force": force}); err != nil {
		return err
	}
	// Wait for cleanup so an immediately following self-update can acquire its lock.
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		release, err := operation.Acquire(lockName(instance.Port))
		if err == nil {
			release()
			return nil
		}
		if !errors.Is(err, operation.ErrBusy) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("shutdown requested but the UI has not finished stopping")
		case <-tick.C:
		}
	}
}
