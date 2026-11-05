package relay

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type relayRouteOptions struct {
	endpointMutator    func(endpoint, requestedModel, effectiveModel string) (string, error)
	queryMutator       func(query map[string]string, provider routingdomain.Provider) map[string]string
	headerMutator      func(headers map[string]string, provider routingdomain.Provider) map[string]string
	bodyModelFormatter func(model string) string
	forceBodyRewrite   bool
}

type RequestMeta struct {
	Endpoint          string
	RequestedModel    string
	SessionID         string
	IsStream          bool
	IsFast            bool
	IsBackgroundTask  bool
	BodyHasModelField bool
	RouteOptions      *relayRouteOptions
}

type RelayContext struct {
	GinCtx      *gin.Context
	Platform    kernel.Platform
	BodyBytes   []byte
	Query       map[string]string
	Headers     map[string]string
	Profile     routingdomain.RouteProfile
	RequestMeta RequestMeta
}

type ForwardContext struct {
	GinCtx           *gin.Context
	Platform         kernel.Platform
	Provider         *routingdomain.Provider
	Endpoint         string
	Query            map[string]string
	Headers          map[string]string
	BodyBytes        []byte
	IsStream         bool
	IsFast           bool
	IsBackgroundTask bool
	Model            string
	SessionID        string
}

type PlatformHandler interface {
	ExtractRequestMeta(path string, bodyBytes []byte, query, headers map[string]string) RequestMeta
}

func cloneHeaders(header http.Header) map[string]string {
	cloned := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) > 0 {
			// 同名多值头按 HTTP/1.1 语义逗号连接（Cookie 会在转发前剥离，
			// 不受影响），避免 anthropic-beta 这类 list-valued 头丢值。
			cloned[key] = strings.Join(values, ", ")
		}
	}
	return cloned
}

func cloneMap(m map[string]string) map[string]string {
	cloned := make(map[string]string, len(m))
	for k, v := range m {
		cloned[k] = v
	}
	return cloned
}

func flattenQuery(values map[string][]string) map[string]string {
	query := make(map[string]string, len(values))
	for key, items := range values {
		if len(items) > 0 {
			query[key] = items[len(items)-1]
		}
	}
	return query
}

func headerValue(headers map[string]string, name string) string {
	if headers == nil {
		return ""
	}
	if value, ok := headers[name]; ok {
		return value
	}
	target := strings.ToLower(name)
	for key, value := range headers {
		if strings.ToLower(key) == target {
			return value
		}
	}
	return ""
}
