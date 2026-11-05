package logging

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"time"
)

var errAsyncWriterClosed = errors.New("async log writer is closed")

// asyncWriter keeps log calls short by queueing records and flushing them in
// batches. The queue is bounded; writers wait when it is full so log records
// are not silently discarded.
type asyncWriter struct {
	target        io.WriteCloser
	queue         chan []byte
	bufferSize    int
	flushInterval time.Duration
	done          chan struct{}
	closeOnce     sync.Once
	wg            sync.WaitGroup
	stateMu       sync.RWMutex
	closed        bool

	errMu    sync.Mutex
	writeErr error
	closeErr error
}

func newAsyncWriter(target io.WriteCloser, queueSize, bufferSize int, flushInterval time.Duration) io.WriteCloser {
	if queueSize <= 0 {
		queueSize = 1
	}
	if bufferSize <= 0 {
		bufferSize = 4 * 1024
	}
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	writer := &asyncWriter{
		target:        target,
		queue:         make(chan []byte, queueSize),
		bufferSize:    bufferSize,
		flushInterval: flushInterval,
		done:          make(chan struct{}),
	}
	writer.wg.Add(1)
	go writer.run()
	return writer
}

func (w *asyncWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	data := append([]byte(nil), p...)
	w.stateMu.RLock()
	defer w.stateMu.RUnlock()
	if w.closed {
		return 0, errAsyncWriterClosed
	}
	// Keep the read lock while queueing so Close cannot stop the worker before
	// this record is accepted and drained.
	w.queue <- data
	return len(p), nil
}

func (w *asyncWriter) Close() error {
	w.closeOnce.Do(func() {
		w.stateMu.Lock()
		w.closed = true
		close(w.done)
		w.stateMu.Unlock()
		w.wg.Wait()
		w.errMu.Lock()
		w.closeErr = w.target.Close()
		w.errMu.Unlock()
	})

	w.errMu.Lock()
	writeErr := w.writeErr
	closeErr := w.closeErr
	w.errMu.Unlock()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func (w *asyncWriter) run() {
	defer w.wg.Done()
	buffered := bufio.NewWriterSize(w.target, w.bufferSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	write := func(data []byte) {
		if _, err := buffered.Write(data); err != nil {
			w.setWriteErr(err)
		}
	}
	flush := func() {
		if err := buffered.Flush(); err != nil {
			w.setWriteErr(err)
		}
	}

	for {
		select {
		case data := <-w.queue:
			write(data)
		case <-ticker.C:
			flush()
		case <-w.done:
			for {
				select {
				case data := <-w.queue:
					write(data)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (w *asyncWriter) setWriteErr(err error) {
	if err == nil {
		return
	}
	w.errMu.Lock()
	if w.writeErr == nil {
		w.writeErr = err
	}
	w.errMu.Unlock()
}
