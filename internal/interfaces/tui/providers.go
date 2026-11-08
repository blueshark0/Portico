package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	routingdomain "portico/internal/routing/domain"
	"portico/internal/shared/kernel"
)

type profileLoadedMsg struct {
	platform kernel.Platform
	profile  routingdomain.RouteProfile
	err      error
}

type profileSavedMsg struct {
	platform kernel.Platform
	profile  routingdomain.RouteProfile
	done     string
	err      error
}

type providerTestedMsg struct {
	name   string
	text   string
	failed bool
}

type providersPage struct {
	env      *env
	platform kernel.Platform
	profile  routingdomain.RouteProfile
	loaded   bool
	err      error
	cursor   listCursor
	form     *form
	editID   int
	confirm  confirmPrompt
	busy     bool
}

func newProvidersPage(e *env) *providersPage { return &providersPage{env: e} }

func (p *providersPage) title() string   { return "供应商" }
func (p *providersPage) capturing() bool { return p.form != nil || p.confirm.active() }
func (p *providersPage) tick() tea.Cmd   { return nil }

func (p *providersPage) help() string {
	return "Enter 设为默认 · t 测试连接 · a 新增 · e 编辑 · x 归档/恢复 · D 删除"
}

func (p *providersPage) load() tea.Cmd {
	platform := p.env.platform
	client := p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		profile, err := client.Profile(ctx, string(platform))
		return profileLoadedMsg{platform: platform, profile: profile, err: err}
	}
}

func (p *providersPage) save(profile routingdomain.RouteProfile, done string) tea.Cmd {
	p.busy = true
	platform := p.env.platform
	client := p.env.client
	return func() tea.Msg {
		ctx, cancel := p.env.ctx()
		defer cancel()
		saved, err := client.SaveProfile(ctx, profile)
		return profileSavedMsg{platform: platform, profile: saved, done: done, err: err}
	}
}

func (p *providersPage) selected() (routingdomain.Provider, bool) {
	if !p.loaded || p.platform != p.env.platform || len(p.profile.Providers) == 0 {
		return routingdomain.Provider{}, false
	}
	p.cursor.clamp(len(p.profile.Providers))
	return p.profile.Providers[p.cursor.index], true
}

func (p *providersPage) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case profileLoadedMsg:
		if msg.platform != p.env.platform {
			return nil
		}
		if p.platform != msg.platform {
			p.cursor = listCursor{}
		}
		p.platform, p.profile, p.err, p.loaded = msg.platform, msg.profile, msg.err, true
		return nil
	case profileSavedMsg:
		p.busy = false
		if msg.err != nil {
			if p.form != nil {
				p.form.err = msg.err.Error()
				return nil
			}
			return notifyErr("保存失败", msg.err)
		}
		p.form = nil
		if msg.platform == p.env.platform {
			p.platform, p.profile, p.err, p.loaded = msg.platform, msg.profile, nil, true
		}
		return notify("%s", msg.done)
	case providerTestedMsg:
		if msg.failed {
			return func() tea.Msg { return statusMsg{text: msg.name + "：" + msg.text, err: true} }
		}
		return notify("%s：%s", msg.name, msg.text)
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return nil
}

func (p *providersPage) handleKey(msg tea.KeyMsg) tea.Cmd {
	if p.confirm.active() {
		return p.confirm.handle(msg)
	}
	if p.form != nil {
		return p.handleFormKey(msg)
	}
	if !p.loaded || p.platform != p.env.platform {
		return nil
	}
	if p.cursor.handleKey(msg, len(p.profile.Providers)) {
		return nil
	}
	if msg.String() == "a" {
		p.openForm(routingdomain.Provider{})
		return textinput.Blink
	}
	provider, ok := p.selected()
	if !ok || p.busy {
		return nil
	}
	switch msg.String() {
	case "enter", "d":
		if isDefaultProvider(p.profile, provider.ID) {
			return notify("%s 已是默认供应商", provider.Name)
		}
		next, err := setDefaultProvider(p.profile, provider.ID)
		if err != nil {
			return notifyErr("无法切换", err)
		}
		return p.save(next, fmt.Sprintf("已切换 %s 默认供应商为 %s", platformLabels[p.platform], provider.Name))
	case "t":
		client := p.env.client
		return tea.Batch(notify("正在测试 %s…", provider.Name), func() tea.Msg {
			ctx, cancel := p.env.ctx()
			defer cancel()
			result, err := client.TestProvider(ctx, provider)
			switch {
			case err != nil:
				return providerTestedMsg{name: provider.Name, text: err.Error(), failed: true}
			case !result.Success:
				text := result.Error
				if result.HTTPStatus != 0 {
					text = fmt.Sprintf("HTTP %d %s", result.HTTPStatus, text)
				}
				return providerTestedMsg{name: provider.Name, text: "连接失败 " + text, failed: true}
			default:
				return providerTestedMsg{name: provider.Name, text: fmt.Sprintf("连接正常（HTTP %d，%d ms）", result.HTTPStatus, result.DurationMs)}
			}
		})
	case "e":
		p.openForm(provider)
		return textinput.Blink
	case "x":
		archive := !provider.Archived
		next, err := setProviderArchived(p.profile, provider.ID, archive)
		if err != nil {
			return notifyErr("无法归档", err)
		}
		if !archive {
			return p.save(next, "已恢复 "+provider.Name)
		}
		p.confirm = confirmPrompt{
			text:  fmt.Sprintf("归档 %s？归档后不再参与路由，可随时恢复。", provider.Name),
			onYes: func() tea.Cmd { return p.save(next, "已归档 "+provider.Name) },
		}
	case "D":
		next, err := deleteProvider(p.profile, provider.ID)
		if err != nil {
			return notifyErr("无法删除", err)
		}
		p.confirm = confirmPrompt{
			text:  fmt.Sprintf("永久删除 %s？此操作不可撤销。", provider.Name),
			onYes: func() tea.Cmd { return p.save(next, "已删除 "+provider.Name) },
		}
	}
	return nil
}

func (p *providersPage) openForm(provider routingdomain.Provider) {
	p.editID = provider.ID
	title := "新增供应商 · " + platformLabels[p.env.platform]
	name := newField("名称", provider.Name, "例如 my-relay")
	if provider.ID != 0 {
		title = "编辑供应商 · " + provider.Name
		name.readOnly = true
	}
	apiURL := newField("API 地址", provider.APIURL, "https://api.example.com")
	apiKey := newField("API Key", provider.APIKey, "sk-…")
	apiKey.secret = true
	apiKey.input.EchoMode = textinput.EchoPassword
	apiKey.input.EchoCharacter = '•'
	p.form = newForm(title, name, apiURL, apiKey)
}

func (p *providersPage) handleFormKey(msg tea.KeyMsg) tea.Cmd {
	if p.busy {
		return nil
	}
	result, cmd := p.form.update(msg)
	switch result {
	case formCancel:
		p.form = nil
		return nil
	case formSubmit:
		input := providerInput{Name: p.form.value(0), APIURL: p.form.value(1), APIKey: p.form.value(2)}
		next, err := upsertProvider(p.profile, p.editID, input)
		if err != nil {
			p.form.err = err.Error()
			return nil
		}
		p.form.err = ""
		done := "已新增 " + input.Name
		if p.editID != 0 {
			done = "已保存 " + input.Name
		}
		return p.save(next, done)
	}
	return cmd
}

func (p *providersPage) view(width, height int) string {
	if p.form != nil {
		return p.form.view(width)
	}
	switch {
	case !p.loaded || p.platform != p.env.platform:
		return styleMuted.Render("加载中…")
	case p.err != nil:
		return styleErr.Render("加载失败：" + p.err.Error())
	}
	providers := p.profile.Providers
	cols := []column{
		{title: "", width: 2},
		{title: "名称", width: 20},
		{title: "API 地址", width: 0},
		{title: "API Key", width: 14},
		{title: "模型映射", width: 12},
		{title: "代理", width: 8},
		{title: "状态", width: 6},
	}
	rows := make([][]string, len(providers))
	for i, provider := range providers {
		marker := ""
		if isDefaultProvider(p.profile, provider.ID) {
			marker = "★"
		}
		state := styleOK.Render("可用")
		if provider.Archived {
			state = "已归档"
		}
		rows[i] = []string{marker, provider.Name, provider.APIURL, maskKey(provider.APIKey),
			mappingSummary(provider), proxySummary(provider), state}
	}
	dim := func(i int) lipgloss.Style {
		if providers[i].Archived {
			return styleMuted
		}
		return lipgloss.NewStyle()
	}

	footer := []string{""}
	if p.confirm.active() {
		footer = []string{"", p.confirm.view()}
	} else if provider, ok := p.selected(); ok {
		footer = append(footer, styleMuted.Render(fmt.Sprintf("ID %d · %s", provider.ID, providerDetail(provider))))
	}
	tableHeight := max(height-len(footer), 3)
	return renderTable(cols, rows, &p.cursor, width, tableHeight, dim) + strings.Join(footer, "\n")
}

func maskKey(key string) string {
	runes := []rune(strings.TrimSpace(key))
	switch {
	case len(runes) == 0:
		return "（未设置）"
	case len(runes) <= 8:
		return "••••"
	default:
		return string(runes[:3]) + "…" + string(runes[len(runes)-4:])
	}
}

func mappingSummary(provider routingdomain.Provider) string {
	if len(provider.ModelMappingSchemes) == 0 && len(provider.ModelMapping) == 0 {
		return "-"
	}
	if provider.ActiveModelMappingScheme != "" {
		return provider.ActiveModelMappingScheme
	}
	return fmt.Sprintf("%d 条", len(provider.ModelMapping))
}

func proxySummary(provider routingdomain.Provider) string {
	switch provider.ProxyMode {
	case routingdomain.ProxyModeDirect:
		return "直连"
	case routingdomain.ProxyModeCustom:
		return "自定义"
	default:
		return "跟随全局"
	}
}

func providerDetail(provider routingdomain.Provider) string {
	var parts []string
	if provider.OfficialSite != "" {
		parts = append(parts, "官网 "+provider.OfficialSite)
	}
	if n := len(provider.RequestBodyReplacements); n > 0 {
		parts = append(parts, fmt.Sprintf("请求体改写 %d 条", n))
	}
	if provider.InsecureSkipVerify {
		parts = append(parts, "跳过 TLS 校验")
	}
	if len(parts) == 0 {
		return "模型映射、请求体改写、代理等高级配置请在 Web UI 中修改"
	}
	return strings.Join(parts, " · ")
}
