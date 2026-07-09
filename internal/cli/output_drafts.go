package cli

import (
	"errors"
	"fmt"
	"io"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// draftNoteOutput is built around GitLab's thin draft-note response, which
// carries no author name or timestamps. DiscussionID is set only for drafts
// replying to an existing thread.
type draftNoteOutput struct {
	ID                int64  `json:"id" toon:"id"`
	Preview           string `json:"preview" toon:"preview"`
	File              string `json:"file,omitempty" toon:"file,omitempty"`
	Line              int64  `json:"line,omitempty" toon:"line,omitempty"`
	DiscussionID      string `json:"discussion_id,omitempty" toon:"discussion_id,omitempty"`
	ResolveDiscussion bool   `json:"resolve_discussion,omitempty" toon:"resolve_discussion,omitempty"`
}

type axiDraftNoteCreatedOutput struct {
	DraftNote draftNoteOutput `json:"draft_note" toon:"draft_note"`
	Help      []string        `json:"help,omitempty" toon:"help,omitempty"`
}

// axiDraftNoteRow is the compact axi drafts list row. Optional fields are
// pointers with omitempty so --fields controls the emitted columns while
// rows stay uniform (required for TOON tabular output).
type axiDraftNoteRow struct {
	ID                int64   `json:"id" toon:"id"`
	File              string  `json:"file" toon:"file"`
	Line              int64   `json:"line" toon:"line"`
	Preview           string  `json:"preview" toon:"preview"`
	DiscussionID      *string `json:"discussion_id,omitempty" toon:"discussion_id,omitempty"`
	ResolveDiscussion *bool   `json:"resolve_discussion,omitempty" toon:"resolve_discussion,omitempty"`
}

type axiDraftNoteListOutput struct {
	DraftNotes []axiDraftNoteRow `json:"draft_notes" toon:"draft_notes"`
	Count      string            `json:"count" toon:"count"`
	Total      int64             `json:"total" toon:"-"`
	Page       int64             `json:"page" toon:"-"`
	TotalPages int64             `json:"total_pages" toon:"-"`
	Help       []string          `json:"help,omitempty" toon:"help,omitempty"`
}

type draftNoteListOutput struct {
	DraftNotes []draftNoteOutput `json:"draft_notes"`
	Count      int               `json:"count"`
	Total      int64             `json:"total"`
	Page       int64             `json:"page"`
	TotalPages int64             `json:"total_pages"`
}

func draftNoteToOutput(draft *gitlab.DraftNote) draftNoteOutput {
	out := draftNoteOutput{
		ID:                draft.ID,
		Preview:           discussionPreview(draft.Note),
		DiscussionID:      shortDiscussionID(draft.DiscussionID),
		ResolveDiscussion: draft.ResolveDiscussion,
	}
	if position := draft.Position; position != nil {
		out.File = position.NewPath
		out.Line = position.NewLine
		if out.File == "" {
			out.File = position.OldPath
		}
		if out.Line == 0 {
			out.Line = position.OldLine
		}
	}

	return out
}

func writeDraftNoteCreated(w io.Writer, format string, mode commandMode, draft *gitlab.DraftNote, iid int64, positionRequested bool, hints *mrHintContext) error {
	if draft == nil {
		return errors.New("gitlab api returned an empty draft note response")
	}

	out := draftNoteToOutput(draft)

	if mode == commandModeAxi {
		suffix := hints.projectSuffix()
		help := []string{
			fmt.Sprintf("Run `mr drafts publish %d %d%s` to publish it, or `mr drafts publish %d --all%s` for the whole pending review", iid, out.ID, suffix, iid, suffix),
			fmt.Sprintf("Run `mr drafts %d%s` to list pending drafts", iid, suffix),
		}
		if positionRequested && draft.Position == nil {
			help = append(help, fmt.Sprintf(
				"GitLab did not anchor the draft to the requested diff position — run `mr drafts %d%s` to verify",
				iid,
				suffix,
			))
		}

		return writeAxi(w, format, axiDraftNoteCreatedOutput{DraftNote: out, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeDraftNoteText(w, out)
}

func writeDraftNoteText(w io.Writer, out draftNoteOutput) error {
	if _, err := fmt.Fprintf(w, "draft_note: %d\npreview: %s\n", out.ID, out.Preview); err != nil {
		return err
	}
	if out.File != "" {
		location := out.File
		if out.Line > 0 {
			location = fmt.Sprintf("%s:%d", out.File, out.Line)
		}
		if _, err := fmt.Fprintf(w, "file: %s\n", location); err != nil {
			return err
		}
	}
	if out.DiscussionID != "" {
		if _, err := fmt.Fprintf(w, "discussion: %s\n", out.DiscussionID); err != nil {
			return err
		}
	}
	if out.ResolveDiscussion {
		if _, err := fmt.Fprintln(w, "resolve_discussion: true"); err != nil {
			return err
		}
	}

	return nil
}

func axiDraftNoteRowFor(out draftNoteOutput, fields []string) axiDraftNoteRow {
	row := axiDraftNoteRow{
		ID:      out.ID,
		File:    out.File,
		Line:    out.Line,
		Preview: out.Preview,
	}

	for _, field := range fields {
		switch field {
		case "discussion_id":
			row.DiscussionID = &out.DiscussionID
		case "resolve_discussion":
			row.ResolveDiscussion = &out.ResolveDiscussion
		}
	}

	return row
}

func writeDraftNoteList(w io.Writer, format string, mode commandMode, drafts []draftNoteOutput, paging mrListPaging, fields []string, iid int64, hints *mrHintContext) error {
	if mode == commandModeAxi {
		rows := make([]axiDraftNoteRow, 0, len(drafts))
		for _, draft := range drafts {
			rows = append(rows, axiDraftNoteRowFor(draft, fields))
		}

		return writeAxi(w, format, axiDraftNoteListOutput{
			DraftNotes: rows,
			Count:      mrListCountLine(len(rows), paging),
			Total:      paging.totalItems,
			Page:       paging.page,
			TotalPages: paging.totalPages,
			Help:       draftNoteListHelp(len(rows), paging, iid, hints),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, draftNoteListOutput{
			DraftNotes: drafts,
			Count:      len(drafts),
			Total:      paging.totalItems,
			Page:       paging.page,
			TotalPages: paging.totalPages,
		})
	}

	return renderDraftNoteTable(w, drafts, paging)
}

func draftNoteListHelp(count int, paging mrListPaging, iid int64, hints *mrHintContext) []string {
	suffix := hints.projectSuffix()

	if count == 0 {
		if paging.totalItems > 0 {
			return []string{fmt.Sprintf(
				"Page %d is past the end (%d pending drafts, %d pages) — run `mr drafts %d --page 1%s`",
				paging.page,
				paging.totalItems,
				paging.totalPages,
				iid,
				suffix,
			)}
		}

		return []string{fmt.Sprintf(
			"No pending draft notes — create one with `mr comment %d --draft --body <text>%s`",
			iid,
			suffix,
		)}
	}

	help := []string{fmt.Sprintf(
		"Run `mr drafts publish %d --all%s` to publish the pending review, or `mr drafts publish %d <id>%s` for a single draft",
		iid,
		suffix,
		iid,
		suffix,
	)}
	if paging.totalPages > paging.page {
		help = append(help, fmt.Sprintf(
			"Run `mr drafts %d --page %d%s` for the next page",
			iid,
			paging.page+1,
			suffix,
		))
	}

	return help
}

type draftPublishResult struct {
	ID    *int64 `json:"id,omitempty" toon:"id,omitempty"`
	All   bool   `json:"all,omitempty" toon:"all,omitempty"`
	Count int    `json:"count" toon:"count"`
	Noop  bool   `json:"noop,omitempty" toon:"noop,omitempty"`
}

type axiDraftPublishOutput struct {
	Published draftPublishResult `json:"published" toon:"published"`
	Help      []string           `json:"help,omitempty" toon:"help,omitempty"`
}

func writeDraftNotesPublished(w io.Writer, format string, mode commandMode, result draftPublishResult, iid int64, hints *mrHintContext) error {
	if mode == commandModeAxi {
		var help []string
		if result.Noop {
			help = append(help, fmt.Sprintf(
				"Nothing was pending — create draft notes with `mr comment %d --draft --body <text>%s`",
				iid,
				hints.projectSuffix(),
			))
		} else {
			help = append(help, fmt.Sprintf(
				"Run `mr discussions %d%s` to see the published threads",
				iid,
				hints.projectSuffix(),
			))
		}

		return writeAxi(w, format, axiDraftPublishOutput{Published: result, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
	}

	switch {
	case result.Noop:
		_, err = fmt.Fprintln(w, "no pending draft notes to publish (no-op)")
	case result.All:
		_, err = fmt.Fprintf(w, "published: %d draft note(s)\n", result.Count)
	default:
		_, err = fmt.Fprintf(w, "published: draft note %d\n", *result.ID)
	}

	return err
}

type draftDeleteResult struct {
	ID   int64 `json:"id" toon:"id"`
	Noop bool  `json:"noop,omitempty" toon:"noop,omitempty"`
}

type axiDraftDeleteOutput struct {
	Deleted draftDeleteResult `json:"deleted" toon:"deleted"`
	Help    []string          `json:"help,omitempty" toon:"help,omitempty"`
}

func writeDraftNoteDeleted(w io.Writer, format string, mode commandMode, result draftDeleteResult, iid int64, hints *mrHintContext) error {
	if mode == commandModeAxi {
		help := []string{fmt.Sprintf(
			"Run `mr drafts %d%s` to list the remaining drafts",
			iid,
			hints.projectSuffix(),
		)}

		return writeAxi(w, format, axiDraftDeleteOutput{Deleted: result, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
	}

	if result.Noop {
		_, err = fmt.Fprintf(w, "draft note %d already absent (no-op)\n", result.ID)
	} else {
		_, err = fmt.Fprintf(w, "deleted: draft note %d\n", result.ID)
	}

	return err
}
