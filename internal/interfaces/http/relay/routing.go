package relay

import (
	"fmt"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

func ReplaceModelInRequestBody(bodyBytes []byte, newModel string) ([]byte, error) {
	result := gjson.GetBytes(bodyBytes, "model")
	if !result.Exists() {
		return bodyBytes, fmt.Errorf("请求体中未找到 model 字段")
	}
	modified, err := sjson.SetBytes(bodyBytes, "model", newModel)
	if err != nil {
		return bodyBytes, fmt.Errorf("替换模型名失败: %w", err)
	}
	return modified, nil
}

// routeToManualProvider 编排一次请求的路由：会话绑定优先，其次默认供应商。
// 供应商解析与模型改写的决策本身在 routing/domain（RouteProfile.ResolveProvider
// 与 Provider.PlanModelRewrite），relay 只负责 HTTP 编排与转发。
func (s *Server) routeToManualProvider(ctx *RelayContext) (bool, error) {
	kind := ctx.Platform.String()
	sessionID := ctx.RequestMeta.SessionID
	relayDebugf("session=%s platform=%s", sessionID, kind)

	if provider, sessionAlreadyBound, sessionGeneration, ok := s.resolveBoundProvider(ctx, kind, sessionID); ok {
		fwdCtx, err := s.prepareForwardContext(ctx, provider)
		if err != nil {
			return false, err
		}
		return s.executeAndHandleSession(fwdCtx, sessionID, sessionAlreadyBound, sessionGeneration)
	}

	provider, err := ctx.Profile.ResolveProvider("", ctx.RequestMeta.RequestedModel)
	if err != nil {
		return false, err
	}
	relayDebugf("using default provider: %s", provider.Name)
	fwdCtx, err := s.prepareForwardContext(ctx, provider)
	if err != nil {
		return false, err
	}
	return s.executeAndHandleSession(fwdCtx, sessionID, false, s.sessionGeneration(kind, sessionID))
}

func (s *Server) resolveBoundProvider(
	ctx *RelayContext,
	kind string,
	sessionID string,
) (*routingdomain.Provider, bool, uint64, bool) {
	if sessionID == "" || s.sessionCache == nil {
		return nil, false, 0, false
	}

	boundProviderName, generation, err := s.sessionCache.GetSessionProviderSnapshot(kind, sessionID)
	if err != nil {
		relayWarnf("查询会话绑定失败: %v", err)
		return nil, false, generation, false
	}
	if boundProviderName == "" {
		return nil, false, generation, false
	}

	relayDebugf("session already bound: %s", boundProviderName)
	provider, err := ctx.Profile.ResolveProvider(boundProviderName, ctx.RequestMeta.RequestedModel)
	if err == nil {
		return provider, true, generation, true
	}

	relayWarnf("检测到失效会话绑定，session=%s provider=%s error=%v", sessionID, boundProviderName, err)
	s.clearInvalidSessionBinding(kind, sessionID)
	return nil, false, generation, false
}

func (s *Server) sessionGeneration(kind, sessionID string) uint64 {
	if sessionID == "" || s.sessionCache == nil {
		return 0
	}
	return s.sessionCache.SessionGeneration(kind, sessionID)
}

func (s *Server) clearInvalidSessionBinding(kind, sessionID string) {
	if sessionID == "" {
		return
	}
	if s.sessionCache != nil {
		s.sessionCache.InvalidateSession(kind, sessionID)
	}
	if s.sessionService == nil {
		return
	}
	if err := s.sessionService.Unbind(kind, sessionID); err != nil {
		relayWarnf("清理失效会话绑定失败: %v", err)
	}
}

func (s *Server) prepareForwardContext(ctx *RelayContext, provider *routingdomain.Provider) (*ForwardContext, error) {
	currentBodyBytes := ctx.BodyBytes
	requestedModel := ctx.RequestMeta.RequestedModel
	bodyHasModelField := ctx.RequestMeta.BodyHasModelField
	opts := ctx.RequestMeta.RouteOptions
	if opts == nil {
		opts = &relayRouteOptions{}
	}
	if ctx.Platform == kernel.PlatformClaude && len(provider.RequestBodyReplacements) > 0 {
		modifiedBody, err := applyRequestBodyReplacements(currentBodyBytes, provider.RequestBodyReplacements)
		if err != nil {
			return nil, err
		}
		currentBodyBytes = modifiedBody
		if bodyHasModelField {
			requestedModel = gjson.GetBytes(currentBodyBytes, "model").String()
		}
	}
	rewrite := provider.PlanModelRewrite(requestedModel, bodyHasModelField, opts.forceBodyRewrite)
	if rewrite.ShouldRewriteBody {
		bodyModelValue := rewrite.EffectiveModel
		if opts.bodyModelFormatter != nil {
			bodyModelValue = opts.bodyModelFormatter(rewrite.EffectiveModel)
		}
		relayDebugf("remap model: %s -> %s", requestedModel, bodyModelValue)
		modifiedBody, err := ReplaceModelInRequestBody(currentBodyBytes, bodyModelValue)
		if err != nil {
			return nil, fmt.Errorf("替换模型名失败: %w", err)
		}
		currentBodyBytes = modifiedBody
	}
	targetEndpoint := ctx.RequestMeta.Endpoint
	if opts.endpointMutator != nil {
		newEndpoint, err := opts.endpointMutator(targetEndpoint, requestedModel, rewrite.EffectiveModel)
		if err != nil {
			return nil, err
		}
		targetEndpoint = newEndpoint
	}
	targetQuery := ctx.Query
	if opts.queryMutator != nil {
		targetQuery = opts.queryMutator(cloneMap(ctx.Query), *provider)
	}
	if targetQuery == nil {
		targetQuery = make(map[string]string)
	}
	targetHeaders := ctx.Headers
	if opts.headerMutator != nil {
		targetHeaders = opts.headerMutator(cloneMap(ctx.Headers), *provider)
	}
	if targetHeaders == nil {
		targetHeaders = make(map[string]string)
	}
	return &ForwardContext{
		GinCtx:           ctx.GinCtx,
		Platform:         ctx.Platform,
		Provider:         provider,
		Endpoint:         targetEndpoint,
		Query:            targetQuery,
		Headers:          targetHeaders,
		BodyBytes:        currentBodyBytes,
		IsStream:         ctx.RequestMeta.IsStream,
		IsFast:           ctx.RequestMeta.IsFast,
		IsBackgroundTask: ctx.RequestMeta.IsBackgroundTask,
		Model:            rewrite.EffectiveModel,
		SessionID:        ctx.RequestMeta.SessionID,
	}, nil
}

func (s *Server) executeAndHandleSession(
	fwdCtx *ForwardContext,
	sessionID string,
	sessionAlreadyBound bool,
	sessionGeneration uint64,
) (bool, error) {
	kind := fwdCtx.Platform.String()
	relayDebugf("forwarding provider=%s model=%s", fwdCtx.Provider.Name, fwdCtx.Model)
	startTime := time.Now()
	ok, err := s.forwardRequest(fwdCtx)
	duration := time.Since(startTime)
	if ok {
		s.handleSuccessfulSession(kind, sessionID, fwdCtx.Provider.Name, sessionAlreadyBound, sessionGeneration, fwdCtx.IsBackgroundTask)
		relayDebugf("request succeeded provider=%s duration=%.2fs", fwdCtx.Provider.Name, duration.Seconds())
		return true, nil
	}
	errorMsg := "未知错误"
	if err != nil {
		errorMsg = err.Error()
	}
	relayWarnf("请求失败: provider=%s error=%s duration=%.2fs", fwdCtx.Provider.Name, errorMsg, duration.Seconds())
	return false, err
}

func (s *Server) handleSuccessfulSession(kind, sessionID, providerName string, alreadyBound bool, generation uint64, backgroundTask bool) {
	if sessionID == "" || s.sessionCache == nil {
		return
	}
	if !alreadyBound {
		if err := s.sessionCache.BindSessionToProviderGeneration(kind, sessionID, providerName, generation); err != nil {
			relayWarnf("绑定会话失败: %v", err)
			return
		}
		s.recordCodexSessionVisibility(kind, sessionID, backgroundTask)
		return
	}
	currentGeneration, persist := s.sessionCache.RecordSessionSuccessGeneration(kind, sessionID, providerName, generation)
	if !currentGeneration {
		return
	}
	s.recordCodexSessionVisibility(kind, sessionID, backgroundTask)
	if !persist {
		return
	}
	if s.sessionUpdateWorker != nil {
		s.sessionUpdateWorker.Enqueue(sessionUpdateRequest{platform: kind, sessionID: sessionID, generation: generation})
		return
	}
	if err := s.sessionCache.UpdateSessionSuccessGeneration(kind, sessionID, generation); err != nil {
		relayWarnf("更新会话时间失败: %v", err)
	}
}

func (s *Server) recordCodexSessionVisibility(kind, sessionID string, backgroundTask bool) {
	if kind != kernel.PlatformCodex.String() || s.sessionService == nil {
		return
	}
	if err := s.sessionService.RecordSessionBackground(kind, sessionID, backgroundTask); err != nil {
		relayWarnf("更新 Codex 会话可见性失败: %v", err)
	}
}
