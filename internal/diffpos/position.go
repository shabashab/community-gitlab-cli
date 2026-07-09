package diffpos

import (
	"context"
	"fmt"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Anchor is the parsed --file/--line/--old-line target of a positioned
// comment. start is 0 for a file-level comment; end equals start for a single
// line.
type Anchor struct {
	File  string
	Side  Side
	Start int64
	End   int64
}

func (a Anchor) HasLine() bool { return a.Start > 0 }

func (a Anchor) IsRange() bool { return a.HasLine() && a.End != a.Start }

const diffFetchPageSize int64 = 100

func FetchAllDiffs(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) ([]*gitlab.MergeRequestDiff, error) {
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

// ResolvePosition turns a comment anchor into the full GitLab position
// object: diff refs from the merge request, paths from the changed-file entry,
// and the old/new line pair from the parsed diff. The caller never supplies
// SHAs or GitLab's position-side rules.
func ResolvePosition(ctx context.Context, client *gitlab.Client, projectRef any, iid int64, anchor Anchor) (*gitlab.PositionOptions, error) {
	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get merge request !%d in project %q: %w", iid, projectRef, err)
	}

	refs := mergeRequest.DiffRefs
	if refs.BaseSha == "" || refs.HeadSha == "" || refs.StartSha == "" {
		return nil, withHints(
			fmt.Errorf("%w: merge request !%d has no diff refs", ErrDiffNotReady, iid),
			"GitLab is still preparing the merge request diff — retry in a few seconds",
		)
	}

	diffs, err := FetchAllDiffs(ctx, client, projectRef, iid)
	if err != nil {
		return nil, err
	}

	entry := findDiffEntry(diffs, anchor.File)
	if entry == nil {
		return nil, withHints(
			fmt.Errorf("%w: %q is not changed in merge request !%d", ErrFileNotInDiff, anchor.File, iid),
			ChangedFilesHint(diffs),
		)
	}

	position := &gitlab.PositionOptions{
		BaseSHA:  gitlab.Ptr(refs.BaseSha),
		HeadSHA:  gitlab.Ptr(refs.HeadSha),
		StartSHA: gitlab.Ptr(refs.StartSha),
		OldPath:  gitlab.Ptr(entry.OldPath),
		NewPath:  gitlab.Ptr(entry.NewPath),
	}

	if !anchor.HasLine() {
		position.PositionType = gitlab.Ptr("file")

		return position, nil
	}
	position.PositionType = gitlab.Ptr("text")

	if entry.Collapsed || entry.TooLarge {
		return nil, withHints(
			fmt.Errorf("%w: GitLab returns no expanded diff for %q", ErrDiffTooLarge, anchor.File),
			"Drop --line/--old-line to comment on the file itself instead",
		)
	}

	index, err := ParseFileDiff(entry.Diff)
	if err != nil {
		return nil, fmt.Errorf("parse diff of %q in merge request !%d: %w", anchor.File, iid, err)
	}

	startLine, err := lookupAnchorLine(index, anchor.Side, anchor.Start)
	if err != nil {
		return nil, err
	}
	endLine := startLine
	if anchor.IsRange() {
		endLine, err = lookupAnchorLine(index, anchor.Side, anchor.End)
		if err != nil {
			return nil, err
		}
	}

	// GitLab treats the range end as the primary commented line, so the
	// top-level old/new pair always comes from the end of the range.
	applyPositionLine(position, endLine)

	if anchor.IsRange() {
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

func ChangedFilesHint(diffs []*gitlab.MergeRequestDiff) string {
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
func lookupAnchorLine(index *FileIndex, side Side, line int64) (Line, error) {
	found, ok := index.LookupLine(side, line)
	if ok {
		return found, nil
	}

	sideName, otherSideName := "new", "old"
	flagName, otherFlag := "--line", "--old-line"
	otherSide := SideOld
	if side == SideOld {
		sideName, otherSideName = "old", "new"
		flagName, otherFlag = "--old-line", "--line"
		otherSide = SideNew
	}

	var help []string
	if ranges := index.CommentableRanges(side); ranges != "" {
		help = append(help, fmt.Sprintf("Commentable %s-side lines for this file: %s", sideName, ranges))
	} else {
		help = append(help, fmt.Sprintf("The diff has no commentable lines on the %s side", sideName))
	}
	if _, onOtherSide := index.LookupLine(otherSide, line); onOtherSide {
		help = append(help, fmt.Sprintf(
			"Line %d exists on the %s side — pass %s %d instead of %s",
			line,
			otherSideName,
			otherFlag,
			line,
			flagName,
		))
	}

	return Line{}, withHints(
		fmt.Errorf("%w: %s %d does not address a line of the current diff", ErrLineNotInDiff, flagName, line),
		help...,
	)
}

// applyPositionLine sets the top-level old/new line pair by line kind: added
// lines exist only on the new side, removed lines only on the old side, and
// context lines carry both numbers (which can differ when earlier hunks
// shifted the file).
func applyPositionLine(position *gitlab.PositionOptions, line Line) {
	switch line.Kind {
	case LineAdded:
		position.NewLine = gitlab.Ptr(line.NewLine)
	case LineRemoved:
		position.OldLine = gitlab.Ptr(line.OldLine)
	default:
		position.OldLine = gitlab.Ptr(line.OldLine)
		position.NewLine = gitlab.Ptr(line.NewLine)
	}
}

func linePositionFor(filePath string, line Line) *gitlab.LinePositionOptions {
	pos := &gitlab.LinePositionOptions{
		LineCode: gitlab.Ptr(LineCode(filePath, line.OldLine, line.NewLine)),
	}

	switch line.Kind {
	case LineAdded:
		pos.Type = gitlab.Ptr("new")
		pos.NewLine = gitlab.Ptr(line.NewLine)
	case LineRemoved:
		pos.Type = gitlab.Ptr("old")
		pos.OldLine = gitlab.Ptr(line.OldLine)
	default:
		pos.OldLine = gitlab.Ptr(line.OldLine)
		pos.NewLine = gitlab.Ptr(line.NewLine)
	}

	return pos
}
