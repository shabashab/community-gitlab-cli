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
