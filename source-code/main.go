package main

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error
type clearStatusMsg struct{}

type model struct {
	lines     []string
	cursorY   int
	cursorX   int
	offsetY   int
	offsetX   int
	width     int
	height    int
	filename  string
	modified  bool
	err       error
	status    string
	quitting  bool
	mode      string // "edit" or "prompt"
	lexer     chroma.Lexer
	theme     *chroma.Style
	viewport  viewport.Model
}

var (
	titleStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4")).
	Padding(0, 1).
	Width(80) // Will adjust

	footerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#626262")).
	Height(2)

	helpStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#626262")).
	Margin(1, 0, 0, 0)

	errorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FF0000")).
	Margin(1, 0, 0, 0)

	lineNumberStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#888888")).
	Align(lipgloss.Right).
	Width(6)

	cursorStyle = lipgloss.NewStyle().
	Reverse(true)

	promptStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFF00")).
	Background(lipgloss.Color("#000000")).
	Padding(1)

	saveKey   = key.NewBinding(key.WithKeys("ctrl+o"))
	exitKey   = key.NewBinding(key.WithKeys("ctrl+x"))
	posKey    = key.NewBinding(key.WithKeys("ctrl+c"))
)

func initialModel(filename string) model {
	content := ""
	if _, err := os.Stat(filename); err == nil {
		data, err := os.ReadFile(filename)
		if err == nil {
			content = string(data)
		}
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")

	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	theme := styles.Get("monokai")
	if theme == nil {
		theme = styles.Fallback
	}

	return model{
		lines:    lines,
		filename: filename,
		lexer:    lexer,
		theme:    theme,
		mode:     "edit",
		viewport: viewport.New(80, 20),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m *model) save() error {
	content := strings.Join(m.lines, "\n") + "\n"
	return os.WriteFile(m.filename, []byte(content), 0644)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height - 4 // header + footer 2 + status/err
			m.viewport.Width = msg.Width
			m.viewport.Height = m.height
			titleStyle = titleStyle.Width(msg.Width)
			return m, nil

		case tea.KeyMsg:
			if m.mode == "prompt" {
				switch strings.ToLower(msg.String()) {
					case "y":
						err := m.save()
						if err != nil {
							m.err = err
							m.mode = "edit"
							return m, nil
						}
						m.quitting = true
						return m, tea.Quit
					case "n":
						m.quitting = true
						return m, tea.Quit
					case "ctrl+c", "esc":
						m.mode = "edit"
						return m, nil
				}
				return m, nil
			}

			switch {
				case key.Matches(msg, saveKey):
					err := m.save()
					if err != nil {
						m.err = err
					} else {
						m.modified = false
						m.status = "File saved"
						return m, m.clearStatusAfter(3 * time.Second)
					}
					return m, nil

				case key.Matches(msg, exitKey):
					if !m.modified {
						m.quitting = true
						return m, tea.Quit
					}
					m.mode = "prompt"
					return m, nil

				case key.Matches(msg, posKey):
					m.status = fmt.Sprintf("Line %d/%d Col %d", m.cursorY+1, len(m.lines), m.cursorX+1)
					return m, m.clearStatusAfter(3 * time.Second)
			}

			// Editing keys
			switch msg.Type {
				case tea.KeyUp:
					if m.cursorY > 0 {
						m.cursorY--
						m.adjustCursorX()
					}
				case tea.KeyDown:
					if m.cursorY < len(m.lines)-1 {
						m.cursorY++
						m.adjustCursorX()
					}
				case tea.KeyLeft:
					if m.cursorX > 0 {
						m.cursorX--
					} else if m.cursorY > 0 {
						m.cursorY--
						m.cursorX = len(m.lines[m.cursorY])
					}
				case tea.KeyRight:
					if m.cursorX < len(m.lines[m.cursorY]) {
						m.cursorX++
					} else if m.cursorY < len(m.lines)-1 {
						m.cursorY++
						m.cursorX = 0
					}
				case tea.KeyHome, tea.KeyCtrlA:
					m.cursorX = 0
				case tea.KeyEnd, tea.KeyCtrlE:
					m.cursorX = len(m.lines[m.cursorY])
				case tea.KeyBackspace:
					if m.cursorX > 0 {
						line := m.lines[m.cursorY]
						m.lines[m.cursorY] = line[:m.cursorX-1] + line[m.cursorX:]
						m.cursorX--
						m.modified = true
					} else if m.cursorY > 0 {
						prevLen := len(m.lines[m.cursorY-1])
						m.lines[m.cursorY-1] += m.lines[m.cursorY]
						m.lines = append(m.lines[:m.cursorY], m.lines[m.cursorY+1:]...)
						m.cursorY--
						m.cursorX = prevLen
						m.modified = true
					}
				case tea.KeyDelete:
					line := m.lines[m.cursorY]
					if m.cursorX < len(line) {
						m.lines[m.cursorY] = line[:m.cursorX] + line[m.cursorX+1:]
						m.modified = true
					} else if m.cursorY < len(m.lines)-1 {
						m.lines[m.cursorY] += m.lines[m.cursorY+1]
						m.lines = append(m.lines[:m.cursorY+1], m.lines[m.cursorY+2:]...)
						m.modified = true
					}
				case tea.KeyEnter:
					line := m.lines[m.cursorY]
					m.lines = append(m.lines[:m.cursorY], append([]string{line[:m.cursorX], line[m.cursorX:]}, m.lines[m.cursorY+1:]...)...)
					m.cursorY++
					m.cursorX = 0
					m.modified = true
				case tea.KeyTab:
					m.insertString("    ") // 4 spaces
				default:
					s := msg.String()
					if len(s) == 1 && unicode.IsGraphic(rune(s[0])) {
						m.insertString(s)
					}
			}

				case clearStatusMsg:
					m.status = ""
					return m, nil
	}

	m.adjustScroll()
	return m, nil
}

func (m *model) insertString(s string) {
	line := m.lines[m.cursorY]
	m.lines[m.cursorY] = line[:m.cursorX] + s + line[m.cursorX:]
	m.cursorX += len(s)
	m.modified = true
}

func (m *model) adjustCursorX() {
	lineLen := len(m.lines[m.cursorY])
	if m.cursorX > lineLen {
		m.cursorX = lineLen
	}
}

func (m model) adjustScroll() {
	// Vertical
	if m.cursorY < m.offsetY {
		m.offsetY = m.cursorY
	}
	if m.cursorY >= m.offsetY+m.height {
		m.offsetY = m.cursorY - m.height + 1
	}

	// Horizontal
	textWidth := m.width - 7 // line num 6 + space
	if m.cursorX < m.offsetX {
		m.offsetX = m.cursorX
	}
	if m.cursorX >= m.offsetX+textWidth {
		m.offsetX = m.cursorX - textWidth + 1
	}
}

func (m model) clearStatusAfter(d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return clearStatusMsg{}
	}
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	header := titleStyle.Render("hedit - " + m.filename)
	if m.modified {
		header = titleStyle.Render("hedit - " + m.filename + " *")
	}

	body := m.renderBody()

	footer := m.renderFooter()

	var statusStr string
	if m.status != "" {
		statusStr = helpStyle.Render(m.status)
	} else if m.err != nil {
		statusStr = errorStyle.Render(m.err.Error())
	}

	if m.mode == "prompt" {
		prompt := "File modified. Save changes? (Y)es (N)o ^C Cancel"
		statusStr = promptStyle.Render(prompt)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		body,
		footer,
		statusStr,
	)
}

func (m model) renderBody() string {
	renderedLines := []string{}
	maxLines := min(m.offsetY+m.height, len(m.lines))
	for i := m.offsetY; i < maxLines; i++ {
		num := lineNumberStyle.Render(fmt.Sprintf("%6d", i+1))
		highlighted := m.highlightLine(i)
		renderedLines = append(renderedLines, num+" "+highlighted)
	}
	for i := len(renderedLines); i < m.height; i++ {
		renderedLines = append(renderedLines, lineNumberStyle.Render("      ")+" ~")
	}
	return strings.Join(renderedLines, "\n")
}

func (m model) highlightLine(y int) string {
	raw := m.lines[y]
	textWidth := m.width - 7
	offsetX := m.offsetX
	if offsetX > len(raw) {
		offsetX = 0
	}
	end := offsetX + textWidth
	if end > len(raw) {
		end = len(raw)
	}
	sliced := raw[offsetX:end]

	iterator, err := m.lexer.Tokenise(nil, sliced+"\n")
	if err != nil {
		return sliced // fallback
	}

	highlighted := ""
	pos := 0
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := m.theme.Get(token.Type)
		ls := lipgloss.NewStyle()
		if entry.Colour.IsSet() {
			ls = ls.Foreground(lipgloss.Color(entry.Colour.String()))
		}
		if entry.Background.IsSet() {
			ls = ls.Background(lipgloss.Color(entry.Background.String()))
		}
		if entry.Bold == chroma.Yes {
			ls = ls.Bold(true)
		}
		if entry.Underline == chroma.Yes {
			ls = ls.Underline(true)
		}
		if entry.Italic == chroma.Yes {
			ls = ls.Italic(true)
		}

		value := token.Value
		for _, r := range []rune(value) {
			char := string(r)
			isCursor := (y == m.cursorY) && (pos+offsetX == m.cursorX)
			if isCursor {
				highlighted += cursorStyle.Render(ls.Render(char))
			} else {
				highlighted += ls.Render(char)
			}
			pos++
		}
	}

	if y == m.cursorY && m.cursorX == len(raw) && m.cursorX >= offsetX && m.cursorX <= end {
		highlighted += cursorStyle.Render(" ")
	}

	return highlighted
}

func (m model) renderFooter() string {
	// Mimic nano footer
	line1 := "^G Get Help    ^O Write Out   ^W Where Is    ^K Cut Text    ^J Justify     ^C Cur Pos"
	line2 := "^X Exit        ^R Read File   ^\\ Replace     ^U Paste Text  ^T To Spell    ^_ Go To Line"
	return footerStyle.Render(line1 + "\n" + line2)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Usage: hedit <filename>")
		os.Exit(1)
	}
	filename := args[0]

	p := tea.NewProgram(initialModel(filename), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
