package worker

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Config defines queue sizing and flush behavior for a background worker.
type Config struct {
	Name          string
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
	DropWhenFull  bool
	DropLogEvery  time.Duration
}

type BatchProcessor[T any] interface {
	Process(batch []T) error
	ProcessSingle(item T) error
}

type BackgroundWorker[T any] struct {
	config    Config
	inputCh   chan T
	shutdown  chan struct{}
	wg        sync.WaitGroup
	processor BatchProcessor[T]
	shutOnce  sync.Once
	stateMu   sync.RWMutex
	stopped   bool
	dropMu    sync.Mutex
	dropped   int
	lastDrop  time.Time
}

func New[T any](config Config, processor BatchProcessor[T]) *BackgroundWorker[T] {
	if config.BufferSize <= 0 {
		config.BufferSize = 1024
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = time.Second
	}
	return &BackgroundWorker[T]{
		config:    config,
		inputCh:   make(chan T, config.BufferSize),
		shutdown:  make(chan struct{}),
		processor: processor,
	}
}

func (w *BackgroundWorker[T]) Start() {
	w.wg.Add(1)
	go w.run()
}

func (w *BackgroundWorker[T]) Stop() {
	w.shutOnce.Do(func() {
		w.stateMu.Lock()
		w.stopped = true
		w.stateMu.Unlock()
		close(w.shutdown)
	})
	w.wg.Wait()
}

// Enqueue returns true when the item is queued and false when it is processed synchronously or dropped.
func (w *BackgroundWorker[T]) Enqueue(item T) bool {
	w.stateMu.RLock()
	if w.stopped {
		w.stateMu.RUnlock()
		w.safeProcessSingle(item)
		return false
	}
	select {
	case w.inputCh <- item:
		w.stateMu.RUnlock()
		return true
	default:
		w.stateMu.RUnlock()
		if w.config.DropWhenFull {
			w.logDrop()
			return false
		}
		log.Printf("%s 缓冲已满，回退为同步处理\n", w.config.Name)
		w.safeProcessSingle(item)
		return false
	}
}

// safeProcess 与 safeProcessSingle 吸收处理器 panic，避免后台 goroutine
// 或调用方（桌面应用主流程）被单条坏数据击穿。
func (w *BackgroundWorker[T]) safeProcess(batch []T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s 批量处理 panic 已恢复: %v\n", w.config.Name, r)
			err = fmt.Errorf("%s: batch processor panic: %v", w.config.Name, r)
		}
	}()
	return w.processor.Process(batch)
}

func (w *BackgroundWorker[T]) safeProcessSingle(item T) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%s 单条处理 panic 已恢复: %v\n", w.config.Name, r)
		}
	}()
	_ = w.processor.ProcessSingle(item)
}

func (w *BackgroundWorker[T]) logDrop() {
	interval := w.config.DropLogEvery
	if interval <= 0 {
		interval = 5 * time.Second
	}
	now := time.Now()

	w.dropMu.Lock()
	w.dropped++
	if !w.lastDrop.IsZero() && now.Sub(w.lastDrop) < interval {
		w.dropMu.Unlock()
		return
	}
	dropped := w.dropped
	w.dropped = 0
	w.lastDrop = now
	w.dropMu.Unlock()

	log.Printf("%s 缓冲已满，已丢弃 %d 条记录\n", w.config.Name, dropped)
}

func (w *BackgroundWorker[T]) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]T, 0, w.config.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := w.safeProcess(batch); err != nil {
			log.Printf("%s 批量处理失败: %v，尝试逐条处理\n", w.config.Name, err)
			for _, item := range batch {
				w.safeProcessSingle(item)
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case item := <-w.inputCh:
			batch = append(batch, item)
			if len(batch) >= w.config.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-w.shutdown:
			for {
				select {
				case item := <-w.inputCh:
					batch = append(batch, item)
					if len(batch) >= w.config.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}
