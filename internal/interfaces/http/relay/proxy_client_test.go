package relay

import (
	"net/http"
	"testing"
)

func TestUpstreamClientKeepsTLSVerificationByDefault(t *testing.T) {
	server := &Server{}
	client, err := server.upstreamClientFor(upstreamClientOptions{proxyURL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("upstream client: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected TLS verification to stay enabled unless provider explicitly opts out")
	}
}

func TestUpstreamClientHonorsExplicitInsecureSkipVerify(t *testing.T) {
	server := &Server{}
	client, err := server.upstreamClientFor(upstreamClientOptions{
		proxyURL:           "http://127.0.0.1:7890",
		insecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("upstream client: %v", err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected TLS verification to be disabled when provider explicitly opts in")
	}
}

func TestUpstreamClientIsReusedForSameOptions(t *testing.T) {
	server := &Server{}
	first, err := server.upstreamClientFor(upstreamClientOptions{proxyURL: ""})
	if err != nil {
		t.Fatalf("first upstream client: %v", err)
	}
	second, err := server.upstreamClientFor(upstreamClientOptions{proxyURL: ""})
	if err != nil {
		t.Fatalf("second upstream client: %v", err)
	}
	if first != second {
		t.Fatal("expected the same client instance to be reused for identical options")
	}
	other, err := server.upstreamClientFor(upstreamClientOptions{proxyURL: "http://127.0.0.1:7890"})
	if err != nil {
		t.Fatalf("proxied upstream client: %v", err)
	}
	if other == first {
		t.Fatal("expected a distinct client for different proxy options")
	}
}

func TestUpstreamClientRejectsInvalidProxyURL(t *testing.T) {
	server := &Server{}
	if _, err := server.upstreamClientFor(upstreamClientOptions{proxyURL: "://bad-url"}); err == nil {
		t.Fatal("expected invalid proxy URL to be rejected")
	}
}

func TestRelayShouldBypassProxy(t *testing.T) {
	tests := []struct {
		name  string
		host  string
		rules []string
		want  bool
	}{
		{name: "wildcard", host: "api.example.com", rules: []string{"*"}, want: true},
		{name: "exact", host: "localhost", rules: []string{"localhost"}, want: true},
		{name: "suffix", host: "api.example.com", rules: []string{".example.com"}, want: true},
		{name: "port stripped", host: "127.0.0.1", rules: []string{"127.0.0.1:8080"}, want: true},
		{name: "miss", host: "api.example.com", rules: []string{"internal.local"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relayShouldBypassProxy(tt.host, tt.rules); got != tt.want {
				t.Fatalf("relayShouldBypassProxy() = %v, want %v", got, tt.want)
			}
		})
	}
}
