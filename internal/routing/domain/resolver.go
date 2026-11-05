package domain

import "fmt"

// ResolveProvider 解析可服务本次请求的供应商，是路由的核心领域决策：
// providerName 为空时取 profile 的默认供应商（会话未绑定的场景），
// 并校验归档状态、配置完整性与模型支持。错误信息面向用户展示。
func (p RouteProfile) ResolveProvider(providerName, requestedModel string) (*Provider, error) {
	var provider *Provider
	if providerName == "" {
		provider = p.DefaultProvider()
		if provider == nil {
			return nil, fmt.Errorf("未配置默认供应商")
		}
	} else {
		provider = p.FindProviderByName(providerName)
		if provider == nil {
			return nil, fmt.Errorf("供应商 %s 不存在", providerName)
		}
	}
	if provider.Archived {
		return nil, fmt.Errorf("供应商 %s 已归档", provider.Name)
	}
	if provider.APIURL == "" || provider.APIKey == "" {
		return nil, fmt.Errorf("供应商 %s 配置不完整", provider.Name)
	}
	if requestedModel != "" && !provider.IsModelSupported(requestedModel) {
		return nil, fmt.Errorf("供应商 %s 不支持模型 %s", provider.Name, requestedModel)
	}
	return provider, nil
}

// ModelRewrite 是一次请求的模型改写决策结果。
type ModelRewrite struct {
	// EffectiveModel 是发往供应商的有效模型名（经 ModelMapping 映射）。
	EffectiveModel string
	// ShouldRewriteBody 表示请求体中的 model 字段需要替换。
	ShouldRewriteBody bool
}

// PlanModelRewrite 计算有效模型与请求体是否需要改写。forceRewrite 用于
// 模型同时出现在路径与请求体的平台（如 Gemini）：即使模型未被映射，
// 请求体内的模型名也需要按平台格式规范化重写。
func (p *Provider) PlanModelRewrite(requestedModel string, bodyHasModelField, forceRewrite bool) ModelRewrite {
	effective := p.GetEffectiveModel(requestedModel)
	should := requestedModel != "" && bodyHasModelField &&
		(effective != requestedModel || forceRewrite)
	return ModelRewrite{EffectiveModel: effective, ShouldRewriteBody: should}
}
