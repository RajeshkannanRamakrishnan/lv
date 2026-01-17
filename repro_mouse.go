package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	mouseEvent string
}

func (m model) Init() tea.Cmd {
	return tea.EnableMouseAllMotion
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.MouseMsg:
		m.mouseEvent = fmt.Sprintf("Mouse Msg: Type=%v, X=%d, Y=%d, Button=%v", msg.Type, msg.X, msg.Y, msg.Button)
        // Store in a file just in case TUI is weird
        f, _ := os.OpenFile("mouse_repro.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
        fmt.Fprintf(f, "%s\n", m.mouseEvent)
        f.Close()
	}
	return m, nil
}

func (m model) View() string {
	s := "Move mouse or scroll here.\n\n"
    if m.mouseEvent == "" {
        s += "No mouse events received yet."
    } else {
        s += m.mouseEvent
    }
	s += "\n\nPress q to quit."
	return s
}

func main() {
	p := tea.NewProgram(model{}, tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
