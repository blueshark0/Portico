package tui

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	mcpapp "portico/internal/mcpgateway/application"
	mcpdomain "portico/internal/mcpgateway/domain"
	"portico/internal/shared/kernel"
)

// mcpInjectablePlatforms 与 Web UI 一致：Gemini CLI 暂不支持 MCP 注入。
var mcpInjectablePlatforms = []kernel.Platform{kernel.PlatformClaude, kernel.PlatformCodex}

func mcpInjectable(platform kernel.Platform) bool {
	for _, p := range mcpInjectablePlatforms {
		if p == platform {
			return true
		}
	}
	return false
}

type mcpLoadedMsg struct {
	servers   []mcpdomain.McpServer
	statuses  map[int64]mcpapp.UpstreamStatus
	endpoint  string
	injection map[kernel.Platform]bool
	err       error
}

type mcpActionMsg struct {
	done string
	err  error
}

type mcpPage struct {
	env    *env
	data   mcpLoadedMsg
	loaded bool
	cursor listCursor
	busy   bool
}

func newMcpPage(e *env) *mcpPage { return &mcpPage{env: e} }

func (p *mcpPage) title() string   { return "MCP" }
func (p *mcpPage) capturing() bool { return false }
func (p *mcpPage) tick() tea.Cmd   { return p.load() }
func (p *mcpPage) help() string {
	return "空格 启用/停用服务 · i 为当前平台注入/移除网关"
}

func (p *mcpPage) load() tea.Cmd {
	client := p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		out := mcpLoadedMsg{statuses: map[int64]mcpapp.UpstreamStatus{}, injection: map[kernel.Platform]bool{}}
		if out.servers, out.err = client.McpServers(ctx); out.err != nil {
			return out
		}
		statuses, err := client.McpStatuses(ctx)
		if err != nil {
			out.err = err
			return out
		}
		for _, s := range statuses {
			out.statuses[s.ServerID] = s
		}
		if out.endpoint, out.err = client.McpEndpoint(ctx); out.err != nil {
			return out
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, platform := range mcpInjectablePlatforms {
			wg.Add(1)
			go func() {
				defer wg.Done()
				injected, err := client.McpInjection(ctx, string(platform))
				mu.Lock()
				defer mu.Unlock()
				if err != nil && out.err == nil {
					out.err = err
				}
				out.injection[platform] = injected
			}()
		}
		wg.Wait()
		return out
	}
}

func (p *mcpPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case mcpLoadedMsg:
		p.data, p.loaded = msg, true
		return nil
	case mcpActionMsg:
		p.busy = false
		if msg.err != nil {
			return tea.Batch(p.load(), notifyErr("操作失败", msg.err))
		}
		return tea.Batch(p.load(), notify("%s", msg.done))
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return nil
}

func (p *mcpPage) handleKey(msg tea.KeyMsg) tea.Cmd {
	if p.cursor.handleKey(msg, len(p.data.servers)) || !p.loaded || p.busy {
		return nil
	}
	client := p.env.client
	switch msg.String() {
	case " ", "e":
		if len(p.data.servers) == 0 {
			return nil
		}
		p.cursor.clamp(len(p.data.servers))
		server := p.data.servers[p.cursor.index]
		enable := !server.Enabled
		p.busy = true
		return func() tea.Msg {
			ctx, cancel := p.env.ctx()
			defer cancel()
			done := "已停用 " + server.Name
			if enable {
				done = "已启用 " + server.Name
			}
			return mcpActionMsg{done: done, err: client.McpSetEnabled(ctx, server.ID, enable)}
		}
	case "i":
		platform := p.env.platform
		if !mcpInjectable(platform) {
			return notify("%s 暂不支持 MCP 注入", platformLabels[platform])
		}
		inject := !p.data.injection[platform]
		p.busy = true
		return func() tea.Msg {
			ctx, cancel := p.env.ctx()
			defer cancel()
			done := "已从 " + platformLabels[platform] + " 移除 MCP 网关"
			if inject {
				done = "已向 " + platformLabels[platform] + " 注入 MCP 网关"
			}
			return mcpActionMsg{done: done, err: client.McpSetInjection(ctx, string(platform), inject)}
		}
	}
	return nil
}

func (p *mcpPage) view(width, height int) string {
	if !p.loaded {
		return styleMuted.Render("加载中…")
	}
	var header []string
	if p.data.err != nil {
		header = append(header, styleErr.Render("加载失败："+p.data.err.Error()))
	}
	header = append(header, "网关端点 "+styleBold.Render(orDash(p.data.endpoint)))
	var inj []string
	for _, platform := range kernel.AllPlatforms() {
		label := platformLabels[platform]
		if platform == p.env.platform {
			label = styleBold.Render(label)
		}
		state := styleMuted.Render("不支持")
		if mcpInjectable(platform) {
			state = yesNo(p.data.injection[platform], "已注入", "未注入")
		}
		inj = append(inj, label+" "+state)
	}
	header = append(header, "CLI 注入 "+strings.Join(inj, "   "), "")

	cols := []column{
		{title: "名称", width: 18},
		{title: "类型", width: 6},
		{title: "目标", width: 0},
		{title: "启用", width: 6},
		{title: "状态", width: 12},
		{title: "工具", width: 5, right: true},
	}
	servers := p.data.servers
	rows := make([][]string, len(servers))
	for i, server := range servers {
		status := p.data.statuses[server.ID]
		rows[i] = []string{
			server.Name,
			string(server.Type),
			mcpTarget(server),
			yesNo(server.Enabled, "是", "否"),
			mcpState(server, status),
			fmt.Sprintf("%d", status.ToolCount),
		}
	}
	dim := func(i int) lipgloss.Style {
		if !servers[i].Enabled {
			return styleMuted
		}
		return lipgloss.NewStyle()
	}
	footer := ""
	if len(servers) > 0 {
		p.cursor.clamp(len(servers))
		if status := p.data.statuses[servers[p.cursor.index].ID]; status.Error != "" {
			footer = "\n" + styleErr.Render("错误："+status.Error)
		}
	} else {
		footer = "\n" + styleMuted.Render("尚未配置上游 MCP 服务，请在 Web UI 中添加。")
	}
	tableHeight := max(height-len(header)-2, 3)
	return strings.Join(header, "\n") + "\n" + renderTable(cols, rows, &p.cursor, width, tableHeight, dim) + footer
}

func mcpTarget(server mcpdomain.McpServer) string {
	if server.Type == mcpdomain.ServerTypeStdio {
		return strings.TrimSpace(server.Command + " " + strings.Join(server.Args, " "))
	}
	return server.URL
}

func mcpState(server mcpdomain.McpServer, status mcpapp.UpstreamStatus) string {
	if !server.Enabled {
		return "-"
	}
	switch status.State {
	case mcpapp.StateConnected:
		return styleOK.Render("已连接")
	case mcpapp.StateConnecting:
		return styleWarn.Render("连接中")
	case mcpapp.StateFailed:
		return styleErr.Render("失败")
	case mcpapp.StateDisabled:
		return styleMuted.Render("已停用")
	case "":
		return styleMuted.Render("未知")
	default:
		return status.State
	}
}
