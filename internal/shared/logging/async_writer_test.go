package logging

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

type testWriteCloser struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	closed bool
}

func (w *testWriteCloser) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(p)
}

func (w *testWriteCloser) Close() error {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	return nil
}

func (w *testWriteCloser) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func TestAsyncWriterCloseDrainsQueue(t *testing.T) {
	target := &testWriteCloser{}
	writer := newAsyncWriter(target, 4, 64, time.Hour)

	const want = "first\nsecond\n"
	if _, err := io.WriteString(writer, want); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := target.String(); got != want {
		t.Fatalf("expected drained output %q, got %q", want, got)
	}
	if !target.closed {
		t.Fatal("expected target to be closed")
	}
}

func TestAsyncWriterFlushesOnInterval(t *testing.T) {
	target := &testWriteCloser{}
	writer := newAsyncWriter(target, 4, 64, 10*time.Millisecond)
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := io.WriteString(writer, "record\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for target.String() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := target.String(); got != "record\n" {
		t.Fatalf("expected interval flush, got %q", got)
	}
}
