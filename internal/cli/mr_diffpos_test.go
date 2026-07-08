package cli

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
	index, err := parseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	// Added lines record the old-side cursor AFTER the preceding removal
	// incremented it (13, not 12) — this mirrors GitLab's own parser, which
	// line_code fabrication must match exactly.
	want := []diffLine{
		{kind: diffLineContext, oldLine: 10, newLine: 12},
		{kind: diffLineContext, oldLine: 11, newLine: 13},
		{kind: diffLineRemoved, oldLine: 12, newLine: 14},
		{kind: diffLineAdded, oldLine: 13, newLine: 14},
		{kind: diffLineAdded, oldLine: 13, newLine: 15},
		{kind: diffLineContext, oldLine: 13, newLine: 16},
	}
	if len(index.lines) != len(want) {
		t.Fatalf("expected %d lines, got %d: %+v", len(want), len(index.lines), index.lines)
	}
	for i, line := range want {
		if index.lines[i] != line {
			t.Fatalf("line %d: expected %+v, got %+v", i, line, index.lines[i])
		}
	}
}

func TestFileDiffIndexLookupLine(t *testing.T) {
	index, err := parseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	tests := []struct {
		name  string
		side  diffSide
		line  int64
		want  diffLine
		found bool
	}{
		{"added line on new side", sideNew, 15, diffLine{diffLineAdded, 13, 15}, true},
		{"context line with differing pair", sideNew, 13, diffLine{diffLineContext, 11, 13}, true},
		{"removed line on old side", sideOld, 12, diffLine{diffLineRemoved, 12, 14}, true},
		{"context line on old side", sideOld, 13, diffLine{diffLineContext, 13, 16}, true},
		{"added line invisible on old side", sideOld, 14, diffLine{}, false},
		{"removed line number is the added line on new side", sideNew, 14, diffLine{diffLineAdded, 13, 14}, true},
		{"line outside the hunk", sideNew, 40, diffLine{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := index.lookupLine(tt.side, tt.line)
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

	index, err := parseFileDiff(diff)
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	if len(index.hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(index.hunks))
	}
	if _, found := index.lookupLine(sideNew, 30); found {
		t.Fatal("line in the gap between hunks must not resolve")
	}
	if got := index.commentableRanges(sideNew); got != "3-4, 45-47" {
		t.Fatalf("new-side ranges: got %q", got)
	}
	if got := index.commentableRanges(sideOld); got != "3-4, 40-42" {
		t.Fatalf("old-side ranges: got %q", got)
	}
}

func TestParseFileDiffHeaderWithoutCounts(t *testing.T) {
	index, err := parseFileDiff("@@ -1 +1 @@\n-a\n+b\n")
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	want := []diffLine{
		{kind: diffLineRemoved, oldLine: 1, newLine: 1},
		{kind: diffLineAdded, oldLine: 2, newLine: 1},
	}
	if len(index.lines) != len(want) || index.lines[0] != want[0] || index.lines[1] != want[1] {
		t.Fatalf("expected %+v, got %+v", want, index.lines)
	}
	if got := index.commentableRanges(sideNew); got != "1" {
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

	index, err := parseFileDiff(diff)
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}
	if len(index.lines) != 2 {
		t.Fatalf("expected 2 lines, got %+v", index.lines)
	}
	if index.lines[0].kind != diffLineRemoved || index.lines[1].kind != diffLineAdded {
		t.Fatalf("expected removed then added, got %+v", index.lines)
	}
}

func TestParseFileDiffNewFileHasNoOldSide(t *testing.T) {
	index, err := parseFileDiff("@@ -0,0 +1,2 @@\n+x\n+y\n")
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	if _, found := index.lookupLine(sideOld, 1); found {
		t.Fatal("a pure-addition diff has no old-side lines")
	}
	if got := index.commentableRanges(sideOld); got != "" {
		t.Fatalf("expected no old-side ranges, got %q", got)
	}
	if got := index.commentableRanges(sideNew); got != "1-2" {
		t.Fatalf("new-side ranges: got %q", got)
	}
}

func TestParseLineSpec(t *testing.T) {
	tests := []struct {
		value      string
		start, end int64
		wantErr    bool
	}{
		{"5", 5, 5, false},
		{"3:9", 3, 9, false},
		{" 4 : 6 ", 4, 6, false},
		{"7:7", 7, 7, false},
		{"9:3", 0, 0, true},
		{"0", 0, 0, true},
		{"-1", 0, 0, true},
		{"a", 0, 0, true},
		{"1:b", 0, 0, true},
		{"1:", 0, 0, true},
		{":", 0, 0, true},
		{"1:2:3", 0, 0, true},
		{"", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			start, end, err := parseLineSpec("line", tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.value)
				}
				if exitCodeForError(err) != 2 {
					t.Fatalf("expected usage exit code 2, got %d", exitCodeForError(err))
				}

				return
			}
			if err != nil {
				t.Fatalf("parseLineSpec(%q): %v", tt.value, err)
			}
			if start != tt.start || end != tt.end {
				t.Fatalf("parseLineSpec(%q): expected (%d, %d), got (%d, %d)", tt.value, tt.start, tt.end, start, end)
			}
		})
	}
}

// TestDiffLineCodeGolden pins the fabricated line_code format: SHA1 hex of
// the file path, then the old and new cursors. GitLab rejects any other
// shape, so a change here is a contract break, not a refactor.
func TestDiffLineCodeGolden(t *testing.T) {
	got := diffLineCode("foo.go", 12, 15)
	want := "dbfc0996fb6f8398f85c3bdab9a47875128b47a4_12_15"
	if got != want {
		t.Fatalf("diffLineCode: expected %q, got %q", want, got)
	}
}

func TestLookupAnchorLineErrors(t *testing.T) {
	index, err := parseFileDiff(testFileDiff)
	if err != nil {
		t.Fatalf("parseFileDiff: %v", err)
	}

	_, err = lookupAnchorLine(index, sideNew, 99)
	if !errors.Is(err, errLineNotInDiff) {
		t.Fatalf("expected errLineNotInDiff, got %v", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("line_not_in_diff must stay a runtime error, got exit %d", exitCodeForError(err))
	}
	help := helpFromError(err)
	if len(help) == 0 || !strings.Contains(help[0], "12-16") {
		t.Fatalf("expected commentable new-side ranges in help, got %v", help)
	}

	// Old-side line 10 exists; requesting it as --line must suggest the
	// cross-side flag. New side spans 12-16, so 10 is not visible there...
	_, err = lookupAnchorLine(index, sideNew, 10)
	if !errors.Is(err, errLineNotInDiff) {
		t.Fatalf("expected errLineNotInDiff, got %v", err)
	}
	found := false
	for _, hint := range helpFromError(err) {
		if strings.Contains(hint, "--old-line 10") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a cross-side suggestion for --old-line 10, got %v", helpFromError(err))
	}
}
