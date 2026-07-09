package cli

import (
	"fmt"
	"io"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mrDiffSummaryOutput struct {
	IID      int64  `json:"iid" toon:"iid"`
	BaseSHA  string `json:"base_sha" toon:"base_sha"`
	StartSHA string `json:"start_sha" toon:"start_sha"`
	HeadSHA  string `json:"head_sha" toon:"head_sha"`
	Files    int    `json:"files" toon:"files"`
}

type mrDiffFileOutput struct {
	Path      string `json:"path" toon:"path"`
	Status    string `json:"status" toon:"status"`
	Additions int    `json:"additions" toon:"additions"`
	Deletions int    `json:"deletions" toon:"deletions"`
	Hunks     int    `json:"hunks" toon:"hunks"`
	OldPath   string `json:"old_path,omitempty" toon:"old_path,omitempty"`
	Generated bool   `json:"generated" toon:"generated"`
	Collapsed bool   `json:"collapsed" toon:"collapsed"`
	TooLarge  bool   `json:"too_large" toon:"too_large"`
	NewRanges string `json:"new_ranges,omitempty" toon:"new_ranges,omitempty"`
	OldRanges string `json:"old_ranges,omitempty" toon:"old_ranges,omitempty"`
}

type axiMRDiffFileRow struct {
	Path      string  `json:"path" toon:"path"`
	Status    string  `json:"status" toon:"status"`
	Additions int     `json:"additions" toon:"additions"`
	Deletions int     `json:"deletions" toon:"deletions"`
	Hunks     int     `json:"hunks" toon:"hunks"`
	OldPath   *string `json:"old_path,omitempty" toon:"old_path,omitempty"`
	Generated *bool   `json:"generated,omitempty" toon:"generated,omitempty"`
	Collapsed *bool   `json:"collapsed,omitempty" toon:"collapsed,omitempty"`
	TooLarge  *bool   `json:"too_large,omitempty" toon:"too_large,omitempty"`
	NewRanges *string `json:"new_ranges,omitempty" toon:"new_ranges,omitempty"`
	OldRanges *string `json:"old_ranges,omitempty" toon:"old_ranges,omitempty"`
}

type axiMRDiffOutput struct {
	Diff       mrDiffSummaryOutput `json:"diff" toon:"diff"`
	Files      []axiMRDiffFileRow  `json:"files" toon:"files"`
	Count      string              `json:"count" toon:"count"`
	Total      int64               `json:"total" toon:"-"`
	Page       int64               `json:"page" toon:"-"`
	TotalPages int64               `json:"total_pages" toon:"-"`
	Help       []string            `json:"help,omitempty" toon:"help,omitempty"`
}

type mrDiffOutput struct {
	Diff       mrDiffSummaryOutput `json:"diff" toon:"diff"`
	Files      []mrDiffFileOutput  `json:"files" toon:"files"`
	Count      int                 `json:"count" toon:"-"`
	Total      int64               `json:"total" toon:"-"`
	Page       int64               `json:"page" toon:"-"`
	TotalPages int64               `json:"total_pages" toon:"-"`
}

type mrDiffFilesDocument struct {
	Files []mrDiffFileOutput `json:"files" toon:"files"`
}

type mrDiffManifestVersionOutput struct {
	ID        int64  `json:"id" toon:"id"`
	State     string `json:"state,omitempty" toon:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty" toon:"created_at,omitempty"`
}

type mrDiffManifestOutput struct {
	IID         int64                        `json:"iid" toon:"iid"`
	Project     string                       `json:"project" toon:"project"`
	BaseSHA     string                       `json:"base_sha" toon:"base_sha"`
	StartSHA    string                       `json:"start_sha" toon:"start_sha"`
	HeadSHA     string                       `json:"head_sha" toon:"head_sha"`
	DiffVersion *mrDiffManifestVersionOutput `json:"diff_version,omitempty" toon:"diff_version,omitempty"`
	Files       int                          `json:"files" toon:"files"`
	Warnings    []string                     `json:"warnings,omitempty" toon:"warnings,omitempty"`
	Help        []string                     `json:"help,omitempty" toon:"help,omitempty"`
}

type mrDiffExportOutput struct {
	Dir      string   `json:"dir" toon:"dir"`
	Files    int      `json:"files" toon:"files"`
	Diffs    int      `json:"diffs" toon:"diffs"`
	OldFiles int      `json:"old_files" toon:"old_files"`
	NewFiles int      `json:"new_files" toon:"new_files"`
	Warnings []string `json:"warnings,omitempty" toon:"warnings,omitempty"`
}

type axiMRDiffExportOutput struct {
	Export mrDiffExportOutput `json:"export" toon:"export"`
	Help   []string           `json:"help,omitempty" toon:"help,omitempty"`
}

type mrDiffHintContext struct {
	mrHintContext
	iid int64
}

func writeMRDiff(w io.Writer, format string, mode commandMode, mergeRequest *gitlab.MergeRequest, files []mrDiffFile, paging mrListPaging, fields []string, hints *mrDiffHintContext) error {
	summary := mrDiffSummaryFromMR(mergeRequest, int(paging.totalItems))
	fullRows := diffFileOutputs(files)

	if mode == commandModeAxi {
		rows := make([]axiMRDiffFileRow, 0, len(fullRows))
		for _, file := range fullRows {
			rows = append(rows, axiMRDiffFileRowFor(file, fields))
		}

		return writeAxi(w, format, axiMRDiffOutput{
			Diff:       summary,
			Files:      rows,
			Count:      mrListCountLine(len(rows), paging),
			Total:      paging.totalItems,
			Page:       paging.page,
			TotalPages: paging.totalPages,
			Help:       mrDiffHelp(len(rows), paging, hints),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(w, mrDiffOutput{
			Diff:       summary,
			Files:      fullRows,
			Count:      len(fullRows),
			Total:      paging.totalItems,
			Page:       paging.page,
			TotalPages: paging.totalPages,
		})
	}

	return renderMRDiffTable(w, fullRows, paging)
}

func mrDiffSummaryFromMR(mergeRequest *gitlab.MergeRequest, files int) mrDiffSummaryOutput {
	refs := mergeRequest.DiffRefs
	return mrDiffSummaryOutput{
		IID:      mergeRequest.IID,
		BaseSHA:  refs.BaseSha,
		StartSHA: refs.StartSha,
		HeadSHA:  refs.HeadSha,
		Files:    files,
	}
}

func diffFileOutputs(files []mrDiffFile) []mrDiffFileOutput {
	out := make([]mrDiffFileOutput, 0, len(files))
	for _, file := range files {
		out = append(out, mrDiffFileOutput{
			Path:      file.path,
			Status:    file.status,
			Additions: file.additions,
			Deletions: file.deletions,
			Hunks:     file.hunks,
			OldPath:   file.oldPath,
			Generated: file.generated,
			Collapsed: file.collapsed,
			TooLarge:  file.tooLarge,
			NewRanges: file.newRanges,
			OldRanges: file.oldRanges,
		})
	}

	return out
}

func axiMRDiffFileRowFor(file mrDiffFileOutput, fields []string) axiMRDiffFileRow {
	row := axiMRDiffFileRow{
		Path:      file.Path,
		Status:    file.Status,
		Additions: file.Additions,
		Deletions: file.Deletions,
		Hunks:     file.Hunks,
	}
	for _, field := range fields {
		switch field {
		case "old_path":
			row.OldPath = &file.OldPath
		case "generated":
			row.Generated = &file.Generated
		case "collapsed":
			row.Collapsed = &file.Collapsed
		case "too_large":
			row.TooLarge = &file.TooLarge
		case "new_ranges":
			row.NewRanges = &file.NewRanges
		case "old_ranges":
			row.OldRanges = &file.OldRanges
		}
	}

	return row
}

func mrDiffHelp(count int, paging mrListPaging, hints *mrDiffHintContext) []string {
	suffix := hints.projectSuffix()
	if count == 0 {
		if paging.totalItems > 0 {
			return []string{fmt.Sprintf(
				"Page %d is past the end (%d changed files, %d pages) — run `mr diff %d --page 1%s`",
				paging.page,
				paging.totalItems,
				paging.totalPages,
				hints.iid,
				suffix,
			)}
		}

		return []string{fmt.Sprintf("No changed files found — run `mr view %d%s` to inspect the merge request", hints.iid, suffix)}
	}

	help := []string{
		fmt.Sprintf("Run `mr diff %d --file <path> --fields new_ranges,old_ranges%s` for one file", hints.iid, suffix),
		fmt.Sprintf("Run `mr diff export %d --dir .gl-axi/mr-%d%s` to create a filesystem review bundle", hints.iid, hints.iid, suffix),
		fmt.Sprintf("Run `mr comment %d --file <path> --line <line> --body <text>%s` to comment on a diff line", hints.iid, suffix),
	}
	if paging.totalPages > paging.page {
		help = append(help, fmt.Sprintf("Run `mr diff %d --page %d%s` for the next page", hints.iid, paging.page+1, suffix))
	}

	return help
}

func mrDiffManifestFromData(iid int64, projectRef any, mergeRequest *gitlab.MergeRequest, version *gitlab.MergeRequestDiffVersion, files []mrDiffFile, warnings []string) mrDiffManifestOutput {
	refs := mergeRequest.DiffRefs
	out := mrDiffManifestOutput{
		IID:      iid,
		Project:  fmt.Sprint(projectRef),
		BaseSHA:  refs.BaseSha,
		StartSHA: refs.StartSha,
		HeadSHA:  refs.HeadSha,
		Files:    len(files),
		Warnings: warnings,
		Help: []string{
			fmt.Sprintf("Run `mr diff %d --project %s` to refresh the changed-file summary", iid, projectRef),
			fmt.Sprintf("Run `mr comment %d --file <path> --line <line> --body <text> --project %s` to comment from this bundle", iid, projectRef),
		},
	}
	if version != nil {
		out.DiffVersion = &mrDiffManifestVersionOutput{
			ID:        version.ID,
			State:     version.State,
			CreatedAt: formatTimeValue(version.CreatedAt),
		}
	}

	return out
}

func writeMRDiffExport(w io.Writer, format string, mode commandMode, result mrDiffExportResult, iid int64, hints *mrHintContext) error {
	out := mrDiffExportOutput{
		Dir:      result.Dir,
		Files:    result.Files,
		Diffs:    result.Diffs,
		OldFiles: result.OldFiles,
		NewFiles: result.NewFiles,
		Warnings: result.Warnings,
	}

	if mode == commandModeAxi {
		help := []string{
			fmt.Sprintf("Inspect `%s/manifest.toon`, `%s/files.toon`, and `%s/new/`", result.Dir, result.Dir, result.Dir),
			fmt.Sprintf("Run `mr drafts publish %d --all%s` after adding draft review comments", iid, hints.projectSuffix()),
		}

		return writeAxi(w, format, axiMRDiffExportOutput{Export: out, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}
	if format == "json" {
		return writeJSON(w, out)
	}

	_, err = fmt.Fprintf(
		w,
		"export: %s\nfiles: %d\ndiffs: %d\nold_files: %d\nnew_files: %d\n",
		out.Dir,
		out.Files,
		out.Diffs,
		out.OldFiles,
		out.NewFiles,
	)
	if err != nil {
		return err
	}
	for _, warning := range out.Warnings {
		if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
			return err
		}
	}

	return nil
}
