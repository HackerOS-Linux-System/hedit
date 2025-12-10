package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error

type model struct {
	textarea textarea.Model
	viewport viewport.Model
	filename string
	err      error
	quitting bool
}

var (
	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Margin(1, 0, 0, 0)

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Margin(1, 0, 0, 0)
)

func initialModel(filename string) model {
	ta := textarea.New()
	ta.Placeholder = "Start typing here..."
	ta.Focus()

	ta.KeyMap.InsertNewline.SetEnabled(true)
	ta.KeyMap.InsertNewline.SetKeys("enter")
	ta.KeyMap.LineNext.SetKeys("down")
	ta.KeyMap.LinePrevious.SetKeys("up")
	ta.KeyMap.CharacterBackward.SetKeys("left")
	ta.KeyMap.CharacterForward.SetKeys("right")
	ta.KeyMap.DeleteCharacterBackward.SetKeys("backspace")
	ta.KeyMap.DeleteCharacterForward.SetKeys("delete")

	// Add save and quit bindings
	ta.KeyMap = textarea.KeyMap{
		CharacterForward:          key.NewBinding(key.WithKeys("right")),
		CharacterBackward:         key.NewBinding(key.WithKeys("left")),
		LineNext:                  key.NewBinding(key.WithKeys("down")),
		LinePrevious:              key.NewBinding(key.WithKeys("up")),
		DeleteCharacterBackward:   key.NewBinding(key.WithKeys("backspace")),
		DeleteCharacterForward:    key.NewBinding(key.WithKeys("delete")),
		InsertNewline:             key.NewBinding(key.WithKeys("enter")),
		DeleteWordBackward:        key.NewBinding(key.WithKeys("ctrl+w")),
		DeleteWordForward:         key.NewBinding(key.WithKeys("ctrl+delete")),
		DeleteAfterCursor:         key.NewBinding(key.WithKeys("ctrl+k")),
		DeleteBeforeCursor:        key.NewBinding(key.WithKeys("ctrl+u")),
		InsertTab:                 key.NewBinding(key.WithKeys("tab")),
		Save:                      key.NewBinding(key.WithKeys("ctrl+s")),
		Quit:                      key.NewBinding(key.WithKeys("ctrl+q")),
	}

	vp := viewport.New(80, 20) // Default size, will be updated

	m := model{
		textarea: ta,
		viewport: vp,
		filename: filename,
	}

	if filename != "" {
		content, err := os.ReadFile(filename)
		if err == nil {
			m.textarea.SetValue(string(content))
		} else {
			m.err = err
		}
	}

	return m
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.textarea.KeyMap.Save):
			if m.filename != "" {
				err := os.WriteFile(m.filename, []byte(m.textarea.Value()), 0644)
				if err != nil {
					m.err = err
				}
			} else {
				m.err = fmt.Errorf("no filename specified")
			}
			return m, nil
		case key.Matches(msg, m.textarea.KeyMap.Quit):
			m.quitting = true
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4 // Reserve space for title and help
		m.textarea.SetWidth(msg.Width)
		m.textarea.SetHeight(msg.Height - 4)

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	// Sync textarea content to viewport for preview if needed, but since we're using textarea as main, perhaps render it directly.

	return m, tea.Batch(taCmd, vpCmd)
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	title := titleStyle.Render("hedit - " + m.filename)

	help := helpStyle.Render("ctrl+s: save • ctrl+q: quit")

	var errStr string
	if m.err != nil {
		errStr = errorStyle.Render(m.err.Error())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		m.textarea.View(),
		help,
		errStr,
	)
}

func main() {
	args := os.Args[1:]
	filename := ""
	if len(args) > 0 {
		filename = args[0]
	}

	p := tea.NewProgram(initialModel(filename), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
