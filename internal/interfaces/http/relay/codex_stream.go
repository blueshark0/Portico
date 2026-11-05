package relay

import (
	"strings"

	"github.com/tidwall/gjson"
)

type codexStreamStatus uint8

const (
	codexStreamPending codexStreamStatus = iota
	codexStreamCompleted
	codexStreamFailed
)

// codexStreamOutcome 跟踪 Responses SSE 的业务终止事件。HTTP 200 只表示
// 流已建立；真正的成功或失败由 response.completed / response.failed /
// response.incomplete 决定。
type codexStreamOutcome struct {
	status    codexStreamStatus
	eventType string
}

func (o *codexStreamOutcome) Observe(payload []byte) {
	if o == nil || o.status != codexStreamPending || len(payload) == 0 {
		return
	}
	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		eventType := gjson.Get(data, "type").String()
		switch eventType {
		case "response.completed":
			response := gjson.Get(data, "response")
			if response.Get("status").String() != "completed" ||
				!missingOrNull(response.Get("error")) ||
				!missingOrNull(response.Get("incomplete_details")) {
				continue
			}
			o.status = codexStreamCompleted
			o.eventType = eventType
			return
		case "response.failed", "response.incomplete":
			o.status = codexStreamFailed
			o.eventType = eventType
			return
		}
	}
}

func (o *codexStreamOutcome) Status() codexStreamStatus {
	if o == nil {
		return codexStreamPending
	}
	return o.status
}

func (o *codexStreamOutcome) EventType() string {
	if o == nil {
		return ""
	}
	return o.eventType
}

func missingOrNull(result gjson.Result) bool {
	return !result.Exists() || result.Type == gjson.Null
}
