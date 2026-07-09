// Package diffpos parses GitLab unified diffs and builds merge request
// comment positions, mirroring GitLab's own diff cursor semantics exactly.
package diffpos

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Side selects which side of a unified diff a line number addresses:
// SideNew is the file after the change (--line), SideOld the file before it
// (--old-line).
type Side int

const (
	SideNew Side = iota
	SideOld
)

type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
)

// Line mirrors GitLab's diff parser: every visible diff line carries both
// side cursors at the moment it appears. An added line's oldLine is the
// old-side cursor (not zero) — GitLab derives line_code values from these
// exact pairs, so the cursors must match its parse.
type Line struct {
	Kind    LineKind
	OldLine int64
	NewLine int64
}

type Hunk struct {
	OldStart, OldCount, NewStart, NewCount int64
}

// FileIndex is the parsed form of one file's unified-diff body: every
// visible line with its old/new cursor pair, plus the hunk spans for
// commentable-range hints.
type FileIndex struct {
	Lines []Line
	Hunks []Hunk
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseFileDiff indexes one file's unified-diff hunk body
// (MergeRequestDiff.Diff). Content before the first hunk header (---/+++ file
// headers) is tolerated, as are "\ No newline at end of file" markers. Hunk
// line counts bound each hunk so trailing noise cannot shift the cursors.
func ParseFileDiff(diff string) (*FileIndex, error) {
	index := &FileIndex{}

	var (
		inHunk                     bool
		oldCursor, newCursor       int64
		oldRemaining, newRemaining int64
	)

	for _, line := range strings.Split(diff, "\n") {
		if match := hunkHeaderPattern.FindStringSubmatch(line); match != nil {
			span, err := parseHunkHeader(match)
			if err != nil {
				return nil, err
			}
			index.Hunks = append(index.Hunks, span)
			oldCursor, newCursor = span.OldStart, span.NewStart
			oldRemaining, newRemaining = span.OldCount, span.NewCount
			inHunk = true
			continue
		}
		if !inHunk || (oldRemaining <= 0 && newRemaining <= 0) {
			continue
		}
		if strings.HasPrefix(line, `\`) { // "\ No newline at end of file"
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			if newRemaining <= 0 {
				continue
			}
			index.Lines = append(index.Lines, Line{Kind: LineAdded, OldLine: oldCursor, NewLine: newCursor})
			newCursor++
			newRemaining--
		case strings.HasPrefix(line, "-"):
			if oldRemaining <= 0 {
				continue
			}
			index.Lines = append(index.Lines, Line{Kind: LineRemoved, OldLine: oldCursor, NewLine: newCursor})
			oldCursor++
			oldRemaining--
		default:
			if oldRemaining <= 0 || newRemaining <= 0 {
				continue
			}
			index.Lines = append(index.Lines, Line{Kind: LineContext, OldLine: oldCursor, NewLine: newCursor})
			oldCursor++
			newCursor++
			oldRemaining--
			newRemaining--
		}
	}

	return index, nil
}

func parseHunkHeader(match []string) (Hunk, error) {
	parse := func(value string, fallback int64) (int64, error) {
		if value == "" {
			return fallback, nil
		}

		return strconv.ParseInt(value, 10, 64)
	}

	span := Hunk{}
	var err error
	if span.OldStart, err = parse(match[1], 0); err != nil {
		return Hunk{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.OldCount, err = parse(match[2], 1); err != nil {
		return Hunk{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.NewStart, err = parse(match[3], 0); err != nil {
		return Hunk{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.NewCount, err = parse(match[4], 1); err != nil {
		return Hunk{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}

	return span, nil
}

// LookupLine finds the diff line whose number on the given side equals line.
// Removed lines do not exist on the new side and added lines do not exist on
// the old side, even though both cursors are recorded for every line.
func (idx *FileIndex) LookupLine(side Side, line int64) (Line, bool) {
	for _, candidate := range idx.Lines {
		switch side {
		case SideNew:
			if candidate.Kind != LineRemoved && candidate.NewLine == line {
				return candidate, true
			}
		case SideOld:
			if candidate.Kind != LineAdded && candidate.OldLine == line {
				return candidate, true
			}
		}
	}

	return Line{}, false
}

// commentableRangeLimit caps how many hunk spans a line_not_in_diff hint
// echoes.
const commentableRangeLimit = 10

// CommentableRanges renders the hunk spans of one side ("3-9, 40-47") for
// line_not_in_diff hints.
func (idx *FileIndex) CommentableRanges(side Side) string {
	var spans []string
	for _, hunk := range idx.Hunks {
		start, count := hunk.NewStart, hunk.NewCount
		if side == SideOld {
			start, count = hunk.OldStart, hunk.OldCount
		}
		if count <= 0 || start <= 0 {
			continue
		}

		if count == 1 {
			spans = append(spans, strconv.FormatInt(start, 10))
		} else {
			spans = append(spans, fmt.Sprintf("%d-%d", start, start+count-1))
		}
	}
	if len(spans) > commentableRangeLimit {
		spans = append(spans[:commentableRangeLimit], "…")
	}

	return strings.Join(spans, ", ")
}

// LineCode fabricates GitLab's line_code identifier for one diff line:
// SHA1 of the file path joined with the old and new cursor values. GitLab
// validates line_range endpoints against its own parse, so the cursor pair
// must come from ParseFileDiff, never be guessed.
func LineCode(filePath string, oldLine, newLine int64) string {
	return fmt.Sprintf("%x_%d_%d", sha1.Sum([]byte(filePath)), oldLine, newLine)
}
