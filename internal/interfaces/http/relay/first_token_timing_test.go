package relay

import (
	"bytes"
	"testing"
	"time"

	observabilitydomain "portico/internal/observability/domain"
)

func TestFirstTokenTimingRecordsFirstDataEventWithoutChangingPayload(t *testing.T) {
	usage := &observabilitydomain.RequestLog{}
	start := time.Now().Add(-25 * time.Millisecond)
	var calls int
	hook := withFirstTokenTiming(start, usage, func(data []byte) (bool, []byte) {
		calls++
		return true, data
	})

	for _, chunk := range [][]byte{
		[]byte("event: message_start\n"),
		[]byte("data:\n"),
		[]byte("data: {\"type\":\"content_block_delta\"}\n"),
		[]byte("data: {\"type\":\"message_delta\"}\n"),
	} {
		flush, output := hook(chunk)
		if !flush || !bytes.Equal(output, chunk) {
			t.Fatalf("timing hook changed response: flush=%v output=%q", flush, output)
		}
	}
	if calls != 4 {
		t.Fatalf("expected downstream hook 4 calls, got %d", calls)
	}
	if usage.TTFTSec <= 0 {
		t.Fatalf("expected first data event timing, got %f", usage.TTFTSec)
	}
	first := usage.TTFTSec
	hook([]byte("data: {\"type\":\"message_stop\"}\n"))
	if usage.TTFTSec != first {
		t.Fatalf("later events must not overwrite first timing: before=%f after=%f", first, usage.TTFTSec)
	}
}

func TestFirstTokenTimingHandlesJSONAndEmptyResponses(t *testing.T) {
	usage := &observabilitydomain.RequestLog{}
	hook := withFirstTokenTiming(time.Now(), usage, nil)
	hook([]byte(" \n"))
	if usage.TTFTSec != 0 {
		t.Fatalf("blank payload must not produce a timing sample: %f", usage.TTFTSec)
	}
	payload := []byte("{\"candidates\":[]}")
	_, output := hook(payload)
	if !bytes.Equal(output, payload) || usage.TTFTSec <= 0 {
		t.Fatalf("JSON response not observed transparently: output=%q ttft=%f", output, usage.TTFTSec)
	}
}
