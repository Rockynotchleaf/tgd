package diff

import (
	"testing"

	godiff "github.com/sourcegraph/go-diff/diff"
)

// helper: parse raw unified diff and align it
func alignFrom(t *testing.T, rawDiff string, orig, curr []string) []AlignedLine {
	t.Helper()
	hunks, err := ParseHunks([]byte(rawDiff))
	if err != nil {
		t.Fatalf("ParseHunks: %v", err)
	}
	return Align(hunks, orig, curr)
}

// minimalDiff produces a minimal unified diff header for testing.
func minimalDiff(hunkHeader, body string) string {
	return "diff --git a/f b/f\n--- a/f\n+++ b/f\n" + hunkHeader + "\n" + body
}

func TestAlign_SimpleReplace(t *testing.T) {
	// orig: [a, b, c]   curr: [a, X, c]   — line 2 replaced
	orig := []string{"a", "b", "c"}
	curr := []string{"a", "X", "c"}

	rawDiff := minimalDiff("@@ -1,3 +1,3 @@",
		" a\n-b\n+X\n c\n")

	got := alignFrom(t, rawDiff, orig, curr)

	// Expect: a/a (context), b/X (replace), c/c (context)
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d: %+v", len(got), got)
	}
	assertRow(t, got[0], KindContext, "a", KindContext, "a")
	assertLineNos(t, got[0], 1, 1)
	assertRow(t, got[1], KindRemoved, "b", KindAdded, "X")
	assertLineNos(t, got[1], 2, 2) // both sides have line 2 (replace pair)
	assertRow(t, got[2], KindContext, "c", KindContext, "c")
	assertLineNos(t, got[2], 3, 3)
}

func TestAlign_PureInsert(t *testing.T) {
	// orig: [a, b]   curr: [a, NEW, b]   — insert in middle
	orig := []string{"a", "b"}
	curr := []string{"a", "NEW", "b"}

	rawDiff := minimalDiff("@@ -1,2 +1,3 @@",
		" a\n+NEW\n b\n")

	got := alignFrom(t, rawDiff, orig, curr)

	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	assertRow(t, got[0], KindContext, "a", KindContext, "a")
	assertLineNos(t, got[0], 1, 1)
	assertRow(t, got[1], KindFiller, "", KindAdded, "NEW")
	assertLineNos(t, got[1], 0, 2) // filler on left (no orig line), line 2 on right
	assertRow(t, got[2], KindContext, "b", KindContext, "b")
	assertLineNos(t, got[2], 2, 3) // orig line 2, curr line 3
}

func TestAlign_PureDelete(t *testing.T) {
	// orig: [a, b, c]   curr: [a, c]   — delete line 2
	orig := []string{"a", "b", "c"}
	curr := []string{"a", "c"}

	rawDiff := minimalDiff("@@ -1,3 +1,2 @@",
		" a\n-b\n c\n")

	got := alignFrom(t, rawDiff, orig, curr)

	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	assertRow(t, got[0], KindContext, "a", KindContext, "a")
	assertLineNos(t, got[0], 1, 1)
	assertRow(t, got[1], KindRemoved, "b", KindFiller, "")
	assertLineNos(t, got[1], 2, 0) // orig line 2, filler on right (no curr line)
	assertRow(t, got[2], KindContext, "c", KindContext, "c")
	assertLineNos(t, got[2], 3, 2) // orig line 3, curr line 2 (b was deleted)
}

func TestAlign_MultipleInsertVsOneDelete(t *testing.T) {
	// orig: [old]   curr: [new1, new2]   — 1 delete + 2 inserts
	orig := []string{"old"}
	curr := []string{"new1", "new2"}

	rawDiff := minimalDiff("@@ -1,1 +1,2 @@",
		"-old\n+new1\n+new2\n")

	got := alignFrom(t, rawDiff, orig, curr)

	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d: %+v", len(got), got)
	}
	// Row 0: old paired with new1
	assertRow(t, got[0], KindRemoved, "old", KindAdded, "new1")
	assertLineNos(t, got[0], 1, 1) // orig line 1, curr line 1
	// Row 1: filler paired with new2
	assertRow(t, got[1], KindFiller, "", KindAdded, "new2")
	assertLineNos(t, got[1], 0, 2) // filler on left, curr line 2
}

func TestAlign_NewFile(t *testing.T) {
	curr := []string{"line1", "line2", "line3"}
	got := AlignNewFile(curr)
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	for i, al := range got {
		if al.LeftKind != KindFiller {
			t.Errorf("row %d: left kind want Filler, got %v", i, al.LeftKind)
		}
		if al.LeftText != "" {
			t.Errorf("row %d: left text want empty, got %q", i, al.LeftText)
		}
		if al.RightKind != KindAdded {
			t.Errorf("row %d: right kind want Added, got %v", i, al.RightKind)
		}
		if al.RightText != curr[i] {
			t.Errorf("row %d: right text want %q, got %q", i, curr[i], al.RightText)
		}
		// Left is filler (no line number); right is 1-based
		assertLineNos(t, al, 0, i+1)
	}
}

func TestAlign_DeletedFile(t *testing.T) {
	orig := []string{"a", "b"}
	got := AlignDeletedFile(orig)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	for i, al := range got {
		if al.LeftKind != KindRemoved {
			t.Errorf("row %d: left kind want Removed, got %v", i, al.LeftKind)
		}
		if al.RightKind != KindFiller {
			t.Errorf("row %d: right kind want Filler, got %v", i, al.RightKind)
		}
		// Left is 1-based; right is filler (no line number)
		assertLineNos(t, al, i+1, 0)
	}
}

func TestAlign_NoHunks(t *testing.T) {
	orig := []string{"a", "b"}
	curr := []string{"a", "b"}
	var hunks []*godiff.Hunk
	got := Align(hunks, orig, curr)
	// With no hunks, all lines are context
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	for i, al := range got {
		if al.LeftKind != KindContext || al.RightKind != KindContext {
			t.Errorf("row %d: want context/context, got %v/%v", i, al.LeftKind, al.RightKind)
		}
	}
}

func TestAlign_ContextPreserved(t *testing.T) {
	// Large file with a change in the middle; context lines before and after
	orig := []string{"1", "2", "3", "OLD", "5", "6", "7"}
	curr := []string{"1", "2", "3", "NEW", "5", "6", "7"}

	rawDiff := minimalDiff("@@ -1,7 +1,7 @@",
		" 1\n 2\n 3\n-OLD\n+NEW\n 5\n 6\n 7\n")

	got := alignFrom(t, rawDiff, orig, curr)

	if len(got) != 7 {
		t.Fatalf("want 7 rows, got %d", len(got))
	}
	// Context rows
	for _, i := range []int{0, 1, 2, 4, 5, 6} {
		if got[i].LeftKind != KindContext {
			t.Errorf("row %d should be context", i)
		}
	}
	// Replace row
	assertRow(t, got[3], KindRemoved, "OLD", KindAdded, "NEW")
}

// assertRow checks a single AlignedLine's kinds and texts.
func assertRow(t *testing.T, al AlignedLine, lk LineKind, lt string, rk LineKind, rt string) {
	t.Helper()
	if al.LeftKind != lk {
		t.Errorf("left kind: want %v, got %v", lk, al.LeftKind)
	}
	if al.LeftText != lt {
		t.Errorf("left text: want %q, got %q", lt, al.LeftText)
	}
	if al.RightKind != rk {
		t.Errorf("right kind: want %v, got %v", rk, al.RightKind)
	}
	if al.RightText != rt {
		t.Errorf("right text: want %q, got %q", rt, al.RightText)
	}
}

// assertLineNos checks the line numbers for a single AlignedLine.
// Pass 0 to assert that a side has no line number (filler row).
func assertLineNos(t *testing.T, al AlignedLine, wantLeft, wantRight int) {
	t.Helper()
	if al.LeftLineNo != wantLeft {
		t.Errorf("left lineNo: want %d, got %d", wantLeft, al.LeftLineNo)
	}
	if al.RightLineNo != wantRight {
		t.Errorf("right lineNo: want %d, got %d", wantRight, al.RightLineNo)
	}
}
