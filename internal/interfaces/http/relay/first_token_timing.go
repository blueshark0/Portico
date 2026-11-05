package relay

import (
	"bytes"
	"sync"
	"time"

	observabilitydomain "portico/internal/observability/domain"
)

// withFirstTokenTiming 只观察首个响应数据事件，始终调用原 hook 并原样返回其数据。
// SSE 以首条非空 data: 事件计时，非流式响应以 JSON body 计时；0 表示没有样本。
func withFirstTokenTiming(start time.Time, usage *observabilitydomain.RequestLog, next func([]byte) (bool, []byte)) func([]byte) (bool, []byte) {
	if next == nil {
		next = func(data []byte) (bool, []byte) { return true, data }
	}
	var once sync.Once
	return func(data []byte) (bool, []byte) {
		if usage != nil && hasResponseDataEvent(data) {
			once.Do(func() {
				usage.TTFTSec = time.Since(start).Seconds()
			})
		}
		return next(data)
	}
}

func hasResponseDataEvent(data []byte) bool {
	payload := bytes.TrimSpace(data)
	if len(payload) == 0 {
		return false
	}
	foundData := false
	for _, line := range bytes.Split(payload, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		foundData = true
		if len(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))) > 0 {
			return true
		}
	}
	return !foundData && bytes.HasPrefix(payload, []byte("{"))
}
