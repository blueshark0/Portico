package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"portico/internal/shared/kernel"
)

// runCmd 执行命令并展开 tea.Batch，返回产生的消息；超时的命令（如光标闪烁定时器）被忽略。
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(300 * time.Millisecond):
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// send 把按键交给模型并把命令产生的消息继续喂回，直到没有新消息。
func send(t *testing.T, m tea.Model, msgs ...tea.Msg) (tea.Model, bool) {
	t.Helper()
	quit := false
	queue := append([]tea.Msg(nil), msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		if _, ok := msg.(tea.QuitMsg); ok {
			quit = true
			continue
		}
		if _, ok := msg.(tickMsg); ok {
			continue
		}
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		queue = append(queue, runCmd(cmd)...)
	}
	return m, quit
}

func TestModelNavigationAndPlatform(t *testing.T) {
	srv, _, _ := newTestServer(t, "", false)
	var m tea.Model = NewModel(NewClient(srv.URL, ""), "test")
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})

	m, _ = send(t, m, key("2"))
	if got := m.(Model).active; got != 1 {
		t.Fatalf("active = %d, want providers", got)
	}
	m, _ = send(t, m, key("tab"))
	if got := m.(Model).active; got != 2 {
		t.Fatalf("active after tab = %d", got)
	}
	m, _ = send(t, m, key("]"))
	if got := m.(Model).env.platform; got != kernel.PlatformCodex {
		t.Fatalf("platform = %s", got)
	}
	m, _ = send(t, m, key("["), key("["))
	if got := m.(Model).env.platform; got != kernel.PlatformGemini {
		t.Fatalf("platform should wrap backwards, got %s", got)
	}
	if _, quit := send(t, m, key("q")); !quit {
		t.Fatal("q should quit outside of forms")
	}
}

func TestProvidersFormCapturesKeysAndSetDefault(t *testing.T) {
	srv, routing, _ := newTestServer(t, "", false)
	var m tea.Model = NewModel(NewClient(srv.URL, ""), "test")
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 120, Height: 30}, key("2"))
	if view := m.View(); !strings.Contains(view, "p7") {
		t.Fatalf("providers page should list p7:\n%s", view)
	}

	// 表单打开时 q 与数字键应写入输入框，而不是退出 / 切页
	m, quit := send(t, m, key("a"), key("q"), key("3"))
	if quit || m.(Model).active != 1 {
		t.Fatalf("form must capture global keys (quit=%v active=%d)", quit, m.(Model).active)
	}
	providers := m.(Model).pages[1].(*providersPage)
	if providers.form == nil || providers.form.value(0) != "q3" {
		t.Fatalf("form name = %q", providers.form.value(0))
	}
	m, _ = send(t, m, key("esc"))
	if providers.form != nil {
		t.Fatal("esc should close the form")
	}

	// Enter 设为默认 → SaveProfile 收到 defaultProviderId
	m, _ = send(t, m, key("enter"))
	if len(routing.saved) != 1 || routing.saved[0].DefaultProviderID == nil || *routing.saved[0].DefaultProviderID != 7 {
		t.Fatalf("saved = %+v", routing.saved)
	}
	if status := m.(Model).status; !strings.Contains(status, "p7") {
		t.Fatalf("status = %q", status)
	}
}

func TestViewsSurviveTinyTerminals(t *testing.T) {
	srv, _, _ := newTestServer(t, "", false)
	var m tea.Model = NewModel(NewClient(srv.URL, ""), "test")
	for _, size := range []tea.WindowSizeMsg{{Width: 10, Height: 3}, {Width: 40, Height: 8}, {Width: 200, Height: 60}} {
		m, _ = send(t, m, size)
		for i := range m.(Model).pages {
			m, _ = send(t, m, key(string(rune('1'+i))))
			view := m.View()
			if lines := strings.Count(view, "\n") + 1; lines > max(size.Height, 8) {
				t.Fatalf("page %d at %dx%d renders %d lines", i+1, size.Width, size.Height, lines)
			}
		}
	}
}
