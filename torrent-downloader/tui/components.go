package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
)

// renderProgressBar draws a custom progress bar with filled/empty segments.
func renderProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 40
	}
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled

	filledStr := ProgressFilledStyle.Render(strings.Repeat("━", filled))
	emptyStr := ProgressEmptyStyle.Render(strings.Repeat("━", empty))

	pctStr := fmt.Sprintf(" %.1f%%", percent*100)
	return filledStr + emptyStr + MutedStyle.Render(pctStr)
}

// renderDownloadItem renders a single download entry in the list.
func renderDownloadItem(item DownloadItem, selected bool, width int) string {
	snap := item.Snapshot

	name := item.Name
	if name == "" {
		name = "Loading metadata..."
	}

	sizeStr := ""
	if snap.TotalBytes > 0 {
		sizeStr = humanize.Bytes(uint64(snap.TotalBytes))
	}

	// Selection indicator
	indicator := "  "
	if selected {
		indicator = SelectedStyle.Render("▸ ")
	}

	// Name line
	nameStyle := lipgloss.NewStyle().Foreground(White)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	nameLine := indicator + nameStyle.Render(name)
	if sizeStr != "" {
		nameLine += MutedStyle.Render("  " + sizeStr)
	}

	// State-dependent rendering
	switch item.State {
	case "waiting":
		peerInfo := fmt.Sprintf("Peers: %d/%d (Dialing: %d, DHT: %d)  Seeders: %d", snap.ActivePeers, snap.TotalPeers, snap.HalfOpenPeers, snap.PendingPeers, snap.ConnectedSeeders)
		return nameLine + "\n" + WarningStyle.Render("    ⏳ Downloading metadata...  ") + MutedStyle.Render(peerInfo)

	case "error":
		errMsg := "Unknown error"
		if item.Error != nil {
			errMsg = item.Error.Error()
		}
		return nameLine + "\n" + ErrorStyle.Render("    ✗ "+errMsg)

	case "complete":
		return nameLine + "\n" + SpeedStyle.Render("    ✓ Download complete")
	}

	// Downloading or paused — show progress bar and stats
	barWidth := width - 14
	if barWidth < 20 {
		barWidth = 20
	}
	var percent float64
	if snap.TotalBytes > 0 {
		percent = float64(snap.CompletedBytes) / float64(snap.TotalBytes)
	}
	progressLine := "    " + renderProgressBar(percent, barWidth)

	speed := humanize.Bytes(uint64(snap.DownloadSpeed)) + "/s"
	peers := fmt.Sprintf("Peers: %d/%d", snap.ActivePeers, snap.TotalPeers)
	seeders := fmt.Sprintf("Seeders: %d", snap.ConnectedSeeders)

	etaStr := "∞"
	if snap.ETA > 0 && snap.ETA < 100*time.Hour {
		etaStr = snap.ETA.Round(time.Second).String()
	}

	stateTag := ""
	if item.State == "paused" {
		stateTag = WarningStyle.Render("⏸ PAUSED  ")
	}

	statsLine := fmt.Sprintf("    %s↓ %s  %s  %s  ETA: %s",
		stateTag,
		SpeedStyle.Render(speed),
		peers,
		seeders,
		etaStr,
	)

	return nameLine + "\n" + progressLine + "\n" + statsLine
}

// renderHeader draws the top bar of the TUI.
func renderHeader(speedLimit int64, width int) string {
	title := TitleStyle.Render("⚡ FastDown")
	subtitle := MutedStyle.Render("  Ultra-fast Torrent Downloader")

	limitStr := "∞"
	if speedLimit > 0 {
		limitStr = humanize.Bytes(uint64(speedLimit)) + "/s"
	}
	limitInfo := MutedStyle.Render("Limit: ") + SpeedStyle.Render(limitStr)

	left := title + subtitle
	pad := width - lipgloss.Width(left) - lipgloss.Width(limitInfo) - 6
	if pad < 2 {
		pad = 2
	}

	return HeaderStyle.Width(width - 2).Render(
		left + strings.Repeat(" ", pad) + limitInfo,
	)
}

// renderHelpBar draws the bottom keybinding help.
func renderHelpBar(mode AppMode) string {
	switch mode {
	case ModeInput:
		return HelpStyle.Render("  [Enter] add  [Esc] cancel  [Ctrl+C] quit")
	case ModeDetail:
		return HelpStyle.Render("  [Tab/i] back to list  [n] toggle network  [q] quit")
	default:
		return HelpStyle.Render("  [a] add  [d] del  [p] pause  [Tab/i] details  [↑↓] nav  [+/-] speed  [q] quit")
	}
}

// renderEmptyState shows a message when there are no downloads.
func renderEmptyState() string {
	return MutedStyle.Render("\n    No downloads yet. Press " +
		SelectedStyle.Render("[a]") +
		MutedStyle.Render(" to add a torrent.") + "\n")
}

// stat renders a single label→value row for the detail panel.
func stat(label, value string) string {
	return StatLabelStyle.Render(label+":") + " " + StatValueStyle.Render(value)
}

// renderPiecesHeatmap draws a compact block-based heatmap of piece completion.
func renderPiecesHeatmap(complete, total, width int) string {
	if total == 0 || width <= 0 {
		return ""
	}
	cells := width
	if cells > total {
		cells = total
	}
	var out strings.Builder
	for i := 0; i < cells; i++ {
		piecesPerCell := float64(total) / float64(cells)
		startPiece := int(float64(i) * piecesPerCell)
		endPiece := int(float64(i+1) * piecesPerCell)
		if endPiece > total {
			endPiece = total
		}
		rangeSize := endPiece - startPiece
		completeInRange := 0
		if startPiece < complete {
			completeInRange = complete - startPiece
			if completeInRange > rangeSize {
				completeInRange = rangeSize
			}
		}
		ratio := float64(completeInRange) / float64(rangeSize)
		switch {
		case ratio >= 1.0:
			out.WriteString(PieceFilledStyle.Render("█"))
		case ratio >= 0.5:
			out.WriteString(PieceHashingStyle.Render("▓"))
		case ratio > 0:
			out.WriteString(PieceHashingStyle.Render("░"))
		default:
			out.WriteString(PiecePendingStyle.Render("·"))
		}
	}
	return out.String()
}

// renderDetailPanel renders the full stats panel for a selected download.
func renderDetailPanel(item DownloadItem, width, height int) string {
	snap := item.Snapshot
	_ = height

	// ── Title ────────────────────────────────────────────────────
	name := item.Name
	if name == "" {
		name = "Loading metadata..."
	}
	titleLine := TitleStyle.Render("⚡ ") + StatValueStyle.Render(name)

	stateTag := ""
	switch item.State {
	case "waiting":
		stateTag = WarningStyle.Render("⏳ Connecting to peers…")
	case "paused":
		stateTag = WarningStyle.Render("⏸  Paused")
	case "complete":
		stateTag = SpeedStyle.Render("✓  Complete")
	case "error":
		msg := "error"
		if item.Error != nil {
			msg = item.Error.Error()
		}
		stateTag = ErrorStyle.Render("✗  " + msg)
	default:
		stateTag = SpeedStyle.Render("↓  Downloading")
	}

	// ── Transfer section ─────────────────────────────────────────
	var percent float64
	if snap.TotalBytes > 0 {
		percent = float64(snap.CompletedBytes) / float64(snap.TotalBytes)
	}
	barWidth := width - 10
	if barWidth < 20 {
		barWidth = 20
	}
	progressBar := renderProgressBar(percent, barWidth)

	dlSpeedStr := humanize.Bytes(uint64(snap.DownloadSpeed)) + "/s"
	ulSpeedStr := humanize.Bytes(uint64(snap.UploadSpeed)) + "/s"

	etaStr := "∞"
	if snap.ETA > 0 && snap.ETA < 100*time.Hour {
		etaStr = snap.ETA.Round(time.Second).String()
	}
	sizeStr := "—"
	if snap.TotalBytes > 0 {
		sizeStr = humanize.Bytes(uint64(snap.CompletedBytes)) + " / " + humanize.Bytes(uint64(snap.TotalBytes))
	}
	ratioStr := fmt.Sprintf("%.4f", percent*100) + " %%"

	transferBlock := strings.Join([]string{
		DetailSectionStyle.Render("  ▸ Transfer"),
		"  " + progressBar,
		"  " + stat("Downloaded", sizeStr) + "   " + stat("Progress", ratioStr),
		"  " + stat("↓ Download speed", SpeedStyle.Render(dlSpeedStr)) + "   " + stat("↑ Upload speed", UploadStyle.Render(ulSpeedStr)),
		"  " + stat("ETA", etaStr),
	}, "\n")

	// ── Peers section ─────────────────────────────────────────────
	peersBlock := strings.Join([]string{
		DetailSectionStyle.Render("  ▸ Peers & Seeds"),
		"  " + stat("Active peers", fmt.Sprintf("%d", snap.ActivePeers)) +
			"   " + stat("Total known peers", fmt.Sprintf("%d", snap.TotalPeers)),
		"  " + stat("Connected seeders", fmt.Sprintf("%d", snap.ConnectedSeeders)),
		"  " + stat("DHT / Tracker peers (pending)", fmt.Sprintf("%d", snap.PendingPeers)) +
			"   " + stat("Dialing (half-open)", fmt.Sprintf("%d", snap.HalfOpenPeers)),
	}, "\n")

	// ── Pieces section ────────────────────────────────────────────
	piecePct := 0.0
	if snap.TotalPieces > 0 {
		piecePct = float64(snap.PiecesComplete) / float64(snap.TotalPieces) * 100
	}
	heatmap := renderPiecesHeatmap(snap.PiecesComplete, snap.TotalPieces, barWidth)

	piecesBlock := strings.Join([]string{
		DetailSectionStyle.Render("  ▸ Pieces"),
		"  " + heatmap,
		"  " + stat("Complete", fmt.Sprintf("%d / %d  (%.1f%%)", snap.PiecesComplete, snap.TotalPieces, piecePct)),
		"  " + stat("Good writes", fmt.Sprintf("%d", snap.PiecesDirtiedGood)) +
			"   " + stat("Bad writes", ErrorStyle.Render(fmt.Sprintf("%d", snap.PiecesDirtiedBad))),
		"  " + stat("Chunks wasted", fmt.Sprintf("%d", snap.ChunksWasted)),
	}, "\n")

	// ── Networks section ─────────────────────────────────────────
	var networksBlock string
	if len(snap.Networks) > 0 {
		var netLines []string
		var sumDown, sumUp int64
		for _, net := range snap.Networks {
			sumDown += int64(net.DownloadSpeed)
			sumUp += int64(net.UploadSpeed)
		}

		header := DetailSectionStyle.Render("  ▸ Networks")
		if sumDown > 0 || sumUp > 0 {
			header += "  " + MutedStyle.Render(fmt.Sprintf("Σ ↓ %s/s  ↑ %s/s",
				humanize.Bytes(uint64(sumDown)), humanize.Bytes(uint64(sumUp))))
		}
		netLines = append(netLines, header)

		for _, net := range snap.Networks {
			kindIcon := networkKindIcon(net.Kind)
			dataStr := formatDataUsage(net.DataUsed, net.DataLimit)
			peerStr := fmt.Sprintf("Peers: %d", net.ActivePeers)
			speedStr := fmt.Sprintf("↓ %s/s  ↑ %s/s",
				humanize.Bytes(uint64(net.DownloadSpeed)), humanize.Bytes(uint64(net.UploadSpeed)))
			statusColor := networkStatusColor(net.Status)
			netLines = append(netLines, fmt.Sprintf("  %s %s %s  %s  %s  %s  %s",
				kindIcon,
				StatValueStyle.Render(net.Label),
				MutedStyle.Render(fmt.Sprintf("(%s)", net.Kind)),
				statusColor.Render(net.Status),
				StatValueStyle.Render(speedStr),
				peerStr,
				MutedStyle.Render(dataStr),
			))
		}
		networksBlock = strings.Join(netLines, "\n")
	}

	// ── Assemble ─────────────────────────────────────────────────
	innerWidth := width - 6
	if innerWidth < 20 {
		innerWidth = 20
	}
	divider := MutedStyle.Render(strings.Repeat("─", innerWidth))

	bodyParts := []string{
		"  " + titleLine,
		"  " + stateTag,
		"  " + divider,
		transferBlock,
		"  " + divider,
		peersBlock,
	}
	if networksBlock != "" {
		bodyParts = append(bodyParts, "  "+divider, networksBlock)
	}
	bodyParts = append(bodyParts, "  "+divider, piecesBlock, "")

	body := strings.Join(bodyParts, "\n")

	return DetailBoxStyle.Width(width - 2).Render(body)
}

// networkKindIcon returns an icon for the network kind.
func networkKindIcon(kind string) string {
	switch kind {
	case "wifi":
		return "📶"
	case "ethernet":
		return "🔗"
	case "usb":
		return "🔌"
	case "bridge":
		return "🌉"
	default:
		return "📡"
	}
}

// networkStatusColor returns a styled string for the network status.
func networkStatusColor(status string) lipgloss.Style {
	switch status {
	case "on":
		return SpeedStyle
	case "off":
		return MutedStyle
	case "offline":
		return WarningStyle
	case "unreachable":
		return ErrorStyle
	case "failed":
		return ErrorStyle
	case "limit":
		return WarningStyle
	default:
		return MutedStyle
	}
}

// formatDataUsage formats data usage with limit.
func formatDataUsage(used, limit int64) string {
	usedStr := humanize.Bytes(uint64(used))
	if limit > 0 {
		limitStr := humanize.Bytes(uint64(limit))
		pct := float64(used) / float64(limit) * 100
		return fmt.Sprintf("%s / %s (%.1f%%)", usedStr, limitStr, pct)
	}
	return usedStr + " / ∞"
}
