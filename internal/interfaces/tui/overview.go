package tui

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	observabilitydomain "portico/internal/observability/domain"
	platformproxydomain "portico/internal/platformproxy/domain"
	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

// platformOverview 是概览页中一个平台的汇总。
type platformOverview struct {
	platform kernel.Platform
	status   platformproxydomain.Status
	profile  routingdomain.RouteProfile
	today    observabilitydomain.LogStats
	err      error
}

type overviewLoadedMsg struct {
	rows []platformOverview
}

type overviewPage struct {
	env    *env
	rows   []platformOverview
	cursor listCursor
	loaded bool
	busy   bool
}

func newOverviewPage(e *env) *overviewPage { return &overviewPage{env: e} }

func (p *overviewPage) title() string   { return "概览" }
func (p *overviewPage) capturing() bool { return false }
func (p *overviewPage) tick() tea.Cmd   { return p.load() }
func (p *overviewPage) help() string {
	return "↑↓ 选择平台 · 空格/e 开关 CLI 接管 · Enter 查看供应商"
}

func (p *overviewPage) load() tea.Cmd {
	client := p.env.client
	return func() tea.Msg {
		platforms := kernel.AllPlatforms()
		rows := make([]platformOverview, len(platforms))
		var wg sync.WaitGroup
		for i, platform := range platforms {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := p.env.ctx()
				defer cancel()
				row := platformOverview{platform: platform}
				name := string(platform)
				if row.status, row.err = client.PlatformStatus(ctx, name); row.err == nil {
					if row.profile, row.err = client.Profile(ctx, name); row.err == nil {
						row.today, row.err = client.Stats(ctx, name, "today")
					}
				}
				rows[i] = row
			}()
		}
		wg.Wait()
		return overviewLoadedMsg{rows: rows}
	}
}

type platformToggledMsg struct {
	platform kernel.Platform
	enabled  bool
	err      error
}

func (p *overviewPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case overviewLoadedMsg:
		p.rows, p.loaded = msg.rows, true
		if !p.busy {
			p.syncCursor()
		}
		return nil
	case platformToggledMsg:
		p.busy = false
		if msg.err != nil {
			return notifyErr("切换 CLI 接管失败", msg.err)
		}
		state := "已关闭"
		if msg.enabled {
			state = "已开启"
		}
		return tea.Batch(p.load(), notify("%s 接管%s", platformLabels[msg.platform], state))
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return nil
}

// syncCursor 让选中行跟随全局平台。
func (p *overviewPage) syncCursor() {
	for i, row := range p.rows {
		if row.platform == p.env.platform {
			p.cursor.index = i
		}
	}
}

func (p *overviewPage) handleKey(msg tea.KeyMsg) tea.Cmd {
	if len(p.rows) == 0 {
		return nil
	}
	if p.cursor.handleKey(msg, len(p.rows)) {
		p.env.platform = p.rows[p.cursor.index].platform
		return nil
	}
	row := p.rows[p.cursor.index]
	switch msg.String() {
	case " ", "e":
		if p.busy || row.err != nil {
			return nil
		}
		p.busy = true
		enable := !row.status.Enabled
		client := p.env.client
		return tea.Batch(notify("正在%s %s 接管…", map[bool]string{true: "开启", false: "关闭"}[enable], platformLabels[row.platform]), func() tea.Msg {
			ctx, cancel := p.env.ctx()
			defer cancel()
			err := client.SetPlatformProxy(ctx, string(row.platform), enable)
			return platformToggledMsg{platform: row.platform, enabled: enable, err: err}
		})
	case "enter":
		return func() tea.Msg { return switchPageMsg{index: 1} }
	}
	return nil
}

func (p *overviewPage) view(width, height int) string {
	if !p.loaded {
		return styleMuted.Render("加载中…")
	}
	cols := []column{
		{title: "平台", width: 12},
		{title: "CLI 接管", width: 9},
		{title: "当前供应商", width: 0},
		{title: "供应商数", width: 8, right: true},
		{title: "今日请求", width: 8, right: true},
		{title: "失败", width: 6, right: true},
		{title: "输入", width: 8, right: true},
		{title: "输出", width: 8, right: true},
		{title: "今日费用", width: 9, right: true},
	}
	rows := make([][]string, len(p.rows))
	for i, row := range p.rows {
		if row.err != nil {
			rows[i] = []string{platformLabels[row.platform], styleErr.Render("错误"), row.err.Error()}
			continue
		}
		failed := fmt.Sprintf("%d", row.today.FailedRequests)
		if row.today.FailedRequests > 0 {
			failed = styleErr.Render(failed)
		}
		active := 0
		for _, provider := range row.profile.Providers {
			if !provider.Archived {
				active++
			}
		}
		rows[i] = []string{
			platformLabels[row.platform],
			yesNo(row.status.Enabled, "● 已开启", "○ 未开启"),
			defaultProviderName(row.profile),
			fmt.Sprintf("%d", active),
			fmt.Sprintf("%d", row.today.TotalRequests),
			failed,
			formatTokens(row.today.InputTokens),
			formatTokens(row.today.OutputTokens),
			formatCost(row.today.CostTotal),
		}
	}
	table := renderTable(cols, rows, &p.cursor, width, len(rows)+1, nil)

	var detail []string
	if p.cursor.index < len(p.rows) {
		row := p.rows[p.cursor.index]
		if row.err == nil {
			detail = append(detail, "", styleBold.Render(platformLabels[row.platform]))
			if row.status.Enabled {
				detail = append(detail, "  CLI 请求经由 "+row.status.BaseURL+" 转发到当前供应商。")
			} else {
				detail = append(detail, "  CLI 未接管：按空格开启后，CLI 配置会指向本地 relay（原配置自动备份）。")
			}
			if row.today.TotalRequests > 0 {
				detail = append(detail, fmt.Sprintf("  今日平均耗时 %s，缓存命中 %d 次，缓存读 %s tokens。",
					formatSeconds(row.today.AvgDurationSec), row.today.CacheHitRequests, formatTokens(row.today.CacheReadTokens)))
			}
		}
	}
	return table + strings.Join(detail, "\n")
}

func defaultProviderName(profile routingdomain.RouteProfile) string {
	if profile.DefaultProviderID == nil {
		return styleMuted.Render("（未设置）")
	}
	for _, provider := range profile.Providers {
		if provider.ID == *profile.DefaultProviderID {
			return provider.Name
		}
	}
	return styleMuted.Render("（未设置）")
}
