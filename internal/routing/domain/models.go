package domain

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"portico/internal/shared/kernel"
)

type AppPreferences struct {
	ShowHeatmap   bool   `json:"show_heatmap"`
	ShowHomeTitle bool   `json:"show_home_title"`
	ProxyEnabled  bool   `json:"proxy_enabled"`
	ProxyURL      string `json:"proxy_url"`
}

func DefaultAppPreferences() AppPreferences {
	return AppPreferences{
		ShowHeatmap:   true,
		ShowHomeTitle: true,
	}
}

func ValidateProxyURL(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("unsupported proxy scheme: %s (must be http, https, socks5, or socks5h)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("proxy URL must include a host")
	}
	return nil
}

const (
	ProxyModeInherit = ""
	ProxyModeCustom  = "custom"
	ProxyModeDirect  = "direct"
)

type Provider struct {
	ID                      int                      `json:"id"`
	Name                    string                   `json:"name"`
	APIURL                  string                   `json:"apiUrl"`
	APIKey                  string                   `json:"apiKey"`
	OfficialSite            string                   `json:"officialSite"`
	Tint                    string                   `json:"tint"`
	Accent                  string                   `json:"accent"`
	Position                int                      `json:"position"`
	SupportedModels         map[string]bool          `json:"supportedModels,omitempty"`
	ModelMapping            map[string]string        `json:"modelMapping,omitempty"`
	RequestBodyReplacements []RequestBodyReplacement `json:"requestBodyReplacements,omitempty"`
	ProxyMode               string                   `json:"proxyMode,omitempty"`
	ProxyURL                string                   `json:"proxyUrl,omitempty"`
	// InsecureSkipVerify 显式跳过该供应商上游 TLS 证书校验（如本地抓包代理
	// 调试）。默认 false：配置代理不再隐式关闭全局证书校验。
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Archived 归档状态：保留配置但彻底退出路由（不可被解析为默认或
	// 会话绑定供应商），可在 UI 中恢复。
	Archived bool `json:"archived,omitempty"`
	// ArchivedAt 记录最近一次归档时间；历史数据可能为空。
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
}

func (p *Provider) ResolveProxyURL(globalEnabled bool, globalURL string) string {
	switch p.ProxyMode {
	case ProxyModeDirect:
		return ""
	case ProxyModeCustom:
		return p.ProxyURL
	default:
		if globalEnabled && globalURL != "" {
			return globalURL
		}
		return ""
	}
}

type RequestBodyReplacement struct {
	Path string `json:"path"`
	// MatchValue 非空表示条件替换：仅当路径当前值与该 JSON 字面量语义相等时才
	// 改写为 Value；为空（或缺省）表示覆写：无条件改写。旧数据无此字段，自动
	// 视为覆写。
	MatchValue string `json:"matchValue,omitempty"`
	Value      string `json:"value"`
}

type RouteProfile struct {
	Platform          kernel.Platform `json:"platform"`
	DefaultProviderID *int            `json:"defaultProviderId"`
	Providers         []Provider      `json:"providers"`
}

func (p RouteProfile) Normalize() RouteProfile {
	cloned := RouteProfile{
		Platform:  p.Platform,
		Providers: make([]Provider, 0, len(p.Providers)),
	}
	if p.DefaultProviderID != nil {
		defaultID := *p.DefaultProviderID
		cloned.DefaultProviderID = &defaultID
	}
	for idx, provider := range p.Providers {
		next := provider
		next.Name = strings.TrimSpace(next.Name)
		if next.Position <= 0 {
			next.Position = idx + 1
		}
		if !next.Archived {
			next.ArchivedAt = nil
		} else if next.ArchivedAt != nil {
			archivedAt := next.ArchivedAt.UTC()
			next.ArchivedAt = &archivedAt
		}
		if next.SupportedModels == nil {
			next.SupportedModels = map[string]bool{}
		}
		if next.ModelMapping == nil {
			next.ModelMapping = map[string]string{}
		}
		next.RequestBodyReplacements = normalizeRequestBodyReplacements(next.RequestBodyReplacements)
		cloned.Providers = append(cloned.Providers, next)
	}
	slices.SortFunc(cloned.Providers, func(a, b Provider) int {
		switch {
		case a.Position < b.Position:
			return -1
		case a.Position > b.Position:
			return 1
		default:
			return 0
		}
	})
	for idx := range cloned.Providers {
		cloned.Providers[idx].Position = idx + 1
	}
	return cloned
}

func normalizeRequestBodyReplacements(input []RequestBodyReplacement) []RequestBodyReplacement {
	if input == nil {
		return []RequestBodyReplacement{}
	}
	normalized := make([]RequestBodyReplacement, 0, len(input))
	for _, replacement := range input {
		replacement.Path = strings.TrimSpace(replacement.Path)
		if replacement.Path == "" {
			continue
		}
		replacement.Value = strings.TrimSpace(replacement.Value)
		if replacement.Value == "" {
			continue
		}
		replacement.MatchValue = strings.TrimSpace(replacement.MatchValue)
		normalized = append(normalized, replacement)
	}
	return normalized
}

func (p RouteProfile) Validate() error {
	if p.Platform == "" {
		return fmt.Errorf("platform is required")
	}
	seenIDs := make(map[int]struct{}, len(p.Providers))
	seenNames := make(map[string]struct{}, len(p.Providers))
	seenPositions := make(map[int]struct{}, len(p.Providers))
	var defaultProvider *Provider
	for _, provider := range p.Providers {
		if provider.ID == 0 {
			return fmt.Errorf("provider id is required")
		}
		normalizedName := strings.ToLower(strings.TrimSpace(provider.Name))
		if normalizedName == "" {
			return fmt.Errorf("provider name is required")
		}
		if _, ok := seenNames[normalizedName]; ok {
			return fmt.Errorf("duplicate provider name: %s", provider.Name)
		}
		seenNames[normalizedName] = struct{}{}
		if _, ok := seenIDs[provider.ID]; ok {
			return fmt.Errorf("duplicate provider id: %d", provider.ID)
		}
		seenIDs[provider.ID] = struct{}{}
		if provider.Position <= 0 {
			return fmt.Errorf("provider %s has invalid position %d", provider.Name, provider.Position)
		}
		if _, ok := seenPositions[provider.Position]; ok {
			return fmt.Errorf("duplicate provider position: %d", provider.Position)
		}
		seenPositions[provider.Position] = struct{}{}
		if errs := provider.ValidateConfiguration(); len(errs) > 0 {
			return fmt.Errorf("[%s] %s", provider.Name, strings.Join(errs, "; "))
		}
		if p.DefaultProviderID != nil && provider.ID == *p.DefaultProviderID {
			defaultProvider = &provider
		}
	}
	if p.DefaultProviderID != nil {
		if defaultProvider == nil {
			return fmt.Errorf("default provider %d not found", *p.DefaultProviderID)
		}
		if defaultProvider.Archived {
			return fmt.Errorf("默认供应商 %s 已归档，请先恢复或另设默认", defaultProvider.Name)
		}
	}
	return nil
}

func (p RouteProfile) FindProviderByID(id int) *Provider {
	for idx := range p.Providers {
		if p.Providers[idx].ID == id {
			return &p.Providers[idx]
		}
	}
	return nil
}

func (p RouteProfile) FindProviderByName(name string) *Provider {
	for idx := range p.Providers {
		if p.Providers[idx].Name == name {
			return &p.Providers[idx]
		}
	}
	return nil
}

func (p RouteProfile) DefaultProvider() *Provider {
	if p.DefaultProviderID == nil {
		return nil
	}
	return p.FindProviderByID(*p.DefaultProviderID)
}

func (p *RouteProfile) SetDefaultProvider(id *int) error {
	if id == nil {
		p.DefaultProviderID = nil
		return nil
	}
	provider := p.FindProviderByID(*id)
	if provider == nil {
		return fmt.Errorf("provider %d not found", *id)
	}
	value := *id
	p.DefaultProviderID = &value
	return nil
}

func (p *Provider) IsModelSupported(modelName string) bool {
	if (p.SupportedModels == nil || len(p.SupportedModels) == 0) &&
		(p.ModelMapping == nil || len(p.ModelMapping) == 0) {
		return true
	}
	if p.SupportedModels != nil && p.SupportedModels[modelName] {
		return true
	}
	for supportedModel, enabled := range p.SupportedModels {
		if !enabled {
			continue
		}
		if matchWildcard(supportedModel, modelName) {
			return true
		}
	}
	if p.ModelMapping != nil {
		if _, exists := p.ModelMapping[modelName]; exists {
			return true
		}
		for pattern := range p.ModelMapping {
			if matchWildcard(pattern, modelName) {
				return true
			}
		}
	}
	return false
}

func (p *Provider) GetEffectiveModel(requestedModel string) string {
	if len(p.ModelMapping) == 0 {
		return requestedModel
	}
	if mappedModel, exists := p.ModelMapping[requestedModel]; exists {
		return mappedModel
	}
	// 多个通配符同时命中时按特异性取最具体的一条：Go map 遍历序随机，
	// 取"第一个命中"会让同一请求在不同时刻映射到不同模型。
	bestPattern, bestReplacement, found := "", "", false
	for pattern, replacement := range p.ModelMapping {
		if !matchWildcard(pattern, requestedModel) {
			continue
		}
		if !found || wildcardMoreSpecific(pattern, bestPattern) {
			bestPattern, bestReplacement, found = pattern, replacement, true
		}
	}
	if found {
		return applyWildcardMapping(bestPattern, bestReplacement, requestedModel)
	}
	return requestedModel
}

// wildcardMoreSpecific 判断 a 是否比 b 更特异：字面字符更多者优先，
// 平局按字典序，保证结果与 map 遍历顺序无关。
func wildcardMoreSpecific(a, b string) bool {
	la := len(strings.ReplaceAll(a, "*", ""))
	lb := len(strings.ReplaceAll(b, "*", ""))
	if la != lb {
		return la > lb
	}
	return a < b
}

func (p *Provider) ValidateConfiguration() []string {
	errors := make([]string, 0)
	if p.ModelMapping != nil && len(p.ModelMapping) > 0 &&
		p.SupportedModels != nil && len(p.SupportedModels) > 0 {
		for externalModel, internalModel := range p.ModelMapping {
			if strings.Contains(internalModel, "*") {
				continue
			}
			supported := false
			if p.SupportedModels[internalModel] {
				supported = true
			} else {
				for supportedPattern, enabled := range p.SupportedModels {
					if !enabled {
						continue
					}
					if matchWildcard(supportedPattern, internalModel) {
						supported = true
						break
					}
				}
			}
			if !supported {
				errors = append(errors, fmt.Sprintf(
					"模型映射无效：'%s' -> '%s'，目标模型 '%s' 不在 supportedModels 中",
					externalModel, internalModel, internalModel,
				))
			}
		}
	}
	return errors
}

func matchWildcard(pattern, text string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == text
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 2 {
		prefix, suffix := parts[0], parts[1]
		return strings.HasPrefix(text, prefix) && strings.HasSuffix(text, suffix)
	}
	return false
}

func applyWildcardMapping(pattern, replacement, input string) string {
	if !strings.Contains(pattern, "*") || !strings.Contains(replacement, "*") {
		return replacement
	}
	parts := strings.Split(pattern, "*")
	if len(parts) != 2 {
		return replacement
	}
	prefix, suffix := parts[0], parts[1]
	if !strings.HasPrefix(input, prefix) || !strings.HasSuffix(input, suffix) {
		return replacement
	}
	wildcardPart := input[len(prefix) : len(input)-len(suffix)]
	return strings.Replace(replacement, "*", wildcardPart, 1)
}
