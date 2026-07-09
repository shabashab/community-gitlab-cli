package output

import (
	"errors"
	"fmt"
	"io"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mergeRequestRowOutput struct {
	IID          int64  `json:"iid" toon:"iid"`
	Title        string `json:"title" toon:"title"`
	State        string `json:"state" toon:"state"`
	Draft        bool   `json:"draft" toon:"draft"`
	Author       string `json:"author" toon:"author"`
	SourceBranch string `json:"source_branch" toon:"source_branch"`
	TargetBranch string `json:"target_branch" toon:"target_branch"`
	UpdatedAt    string `json:"updated_at" toon:"updated_at"`
	WebURL       string `json:"web_url" toon:"web_url"`
}

// AxiMergeRequestRow is the compact axi list row. Optional fields are
// pointers with omitempty so --fields controls exactly which columns are
// emitted while every row stays uniform (required for TOON tabular output).
type AxiMergeRequestRow struct {
	IID          int64   `json:"iid" toon:"iid"`
	Title        string  `json:"title" toon:"title"`
	State        string  `json:"state" toon:"state"`
	Draft        *bool   `json:"draft,omitempty" toon:"draft,omitempty"`
	Author       string  `json:"author" toon:"author"`
	SourceBranch *string `json:"source_branch,omitempty" toon:"source_branch,omitempty"`
	TargetBranch *string `json:"target_branch,omitempty" toon:"target_branch,omitempty"`
	UpdatedAt    *string `json:"updated_at,omitempty" toon:"updated_at,omitempty"`
	WebURL       *string `json:"web_url,omitempty" toon:"web_url,omitempty"`
}

type mergeRequestOutput struct {
	IID                         int64                              `json:"iid" toon:"iid"`
	Title                       string                             `json:"title" toon:"title"`
	State                       string                             `json:"state" toon:"state"`
	Draft                       bool                               `json:"draft" toon:"draft"`
	Author                      string                             `json:"author" toon:"author"`
	Assignees                   []string                           `json:"assignees" toon:"assignees"`
	Reviewers                   []string                           `json:"reviewers" toon:"reviewers"`
	SourceBranch                string                             `json:"source_branch" toon:"source_branch"`
	TargetBranch                string                             `json:"target_branch" toon:"target_branch"`
	Labels                      []string                           `json:"labels" toon:"labels"`
	Milestone                   string                             `json:"milestone" toon:"milestone"`
	DetailedMergeStatus         string                             `json:"detailed_merge_status" toon:"detailed_merge_status"`
	HasConflicts                bool                               `json:"has_conflicts" toon:"has_conflicts"`
	BlockingDiscussionsResolved bool                               `json:"blocking_discussions_resolved" toon:"blocking_discussions_resolved"`
	UserNotesCount              int64                              `json:"user_notes_count" toon:"user_notes_count"`
	ChangesCount                string                             `json:"changes_count" toon:"changes_count"`
	PipelineStatus              string                             `json:"pipeline_status" toon:"pipeline_status"`
	SHA                         string                             `json:"sha" toon:"sha"`
	CreatedAt                   string                             `json:"created_at" toon:"created_at"`
	UpdatedAt                   string                             `json:"updated_at" toon:"updated_at"`
	MergedAt                    string                             `json:"merged_at" toon:"merged_at"`
	ClosedAt                    string                             `json:"closed_at" toon:"closed_at"`
	WebURL                      string                             `json:"web_url" toon:"web_url"`
	Approval                    *mergeRequestApprovalCompactOutput `json:"approval,omitempty" toon:"approval,omitempty"`
	Description                 string                             `json:"description" toon:"description"`
}

// axiMergeRequestCompact is the token-frugal axi detail view.
type axiMergeRequestCompact struct {
	IID                 int64                              `json:"iid" toon:"iid"`
	Title               string                             `json:"title" toon:"title"`
	State               string                             `json:"state" toon:"state"`
	Draft               bool                               `json:"draft" toon:"draft"`
	Author              string                             `json:"author" toon:"author"`
	SourceBranch        string                             `json:"source_branch" toon:"source_branch"`
	TargetBranch        string                             `json:"target_branch" toon:"target_branch"`
	DetailedMergeStatus string                             `json:"detailed_merge_status" toon:"detailed_merge_status"`
	HasConflicts        bool                               `json:"has_conflicts" toon:"has_conflicts"`
	PipelineStatus      string                             `json:"pipeline_status" toon:"pipeline_status"`
	UserNotesCount      int64                              `json:"user_notes_count" toon:"user_notes_count"`
	UpdatedAt           string                             `json:"updated_at" toon:"updated_at"`
	WebURL              string                             `json:"web_url" toon:"web_url"`
	Approval            *mergeRequestApprovalCompactOutput `json:"approval,omitempty" toon:"approval,omitempty"`
	Description         string                             `json:"description" toon:"description"`
}

type axiMergeRequestViewOutput struct {
	MergeRequest any      `json:"merge_request" toon:"merge_request"`
	Help         []string `json:"help,omitempty" toon:"help,omitempty"`
}

type mergeRequestActionOutput struct {
	MergeRequest any      `json:"merge_request" toon:"merge_request"`
	Action       string   `json:"action" toon:"action"`
	Noop         bool     `json:"noop,omitempty" toon:"noop,omitempty"`
	Help         []string `json:"help,omitempty" toon:"help,omitempty"`
}

type mergeRequestListOutput struct {
	MergeRequests []mergeRequestRowOutput `json:"merge_requests" toon:"merge_requests"`
	Count         int                     `json:"count" toon:"-"`
	Total         int64                   `json:"total" toon:"-"`
	Page          int64                   `json:"page" toon:"-"`
	TotalPages    int64                   `json:"total_pages" toon:"-"`
}

type axiMergeRequestListOutput struct {
	MergeRequests []AxiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Total         int64                `json:"total" toon:"-"`
	Page          int64                `json:"page" toon:"-"`
	TotalPages    int64                `json:"total_pages" toon:"-"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

func WriteMergeRequest(w io.Writer, format string, mode Mode, mergeRequest *gitlab.MergeRequest, full bool, hints *MRHintContext) error {
	return WriteMergeRequestWithApprovals(w, format, mode, mergeRequest, nil, full, hints)
}

func WriteMergeRequestWithApprovals(w io.Writer, format string, mode Mode, mergeRequest *gitlab.MergeRequest, approvals *gitlab.MergeRequestApprovals, full bool, hints *MRHintContext) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, truncated := mergeRequestToOutput(mergeRequest, full, mode)
	if approvals != nil {
		approval := mergeRequestApprovalCompactFromAPI(approvals)
		approval.IID = 0
		out.Approval = &approval
	}

	if mode == ModeAxi {
		var view any = out
		if !full {
			view = compactMergeRequestView(out)
		}

		// The escape-hatch hint appears only when content was actually
		// truncated; otherwise a detail view is self-contained.
		var help []string
		if truncated {
			help = []string{fmt.Sprintf(
				"Run `mr view %d --full%s` for the complete description and all fields",
				out.IID,
				hints.ProjectSuffix(),
			)}
		}

		return WriteAxi(w, format, axiMergeRequestViewOutput{MergeRequest: view, Help: help})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, out)
	}

	return writeMergeRequestText(w, out, full)
}

// WriteMergeRequestCreated renders a freshly created or updated merge
// request. It reuses the view's merge_request shape so agents parse one
// schema, but unlike the self-contained view a mutation has a genuine next
// step, so the axi variant always suggests checking merge status.
func WriteMergeRequestCreated(w io.Writer, format string, mode Mode, mergeRequest *gitlab.MergeRequest, hints *MRHintContext) error {
	if mode != ModeAxi {
		return WriteMergeRequest(w, format, mode, mergeRequest, false, hints)
	}

	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, truncated := mergeRequestToOutput(mergeRequest, false, mode)

	help := []string{fmt.Sprintf(
		"Run `mr view %d%s` to check merge status and pipeline results",
		out.IID,
		hints.ProjectSuffix(),
	)}
	if truncated {
		help = append(help, fmt.Sprintf(
			"Run `mr view %d --full%s` for the complete description and all fields",
			out.IID,
			hints.ProjectSuffix(),
		))
	}

	return WriteAxi(w, format, axiMergeRequestViewOutput{
		MergeRequest: compactMergeRequestView(out),
		Help:         help,
	})
}

func WriteMergeRequestAction(w io.Writer, format string, mode Mode, mergeRequest *gitlab.MergeRequest, action string, noop bool, hints *MRHintContext) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, _ := mergeRequestToOutput(mergeRequest, false, mode)
	view := compactMergeRequestView(out)

	if mode == ModeAxi {
		return WriteAxi(w, format, mergeRequestActionOutput{
			MergeRequest: view,
			Action:       action,
			Noop:         noop,
			Help:         mergeRequestActionHelp(action, out.IID, hints),
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, mergeRequestActionOutput{
			MergeRequest: view,
			Action:       action,
			Noop:         noop,
		})
	}

	if noop {
		if _, err := fmt.Fprintf(w, "merge request !%d already %s (no-op)\n", out.IID, mergeRequestActionDoneState(action)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "%s: merge request !%d\n", mergeRequestActionPastTense(action), out.IID); err != nil {
			return err
		}
	}

	return writeMergeRequestText(w, out, false)
}

func mergeRequestActionHelp(action string, iid int64, hints *MRHintContext) []string {
	suffix := hints.ProjectSuffix()

	switch action {
	case "close":
		return []string{
			fmt.Sprintf("Run `mr reopen %d%s` to reopen it", iid, suffix),
			fmt.Sprintf("Run `mr view %d%s` to inspect the closed merge request", iid, suffix),
		}
	case "reopen":
		return []string{
			fmt.Sprintf("Run `mr view %d%s` to check merge status and pipeline results", iid, suffix),
			fmt.Sprintf("Run `mr merge %d%s` when it is ready to merge", iid, suffix),
		}
	default:
		return []string{
			fmt.Sprintf("Run `mr view %d%s` to verify the merge request state", iid, suffix),
		}
	}
}

func mergeRequestActionPastTense(action string) string {
	switch action {
	case "close":
		return "closed"
	case "reopen":
		return "reopened"
	default:
		return "merged"
	}
}

func mergeRequestActionDoneState(action string) string {
	switch action {
	case "close":
		return "closed"
	case "reopen":
		return "open"
	default:
		return "merged"
	}
}

func compactMergeRequestView(out mergeRequestOutput) axiMergeRequestCompact {
	return axiMergeRequestCompact{
		IID:                 out.IID,
		Title:               out.Title,
		State:               out.State,
		Draft:               out.Draft,
		Author:              out.Author,
		SourceBranch:        out.SourceBranch,
		TargetBranch:        out.TargetBranch,
		DetailedMergeStatus: out.DetailedMergeStatus,
		HasConflicts:        out.HasConflicts,
		PipelineStatus:      out.PipelineStatus,
		UserNotesCount:      out.UserNotesCount,
		UpdatedAt:           out.UpdatedAt,
		WebURL:              out.WebURL,
		Approval:            out.Approval,
		Description:         out.Description,
	}
}

func writeMergeRequestText(w io.Writer, out mergeRequestOutput, full bool) error {
	var err error
	if full {
		_, err = fmt.Fprintf(
			w,
			"iid: %d\ntitle: %s\nstate: %s\ndraft: %t\nauthor: %s\nassignees: %s\nreviewers: %s\nsource_branch: %s\ntarget_branch: %s\nlabels: %s\nmilestone: %s\ndetailed_merge_status: %s\nhas_conflicts: %t\nblocking_discussions_resolved: %t\nuser_notes_count: %d\nchanges_count: %s\npipeline_status: %s\nsha: %s\ncreated_at: %s\nupdated_at: %s\nmerged_at: %s\nclosed_at: %s\nweb_url: %s\n",
			out.IID,
			out.Title,
			out.State,
			out.Draft,
			out.Author,
			strings.Join(out.Assignees, ", "),
			strings.Join(out.Reviewers, ", "),
			out.SourceBranch,
			out.TargetBranch,
			strings.Join(out.Labels, ", "),
			out.Milestone,
			out.DetailedMergeStatus,
			out.HasConflicts,
			out.BlockingDiscussionsResolved,
			out.UserNotesCount,
			out.ChangesCount,
			out.PipelineStatus,
			out.SHA,
			out.CreatedAt,
			out.UpdatedAt,
			out.MergedAt,
			out.ClosedAt,
			out.WebURL,
		)
	} else {
		_, err = fmt.Fprintf(
			w,
			"iid: %d\ntitle: %s\nstate: %s\ndraft: %t\nauthor: %s\nsource_branch: %s\ntarget_branch: %s\ndetailed_merge_status: %s\nhas_conflicts: %t\npipeline_status: %s\nuser_notes_count: %d\nupdated_at: %s\nweb_url: %s\n",
			out.IID,
			out.Title,
			out.State,
			out.Draft,
			out.Author,
			out.SourceBranch,
			out.TargetBranch,
			out.DetailedMergeStatus,
			out.HasConflicts,
			out.PipelineStatus,
			out.UserNotesCount,
			out.UpdatedAt,
			out.WebURL,
		)
	}
	if err != nil {
		return err
	}
	if out.Approval != nil {
		if err := writeMergeRequestApprovalCompactText(w, *out.Approval); err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(w, "description:\n%s\n", out.Description)

	return err
}

func WriteMergeRequestList(w io.Writer, format string, mode Mode, mergeRequests []*gitlab.BasicMergeRequest, paging MRListPaging, fields []string, hints *MRHintContext) error {
	if mode == ModeAxi {
		rows := make([]AxiMergeRequestRow, 0, len(mergeRequests))
		for _, mergeRequest := range mergeRequests {
			if mergeRequest == nil {
				continue
			}
			rows = append(rows, AxiMergeRequestRowFor(mergeRequest, fields))
		}

		return WriteAxi(w, format, axiMergeRequestListOutput{
			MergeRequests: rows,
			Count:         MRListCountLine(len(rows), paging),
			Total:         paging.TotalItems,
			Page:          paging.Page,
			TotalPages:    paging.TotalPages,
			Help:          mrListHelp(len(rows), paging, hints),
		})
	}

	format, err := NormalizeFormat(format, mode)
	if err != nil {
		return err
	}

	rows := make([]mergeRequestRowOutput, 0, len(mergeRequests))
	for _, mergeRequest := range mergeRequests {
		if mergeRequest == nil {
			continue
		}
		rows = append(rows, basicMergeRequestToRow(mergeRequest))
	}

	if format == "json" {
		return WriteJSON(w, mergeRequestListOutput{
			MergeRequests: rows,
			Count:         len(rows),
			Total:         paging.TotalItems,
			Page:          paging.Page,
			TotalPages:    paging.TotalPages,
		})
	}

	return renderMergeRequestTable(w, rows, paging)
}

func mrListHelp(count int, paging MRListPaging, hints *MRHintContext) []string {
	suffix := hints.ProjectSuffix()

	if count == 0 {
		return []string{
			fmt.Sprintf("No merge requests matched — run `mr list --state all%s` to include merged and closed ones, or relax other filters", suffix),
		}
	}

	help := []string{fmt.Sprintf("Run `mr view <iid>%s` for details", suffix)}
	switch {
	case paging.TotalPages > paging.Page:
		help = append(help, fmt.Sprintf("Run `mr list --page %d%s` for the next page", paging.Page+1, suffix))
	case paging.TotalItems == 0 && hints != nil && int64(count) >= hints.Limit:
		help = append(help, fmt.Sprintf("More results may exist — run `mr list --page %d%s`", paging.Page+1, suffix))
	}

	return help
}

func mergeRequestToOutput(mergeRequest *gitlab.MergeRequest, full bool, mode Mode) (mergeRequestOutput, bool) {
	out := mergeRequestOutput{
		IID:                         mergeRequest.IID,
		Title:                       mergeRequest.Title,
		State:                       mergeRequest.State,
		Draft:                       mergeRequest.Draft,
		Assignees:                   usernamesOf(mergeRequest.Assignees),
		Reviewers:                   usernamesOf(mergeRequest.Reviewers),
		SourceBranch:                mergeRequest.SourceBranch,
		TargetBranch:                mergeRequest.TargetBranch,
		Labels:                      []string(mergeRequest.Labels),
		DetailedMergeStatus:         mergeRequest.DetailedMergeStatus,
		HasConflicts:                mergeRequest.HasConflicts,
		BlockingDiscussionsResolved: mergeRequest.BlockingDiscussionsResolved,
		UserNotesCount:              mergeRequest.UserNotesCount,
		ChangesCount:                mergeRequest.ChangesCount,
		SHA:                         mergeRequest.SHA,
		CreatedAt:                   formatTimeValue(mergeRequest.CreatedAt),
		UpdatedAt:                   formatTimeValue(mergeRequest.UpdatedAt),
		MergedAt:                    formatTimeValue(mergeRequest.MergedAt),
		ClosedAt:                    formatTimeValue(mergeRequest.ClosedAt),
		WebURL:                      mergeRequest.WebURL,
		Description:                 mergeRequest.Description,
	}
	if out.Labels == nil {
		out.Labels = []string{}
	}
	if mergeRequest.Author != nil {
		out.Author = mergeRequest.Author.Username
	}
	if mergeRequest.Milestone != nil {
		out.Milestone = mergeRequest.Milestone.Title
	}
	if mergeRequest.HeadPipeline != nil {
		out.PipelineStatus = mergeRequest.HeadPipeline.Status
	} else if mergeRequest.Pipeline != nil {
		out.PipelineStatus = mergeRequest.Pipeline.Status
	}

	truncated := false
	if !full {
		out.Description, truncated = TruncateDescription(out.Description, descriptionTruncateLimit, mode)
	}

	return out, truncated
}

func basicMergeRequestToRow(mergeRequest *gitlab.BasicMergeRequest) mergeRequestRowOutput {
	row := mergeRequestRowOutput{
		IID:          mergeRequest.IID,
		Title:        mergeRequest.Title,
		State:        mergeRequest.State,
		Draft:        mergeRequest.Draft,
		SourceBranch: mergeRequest.SourceBranch,
		TargetBranch: mergeRequest.TargetBranch,
		UpdatedAt:    formatTimeValue(mergeRequest.UpdatedAt),
		WebURL:       mergeRequest.WebURL,
	}
	if mergeRequest.Author != nil {
		row.Author = mergeRequest.Author.Username
	}

	return row
}

func AxiMergeRequestRowFor(mergeRequest *gitlab.BasicMergeRequest, fields []string) AxiMergeRequestRow {
	full := basicMergeRequestToRow(mergeRequest)
	row := AxiMergeRequestRow{
		IID:    full.IID,
		Title:  full.Title,
		State:  full.State,
		Author: full.Author,
	}

	for _, field := range fields {
		switch field {
		case "draft":
			row.Draft = &full.Draft
		case "source_branch":
			row.SourceBranch = &full.SourceBranch
		case "target_branch":
			row.TargetBranch = &full.TargetBranch
		case "updated_at":
			row.UpdatedAt = &full.UpdatedAt
		case "web_url":
			row.WebURL = &full.WebURL
		}
	}

	return row
}

const descriptionTruncateLimit = 500
