package relay

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// relayResponseHook 观察响应片段；返回 false 时仅对流式响应跳过写出。
// 当前 relay 的 hook 都只观察并原样返回数据，不能用 parse→marshal 改写响应。
type relayResponseHook func(data []byte) (bool, []byte)

func copyUpstreamResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
}

func writeUpstreamResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	copyUpstreamResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
}

// writeUpstreamResponse 处理成功响应。只有上游明确声明 SSE 时才走流式路径；
// 其他响应仍一次性交给 hook，保证非流式 JSON 的 usage 解析不被切碎。
func writeUpstreamResponse(w http.ResponseWriter, resp *http.Response, hook relayResponseHook) (int64, error) {
	if resp.Body == nil {
		writeUpstreamResponseHeaders(w, resp)
		return 0, nil
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices &&
		strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return writeSSEUpstreamResponse(w, resp, hook)
	}
	return writeBufferedUpstreamResponse(w, resp, hook)
}

// writeBufferedUpstreamResponse 也用于非 2xx：即使上游声明了 SSE，错误响应仍按
// 原来的非流式路径透传，并让 collector 看到完整错误体。
func writeBufferedUpstreamResponse(w http.ResponseWriter, resp *http.Response, hook relayResponseHook) (int64, error) {
	writeUpstreamResponseHeaders(w, resp)
	if resp.Body == nil {
		return 0, nil
	}
	body, readErr := io.ReadAll(resp.Body)
	written, writeErr := writeResponseBytes(w, body, hook, false)
	if writeErr != nil {
		return written, writeErr
	}
	if readErr != nil {
		return written, fmt.Errorf("read upstream response: %w", readErr)
	}
	return written, nil
}

// writeSSEUpstreamResponse 使用小缓冲读取首行，但不等待凑满 1024 字节。
// 首行只有在小于 1024 字节、EOF 且没有换行时才进入旧的伪 SSE 兼容分支；
// 正常 SSE 首行收到后即可开始转发。
func writeSSEUpstreamResponse(w http.ResponseWriter, resp *http.Response, hook relayResponseHook) (int64, error) {
	reader := bufio.NewReaderSize(resp.Body, 1024)
	firstLine, readErr := reader.ReadSlice('\n')
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, bufio.ErrBufferFull) {
		return 0, fmt.Errorf("read upstream SSE response: %w", readErr)
	}
	if errors.Is(readErr, io.EOF) && len(firstLine) < 1024 && !bytes.Contains(firstLine, []byte("\n")) {
		// 某些中转站把普通 JSON 标成 text/event-stream。保留旧的 Content-Type
		// 兼容行为，同时让 token hook 看到完整 JSON。
		resp.Header.Set("Content-Type", "application/json")
		writeUpstreamResponseHeaders(w, resp)
		return writeResponseBytes(w, firstLine, hook, true)
	}

	writeUpstreamResponseHeaders(w, resp)
	if errors.Is(readErr, bufio.ErrBufferFull) {
		// ReadSlice 的片段指向 reader 内部缓冲；继续读之前先复制，且保留
		// 完整行给 hook，不能把长行拆成多个 usage/事件片段。
		prefix := append([]byte(nil), firstLine...)
		remainder, err := reader.ReadBytes('\n')
		firstLine = append(prefix, remainder...)
		readErr = err
	}
	var total int64
	line, lineErr := firstLine, readErr
	for {
		if len(line) > 0 {
			written, err := writeSSELine(w, line, hook)
			total += written
			if err != nil {
				return total, err
			}
			if isSSEBlankLine(line) {
				flushUpstreamResponse(w)
			}
		}
		if lineErr != nil {
			// EOF 前可能有没有空行的最后一行；读错时也把已读到的片段交给客户端。
			flushUpstreamResponse(w)
			if errors.Is(lineErr, io.EOF) {
				return total, nil
			}
			return total, fmt.Errorf("read upstream SSE response: %w", lineErr)
		}
		line, lineErr = reader.ReadBytes('\n')
	}
}

func writeSSELine(w http.ResponseWriter, line []byte, hook relayResponseHook) (int64, error) {
	trimmedLine := bytes.TrimRight(line, "\n")
	if len(trimmedLine) == 0 {
		// 与旧实现一致：纯换行的空行不经过 hook，但原样写出。
		return writeResponseBytes(w, line, nil, false)
	}

	processed := trimmedLine
	flush := true
	if hook != nil {
		flush, processed = hook(trimmedLine)
	}
	if !flush {
		return 0, nil
	}
	if bytes.HasSuffix(line, []byte("\n")) {
		processed = append(processed, '\n')
	}
	return writeResponseBytes(w, processed, nil, false)
}

func isSSEBlankLine(line []byte) bool {
	return len(bytes.TrimRight(line, "\r\n")) == 0
}

func writeResponseBytes(w http.ResponseWriter, data []byte, hook relayResponseHook, honorFlush bool) (int64, error) {
	processed := data
	flush := true
	if hook != nil {
		flush, processed = hook(data)
	}
	if honorFlush && !flush {
		return 0, nil
	}
	if len(processed) == 0 {
		return 0, nil
	}
	n, err := w.Write(processed)
	if err != nil {
		return int64(n), fmt.Errorf("write upstream response: %w", err)
	}
	if n != len(processed) {
		return int64(n), io.ErrShortWrite
	}
	return int64(n), nil
}

func flushUpstreamResponse(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
