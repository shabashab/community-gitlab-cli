package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errUnsafeExportPath  = errors.New("unsafe export path")
	errExportDirNotEmpty = errors.New("export directory is not empty")
)

type mrDiffExportOptions struct {
	dir   string
	force bool
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

	result := output.MRDiffExportResult{Dir: dir, Files: len(files)}
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

	manifest := output.MRDiffManifestFromData(iid, resolved.ref, data.mergeRequest, version, files, result.Warnings)
	manifestText, err := output.EncodeTOON(manifest)
	if err != nil {
		return err
	}
	if err := writeBundleFile(dir, "manifest.toon", []byte(manifestText)); err != nil {
		return err
	}
	filesText, err := output.EncodeTOON(output.MRDiffFilesDocument{Files: output.DiffFileOutputs(files)})
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
		if err := writeBundleRepoFile(dir, "diffs", file.Path+".diff", []byte(formatFilePatch(diff))); err != nil {
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
		manifestText, err = output.EncodeTOON(manifest)
		if err != nil {
			return err
		}
		if err := writeBundleFile(dir, "manifest.toon", []byte(manifestText)); err != nil {
			return err
		}
	}

	return output.WriteMRDiffExport(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, result, iid, &output.MRHintContext{Project: explicitProjectRef(projOpts)})
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
