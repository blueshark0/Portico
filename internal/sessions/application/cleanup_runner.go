package application

import (
	"log"
	"sync"
	"time"
)

const defaultCleanupInterval = 5 * time.Minute

type CleanupRunner struct {
	service  interface{ CleanExpiredSessions() error }
	interval time.Duration

	ticker *time.Ticker
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewCleanupRunner(service interface{ CleanExpiredSessions() error }, interval time.Duration) *CleanupRunner {
	if interval <= 0 {
		interval = defaultCleanupInterval
	}
	return &CleanupRunner{
		service:  service,
		interval: interval,
	}
}

func (r *CleanupRunner) Start() {
	if r == nil || r.service == nil || r.ticker != nil {
		return
	}
	r.ticker = time.NewTicker(r.interval)
	r.stopCh = make(chan struct{})
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.ticker.C:
				if err := r.service.CleanExpiredSessions(); err != nil {
					log.Printf("[ERROR] 定时清理过期会话失败: %v\n", err)
				}
			case <-r.stopCh:
				return
			}
		}
	}()
	log.Printf("[INFO] 会话清理定时任务已启动（每 %s 执行）", r.interval)
}

func (r *CleanupRunner) Stop() {
	if r == nil || r.ticker == nil {
		return
	}
	r.ticker.Stop()
	close(r.stopCh)
	r.wg.Wait()
	r.ticker = nil
	r.stopCh = nil
}
