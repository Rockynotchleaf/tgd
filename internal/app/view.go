package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	rw "github.com/mattn/go-runewidth"
	"github.com/chrisrogers/tgd/internal/diff"
)

const (
	sideLeft  = "left"
	sideRight = "right"
)

// View implements the bubbletea Model interface.
func (m Model) View() string {
	if !m.ready {
		return "Initializing…\n"
	}

	styles := defaultStyles()
	fileW, leftW, rightW := m.paneWidths()
	ph := m.paneHeight()
	_ = ph

	// ── File list panel ──────────────────────────────────────────────────
	fileTitle := styles.PanelTitle.Width(fileW).Render("CHANGED FILES")
	fileContent := m.renderFileList(fileW)
	filePane := styles.FilePanel.
		Width(fileW).
		Render(fileTitle + "\n" + fileContent)

	// ── Original (HEAD) panel ────────────────────────────────────────────
	leftTitle := styles.PanelTitle.Width(leftW).Render("ORIGINAL (HEAD)")
	leftPane := lipgloss.NewStyle().Width(leftW).Render(
		leftTitle + "\n" + m.leftVP.View(),
	)

	// ── Modified (working tree) panel ────────────────────────────────────
	rightTitle := styles.PanelTitle.Width(rightW).Render("MODIFIED")
	rightPane := lipgloss.NewStyle().Width(rightW).Render(
		rightTitle + "\n" + m.rightVP.View(),
	)

	// ── Join panels horizontally ─────────────────────────────────────────
	body := lipgloss.JoinHorizontal(lipgloss.Top, filePane, leftPane, rightPane)

	// ── Status bar ───────────────────────────────────────────────────────
	status := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, body, status)
}

// renderFileList renders the file list panel content.
func (m Model) renderFileList(width int) string {
	styles := defaultStyles()
	ph := m.paneHeight() - 1 // minus title
	if ph < 1 {
		ph = 1
	}

	if len(m.files) == 0 {
		if m.loading {
			return styles.Filler.Width(width).Render("  loading…")
		}
		return styles.Filler.Width(width).Render("  no changes")
	}

	var sb strings.Builder
	end := m.fileScroll + ph
	if end > len(m.files) {
		end = len(m.files)
	}

	for i := m.fileScroll; i < end; i++ {
		f := m.files[i]
		// Status indicator with color
		statusStr := statusLabel(f.Status)
		line := fmt.Sprintf("%s %s", statusStr, f.Path)

		// Truncate to width (leave 1 space for border)
		line = truncateTo(line, width-2)

		var rendered string
		if i == m.cursor {
			rendered = styles.Selected.Width(width).Render(line)
		} else {
			rendered = lipgloss.NewStyle().Width(width).Render(line)
		}
		sb.WriteString(rendered)
		sb.WriteByte('\n')
	}

	// Fill remaining rows with blank lines
	for i := end - m.fileScroll; i < ph; i++ {
		sb.WriteString(lipgloss.NewStyle().Width(width).Render(""))
		sb.WriteByte('\n')
	}

	return sb.String()
}

// renderStatusBar renders the bottom status bar.
func (m Model) renderStatusBar() string {
	styles := defaultStyles()

	var parts []string

	// Loading indicator
	if m.loading {
		parts = append(parts, "⟳ loading")
	}

	// Error message
	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		parts = append(parts, errStyle.Render("✗ "+m.errMsg))
	}

	// Current file
	if m.cursor < len(m.files) {
		f := m.files[m.cursor]
		parts = append(parts, statusLabel(f.Status)+" "+f.Path)
	}

	// Scroll percent
	if m.focus == FocusDiff && len(m.aligned) > 0 {
		pct := int(m.leftVP.ScrollPercent() * 100)
		parts = append(parts, fmt.Sprintf("%d%%", pct))
	}

	// Key hints
	hintsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	hints := hintsStyle.Render(" j/k:nav  tab:focus  r:refresh  q:quit")
	if m.focus == FocusDiff {
		hints = hintsStyle.Render(" j/k:scroll  g/G:top/bot  tab:focus  r:refresh  q:quit")
	}

	left := strings.Join(parts, "  ")
	// Pad between left and hints
	gap := m.width - rw.StringWidth(left) - rw.StringWidth(hints) - 2
	if gap < 1 {
		gap = 1
	}

	bar := left + strings.Repeat(" ", gap) + hints
	return styles.StatusBar.Width(m.width).Render(bar)
}

// renderSide renders all AlignedLines for one side (left/right) into a
// newline-joined string suitable for viewport.SetContent.
//
// Lines are padded to exactly `width` display chars (for full-width bg color)
// then styled according to their LineKind.
func renderSide(lines []diff.AlignedLine, side string, width int, styles Styles) string {
	if width < 1 {
		return ""
	}
	var sb strings.Builder
	for _, al := range lines {
		var text string
		var kind diff.LineKind
		if side == sideLeft {
			text, kind = al.LeftText, al.LeftKind
		} else {
			text, kind = al.RightText, al.RightKind
		}

		padded := padToWidth(text, width)

		var s lipgloss.Style
		switch kind {
		case diff.KindRemoved:
			s = styles.Removed
		case diff.KindAdded:
			s = styles.Added
		case diff.KindFiller:
			s = styles.Filler
		default:
			s = styles.Context
		}

		sb.WriteString(s.Render(padded))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// padToWidth pads or truncates a string to exactly `width` display columns.
func padToWidth(s string, width int) string {
	w := rw.StringWidth(s)
	if w > width {
		return rw.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-w)
}

// truncateTo truncates a string to at most `width` display columns.
func truncateTo(s string, width int) string {
	if rw.StringWidth(s) > width {
		return rw.Truncate(s, width, "…")
	}
	return s
}

// statusLabel returns a colored status indicator for a file's git status.
func statusLabel(status string) string {
	switch status {
	case "M":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("M")
	case "A":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("156")).Render("A")
	case "D":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render("D")
	case "??":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("?")
	case "R":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Render("R")
	default:
		return status
	}
}
