package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	observabilitydomain "portico/internal/observability/domain"
)

const (
	appLogMaxEntries = 2000
	appLogBatchLines = 500
)

var appLogLevels = []string{"", "ERROR", "WARN", "INFO", "DEBUG"}

type appLogQuery struct {
	level   string
	keyword string
}

type appLogLoadedMsg struct {
	query  appLogQuery
	offset int64
	chunk  observabilitydomain.AppLogChunk
	err    error
}

// appLogPage 增量跟随 ~/.portico/app.log（经由后端读取，远程同样可用）。
type appLogPage struct {
	env       *env
	query     appLogQuery
	entries   []observabilitydomain.AppLogEntry
	offset    int64
	truncated bool
	loaded    bool
	err       error
	// scroll 是距底部的行数；0 表示跟随最新日志。
	scroll    int
	searching bool
	search    textinput.Model
	inflight  bool
}

func newAppLogPage(e *env) *appLogPage {
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "关键字（Enter 应用，Esc 取消）"
	search.CharLimit = 200
	return &appLogPage{env: e, search: search}
}

func (p *appLogPage) title() string   { return "应用日志" }
func (p *appLogPage) capturing() bool { return p.searching }
func (p *appLogPage) tick() tea.Cmd   { return p.fetch() }

func (p *appLogPage) help() string {
	if p.searching {
		return "Enter 应用关键字 · Esc 取消"
	}
	return "↑↓/PgUp/PgDn 滚动 · End 跟随最新 · L 级别 · / 关键字 · c 清空过滤"
}

// load 在切换到本页 / 手动刷新时调用：已加载时只做增量读取。
func (p *appLogPage) load() tea.Cmd { return p.fetch() }

func (p *appLogPage) fetch() tea.Cmd {
	if p.inflight {
		return nil
	}
	p.inflight = true
	query, offset, client := p.query, p.offset, p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		chunk, err := client.TailAppLog(ctx, offset, appLogBatchLines, query.level, query.keyword)
		return appLogLoadedMsg{query: query, offset: offset, chunk: chunk, err: err}
	}
}

// reset 清空已加载日志并按新过滤条件从尾部重新读取。
func (p *appLogPage) reset() tea.Cmd {
	p.entries, p.offset, p.scroll, p.loaded, p.truncated = nil, 0, 0, false, false
	p.inflight = false
	return p.fetch()
}

func (p *appLogPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case appLogLoadedMsg:
		if msg.query != p.query || msg.offset != p.offset {
			return nil // 过滤条件已变化，丢弃旧结果（新请求仍在进行）
		}
		p.inflight = false
		p.loaded, p.err = true, msg.err
		if msg.err != nil {
			return nil
		}
		if msg.chunk.Reset || msg.offset == 0 {
			p.entries = nil
			p.truncated = msg.chunk.Truncated
		}
		added := len(msg.chunk.Entries)
		p.entries = append(p.entries, msg.chunk.Entries...)
		if over := len(p.entries) - appLogMaxEntries; over > 0 {
			p.entries = append([]observabilitydomain.AppLogEntry(nil), p.entries[over:]...)
			p.truncated = true
		}
		p.offset = msg.chunk.NextOffset
		if p.scroll > 0 {
			// 正在回看历史时保持视图位置不动
			p.scroll += added
		}
		return nil
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return nil
}

func (p *appLogPage) handleKey(msg tea.KeyMsg) tea.Cmd {
	if p.searching {
		switch msg.String() {
		case "enter":
			p.searching = false
			p.search.Blur()
			p.query.keyword = strings.TrimSpace(p.search.Value())
			return p.reset()
		case "esc":
			p.searching = false
			p.search.Blur()
			p.search.SetValue(p.query.keyword)
			return nil
		}
		var cmd tea.Cmd
		p.search, cmd = p.search.Update(msg)
		return cmd
	}
	switch msg.String() {
	case "up", "k":
		p.scroll++
	case "down", "j":
		p.scroll--
	case "pgup", "ctrl+u":
		p.scroll += 10
	case "pgdown", "ctrl+d":
		p.scroll -= 10
	case "home", "g":
		p.scroll = len(p.entries)
	case "end", "G":
		p.scroll = 0
	case "L":
		p.query.level = cycle(appLogLevels, p.query.level)
		return p.reset()
	case "/":
		p.searching = true
		p.search.SetValue(p.query.keyword)
		p.search.CursorEnd()
		return p.search.Focus()
	case "c":
		if p.query != (appLogQuery{}) {
			p.query = appLogQuery{}
			p.search.SetValue("")
			return p.reset()
		}
	}
	p.scroll = max(0, min(p.scroll, max(len(p.entries)-1, 0)))
	return nil
}

func (p *appLogPage) view(width, height int) string {
	level := p.query.level
	if level == "" {
		level = "全部"
	}
	header := "级别 " + styleBold.Render(level) + " · 关键字 " + styleBold.Render(orDash(p.query.keyword))
	if p.scroll == 0 {
		header += " · " + styleOK.Render("跟随中")
	} else {
		header += " · " + styleWarn.Render("已暂停跟随（End 恢复）")
	}
	if p.truncated {
		header += styleMuted.Render(" · 仅显示最近部分")
	}
	lines := []string{header}
	if p.searching {
		lines = append(lines, p.search.View())
	}
	switch {
	case !p.loaded:
		return strings.Join(append(lines, styleMuted.Render("加载中…")), "\n")
	case p.err != nil:
		return strings.Join(append(lines, styleErr.Render("读取失败："+p.err.Error())), "\n")
	case len(p.entries) == 0:
		return strings.Join(append(lines, styleMuted.Render("（没有匹配的日志）")), "\n")
	}

	// 自底向上填充可见行；多行消息按行展开。
	available := height - len(lines)
	end := len(p.entries) - p.scroll
	var body []string
	for i := end - 1; i >= 0 && len(body) < available; i-- {
		rendered := renderLogEntry(p.entries[i], width)
		for j := len(rendered) - 1; j >= 0 && len(body) < available; j-- {
			body = append(body, rendered[j])
		}
	}
	for i, j := 0, len(body)-1; i < j; i, j = i+1, j-1 {
		body[i], body[j] = body[j], body[i]
	}
	return strings.Join(append(lines, body...), "\n")
}

func renderLogEntry(entry observabilitydomain.AppLogEntry, width int) []string {
	prefix := ""
	if entry.Time != "" {
		prefix = styleMuted.Render(entry.Time) + " "
	}
	switch entry.Level {
	case "ERROR":
		prefix += styleErr.Render("ERROR") + " "
	case "WARN":
		prefix += styleWarn.Render("WARN ") + " "
	case "INFO":
		prefix += styleOK.Render("INFO ") + " "
	case "DEBUG":
		prefix += styleMuted.Render("DEBUG") + " "
	}
	if entry.Source != "" {
		prefix += styleMuted.Render(entry.Source) + " "
	}
	msgLines := strings.Split(entry.Message, "\n")
	out := make([]string, 0, len(msgLines))
	for i, line := range msgLines {
		if i == 0 {
			out = append(out, fit(prefix+line, width, false))
		} else {
			out = append(out, fit("    "+line, width, false))
		}
	}
	return out
}
