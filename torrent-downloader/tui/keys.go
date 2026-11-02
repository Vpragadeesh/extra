package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard shortcuts for the TUI
type KeyMap struct {
	Quit          key.Binding
	Add           key.Binding
	Delete        key.Binding
	Pause         key.Binding
	Up            key.Binding
	Down          key.Binding
	SpeedUp       key.Binding
	SpeedDown     key.Binding
	NoLimit       key.Binding
	Detail        key.Binding
	NetworkToggle key.Binding
}

// Keys is the global key binding configuration
var Keys = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Add: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d", "x"),
		key.WithHelp("d", "delete"),
	),
	Pause: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "pause/resume"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	SpeedUp: key.NewBinding(
		key.WithKeys("+", "="),
		key.WithHelp("+", "speed +1MB/s"),
	),
	SpeedDown: key.NewBinding(
		key.WithKeys("-"),
		key.WithHelp("-", "speed -1MB/s"),
	),
	NoLimit: key.NewBinding(
		key.WithKeys("0"),
		key.WithHelp("0", "unlimited"),
	),
	Detail: key.NewBinding(
		key.WithKeys("tab", "i"),
		key.WithHelp("tab", "details"),
	),
	NetworkToggle: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "toggle network"),
	),
}
