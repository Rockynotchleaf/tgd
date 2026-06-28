package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update implements the bubbletea Model interface.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ── Mouse wheel → always scrolls the diff, regardless of focus ───────
	case tea.MouseMsg:
		if !m.ready {
			return m, nil
		}
		var cmd tea.Cmd
		m.leftVP, cmd = m.leftVP.Update(msg)
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, cmd

	// ── Terminal resize ──────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			// First size message — initialize viewports
			_, leftW, rightW := m.paneWidths()
			ph := m.paneHeight()
			m.leftVP.Width = leftW
			m.leftVP.Height = ph
			m.rightVP.Width = rightW
			m.rightVP.Height = ph
			m.ready = true
		} else {
			m.rebuildViewports()
		}
		return m, nil

	// ── Async diff load completed ────────────────────────────────────────
	case DiffLoadedMsg:
		m.loading = false
		m.repoRoot = msg.RepoRoot
		m.files = msg.Files
		m.cursor = msg.Cursor
		m.aligned = msg.Aligned
		m.fileScroll = 0
		if m.ready {
			m.rebuildViewports()
		}
		return m, nil

	// ── File selection reload ────────────────────────────────────────────
	case FileSelectedMsg:
		m.loading = false
		m.cursor = msg.Cursor
		m.aligned = msg.Aligned
		if m.ready {
			m.rebuildViewports()
		}
		return m, nil

	// ── Non-fatal error ──────────────────────────────────────────────────
	case ErrorMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMsg = msg.Err.Error()
		}
		return m, nil

	// ── IPC refresh signal ───────────────────────────────────────────────
	case RefreshMsg:
		// Ignore edits from a different repo than the one this window shows.
		// A tgd window is bound to the repo it was launched for; this prevents
		// repo-relative path collisions (e.g. two repos both with src/main.go)
		// from leaking another repo's files into the touched set.
		if msg.RepoRoot != "" && m.repoRoot != "" && msg.RepoRoot != m.repoRoot {
			return m, nil
		}
		cwd := msg.CWD
		if cwd == "" {
			cwd = m.cwd
		}
		m.cwd = cwd
		for _, f := range msg.Files {
			if f != "" {
				m.touched[f] = true
			}
		}
		m.loading = true
		m.errMsg = ""
		return m, cmdLoadAll(cwd, m.touched, m.cursor)

	// ── Keyboard input ───────────────────────────────────────────────────
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys (work in any focus)
	switch {
	case keyIs(msg, "q", "ctrl+c"):
		return m, tea.Quit

	case keyIs(msg, "tab"):
		if m.focus == FocusFileList {
			m.focus = FocusDiff
		} else {
			m.focus = FocusFileList
		}
		return m, nil

	case keyIs(msg, "r"):
		m.loading = true
		m.errMsg = ""
		return m, cmdLoadAll(m.cwd, m.touched, m.cursor)

	// Diff-scroll keys work regardless of focus so the user can scroll
	// the diff without first hitting Tab. Capital J/K scroll one line;
	// page-up/down + ctrl+u/d scroll a page.
	case keyIs(msg, "J"):
		m.leftVP.ScrollDown(1)
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil

	case keyIs(msg, "K"):
		m.leftVP.ScrollUp(1)
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil

	case keyIs(msg, "ctrl+d", "pgdown"):
		m.leftVP.HalfPageDown()
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil

	case keyIs(msg, "ctrl+u", "pgup"):
		m.leftVP.HalfPageUp()
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil
	}

	// Focus-specific keys
	switch m.focus {

	case FocusFileList:
		return m.handleFileListKey(msg)

	case FocusDiff:
		return m.handleDiffKey(msg)
	}

	return m, nil
}

func (m Model) handleFileListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyIs(msg, "j", "down"):
		if m.cursor < len(m.files)-1 {
			m.cursor++
			m.clampFileScroll()
			m.loading = true
			return m, cmdLoadFile(m.repoRoot, m.files, m.cursor)
		}

	case keyIs(msg, "k", "up"):
		if m.cursor > 0 {
			m.cursor--
			m.clampFileScroll()
			m.loading = true
			return m, cmdLoadFile(m.repoRoot, m.files, m.cursor)
		}

	case keyIs(msg, "enter"):
		m.focus = FocusDiff
	}
	return m, nil
}

func (m Model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch {
	case keyIs(msg, "g"):
		m.leftVP.GotoTop()
		m.rightVP.SetYOffset(0)
		return m, nil

	case keyIs(msg, "G"):
		m.leftVP.GotoBottom()
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil

	case keyIs(msg, "j", "down"):
		m.leftVP.ScrollDown(1)
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil

	case keyIs(msg, "k", "up"):
		m.leftVP.ScrollUp(1)
		m.rightVP.SetYOffset(m.leftVP.YOffset)
		return m, nil
	}

	// Delegate other keys to left viewport (handles its own scrolling)
	m.leftVP, cmd = m.leftVP.Update(msg)
	m.rightVP.SetYOffset(m.leftVP.YOffset)
	return m, cmd
}

// clampFileScroll adjusts fileScroll so cursor is always visible.
func (m *Model) clampFileScroll() {
	visible := m.paneHeight() - 1 // one title row
	if m.cursor < m.fileScroll {
		m.fileScroll = m.cursor
	}
	if m.cursor >= m.fileScroll+visible {
		m.fileScroll = m.cursor - visible + 1
	}
	if m.fileScroll < 0 {
		m.fileScroll = 0
	}
}
