package tui

import (
	"context"
	"strings"

	"torrent-downloader/config"
	"torrent-downloader/manager"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AppMode tracks whether the TUI is in normal browsing, text input, or detail mode.
type AppMode int

const (
	ModeNormal AppMode = iota
	ModeInput
	ModeDetail
)

// DownloadItem represents a single torrent download shown in the list.
type DownloadItem struct {
	ID       int
	Name     string
	State    string // "waiting", "downloading", "paused", "complete", "error"
	Snapshot manager.ProgressSnapshot
	Error    error
}

// Model is the root Bubble Tea model for the entire TUI.
type Model struct {
	manager   *manager.Manager
	config    *config.Config
	downloads []DownloadItem
	selected  int
	mode      AppMode
	textInput textinput.Model
	width     int
	height    int
	events    <-chan manager.ManagerEvent
	ctx       context.Context
	cancel    context.CancelFunc
	preload   string
}

// NewModel creates the initial TUI model. If preload is non-empty, that
// magnet/torrent will be added automatically on startup.
func NewModel(mgr *manager.Manager, cfg *config.Config, ctx context.Context, cancel context.CancelFunc, preload string) Model {
	ti := textinput.New()
	ti.Placeholder = "magnet:?xt=urn:btih:... or /path/to/file.torrent"
	ti.CharLimit = 4096
	ti.Width = 80

	return Model{
		manager:   mgr,
		config:    cfg,
		downloads: []DownloadItem{},
		mode:      ModeNormal,
		textInput: ti,
		events:    mgr.Events,
		ctx:       ctx,
		cancel:    cancel,
		preload:   preload,
	}
}

// ── Bubble Tea messages ──────────────────────────────────────────

type eventMsg manager.ManagerEvent
type addedMsg struct{}
type preloadMsg string
type errorMsg struct{ err error }

func waitForEvent(events <-chan manager.ManagerEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

func (m Model) addTorrent(resource string) tea.Cmd {
	mgr := m.manager
	ctx := m.ctx
	return func() tea.Msg {
		_, err := mgr.AddTorrent(ctx, resource)
		if err != nil {
			return errorMsg{err: err}
		}
		return addedMsg{}
	}
}

// ── Init / Update / View ─────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForEvent(m.events)}
	if m.preload != "" {
		resource := m.preload
		cmds = append(cmds, func() tea.Msg { return preloadMsg(resource) })
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 10
		if m.textInput.Width < 40 {
			m.textInput.Width = 40
		}
		return m, nil

	case preloadMsg:
		return m, m.addTorrent(string(msg))

	case addedMsg:
		return m, nil

	case errorMsg:
		// Show the error as a failed entry in the download list.
		m.downloads = append(m.downloads, DownloadItem{
			ID:    -1,
			Name:  "Failed to add torrent",
			State: "error",
			Error: msg.err,
		})
		if len(m.downloads) == 1 {
			m.selected = 0
		}
		return m, nil

	case tea.KeyMsg:
		// ── Input mode ──────────────────────────────────────
		if m.mode == ModeInput {
			switch msg.String() {
			case "ctrl+c":
				m.cancel()
				return m, tea.Quit
			case "enter":
				value := m.textInput.Value()
				if value != "" {
					m.mode = ModeNormal
					m.textInput.SetValue("")
					m.textInput.Blur()
					return m, m.addTorrent(value)
				}
			case "esc":
				m.mode = ModeNormal
				m.textInput.SetValue("")
				m.textInput.Blur()
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// ── Detail mode ─────────────────────────────────────
		if m.mode == ModeDetail {
			switch msg.String() {
			case "ctrl+c", "q":
				m.cancel()
				return m, tea.Quit
			case "esc", "tab", "i":
				m.mode = ModeNormal
			case "n":
				// Toggle network enabled/disabled
				if len(m.downloads) > 0 && m.selected < len(m.downloads) {
					item := m.downloads[m.selected]
					if len(item.Snapshot.Networks) > 0 {
						// For now, toggle the first network - in future could add sub-selection
						net := item.Snapshot.Networks[0]
						m.manager.ToggleNetwork(net.InterfaceID)
					}
				}
			}
			return m, nil
		}

		// ── Normal mode ─────────────────────────────────────
		switch {
		case key.Matches(msg, Keys.Quit):
			m.cancel()
			return m, tea.Quit

		case key.Matches(msg, Keys.Add):
			m.mode = ModeInput
			cmd := m.textInput.Focus()
			return m, cmd

		case key.Matches(msg, Keys.Detail):
			if len(m.downloads) > 0 {
				m.mode = ModeDetail
			}

		case key.Matches(msg, Keys.Delete):
			if len(m.downloads) > 0 && m.selected < len(m.downloads) {
				item := m.downloads[m.selected]
				m.manager.RemoveTorrent(item.ID)
				m.downloads = append(m.downloads[:m.selected], m.downloads[m.selected+1:]...)
				if m.selected >= len(m.downloads) && m.selected > 0 {
					m.selected--
				}
			}

		case key.Matches(msg, Keys.Pause):
			if len(m.downloads) > 0 && m.selected < len(m.downloads) {
				item := &m.downloads[m.selected]
				if item.State == "downloading" {
					m.manager.PauseTorrent(item.ID)
					item.State = "paused"
				} else if item.State == "paused" {
					m.manager.ResumeTorrent(item.ID)
					item.State = "downloading"
				}
			}

		case key.Matches(msg, Keys.Up):
			if m.selected > 0 {
				m.selected--
			}
		case key.Matches(msg, Keys.Down):
			if m.selected < len(m.downloads)-1 {
				m.selected++
			}

		case key.Matches(msg, Keys.SpeedUp):
			limit := m.manager.GetSpeedLimit() + 1024*1024
			m.manager.SetSpeedLimit(limit)
		case key.Matches(msg, Keys.SpeedDown):
			limit := m.manager.GetSpeedLimit() - 1024*1024
			if limit < 0 {
				limit = 0
			}
			m.manager.SetSpeedLimit(limit)
		case key.Matches(msg, Keys.NoLimit):
			m.manager.SetSpeedLimit(0)
		}

	// ── Manager events ──────────────────────────────────────
	case eventMsg:
		ev := manager.ManagerEvent(msg)
		idx := m.findDownload(ev.TorrentID)

		switch ev.Type {
		case manager.EventMetadataWaiting:
			if idx == -1 {
				m.downloads = append(m.downloads, DownloadItem{
					ID:    ev.TorrentID,
					Name:  "Connecting...",
					State: "waiting",
				})
				if len(m.downloads) == 1 {
					m.selected = 0
				}
			}
		case manager.EventMetadataReceived:
			if idx != -1 {
				m.downloads[idx].Name = ev.Snapshot.Name
				m.downloads[idx].State = "downloading"
				m.downloads[idx].Snapshot = ev.Snapshot
			}
		case manager.EventProgress:
			if idx != -1 {
				m.downloads[idx].Snapshot = ev.Snapshot
				if m.downloads[idx].Name == "Connecting..." {
					m.downloads[idx].Name = ev.Snapshot.Name
				}
			}
		case manager.EventComplete:
			if idx != -1 {
				m.downloads[idx].State = "complete"
				m.downloads[idx].Snapshot = ev.Snapshot
			}
		case manager.EventError:
			if idx != -1 {
				m.downloads[idx].State = "error"
				m.downloads[idx].Error = ev.Err
			} else {
				m.downloads = append(m.downloads, DownloadItem{
					ID:    ev.TorrentID,
					Name:  "Error",
					State: "error",
					Error: ev.Err,
				})
			}
		}
		return m, waitForEvent(m.events)
	}

	return m, nil
}

func (m Model) View() string {
	header := renderHeader(m.manager.GetSpeedLimit(), m.width)

	// ── Detail panel mode ────────────────────────────────────────
	if m.mode == ModeDetail && len(m.downloads) > 0 && m.selected < len(m.downloads) {
		item := m.downloads[m.selected]
		boxWidth := m.width - 4
		if boxWidth < 40 {
			boxWidth = 40
		}
		detail := renderDetailPanel(item, boxWidth, m.height)
		help := renderHelpBar(m.mode)
		return lipgloss.JoinVertical(lipgloss.Left, header, detail, help)
	}

	// ── Normal / Input mode ──────────────────────────────────────

	// Input section
	inputSection := ""
	if m.mode == ModeInput {
		prompt := InputPromptStyle.Render("  🔗 Paste magnet link or .torrent path:")
		input := "  " + m.textInput.View()
		inputSection = "\n" + prompt + "\n" + input + "\n"
	}

	// Downloads list
	var downloadsContent string
	if len(m.downloads) == 0 {
		downloadsContent = renderEmptyState()
	} else {
		var items []string
		for i, item := range m.downloads {
			items = append(items, renderDownloadItem(item, i == m.selected, m.width-8))
		}
		downloadsContent = "\n" + strings.Join(items, "\n\n") + "\n"
	}

	boxWidth := m.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}
	downloadsBox := DownloadsBoxStyle.Width(boxWidth).Render(
		MutedStyle.Render(" Downloads ─────────────") + downloadsContent,
	)

	help := renderHelpBar(m.mode)

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		inputSection,
		downloadsBox,
		help,
	)
}

func (m Model) findDownload(id int) int {
	for i, d := range m.downloads {
		if d.ID == id {
			return i
		}
	}
	return -1
}
