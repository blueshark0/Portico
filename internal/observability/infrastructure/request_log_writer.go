package infrastructure

import (
	"context"
	"log"
	"time"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/storage"
	modelpricing "portico/resources/model-pricing"
)

// RequestLogWriter 在写入前按当时定价物化 total_cost/has_pricing，
// 让费用档位过滤与统计可以下推到 SQL。pricing 为 nil 时使用共享定价服务
// （测试可注入固定价格表）。
type RequestLogWriter struct {
	pricing *modelpricing.Service
}

const (
	requestLogWriteAttempts    = 3
	requestLogWriteBaseBackoff = 200 * time.Millisecond
)

func (w *RequestLogWriter) pricingService() *modelpricing.Service {
	if w != nil && w.pricing != nil {
		return w.pricing
	}
	return sharedPricingService()
}

func (w *RequestLogWriter) Process(batch []*observabilitydomain.RequestLog) error {
	pricing := w.pricingService()
	logs := make([]*observabilitydomain.RequestLog, 0, len(batch))
	for _, logEntry := range batch {
		if logEntry != nil {
			decorateRequestLogCost(pricing, logEntry)
			logs = append(logs, logEntry)
		}
	}
	return insertRequestLogsWithRetry(logs)
}

func (w *RequestLogWriter) ProcessSingle(item *observabilitydomain.RequestLog) error {
	if item == nil {
		return nil
	}
	decorateRequestLogCost(w.pricingService(), item)
	err := insertRequestLogsWithRetry([]*observabilitydomain.RequestLog{item})
	if err != nil {
		log.Printf("写入 request_log 失败: %v\n", err)
	}
	return err
}

func insertRequestLogsWithRetry(logs []*observabilitydomain.RequestLog) error {
	if len(logs) == 0 {
		return nil
	}

	var err error
	for attempt := 1; attempt <= requestLogWriteAttempts; attempt++ {
		err = insertRequestLogs(logs)
		if err == nil {
			return nil
		}
		if !isSQLiteBusyErr(err) {
			return err
		}
		if attempt < requestLogWriteAttempts {
			time.Sleep(time.Duration(attempt) * requestLogWriteBaseBackoff)
		}
	}

	log.Printf("写入 request_log 失败: 数据库持续繁忙，已丢弃 %d 条记录: %v\n", len(logs), err)
	return nil
}

func insertRequestLogs(logs []*observabilitydomain.RequestLog) error {
	db, err := storage.DB(storage.RequestLogDBName)
	if err != nil {
		return err
	}

	requestLogWriteMu.Lock()
	defer requestLogWriteMu.Unlock()

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	const insertSQL = `INSERT INTO request_log (
		platform,
		model,
		provider,
		http_code,
		input_tokens,
		output_tokens,
		cache_create_tokens,
		cache_read_tokens,
		reasoning_tokens,
		is_stream,
		is_fast,
		duration_sec,
		ttft_sec,
		created_at,
		total_cost,
		has_pricing,
		session_id
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, logEntry := range logs {
		if logEntry == nil {
			continue
		}
		if _, err := stmt.ExecContext(
			ctx,
			logEntry.Platform,
			logEntry.Model,
			logEntry.Provider,
			logEntry.HttpCode,
			logEntry.InputTokens,
			logEntry.OutputTokens,
			logEntry.CacheCreateTokens,
			logEntry.CacheReadTokens,
			logEntry.ReasoningTokens,
			boolToInt(logEntry.IsStream),
			boolToInt(logEntry.IsFast),
			logEntry.DurationSec,
			logEntry.TTFTSec,
			requestLogCreatedAt(logEntry.CreatedAt),
			logEntry.TotalCost,
			boolToInt(logEntry.HasPricing),
			logEntry.SessionID,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
