package storage

const (
	CoreDBName       = "default"
	RequestLogDBName = "request_log"
	// RequestLogReadDBName 是同一 request_log.db 文件上的只读查询连接池：
	// 写入池保持单连接串行写，统计/列表查询走本池，在 WAL 下并发读且不阻塞写入。
	RequestLogReadDBName = "request_log_read"
	SessionDBName        = "session"
)
