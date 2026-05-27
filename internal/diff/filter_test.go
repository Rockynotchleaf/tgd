package diff

import "testing"

func TestFilterTouched(t *testing.T) {
	files := []FileEntry{
		{Status: "M", Path: "cmd/tgd/main.go"},
		{Status: "??", Path: "scratch/notes.txt"}, // intentionally-untracked clutter
		{Status: "A", Path: "internal/diff/filter.go"},
		{Status: "D", Path: "old/gone.go"},
	}

	tests := []struct {
		name    string
		touched map[string]bool
		want    []string // expected Paths, in input order
	}{
		{
			name:    "nil set yields nothing",
			touched: nil,
			want:    nil,
		},
		{
			name:    "empty set yields nothing",
			touched: map[string]bool{},
			want:    nil,
		},
		{
			name:    "keeps only touched files, drops untracked clutter",
			touched: map[string]bool{"cmd/tgd/main.go": true, "internal/diff/filter.go": true},
			want:    []string{"cmd/tgd/main.go", "internal/diff/filter.go"},
		},
		{
			name:    "touched path with no git change is silently absent",
			touched: map[string]bool{"cmd/tgd/main.go": true, "never/changed.go": true},
			want:    []string{"cmd/tgd/main.go"},
		},
		{
			name:    "preserves input order, not map order",
			touched: map[string]bool{"old/gone.go": true, "cmd/tgd/main.go": true},
			want:    []string{"cmd/tgd/main.go", "old/gone.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterTouched(files, tt.touched)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries %v, want %d %v", len(got), paths(got), len(tt.want), tt.want)
			}
			for i, p := range tt.want {
				if got[i].Path != p {
					t.Errorf("entry %d: got %q, want %q", i, got[i].Path, p)
				}
			}
		})
	}
}

func paths(entries []FileEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}
