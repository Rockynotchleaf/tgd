package app

import (
	"strings"
	"testing"

	rw "github.com/mattn/go-runewidth"
	"github.com/Rockynotchleaf/tgd/internal/diff"
)

func TestWrapToWidth(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"empty yields one row", "", 10, []string{""}},
		{"fits on one row", "hello", 10, []string{"hello"}},
		{"exact width", "hello", 5, []string{"hello"}},
		{"wraps into chunks", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"zero width treated as one", "ab", 0, []string{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := wrapToWidth(c.in, c.width)
			if strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Fatalf("wrapToWidth(%q,%d) = %q, want %q", c.in, c.width, got, c.want)
			}
			// Reassembly must equal the original (no chars dropped).
			if join := strings.Join(got, ""); c.in != "" && join != c.in {
				t.Fatalf("reassembly = %q, want %q", join, c.in)
			}
			// No chunk exceeds the requested width.
			w := c.width
			if w < 1 {
				w = 1
			}
			for _, ch := range got {
				if rw.StringWidth(ch) > w {
					t.Fatalf("chunk %q exceeds width %d", ch, w)
				}
			}
		})
	}
}

// TestRenderBothSidesAligned verifies the core invariant that keeps the two
// panes in sync under lockstep scrolling: both sides emit exactly the same
// number of visual rows, equal to the sum of per-line max(leftRows,rightRows).
func TestRenderBothSidesAligned(t *testing.T) {
	long := strings.Repeat("x", 200)
	lines := []diff.AlignedLine{
		{LeftText: "short", LeftKind: diff.KindContext, LeftLineNo: 1,
			RightText: "short", RightKind: diff.KindContext, RightLineNo: 1},
		// Left wraps to several rows; right is a filler (empty) → must pad.
		{LeftText: long, LeftKind: diff.KindRemoved, LeftLineNo: 2,
			RightText: "", RightKind: diff.KindFiller, RightLineNo: 0},
		// Right wraps; left is filler.
		{LeftText: "", LeftKind: diff.KindFiller, LeftLineNo: 0,
			RightText: long, RightKind: diff.KindAdded, RightLineNo: 2},
	}

	const leftW, rightW = 30, 30
	left, right := renderBothSides(lines, leftW, rightW, defaultStyles())

	leftRows := strings.Count(left, "\n")
	rightRows := strings.Count(right, "\n")
	if leftRows != rightRows {
		t.Fatalf("side row counts differ: left=%d right=%d (must match for aligned scrolling)", leftRows, rightRows)
	}

	// Compute expected total rows independently.
	_, leftContent := gutterDims(lines, sideLeft, leftW)
	_, rightContent := gutterDims(lines, sideRight, rightW)
	want := 0
	for _, al := range lines {
		l := len(wrapToWidth(al.LeftText, leftContent))
		r := len(wrapToWidth(al.RightText, rightContent))
		if r > l {
			l = r
		}
		want += l
	}
	if leftRows != want {
		t.Fatalf("row count = %d, want %d", leftRows, want)
	}
}
