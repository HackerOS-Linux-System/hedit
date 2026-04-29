package src

import (
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
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

// wordStart returns the byte index of the start of the word at/before pos
func wordStart(line string, pos int) int {
	if pos <= 0 {
		return 0
	}
	// skip whitespace backwards
	i := pos
	for i > 0 {
		prev := utf8Prev(line, i)
		r, _ := utf8.DecodeRuneInString(line[prev:])
		if !unicode.IsSpace(r) {
			break
		}
		i = prev
	}
	// skip word chars backwards
	for i > 0 {
		prev := utf8Prev(line, i)
		r, _ := utf8.DecodeRuneInString(line[prev:])
		if unicode.IsSpace(r) {
			break
		}
		i = prev
	}
	return i
}

// wordEnd returns the byte index of the end of the word at/after pos
func wordEnd(line string, pos int) int {
	n := len(line)
	if pos >= n {
		return n
	}
	// skip whitespace
	i := pos
	for i < n {
		r, size := utf8.DecodeRuneInString(line[i:])
		if !unicode.IsSpace(r) {
			break
		}
		i += size
	}
	// skip word chars
	for i < n {
		r, size := utf8.DecodeRuneInString(line[i:])
		if unicode.IsSpace(r) {
			break
		}
		i += size
	}
	return i
}
