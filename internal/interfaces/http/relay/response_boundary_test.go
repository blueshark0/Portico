package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteUpstreamResponsePseudoSSEBoundary(t *testing.T) {
	for _, size := range []int{1023, 1024, 1025} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), size)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(bytes.NewReader(payload)),
			}
			recorder := httptest.NewRecorder()
			var chunks [][]byte
			if _, err := writeUpstreamResponse(recorder, resp, func(data []byte) (bool, []byte) {
				chunks = append(chunks, append([]byte(nil), data...))
				return true, data
			}); err != nil {
				t.Fatalf("复制响应: %v", err)
			}
			wantType := "text/event-stream"
			if size < 1024 {
				wantType = "application/json"
			}
			if got := recorder.Header().Get("Content-Type"); got != wantType {
				t.Fatalf("Content-Type = %q, want %q", got, wantType)
			}
			if !bytes.Equal(recorder.Body.Bytes(), payload) {
				t.Fatalf("body 长度 = %d, want %d", recorder.Body.Len(), size)
			}
			if len(chunks) != 1 || !bytes.Equal(chunks[0], payload) {
				t.Fatalf("hook chunks = %d 个，长度不符", len(chunks))
			}
		})
	}
}
