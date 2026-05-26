package diff

import (
	godiff "github.com/sourcegraph/go-diff/diff"
)

// ParseHunks parses a unified diff (output of git diff) into a slice of hunks.
// Returns nil, nil for empty diffs (new/deleted files with no unified diff output).
func ParseHunks(rawDiff []byte) ([]*godiff.Hunk, error) {
	if len(rawDiff) == 0 {
		return nil, nil
	}
	fd, err := godiff.ParseFileDiff(rawDiff)
	if err != nil {
		return nil, err
	}
	if fd == nil {
		return nil, nil
	}
	return fd.Hunks, nil
}
