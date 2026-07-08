package cli

import (
	"fmt"
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

func renderMergeRequestTable(w io.Writer, rows []mergeRequestRowOutput, paging mrListPaging) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No merge requests found. Try --state all or relax other filters.")
		return err
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.SetStyle(table.StyleRounded)
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "TITLE", WidthMax: 60},
	})
	tw.AppendHeader(table.Row{"IID", "TITLE", "STATE", "DRAFT", "AUTHOR", "SOURCE", "TARGET", "UPDATED"})

	for _, row := range rows {
		draft := "no"
		if row.Draft {
			draft = "yes"
		}
		updated := row.UpdatedAt
		if len(updated) >= 10 {
			updated = updated[:10]
		}
		tw.AppendRow(table.Row{
			fmt.Sprintf("!%d", row.IID),
			row.Title,
			row.State,
			draft,
			row.Author,
			row.SourceBranch,
			row.TargetBranch,
			updated,
		})
	}

	tw.Render()

	if paging.totalItems == 0 {
		_, err := fmt.Fprintf(w, "\n%d merge requests (page %d)\n", len(rows), paging.page)
		return err
	}

	_, err := fmt.Fprintf(
		w,
		"\n%d of %d merge requests (page %d of %d)\n",
		len(rows),
		paging.totalItems,
		paging.page,
		paging.totalPages,
	)
	return err
}

func renderDraftNoteTable(w io.Writer, rows []draftNoteOutput, paging mrListPaging) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No pending draft notes. Create one with `mr comment <iid> --draft --body <text>`.")
		return err
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.SetStyle(table.StyleRounded)
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "PREVIEW", WidthMax: 60},
	})
	tw.AppendHeader(table.Row{"ID", "FILE", "LINE", "REPLY", "PREVIEW"})

	for _, row := range rows {
		line := ""
		if row.Line > 0 {
			line = fmt.Sprintf("%d", row.Line)
		}
		tw.AppendRow(table.Row{
			row.ID,
			row.File,
			line,
			row.DiscussionID,
			row.Preview,
		})
	}

	tw.Render()

	_, err := fmt.Fprintf(
		w,
		"\n%d of %d draft notes (page %d of %d)\n",
		len(rows),
		paging.totalItems,
		paging.page,
		paging.totalPages,
	)
	return err
}

func renderDiscussionTable(w io.Writer, rows []discussionRowOutput, paging mrListPaging) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "No discussion threads found. Try --state all, --system, or relax other filters.")
		return err
	}

	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	tw.SetStyle(table.StyleRounded)
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "PREVIEW", WidthMax: 60},
	})
	tw.AppendHeader(table.Row{"ID", "AUTHOR", "STATE", "NOTES", "UPDATED", "PREVIEW"})

	for _, row := range rows {
		updated := row.UpdatedAt
		if len(updated) >= 10 {
			updated = updated[:10]
		}
		tw.AppendRow(table.Row{
			shortDiscussionID(row.ID),
			row.Author,
			row.State,
			row.Notes,
			updated,
			row.Preview,
		})
	}

	tw.Render()

	_, err := fmt.Fprintf(
		w,
		"\n%d of %d discussions (page %d of %d)\n",
		len(rows),
		paging.totalItems,
		paging.page,
		paging.totalPages,
	)
	return err
}
