package diffpos

import (
	"errors"
	"strings"
	"testing"
)

// testFileDiff exercises the cursor rules: a hunk with offset old/new starts,
// a removal, two additions, and context lines whose old/new numbers differ.
const testFileDiff = "@@ -10,4 +12,5 @@ func existing() {\n" +
	" ctx1\n" +
	" ctx2\n" +
	"-rm1\n" +
	"+add1\n" +
	"+add2\n" +
	" ctx3\n"

func TestParseFileDiffCursorPairs(t *testing.T) {
	index, err := ParseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	// Added lines record the old-side cursor AFTER the preceding removal
	// incremented it (13, not 12) — this mirrors GitLab's own parser, which
	// line_code fabrication must match exactly.
	want := []Line{
		{Kind: LineContext, OldLine: 10, NewLine: 12},
		{Kind: LineContext, OldLine: 11, NewLine: 13},
		{Kind: LineRemoved, OldLine: 12, NewLine: 14},
		{Kind: LineAdded, OldLine: 13, NewLine: 14},
		{Kind: LineAdded, OldLine: 13, NewLine: 15},
		{Kind: LineContext, OldLine: 13, NewLine: 16},
	}
	if len(index.Lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %+v", len(want), len(index.Lines), index.Lines)
	}
	for i, line := range want {
		if index.Lines[i] != line {
			t.Fatalf("line %d: expected %+v, got %+v", i, line, index.Lines[i])
		}
	}
}

func TestFileDiffIndexLookupLine(t *testing.T) {
	index, err := ParseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	tests := []struct {
		name  string
		side  Side
		line  int64
		want  Line
		found bool
	}{
		{"added line on new side", SideNew, 15, Line{LineAdded, 13, 15}, true},
		{"context line with differing pair", SideNew, 13, Line{LineContext, 11, 13}, true},
		{"removed line on old side", SideOld, 12, Line{LineRemoved, 12, 14}, true},
		{"context line on old side", SideOld, 13, Line{LineContext, 13, 16}, true},
		{"added line invisible on old side", SideOld, 14, Line{}, false},
		{"removed line number is the added line on new side", SideNew, 14, Line{LineAdded, 13, 14}, true},
		{"line outside the hunk", SideNew, 40, Line{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := index.LookupLine(tt.side, tt.line)
			if found != tt.found {
				t.Fatalf("expected found=%t, got %t (%+v)", tt.found, found, got)
			}
			if found && got != tt.want {
				t.Fatalf("expected %+v, got %+v", tt.want, got)
			}
		})
	}
}

func TestParseFileDiffMultiHunkAndRanges(t *testing.T) {
	diff := "@@ -3,2 +3,2 @@\n" +
		" a\n" +
		" b\n" +
		"@@ -40,3 +45,3 @@\n" +
		" c\n" +
		"-d\n" +
		"+e\n" +
		" f\n"

	index, err := ParseFileDiff(diff)
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	if len(index.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(index.Hunks))
	}
	if _, found := index.LookupLine(SideNew, 30); found {
		t.Fatal("line in the gap between hunks must not resolve")
	}
	if got := index.CommentableRanges(SideNew); got != "3-4, 45-47" {
		t.Fatalf("new-side ranges: got %q", got)
	}
	if got := index.CommentableRanges(SideOld); got != "3-4, 40-42" {
		t.Fatalf("old-side ranges: got %q", got)
	}
}

func TestParseFileDiffHeaderWithoutCounts(t *testing.T) {
	index, err := ParseFileDiff("@@ -1 +1 @@\n-a\n+b\n")
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	want := []Line{
		{Kind: LineRemoved, OldLine: 1, NewLine: 1},
		{Kind: LineAdded, OldLine: 2, NewLine: 1},
	}
	if len(index.Lines) != len(want) || index.Lines[0] != want[0] || index.Lines[1] != want[1] {
		t.Fatalf("expected %+v, got %+v", want, index.Lines)
	}
	if got := index.CommentableRanges(SideNew); got != "1" {
		t.Fatalf("single-line range: got %q", got)
	}
}

func TestParseFileDiffToleratesNoiseLines(t *testing.T) {
	diff := "--- a/foo.go\n" +
		"+++ b/foo.go\n" +
		"@@ -1,1 +1,1 @@\n" +
		"-a\n" +
		"\\ No newline at end of file\n" +
		"+b\n" +
		"\\ No newline at end of file\n"

	index, err := ParseFileDiff(diff)
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}
	if len(index.Lines) != 2 {
		t.Fatalf("expected 2 lines, got %+v", index.Lines)
	}
	if index.Lines[0].Kind != LineRemoved || index.Lines[1].Kind != LineAdded {
		t.Fatalf("expected removed then added, got %+v", index.Lines)
	}
}

func TestParseFileDiffNewFileHasNoOldSide(t *testing.T) {
	index, err := ParseFileDiff("@@ -0,0 +1,2 @@\n+x\n+y\n")
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	if _, found := index.LookupLine(SideOld, 1); found {
		t.Fatal("a pure-addition diff has no old-side lines")
	}
	if got := index.CommentableRanges(SideOld); got != "" {
		t.Fatalf("expected no old-side ranges, got %q", got)
	}
	if got := index.CommentableRanges(SideNew); got != "1-2" {
		t.Fatalf("new-side ranges: got %q", got)
	}
}

// TestDiffLineCodeGolden pins the fabricated line_code format: SHA1 hex of
// the file path, then the old and new cursors. GitLab rejects any other
// shape, so a change here is a contract break, not a refactor.
func TestDiffLineCodeGolden(t *testing.T) {
	got := LineCode("foo.go", 12, 15)
	want := "dbfc0996fb6f8398f85c3bdab9a47875128b47a4_12_15"
	if got != want {
		t.Fatalf("LineCode: expected %q, got %q", want, got)
	}
}

func TestLookupAnchorLineErrors(t *testing.T) {
	index, err := ParseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("ParseFileDiff: %v", err)
	}

	_, err = lookupAnchorLine(index, SideNew, 99)
	if !errors.Is(err, ErrLineNotInDiff) {
		t.Fatalf("expected ErrLineNotInDiff, got %v", err)
	}
	var hinted *hintedError
	if !errors.As(err, &hinted) {
		t.Fatalf("expected a hinted error, got %v", err)
	}
	help := hinted.HelpHints()
	if len(help) == 0 || !strings.Contains(help[0], "12-16") {
		t.Fatalf("expected commentable new-side ranges in help, got %v", help)
	}

	// Old-side line 10 exists; requesting it as --line must suggest the
	// cross-side flag. New side spans 12-16, so 10 is not visible there...
	_, err = lookupAnchorLine(index, SideNew, 10)
	if !errors.Is(err, ErrLineNotInDiff) {
		t.Fatalf("expected ErrLineNotInDiff, got %v", err)
	}
	hinted = nil
	if !errors.As(err, &hinted) {
		t.Fatalf("expected a hinted error, got %v", err)
	}
	found := false
	for _, hint := range hinted.HelpHints() {
		if strings.Contains(hint, "--old-line 10") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a cross-side suggestion for --old-line 10, got %v", hinted.HelpHints())
	}
}
