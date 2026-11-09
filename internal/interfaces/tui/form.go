package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formField 是表单中的一个单行输入框。
type formField struct {
	label    string
	input    textinput.Model
	readOnly bool
	secret   bool
}

// form 是一个简单的多字段表单：Tab/↑↓ 切换字段，Enter 在最后一个字段（或 Ctrl+S）提交，Esc 取消。
type form struct {
	title  string
	fields []*formField
	focus  int
	err    string
}

type formResult int

const (
	formContinue formResult = iota
	formSubmit
	formCancel
)

func newField(label, value, placeholder string) *formField {
	in := textinput.New()
	in.Placeholder = placeholder
	in.SetValue(value)
	in.CharLimit = 2048
	in.Prompt = ""
	return &formField{label: label, input: in}
}

func newForm(title string, fields ...*formField) *form {
	f := &form{title: title, fields: fields}
	f.focus = -1
	f.move(1)
	return f
}

func (f *form) value(i int) string { return strings.TrimSpace(f.fields[i].input.Value()) }

// move 把焦点移到下一个（delta>0）或上一个可编辑字段。
func (f *form) move(delta int) {
	n := len(f.fields)
	for step := 0; step < n; step++ {
		f.focus = (f.focus + delta + n) % n
		if !f.fields[f.focus].readOnly {
			break
		}
	}
	for i, field := range f.fields {
		if i == f.focus {
			field.input.Focus()
		} else {
			field.input.Blur()
		}
	}
}

func (f *form) lastEditable() int {
	for i := len(f.fields) - 1; i >= 0; i-- {
		if !f.fields[i].readOnly {
			return i
		}
	}
	return -1
}

func (f *form) update(msg tea.KeyMsg) (formResult, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return formCancel, nil
	case "ctrl+s":
		return formSubmit, nil
	case "tab", "down":
		f.move(1)
		return formContinue, nil
	case "shift+tab", "up":
		f.move(-1)
		return formContinue, nil
	case "enter":
		if f.focus == f.lastEditable() {
			return formSubmit, nil
		}
		f.move(1)
		return formContinue, nil
	case "ctrl+r":
		// 切换密钥字段的明文显示
		for _, field := range f.fields {
			if field.secret {
				if field.input.EchoMode == textinput.EchoPassword {
					field.input.EchoMode = textinput.EchoNormal
				} else {
					field.input.EchoMode = textinput.EchoPassword
				}
			}
		}
		return formContinue, nil
	}
	field := f.fields[f.focus]
	var cmd tea.Cmd
	field.input, cmd = field.input.Update(msg)
	return formContinue, cmd
}

func (f *form) view(width int) string {
	boxWidth := min(max(width-4, 40), 90)
	labelWidth := 10
	inputWidth := boxWidth - labelWidth - 6
	var lines []string
	lines = append(lines, styleTitle.Render(f.title), "")
	for i, field := range f.fields {
		field.input.Width = inputWidth
		label := fit(field.label, labelWidth, false)
		marker := "  "
		if i == f.focus {
			marker = styleTitle.Render("› ")
			label = styleBold.Render(label)
		} else {
			label = styleMuted.Render(label)
		}
		value := field.input.View()
		if field.readOnly {
			value = styleMuted.Render(field.input.Value() + "（不可修改）")
		}
		lines = append(lines, marker+label+value)
	}
	lines = append(lines, "")
	if f.err != "" {
		lines = append(lines, styleErr.Render(f.err))
	}
	lines = append(lines, styleMuted.Render("Tab/↑↓ 切换字段 · Enter 下一项/保存 · Ctrl+S 保存 · Ctrl+R 显示/隐藏密钥 · Esc 取消"))
	return styleBox.Width(boxWidth).Render(strings.Join(lines, "\n"))
}
