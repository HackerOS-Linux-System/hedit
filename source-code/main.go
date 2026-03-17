package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"hedit/src"
)

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
