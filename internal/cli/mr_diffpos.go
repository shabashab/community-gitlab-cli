package cli

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errMergeRequestDiffNotReady = errors.New("merge request diff is not ready yet")
	errFileNotInDiff            = errors.New("file is not part of the merge request diff")
	errLineNotInDiff            = errors.New("line is not part of the merge request diff")
	errDiffTooLarge             = errors.New("merge request diff for this file is collapsed or too large")
)

// diffSide selects which side of a unified diff a line number addresses:
// sideNew is the file after the change (--line), sideOld the file before it
// (--old-line).
type diffSide int

const (
	sideNew diffSide = iota
	sideOld
)

type diffLineKind int

const (
	diffLineContext diffLineKind = iota
	diffLineAdded
	diffLineRemoved
)

// diffLine mirrors GitLab's diff parser: every visible diff line carries both
// side cursors at the moment it appears. An added line's oldLine is the
// old-side cursor (not zero) — GitLab derives line_code values from these
// exact pairs, so the cursors must match its parse.
type diffLine struct {
	kind    diffLineKind
	oldLine int64
	newLine int64
}

type hunkSpan struct {
	oldStart, oldCount, newStart, newCount int64
}

// fileDiffIndex is the parsed form of one file's unified-diff body: every
// visible line with its old/new cursor pair, plus the hunk spans for
// commentable-range hints.
type fileDiffIndex struct {
	lines []diffLine
	hunks []hunkSpan
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseFileDiff indexes one file's unified-diff hunk body
// (MergeRequestDiff.Diff). Content before the first hunk header (---/+++ file
// headers) is tolerated, as are "\ No newline at end of file" markers. Hunk
// line counts bound each hunk so trailing noise cannot shift the cursors.
func parseFileDiff(diff string) (*fileDiffIndex, error) {
	index := &fileDiffIndex{}

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
			index.hunks = append(index.hunks, span)
			oldCursor, newCursor = span.oldStart, span.newStart
			oldRemaining, newRemaining = span.oldCount, span.newCount
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
			index.lines = append(index.lines, diffLine{kind: diffLineAdded, oldLine: oldCursor, newLine: newCursor})
			newCursor++
			newRemaining--
		case strings.HasPrefix(line, "-"):
			if oldRemaining <= 0 {
				continue
			}
			index.lines = append(index.lines, diffLine{kind: diffLineRemoved, oldLine: oldCursor, newLine: newCursor})
			oldCursor++
			oldRemaining--
		default:
			if oldRemaining <= 0 || newRemaining <= 0 {
				continue
			}
			index.lines = append(index.lines, diffLine{kind: diffLineContext, oldLine: oldCursor, newLine: newCursor})
			oldCursor++
			newCursor++
			oldRemaining--
			newRemaining--
		}
	}

	return index, nil
}

func parseHunkHeader(match []string) (hunkSpan, error) {
	parse := func(value string, fallback int64) (int64, error) {
		if value == "" {
			return fallback, nil
		}

		return strconv.ParseInt(value, 10, 64)
	}

	span := hunkSpan{}
	var err error
	if span.oldStart, err = parse(match[1], 0); err != nil {
		return hunkSpan{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.oldCount, err = parse(match[2], 1); err != nil {
		return hunkSpan{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.newStart, err = parse(match[3], 0); err != nil {
		return hunkSpan{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}
	if span.newCount, err = parse(match[4], 1); err != nil {
		return hunkSpan{}, fmt.Errorf("parse hunk header %q: %w", match[0], err)
	}

	return span, nil
}

// lookupLine finds the diff line whose number on the given side equals line.
// Removed lines do not exist on the new side and added lines do not exist on
// the old side, even though both cursors are recorded for every line.
func (idx *fileDiffIndex) lookupLine(side diffSide, line int64) (diffLine, bool) {
	for _, candidate := range idx.lines {
		switch side {
		case sideNew:
			if candidate.kind != diffLineRemoved && candidate.newLine == line {
				return candidate, true
			}
		case sideOld:
			if candidate.kind != diffLineAdded && candidate.oldLine == line {
				return candidate, true
			}
		}
	}

	return diffLine{}, false
}

// commentableRangeLimit caps how many hunk spans a line_not_in_diff hint
// echoes.
const commentableRangeLimit = 10

// commentableRanges renders the hunk spans of one side ("3-9, 40-47") for
// line_not_in_diff hints.
func (idx *fileDiffIndex) commentableRanges(side diffSide) string {
	var spans []string
	for _, hunk := range idx.hunks {
		start, count := hunk.newStart, hunk.newCount
		if side == sideOld {
			start, count = hunk.oldStart, hunk.oldCount
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

// parseLineSpec parses a --line/--old-line value: a single 1-based line
// number ("42") or an inclusive range ("10:15").
func parseLineSpec(flagName, value string) (start, end int64, err error) {
	trimmed := strings.TrimSpace(value)
	help := fmt.Sprintf(
		"Pass --%s <line> for a single line or --%s <start>:<end> for a range, with 1-based line numbers",
		flagName,
		flagName,
	)

	parts := strings.Split(trimmed, ":")
	if trimmed == "" || len(parts) > 2 {
		return 0, 0, newUsageError(
			fmt.Errorf("invalid --%s %q: expected <line> or <start>:<end>", flagName, value),
			help,
		)
	}

	numbers := make([]int64, len(parts))
	for i, part := range parts {
		number, parseErr := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if parseErr != nil || number <= 0 {
			return 0, 0, newUsageError(
				fmt.Errorf("invalid --%s %q: line numbers must be positive integers", flagName, value),
				help,
			)
		}
		numbers[i] = number
	}

	start = numbers[0]
	end = start
	if len(numbers) == 2 {
		end = numbers[1]
	}
	if end < start {
		return 0, 0, newUsageError(
			fmt.Errorf("invalid --%s %q: range start %d is after end %d", flagName, value, start, end),
			help,
		)
	}

	return start, end, nil
}

// diffLineCode fabricates GitLab's line_code identifier for one diff line:
// SHA1 of the file path joined with the old and new cursor values. GitLab
// validates line_range endpoints against its own parse, so the cursor pair
// must come from parseFileDiff, never be guessed.
func diffLineCode(filePath string, oldLine, newLine int64) string {
	return fmt.Sprintf("%x_%d_%d", sha1.Sum([]byte(filePath)), oldLine, newLine)
}

// commentAnchor is the parsed --file/--line/--old-line target of a positioned
// comment. start is 0 for a file-level comment; end equals start for a single
// line.
type commentAnchor struct {
	file  string
	side  diffSide
	start int64
	end   int64
}

func (a commentAnchor) hasLine() bool { return a.start > 0 }

func (a commentAnchor) isRange() bool { return a.hasLine() && a.end != a.start }

const diffFetchPageSize int64 = 100

func fetchAllMergeRequestDiffs(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) ([]*gitlab.MergeRequestDiff, error) {
	opt := &gitlab.ListMergeRequestDiffsOptions{
		ListOptions: gitlab.ListOptions{PerPage: diffFetchPageSize, Page: 1},
	}

	var all []*gitlab.MergeRequestDiff
	for {
		diffs, resp, err := client.MergeRequests.ListMergeRequestDiffs(projectRef, iid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list changed files of merge request !%d in project %q: %w", iid, projectRef, err)
		}
		all = append(all, diffs...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return all, nil
}

// resolveCommentPosition turns a comment anchor into the full GitLab position
// object: diff refs from the merge request, paths from the changed-file entry,
// and the old/new line pair from the parsed diff. The caller never supplies
// SHAs or GitLab's position-side rules.
func resolveCommentPosition(ctx context.Context, client *gitlab.Client, projectRef any, iid int64, anchor commentAnchor) (*gitlab.PositionOptions, error) {
	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get merge request !%d in project %q: %w", iid, projectRef, err)
	}

	refs := mergeRequest.DiffRefs
	if refs.BaseSha == "" || refs.HeadSha == "" || refs.StartSha == "" {
		return nil, newHelpError(
			fmt.Errorf("%w: merge request !%d has no diff refs", errMergeRequestDiffNotReady, iid),
			"GitLab is still preparing the merge request diff — retry in a few seconds",
		)
	}

	diffs, err := fetchAllMergeRequestDiffs(ctx, client, projectRef, iid)
	if err != nil {
		return nil, err
	}

	entry := findDiffEntry(diffs, anchor.file)
	if entry == nil {
		return nil, newHelpError(
			fmt.Errorf("%w: %q is not changed in merge request !%d", errFileNotInDiff, anchor.file, iid),
			changedFilesHint(diffs),
		)
	}

	position := &gitlab.PositionOptions{
		BaseSHA:  gitlab.Ptr(refs.BaseSha),
		HeadSHA:  gitlab.Ptr(refs.HeadSha),
		StartSHA: gitlab.Ptr(refs.StartSha),
		OldPath:  gitlab.Ptr(entry.OldPath),
		NewPath:  gitlab.Ptr(entry.NewPath),
	}

	if !anchor.hasLine() {
		position.PositionType = gitlab.Ptr("file")

		return position, nil
	}
	position.PositionType = gitlab.Ptr("text")

	if entry.Collapsed || entry.TooLarge {
		return nil, newHelpError(
			fmt.Errorf("%w: GitLab returns no expanded diff for %q", errDiffTooLarge, anchor.file),
			"Drop --line/--old-line to comment on the file itself instead",
		)
	}

	index, err := parseFileDiff(entry.Diff)
	if err != nil {
		return nil, fmt.Errorf("parse diff of %q in merge request !%d: %w", anchor.file, iid, err)
	}

	startLine, err := lookupAnchorLine(index, anchor.side, anchor.start)
	if err != nil {
		return nil, err
	}
	endLine := startLine
	if anchor.isRange() {
		endLine, err = lookupAnchorLine(index, anchor.side, anchor.end)
		if err != nil {
			return nil, err
		}
	}

	// GitLab treats the range end as the primary commented line, so the
	// top-level old/new pair always comes from the end of the range.
	applyPositionLine(position, endLine)

	if anchor.isRange() {
		lineCodePath := entry.NewPath
		if lineCodePath == "" {
			lineCodePath = entry.OldPath
		}
		position.LineRange = &gitlab.LineRangeOptions{
			Start: linePositionFor(lineCodePath, startLine),
			End:   linePositionFor(lineCodePath, endLine),
		}
	}

	return position, nil
}

func findDiffEntry(diffs []*gitlab.MergeRequestDiff, file string) *gitlab.MergeRequestDiff {
	for _, diff := range diffs {
		if diff == nil {
			continue
		}
		if diff.NewPath == file || diff.OldPath == file {
			return diff
		}
	}

	return nil
}

// changedFilesHintLimit caps how many changed paths a file_not_in_diff hint
// echoes.
const changedFilesHintLimit = 5

func changedFilesHint(diffs []*gitlab.MergeRequestDiff) string {
	var paths []string
	for _, diff := range diffs {
		if diff == nil {
			continue
		}
		path := diff.NewPath
		if path == "" {
			path = diff.OldPath
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return "The merge request changes no files"
	}

	extra := ""
	if len(paths) > changedFilesHintLimit {
		extra = fmt.Sprintf(" and %d more", len(paths)-changedFilesHintLimit)
		paths = paths[:changedFilesHintLimit]
	}

	return fmt.Sprintf("Changed files: %s%s — pass --file with one of these paths", strings.Join(paths, ", "), extra)
}

// lookupAnchorLine resolves one requested line number against the parsed diff
// and fails loud with the commentable ranges (and a cross-side suggestion)
// when the line is not part of the diff on the requested side.
func lookupAnchorLine(index *fileDiffIndex, side diffSide, line int64) (diffLine, error) {
	found, ok := index.lookupLine(side, line)
	if ok {
		return found, nil
	}

	sideName, otherSideName := "new", "old"
	flagName, otherFlag := "--line", "--old-line"
	otherSide := sideOld
	if side == sideOld {
		sideName, otherSideName = "old", "new"
		flagName, otherFlag = "--old-line", "--line"
		otherSide = sideNew
	}

	var help []string
	if ranges := index.commentableRanges(side); ranges != "" {
		help = append(help, fmt.Sprintf("Commentable %s-side lines for this file: %s", sideName, ranges))
	} else {
		help = append(help, fmt.Sprintf("The diff has no commentable lines on the %s side", sideName))
	}
	if _, onOtherSide := index.lookupLine(otherSide, line); onOtherSide {
		help = append(help, fmt.Sprintf(
			"Line %d exists on the %s side — pass %s %d instead of %s",
			line,
			otherSideName,
			otherFlag,
			line,
			flagName,
		))
	}

	return diffLine{}, newHelpError(
		fmt.Errorf("%w: %s %d does not address a line of the current diff", errLineNotInDiff, flagName, line),
		help...,
	)
}

// applyPositionLine sets the top-level old/new line pair by line kind: added
// lines exist only on the new side, removed lines only on the old side, and
// context lines carry both numbers (which can differ when earlier hunks
// shifted the file).
func applyPositionLine(position *gitlab.PositionOptions, line diffLine) {
	switch line.kind {
	case diffLineAdded:
		position.NewLine = gitlab.Ptr(line.newLine)
	case diffLineRemoved:
		position.OldLine = gitlab.Ptr(line.oldLine)
	default:
		position.OldLine = gitlab.Ptr(line.oldLine)
		position.NewLine = gitlab.Ptr(line.newLine)
	}
}

func linePositionFor(filePath string, line diffLine) *gitlab.LinePositionOptions {
	pos := &gitlab.LinePositionOptions{
		LineCode: gitlab.Ptr(diffLineCode(filePath, line.oldLine, line.newLine)),
	}

	switch line.kind {
	case diffLineAdded:
		pos.Type = gitlab.Ptr("new")
		pos.NewLine = gitlab.Ptr(line.newLine)
	case diffLineRemoved:
		pos.Type = gitlab.Ptr("old")
		pos.OldLine = gitlab.Ptr(line.oldLine)
	default:
		pos.OldLine = gitlab.Ptr(line.oldLine)
		pos.NewLine = gitlab.Ptr(line.newLine)
	}

	return pos
}
