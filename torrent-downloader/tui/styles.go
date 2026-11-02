package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	Cyan     = lipgloss.Color("#00F0FF")
	Green    = lipgloss.Color("#00FF85")
	Red      = lipgloss.Color("#FF3366")
	Yellow   = lipgloss.Color("#FFB000")
	Purple   = lipgloss.Color("#BF5FFF")
	Orange   = lipgloss.Color("#FF7A00")
	Muted    = lipgloss.Color("#555555")
	Muted2   = lipgloss.Color("#888888")
	White    = lipgloss.Color("#EEEEEE")
	DarkGray = lipgloss.Color("#333333")
	BGPanel  = lipgloss.Color("#111111")

	// Typography
	TitleStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	SpeedStyle = lipgloss.NewStyle().
			Foreground(Green).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(Muted)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Red).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(Yellow)

	// Layout
	HeaderStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(Cyan).
			Padding(0, 2)

	DownloadsBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(DarkGray).
				Padding(0, 1)

	HelpStyle = lipgloss.NewStyle().
			Foreground(Muted).
			MarginTop(1)

	InputPromptStyle = lipgloss.NewStyle().
				Foreground(Yellow).
				Bold(true)

	ProgressFilledStyle = lipgloss.NewStyle().
				Foreground(Green)

	ProgressEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#2a2a2a"))

	// Upload speed
	UploadStyle = lipgloss.NewStyle().
			Foreground(Purple).
			Bold(true)

	// Detail panel
	DetailBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(Cyan).
			Padding(0, 2)

	DetailSectionStyle = lipgloss.NewStyle().
				Foreground(Cyan).
				Bold(true)

	StatLabelStyle = lipgloss.NewStyle().
			Foreground(Muted2).
			Width(22)

	StatValueStyle = lipgloss.NewStyle().
			Foreground(White).
			Bold(true)

	PieceFilledStyle = lipgloss.NewStyle().
				Foreground(Green)

	PiecePendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#2a2a2a"))

	PieceHashingStyle = lipgloss.NewStyle().
				Foreground(Yellow)
)
