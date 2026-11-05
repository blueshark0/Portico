package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"portico/internal/shared/storage"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	appLogQueueSize     = 512
	appLogBufferSize    = 64 * 1024
	appLogFlushInterval = time.Second
)

// Setup configures the global logger to write to stdout and a rotating file.
func Setup() io.WriteCloser {
	logDir, err := storage.AppDataDir()
	if err != nil {
		log.Printf("无法获取日志目录，使用当前目录: %v", err)
		logDir = "."
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.Printf("无法创建日志目录 %s: %v", logDir, err)
		return nil
	}

	logger := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "app.log"),
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     14,
		Compress:   true,
	}
	bufferedLogger := newAsyncWriter(logger, appLogQueueSize, appLogBufferSize, appLogFlushInterval)

	log.SetOutput(io.MultiWriter(os.Stdout, bufferedLogger))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("日志系统初始化成功，日志文件: %s", logger.Filename)
	return bufferedLogger
}
