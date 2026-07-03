package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/shabashab/community-gitlab-cli/internal/credstore"
	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type userOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
}

type axiUserOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	WebURL   string `json:"web_url"`
}

type axiWhoamiOutput struct {
	User axiUserOutput `json:"user"`
	Next string        `json:"next"`
}

type authLoginResult struct {
	Username string `json:"username"`
	Domain   string `json:"domain"`
	Backend  string `json:"backend"`
}

type axiAuthLoginOutput struct {
	Login authLoginResult `json:"login"`
	Next  string          `json:"next"`
}

type authLogoutResult struct {
	Domain   string   `json:"domain"`
	Backends []string `json:"backends"`
}

type axiAuthLogoutOutput struct {
	Logout authLogoutResult `json:"logout"`
	Next   string           `json:"next"`
}

type authStatusResult struct {
	Domain        string   `json:"domain"`
	Authenticated bool     `json:"authenticated"`
	Backends      []string `json:"backends"`
	Warnings      []string `json:"warnings,omitempty"`
}

type axiAuthStatusOutput struct {
	Status authStatusResult `json:"status"`
	Next   string           `json:"next"`
}

type projectNamespaceOutput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	FullPath string `json:"full_path"`
	WebURL   string `json:"web_url"`
}

type projectOutput struct {
	ID                int64                   `json:"id"`
	Name              string                  `json:"name"`
	NameWithNamespace string                  `json:"name_with_namespace"`
	Path              string                  `json:"path"`
	PathWithNamespace string                  `json:"path_with_namespace"`
	Description       string                  `json:"description"`
	DefaultBranch     string                  `json:"default_branch"`
	Visibility        string                  `json:"visibility"`
	WebURL            string                  `json:"web_url"`
	SSHURLToRepo      string                  `json:"ssh_url_to_repo"`
	HTTPURLToRepo     string                  `json:"http_url_to_repo"`
	Archived          bool                    `json:"archived"`
	EmptyRepo         bool                    `json:"empty_repo"`
	OpenIssuesCount   int64                   `json:"open_issues_count"`
	StarCount         int64                   `json:"star_count"`
	ForksCount        int64                   `json:"forks_count"`
	LastActivityAt    string                  `json:"last_activity_at"`
	Namespace         *projectNamespaceOutput `json:"namespace"`
}

type axiProjectInfoOutput struct {
	Project projectOutput `json:"project"`
	Next    string        `json:"next"`
}

type mergeRequestRowOutput struct {
	IID          int64  `json:"iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	Draft        bool   `json:"draft"`
	Author       string `json:"author"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	UpdatedAt    string `json:"updated_at"`
	WebURL       string `json:"web_url"`
}

type mergeRequestOutput struct {
	IID                         int64    `json:"iid"`
	Title                       string   `json:"title"`
	State                       string   `json:"state"`
	Draft                       bool     `json:"draft"`
	Author                      string   `json:"author"`
	Assignees                   []string `json:"assignees"`
	Reviewers                   []string `json:"reviewers"`
	SourceBranch                string   `json:"source_branch"`
	TargetBranch                string   `json:"target_branch"`
	Labels                      []string `json:"labels"`
	Milestone                   string   `json:"milestone"`
	DetailedMergeStatus         string   `json:"detailed_merge_status"`
	HasConflicts                bool     `json:"has_conflicts"`
	BlockingDiscussionsResolved bool     `json:"blocking_discussions_resolved"`
	UserNotesCount              int64    `json:"user_notes_count"`
	ChangesCount                string   `json:"changes_count"`
	PipelineStatus              string   `json:"pipeline_status"`
	SHA                         string   `json:"sha"`
	CreatedAt                   string   `json:"created_at"`
	UpdatedAt                   string   `json:"updated_at"`
	MergedAt                    string   `json:"merged_at"`
	ClosedAt                    string   `json:"closed_at"`
	WebURL                      string   `json:"web_url"`
	Description                 string   `json:"description"`
}

type mergeRequestListOutput struct {
	MergeRequests []mergeRequestRowOutput `json:"merge_requests"`
	Count         int                     `json:"count"`
	Total         int64                   `json:"total"`
	Page          int64                   `json:"page"`
	TotalPages    int64                   `json:"total_pages"`
}

type axiMergeRequestViewOutput struct {
	MergeRequest mergeRequestOutput `json:"merge_request"`
	Next         string             `json:"next"`
}

type axiMergeRequestListOutput struct {
	MergeRequests []mergeRequestRowOutput `json:"merge_requests"`
	Count         int                     `json:"count"`
	Total         int64                   `json:"total"`
	Page          int64                   `json:"page"`
	TotalPages    int64                   `json:"total_pages"`
	Next          string                  `json:"next"`
}

type mrListPaging struct {
	page       int64
	totalItems int64
	totalPages int64
}

func defaultOutputFormat(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon"
	}

	return "text"
}

func outputFormats(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon, json"
	}

	return "text, json"
}

func writeUser(w io.Writer, format string, mode commandMode, user *gitlab.User) error {
	if user == nil {
		return errors.New("gitlab api returned an empty current user response")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiUser(w, format, user)
	}

	out := userOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		State:    user.State,
		WebURL:   user.WebURL,
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	case "text":
		_, err := fmt.Fprintf(
			w,
			"id: %d\nusername: %s\nname: %s\nstate: %s\nweb_url: %s\n",
			out.ID,
			out.Username,
			out.Name,
			out.State,
			out.WebURL,
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiUser(w io.Writer, format string, user *gitlab.User) error {
	out := axiUserOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		WebURL:   user.WebURL,
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiWhoamiOutput{
			User: out,
			Next: "Use project list when available to inspect accessible projects.",
		})
	case "toon":
		_, err := fmt.Fprintf(
			w,
			"user{id,username,name,web_url}:\n  %d,%s,%s,%s\nnext: %s\n",
			out.ID,
			toonValue(out.Username),
			toonValue(out.Name),
			toonValue(out.WebURL),
			toonValue("Use project list when available to inspect accessible projects."),
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAuthLogin(w io.Writer, format string, mode commandMode, result authLoginResult) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiAuthLogin(w, format, result)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		_, err := fmt.Fprintf(
			w,
			"username: %s\ndomain: %s\nbackend: %s\n",
			result.Username,
			result.Domain,
			result.Backend,
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiAuthLogin(w io.Writer, format string, result authLoginResult) error {
	next := fmt.Sprintf("Credential stored for %s. Run whoami or mr to start working.", result.Domain)

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiAuthLoginOutput{Login: result, Next: next})
	case "toon":
		_, err := fmt.Fprintf(
			w,
			"login{username,domain,backend}:\n  %s,%s,%s\nnext: %s\n",
			toonValue(result.Username),
			toonValue(result.Domain),
			toonValue(result.Backend),
			toonValue(next),
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAuthLogout(w io.Writer, format string, mode commandMode, result authLogoutResult) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiAuthLogout(w, format, result)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		_, err := fmt.Fprintf(
			w,
			"domain: %s\nremoved_from: %s\n",
			result.Domain,
			strings.Join(result.Backends, ", "),
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiAuthLogout(w io.Writer, format string, result authLogoutResult) error {
	next := "Credential removed. Run auth login <token> --gitlab-base-url <url> to authenticate again."

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiAuthLogoutOutput{Logout: result, Next: next})
	case "toon":
		_, err := fmt.Fprintf(
			w,
			"logout{domain,backends}:\n  %s,%s\nnext: %s\n",
			toonValue(result.Domain),
			toonValue(strings.Join(result.Backends, " ")),
			toonValue(next),
		)
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAuthStatus(w io.Writer, format string, mode commandMode, result authStatusResult) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiAuthStatus(w, format, result)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		if _, err := fmt.Fprintf(
			w,
			"domain: %s\nauthenticated: %t\nbackends: %s\n",
			result.Domain,
			result.Authenticated,
			strings.Join(result.Backends, ", "),
		); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "warning: %s\n", warning); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiAuthStatus(w io.Writer, format string, result authStatusResult) error {
	next := "Run auth login <token> --gitlab-base-url <url> to store a credential."
	if result.Authenticated {
		next = "Run whoami to verify the stored token still works."
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiAuthStatusOutput{Status: result, Next: next})
	case "toon":
		if _, err := fmt.Fprintf(
			w,
			"status{domain,authenticated,backends}:\n  %s,%t,%s\n",
			toonValue(result.Domain),
			result.Authenticated,
			toonValue(strings.Join(result.Backends, " ")),
		); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "warning: %s\n", toonValue(warning)); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w, "next: %s\n", toonValue(next))
		return err
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeProject(w io.Writer, format string, mode commandMode, project *gitlab.Project) error {
	if project == nil {
		return errors.New("gitlab api returned an empty project response")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	if mode == commandModeAxi {
		return writeAxiProject(w, format, project)
	}

	out := projectToOutput(project)
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	case "text":
		if err := writeProjectText(w, out); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeProjectText(w io.Writer, out projectOutput) error {
	_, err := fmt.Fprintf(
		w,
		"id: %d\nname: %s\nname_with_namespace: %s\npath: %s\npath_with_namespace: %s\ndescription: %s\ndefault_branch: %s\nvisibility: %s\nweb_url: %s\nssh_url_to_repo: %s\nhttp_url_to_repo: %s\narchived: %t\nempty_repo: %t\nopen_issues_count: %d\nstar_count: %d\nforks_count: %d\nlast_activity_at: %s\n",
		out.ID,
		out.Name,
		out.NameWithNamespace,
		out.Path,
		out.PathWithNamespace,
		out.Description,
		out.DefaultBranch,
		out.Visibility,
		out.WebURL,
		out.SSHURLToRepo,
		out.HTTPURLToRepo,
		out.Archived,
		out.EmptyRepo,
		out.OpenIssuesCount,
		out.StarCount,
		out.ForksCount,
		out.LastActivityAt,
	)
	if err != nil {
		return err
	}

	if out.Namespace == nil {
		return nil
	}

	_, err = fmt.Fprintf(
		w,
		"namespace_id: %d\nnamespace_name: %s\nnamespace_path: %s\nnamespace_kind: %s\nnamespace_full_path: %s\nnamespace_web_url: %s\n",
		out.Namespace.ID,
		out.Namespace.Name,
		out.Namespace.Path,
		out.Namespace.Kind,
		out.Namespace.FullPath,
		out.Namespace.WebURL,
	)
	return err
}

func writeAxiProject(w io.Writer, format string, project *gitlab.Project) error {
	out := projectToOutput(project)
	next := "Use --project to inspect another project, or run inside a GitLab repository with origin configured."

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiProjectInfoOutput{
			Project: out,
			Next:    next,
		})
	case "toon":
		if err := writeAxiProjectTOON(w, out, next); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAxiProjectTOON(w io.Writer, out projectOutput, next string) error {
	_, err := fmt.Fprintf(
		w,
		"project{id,name,name_with_namespace,path,path_with_namespace,description,default_branch,visibility,web_url,ssh_url_to_repo,http_url_to_repo,archived,empty_repo,open_issues_count,star_count,forks_count,last_activity_at}:\n  %d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%t,%t,%d,%d,%d,%s\n",
		out.ID,
		toonValue(out.Name),
		toonValue(out.NameWithNamespace),
		toonValue(out.Path),
		toonValue(out.PathWithNamespace),
		toonValue(out.Description),
		toonValue(out.DefaultBranch),
		toonValue(out.Visibility),
		toonValue(out.WebURL),
		toonValue(out.SSHURLToRepo),
		toonValue(out.HTTPURLToRepo),
		out.Archived,
		out.EmptyRepo,
		out.OpenIssuesCount,
		out.StarCount,
		out.ForksCount,
		toonValue(out.LastActivityAt),
	)
	if err != nil {
		return err
	}

	if out.Namespace != nil {
		_, err = fmt.Fprintf(
			w,
			"namespace{id,name,path,kind,full_path,web_url}:\n  %d,%s,%s,%s,%s,%s\n",
			out.Namespace.ID,
			toonValue(out.Namespace.Name),
			toonValue(out.Namespace.Path),
			toonValue(out.Namespace.Kind),
			toonValue(out.Namespace.FullPath),
			toonValue(out.Namespace.WebURL),
		)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintf(w, "next: %s\n", toonValue(next))
	return err
}

func projectToOutput(project *gitlab.Project) projectOutput {
	out := projectOutput{
		ID:                project.ID,
		Name:              project.Name,
		NameWithNamespace: project.NameWithNamespace,
		Path:              project.Path,
		PathWithNamespace: project.PathWithNamespace,
		Description:       project.Description,
		DefaultBranch:     project.DefaultBranch,
		Visibility:        string(project.Visibility),
		WebURL:            project.WebURL,
		SSHURLToRepo:      project.SSHURLToRepo,
		HTTPURLToRepo:     project.HTTPURLToRepo,
		Archived:          project.Archived,
		EmptyRepo:         project.EmptyRepo,
		OpenIssuesCount:   project.OpenIssuesCount,
		StarCount:         project.StarCount,
		ForksCount:        project.ForksCount,
	}
	if project.LastActivityAt != nil {
		out.LastActivityAt = project.LastActivityAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if project.Namespace != nil {
		out.Namespace = &projectNamespaceOutput{
			ID:       project.Namespace.ID,
			Name:     project.Namespace.Name,
			Path:     project.Namespace.Path,
			Kind:     project.Namespace.Kind,
			FullPath: project.Namespace.FullPath,
			WebURL:   project.Namespace.WebURL,
		}
	}

	return out
}

func writeMergeRequest(w io.Writer, format string, mode commandMode, mergeRequest *gitlab.MergeRequest, full bool) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	out := mergeRequestToOutput(mergeRequest, full)

	if mode == commandModeAxi {
		return writeAxiMergeRequest(w, format, out, full)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(out)
	case "text":
		return writeMergeRequestText(w, out, full)
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
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
			"iid: %d\ntitle: %s\nstate: %s\ndraft: %t\nauthor: %s\nsource_branch: %s\ntarget_branch: %s\ndetailed_merge_status: %s\nhas_conflicts: %t\nuser_notes_count: %d\nupdated_at: %s\nweb_url: %s\n",
			out.IID,
			out.Title,
			out.State,
			out.Draft,
			out.Author,
			out.SourceBranch,
			out.TargetBranch,
			out.DetailedMergeStatus,
			out.HasConflicts,
			out.UserNotesCount,
			out.UpdatedAt,
			out.WebURL,
		)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "description:\n%s\n", out.Description)
	return err
}

func writeAxiMergeRequest(w io.Writer, format string, out mergeRequestOutput, full bool) error {
	next := "Use mr list to browse merge requests."
	if !full {
		next = "Use --full for all fields, or mr list to browse merge requests."
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiMergeRequestViewOutput{
			MergeRequest: out,
			Next:         next,
		})
	case "toon":
		return writeAxiMergeRequestTOON(w, out, full, next)
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAxiMergeRequestTOON(w io.Writer, out mergeRequestOutput, full bool, next string) error {
	var err error
	if full {
		_, err = fmt.Fprintf(
			w,
			"merge_request{iid,title,state,draft,author,assignees,reviewers,source_branch,target_branch,labels,milestone,detailed_merge_status,has_conflicts,blocking_discussions_resolved,user_notes_count,changes_count,pipeline_status,sha,created_at,updated_at,merged_at,closed_at,web_url}:\n  %d,%s,%s,%t,%s,%s,%s,%s,%s,%s,%s,%s,%t,%t,%d,%s,%s,%s,%s,%s,%s,%s,%s\n",
			out.IID,
			toonValue(out.Title),
			toonValue(out.State),
			out.Draft,
			toonValue(out.Author),
			toonValue(strings.Join(out.Assignees, ";")),
			toonValue(strings.Join(out.Reviewers, ";")),
			toonValue(out.SourceBranch),
			toonValue(out.TargetBranch),
			toonValue(strings.Join(out.Labels, ";")),
			toonValue(out.Milestone),
			toonValue(out.DetailedMergeStatus),
			out.HasConflicts,
			out.BlockingDiscussionsResolved,
			out.UserNotesCount,
			toonValue(out.ChangesCount),
			toonValue(out.PipelineStatus),
			toonValue(out.SHA),
			toonValue(out.CreatedAt),
			toonValue(out.UpdatedAt),
			toonValue(out.MergedAt),
			toonValue(out.ClosedAt),
			toonValue(out.WebURL),
		)
	} else {
		_, err = fmt.Fprintf(
			w,
			"merge_request{iid,title,state,draft,author,source_branch,target_branch,detailed_merge_status,has_conflicts,user_notes_count,updated_at,web_url}:\n  %d,%s,%s,%t,%s,%s,%s,%s,%t,%d,%s,%s\n",
			out.IID,
			toonValue(out.Title),
			toonValue(out.State),
			out.Draft,
			toonValue(out.Author),
			toonValue(out.SourceBranch),
			toonValue(out.TargetBranch),
			toonValue(out.DetailedMergeStatus),
			out.HasConflicts,
			out.UserNotesCount,
			toonValue(out.UpdatedAt),
			toonValue(out.WebURL),
		)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "description: %s\nnext: %s\n", toonValue(out.Description), toonValue(next))
	return err
}

func writeMergeRequestList(w io.Writer, format string, mode commandMode, mergeRequests []*gitlab.BasicMergeRequest, paging mrListPaging) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = defaultOutputFormat(mode)
	}

	rows := make([]mergeRequestRowOutput, 0, len(mergeRequests))
	for _, mergeRequest := range mergeRequests {
		if mergeRequest == nil {
			continue
		}
		rows = append(rows, basicMergeRequestToRow(mergeRequest))
	}

	if mode == commandModeAxi {
		return writeAxiMergeRequestList(w, format, rows, paging)
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(mergeRequestListOutput{
			MergeRequests: rows,
			Count:         len(rows),
			Total:         paging.totalItems,
			Page:          paging.page,
			TotalPages:    paging.totalPages,
		})
	case "text":
		return renderMergeRequestTable(w, rows, paging)
	default:
		return fmt.Errorf("unsupported output format %q: use text or json", format)
	}
}

func writeAxiMergeRequestList(w io.Writer, format string, rows []mergeRequestRowOutput, paging mrListPaging) error {
	next := "Use mr !<iid> for details, --page <n> for more results, or filters like --state/--author/--search/--label to narrow."
	if len(rows) == 0 {
		next = "No merge requests matched. Try --state all or relax other filters."
	}

	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(axiMergeRequestListOutput{
			MergeRequests: rows,
			Count:         len(rows),
			Total:         paging.totalItems,
			Page:          paging.page,
			TotalPages:    paging.totalPages,
			Next:          next,
		})
	case "toon":
		return writeAxiMergeRequestListTOON(w, rows, paging, next)
	default:
		return fmt.Errorf("unsupported output format %q: use toon or json", format)
	}
}

func writeAxiMergeRequestListTOON(w io.Writer, rows []mergeRequestRowOutput, paging mrListPaging, next string) error {
	if _, err := fmt.Fprintf(
		w,
		"merge_requests[%d]{iid,title,state,draft,author,source_branch,target_branch,updated_at}:\n",
		len(rows),
	); err != nil {
		return err
	}

	for _, row := range rows {
		if _, err := fmt.Fprintf(
			w,
			"  %d,%s,%s,%t,%s,%s,%s,%s\n",
			row.IID,
			toonValue(row.Title),
			toonValue(row.State),
			row.Draft,
			toonValue(row.Author),
			toonValue(row.SourceBranch),
			toonValue(row.TargetBranch),
			toonValue(row.UpdatedAt),
		); err != nil {
			return err
		}
	}

	if len(rows) > 0 && paging.totalItems == 0 {
		if _, err := fmt.Fprintf(w, "count: %d\n", len(rows)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "count: %d of %d total\n", len(rows), paging.totalItems); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w, "next: %s\n", toonValue(next))
	return err
}

func mergeRequestToOutput(mergeRequest *gitlab.MergeRequest, full bool) mergeRequestOutput {
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
	if !full {
		out.Description = truncateWithHint(out.Description, descriptionTruncateLimit)
	}

	return out
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

func usernamesOf(users []*gitlab.BasicUser) []string {
	if len(users) == 0 {
		return nil
	}

	names := make([]string, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		names = append(names, user.Username)
	}

	return names
}

func formatTimeValue(t *time.Time) string {
	if t == nil {
		return ""
	}

	return t.Format("2006-01-02T15:04:05Z07:00")
}

func truncateWithHint(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return fmt.Sprintf(
		"%s… (truncated, %d chars total — use --full for the complete description)",
		string(runes[:limit]),
		len(runes),
	)
}

func writeCommandError(w io.Writer, mode commandMode, err error) {
	if mode != commandModeAxi {
		fmt.Fprintln(w, err)
		return
	}

	code := "command_failed"
	next := "Inspect the error message, fix the input or GitLab configuration, then retry."
	if errors.Is(err, errMissingExplicitBaseURL) {
		code = "missing_gitlab_base_url"
		next = "Pass --gitlab-base-url https://<gitlab-host> to auth login, then retry."
	} else if errors.Is(err, errTokenVerification) {
		code = "invalid_gitlab_token"
		next = "Check the token value and scopes (read_api at minimum), then retry auth login."
	} else if errors.Is(err, credstore.ErrNotFound) {
		code = "no_stored_credential"
		next = "Run auth status to inspect credential state or auth login <token> --gitlab-base-url <url> to store one."
	} else if errors.Is(err, credstore.ErrCorruptCredentials) || errors.Is(err, credstore.ErrUnsupportedVersion) {
		code = "credential_store_unreadable"
		next = "Inspect or remove ~/.gl/credentials.json, then run auth login again."
	} else if errors.Is(err, gitlabclient.ErrMissingToken) {
		code = "missing_gitlab_token"
		next = "Set GITLAB_TOKEN, pass --gitlab-token, or run auth login <token> --gitlab-base-url <url>, then retry."
	} else if errors.Is(err, errMissingProject) {
		code = "missing_gitlab_project"
		next = "Run inside a git repository with remote origin configured or pass --project."
	} else if errors.Is(err, errInvalidMergeRequestRef) {
		code = "invalid_merge_request_ref"
		next = "Reference merge requests as !<iid> or <iid>, for example mr !123."
	} else if errors.Is(err, errUnknownMergeRequestAction) {
		code = "unknown_merge_request_action"
		next = "Supported per-merge-request actions: view. Run mr --help for usage."
	}

	fmt.Fprintf(
		w,
		"error{code,message}:\n  %s,%s\nnext: %s\n",
		code,
		toonValue(err.Error()),
		toonValue(next),
	)
}

func toonValue(value string) string {
	if value == "" {
		return `""`
	}

	if strings.ContainsAny(value, ",\n\r\t\"{}[]:") || strings.Contains(value, " ") {
		return strconv.Quote(value)
	}

	return value
}
