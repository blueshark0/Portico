package infrastructure

import "strings"

func normalizeRelayBaseURL(addr string) string {
	host := strings.TrimSpace(addr)
	if host == "" {
		host = ":18100"
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	return host
}
