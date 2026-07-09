package output

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

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

// DiscussionHintContext extends the project-suffix carrying with the filter
// flags of the current invocation, so paging hints re-emit every non-default
// flag and stay runnable as-is (axi guide §9).
type DiscussionHintContext struct {
	MRHintContext
	IID            int64
	State          string
	Author         string
	System         bool
	OrderBy        string
	SortDir        string
	ExcludedSystem int
}

func (c *DiscussionHintContext) filterSuffix() string {
	if c == nil {
		return ""
	}

	var parts []string
	if c.State != DefaultDiscussionStateFilter {
		parts = append(parts, "--state "+c.State)
	}
	if strings.TrimSpace(c.Author) != "" {
		parts = append(parts, "--author "+strings.TrimSpace(c.Author))
	}
	if c.System {
		parts = append(parts, "--system")
	}
	if c.OrderBy != "" && c.OrderBy != DefaultDiscussionOrderBy {
		parts = append(parts, "--order-by "+c.OrderBy)
	}
	if c.SortDir != "" && c.SortDir != DefaultDiscussionSortDirection {
		parts = append(parts, "--sort "+c.SortDir)
	}
	if c.Limit != DefaultMergeRequestListLimit {
		parts = append(parts, fmt.Sprintf("--limit %d", c.Limit))
	}
	if len(parts) == 0 {
		return ""
	}

	return " " + strings.Join(parts, " ")
}

func WriteDiscussionList(w io.Writer, format string, mode Mode, summaries []DiscussionSummary, paging MRListPaging, fields []string, hints *DiscussionHintContext) error {
	if mode == ModeAxi {
		rows := make([]axiDiscussionRow, 0, len(summaries))
		for _, summary := range summaries {
			rows = append(rows, axiDiscussionRowFor(summary, fields))
		}

		return WriteAxi(w, format, axiDiscussionListOutput{
			Discussions: rows,
			Count:       MRListCountLine(len(rows), paging),
			Total:       paging.TotalItems,
			Page:        paging.Page,
			TotalPages:  paging.TotalPages,
			Help:        discussionListHelp(len(rows), paging, hints),
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	rows := make([]discussionRowOutput, 0, len(summaries))
	for _, summary := range summaries {
		rows = append(rows, discussionSummaryToRow(summary))
	}

	if format == "json" {
		return WriteJSON(w, discussionListOutput{
			Discussions: rows,
			Count:       len(rows),
			Total:       paging.TotalItems,
			Page:        paging.Page,
			TotalPages:  paging.TotalPages,
		})
	}

	return renderDiscussionTable(w, rows, paging)
}

func discussionSummaryToRow(summary DiscussionSummary) discussionRowOutput {
	return discussionRowOutput{
		ID:         summary.ID,
		Author:     summary.Author,
		State:      summary.State,
		Resolvable: summary.Resolvable,
		Notes:      summary.NotesCount,
		Type:       summary.NoteType,
		File:       summary.File,
		Line:       summary.Line,
		CreatedAt:  formatLocalTime(summary.CreatedAt),
		UpdatedAt:  formatLocalTime(summary.UpdatedAt),
		Preview:    summary.Preview,
	}
}

func axiDiscussionRowFor(summary DiscussionSummary, fields []string) axiDiscussionRow {
	full := discussionSummaryToRow(summary)
	row := axiDiscussionRow{
		ID:        ShortDiscussionID(full.ID),
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

func discussionListHelp(count int, paging MRListPaging, hints *DiscussionHintContext) []string {
	suffix := hints.filterSuffix() + hints.ProjectSuffix()

	if count == 0 {
		if paging.TotalItems > 0 {
			return []string{fmt.Sprintf(
				"Page %d is past the end (%d matching threads, %d pages) — run `mr discussions %d --page 1%s`",
				paging.Page,
				paging.TotalItems,
				paging.TotalPages,
				hints.IID,
				suffix,
			)}
		}

		help := []string{fmt.Sprintf(
			"No discussion threads matched — run `mr discussions %d --state all%s`, drop --author, or pass --system to include system activity",
			hints.IID,
			hints.ProjectSuffix(),
		)}
		if hints.ExcludedSystem > 0 {
			help = append(help, fmt.Sprintf(
				"%d system discussion(s) were excluded — pass --system to include them",
				hints.ExcludedSystem,
			))
		}

		return help
	}

	help := []string{fmt.Sprintf(
		"Run `%s %d <id>%s` for the full conversation",
		MRDiscussionViewCommandName,
		hints.IID,
		hints.ProjectSuffix(),
	)}
	if paging.TotalPages > paging.Page {
		help = append(help, fmt.Sprintf(
			"Run `mr discussions %d --page %d%s` for the next page",
			hints.IID,
			paging.Page+1,
			suffix,
		))
	}

	return help
}

func WriteDiscussion(w io.Writer, format string, mode Mode, discussion *gitlab.Discussion) error {
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

	if mode == ModeAxi {
		return WriteAxi(w, format, out)
	}

	format, err = NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, out)
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
	if summary, ok := SummarizeDiscussion(discussion); ok {
		detail.State = summary.State
		detail.Resolvable = summary.Resolvable
		detail.File = summary.File
		detail.Line = summary.Line
		detail.UpdatedAt = formatLocalTime(summary.UpdatedAt)
		if summary.Resolved {
			detail.ResolvedBy = summary.ResolvedBy
			detail.ResolvedAt = formatTimeValue(summary.ResolvedAt)
		}
	}

	return detail, nil
}

func WriteDiscussionAction(w io.Writer, format string, mode Mode, discussion *gitlab.Discussion, action string, noop bool, iid int64, hints *MRHintContext) error {
	detail, err := discussionDetailFromDiscussion(discussion)
	if err != nil {
		return err
	}

	out := discussionActionOutput{
		Discussion: detail,
		Action:     action,
		Noop:       noop,
	}

	if mode == ModeAxi {
		out.Discussion.ID = ShortDiscussionID(out.Discussion.ID)
		out.Help = discussionActionHelp(action, iid, out.Discussion.ID, hints)
		return WriteAxi(w, format, out)
	}

	format, err = NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, out)
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

func discussionActionHelp(action string, iid int64, discussionID string, hints *MRHintContext) []string {
	suffix := hints.ProjectSuffix()
	viewHint := fmt.Sprintf("Run `%s %d %s%s` for the full thread", MRDiscussionViewCommandName, iid, ShortDiscussionID(discussionID), suffix)

	if action == "resolve" {
		return []string{
			viewHint,
			fmt.Sprintf("Run `mr discussion unresolve %d %s%s` to reopen the thread", iid, ShortDiscussionID(discussionID), suffix),
		}
	}

	return []string{
		viewHint,
		fmt.Sprintf("Run `mr discussion resolve %d %s%s` to resolve the thread", iid, ShortDiscussionID(discussionID), suffix),
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

const (
	DefaultDiscussionStateFilter   = "unresolved"
	DefaultDiscussionOrderBy       = "created_at"
	DefaultDiscussionSortDirection = "asc"
	DiscussionPreviewLimit         = 80
	discussionShortIDLength        = 8
	// MRDiscussionViewCommandName is how help hints reference the
	// single-thread view command; change here if it is ever renamed.
	MRDiscussionViewCommandName = "mr discussion"
)

// DiscussionSummary is the computed per-thread view the list pipeline works
// on: resolution state, last activity, and the compact row fields.
type DiscussionSummary struct {
	ID         string
	Author     string
	State      string
	Preview    string
	NoteType   string
	File       string
	Line       int64
	Resolvable bool
	Resolved   bool
	System     bool
	NotesCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ResolvedBy string
	ResolvedAt *time.Time
}

// SummarizeDiscussion computes the thread-level summary. ok is false for nil
// discussions and discussions without notes, which carry nothing to show.
func SummarizeDiscussion(discussion *gitlab.Discussion) (DiscussionSummary, bool) {
	if discussion == nil {
		return DiscussionSummary{}, false
	}

	notes := make([]*gitlab.Note, 0, len(discussion.Notes))
	for _, note := range discussion.Notes {
		if note != nil {
			notes = append(notes, note)
		}
	}
	if len(notes) == 0 {
		return DiscussionSummary{}, false
	}

	summary := DiscussionSummary{
		ID:         strings.ToLower(discussion.ID),
		NotesCount: len(notes),
		System:     true,
	}

	first := notes[0]
	summary.Author = first.Author.Username
	summary.Preview = DiscussionPreview(first.Body)
	summary.NoteType = string(first.Type)
	if summary.NoteType == "" {
		summary.NoteType = string(gitlab.GenericNote)
	}
	if first.CreatedAt != nil {
		summary.CreatedAt = *first.CreatedAt
	}
	if position := first.Position; position != nil {
		summary.File = position.NewPath
		summary.Line = position.NewLine
		if summary.File == "" {
			summary.File = position.OldPath
		}
		if summary.Line == 0 {
			summary.Line = position.OldLine
		}
	}

	resolvedAll := true
	for _, note := range notes {
		if !note.System {
			summary.System = false
		}

		updated := note.UpdatedAt
		if updated == nil {
			updated = note.CreatedAt
		}
		if updated != nil && updated.After(summary.UpdatedAt) {
			summary.UpdatedAt = *updated
		}

		if note.Resolvable {
			summary.Resolvable = true
			if note.Resolved {
				if note.ResolvedBy.Username != "" {
					summary.ResolvedBy = note.ResolvedBy.Username
				}
				if note.ResolvedAt != nil {
					summary.ResolvedAt = note.ResolvedAt
				}
			} else {
				resolvedAll = false
			}
		}
	}
	summary.Resolved = summary.Resolvable && resolvedAll

	switch {
	case !summary.Resolvable:
		summary.State = "none"
	case summary.Resolved:
		summary.State = "resolved"
	default:
		summary.State = "unresolved"
	}

	return summary, true
}

// DiscussionPreview flattens a note body to one line and truncates it at
// DiscussionPreviewLimit runes with an explicit ellipsis.
func DiscussionPreview(body string) string {
	flattened := strings.Join(strings.Fields(body), " ")

	runes := []rune(flattened)
	if len(runes) <= DiscussionPreviewLimit {
		return flattened
	}

	return string(runes[:DiscussionPreviewLimit]) + "…"
}

func ShortDiscussionID(id string) string {
	id = strings.ToLower(id)
	if len(id) <= discussionShortIDLength {
		return id
	}

	return id[:discussionShortIDLength]
}
