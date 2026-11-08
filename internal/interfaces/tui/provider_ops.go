package tui

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	routingdomain "portico/internal/routing/domain"
)

// 以下函数复刻 Web UI（Providers/useProviderDirectory.ts）对路由配置的修改规则，
// 均返回修改后的副本，调用方随后整体 SaveProfile。

func cloneProfile(profile routingdomain.RouteProfile) routingdomain.RouteProfile {
	out := profile
	out.Providers = append([]routingdomain.Provider(nil), profile.Providers...)
	if profile.DefaultProviderID != nil {
		id := *profile.DefaultProviderID
		out.DefaultProviderID = &id
	}
	return out
}

func findProvider(profile routingdomain.RouteProfile, id int) int {
	for i, provider := range profile.Providers {
		if provider.ID == id {
			return i
		}
	}
	return -1
}

func isDefaultProvider(profile routingdomain.RouteProfile, id int) bool {
	return profile.DefaultProviderID != nil && *profile.DefaultProviderID == id
}

// renumber 与前端 persistPlatform 一致：按当前顺序重排 position。
func renumber(profile *routingdomain.RouteProfile) {
	for i := range profile.Providers {
		profile.Providers[i].Position = i + 1
	}
}

func setDefaultProvider(profile routingdomain.RouteProfile, id int) (routingdomain.RouteProfile, error) {
	index := findProvider(profile, id)
	if index < 0 {
		return profile, errors.New("供应商不存在")
	}
	if profile.Providers[index].Archived {
		return profile, errors.New("已归档的供应商不能设为默认，请先恢复")
	}
	out := cloneProfile(profile)
	out.DefaultProviderID = &id
	return out, nil
}

// providerInput 是 TUI 表单可编辑的字段；其余字段（模型映射、请求体改写等）原样保留。
type providerInput struct {
	Name   string
	APIURL string
	APIKey string
}

func validateProviderInput(profile routingdomain.RouteProfile, editingID int, input providerInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return errors.New("名称不能为空")
	}
	for _, provider := range profile.Providers {
		if strings.EqualFold(strings.TrimSpace(provider.Name), name) && provider.ID != editingID {
			return fmt.Errorf("名称 %q 已存在", name)
		}
	}
	u, err := url.Parse(strings.TrimSpace(input.APIURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("API 地址需为 http:// 或 https:// 开头的完整 URL")
	}
	return nil
}

// upsertProvider 新增（editingID == 0）或更新供应商。已有供应商的名称不可修改（后端约束）。
func upsertProvider(profile routingdomain.RouteProfile, editingID int, input providerInput) (routingdomain.RouteProfile, error) {
	if err := validateProviderInput(profile, editingID, input); err != nil {
		return profile, err
	}
	out := cloneProfile(profile)
	if editingID != 0 {
		index := findProvider(out, editingID)
		if index < 0 {
			return profile, errors.New("供应商不存在")
		}
		out.Providers[index].APIURL = strings.TrimSpace(input.APIURL)
		out.Providers[index].APIKey = strings.TrimSpace(input.APIKey)
		renumber(&out)
		return out, nil
	}
	nextID := 1
	for _, provider := range out.Providers {
		nextID = max(nextID, provider.ID+1)
	}
	created := routingdomain.Provider{
		ID:     nextID,
		Name:   strings.TrimSpace(input.Name),
		APIURL: strings.TrimSpace(input.APIURL),
		APIKey: strings.TrimSpace(input.APIKey),
	}
	out.Providers = append([]routingdomain.Provider{created}, out.Providers...)
	renumber(&out)
	return out, nil
}

func setProviderArchived(profile routingdomain.RouteProfile, id int, archived bool) (routingdomain.RouteProfile, error) {
	index := findProvider(profile, id)
	if index < 0 {
		return profile, errors.New("供应商不存在")
	}
	if archived && isDefaultProvider(profile, id) {
		return profile, errors.New("当前默认供应商不能归档，请先切换默认供应商")
	}
	out := cloneProfile(profile)
	out.Providers[index].Archived = archived
	out.Providers[index].ArchivedAt = nil
	renumber(&out)
	return out, nil
}

func deleteProvider(profile routingdomain.RouteProfile, id int) (routingdomain.RouteProfile, error) {
	index := findProvider(profile, id)
	if index < 0 {
		return profile, errors.New("供应商不存在")
	}
	if isDefaultProvider(profile, id) {
		return profile, errors.New("当前默认供应商不能删除，请先切换默认供应商")
	}
	out := cloneProfile(profile)
	out.Providers = append(out.Providers[:index:index], out.Providers[index+1:]...)
	renumber(&out)
	return out, nil
}
