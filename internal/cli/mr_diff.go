package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errUnsafeExportPath  = errors.New("unsafe export path")
	errExportDirNotEmpty = errors.New("export directory is not empty")
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

type mrDiffExportOptions struct {
	dir   string
	force bool
}

type mrDiffData struct {
	mergeRequest *gitlab.MergeRequest
	diffs        []*gitlab.MergeRequestDiff
}

type mrDiffFile struct {
	path      string
	oldPath   string
	status    string
	additions int
	deletions int
	hunks     int
	generated bool
	collapsed bool
	tooLarge  bool
	newRanges string
	oldRanges string
}

type mrDiffExportResult struct {
	Dir      string
	Files    int
	Diffs    int
	OldFiles int
	NewFiles int
	Warnings []string
}

func newMRDiffListOptions() *mrDiffListOptions {
	return &mrDiffListOptions{limit: defaultMergeRequestListLimit, page: 1}
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

func newMRDiffExportCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrDiffExportOptions{}

	cmd := &cobra.Command{
		Use:   "export <!iid|iid|current>",
		Short: "Create a filesystem review bundle for a merge request diff",
		Long: `Create a filesystem review bundle for a merge request diff.

The bundle contains manifest.toon, files.toon, patch.diff, per-file diffs, and
old/new copies of changed files pinned to the merge request diff refs.`,
		Args: wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRDiffExport(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dir, "dir", "", "Directory to create or overwrite with --force")
	flags.BoolVar(&opts.force, "force", false, "Overwrite an existing non-empty export directory")

	return cmd
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
	hints := &mrDiffHintContext{mrHintContext: mrHintContext{project: explicitProjectRef(projOpts), limit: opts.limit}, iid: iid}

	return writeMRDiff(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, data.mergeRequest, rows, paging, opts.fields, hints)
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

func runMRDiffExport(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrDiffExportOptions, iid int64) error {
	if strings.TrimSpace(opts.dir) == "" {
		return newUsageError(
			errors.New("missing required flag --dir"),
			"Pass --dir <path> to choose where the review bundle should be written",
		)
	}

	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := commandContext(cmd)
	data, err := fetchMRDiffData(ctx, client, resolved.ref, iid)
	if err != nil {
		return err
	}
	files, err := mrDiffFilesFromAPI(data.diffs)
	if err != nil {
		return err
	}

	dir, err := prepareMRDiffExportDir(opts.dir, opts.force)
	if err != nil {
		return err
	}

	result := mrDiffExportResult{Dir: dir, Files: len(files)}
	version, versionErr := latestMergeRequestDiffVersion(ctx, client, resolved.ref, iid)
	if versionErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("could not read diff version metadata: %v", versionErr))
	}

	patch, _, err := client.MergeRequests.ShowMergeRequestRawDiffs(resolved.ref, iid, &gitlab.ShowMergeRequestRawDiffsOptions{}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get raw diff of merge request !%d in project %q: %w", iid, resolved.ref, err)
	}
	if err := writeBundleFile(dir, "patch.diff", patch); err != nil {
		return err
	}

	manifest := mrDiffManifestFromData(iid, resolved.ref, data.mergeRequest, version, files, result.Warnings)
	manifestText, err := encodeTOON(manifest)
	if err != nil {
		return err
	}
	if err := writeBundleFile(dir, "manifest.toon", []byte(manifestText)); err != nil {
		return err
	}
	filesText, err := encodeTOON(mrDiffFilesDocument{Files: diffFileOutputs(files)})
	if err != nil {
		return err
	}
	if err := writeBundleFile(dir, "files.toon", []byte(filesText)); err != nil {
		return err
	}

	for _, diff := range data.diffs {
		if diff == nil {
			continue
		}
		file, err := mrDiffFileFromAPI(diff)
		if err != nil {
			return err
		}
		if err := writeBundleRepoFile(dir, "diffs", file.path+".diff", []byte(formatFilePatch(diff))); err != nil {
			return err
		}
		result.Diffs++

		if !diff.NewFile {
			if ok, warning := exportRawFile(ctx, client, resolved.ref, data.mergeRequest.DiffRefs.BaseSha, diff.OldPath, dir, "old"); ok {
				result.OldFiles++
			} else if warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
		}
		if !diff.DeletedFile {
			if ok, warning := exportRawFile(ctx, client, resolved.ref, data.mergeRequest.DiffRefs.HeadSha, diff.NewPath, dir, "new"); ok {
				result.NewFiles++
			} else if warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
		}
	}

	if len(result.Warnings) != len(manifest.Warnings) {
		manifest.Warnings = result.Warnings
		manifestText, err = encodeTOON(manifest)
		if err != nil {
			return err
		}
		if err := writeBundleFile(dir, "manifest.toon", []byte(manifestText)); err != nil {
			return err
		}
	}

	return writeMRDiffExport(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, result, iid, &mrHintContext{project: explicitProjectRef(projOpts)})
}

func fetchMRDiffData(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) (mrDiffData, error) {
	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(projectRef, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return mrDiffData{}, fmt.Errorf("get merge request !%d in project %q: %w", iid, projectRef, err)
	}
	if err := ensureMergeRequestDiffRefs(mergeRequest, iid); err != nil {
		return mrDiffData{}, err
	}

	diffs, err := fetchAllMergeRequestDiffs(ctx, client, projectRef, iid)
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
			fmt.Errorf("%w: merge request !%d has no diff refs", errMergeRequestDiffNotReady, iid),
			"GitLab is still preparing the merge request diff — retry in a few seconds",
		)
	}

	return nil
}

func mrDiffFilesFromAPI(diffs []*gitlab.MergeRequestDiff) ([]mrDiffFile, error) {
	files := make([]mrDiffFile, 0, len(diffs))
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

func mrDiffFileFromAPI(diff *gitlab.MergeRequestDiff) (mrDiffFile, error) {
	file := mrDiffFile{
		path:      displayDiffPath(diff),
		oldPath:   diff.OldPath,
		status:    diffStatus(diff),
		generated: diff.GeneratedFile,
		collapsed: diff.Collapsed,
		tooLarge:  diff.TooLarge,
	}
	if diff.OldPath == diff.NewPath {
		file.oldPath = ""
	}

	index, err := parseFileDiff(diff.Diff)
	if err != nil {
		return mrDiffFile{}, fmt.Errorf("parse diff of %q: %w", file.path, err)
	}
	for _, line := range index.lines {
		switch line.kind {
		case diffLineAdded:
			file.additions++
		case diffLineRemoved:
			file.deletions++
		}
	}
	file.hunks = len(index.hunks)
	file.newRanges = index.commentableRanges(sideNew)
	file.oldRanges = index.commentableRanges(sideOld)

	return file, nil
}

func filterMRDiffFiles(diffs []*gitlab.MergeRequestDiff, files []mrDiffFile, file string, iid int64) ([]mrDiffFile, error) {
	normalized := strings.TrimPrefix(strings.TrimSpace(file), "./")
	for _, candidate := range files {
		if candidate.path == normalized || candidate.oldPath == normalized {
			return []mrDiffFile{candidate}, nil
		}
	}

	return nil, newHelpError(
		fmt.Errorf("%w: %q is not changed in merge request !%d", errFileNotInDiff, normalized, iid),
		changedFilesHint(diffs),
	)
}

func pageMRDiffFiles(files []mrDiffFile, page, limit int64) ([]mrDiffFile, mrListPaging) {
	total := int64(len(files))
	paging := mrListPaging{
		page:       page,
		totalItems: total,
		totalPages: (total + limit - 1) / limit,
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

func formatFilePatch(diff *gitlab.MergeRequestDiff) string {
	oldPath := diff.OldPath
	newPath := diff.NewPath
	if oldPath == "" {
		oldPath = newPath
	}
	if newPath == "" {
		newPath = oldPath
	}

	oldLabel := "a/" + oldPath
	newLabel := "b/" + newPath
	if diff.NewFile {
		oldLabel = "/dev/null"
	}
	if diff.DeletedFile {
		newLabel = "/dev/null"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", oldPath, newPath)
	if diff.RenamedFile {
		fmt.Fprintf(&b, "rename from %s\nrename to %s\n", diff.OldPath, diff.NewPath)
	}
	if diff.NewFile && diff.BMode != "" {
		fmt.Fprintf(&b, "new file mode %s\n", diff.BMode)
	}
	if diff.DeletedFile && diff.AMode != "" {
		fmt.Fprintf(&b, "deleted file mode %s\n", diff.AMode)
	}
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldLabel, newLabel)
	b.WriteString(diff.Diff)
	if !strings.HasSuffix(diff.Diff, "\n") {
		b.WriteString("\n")
	}

	return b.String()
}

func latestMergeRequestDiffVersion(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) (*gitlab.MergeRequestDiffVersion, error) {
	opt := &gitlab.GetMergeRequestDiffVersionsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
	}
	var latest *gitlab.MergeRequestDiffVersion
	for {
		versions, resp, err := client.MergeRequests.GetMergeRequestDiffVersions(projectRef, iid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list diff versions of merge request !%d in project %q: %w", iid, projectRef, err)
		}
		for _, version := range versions {
			if version == nil {
				continue
			}
			if latest == nil || version.ID > latest.ID {
				latest = version
			}
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return latest, nil
}

func prepareMRDiffExportDir(dir string, force bool) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(dir))
	if clean == "." || clean == "" {
		return "", newUsageError(errors.New("--dir must not be empty"))
	}

	if info, err := os.Stat(clean); err == nil {
		if !info.IsDir() {
			if !force {
				return "", newUsageError(
					fmt.Errorf("%w: %s exists and is not a directory", errExportDirNotEmpty, clean),
					"Pass --force to replace it, or choose an empty directory",
				)
			}
			if err := os.RemoveAll(clean); err != nil {
				return "", fmt.Errorf("replace export path %s: %w", clean, err)
			}
		} else if nonEmpty, err := dirIsNonEmpty(clean); err != nil {
			return "", err
		} else if nonEmpty {
			if !force {
				return "", newUsageError(
					fmt.Errorf("%w: %s", errExportDirNotEmpty, clean),
					"Pass --force to overwrite it, or choose an empty directory",
				)
			}
			if err := os.RemoveAll(clean); err != nil {
				return "", fmt.Errorf("replace export directory %s: %w", clean, err)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect export directory %s: %w", clean, err)
	}

	if err := os.MkdirAll(clean, 0o755); err != nil {
		return "", fmt.Errorf("create export directory %s: %w", clean, err)
	}

	return clean, nil
}

func dirIsNonEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, fmt.Errorf("read export directory %s: %w", dir, err)
	}

	return len(entries) > 0, nil
}

func exportRawFile(ctx context.Context, client *gitlab.Client, projectRef any, ref string, repoPath string, dir string, side string) (bool, string) {
	if strings.TrimSpace(repoPath) == "" {
		return false, ""
	}

	body, _, err := client.RepositoryFiles.GetRawFile(projectRef, repoPath, &gitlab.GetRawFileOptions{Ref: gitlab.Ptr(ref)}, gitlab.WithContext(ctx))
	if err != nil {
		return false, fmt.Sprintf("could not export %s/%s at %s: %v", side, repoPath, ref, err)
	}
	if err := writeBundleRepoFile(dir, side, repoPath, body); err != nil {
		return false, err.Error()
	}

	return true, ""
}

func writeBundleFile(root, name string, body []byte) error {
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create bundle directory for %s: %w", name, err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fmt.Errorf("write bundle file %s: %w", name, err)
	}

	return nil
}

func writeBundleRepoFile(root, section, repoPath string, body []byte) error {
	safe, err := safeRepoPath(repoPath)
	if err != nil {
		return err
	}
	relative := path.Join(section, safe)
	target := filepath.Join(root, filepath.FromSlash(relative))
	if err := ensureBundlePath(root, target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create bundle directory for %s: %w", relative, err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fmt.Errorf("write bundle file %s: %w", relative, err)
	}

	return nil
}

func safeRepoPath(repoPath string) (string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(repoPath), "./")
	if trimmed == "" || strings.Contains(trimmed, "\\") || path.IsAbs(trimmed) {
		return "", fmt.Errorf("%w: %q", errUnsafeExportPath, repoPath)
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", errUnsafeExportPath, repoPath)
	}

	return clean, nil
}

func ensureBundlePath(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return fmt.Errorf("%w: %s escapes %s", errUnsafeExportPath, target, root)
	}

	return nil
}
