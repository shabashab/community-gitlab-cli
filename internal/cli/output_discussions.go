package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// axiDiscussionRow is the compact axi discussion list row. Optional fields
// are pointers with omitempty so --fields controls exactly which columns are
// emitted while every row stays uniform (required for TOON tabular output).
type axiDiscussionRow struct {
	ID        string  `json:"id" toon:"id"`
	Author    string  `json:"author" toon:"author"`
	State     string  `json:"state" toon:"state"`
	Notes     int     `json:"notes" toon:"notes"`
	UpdatedAt string  `json:"updated_at" toon:"updated_at"`
	Preview   string  `json:"preview" toon:"preview"`
	Type      *string `json:"type,omitempty" toon:"type,omitempty"`
	File      *string `json:"file,omitempty" toon:"file,omitempty"`
	Line      *int64  `json:"line,omitempty" toon:"line,omitempty"`
	CreatedAt *string `json:"created_at,omitempty" toon:"created_at,omitempty"`
	IDFull    *string `json:"id_full,omitempty" toon:"id_full,omitempty"`
}

type axiDiscussionListOutput struct {
	Discussions []axiDiscussionRow `json:"discussions" toon:"discussions"`
	Count       string             `json:"count" toon:"count"`
	Total       int64              `json:"total" toon:"-"`
	Page        int64              `json:"page" toon:"-"`
	TotalPages  int64              `json:"total_pages" toon:"-"`
	Help        []string           `json:"help,omitempty" toon:"help,omitempty"`
}

// discussionRowOutput is the standard-mode row (gl json and table source);
// the id is the full 40-character discussion ID.
type discussionRowOutput struct {
	ID         string `json:"id"`
	Author     string `json:"author"`
	State      string `json:"state"`
	Resolvable bool   `json:"resolvable"`
	Notes      int    `json:"notes"`
	Type       string `json:"type"`
	File       string `json:"file,omitempty"`
	Line       int64  `json:"line,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Preview    string `json:"preview"`
}

type discussionListOutput struct {
	Discussions []discussionRowOutput `json:"discussions"`
	Count       int                   `json:"count"`
	Total       int64                 `json:"total"`
	Page        int64                 `json:"page"`
	TotalPages  int64                 `json:"total_pages"`
}

type discussionDetailOutput struct {
	ID         string `json:"id" toon:"id"`
	State      string `json:"state" toon:"state"`
	Resolvable bool   `json:"resolvable" toon:"resolvable"`
	File       string `json:"file,omitempty" toon:"file,omitempty"`
	Line       int64  `json:"line,omitempty" toon:"line,omitempty"`
	ResolvedBy string `json:"resolved_by,omitempty" toon:"resolved_by,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty" toon:"resolved_at,omitempty"`
	UpdatedAt  string `json:"updated_at" toon:"updated_at"`
	Notes      int    `json:"notes" toon:"notes"`
}

// discussionNoteOutput fields are all non-optional so notes[] stays a uniform
// TOON tabular array. The body is complete — showing full conversations is
// the thread view's purpose.
type discussionNoteOutput struct {
	ID        int64  `json:"id" toon:"id"`
	Author    string `json:"author" toon:"author"`
	CreatedAt string `json:"created_at" toon:"created_at"`
	UpdatedAt string `json:"updated_at" toon:"updated_at"`
	System    bool   `json:"system" toon:"system"`
	Body      string `json:"body" toon:"body"`
}

// discussionViewOutput carries no help field: a thread view is self-contained
// (axi guide §9).
type discussionViewOutput struct {
	Discussion discussionDetailOutput `json:"discussion" toon:"discussion"`
	Notes      []discussionNoteOutput `json:"notes" toon:"notes"`
}

type discussionActionOutput struct {
	Discussion discussionDetailOutput `json:"discussion" toon:"discussion"`
	Action     string                 `json:"action" toon:"action"`
	Noop       bool                   `json:"noop,omitempty" toon:"noop,omitempty"`
	Help       []string               `json:"help,omitempty" toon:"help,omitempty"`
}

// discussionHintContext extends the project-suffix carrying with the filter
// flags of the current invocation, so paging hints re-emit every non-default
// flag and stay runnable as-is (axi guide §9).
type discussionHintContext struct {
	mrHintContext
	iid            int64
	state          string
	author         string
	system         bool
	orderBy        string
	sortDir        string
	excludedSystem int
}

func (c *discussionHintContext) filterSuffix() string {
	if c == nil {
		return ""
	}

	var parts []string
	if c.state != defaultDiscussionStateFilter {
		parts = append(parts, "--state "+c.state)
	}
	if strings.TrimSpace(c.author) != "" {
		parts = append(parts, "--author "+strings.TrimSpace(c.author))
	}
	if c.system {
		parts = append(parts, "--system")
	}
	if c.orderBy != "" && c.orderBy != defaultDiscussionOrderBy {
		parts = append(parts, "--order-by "+c.orderBy)
	}
	if c.sortDir != "" && c.sortDir != defaultDiscussionSortDirection {
		parts = append(parts, "--sort "+c.sortDir)
	}
	if c.limit != defaultMergeRequestListLimit {
		parts = append(parts, fmt.Sprintf("--limit %d", c.limit))
	}
	if len(parts) == 0 {
		return ""
	}

	return " " + strings.Join(parts, " ")
}

func writeDiscussionList(w io.Writer, format string, mode commandMode, summaries []discussionSummary, paging mrListPaging, fields []string, hints *discussionHintContext) error {
	if mode == commandModeAxi {
		rows := make([]axiDiscussionRow, 0, len(summaries))
		for _, summary := range summaries {
			rows = append(rows, axiDiscussionRowFor(summary, fields))
		}

		return writeAxi(w, format, axiDiscussionListOutput{
			Discussions: rows,
			Count:       mrListCountLine(len(rows), paging),
			Total:       paging.totalItems,
			Page:        paging.page,
			TotalPages:  paging.totalPages,
			Help:        discussionListHelp(len(rows), paging, hints),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	rows := make([]discussionRowOutput, 0, len(summaries))
	for _, summary := range summaries {
		rows = append(rows, discussionSummaryToRow(summary))
	}

	if format == "json" {
		return writeJSON(w, discussionListOutput{
			Discussions: rows,
			Count:       len(rows),
			Total:       paging.totalItems,
			Page:        paging.page,
			TotalPages:  paging.totalPages,
		})
	}

	return renderDiscussionTable(w, rows, paging)
}

func discussionSummaryToRow(summary discussionSummary) discussionRowOutput {
	return discussionRowOutput{
		ID:         summary.id,
		Author:     summary.author,
		State:      summary.state,
		Resolvable: summary.resolvable,
		Notes:      summary.notesCount,
		Type:       summary.noteType,
		File:       summary.file,
		Line:       summary.line,
		CreatedAt:  formatLocalTime(summary.createdAt),
		UpdatedAt:  formatLocalTime(summary.updatedAt),
		Preview:    summary.preview,
	}
}

func axiDiscussionRowFor(summary discussionSummary, fields []string) axiDiscussionRow {
	full := discussionSummaryToRow(summary)
	row := axiDiscussionRow{
		ID:        shortDiscussionID(full.ID),
		Author:    full.Author,
		State:     full.State,
		Notes:     full.Notes,
		UpdatedAt: full.UpdatedAt,
		Preview:   full.Preview,
	}

	for _, field := range fields {
		switch field {
		case "type":
			row.Type = &full.Type
		case "file":
			row.File = &full.File
		case "line":
			row.Line = &full.Line
		case "created_at":
			row.CreatedAt = &full.CreatedAt
		case "id_full":
			row.IDFull = &full.ID
		}
	}

	return row
}

func discussionListHelp(count int, paging mrListPaging, hints *discussionHintContext) []string {
	suffix := hints.filterSuffix() + hints.projectSuffix()

	if count == 0 {
		if paging.totalItems > 0 {
			return []string{fmt.Sprintf(
				"Page %d is past the end (%d matching threads, %d pages) — run `mr discussions %d --page 1%s`",
				paging.page,
				paging.totalItems,
				paging.totalPages,
				hints.iid,
				suffix,
			)}
		}

		help := []string{fmt.Sprintf(
			"No discussion threads matched — run `mr discussions %d --state all%s`, drop --author, or pass --system to include system activity",
			hints.iid,
			hints.projectSuffix(),
		)}
		if hints.excludedSystem > 0 {
			help = append(help, fmt.Sprintf(
				"%d system discussion(s) were excluded — pass --system to include them",
				hints.excludedSystem,
			))
		}

		return help
	}

	help := []string{fmt.Sprintf(
		"Run `%s %d <id>%s` for the full conversation",
		mrDiscussionViewCommandName,
		hints.iid,
		hints.projectSuffix(),
	)}
	if paging.totalPages > paging.page {
		help = append(help, fmt.Sprintf(
			"Run `mr discussions %d --page %d%s` for the next page",
			hints.iid,
			paging.page+1,
			suffix,
		))
	}

	return help
}

func writeDiscussion(w io.Writer, format string, mode commandMode, discussion *gitlab.Discussion) error {
	detail, err := discussionDetailFromDiscussion(discussion)
	if err != nil {
		return err
	}

	notes := make([]discussionNoteOutput, 0, len(discussion.Notes))
	for _, note := range discussion.Notes {
		if note == nil {
			continue
		}
		notes = append(notes, discussionNoteOutput{
			ID:        note.ID,
			Author:    note.Author.Username,
			CreatedAt: formatTimeValue(note.CreatedAt),
			UpdatedAt: formatTimeValue(note.UpdatedAt),
			System:    note.System,
			Body:      note.Body,
		})
	}

	out := discussionViewOutput{Discussion: detail, Notes: notes}

	if mode == commandModeAxi {
		return writeAxi(w, format, out)
	}

	format, err = normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeDiscussionText(w, out)
}

func discussionDetailFromDiscussion(discussion *gitlab.Discussion) (discussionDetailOutput, error) {
	if discussion == nil {
		return discussionDetailOutput{}, errors.New("gitlab api returned an empty discussion response")
	}

	notes := 0
	for _, note := range discussion.Notes {
		if note != nil {
			notes++
		}
	}

	detail := discussionDetailOutput{
		ID:    strings.ToLower(discussion.ID),
		State: "none",
		Notes: notes,
	}
	if summary, ok := summarizeDiscussion(discussion); ok {
		detail.State = summary.state
		detail.Resolvable = summary.resolvable
		detail.File = summary.file
		detail.Line = summary.line
		detail.UpdatedAt = formatLocalTime(summary.updatedAt)
		if summary.resolved {
			detail.ResolvedBy = summary.resolvedBy
			detail.ResolvedAt = formatTimeValue(summary.resolvedAt)
		}
	}

	return detail, nil
}

func writeDiscussionAction(w io.Writer, format string, mode commandMode, discussion *gitlab.Discussion, action string, noop bool, iid int64, hints *mrHintContext) error {
	detail, err := discussionDetailFromDiscussion(discussion)
	if err != nil {
		return err
	}

	out := discussionActionOutput{
		Discussion: detail,
		Action:     action,
		Noop:       noop,
	}

	if mode == commandModeAxi {
		out.Discussion.ID = shortDiscussionID(out.Discussion.ID)
		out.Help = discussionActionHelp(action, iid, out.Discussion.ID, hints)
		return writeAxi(w, format, out)
	}

	format, err = normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	if noop {
		if _, err := fmt.Fprintf(w, "discussion %s already %s (no-op)\n", detail.ID, discussionActionDoneState(action)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%s: discussion %s\n", discussionActionPastTense(action), detail.ID); err != nil {
			return err
		}
	}

	return writeDiscussionDetailText(w, detail, true)
}

func discussionActionHelp(action string, iid int64, discussionID string, hints *mrHintContext) []string {
	suffix := hints.projectSuffix()
	viewHint := fmt.Sprintf("Run `%s %d %s%s` for the full thread", mrDiscussionViewCommandName, iid, shortDiscussionID(discussionID), suffix)

	if action == "resolve" {
		return []string{
			viewHint,
			fmt.Sprintf("Run `mr discussion unresolve %d %s%s` to reopen the thread", iid, shortDiscussionID(discussionID), suffix),
		}
	}

	return []string{
		viewHint,
		fmt.Sprintf("Run `mr discussion resolve %d %s%s` to resolve the thread", iid, shortDiscussionID(discussionID), suffix),
	}
}

func discussionActionPastTense(action string) string {
	if action == "resolve" {
		return "resolved"
	}

	return "unresolved"
}

func discussionActionDoneState(action string) string {
	if action == "resolve" {
		return "resolved"
	}

	return "unresolved"
}

func writeDiscussionText(w io.Writer, out discussionViewOutput) error {
	if err := writeDiscussionDetailText(w, out.Discussion, false); err != nil {
		return err
	}

	for i, note := range out.Notes {
		header := fmt.Sprintf("[%d] %s — %s", i+1, note.Author, note.CreatedAt)
		if note.UpdatedAt != "" && note.UpdatedAt != note.CreatedAt {
			header += fmt.Sprintf(" (edited %s)", note.UpdatedAt)
		}
		if note.System {
			header += " [system]"
		}
		if _, err := fmt.Fprintf(w, "\n%s\n%s\n", header, note.Body); err != nil {
			return err
		}
	}

	return nil
}

func writeDiscussionDetailText(w io.Writer, detail discussionDetailOutput, includeResolvable bool) error {
	if _, err := fmt.Fprintf(w, "discussion: %s\nstate: %s\n", detail.ID, detail.State); err != nil {
		return err
	}
	if includeResolvable {
		if _, err := fmt.Fprintf(w, "resolvable: %t\n", detail.Resolvable); err != nil {
			return err
		}
	}
	if detail.File != "" {
		if _, err := fmt.Fprintf(w, "file: %s:%d\n", detail.File, detail.Line); err != nil {
			return err
		}
	}
	if detail.ResolvedBy != "" || detail.ResolvedAt != "" {
		if _, err := fmt.Fprintf(w, "resolved_by: %s\nresolved_at: %s\n", detail.ResolvedBy, detail.ResolvedAt); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "updated_at: %s\nnotes: %d\n", detail.UpdatedAt, detail.Notes)
	return err
}
