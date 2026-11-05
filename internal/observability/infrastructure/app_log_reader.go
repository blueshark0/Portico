package infrastructure

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"

	observabilitydomain "portico/internal/observability/domain"
)

const (
	defaultAppLogTailLines = 500
	maxAppLogTailLines     = 2000
	// 首次加载/重置时最多回扫的尾部窗口；lumberjack 在 10MB 轮转，2MB 足够覆盖近期日志。
	defaultAppLogScanBytes = int64(2 << 20)
	// 跟随模式单次增量上限，积压超过该值直接重开尾部窗口（Reset）。
	defaultAppLogFollowLimit = int64(4 << 20)
)

var (
	// log.Ldate|log.Ltime|log.Lshortfile 产生 "2026/07/08 18:27:22 logging.go:23: 消息"。
	appLogHeadRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) (\S+:\d+): (.*)$`)
	// 级别标签由各调用点手动写入消息前缀，例如 "[WARN] [relay] ..."。
	appLogLevelRe = regexp.MustCompile(`^\[(INFO|WARN|ERROR|DEBUG)\] ?`)
)

// AppLogReader 按需读取 app.log 尾部：offset<=0 时从尾部窗口取最近 maxLines 条，
// offset>0 时只读增量字节（tail -f 语义）。每次调用独立打开文件，不持有 fd，
// 避免影响 lumberjack 轮转时的 rename。
type AppLogReader struct {
	path        string
	scanBytes   int64
	followLimit int64
}

func NewAppLogReader(path string) *AppLogReader {
	return &AppLogReader{
		path:        path,
		scanBytes:   defaultAppLogScanBytes,
		followLimit: defaultAppLogFollowLimit,
	}
}

// Tail 返回过滤后的日志条目。level 取 ""（不过滤）/INFO/WARN/ERROR/DEBUG/none（仅无级别行），
// keyword 对 Message+Source 做大小写不敏感子串匹配。文件不存在视为空日志而非错误。
func (r *AppLogReader) Tail(offset int64, maxLines int, level string, keyword string) (observabilitydomain.AppLogChunk, error) {
	if maxLines <= 0 {
		maxLines = defaultAppLogTailLines
	}
	if maxLines > maxAppLogTailLines {
		maxLines = maxAppLogTailLines
	}

	chunk := observabilitydomain.AppLogChunk{Entries: []observabilitydomain.AppLogEntry{}}

	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return chunk, nil
		}
		return chunk, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return chunk, err
	}
	size := info.Size()
	chunk.FileSize = size
	chunk.NextOffset = size

	start := offset
	fromTail := offset <= 0
	if offset > 0 && (size < offset || size-offset > r.followLimit) {
		// 文件被轮转/截断，或增量积压过多：重开尾部窗口。
		chunk.Reset = true
		fromTail = true
	}
	if fromTail {
		start = size - r.scanBytes
		if start < 0 {
			start = 0
		}
		chunk.Truncated = start > 0
	}
	if start >= size {
		chunk.NextOffset = offset
		return chunk, nil
	}

	if _, err := f.Seek(start, 0); err != nil {
		return chunk, err
	}
	data := make([]byte, size-start)
	if _, err := io.ReadFull(f, data); err != nil {
		return chunk, err
	}

	base := start
	if fromTail && start > 0 {
		// 丢弃窗口开头的残行，同时在 \n 处重新对齐 UTF-8 边界。
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return chunk, nil
		}
		data = data[i+1:]
		base += int64(i + 1)
	}
	// 末尾不带 \n 的残段可能仍在写入，不消费，留待下一轮。
	last := bytes.LastIndexByte(data, '\n')
	if last < 0 {
		chunk.NextOffset = base
		return chunk, nil
	}
	chunk.NextOffset = base + int64(last+1)
	data = data[:last]
	if len(data) == 0 {
		return chunk, nil
	}

	entries := parseAppLogEntries(string(data), level, keyword)
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
		chunk.Truncated = true
	}
	chunk.Entries = entries
	return chunk, nil
}

// parseAppLogEntries 逐行解析并合并续行（不以日期头开始的行），过滤在合并之后进行，
// 保证多行 JSON 错误体整体参与关键字匹配。窗口开头的孤儿续行保留为 Time=="" 的裸条目。
func parseAppLogEntries(text string, level string, keyword string) []observabilitydomain.AppLogEntry {
	entries := []observabilitydomain.AppLogEntry{}
	var cur *observabilitydomain.AppLogEntry
	flush := func() {
		if cur != nil && appLogEntryMatches(cur, level, keyword) {
			entries = append(entries, *cur)
		}
		cur = nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if m := appLogHeadRe.FindStringSubmatch(line); m != nil {
			flush()
			msg := m[3]
			lvl := ""
			if lm := appLogLevelRe.FindStringSubmatch(msg); lm != nil {
				lvl = lm[1]
				msg = msg[len(lm[0]):]
			}
			cur = &observabilitydomain.AppLogEntry{Time: m[1], Source: m[2], Level: lvl, Message: msg}
			continue
		}
		if cur == nil {
			cur = &observabilitydomain.AppLogEntry{Message: line}
			continue
		}
		cur.Message += "\n" + line
	}
	flush()
	return entries
}

func appLogEntryMatches(e *observabilitydomain.AppLogEntry, level string, keyword string) bool {
	switch level {
	case "":
	case "none":
		if e.Level != "" {
			return false
		}
	default:
		if !strings.EqualFold(e.Level, level) {
			return false
		}
	}
	if keyword != "" {
		k := strings.ToLower(keyword)
		if !strings.Contains(strings.ToLower(e.Message), k) &&
			!strings.Contains(strings.ToLower(e.Source), k) {
			return false
		}
	}
	return true
}
