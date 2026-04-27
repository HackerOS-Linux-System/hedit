package src

import "time"

// ================================================
// TYP AKCJI + WSZYSTKIE METODY UNDO/REDO
// ================================================

type action struct {
	kind  string   // "insert", "delete", "split", "join", "insertLines", "deleteLines"
	y     int
	x     int
	text  string
	lines []string // używane przy Cut / Paste (multi-line)
}

func (m *model) pushUndo(a action) {
	m.undoStack = append(m.undoStack, a)
	m.redoStack = nil // czyszczenie redo przy nowej akcji
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
		case "insertLines":
			return action{kind: "deleteLines", y: a.y, lines: a.lines}
		case "deleteLines":
			return action{kind: "insertLines", y: a.y, lines: a.lines}
	}
	return action{}
}

func (m *model) applyAction(a action) {
	switch a.kind {
		case "insertLines":
			m.lines = append(m.lines[:a.y], append(a.lines, m.lines[a.y:]...)...)
			m.cursorY = a.y + len(a.lines) - 1
			m.cursorX = 0
			for i := range a.lines {
				m.invalidateCache(a.y + i)
			}

		case "deleteLines":
			lineCount := len(a.lines)
			m.lines = append(m.lines[:a.y], m.lines[a.y+lineCount:]...)
			m.cursorY = a.y
			m.cursorX = 0
			m.updateLineNumWidth()
			if a.y < len(m.lines) {
				m.invalidateCache(a.y)
			}

		default:
			// stare akcje pojedyncze
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
	}

	m.updateLineNumWidth()
	m.modified = true
}

// ================================================
// WSTAWIANIE TEKSTU Z BATCHINGIEM UNDO (500ms)
// ================================================

func (m *model) insertString(s string) {
	oldY := m.cursorY
	oldX := m.cursorX
	line := m.lines[oldY]

	m.lines[oldY] = line[:oldX] + s + line[oldX:]
	m.cursorX = oldX + len(s)
	m.invalidateCache(oldY)
	m.modified = true
	m.targetVisualCol = visualCol(m.lines[oldY], m.cursorX)

	// === BATCH UNDO – grupowanie wstawień co < 500ms ===
	if time.Since(m.lastUndoTime) < 500*time.Millisecond && len(m.undoStack) > 0 {
		last := &m.undoStack[len(m.undoStack)-1]
		if last.kind == "delete" && last.y == oldY && last.x == oldX {
			last.text += s
			return
		}
	}

	m.pushUndo(action{kind: "delete", y: oldY, x: oldX, text: s})
	m.lastUndoTime = time.Now()
}

// ================================================
// UN-INDENT (Shift+Tab)
// ================================================

func (m *model) unindentLine() {
	line := m.lines[m.cursorY]
	indent := leadingWhitespace(line)
	if len(indent) == 0 {
		return
	}

	removeLen := tabWidth
	if len(indent) > 0 && indent[0] == '\t' {
		removeLen = 1
	} else if len(indent) < tabWidth {
		removeLen = len(indent)
	}

	removed := indent[:removeLen]
	m.lines[m.cursorY] = line[removeLen:]
	m.cursorX = max(0, m.cursorX-removeLen)
	m.invalidateCache(m.cursorY)

	m.pushUndo(action{kind: "insert", y: m.cursorY, x: m.cursorX, text: removed})
	m.modified = true
	m.lastUndoTime = time.Now()
	m.targetVisualCol = visualCol(m.lines[m.cursorY], m.cursorX)
}
