package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreamWriterChunksAndLimit(t *testing.T) {
	var capture bytes.Buffer
	var events []string
	w := newStreamWriter(&capture, "stdout", func(stream, line string) {
		if stream != "stdout" {
			t.Error(stream)
		}
		events = append(events, line)
	})
	input := "hello\r\n中文\npartial"
	for _, b := range []byte(input) {
		_, _ = w.Write([]byte{b})
	}
	w.flush()
	if got := strings.Join(events, ""); got != "hello\n中文\npartial" {
		t.Fatalf("broken output: %q", got)
	}
	if capture.String() != input {
		t.Fatalf("capture: %q", capture.String())
	}
	_, _ = w.Write([]byte(strings.Repeat("x", 2*maxCapturedOutput)))
	w.flush()
	if capture.Len() != maxCapturedOutput {
		t.Fatal("capture is not bounded")
	}
	if w.pending != "" {
		t.Fatal("pending output was not flushed")
	}
}

func TestStreamingProcess(t *testing.T) {
	if os.Getenv("AGENTUP_STREAM_TEST") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "first")
	fmt.Fprintln(os.Stderr, "warning")
	time.Sleep(400 * time.Millisecond)
	fmt.Fprint(os.Stdout, "last")
	os.Exit(0)
}

func TestDefaultRunnerStreamsBeforeCompletion(t *testing.T) {
	t.Setenv("AGENTUP_STREAM_TEST", "1")
	started := make(chan struct{}, 1)
	done := make(chan struct{})
	var mu sync.Mutex
	var events []string
	go func() {
		defer close(done)
		out, errOut, code, err := (&DefaultRunner{}).RunStreaming(os.Args[0], []string{"-test.run=^TestStreamingProcess$"}, func(stream, text string) {
			mu.Lock()
			events = append(events, stream+":"+text)
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
		})
		if err != nil || code != 0 || out != "first\nlast" || errOut != "warning\n" {
			t.Errorf("unexpected result: %q %q %d %v", out, errOut, code, err)
		}
	}()
	select {
	case <-started:
		select {
		case <-done:
			t.Error("output was buffered until exit")
		default:
		}
	case <-time.After(3 * time.Second):
		t.Error("no streamed output")
	}
	<-done
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(strings.Join(events, ""), "stdout:last") {
		t.Fatal("final partial line was lost")
	}
}

func TestStreamingHonorsTimeout(t *testing.T) {
	t.Setenv("AGENTUP_TEST_SLEEP", "1")
	r := WithTimeout(&DefaultRunner{}, 50*time.Millisecond).(interface {
		RunStreaming(string, []string, func(string, string)) (string, string, int, error)
	})
	_, _, code, err := r.RunStreaming(os.Args[0], []string{"-test.run=^TestRunnerHelperProcess$"}, func(string, string) {})
	if code != -1 || err != context.DeadlineExceeded {
		t.Fatalf("expected timeout: %d %v", code, err)
	}
}
