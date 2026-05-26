package diff

import (
	"strings"

	godiff "github.com/sourcegraph/go-diff/diff"
)

// LineKind identifies how a line should be styled in the diff view.
type LineKind int

const (
	KindContext LineKind = iota // unchanged line, shown on both sides
	KindRemoved                 // deleted line, shown on left with filler on right
	KindAdded                   // inserted line, shown on right with filler on left
	KindFiller                  // blank padding to keep sides aligned
)

// AlignedLine is one row in the side-by-side diff view.
// Both left and right can be non-empty (KindContext or Replace-paired).
// A KindFiller side will have an empty text.
type AlignedLine struct {
	LeftText  string
	LeftKind  LineKind
	RightText string
	RightKind LineKind
}

// Align converts unified diff hunks + full file lines into a side-by-side
// aligned slice where corresponding lines share the same row.
//
// orig is the original file lines (from HEAD); nil for new files.
// curr is the current file lines (from working tree); nil for deleted files.
func Align(hunks []*godiff.Hunk, orig, curr []string) []AlignedLine {
	var result []AlignedLine
	origIdx := 0
	currIdx := 0

	for _, h := range hunks {
		hunkOrigStart := int(h.OrigStartLine) - 1 // convert to 0-based
		hunkCurrStart := int(h.NewStartLine) - 1  // convert to 0-based

		// Handle new file (OrigStartLine = 0 in the diff header)
		if hunkOrigStart < 0 {
			hunkOrigStart = 0
		}
		if hunkCurrStart < 0 {
			hunkCurrStart = 0
		}

		// Emit context lines before this hunk (lines unchanged before any edits)
		for origIdx < hunkOrigStart && origIdx < len(orig) {
			var rText string
			if currIdx < len(curr) {
				rText = curr[currIdx]
			}
			result = append(result, AlignedLine{
				LeftText: orig[origIdx], LeftKind: KindContext,
				RightText: rText, RightKind: KindContext,
			})
			origIdx++
			currIdx++
		}

		// Walk the hunk body, accumulating pending - and + lines
		var leftPending, rightPending []string

		body := string(h.Body)
		for _, line := range strings.Split(body, "\n") {
			if len(line) == 0 {
				continue
			}
			prefix := line[0]

			// Skip "\ No newline at end of file" markers
			if prefix == '\\' {
				continue
			}

			switch prefix {
			case ' ': // context line within hunk
				result = flushPending(result, leftPending, rightPending)
				leftPending, rightPending = nil, nil

				var lText, rText string
				if origIdx < len(orig) {
					lText = orig[origIdx]
				}
				if currIdx < len(curr) {
					rText = curr[currIdx]
				}
				result = append(result, AlignedLine{
					LeftText: lText, LeftKind: KindContext,
					RightText: rText, RightKind: KindContext,
				})
				origIdx++
				currIdx++

			case '-': // removed line
				if origIdx < len(orig) {
					leftPending = append(leftPending, orig[origIdx])
				}
				origIdx++

			case '+': // added line
				if currIdx < len(curr) {
					rightPending = append(rightPending, curr[currIdx])
				}
				currIdx++
			}
		}
		result = flushPending(result, leftPending, rightPending)
	}

	// Emit remaining context lines after the last hunk
	for origIdx < len(orig) && currIdx < len(curr) {
		result = append(result, AlignedLine{
			LeftText: orig[origIdx], LeftKind: KindContext,
			RightText: curr[currIdx], RightKind: KindContext,
		})
		origIdx++
		currIdx++
	}
	// Overflow safety: lines remaining on one side only (shouldn't happen in well-formed diffs)
	for origIdx < len(orig) {
		result = append(result, AlignedLine{
			LeftText: orig[origIdx], LeftKind: KindRemoved,
			RightKind: KindFiller,
		})
		origIdx++
	}
	for currIdx < len(curr) {
		result = append(result, AlignedLine{
			LeftKind:  KindFiller,
			RightText: curr[currIdx], RightKind: KindAdded,
		})
		currIdx++
	}

	return result
}

// AlignNewFile creates an aligned diff for a completely new (untracked) file.
// All lines appear on the right as additions; left is filler.
func AlignNewFile(curr []string) []AlignedLine {
	result := make([]AlignedLine, len(curr))
	for i, line := range curr {
		result[i] = AlignedLine{
			LeftKind:  KindFiller,
			RightText: line, RightKind: KindAdded,
		}
	}
	return result
}

// AlignDeletedFile creates an aligned diff for a completely deleted file.
// All lines appear on the left as removals; right is filler.
func AlignDeletedFile(orig []string) []AlignedLine {
	result := make([]AlignedLine, len(orig))
	for i, line := range orig {
		result[i] = AlignedLine{
			LeftText: line, LeftKind: KindRemoved,
			RightKind: KindFiller,
		}
	}
	return result
}

// flushPending pairs up accumulated removed and added lines 1:1 (replace pairs),
// emitting filler on the shorter side for any excess lines.
func flushPending(result []AlignedLine, left, right []string) []AlignedLine {
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		al := AlignedLine{LeftKind: KindFiller, RightKind: KindFiller}
		if i < len(left) {
			al.LeftText = left[i]
			al.LeftKind = KindRemoved
		}
		if i < len(right) {
			al.RightText = right[i]
			al.RightKind = KindAdded
		}
		result = append(result, al)
	}
	return result
}
