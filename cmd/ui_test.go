package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"agentup/internal/operation"
	"agentup/internal/uisession"
)

func setupUITest(t *testing.T) {
	t.Helper()
	setupCommandTest(t)
	oldPort, oldNoOpen := uiPort, uiNoOpen
	t.Cleanup(func() { uiPort, uiNoOpen = oldPort, oldNoOpen })
	uiPort, uiNoOpen, outputFlags.Offline = 0, true, true
}

func startTestUI(t *testing.T, count int) (uisession.Instance, <-chan error, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	done := make(chan error, 1)
	exited := make(chan struct{})
	before, _ := uisession.List()
	go func() {
		done <- runUI(cmd, nil)
		close(exited)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			t.Error("UI did not exit during cleanup")
		}
	})
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		instances, err := uisession.List()
		if err == nil && len(instances) == count {
			for _, instance := range instances {
				found := false
				for _, old := range before {
					found = found || instance.Port == old.Port
				}
				if !found {
					return instance, done, cancel
				}
			}
		}
		select {
		case err := <-done:
			t.Fatalf("UI exited during launch: %v", err)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("UI did not register")
	return uisession.Instance{}, nil, cancel
}

func waitUIExit(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UI did not stop")
	}
}

func TestMultipleUIInstancesAndTargetedShutdown(t *testing.T) {
	setupUITest(t)
	outputFlags.JSON = true
	first, firstDone, _ := startTestUI(t, 1)
	second, secondDone, _ := startTestUI(t, 2)
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runUIStop(cmd, 0, false, false); err == nil || !strings.Contains(err.Error(), "--all") {
		t.Fatal("ambiguous shutdown should not stop either instance", err)
	}
	if err := runUIStatus(cmd, nil); err != nil {
		t.Fatal(err)
	}
	var status []uisession.Instance
	if err := json.Unmarshal(out.Bytes(), &status); err != nil || len(status) != 2 || strings.Contains(out.String(), first.Token) {
		t.Fatal("invalid or unsafe status JSON", out.String(), err)
	}
	out.Reset()
	if err := runUIStop(cmd, first.Port, false, false); err != nil {
		t.Fatal(err)
	}
	waitUIExit(t, firstDone)
	if release, err := operation.Acquire("ui"); !errors.Is(err, operation.ErrBusy) {
		if release != nil {
			release()
		}
		t.Fatal("remaining instance did not block self-update", err)
	}
	instances, err := uisession.List()
	if err != nil || len(instances) != 1 || instances[0].Port != second.Port {
		t.Fatal("targeted stop affected another instance", instances, err)
	}
	out.Reset()
	if err := runUIStop(cmd, 0, true, false); err != nil {
		t.Fatal(err)
	}
	waitUIExit(t, secondDone)
	release, err := operation.Acquire("ui")
	if err != nil {
		t.Fatal("stop --all did not release the self-update lock", err)
	}
	release()
	var result struct {
		Stopped []int `json:"stopped"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || len(result.Stopped) != 1 || result.Stopped[0] != second.Port {
		t.Fatal("invalid stop JSON", out.String(), err)
	}
}

func TestSingleUIStopAndCancellationCleanup(t *testing.T) {
	setupUITest(t)
	_, done, _ := startTestUI(t, 1)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := runUIStop(cmd, 0, false, false); err != nil {
		t.Fatal(err)
	}
	waitUIExit(t, done)
	if err := runUIStop(cmd, 0, true, false); err != nil {
		t.Fatal("repeated stop should be harmless", err)
	}
	if err := runUIStop(cmd, 49100, false, false); err == nil {
		t.Fatal("unknown port was accepted")
	}
	_, canceled, cancel := startTestUI(t, 1)
	cancel()
	waitUIExit(t, canceled)
	instances, err := uisession.List()
	if err != nil || len(instances) != 0 {
		t.Fatal("terminal cancellation left a registered instance", instances, err)
	}
	release, err := operation.Acquire("ui")
	if err != nil {
		t.Fatal("terminal cancellation retained the self-update lock", err)
	}
	release()
}

func TestUIStartupGuards(t *testing.T) {
	setupUITest(t)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	release, err := operation.Acquire("ui")
	if err != nil {
		t.Fatal(err)
	}
	if err := runUI(cmd, nil); err == nil || strings.Contains(err.Error(), "locked a portion") {
		t.Fatal("update contention should have an actionable error", err)
	}
	release()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	uiPort = listener.Addr().(*net.TCPAddr).Port
	if err := runUI(cmd, nil); err == nil || !strings.Contains(err.Error(), "choose another --port") {
		t.Fatal("occupied port should have an actionable error", err)
	}
	uiPort = -1
	if err := runUI(cmd, nil); err == nil {
		t.Fatal("negative port accepted")
	}
}
