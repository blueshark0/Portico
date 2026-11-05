package wails

import (
	observabilitydomain "portico/internal/observability/domain"
)

// AppLogTailer 是 facade 对 app.log 读取的窄依赖（由
// observability/infrastructure.AppLogReader 满足）。读路径没有用例逻辑，
// facade 直接依赖读取接口，不经过 application 层。
type AppLogTailer interface {
	Tail(offset int64, maxLines int, level string, keyword string) (observabilitydomain.AppLogChunk, error)
}

type AppLogFacade struct {
	tailer AppLogTailer
}

func NewAppLogFacade(tailer AppLogTailer) *AppLogFacade {
	return &AppLogFacade{tailer: tailer}
}

func (f *AppLogFacade) TailAppLog(offset int64, maxLines int, level string, keyword string) (observabilitydomain.AppLogChunk, error) {
	return f.tailer.Tail(offset, maxLines, level, keyword)
}
