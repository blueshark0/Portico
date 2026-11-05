package relay

import (
	"bytes"
	"log"
	"os"
	"strings"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/kernel"
)

// maxUpstreamErrorBodyBytes 限制写入应用日志的上游错误体大小，避免超大 HTML
// 错误页撑爆单条日志（app_log_reader 的回扫窗口为 2MB）。
const maxUpstreamErrorBodyBytes = 8 * 1024

var relayDebugEnabled = os.Getenv("PORTICO_DEBUG_RELAY") == "1" || os.Getenv("CODE_SWITCH_DEBUG_RELAY") == "1"

func relayDebugf(format string, args ...any) {
	if !relayDebugEnabled {
		return
	}
	log.Printf(format, args...)
}

func relayWarnf(format string, args ...any) {
	log.Printf("[WARN] [relay] "+format, args...)
}

// upstreamErrorCollector 收集非 2xx 响应体用于应用日志：只保留一份截断副本，
// hook 始终原样放行数据，不影响透传给客户端的内容。
type upstreamErrorCollector struct {
	buf       bytes.Buffer
	truncated bool
}

func (c *upstreamErrorCollector) hook(data []byte) (bool, []byte) {
	if remaining := maxUpstreamErrorBodyBytes - c.buf.Len(); remaining > 0 {
		if len(data) > remaining {
			c.buf.Write(data[:remaining])
			c.truncated = true
		} else {
			c.buf.Write(data)
		}
	} else if len(data) > 0 {
		c.truncated = true
	}
	return true, data
}

// logWith 把状态码与错误体打成一条 [WARN] 日志（应用日志页可按级别筛出），
// 紧邻"请求失败"行、时间戳相邻，便于与请求日志对照。
func (c *upstreamErrorCollector) logWith(providerName, model string, status int) {
	body := strings.TrimSpace(c.buf.String())
	if body == "" {
		relayWarnf("上游错误响应: provider=%s model=%s status=%d (无响应体)", providerName, model, status)
		return
	}
	if c.truncated {
		body += "...(已截断)"
	}
	relayWarnf("上游错误响应: provider=%s model=%s status=%d body=%s", providerName, model, status, body)
}

func (s *Server) startRequestLogRetentionTask() {
	if s == nil || s.retentionCleanup == nil {
		return
	}
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		ticker := time.NewTicker(requestLogCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.retentionCleanup(); err != nil {
					log.Printf("定时清理 request_log 失败: %v\n", err)
				}
			case <-s.shutdownCh:
				return
			}
		}
	}()
}

func (s *Server) stopBackgroundWorkers() {
	if s == nil {
		return
	}
	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})
	if s.requestLogWorker != nil {
		s.requestLogWorker.Stop()
	}
	if s.sessionUpdateWorker != nil {
		s.sessionUpdateWorker.Stop()
	}
	s.backgroundWG.Wait()
}

func (s *Server) enqueueRequestLog(logEntry *observabilitydomain.RequestLog) {
	if s == nil || logEntry == nil {
		return
	}
	if logEntry.CreatedAt == "" {
		// 批量落盘会延迟写入；时间必须表示请求结束并入队的时刻，而不是
		// worker 真正拿到数据库连接的时刻，避免统计区间随批量间隔漂移。
		logEntry.CreatedAt = time.Now().UTC().Format(time.DateTime)
	}
	if s.requestLogWorker == nil {
		s.writeRequestLogSync(logEntry)
		return
	}
	s.requestLogWorker.Enqueue(logEntry)
}

func (s *Server) writeRequestLogSync(logEntry *observabilitydomain.RequestLog) {
	if logEntry == nil || s.requestLogSink == nil {
		return
	}
	if err := s.requestLogSink.ProcessSingle(logEntry); err != nil {
		log.Printf("写入 request_log 失败: %v\n", err)
	}
}

func (s *Server) requestLogHook(platform kernel.Platform, usage *observabilitydomain.RequestLog) func(data []byte) (bool, []byte) {
	var parser TokenUsageParser
	if s.tokenParserFor != nil {
		parser = s.tokenParserFor(platform)
	}
	return func(data []byte) (bool, []byte) {
		payload := strings.TrimSpace(string(data))
		parseEventPayload(payload, parser, usage)
		return true, data
	}
}

func parseEventPayload(payload string, parser TokenUsageParser, usage *observabilitydomain.RequestLog) {
	if parser == nil || usage == nil || payload == "" {
		return
	}
	foundData := false
	lines := strings.Split(payload, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			foundData = true
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				continue
			}
			parser.Parse(data, usage)
		}
	}
	// 非流式响应（stream:false、/responses/compact、Gemini generateContent）
	// 是纯 JSON 而非 SSE：没有 data: 行时把整个 payload 交给解析器。
	if !foundData && strings.HasPrefix(strings.TrimSpace(payload), "{") {
		parser.Parse(payload, usage)
	}
}
