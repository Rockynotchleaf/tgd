package app

import (
	"fmt"
	"strconv"
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
	leftPane := lipgloss.NewStyle().
		Width(leftW).
		BorderRight(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Render(leftTitle + "\n" + m.leftVP.View())

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

		// Build and truncate using a PLAIN string (no ANSI escape codes).
		// go-runewidth does not strip ANSI, so measuring a lipgloss-rendered
		// string produces wildly wrong widths and causes premature truncation.
		plain := statusPlain(f.Status) + " " + f.Path
		plain = truncateTo(plain, width-2)

		var rendered string
		if i == m.cursor {
			// Selected row: full-width highlight; no need for separate status color
			rendered = styles.Selected.Width(width).Render(plain)
		} else {
			// Unselected: colorize just the status character, keep path plain.
			// Lipgloss's own Render() is ANSI-aware, so joining colored+plain
			// content is safe to pass into Width().Render().
			colored := lipgloss.NewStyle().
				Foreground(statusFg(f.Status)).
				Render(statusPlain(f.Status))
			rest := plain[len(statusPlain(f.Status)):]
			rendered = lipgloss.NewStyle().Width(width).Render(colored + rest)
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
// Each line is prefixed with a line-number gutter: "  42 │ content…"
// Filler rows (LineNo == 0) show blank space in the gutter: "     │"
// Lines are then padded to exactly `width` display chars total.
func renderSide(lines []diff.AlignedLine, side string, width int, styles Styles) string {
	if width < 1 {
		return ""
	}

	// Determine gutter width from the highest line number on this side.
	maxLineNo := 0
	for _, al := range lines {
		n := al.LeftLineNo
		if side == sideRight {
			n = al.RightLineNo
		}
		if n > maxLineNo {
			maxLineNo = n
		}
	}
	numWidth := len(strconv.Itoa(maxLineNo))
	if numWidth < 1 {
		numWidth = 1
	}
	// Gutter layout: "<numWidth digits> │ " = numWidth + 3 chars
	gutterWidth := numWidth + 3
	contentWidth := width - gutterWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	lineNoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("237"))

	var sb strings.Builder
	for _, al := range lines {
		var text string
		var kind diff.LineKind
		var lineNo int
		if side == sideLeft {
			text, kind, lineNo = al.LeftText, al.LeftKind, al.LeftLineNo
		} else {
			text, kind, lineNo = al.RightText, al.RightKind, al.RightLineNo
		}

		// Gutter: right-aligned number or blank, then " │ "
		var numStr string
		if lineNo > 0 {
			numStr = fmt.Sprintf("%*d", numWidth, lineNo)
		} else {
			numStr = strings.Repeat(" ", numWidth)
		}
		gutter := lineNoStyle.Render(numStr) + sepStyle.Render(" │ ")

		// Content: pad/truncate to contentWidth, then apply diff styling
		padded := padToWidth(text, contentWidth)
		var content string
		switch kind {
		case diff.KindRemoved:
			content = styles.Removed.Render(padded)
		case diff.KindAdded:
			content = styles.Added.Render(padded)
		case diff.KindFiller:
			content = styles.Filler.Render(padded)
		default:
			content = styles.Context.Render(padded)
		}

		sb.WriteString(gutter + content)
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

// statusPlain returns the single plain-text character for a file's git status.
// Always returns a plain string with no ANSI escape codes — safe to use with
// go-runewidth for measurement and truncation.
func statusPlain(status string) string {
	switch status {
	case "??":
		return "?"
	case "M", "A", "D", "R":
		return status
	default:
		if len(status) > 0 {
			return string(status[0])
		}
		return "?"
	}
}

// statusFg returns the foreground color for a file's git status.
func statusFg(status string) lipgloss.Color {
	switch status {
	case "M":
		return lipgloss.Color("214") // amber
	case "A":
		return lipgloss.Color("156") // green
	case "D":
		return lipgloss.Color("203") // red
	case "??":
		return lipgloss.Color("39") // blue
	case "R":
		return lipgloss.Color("141") // purple
	default:
		return lipgloss.Color("244") // gray
	}
}

// statusLabel returns a colored status indicator. Use only where the result
// will NOT be passed to go-runewidth (rw.StringWidth / rw.Truncate), as those
// functions do not strip ANSI escape codes.
func statusLabel(status string) string {
	return lipgloss.NewStyle().Foreground(statusFg(status)).Render(statusPlain(status))
}
