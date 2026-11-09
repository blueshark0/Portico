package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorAccent = lipgloss.AdaptiveColor{Light: "#2f5fd0", Dark: "#6e9bff"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#8b95a5"}
	colorOK     = lipgloss.AdaptiveColor{Light: "#15803d", Dark: "#4ade80"}
	colorWarn   = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#fbbf24"}
	colorErr    = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"}

	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styleMuted     = lipgloss.NewStyle().Foreground(colorMuted)
	styleOK        = lipgloss.NewStyle().Foreground(colorOK)
	styleWarn      = lipgloss.NewStyle().Foreground(colorWarn)
	styleErr       = lipgloss.NewStyle().Foreground(colorErr)
	styleBold      = lipgloss.NewStyle().Bold(true)
	styleHeader    = lipgloss.NewStyle().Bold(true).Foreground(colorMuted)
	styleSelected  = lipgloss.NewStyle().Reverse(true)
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Underline(true)
	styleTab       = lipgloss.NewStyle().Foreground(colorMuted)
	styleBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 1)
)

// column 描述表格列；width 为 0 表示弹性列，平分剩余宽度。
type column struct {
	title string
	width int
	right bool
}

// fit 把字符串截断 / 填充到恰好 width 个显示列（正确处理中文宽字符）。
func fit(s string, width int, right bool) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "…")
	}
	pad := width - ansi.StringWidth(s)
	if right {
		return strings.Repeat(" ", pad) + s
	}
	return s + strings.Repeat(" ", pad)
}

func resolveWidths(cols []column, total int) []int {
	widths := make([]int, len(cols))
	fixed, flex := 0, 0
	for i, c := range cols {
		widths[i] = c.width
		fixed += c.width
		if c.width == 0 {
			flex++
		}
	}
	gaps := len(cols) - 1
	remaining := total - fixed - gaps
	if flex > 0 {
		each := remaining / flex
		if each < 6 {
			each = 6
		}
		for i := range widths {
			if widths[i] == 0 {
				widths[i] = each
			}
		}
	}
	return widths
}

// rowStyle 允许调用方对整行着色（如归档供应商置灰）；返回零值样式表示默认。
type rowStyle func(index int) lipgloss.Style

// renderTable 渲染带表头、选中行高亮与纵向滚动的表格，输出恰好 height 行（不足时补空行）。
func renderTable(cols []column, rows [][]string, cur *listCursor, width, height int, style rowStyle) string {
	if height < 2 {
		height = 2
	}
	widths := resolveWidths(cols, width)
	var b strings.Builder
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = fit(c.title, widths[i], c.right)
	}
	b.WriteString(styleHeader.Render(strings.Join(header, " ")))

	bodyHeight := height - 1
	if len(rows) == 0 {
		b.WriteString("\n" + styleMuted.Render("  （暂无数据）"))
		bodyHeight--
		for ; bodyHeight > 0; bodyHeight-- {
			b.WriteString("\n")
		}
		return b.String()
	}
	cur.clamp(len(rows))
	cur.scroll(bodyHeight)
	end := min(cur.offset+bodyHeight, len(rows))
	for i := cur.offset; i < end; i++ {
		cells := make([]string, len(cols))
		for j, c := range cols {
			value := ""
			if j < len(rows[i]) {
				value = rows[i][j]
			}
			cells[j] = fit(value, widths[j], c.right)
		}
		line := strings.Join(cells, " ")
		switch {
		case i == cur.index:
			// 去掉单元格自带的颜色，否则其重置序列会截断整行的反色高亮。
			line = styleSelected.Render(ansi.Strip(line))
		case style != nil:
			line = style(i).Render(line)
		}
		b.WriteString("\n" + line)
	}
	for i := end - cur.offset; i < bodyHeight; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

// listCursor 记录选中行与滚动偏移。
type listCursor struct {
	index  int
	offset int
}

func (c *listCursor) clamp(n int) {
	if n <= 0 {
		c.index, c.offset = 0, 0
		return
	}
	c.index = max(0, min(c.index, n-1))
}

func (c *listCursor) scroll(height int) {
	if height <= 0 {
		return
	}
	if c.index < c.offset {
		c.offset = c.index
	}
	if c.index >= c.offset+height {
		c.offset = c.index - height + 1
	}
	c.offset = max(0, c.offset)
}

// handleKey 处理通用的上下移动按键，返回是否已处理。
func (c *listCursor) handleKey(msg tea.KeyMsg, n int) bool {
	switch msg.String() {
	case "up", "k":
		c.index--
	case "down", "j":
		c.index++
	case "pgup", "ctrl+u":
		c.index -= 10
	case "pgdown", "ctrl+d":
		c.index += 10
	case "home", "g":
		c.index = 0
	case "end", "G":
		c.index = n - 1
	default:
		return false
	}
	c.clamp(n)
	return true
}

// confirmPrompt 是「y 确认 / 其他键取消」的单行确认框。
type confirmPrompt struct {
	text  string
	onYes func() tea.Cmd
}

func (p *confirmPrompt) active() bool { return p != nil && p.onYes != nil }

// handle 处理确认框按键并返回待执行命令；调用后确认框关闭。
func (p *confirmPrompt) handle(msg tea.KeyMsg) tea.Cmd {
	onYes := p.onYes
	p.onYes, p.text = nil, ""
	if msg.String() == "y" || msg.String() == "Y" {
		return onYes()
	}
	return notify("已取消")
}

func (p *confirmPrompt) view() string {
	return styleWarn.Render(p.text + "  [y 确认 / 其他键取消]")
}

// ---- 状态栏消息 ----

type statusMsg struct {
	text string
	err  bool
}

func notify(format string, args ...any) tea.Cmd {
	text := fmt.Sprintf(format, args...)
	return func() tea.Msg { return statusMsg{text: text} }
}

func notifyErr(prefix string, err error) tea.Cmd {
	text := prefix + "：" + err.Error()
	return func() tea.Msg { return statusMsg{text: text, err: true} }
}

// ---- 格式化 ----

func formatTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func formatCost(v float64) string {
	if v == 0 {
		return "-"
	}
	if v < 0.01 {
		return fmt.Sprintf("$%.4f", v)
	}
	return fmt.Sprintf("$%.2f", v)
}

func formatSeconds(v float64) string {
	switch {
	case v <= 0:
		return "-"
	case v < 1:
		return fmt.Sprintf("%dms", int(v*1000))
	default:
		return fmt.Sprintf("%.1fs", v)
	}
}

func yesNo(v bool, yes, no string) string {
	if v {
		return styleOK.Render(yes)
	}
	return styleMuted.Render(no)
}

// placeLines 把内容裁剪 / 补齐到 height 行，并把超宽的行截断到 width 列（窄终端下避免折行错位）。
func placeLines(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "…")
		}
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
