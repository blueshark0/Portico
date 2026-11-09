package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"portico/internal/interfaces/wails"
	"portico/internal/shared/kernel"
)

type sessionsLoadedMsg struct {
	platform kernel.Platform
	items    []wails.SessionSummary
	err      error
}

type sessionUnboundMsg struct {
	sessionID string
	err       error
}

type sessionsPage struct {
	env      *env
	platform kernel.Platform
	items    []wails.SessionSummary
	loaded   bool
	err      error
	cursor   listCursor
	confirm  confirmPrompt
}

func newSessionsPage(e *env) *sessionsPage { return &sessionsPage{env: e} }

func (p *sessionsPage) title() string   { return "会话" }
func (p *sessionsPage) capturing() bool { return p.confirm.active() }
func (p *sessionsPage) tick() tea.Cmd   { return p.load() }
func (p *sessionsPage) help() string {
	return "u 解除会话绑定（下次请求重新按默认供应商路由）"
}

func (p *sessionsPage) load() tea.Cmd {
	platform, client := p.env.platform, p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		items, err := client.Sessions(ctx, string(platform))
		return sessionsLoadedMsg{platform: platform, items: items, err: err}
	}
}

func (p *sessionsPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		if msg.platform != p.env.platform {
			return nil
		}
		if p.platform != msg.platform {
			p.cursor = listCursor{}
		}
		p.platform, p.items, p.err, p.loaded = msg.platform, msg.items, msg.err, true
		return nil
	case sessionUnboundMsg:
		if msg.err != nil {
			return notifyErr("解除绑定失败", msg.err)
		}
		return tea.Batch(p.load(), notify("已解除会话 %s 的绑定", shortID(msg.sessionID)))
	case tea.KeyMsg:
		if p.confirm.active() {
			return p.confirm.handle(msg)
		}
		if p.cursor.handleKey(msg, len(p.items)) {
			return nil
		}
		if msg.String() == "u" && p.loaded && p.platform == p.env.platform && len(p.items) > 0 {
			p.cursor.clamp(len(p.items))
			item := p.items[p.cursor.index]
			platform, client := p.platform, p.env.client
			p.confirm = confirmPrompt{
				text: fmt.Sprintf("解除会话 %s 与 %s 的绑定？", shortID(item.SessionID), item.ProviderName),
				onYes: func() tea.Cmd {
					return func() tea.Msg {
						ctx, cancel := p.env.ctx()
						defer cancel()
						return sessionUnboundMsg{sessionID: item.SessionID, err: client.UnbindSession(ctx, string(platform), item.SessionID)}
					}
				},
			}
		}
	}
	return nil
}

func (p *sessionsPage) view(width, height int) string {
	switch {
	case !p.loaded || p.platform != p.env.platform:
		return styleMuted.Render("加载中…")
	case p.err != nil:
		return styleErr.Render("加载失败：" + p.err.Error())
	}
	cols := []column{
		{title: "会话", width: 14},
		{title: "绑定供应商", width: 18},
		{title: "请求", width: 6, right: true},
		{title: "输入", width: 8, right: true},
		{title: "输出", width: 8, right: true},
		{title: "费用", width: 9, right: true},
		{title: "模型", width: 0},
		{title: "最近成功", width: 11},
	}
	rows := make([][]string, len(p.items))
	for i, item := range p.items {
		rows[i] = []string{
			shortID(item.SessionID),
			item.ProviderName,
			fmt.Sprintf("%d", item.Requests),
			formatTokens(int64(item.InputTokens)),
			formatTokens(int64(item.OutputTokens)),
			formatCost(item.TotalCost),
			strings.Join(item.Models, ", "),
			relativeTime(item.LastSuccessAt),
		}
	}
	footer := ""
	if p.confirm.active() {
		footer = "\n" + p.confirm.view()
	} else if len(p.items) > 0 {
		p.cursor.clamp(len(p.items))
		item := p.items[p.cursor.index]
		footer = "\n" + styleMuted.Render(fmt.Sprintf("会话 ID %s · 创建于 %s", item.SessionID, item.CreatedAt.Local().Format("2006-01-02 15:04:05")))
	} else {
		footer = "\n" + styleMuted.Render(fmt.Sprintf("%s 的会话在空闲 %s 后过期。", platformLabels[p.platform], p.platform.SessionTimeout()))
	}
	return renderTable(cols, rows, &p.cursor, width, height-2, nil) + "\n" + footer
}

func shortID(id string) string {
	runes := []rune(id)
	if len(runes) <= 12 {
		return id
	}
	return string(runes[:8]) + "…" + string(runes[len(runes)-3:])
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	default:
		return t.Local().Format("01-02 15:04")
	}
}
