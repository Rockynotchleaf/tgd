// Package diff provides git operations and diff computation for tgd.
package diff

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileEntry represents a file that has changed relative to HEAD.
type FileEntry struct {
	Status string // M=modified, A=added, D=deleted, ??=untracked
	Path   string // relative to repo root
}

// RepoRoot returns the absolute path to the git repository root containing cwd.
func RepoRoot(cwd string) (string, error) {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedFiles returns all files changed relative to HEAD plus any untracked files.
// root must be the absolute path to the repo root (from RepoRoot).
func ChangedFiles(root string) ([]FileEntry, error) {
	var entries []FileEntry

	// Tracked changes vs HEAD (modified, added, deleted, renamed)
	out, err := exec.Command(
		"git", "-C", root, "diff", "HEAD",
		"--name-status", "--diff-filter=MADR",
	).Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			// Status is the first field; path is the last (handles renames "R100 old new")
			status := parts[0][:1] // take first char: M, A, D, R
			path := parts[len(parts)-1]
			entries = append(entries, FileEntry{Status: status, Path: path})
		}
	}

	// Untracked files (not staged, not tracked)
	out2, err2 := exec.Command(
		"git", "-C", root, "status", "--porcelain", "--untracked-files=all",
	).Output()
	if err2 == nil && len(out2) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(out2)), "\n") {
			if len(line) < 3 {
				continue
			}
			xy := line[:2]
			path := line[3:]
			if strings.TrimSpace(xy) == "??" {
				entries = append(entries, FileEntry{Status: "??", Path: path})
			}
		}
	}

	return entries, nil
}

// RawDiff returns the unified diff of a file relative to HEAD.
// relPath is relative to root.
func RawDiff(root, relPath string) ([]byte, error) {
	return exec.Command(
		"git", "-C", root, "diff", "HEAD", "--", relPath,
	).Output()
}

// OrigLines returns the lines of a file as it exists in HEAD.
// relPath is relative to root. Returns nil, nil for new files (not in HEAD).
func OrigLines(root, relPath string) ([]string, error) {
	out, err := exec.Command(
		"git", "-C", root, "show", "HEAD:"+relPath,
	).Output()
	if err != nil {
		return nil, err // file doesn't exist in HEAD (new file)
	}
	return splitLines(string(out)), nil
}

// CurrentLines returns the lines of the working-tree file.
// Returns nil, nil for deleted files.
func CurrentLines(absPath string) ([]string, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err // file deleted from working tree
	}
	return splitLines(string(data)), nil
}

// LoadAligned computes the aligned side-by-side diff for a single FileEntry.
// root is the repo root; file.Path is relative to root.
func LoadAligned(root string, file FileEntry) ([]AlignedLine, error) {
	absPath := filepath.Join(root, file.Path)

	switch file.Status {
	case "??":
		// Untracked: show entire file as additions, nothing on left.
		// Guard against binary files — reading them as text produces garbage.
		if isBinaryFile(absPath) {
			return binaryPlaceholder(), nil
		}
		curr, err := CurrentLines(absPath)
		if err != nil {
			return nil, err
		}
		return AlignNewFile(curr), nil

	case "D":
		// Deleted: show entire original as removals, nothing on right.
		raw, err := exec.Command("git", "-C", root, "show", "HEAD:"+file.Path).Output()
		if err != nil {
			return nil, err
		}
		if isBinaryContent(raw) {
			return binaryPlaceholder(), nil
		}
		return AlignDeletedFile(splitLines(string(raw))), nil

	default:
		// Modified or Added (tracked): use git diff HEAD.
		// Guard against binary files.
		if file.Status != "D" && isBinaryFile(absPath) {
			return binaryPlaceholder(), nil
		}
		rawDiff, err := RawDiff(root, file.Path)
		if err != nil {
			return nil, err
		}
		// git diff itself signals binary files with a "Binary files … differ" line
		if isBinaryDiff(rawDiff) {
			return binaryPlaceholder(), nil
		}
		hunks, err := ParseHunks(rawDiff)
		if err != nil {
			return nil, err
		}
		orig, _ := OrigLines(root, file.Path) // nil for new tracked files
		curr, _ := CurrentLines(absPath)      // nil for deleted tracked files
		return Align(hunks, orig, curr), nil
	}
}

// isBinaryFile reports whether the file at path appears to be binary,
// by scanning the first 8 KiB for null bytes — the same heuristic used by
// git and less.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	return isBinaryContent(buf[:n])
}

// isBinaryContent reports whether raw byte content appears to be binary.
func isBinaryContent(data []byte) bool {
	if len(data) > 8192 {
		data = data[:8192]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// isBinaryDiff reports whether a raw git diff output indicates a binary file
// (git emits "Binary files … differ" instead of a unified diff).
func isBinaryDiff(diff []byte) bool {
	return bytes.Contains(diff, []byte("Binary files")) ||
		bytes.Contains(diff, []byte("GIT binary patch"))
}

// binaryPlaceholder returns a single filler AlignedLine displayed when a
// binary file is selected — avoids rendering raw binary bytes in the TUI.
func binaryPlaceholder() []AlignedLine {
	return []AlignedLine{{
		LeftText:  "",
		LeftKind:  KindFiller,
		RightText: "  (binary file — not shown)",
		RightKind: KindFiller,
	}}
}

// splitLines splits a string into lines, stripping the trailing empty entry
// that results from a final newline.
func splitLines(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
