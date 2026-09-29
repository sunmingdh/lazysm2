package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Version is shown in the header, e.g. "v0.1.3". Set by main before starting the UI.
var Version string

func StartUIWithFallback() error {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
