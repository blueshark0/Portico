package relay

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

func TestUpstreamErrorCollectorAccumulatesAndPassesThrough(t *testing.T) {
	c := &upstreamErrorCollector{}
	for _, chunk := range []string{"chunk-one ", "chunk-two"} {
		flush, out := c.hook([]byte(chunk))
		if !flush || string(out) != chunk {
			t.Fatalf("hook 必须原样放行数据: flush=%v out=%q", flush, out)
		}
	}
	if got := c.buf.String(); got != "chunk-one chunk-two" {
		t.Fatalf("累积结果 = %q", got)
	}
	if c.truncated {
		t.Fatal("未超限不应标记截断")
	}
}

func TestUpstreamErrorCollectorTruncatesAtLimit(t *testing.T) {
	c := &upstreamErrorCollector{}
	first := strings.Repeat("a", maxUpstreamErrorBodyBytes-10)
	overflow := strings.Repeat("b", 100)
	if _, out := c.hook([]byte(first)); string(out) != first {
		t.Fatal("第一块必须原样放行")
	}
	c.hook([]byte(overflow))
	if c.buf.Len() != maxUpstreamErrorBodyBytes {
		t.Fatalf("buffer 长度 = %d, want %d", c.buf.Len(), maxUpstreamErrorBodyBytes)
	}
	if !c.truncated {
		t.Fatal("超限应标记截断")
	}
	if _, out := c.hook([]byte("more")); string(out) != "more" {
		t.Fatal("已满后仍须原样放行")
	}
}

func TestUpstreamErrorCollectorLogWith(t *testing.T) {
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	c := &upstreamErrorCollector{}
	c.hook([]byte("\n  {\"error\":\"rate limited\"}\n"))
	c.logWith("prov-a", "claude-sonnet-4", 429)
	for _, want := range []string{"[WARN] [relay]", "provider=prov-a", "model=claude-sonnet-4", "status=429", `"error":"rate limited"`} {
		if !strings.Contains(logBuf.String(), want) {
			t.Fatalf("日志缺少 %q: %s", want, logBuf.String())
		}
	}

	logBuf.Reset()
	empty := &upstreamErrorCollector{}
	empty.logWith("prov-b", "m", 502)
	if !strings.Contains(logBuf.String(), "(无响应体)") || strings.Contains(logBuf.String(), "body=") {
		t.Fatalf("空 body 应提示无响应体且不带 body=: %s", logBuf.String())
	}

	logBuf.Reset()
	truncated := &upstreamErrorCollector{}
	truncated.hook([]byte(strings.Repeat("x", maxUpstreamErrorBodyBytes+1)))
	truncated.logWith("prov-c", "m", 500)
	if !strings.Contains(logBuf.String(), "...(已截断)") {
		t.Fatalf("截断应带标记: %s", logBuf.String())
	}
}
