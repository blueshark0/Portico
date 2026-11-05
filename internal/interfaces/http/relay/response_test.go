package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type flushCaptureWriter struct {
	mu      sync.Mutex
	header  http.Header
	status  int
	body    bytes.Buffer
	flushes int
	writes  chan struct{}
	flushed chan struct{}
}

func newFlushCaptureWriter() *flushCaptureWriter {
	return &flushCaptureWriter{
		header:  make(http.Header),
		writes:  make(chan struct{}, 16),
		flushed: make(chan struct{}, 16),
	}
}

func (w *flushCaptureWriter) Header() http.Header { return w.header }

func (w *flushCaptureWriter) WriteHeader(status int) { w.status = status }

func (w *flushCaptureWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	n, err := w.body.Write(data)
	w.mu.Unlock()
	select {
	case w.writes <- struct{}{}:
	default:
	}
	return n, err
}

func (w *flushCaptureWriter) Flush() {
	w.mu.Lock()
	w.flushes++
	w.mu.Unlock()
	select {
	case w.flushed <- struct{}{}:
	default:
	}
}

func (w *flushCaptureWriter) snapshot() (string, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String(), w.flushes
}

func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("等待%s超时", what)
	}
}

func TestWriteUpstreamResponseSSEFlushesSmallEventAtBoundary(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       reader,
	}
	out := newFlushCaptureWriter()
	done := make(chan error, 1)
	go func() {
		_, err := writeUpstreamResponse(out, resp, nil)
		done <- err
	}()

	// 首行远小于 xrequest 原来的 1024 字节 Peek 窗口；只到 data 行还不能 Flush。
	firstLine := "data: {\"type\":\"message_start\"}\n"
	if _, err := io.WriteString(writer, firstLine); err != nil {
		t.Fatalf("写首行: %v", err)
	}
	waitSignal(t, out.writes, "首行写出")
	if _, flushes := out.snapshot(); flushes != 0 {
		t.Fatalf("事件未结束不应 Flush，got %d", flushes)
	}

	if _, err := io.WriteString(writer, "\n"); err != nil {
		t.Fatalf("写事件结束: %v", err)
	}
	waitSignal(t, out.flushed, "事件结束 Flush")
	if body, _ := out.snapshot(); body != firstLine+"\n" {
		t.Fatalf("首事件 body = %q, want %q", body, firstLine+"\n")
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("关闭上游流: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("复制 SSE: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待 SSE 复制结束超时")
	}
}

func TestWriteUpstreamResponsePseudoSSEKeepsJSONCompatibility(t *testing.T) {
	payload := `{"usage":{"input_tokens":7,"output_tokens":3},"safeguard_results":[{"decision":"allow"}]}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(payload)),
	}
	recorder := httptest.NewRecorder()
	var chunks [][]byte
	hook := func(data []byte) (bool, []byte) {
		chunks = append(chunks, append([]byte(nil), data...))
		return true, data
	}

	if _, err := writeUpstreamResponse(recorder, resp, hook); err != nil {
		t.Fatalf("复制伪 SSE: %v", err)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := recorder.Body.String(); got != payload {
		t.Fatalf("body = %q, want %q", got, payload)
	}
	if len(chunks) != 1 || !bytes.Equal(chunks[0], []byte(payload)) {
		t.Fatalf("hook chunks = %q, want one complete JSON body", chunks)
	}
}

func TestWriteBufferedUpstreamResponseKeepsJSONHookWhole(t *testing.T) {
	payload := `{"usage":{"input_tokens":11},"unknown_field":{"keep":true}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(io.MultiReader(strings.NewReader(payload[:20]), strings.NewReader(payload[20:]))),
	}
	recorder := httptest.NewRecorder()
	var chunks [][]byte
	if _, err := writeUpstreamResponse(recorder, resp, func(data []byte) (bool, []byte) {
		chunks = append(chunks, append([]byte(nil), data...))
		return true, data
	}); err != nil {
		t.Fatalf("复制非流式响应: %v", err)
	}
	if len(chunks) != 1 || !bytes.Equal(chunks[0], []byte(payload)) {
		t.Fatalf("非流式 hook chunks = %q, want complete body", chunks)
	}
	if recorder.Body.String() != payload {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), payload)
	}
}
