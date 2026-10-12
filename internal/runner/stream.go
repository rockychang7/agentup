package runner

import (
	"bytes"
	"strings"
)

const maxCapturedOutput = 1 << 20

type streamWriter struct {
	capture *bytes.Buffer
	pending string
	stream  string
	output  func(string, string)
	skipLF  bool
}

func newStreamWriter(capture *bytes.Buffer, stream string, output func(string, string)) *streamWriter {
	return &streamWriter{capture: capture, stream: stream, output: output}
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if remaining := maxCapturedOutput - w.capture.Len(); remaining > 0 {
		w.capture.Write(p[:min(len(p), remaining)])
	}
	w.pending += string(p)
	for {
		if w.skipLF && len(w.pending) > 0 {
			w.skipLF = false
			w.pending = strings.TrimPrefix(w.pending, "\n")
		}
		index := strings.IndexAny(w.pending, "\r\n")
		if index < 0 {
			break
		}
		w.output(w.stream, w.pending[:index]+"\n")
		w.skipLF = w.pending[index] == '\r'
		w.pending = w.pending[index+1:]
	}
	if len(w.pending) > 16<<10 {
		w.flush()
	}
	return len(p), nil
}

func (w *streamWriter) flush() {
	if w.pending != "" {
		w.output(w.stream, w.pending)
		w.pending = ""
	}
}
