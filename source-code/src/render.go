package src

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
)

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
		statusStr = promptStyle.Render("File modified. Save changes? (Y)es (N)o ^C Cancel")
	} else if m.mode == "search" {
		statusStr = promptStyle.Render("Search: " + m.searchInput.View())
	} else if m.mode == "gotoline" {
		statusStr = promptStyle.Render("Go to line: " + m.searchInput.View())
	} else if m.mode == "replace" {
		if m.replaceStep == 0 {
			statusStr = promptStyle.Render("Find: " + m.searchInput.View())
		} else {
			statusStr = promptStyle.Render("Replace with: " + m.replaceInput.View())
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer, statusStr)
}

func (m model) renderBody() string {
	renderedLines := []string{}
	maxLines := min(m.offsetY+m.height, len(m.lines))

	for i := m.offsetY; i < maxLines; i++ {
		num := lineNumberStyle.Width(m.lineNumWidth).Align(lipgloss.Right).Render(fmt.Sprintf("%*d", m.lineNumWidth-1, i+1))
		highlighted := m.highlightLine(i)
		renderedLines = append(renderedLines, num+" "+highlighted)
	}

	// puste linie na dole (~)
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
			return m.fallbackHighlight(raw, y, offsetX, textWidth)
		}
		tokens = []chroma.Token{}
		for token := iterator(); token != chroma.EOF; token = iterator() {
			tokens = append(tokens, token)
		}
		m.cachedTokens[y] = tokens
	}

	highlighted := ""
	pos := 0
	cursorVisual := visualCol(raw, m.cursorX)
	isSel, selStart, selEnd := m.getSelectionForLine(y)
	bytePos := 0

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

			// pomijanie poza widokiem
			if pos < offsetX {
				skip := offsetX - pos
				if skip >= w {
					pos += w
					j += size
					bytePos += size
					continue
				}
				pos += skip
				w -= skip
			}

			// obcinanie prawej krawędzi
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
			isSelectedChar := isSel && bytePos >= selStart && bytePos < selEnd

			charStyle := ls
			if isSelectedChar {
				charStyle = charStyle.Background(lipgloss.Color("#555555"))
			}

			if isCursor {
				highlighted += cursorStyle.Render(charStyle.Render(char))
			} else {
				highlighted += charStyle.Render(char)
			}

			pos += w
			bytePos += size
			j += size

			if pos >= offsetX+textWidth {
				break
			}
		}
		if pos >= offsetX+textWidth {
			break
		}
	}

	// kursor na końcu linii (gdy jest poza tekstem)
	lineVisualWidth := visualCol(raw, len(raw))
	if y == m.cursorY && m.cursorX == len(raw) {
		if lineVisualWidth >= offsetX && lineVisualWidth < offsetX+textWidth {
			highlighted += cursorStyle.Render(" ")
		}
	}

	// dopełnienie spacjami do końca ekranu
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
	isSel, selStart, selEnd := m.getSelectionForLine(y)
	bytePos := 0

	for j := 0; j < len(raw); {
		r, size := utf8.DecodeRuneInString(raw[j:])
		if r == utf8.RuneError {
			break
		}
		w := visualWidth(r, pos)

		if pos < offsetX {
			skip := offsetX - pos
			if skip >= w {
				pos += w
				j += size
				bytePos += size
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
		isSelectedChar := isSel && bytePos >= selStart && bytePos < selEnd

		if isSelectedChar {
			char = lipgloss.NewStyle().Background(lipgloss.Color("#555555")).Render(char)
		}

		if isCursor {
			highlighted += cursorStyle.Render(char)
		} else {
			highlighted += char
		}

		pos += w
		bytePos += size
		j += size

		if pos >= offsetX+textWidth {
			break
		}
	}

	// kursor na końcu linii
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
	line1 := "^O Save  ^X Exit  ^W Search  ^R Replace  ^K Cut  ^P Copy  ^U Paste  ^Z Undo  ^Y Redo"
	line2 := "^G GoTo Line  ^D Duplicate  ^A Select All  ^/ Comment  ^← Word Left  ^→ Word Right"
	return footerStyle.Render(line1 + "\n" + line2)
}
