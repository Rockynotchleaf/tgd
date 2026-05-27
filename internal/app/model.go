package app

import (
	"path/filepath"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Rockynotchleaf/tgd/internal/diff"
	"github.com/Rockynotchleaf/tgd/internal/ipc"
)

// Focus identifies which panel has keyboard focus.
type Focus int

const (
	FocusFileList Focus = iota
	FocusDiff
)

// Styles holds the lipgloss styles used throughout the TUI.
type Styles struct {
	Removed    lipgloss.Style
	Added      lipgloss.Style
	Filler     lipgloss.Style
	Context    lipgloss.Style
	Selected   lipgloss.Style
	FilePanel  lipgloss.Style
	DiffPanel  lipgloss.Style
	StatusBar  lipgloss.Style
	PanelTitle lipgloss.Style
}

func defaultStyles() Styles {
	return Styles{
		Removed:   lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("203")),
		Added:     lipgloss.NewStyle().Background(lipgloss.Color("22")).Foreground(lipgloss.Color("156")),
		Filler:    lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("240")),
		Context:   lipgloss.NewStyle(),
		Selected:  lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("255")).Bold(true),
		FilePanel: lipgloss.NewStyle().BorderRight(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")),
		DiffPanel: lipgloss.NewStyle(),
		StatusBar: lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("244")).Padding(0, 1),
		PanelTitle: lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("250")).
			Padding(0, 1),
	}
}

// DiffLoadedMsg is delivered when async diff loading completes.
type DiffLoadedMsg struct {
	RepoRoot string
	Files    []diff.FileEntry
	Aligned  []diff.AlignedLine
	Cursor   int
}

// FileSelectedMsg is delivered when only the diff content (not file list) reloads.
type FileSelectedMsg struct {
	Aligned []diff.AlignedLine
	Cursor  int
}

// ErrorMsg carries a non-fatal error to display in the status bar.
type ErrorMsg struct{ Err error }

// Model is the bubbletea application state.
type Model struct {
	// Terminal dimensions
	width, height int
	ready         bool

	// Repo context (set on first load)
	cwd      string
	repoRoot string

	// Panel focus
	focus Focus

	// File list (left panel)
	files      []diff.FileEntry
	cursor     int
	fileScroll int // index of first visible file in list

	// Diff content (middle + right panels)
	aligned []diff.AlignedLine
	leftVP  viewport.Model
	rightVP viewport.Model

	// State
	loading bool
	errMsg  string

	// IPC paths (for display in status bar)
	socketPath string
}

// New creates an initial Model. cwd is the directory to diff; sockPath is
// the path to the unix socket this tgd instance will listen on.
func New(cwd, sockPath string) Model {
	return Model{
		cwd:        cwd,
		socketPath: sockPath,
		focus:      FocusFileList,
		loading:    true,
	}
}

// Init loads the initial diff as the first command.
func (m Model) Init() tea.Cmd {
	return cmdLoadAll(m.cwd, 0)
}

// cmdLoadAll is a tea.Cmd that loads the file list + diff for a given cursor.
func cmdLoadAll(cwd string, cursor int) tea.Cmd {
	return func() tea.Msg {
		root, err := diff.RepoRoot(cwd)
		if err != nil {
			return ErrorMsg{err}
		}
		files, err := diff.ChangedFiles(root)
		if err != nil {
			return ErrorMsg{err}
		}
		if len(files) == 0 {
			return DiffLoadedMsg{RepoRoot: root, Files: nil, Aligned: nil, Cursor: 0}
		}
		if cursor >= len(files) {
			cursor = 0
		}
		aligned, err := diff.LoadAligned(root, files[cursor])
		if err != nil {
			return ErrorMsg{err}
		}
		return DiffLoadedMsg{
			RepoRoot: root,
			Files:    files,
			Aligned:  aligned,
			Cursor:   cursor,
		}
	}
}

// cmdLoadFile is a tea.Cmd that loads only the diff for a specific file.
func cmdLoadFile(root string, files []diff.FileEntry, cursor int) tea.Cmd {
	return func() tea.Msg {
		if cursor >= len(files) || cursor < 0 {
			return FileSelectedMsg{Cursor: cursor}
		}
		aligned, err := diff.LoadAligned(root, files[cursor])
		if err != nil {
			return ErrorMsg{err}
		}
		return FileSelectedMsg{Aligned: aligned, Cursor: cursor}
	}
}

// paneWidths returns the character widths for [filePanel, leftDiff, rightDiff].
func (m Model) paneWidths() (fileW, leftW, rightW int) {
	const minFileW = 18
	fileW = m.width / 5
	if fileW < minFileW {
		fileW = minFileW
	}
	// Account for: file panel right border (1) + left/right diff pane divider (1)
	remaining := m.width - fileW - 2
	leftW = remaining / 2
	rightW = remaining - leftW
	return
}

// paneHeight returns the usable height for diff content (minus title + status bars).
func (m Model) paneHeight() int {
	h := m.height - 2 // 1 title row + 1 status bar
	if h < 1 {
		h = 1
	}
	return h
}

// rebuildViewports re-renders both diff viewports from m.aligned.
// Must be called after aligned changes or terminal resizes.
func (m *Model) rebuildViewports() {
	_, leftW, rightW := m.paneWidths()
	ph := m.paneHeight()
	styles := defaultStyles()

	m.leftVP.Width = leftW
	m.leftVP.Height = ph
	m.rightVP.Width = rightW
	m.rightVP.Height = ph

	left := renderSide(m.aligned, sideLeft, leftW, styles)
	right := renderSide(m.aligned, sideRight, rightW, styles)

	m.leftVP.SetContent(left)
	m.rightVP.SetContent(right)
}

// stateDir returns the base path for tgd's runtime state files.
func stateDir() string {
	home, _ := filepath.Abs(filepath.Join("~"))
	_ = home
	// Using the same path computation as main.go
	return filepath.Join(userHomeDir(), ".local", "share", "tgd")
}

func userHomeDir() string {
	home, err := filepath.Abs(".")
	if err == nil {
		_ = home
	}
	// os.UserHomeDir is called in main; here we just return a sentinel
	return "~"
}

// RefreshMsg (re-exported from ipc for use in Update)
type RefreshMsg = ipc.RefreshMsg
