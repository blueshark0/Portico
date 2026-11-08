// Package tui 是 Portico 的终端管理界面：通过 portico-server 的管理 API 工作，
// 适用于无桌面环境（如 SSH 登录的服务器）。与 Web UI 共用同一套接口与令牌认证。
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"portico/internal/shared/kernel"
)

const refreshInterval = 5 * time.Second

// page 是一个标签页。数据消息会广播给所有页面（各页只处理自己的消息类型），
// 按键只发给当前页面。
type page interface {
	title() string
	// load 拉取数据；切换到该页、切换平台、手动刷新时调用。
	load() tea.Cmd
	// tick 在定时刷新时调用；返回 nil 表示该页不自动刷新。
	tick() tea.Cmd
	update(msg tea.Msg) tea.Cmd
	view(width, height int) string
	help() string
	// capturing 为 true 时（表单 / 确认框 / 输入框）全局快捷键不生效。
	capturing() bool
}

// env 是各页面共享的上下文。
type env struct {
	client   *Client
	platform kernel.Platform
}

// ctx 返回单次后端请求使用的超时 context。
func (e *env) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestTimeout)
}

var platformLabels = map[kernel.Platform]string{
	kernel.PlatformClaude: "Claude Code",
	kernel.PlatformCodex:  "Codex",
	kernel.PlatformGemini: "Gemini CLI",
}

type tickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Model 是 TUI 的根模型。
type Model struct {
	env       *env
	version   string
	pages     []page
	active    int
	width     int
	height    int
	status    string
	statusErr bool
	showHelp  bool
}

// NewModel 创建根模型；version 为后端版本，仅用于展示。
func NewModel(client *Client, version string) Model {
	e := &env{client: client, platform: kernel.PlatformClaude}
	return Model{
		env:     e,
		version: version,
		pages: []page{
			newOverviewPage(e),
			newProvidersPage(e),
			newLogsPage(e),
			newSessionsPage(e),
			newMcpPage(e),
			newAppLogPage(e),
		},
		width:  100,
		height: 30,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.pages[m.active].load(), tickEvery())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case statusMsg:
		m.status, m.statusErr = msg.text, msg.err
		return m, nil
	case tickMsg:
		var cmd tea.Cmd
		if p := m.pages[m.active]; !p.capturing() {
			cmd = p.tick()
		}
		return m, tea.Batch(cmd, tickEvery())
	case switchPageMsg:
		return m.switchTo(msg.index)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmds []tea.Cmd
	for _, p := range m.pages {
		cmds = append(cmds, p.update(msg))
	}
	return m, tea.Batch(cmds...)
}

// switchPageMsg 由页面发出，请求切换到指定标签页（如概览中 Enter 跳到供应商）。
type switchPageMsg struct{ index int }

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	current := m.pages[m.active]
	if current.capturing() {
		return m, current.update(msg)
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	switch key := msg.String(); key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab":
		return m.switchTo((m.active + 1) % len(m.pages))
	case "shift+tab":
		return m.switchTo((m.active + len(m.pages) - 1) % len(m.pages))
	case "1", "2", "3", "4", "5", "6":
		return m.switchTo(int(key[0] - '1'))
	case "[", "]":
		m.env.platform = cyclePlatform(m.env.platform, key == "]")
		m.status, m.statusErr = "", false
		return m, current.load()
	case "r":
		return m, tea.Batch(current.load(), notify("已刷新"))
	}
	return m, current.update(msg)
}

func (m Model) switchTo(index int) (tea.Model, tea.Cmd) {
	if index < 0 || index >= len(m.pages) {
		return m, nil
	}
	m.active = index
	m.status, m.statusErr = "", false
	return m, m.pages[index].load()
}

func cyclePlatform(current kernel.Platform, forward bool) kernel.Platform {
	all := kernel.AllPlatforms()
	for i, p := range all {
		if p == current {
			if forward {
				return all[(i+1)%len(all)]
			}
			return all[(i+len(all)-1)%len(all)]
		}
	}
	return all[0]
}

func (m Model) View() string {
	width := max(m.width, 40)
	var b strings.Builder

	// 第 1 行：标题与平台
	left := styleTitle.Render("Portico") + styleMuted.Render(fmt.Sprintf("  %s · %s", m.version, m.env.client.BaseURL()))
	var plats []string
	for _, p := range kernel.AllPlatforms() {
		if p == m.env.platform {
			plats = append(plats, styleTabActive.Render(platformLabels[p]))
		} else {
			plats = append(plats, styleTab.Render(platformLabels[p]))
		}
	}
	right := styleMuted.Render("[ ] 平台: ") + strings.Join(plats, "  ")
	b.WriteString(joinEdges(left, right, width) + "\n")

	// 第 2 行：标签页
	var tabs []string
	for i, p := range m.pages {
		label := fmt.Sprintf("%d %s", i+1, p.title())
		if i == m.active {
			tabs = append(tabs, styleTabActive.Render(label))
		} else {
			tabs = append(tabs, styleTab.Render(label))
		}
	}
	b.WriteString(fit(strings.Join(tabs, "   "), width, false) + "\n")
	b.WriteString(styleMuted.Render(strings.Repeat("─", width)) + "\n")

	bodyHeight := max(m.height-5, 3)
	if m.showHelp {
		b.WriteString(placeLines(m.helpView(), width, bodyHeight))
	} else {
		b.WriteString(placeLines(m.pages[m.active].view(width, bodyHeight), width, bodyHeight))
	}
	b.WriteString("\n" + styleMuted.Render(strings.Repeat("─", width)) + "\n")

	// 底部：状态消息优先，否则显示当前页快捷键
	footer := styleMuted.Render(m.pages[m.active].help() + "  · r 刷新 · ? 帮助 · q 退出")
	if m.status != "" {
		if m.statusErr {
			footer = styleErr.Render(m.status)
		} else {
			footer = styleOK.Render(m.status)
		}
	}
	b.WriteString(fit(footer, width, false))
	return b.String()
}

func (m Model) helpView() string {
	lines := []string{
		styleBold.Render("全局快捷键"),
		"  1-6 / Tab / Shift+Tab   切换页面",
		"  [ / ]                   切换平台（Claude Code / Codex / Gemini CLI）",
		"  ↑↓ / j k / PgUp PgDn    移动选中行",
		"  r                       立即刷新（概览、请求日志、会话、MCP、应用日志每 5 秒自动刷新）",
		"  q / Ctrl+C              退出（不影响后端服务）",
		"",
		styleBold.Render("当前页面"),
		"  " + m.pages[m.active].help(),
		"",
		styleMuted.Render("模型映射、请求体改写、配置同步等高级配置请使用 Web UI：" + m.env.client.BaseURL()),
		"",
		styleMuted.Render("按任意键返回"),
	}
	return strings.Join(lines, "\n")
}

// joinEdges 把 left 左对齐、right 右对齐放在同一行。
func joinEdges(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		// 宽度不足时优先保留平台切换（右侧），左侧只留产品名
		left = styleTitle.Render("Portico")
		gap = width - ansi.StringWidth(left) - ansi.StringWidth(right)
		if gap < 1 {
			return fit(right, width, false)
		}
	}
	return left + strings.Repeat(" ", gap) + right
}

// Run 检查连接后启动全屏 TUI。
func Run(target Target) error {
	client := NewClient(target.URL, target.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	health, err := client.Health(ctx)
	if err != nil {
		return fmt.Errorf("无法连接 %s（portico-server 是否在运行？可用 --url 指定地址）: %w", client.BaseURL(), err)
	}
	if err := client.CheckSession(ctx); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return fmt.Errorf("%s 需要访问令牌：在服务器上运行 `portico-server token show` 获取，然后通过 PORTICO_ADMIN_TOKEN 环境变量或 --token 传入", client.BaseURL())
		}
		return err
	}
	_, err = tea.NewProgram(NewModel(client, health.Version), tea.WithAltScreen()).Run()
	return err
}
