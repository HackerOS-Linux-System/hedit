package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type errMsg error
type clearStatusMsg struct{}

type action struct {
	kind string // "insert", "delete", "split", "join"
	y    int
	x    int
	text string
}

type model struct {
	lines          []string
	cursorY        int
	cursorX        int // byte index
	offsetY        int
	offsetX        int // visual column offset
	width          int
	height         int
	filename       string
	modified       bool
	err            error
	status         string
	quitting       bool
	mode           string // "edit", "prompt", "search"
	lexer          chroma.Lexer
	theme          *chroma.Style
	cachedTokens   map[int][]chroma.Token
	undoStack      []action
	redoStack      []action
	searchInput    textinput.Model
	lineNumWidth   int
	targetVisualCol int
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
		Align(lipgloss.Right)
	cursorStyle = lipgloss.NewStyle().
		Reverse(true)
	promptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFF00")).
		Background(lipgloss.Color("#000000")).
		Padding(1)
	saveKey   = key.NewBinding(key.WithKeys("ctrl+o"))
	exitKey   = key.NewBinding(key.WithKeys("ctrl+x"))
	posKey    = key.NewBinding(key.WithKeys("ctrl+c"))
	undoKey   = key.NewBinding(key.WithKeys("ctrl+z"))
	redoKey   = key.NewBinding(key.WithKeys("ctrl+y"))
	searchKey = key.NewBinding(key.WithKeys("ctrl+w"))
	cutKey    = key.NewBinding(key.WithKeys("ctrl+k"))
	copyKey   = key.NewBinding(key.WithKeys("ctrl+p")) // Changed to ctrl+p since ctrl+y is redo
	pasteKey  = key.NewBinding(key.WithKeys("ctrl+u"))
	tabWidth  = 4
)

func initialModel(filename, themeName string) model {
	content := ""
	if _, err := os.Stat(filename); err == nil {
		data, err := os.ReadFile(filename)
		if err == nil {
			content = string(data)
		}
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	theme := styles.Get(themeName)
	if theme == nil {
		theme = styles.Fallback
	}
	searchInput := textinput.New()
	searchInput.Placeholder = "Search for..."
	lineNumWidth := len(fmt.Sprint(len(lines))) + 1
	if lineNumWidth < 4 {
		lineNumWidth = 4
	}
	m := model{
		lines:         lines,
		filename:      filename,
		lexer:         lexer,
		theme:         theme,
		mode:          "edit",
		cachedTokens:  make(map[int][]chroma.Token),
		searchInput:   searchInput,
		lineNumWidth:  lineNumWidth,
		targetVisualCol: 0,
	}
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m *model) save() error {
	// Backup
	if _, err := os.Stat(m.filename); err == nil {
		data, err := os.ReadFile(m.filename)
		if err == nil {
			os.WriteFile(m.filename+".bak", data, 0644)
		}
	}
	content := strings.Join(m.lines, "\n") + "\n"
	return os.WriteFile(m.filename, []byte(content), 0644)
}

func (m *model) invalidateCache(y int) {
	delete(m.cachedTokens, y)
}

func (m *model) updateLineNumWidth() {
	digits := len(fmt.Sprint(len(m.lines)))
	m.lineNumWidth = digits + 1
	if m.lineNumWidth < 4 {
		m.lineNumWidth = 4
	}
}

func (m *model) pushUndo(a action) {
	m.undoStack = append(m.undoStack, a)
	m.redoStack = nil // Clear redo on new action
}

func inverse(a action) action {
	switch a.kind {
	case "insert":
		return action{kind: "delete", y: a.y, x: a.x, text: a.text}
	case "delete":
		return action{kind: "insert", y: a.y, x: a.x, text: a.text}
	case "split":
		return action{kind: "join", y: a.y + 1, x: a.x, text: ""}
	case "join":
		return action{kind: "split", y: a.y, x: a.x, text: ""}
	}
	return action{}
}

func (m *model) applyAction(a action) {
	switch a.kind {
	case "insert":
		line := m.lines[a.y]
		m.lines[a.y] = line[:a.x] + a.text + line[a.x:]
		m.cursorY = a.y
		m.cursorX = a.x + len(a.text)
		m.invalidateCache(a.y)
	case "delete":
		line := m.lines[a.y]
		end := a.x + len(a.text)
		m.lines[a.y] = line[:a.x] + line[end:]
		m.cursorY = a.y
		m.cursorX = a.x
		m.invalidateCache(a.y)
	case "split":
		line := m.lines[a.y]
		left := line[:a.x]
		right := line[a.x:]
		m.lines = append(m.lines[:a.y+1], append([]string{right}, m.lines[a.y+1:]...)...)
		m.lines[a.y] = left
		m.cursorY = a.y + 1
		m.cursorX = 0
		m.invalidateCache(a.y)
		m.invalidateCache(a.y + 1)
	case "join":
		left := m.lines[a.y-1]
		right := m.lines[a.y]
		m.lines[a.y-1] = left + right
		m.lines = append(m.lines[:a.y], m.lines[a.y+1:]...)
		m.cursorY = a.y - 1
		m.cursorX = len(left)
		m.invalidateCache(a.y - 1)
	}
	m.updateLineNumWidth()
	m.modified = true
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 4 // header + footer 2 + status/err
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
		if m.mode == "search" {
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			if msg.String() == "enter" {
				phrase := m.searchInput.Value()
				m.mode = "edit"
				found := false
				startY := m.cursorY
				startX := m.cursorX
				if startX > 0 {
					idx := strings.Index(m.lines[startY][startX:], phrase)
					if idx >= 0 {
						m.cursorX = startX + idx
						found = true
					} else {
						startY++
						startX = 0
					}
				} else {
					startY++
					startX = 0
				}
				if !found {
					for i := startY; i < len(m.lines); i++ {
						idx := strings.Index(m.lines[i], phrase)
						if idx >= 0 {
							m.cursorY = i
							m.cursorX = idx
							found = true
							break
						}
					}
				}
				if !found {
					for i := 0; i < m.cursorY; i++ {
						idx := strings.Index(m.lines[i], phrase)
						if idx >= 0 {
							m.cursorY = i
							m.cursorX = idx
							found = true
							break
						}
					}
				}
				if !found {
					m.status = "Not found: " + phrase
					m.cursorY = startY
					m.cursorX = startX
					return m, m.clearStatusAfter(3 * time.Second)
				}
				m.adjustScroll()
				return m, nil
			}
			if msg.String() == "esc" {
				m.mode = "edit"
				return m, nil
			}
			return m, cmd
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
		case key.Matches(msg, undoKey):
			if len(m.undoStack) > 0 {
				a := m.undoStack[len(m.undoStack)-1]
				m.undoStack = m.undoStack[:len(m.undoStack)-1]
				inv := inverse(a)
				m.applyAction(inv)
				m.redoStack = append(m.redoStack, a)
			}
			return m, nil
		case key.Matches(msg, redoKey):
			if len(m.redoStack) > 0 {
				a := m.redoStack[len(m.redoStack)-1]
				m.redoStack = m.redoStack[:len(m.redoStack)-1]
				inv := inverse(a)
				m.applyAction(inv)
				m.undoStack = append(m.undoStack, a)
			}
			return m, nil
		case key.Matches(msg, searchKey):
			m.mode = "search"
			m.searchInput.Focus()
			return m, nil
		case key.Matches(msg, copyKey):
			if err := clipboard.WriteAll(m.lines[m.cursorY] + "\n"); err != nil {
				m.err = err
			} else {
				m.status = "Line copied"
				return m, m.clearStatusAfter(3 * time.Second)
			}
			return m, nil
		case key.Matches(msg, cutKey):
			line := m.lines[m.cursorY]
			if err := clipboard.WriteAll(line + "\n"); err != nil {
				m.err = err
				return m, nil
			}
			m.lines = append(m.lines[:m.cursorY], m.lines[m.cursorY+1:]...)
			if m.cursorY >= len(m.lines) && len(m.lines) > 0 {
				m.cursorY = len(m.lines) - 1
			}
			m.cursorX = 0
			m.modified = true
			m.updateLineNumWidth()
			m.invalidateCache(m.cursorY)
			// For simplicity, undo for cut/paste not implemented fully
			return m, nil
		case key.Matches(msg, pasteKey):
			text, err := clipboard.ReadAll()
			if err != nil {
				m.err = err
				return m, nil
			}
			pasteLines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			m.lines = append(m.lines[:m.cursorY], append(pasteLines, m.lines[m.cursorY:]...)...)
			m.cursorY += len(pasteLines)
			m.cursorX = 0
			m.modified = true
			m.updateLineNumWidth()
			for i := 0; i < len(pasteLines); i++ {
				m.invalidateCache(m.cursorY - len(pasteLines) + i)
			}
			return m, nil
		}
		// Editing keys
		switch msg.Type {
		case tea.KeyUp:
			if m.cursorY > 0 {
				m.cursorY--
				m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
			}
		case tea.KeyDown:
			if m.cursorY < len(m.lines)-1 {
				m.cursorY++
				m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
			}
		case tea.KeyLeft:
			line := m.lines[m.cursorY]
			if m.cursorX > 0 {
				m.cursorX = utf8Prev(line, m.cursorX)
			} else if m.cursorY > 0 {
				m.cursorY--
				m.cursorX = len(m.lines[m.cursorY])
			}
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		case tea.KeyRight:
			line := m.lines[m.cursorY]
			if m.cursorX < len(line) {
				m.cursorX = utf8Next(line, m.cursorX)
			} else if m.cursorY < len(m.lines)-1 {
				m.cursorY++
				m.cursorX = 0
			}
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		case tea.KeyHome, tea.KeyCtrlA:
			m.cursorX = 0
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		case tea.KeyEnd, tea.KeyCtrlE:
			m.cursorX = len(m.lines[m.cursorY])
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		case tea.KeyBackspace:
			if m.cursorX > 0 {
				line := m.lines[m.cursorY]
				prev := utf8Prev(line, m.cursorX)
				deleted := line[prev:m.cursorX]
				m.lines[m.cursorY] = line[:prev] + line[m.cursorX:]
				m.cursorX = prev
				m.invalidateCache(m.cursorY)
				m.pushUndo(action{kind: "insert", y: m.cursorY, x: m.cursorX, text: deleted})
				m.modified = true
				m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
			} else if m.cursorY > 0 {
				prevLen := len(m.lines[m.cursorY-1])
				m.lines[m.cursorY-1] += m.lines[m.cursorY]
				m.lines = append(m.lines[:m.cursorY], m.lines[m.cursorY+1:]...)
				m.invalidateCache(m.cursorY - 1)
				m.pushUndo(action{kind: "split", y: m.cursorY, x: prevLen, text: ""})
				m.cursorY--
				m.cursorX = prevLen
				m.modified = true
				m.updateLineNumWidth()
				m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
			}
		case tea.KeyDelete:
			line := m.lines[m.cursorY]
			if m.cursorX < len(line) {
				next := utf8Next(line, m.cursorX)
				deleted := line[m.cursorX:next]
				m.lines[m.cursorY] = line[:m.cursorX] + line[next:]
				m.invalidateCache(m.cursorY)
				m.pushUndo(action{kind: "insert", y: m.cursorY, x: m.cursorX, text: deleted})
				m.modified = true
				m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
			} else if m.cursorY < len(m.lines)-1 {
				nextLine := m.lines[m.cursorY+1]
				m.lines[m.cursorY] += nextLine
				m.lines = append(m.lines[:m.cursorY+1], m.lines[m.cursorY+2:]...)
				m.invalidateCache(m.cursorY)
				m.pushUndo(action{kind: "split", y: m.cursorY, x: len(m.lines[m.cursorY]) - len(nextLine), text: ""})
				m.modified = true
				m.updateLineNumWidth()
				m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
			}
		case tea.KeyEnter:
			oldY := m.cursorY
			oldX := m.cursorX
			line := m.lines[oldY]
			left := line[:oldX]
			right := line[oldX:]
			m.lines[oldY] = left
			m.lines = append(m.lines[:oldY+1], append([]string{right}, m.lines[oldY+1:]...)...)
			// Auto-indent
			indent := leadingWhitespace(left)
			m.lines[oldY+1] = indent + m.lines[oldY+1]
			m.cursorY = oldY + 1
			m.cursorX = len(indent)
			m.invalidateCache(oldY)
			m.invalidateCache(oldY + 1)
			m.pushUndo(action{kind: "join", y: oldY + 1, x: len(left), text: ""})
			m.modified = true
			m.updateLineNumWidth()
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		case tea.KeyTab:
			m.insertString("\t")
			m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
		default:
			s := msg.String()
			if len(s) == 1 && unicode.IsGraphic(rune(s[0])) {
				m.insertString(s)
				m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
			}
		}
	case clearStatusMsg:
		m.status = ""
		return m, nil
	}
	m.adjustScroll()
	return m, nil
}

func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return s[:i]
}

func (m *model) insertString(s string) {
	oldX := m.cursorX
	line := m.lines[m.cursorY]
	m.lines[m.cursorY] = line[:oldX] + s + line[oldX:]
	m.cursorX += len(s)
	m.invalidateCache(m.cursorY)
	m.pushUndo(action{kind: "delete", y: m.cursorY, x: oldX, text: s})
	m.modified = true
}

func (m *model) adjustScroll() {
	// Vertical
	if m.cursorY < m.offsetY {
		m.offsetY = m.cursorY
	}
	if m.cursorY >= m.offsetY+m.height {
		m.offsetY = m.cursorY - m.height + 1
	}
	// Horizontal
	textWidth := m.width - m.lineNumWidth - 1
	cursorVisual := visualCol(m.lines[m.cursorY], m.cursorX)
	if cursorVisual < m.offsetX {
		m.offsetX = cursorVisual
	}
	if cursorVisual >= m.offsetX+textWidth {
		m.offsetX = cursorVisual - textWidth + 1
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
	} else if m.mode == "search" {
		statusStr = promptStyle.Render("Search: " + m.searchInput.View())
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
		num := lineNumberStyle.Width(m.lineNumWidth).Align(lipgloss.Right).Render(fmt.Sprintf("%*d", m.lineNumWidth-1, i+1))
		highlighted := m.highlightLine(i)
		renderedLines = append(renderedLines, num+" "+highlighted)
	}
	for i := len(renderedLines); i < m.height; i++ {
		num := lineNumberStyle.Width(m.lineNumWidth).Render(strings.Repeat(" ", m.lineNumWidth))
		renderedLines = append(renderedLines, num+" ~")
	}
	return strings.Join(renderedLines, "\n")
}

func (m model) highlightLine(y int) string {
	raw := m.lines[y]
	textWidth := m.width - m.lineNumWidth - 1
	offsetX := m.offsetX
	tokens, ok := m.cachedTokens[y]
	if !ok {
		iterator, err := m.lexer.Tokenise(nil, raw+"\n")
		if err != nil {
			// fallback
			return m.fallbackHighlight(raw, y, offsetX, textWidth)
		}
		tokens = []chroma.Token{}
		for token := iterator(); token != chroma.EOF; token = iterator() {
			tokens = append(tokens, token)
		}
		m.cachedTokens[y] = tokens
	}
	highlighted := ""
	pos := 0 // visual pos from line start
	cursorVisual := visualCol(raw, m.cursorX)
	for _, token := range tokens {
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
		for j := 0; j < len(value); {
			r, size := utf8.DecodeRuneInString(value[j:])
			if r == '\n' || r == utf8.RuneError {
				break
			}
			w := visualWidth(r, pos)
			skip := 0
			if pos < offsetX {
				skip = offsetX - pos
				if skip >= w {
					pos += w
					j += size
					continue
				}
				pos += skip
				w -= skip
			}
			over := pos + w - (offsetX + textWidth)
			if over > 0 {
				w -= over
				if w <= 0 {
					break
				}
			}
			char := string(r)
			if r == '\t' {
				char = strings.Repeat(" ", w)
			}
			isCursor := (y == m.cursorY) && (pos == cursorVisual)
			if isCursor {
				highlighted += cursorStyle.Render(ls.Render(char))
			} else {
				highlighted += ls.Render(char)
			}
			pos += w
			j += size
			if pos >= offsetX + textWidth {
				break
			}
		}
		if pos >= offsetX + textWidth {
			break
		}
	}
	// Cursor at end
	lineVisualWidth := visualCol(raw, len(raw))
	if y == m.cursorY && m.cursorX == len(raw) {
		if lineVisualWidth >= offsetX && lineVisualWidth < offsetX+textWidth {
			highlighted += cursorStyle.Render(" ")
		}
	}
	// Pad
	currentViewWidth := pos - offsetX
	if currentViewWidth < textWidth {
		highlighted += strings.Repeat(" ", textWidth-currentViewWidth)
	}
	return highlighted
}

func (m model) fallbackHighlight(raw string, y int, offsetX, textWidth int) string {
	highlighted := ""
	pos := 0
	cursorVisual := visualCol(raw, m.cursorX)
	for j := 0; j < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[j:])
		if r == utf8.RuneError {
			break
		}
		w := visualWidth(r, pos)
		skip := 0
		if pos < offsetX {
			skip = offsetX - pos
			if skip >= w {
				pos += w
				j += size
				continue
			}
			pos += skip
			w -= skip
		}
		over := pos + w - (offsetX + textWidth)
		if over > 0 {
			w -= over
			if w <= 0 {
				break
			}
		}
		char := string(r)
		if r == '\t' {
			char = strings.Repeat(" ", w)
		}
		isCursor := (y == m.cursorY) && (pos == cursorVisual)
		if isCursor {
			highlighted += cursorStyle.Render(char)
		} else {
			highlighted += char
		}
		pos += w
		j += size
		if pos >= offsetX + textWidth {
			break
		}
	}
	lineVisualWidth := visualCol(raw, len(raw))
	if y == m.cursorY && m.cursorX == len(raw) {
		if lineVisualWidth >= offsetX && lineVisualWidth < offsetX+textWidth {
			highlighted += cursorStyle.Render(" ")
		}
	}
	currentViewWidth := pos - offsetX
	if currentViewWidth < textWidth {
		highlighted += strings.Repeat(" ", textWidth-currentViewWidth)
	}
	return highlighted
}

func (m model) renderFooter() string {
	line1 := "^G Get Help ^O Write Out ^W Where Is ^K Cut ^P Copy ^U Paste ^C Cur Pos ^Z Undo ^Y Redo"
	line2 := "^X Exit      ^R Read File ^\\ Replace ^J Justify ^T To Spell ^_ Go To Line"
	return footerStyle.Render(line1 + "\n" + line2)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func visualWidth(r rune, col int) int {
	if r == '\t' {
		return tabWidth - (col % tabWidth)
	}
	return runewidth.RuneWidth(r)
}

func visualCol(line string, bytePos int) int {
	col := 0
	j := 0
	for j < bytePos {
		r, size := utf8.DecodeRuneInString(line[j:])
		if size == 0 {
			break
		}
		col += visualWidth(r, col)
		j += size
	}
	return col
}

func bytePosFromVisual(line string, target int) int {
	col := 0
	j := 0
	for j < len(line) {
		r, size := utf8.DecodeRuneInString(line[j:])
		if size == 0 {
			break
		}
		w := visualWidth(r, col)
		if col+w > target {
			break
		}
		col += w
		j += size
	}
	return j
}

func utf8Prev(line string, pos int) int {
	if pos <= 0 {
		return 0
	}
	pos--
	for pos > 0 && !utf8.RuneStart(line[pos]) {
		pos--
	}
	return pos
}

func utf8Next(line string, pos int) int {
	if pos >= len(line) {
		return pos
	}
	_, size := utf8.DecodeRuneInString(line[pos:])
	if size == 0 {
		return len(line)
	}
	return pos + size
}

func main() {
	themeName := flag.String("theme", "monokai", "Chroma theme to use")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: hedit <filename>")
		os.Exit(1)
	}
	filename := args[0]
	p := tea.NewProgram(initialModel(filename, *themeName), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
