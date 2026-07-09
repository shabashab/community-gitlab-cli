package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/shabashab/community-gitlab-cli/internal/diffpos"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	mrDiffDefaultFields = []string{"path", "status", "additions", "deletions", "hunks"}
	mrDiffExtraFields   = []string{"old_path", "generated", "collapsed", "too_large", "new_ranges", "old_ranges"}
)

type mrDiffListOptions struct {
	file   string
	fields []string
	limit  int64
	page   int64

	fileSet bool
}

type mrDiffData struct {
	mergeRequest *gitlab.MergeRequest
	diffs        []*gitlab.MergeRequestDiff
}

func newMRDiffListOptions() *mrDiffListOptions {
	return &mrDiffListOptions{limit: output.DefaultMergeRequestListLimit, page: 1}
}

func newMRDiffCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := newMRDiffListOptions()
	var fieldsFlag string

	cmd := &cobra.Command{
		Use:     "diff <!iid|iid|current>",
		Aliases: []string{"changes"},
		Short:   "Inspect merge request changed files",
		Long: `Inspect changed files on a merge request.

The default output is a compact changed-file summary for agents. Use
"mr diff patch" for raw unified diff output, or "mr diff export" to create a
filesystem review bundle with patch files plus old/new changed-file contents.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			opts.fileSet = cmd.Flags().Changed("file")
			fields, err := parseExtraFields(fieldsFlag, mrDiffExtraFields, mrDiffDefaultFields)
			if err != nil {
				return err
			}
			opts.fields = fields
			if err := validateMRDiffListOptions(opts); err != nil {
				return err
			}

			return runMRDiff(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.file, "file", "", "Show one changed file by repository-relative old or new path")
	flags.Int64Var(&opts.limit, "limit", opts.limit, "Changed files per page")
	flags.Int64Var(&opts.page, "page", opts.page, "Page of changed files to show")
	flags.StringVar(
		&fieldsFlag,
		"fields",
		"",
		fmt.Sprintf("Comma-separated extra columns to add to the compact schema: %s", strings.Join(mrDiffExtraFields, ", ")),
	)

	cmd.AddCommand(newMRDiffPatchCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiffExportCommand(rootOpts, projOpts))

	return cmd
}

func newMRDiffPatchCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "patch <!iid|iid|current>",
		Short: "Print the raw merge request patch",
		Long: `Print the raw unified diff for a merge request.

This command is the raw patch escape hatch. It writes the patch bytes directly
to stdout and ignores the structured --output mode.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRDiffPatch(cmd, rootOpts, projOpts, iid)
		},
	}
}

func validateMRDiffListOptions(opts *mrDiffListOptions) error {
	if opts.limit < 1 {
		return newUsageError(fmt.Errorf("--limit must be at least 1, got %d", opts.limit))
	}
	if opts.page < 1 {
		return newUsageError(fmt.Errorf("--page must be at least 1, got %d", opts.page))
	}
	if opts.fileSet && strings.TrimSpace(opts.file) == "" {
		return newUsageError(
			errors.New("--file requires a non-empty path"),
			"Pass the repository-relative old or new path of a file changed by the merge request",
		)
	}

	return nil
}

func runMRDiff(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrDiffListOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	data, err := fetchMRDiffData(commandContext(cmd), client, resolved.ref, iid)
	if err != nil {
		return err
	}

	files, err := mrDiffFilesFromAPI(data.diffs)
	if err != nil {
		return err
	}
	if opts.fileSet {
		files, err = filterMRDiffFiles(data.diffs, files, opts.file, iid)
		if err != nil {
			return err
		}
	}

	rows, paging := pageMRDiffFiles(files, opts.page, opts.limit)
	hints := &output.MRDiffHintContext{MRHintContext: output.MRHintContext{Project: explicitProjectRef(projOpts), Limit: opts.limit}, IID: iid}

	return output.WriteMRDiff(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, data.mergeRequest, rows, paging, opts.fields, hints)
}

func runMRDiffPatch(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	patch, _, err := client.MergeRequests.ShowMergeRequestRawDiffs(resolved.ref, iid, &gitlab.ShowMergeRequestRawDiffsOptions{}, gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("get raw diff of merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	_, err = cmd.OutOrStdout().Write(patch)
	if err == nil && len(patch) > 0 && patch[len(patch)-1] != '\n' {
		_, err = fmt.Fprintln(cmd.OutOrStdout())
	}

	return err
}

func fetchMRDiffData(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) (mrDiffData, error) {
	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return mrDiffData{}, fmt.Errorf("get merge request !%d in project %q: %w", iid, projectRef, err)
	}
	if err := ensureMergeRequestDiffRefs(mergeRequest, iid); err != nil {
		return mrDiffData{}, err
	}

	diffs, err := diffpos.FetchAllDiffs(ctx, client, projectRef, iid)
	if err != nil {
		return mrDiffData{}, err
	}

	return mrDiffData{mergeRequest: mergeRequest, diffs: diffs}, nil
}

func ensureMergeRequestDiffRefs(mergeRequest *gitlab.MergeRequest, iid int64) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}
	refs := mergeRequest.DiffRefs
	if refs.BaseSha == "" || refs.HeadSha == "" || refs.StartSha == "" {
		return newHelpError(
			fmt.Errorf("%w: merge request !%d has no diff refs", diffpos.ErrDiffNotReady, iid),
			"GitLab is still preparing the merge request diff — retry in a few seconds",
		)
	}

	return nil
}

func mrDiffFilesFromAPI(diffs []*gitlab.MergeRequestDiff) ([]output.MRDiffFile, error) {
	files := make([]output.MRDiffFile, 0, len(diffs))
	for _, diff := range diffs {
		if diff == nil {
			continue
		}
		file, err := mrDiffFileFromAPI(diff)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

func mrDiffFileFromAPI(diff *gitlab.MergeRequestDiff) (output.MRDiffFile, error) {
	file := output.MRDiffFile{
		Path:      displayDiffPath(diff),
		OldPath:   diff.OldPath,
		Status:    diffStatus(diff),
		Generated: diff.GeneratedFile,
		Collapsed: diff.Collapsed,
		TooLarge:  diff.TooLarge,
	}
	if diff.OldPath == diff.NewPath {
		file.OldPath = ""
	}

	index, err := diffpos.ParseFileDiff(diff.Diff)
	if err != nil {
		return output.MRDiffFile{}, fmt.Errorf("parse diff of %q: %w", file.Path, err)
	}
	for _, line := range index.Lines {
		switch line.Kind {
		case diffpos.LineAdded:
			file.Additions++
		case diffpos.LineRemoved:
			file.Deletions++
		}
	}
	file.Hunks = len(index.Hunks)
	file.NewRanges = index.CommentableRanges(diffpos.SideNew)
	file.OldRanges = index.CommentableRanges(diffpos.SideOld)

	return file, nil
}

func filterMRDiffFiles(diffs []*gitlab.MergeRequestDiff, files []output.MRDiffFile, file string, iid int64) ([]output.MRDiffFile, error) {
	normalized := strings.TrimPrefix(strings.TrimSpace(file), "./")
	for _, candidate := range files {
		if candidate.Path == normalized || candidate.OldPath == normalized {
			return []output.MRDiffFile{candidate}, nil
		}
	}

	return nil, newHelpError(
		fmt.Errorf("%w: %q is not changed in merge request !%d", diffpos.ErrFileNotInDiff, normalized, iid),
		diffpos.ChangedFilesHint(diffs),
	)
}

func pageMRDiffFiles(files []output.MRDiffFile, page, limit int64) ([]output.MRDiffFile, output.MRListPaging) {
	total := int64(len(files))
	paging := output.MRListPaging{
		Page:       page,
		TotalItems: total,
		TotalPages: (total + limit - 1) / limit,
	}

	start := (page - 1) * limit
	if start >= total {
		return nil, paging
	}

	end := start + limit
	if end > total {
		end = total
	}

	return files[start:end], paging
}

func diffStatus(diff *gitlab.MergeRequestDiff) string {
	switch {
	case diff.NewFile:
		return "added"
	case diff.DeletedFile:
		return "deleted"
	case diff.RenamedFile:
		return "renamed"
	default:
		return "modified"
	}
}

func displayDiffPath(diff *gitlab.MergeRequestDiff) string {
	if diff.NewPath != "" {
		return diff.NewPath
	}

	return diff.OldPath
}
