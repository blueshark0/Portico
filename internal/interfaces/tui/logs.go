package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	observabilitydomain "portico/internal/observability/domain"
	"portico/internal/shared/kernel"
)

const logsPageSize = 50

var (
	logRanges      = []string{"today", "last3days", "last7days"}
	logRangeLabels = map[string]string{"today": "今天", "last3days": "近 3 天", "last7days": "近 7 天"}
	logStatuses    = []string{"all", "success", "failed"}
	logStatusLabel = map[string]string{"all": "全部", "success": "成功", "failed": "失败"}
)

type logsLoadedMsg struct {
	platform kernel.Platform
	query    logsQuery
	page     observabilitydomain.RequestLogPage
	err      error
}

type logsQuery struct {
	rangeKey string
	status   string
	page     int
}

type logsPage struct {
	env      *env
	query    logsQuery
	platform kernel.Platform
	data     observabilitydomain.RequestLogPage
	loaded   bool
	err      error
	cursor   listCursor
	detail   bool
}

func newLogsPage(e *env) *logsPage {
	return &logsPage{env: e, query: logsQuery{rangeKey: "today", status: "all", page: 1}}
}

func (p *logsPage) title() string   { return "请求日志" }
func (p *logsPage) capturing() bool { return p.detail }

func (p *logsPage) help() string {
	if p.detail {
		return "Esc/Enter 返回列表"
	}
	return "Enter 详情 · t 时间范围 · s 状态 · ←→ 翻页"
}

// tick 只在第一页且未打开详情时自动刷新，避免翻页浏览时列表跳动。
func (p *logsPage) tick() tea.Cmd {
	if p.query.page != 1 || p.detail {
		return nil
	}
	return p.load()
}

func (p *logsPage) load() tea.Cmd {
	if p.loaded && p.platform != p.env.platform {
		// 切换平台后回到第一页
		p.query.page, p.cursor = 1, listCursor{}
	}
	platform, query, client := p.env.platform, p.query, p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		page, err := client.RequestLogs(ctx, string(platform), query.rangeKey, query.status, query.page, logsPageSize)
		return logsLoadedMsg{platform: platform, query: query, page: page, err: err}
	}
}

func (p *logsPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case logsLoadedMsg:
		if msg.platform != p.env.platform || msg.query != p.query {
			return nil
		}
		if p.platform != msg.platform {
			p.cursor = listCursor{}
		}
		p.platform, p.data, p.err, p.loaded = msg.platform, msg.page, msg.err, true
		if msg.err != nil {
			p.data = observabilitydomain.RequestLogPage{}
		}
		return nil
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return nil
}

func (p *logsPage) totalPages() int {
	if p.data.Total <= 0 {
		return 1
	}
	return int((p.data.Total + logsPageSize - 1) / logsPageSize)
}

func (p *logsPage) handleKey(msg tea.KeyMsg) tea.Cmd {
	if p.detail {
		switch msg.String() {
		case "esc", "enter", "q":
			p.detail = false
		}
		return nil
	}
	if p.cursor.handleKey(msg, len(p.data.Items)) {
		return nil
	}
	switch msg.String() {
	case "enter":
		if len(p.data.Items) > 0 {
			p.detail = true
		}
	case "t":
		p.query.rangeKey = cycle(logRanges, p.query.rangeKey)
		p.query.page, p.cursor = 1, listCursor{}
		return p.load()
	case "s":
		p.query.status = cycle(logStatuses, p.query.status)
		p.query.page, p.cursor = 1, listCursor{}
		return p.load()
	case "right", "l", "n":
		if p.query.page < p.totalPages() {
			p.query.page++
			p.cursor = listCursor{}
			return p.load()
		}
	case "left", "h", "N":
		if p.query.page > 1 {
			p.query.page--
			p.cursor = listCursor{}
			return p.load()
		}
	}
	return nil
}

func cycle(values []string, current string) string {
	for i, v := range values {
		if v == current {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

func (p *logsPage) view(width, height int) string {
	filter := fmt.Sprintf("时间 %s · 状态 %s · 第 %d/%d 页 · 共 %d 条",
		styleBold.Render(logRangeLabels[p.query.rangeKey]),
		styleBold.Render(logStatusLabel[p.query.status]),
		p.query.page, p.totalPages(), p.data.Total)
	switch {
	case !p.loaded || p.platform != p.env.platform:
		return filter + "\n" + styleMuted.Render("加载中…")
	case p.err != nil:
		return filter + "\n" + styleErr.Render("加载失败："+p.err.Error())
	}
	if p.detail && p.cursor.index < len(p.data.Items) {
		return filter + "\n\n" + logDetail(p.data.Items[p.cursor.index])
	}
	cols := []column{
		{title: "时间", width: 19},
		{title: "模型", width: 0},
		{title: "供应商", width: 16},
		{title: "状态", width: 5, right: true},
		{title: "输入", width: 8, right: true},
		{title: "输出", width: 8, right: true},
		{title: "缓存读", width: 8, right: true},
		{title: "首字", width: 6, right: true},
		{title: "耗时", width: 7, right: true},
		{title: "费用", width: 9, right: true},
	}
	rows := make([][]string, len(p.data.Items))
	for i, item := range p.data.Items {
		rows[i] = []string{
			item.CreatedAt,
			item.Model,
			item.Provider,
			httpCode(item.HttpCode),
			formatTokens(int64(item.InputTokens)),
			formatTokens(int64(item.OutputTokens)),
			formatTokens(int64(item.CacheReadTokens)),
			formatSeconds(item.TTFTSec),
			formatSeconds(item.DurationSec),
			costCell(item),
		}
	}
	return filter + "\n" + renderTable(cols, rows, &p.cursor, width, height-1, nil)
}

func httpCode(code int) string {
	text := fmt.Sprintf("%d", code)
	switch {
	case code == 0:
		return styleMuted.Render("-")
	case code >= 200 && code < 300:
		return styleOK.Render(text)
	case code >= 500:
		return styleErr.Render(text)
	default:
		return styleWarn.Render(text)
	}
}

func costCell(item observabilitydomain.RequestLog) string {
	if !item.HasPricing {
		return styleMuted.Render("无定价")
	}
	text := formatCost(item.TotalCost)
	switch item.CostTier {
	case "high":
		return styleErr.Render(text)
	case "medium":
		return styleWarn.Render(text)
	}
	return text
}

func logDetail(item observabilitydomain.RequestLog) string {
	flags := []string{}
	if item.IsStream {
		flags = append(flags, "流式")
	}
	if item.IsFast {
		flags = append(flags, "fast")
	}
	kv := [][2]string{
		{"时间", item.CreatedAt},
		{"模型", item.Model},
		{"供应商", item.Provider},
		{"会话", orDash(item.SessionID)},
		{"HTTP 状态", httpCode(item.HttpCode)},
		{"模式", orDash(strings.Join(flags, " · "))},
		{"首字耗时", formatSeconds(item.TTFTSec)},
		{"总耗时", formatSeconds(item.DurationSec)},
		{"输入 tokens", fmt.Sprintf("%d", item.InputTokens)},
		{"输出 tokens", fmt.Sprintf("%d", item.OutputTokens)},
		{"推理 tokens", fmt.Sprintf("%d", item.ReasoningTokens)},
		{"缓存写入", fmt.Sprintf("%d", item.CacheCreateTokens)},
		{"缓存读取", fmt.Sprintf("%d", item.CacheReadTokens)},
	}
	if item.HasPricing {
		kv = append(kv,
			[2]string{"费用合计", formatCost(item.TotalCost)},
			[2]string{"费用明细", fmt.Sprintf("输入 %s · 输出 %s · 缓存写 %s · 缓存读 %s",
				formatCost(item.InputCost), formatCost(item.OutputCost), formatCost(item.CacheCreateCost), formatCost(item.CacheReadCost))},
		)
	} else {
		kv = append(kv, [2]string{"费用", styleMuted.Render("该模型无定价数据")})
	}
	lines := make([]string, len(kv))
	for i, pair := range kv {
		lines[i] = styleMuted.Render(fit(pair[0], 12, false)) + pair[1]
	}
	return styleBox.Render(strings.Join(lines, "\n"))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
