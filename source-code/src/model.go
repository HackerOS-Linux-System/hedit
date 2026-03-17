package src

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error
type clearStatusMsg struct{}

type model struct {
	lines           []string
	cursorY         int
	cursorX         int
	offsetY         int
	offsetX         int
	width           int
	height          int
	filename        string
	modified        bool
	err             error
	status          string
	quitting        bool
	mode            string
	lexer           chroma.Lexer
	theme           *chroma.Style
	cachedTokens    map[int][]chroma.Token
	undoStack       []action
	redoStack       []action
	searchInput     textinput.Model
	lineNumWidth    int
	targetVisualCol int
	softTabs        bool
	lastUndoTime    time.Time
	searchPhrase    string
	hasSelection    bool
	anchorY         int
	anchorX         int
}

var (
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	footerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Height(2)
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Margin(1, 0, 0, 0)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Margin(1, 0, 0, 0)
	lineNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Align(lipgloss.Right)
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	promptStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")).Background(lipgloss.Color("#000000")).Padding(1)

	saveKey      = key.NewBinding(key.WithKeys("ctrl+o"))
	exitKey      = key.NewBinding(key.WithKeys("ctrl+x"))
	posKey       = key.NewBinding(key.WithKeys("ctrl+c"))
	undoKey      = key.NewBinding(key.WithKeys("ctrl+z"))
	redoKey      = key.NewBinding(key.WithKeys("ctrl+y"))
	searchKey    = key.NewBinding(key.WithKeys("ctrl+w"))
	cutKey       = key.NewBinding(key.WithKeys("ctrl+k"))
	copyKey      = key.NewBinding(key.WithKeys("ctrl+p"))
	pasteKey     = key.NewBinding(key.WithKeys("ctrl+u"))
	goToLineKey  = key.NewBinding(key.WithKeys("ctrl+g"))

	// <<< DODANE – brakowało ich w poprzedniej wersji >>>
	copyShiftKey  = key.NewBinding(key.WithKeys("ctrl+shift+c"))
	pasteShiftKey = key.NewBinding(key.WithKeys("ctrl+shift+v"))

	tabWidth = 4
)

func InitialModel(filename, themeName string) model {
	content := ""
	if data, err := os.ReadFile(filename); err == nil {
		content = string(data)
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

	return model{
		lines:           lines,
		filename:        filename,
		lexer:           lexer,
		theme:           theme,
		mode:            "edit",
		cachedTokens:    make(map[int][]chroma.Token),
		searchInput:     searchInput,
		lineNumWidth:    lineNumWidth,
		targetVisualCol: 0,
		softTabs:        true,
		lastUndoTime:    time.Now(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) save() error {
	if _, err := os.Stat(m.filename); err == nil {
		data, _ := os.ReadFile(m.filename)
		os.WriteFile(m.filename+".bak", data, 0644)
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

func (m model) clearStatusAfter(d time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(d)
		return clearStatusMsg{}
	}
}

func (m model) getSelectedText() string {
	if !m.hasSelection {
		return m.lines[m.cursorY] + "\n"
	}
	sY, sX, eY, eX := m.getSelectionBounds()
	var sb strings.Builder
	if sY == eY {
		sb.WriteString(m.lines[sY][sX:eX])
	} else {
		sb.WriteString(m.lines[sY][sX:] + "\n")
		for i := sY + 1; i < eY; i++ {
			sb.WriteString(m.lines[i] + "\n")
		}
		sb.WriteString(m.lines[eY][:eX])
	}
	return sb.String()
}

func (m model) getSelectionBounds() (int, int, int, int) {
	if !m.hasSelection {
		return m.cursorY, m.cursorX, m.cursorY, m.cursorX
	}
	if m.anchorY < m.cursorY || (m.anchorY == m.cursorY && m.anchorX < m.cursorX) {
		return m.anchorY, m.anchorX, m.cursorY, m.cursorX
	}
	return m.cursorY, m.cursorX, m.anchorY, m.anchorX
}

func (m model) getSelectionForLine(y int) (bool, int, int) {
	if !m.hasSelection {
		return false, 0, 0
	}
	sY, sX, eY, eX := m.getSelectionBounds()
	if y < sY || y > eY {
		return false, 0, 0
	}
	if y == sY && y == eY {
		return true, min(sX, eX), max(sX, eX)
	}
	if y == sY {
		return true, sX, len(m.lines[y])
	}
	if y == eY {
		return true, 0, eX
	}
	return true, 0, len(m.lines[y])
}

func (m *model) deleteSelection() {
	if !m.hasSelection {
		return
	}
	sY, sX, eY, eX := m.getSelectionBounds()
	if sY == eY {
		line := m.lines[sY]
		m.lines[sY] = line[:sX] + line[eX:]
		m.cursorY = sY
		m.cursorX = sX
		m.invalidateCache(sY)
	} else {
		line := m.lines[sY][:sX] + m.lines[eY][eX:]
		m.lines[sY] = line
		m.lines = append(m.lines[:sY+1], m.lines[eY+1:]...)
		m.cursorY = sY
		m.cursorX = sX
		m.updateLineNumWidth()
		m.invalidateCache(sY)
	}
	m.hasSelection = false
	m.modified = true
}

func (m *model) adjustScroll() {
	if m.cursorY < m.offsetY {
		m.offsetY = m.cursorY
	}
	if m.cursorY >= m.offsetY+m.height {
		m.offsetY = m.cursorY - m.height + 1
	}
	textWidth := m.width - m.lineNumWidth - 1
	cursorVisual := visualCol(m.lines[m.cursorY], m.cursorX)
	if cursorVisual < m.offsetX {
		m.offsetX = cursorVisual
	}
	if cursorVisual >= m.offsetX+textWidth {
		m.offsetX = cursorVisual - textWidth + 1
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height - 4
			titleStyle = titleStyle.Width(msg.Width)
			return m, nil

		case clearStatusMsg:
			m.status = ""
			return m, nil

		case tea.KeyMsg:
			if m.mode == "prompt" {
				switch strings.ToLower(msg.String()) {
					case "y":
						if err := m.save(); err != nil {
							m.err = err
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

			if m.mode == "search" || m.mode == "gotoline" {
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)

				if msg.String() == "enter" {
					if m.mode == "gotoline" {
						if line, err := strconv.Atoi(m.searchInput.Value()); err == nil && line > 0 {
							m.cursorY = min(len(m.lines)-1, line-1)
							m.cursorX = 0
							m.targetVisualCol = 0
						} else {
							m.status = "Nieprawidłowy numer linii"
							return m, m.clearStatusAfter(3 * time.Second)
						}
						m.mode = "edit"
						m.adjustScroll()
						return m, nil
					}

					phrase := m.searchInput.Value()
					if phrase == "" {
						m.mode = "edit"
						return m, nil
					}
					m.searchPhrase = phrase
					m.mode = "edit"

					originalY := m.cursorY
					originalX := m.cursorX
					found := false

					if originalX <= len(m.lines[originalY]) {
						idx := strings.Index(m.lines[originalY][originalX:], phrase)
						if idx >= 0 {
							m.cursorY = originalY
							m.cursorX = originalX + idx
							found = true
						}
					}
					if !found {
						for i := originalY + 1; i < len(m.lines); i++ {
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
						for i := 0; i < originalY; i++ {
							idx := strings.Index(m.lines[i], phrase)
							if idx >= 0 {
								m.cursorY = i
								m.cursorX = idx
								found = true
								break
							}
						}
					}
					if !found && originalX > 0 {
						idx := strings.Index(m.lines[originalY][:originalX], phrase)
						if idx >= 0 {
							m.cursorY = originalY
							m.cursorX = idx
							found = true
						}
					}

					if !found {
						m.status = "Not found: " + phrase
						m.cursorY = originalY
						m.cursorX = originalX
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
					if err := m.save(); err != nil {
						m.err = err
					} else {
						m.modified = false
						m.status = "File saved"
						return m, m.clearStatusAfter(3 * time.Second)
					}
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
						m.applyAction(inverse(a))
						m.redoStack = append(m.redoStack, a)
					}
					return m, nil
				case key.Matches(msg, redoKey):
					if len(m.redoStack) > 0 {
						a := m.redoStack[len(m.redoStack)-1]
						m.redoStack = m.redoStack[:len(m.redoStack)-1]
						m.applyAction(inverse(a))
						m.undoStack = append(m.undoStack, a)
					}
					return m, nil
				case key.Matches(msg, searchKey):
					if m.searchPhrase != "" {
						found := false
						for i := m.cursorY; i < len(m.lines); i++ {
							start := 0
							if i == m.cursorY {
								start = m.cursorX + 1
							}
							idx := strings.Index(m.lines[i][start:], m.searchPhrase)
							if idx >= 0 {
								m.cursorY = i
								m.cursorX = start + idx
								found = true
								break
							}
						}
						if !found {
							m.status = "No more occurrences"
							return m, m.clearStatusAfter(3 * time.Second)
						}
						m.adjustScroll()
						return m, nil
					}
					m.mode = "search"
					m.searchInput.Placeholder = "Search for..."
					m.searchInput.SetValue("")
					m.searchInput.Focus()
					return m, nil
				case key.Matches(msg, goToLineKey):
					m.mode = "gotoline"
					m.searchInput.Placeholder = "Go to line: "
					m.searchInput.SetValue("")
					m.searchInput.Focus()
					return m, nil
				case key.Matches(msg, copyKey), key.Matches(msg, copyShiftKey):
					if err := clipboard.WriteAll(m.getSelectedText()); err != nil {
						m.err = err
					} else {
						m.status = "Copied"
						if m.hasSelection {
							m.hasSelection = false
						}
						return m, m.clearStatusAfter(3 * time.Second)
					}
				case key.Matches(msg, cutKey):
					textToCut := m.getSelectedText()
					if err := clipboard.WriteAll(textToCut); err != nil {
						m.err = err
					} else {
						m.status = "Cut"
					}

					if m.hasSelection {
						sY, sX, eY, eX := m.getSelectionBounds()
						if sY == eY {
							deleted := m.lines[sY][sX:eX]
							m.pushUndo(action{kind: "insert", y: sY, x: sX, text: deleted})
						}
						m.deleteSelection()
						m.hasSelection = false
					} else {
						line := m.lines[m.cursorY]
						m.pushUndo(action{kind: "insertLines", y: m.cursorY, lines: []string{line}})
						m.lines = append(m.lines[:m.cursorY], m.lines[m.cursorY+1:]...)
						if m.cursorY >= len(m.lines) && len(m.lines) > 0 {
							m.cursorY = len(m.lines) - 1
						}
						m.cursorX = 0
						m.updateLineNumWidth()
						m.invalidateCache(m.cursorY)
						m.lastUndoTime = time.Now()
					}
					m.modified = true
					return m, nil

				case key.Matches(msg, pasteKey), key.Matches(msg, pasteShiftKey):
					text, err := clipboard.ReadAll()
					if err != nil {
						m.err = err
						return m, nil
					}
					pasteLines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
					m.pushUndo(action{kind: "deleteLines", y: m.cursorY, lines: pasteLines})
					m.lines = append(m.lines[:m.cursorY], append(pasteLines, m.lines[m.cursorY:]...)...)
					m.cursorY += len(pasteLines)
					m.cursorX = 0
					m.modified = true
					m.updateLineNumWidth()
					for i := range pasteLines {
						m.invalidateCache(m.cursorY - len(pasteLines) + i)
					}
					m.lastUndoTime = time.Now()
					return m, nil
			}

			didReplaceSelection := false
			if m.hasSelection {
				switch msg.Type {
					case tea.KeyRunes, tea.KeyEnter, tea.KeyTab, tea.KeyBackspace, tea.KeyDelete:
						sY, sX, eY, eX := m.getSelectionBounds()
						if sY == eY {
							deleted := m.lines[sY][sX:eX]
							m.pushUndo(action{kind: "insert", y: sY, x: sX, text: deleted})
						}
						m.deleteSelection()
						m.hasSelection = false
						m.modified = true
						didReplaceSelection = true
				}
			}

			shift := strings.HasPrefix(msg.String(), "shift+")
			if shift && !m.hasSelection {
				m.anchorY = m.cursorY
				m.anchorX = m.cursorX
				m.hasSelection = true
			} else if !shift && m.hasSelection {
				m.hasSelection = false
			}

			switch msg.Type {
				case tea.KeyPgUp:
					m.cursorY = max(0, m.cursorY-m.height+1)
					m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
					m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
				case tea.KeyPgDown:
					m.cursorY = min(len(m.lines)-1, m.cursorY+m.height-1)
					m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
					m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
				case tea.KeyUp:
					if m.cursorY > 0 {
						m.cursorY--
						m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
					}
					m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
				case tea.KeyDown:
					if m.cursorY < len(m.lines)-1 {
						m.cursorY++
						m.cursorX = bytePosFromVisual(m.lines[m.cursorY], m.targetVisualCol)
					}
					m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
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
				case tea.KeyTab:
					s := "\t"
					if m.softTabs {
						s = strings.Repeat(" ", tabWidth)
					}
					m.insertString(s)
					m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
				case tea.KeyBackspace:
					if didReplaceSelection {
						m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
					} else if m.cursorX > 0 {
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
					if didReplaceSelection {
						m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
					} else {
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
					}
				case tea.KeyEnter:
					oldY := m.cursorY
					oldX := m.cursorX
					line := m.lines[oldY]
					left := line[:oldX]
					right := line[oldX:]
					m.lines[oldY] = left
					m.lines = append(m.lines[:oldY+1], append([]string{right}, m.lines[oldY+1:]...)...)
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
				default:
					s := msg.String()
					if len(s) == 1 && unicode.IsGraphic(rune(s[0])) {
						m.insertString(s)
						m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
					}
			}

			if msg.String() == "shift+tab" {
				m.unindentLine()
			}

			m.adjustScroll()
			return m, nil

				default:
					return m, nil
	}
}
