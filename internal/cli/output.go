package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	toon "github.com/toon-format/toon-go"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type userOutput struct {
	ID       int64  `json:"id" toon:"id"`
	Username string `json:"username" toon:"username"`
	Name     string `json:"name" toon:"name"`
	State    string `json:"state" toon:"state"`
	WebURL   string `json:"web_url" toon:"web_url"`
}

type axiUserOutput struct {
	ID       int64  `json:"id" toon:"id"`
	Username string `json:"username" toon:"username"`
	Name     string `json:"name" toon:"name"`
	WebURL   string `json:"web_url" toon:"web_url"`
}

type axiWhoamiOutput struct {
	User axiUserOutput `json:"user" toon:"user"`
	Help []string      `json:"help,omitempty" toon:"help,omitempty"`
}

type authLoginResult struct {
	Username string `json:"username" toon:"username"`
	Domain   string `json:"domain" toon:"domain"`
	Backend  string `json:"backend" toon:"backend"`
}

type axiAuthLoginOutput struct {
	Login authLoginResult `json:"login" toon:"login"`
	Help  []string        `json:"help,omitempty" toon:"help,omitempty"`
}

type authLogoutResult struct {
	Domain   string   `json:"domain" toon:"domain"`
	Backends []string `json:"backends" toon:"backends"`
	Noop     bool     `json:"noop,omitempty" toon:"noop,omitempty"`
}

type axiAuthLogoutOutput struct {
	Logout authLogoutResult `json:"logout" toon:"logout"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

type authStatusResult struct {
	Domain        string   `json:"domain" toon:"domain"`
	Authenticated bool     `json:"authenticated" toon:"authenticated"`
	Backends      []string `json:"backends" toon:"backends"`
	Warnings      []string `json:"warnings,omitempty" toon:"warnings,omitempty"`
}

type axiAuthStatusOutput struct {
	Status authStatusResult `json:"status" toon:"status"`
	Help   []string         `json:"help,omitempty" toon:"help,omitempty"`
}

type projectNamespaceOutput struct {
	ID       int64  `json:"id" toon:"id"`
	Name     string `json:"name" toon:"name"`
	Path     string `json:"path" toon:"path"`
	Kind     string `json:"kind" toon:"kind"`
	FullPath string `json:"full_path" toon:"full_path"`
	WebURL   string `json:"web_url" toon:"web_url"`
}

type projectOutput struct {
	ID                int64                   `json:"id" toon:"id"`
	Name              string                  `json:"name" toon:"name"`
	NameWithNamespace string                  `json:"name_with_namespace" toon:"name_with_namespace"`
	Path              string                  `json:"path" toon:"path"`
	PathWithNamespace string                  `json:"path_with_namespace" toon:"path_with_namespace"`
	Description       string                  `json:"description" toon:"description"`
	DefaultBranch     string                  `json:"default_branch" toon:"default_branch"`
	Visibility        string                  `json:"visibility" toon:"visibility"`
	WebURL            string                  `json:"web_url" toon:"web_url"`
	SSHURLToRepo      string                  `json:"ssh_url_to_repo" toon:"ssh_url_to_repo"`
	HTTPURLToRepo     string                  `json:"http_url_to_repo" toon:"http_url_to_repo"`
	Archived          bool                    `json:"archived" toon:"archived"`
	EmptyRepo         bool                    `json:"empty_repo" toon:"empty_repo"`
	OpenIssuesCount   int64                   `json:"open_issues_count" toon:"open_issues_count"`
	StarCount         int64                   `json:"star_count" toon:"star_count"`
	ForksCount        int64                   `json:"forks_count" toon:"forks_count"`
	LastActivityAt    string                  `json:"last_activity_at" toon:"last_activity_at"`
	Namespace         *projectNamespaceOutput `json:"namespace,omitempty" toon:"namespace,omitempty"`
}

type axiProjectInfoOutput struct {
	Project projectOutput `json:"project" toon:"project"`
}

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

// axiMergeRequestRow is the compact axi list row. Optional fields are
// pointers with omitempty so --fields controls exactly which columns are
// emitted while every row stays uniform (required for TOON tabular output).
type axiMergeRequestRow struct {
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
	IID                         int64    `json:"iid" toon:"iid"`
	Title                       string   `json:"title" toon:"title"`
	State                       string   `json:"state" toon:"state"`
	Draft                       bool     `json:"draft" toon:"draft"`
	Author                      string   `json:"author" toon:"author"`
	Assignees                   []string `json:"assignees" toon:"assignees"`
	Reviewers                   []string `json:"reviewers" toon:"reviewers"`
	SourceBranch                string   `json:"source_branch" toon:"source_branch"`
	TargetBranch                string   `json:"target_branch" toon:"target_branch"`
	Labels                      []string `json:"labels" toon:"labels"`
	Milestone                   string   `json:"milestone" toon:"milestone"`
	DetailedMergeStatus         string   `json:"detailed_merge_status" toon:"detailed_merge_status"`
	HasConflicts                bool     `json:"has_conflicts" toon:"has_conflicts"`
	BlockingDiscussionsResolved bool     `json:"blocking_discussions_resolved" toon:"blocking_discussions_resolved"`
	UserNotesCount              int64    `json:"user_notes_count" toon:"user_notes_count"`
	ChangesCount                string   `json:"changes_count" toon:"changes_count"`
	PipelineStatus              string   `json:"pipeline_status" toon:"pipeline_status"`
	SHA                         string   `json:"sha" toon:"sha"`
	CreatedAt                   string   `json:"created_at" toon:"created_at"`
	UpdatedAt                   string   `json:"updated_at" toon:"updated_at"`
	MergedAt                    string   `json:"merged_at" toon:"merged_at"`
	ClosedAt                    string   `json:"closed_at" toon:"closed_at"`
	WebURL                      string   `json:"web_url" toon:"web_url"`
	Description                 string   `json:"description" toon:"description"`
}

// axiMergeRequestCompact is the token-frugal axi detail view.
type axiMergeRequestCompact struct {
	IID                 int64  `json:"iid" toon:"iid"`
	Title               string `json:"title" toon:"title"`
	State               string `json:"state" toon:"state"`
	Draft               bool   `json:"draft" toon:"draft"`
	Author              string `json:"author" toon:"author"`
	SourceBranch        string `json:"source_branch" toon:"source_branch"`
	TargetBranch        string `json:"target_branch" toon:"target_branch"`
	DetailedMergeStatus string `json:"detailed_merge_status" toon:"detailed_merge_status"`
	HasConflicts        bool   `json:"has_conflicts" toon:"has_conflicts"`
	PipelineStatus      string `json:"pipeline_status" toon:"pipeline_status"`
	UserNotesCount      int64  `json:"user_notes_count" toon:"user_notes_count"`
	UpdatedAt           string `json:"updated_at" toon:"updated_at"`
	WebURL              string `json:"web_url" toon:"web_url"`
	Description         string `json:"description" toon:"description"`
}

type axiMergeRequestViewOutput struct {
	MergeRequest any      `json:"merge_request" toon:"merge_request"`
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
	MergeRequests []axiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Total         int64                `json:"total" toon:"-"`
	Page          int64                `json:"page" toon:"-"`
	TotalPages    int64                `json:"total_pages" toon:"-"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// axiHomeRepoOutput is the gl-axi no-args dashboard inside a GitLab repo.
type axiHomeRepoOutput struct {
	Bin           string               `json:"bin" toon:"bin"`
	Description   string               `json:"description" toon:"description"`
	Project       string               `json:"project" toon:"project"`
	MergeRequests []axiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// axiHomeUserOutput is the gl-axi no-args dashboard outside a repo.
type axiHomeUserOutput struct {
	Bin         string        `json:"bin" toon:"bin"`
	Description string        `json:"description" toon:"description"`
	User        axiUserOutput `json:"user" toon:"user"`
	Help        []string      `json:"help,omitempty" toon:"help,omitempty"`
}

// axiContextOutput is the compact session-start ambient context printed by
// `gl-axi context` for agent session hooks.
type axiContextOutput struct {
	Project       string               `json:"project" toon:"project"`
	MergeRequests []axiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

type setupTargetOutput struct {
	App    string `json:"app" toon:"app"`
	Path   string `json:"path" toon:"path"`
	Status string `json:"status" toon:"status"`
}

type axiSetupHooksOutput struct {
	Hooks []setupTargetOutput `json:"hooks" toon:"hooks"`
	Help  []string            `json:"help,omitempty" toon:"help,omitempty"`
}

type axiErrorOutput struct {
	Error string   `json:"error" toon:"error"`
	Code  string   `json:"code" toon:"code"`
	Help  []string `json:"help,omitempty" toon:"help,omitempty"`
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

func normalizeOutputFormat(format string, mode commandMode) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return defaultOutputFormat(mode), nil
	}

	switch mode {
	case commandModeAxi:
		if format == "toon" || format == "json" {
			return format, nil
		}
	default:
		if format == "text" || format == "json" {
			return format, nil
		}
	}

	return "", newUsageError(
		fmt.Errorf("unsupported output format %q: use %s", format, outputFormats(mode)),
		fmt.Sprintf("Valid --output values: %s", outputFormats(mode)),
	)
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(v)
}

// writeAxi renders v as TOON (default) or JSON. The trailing newline is a CLI
// convention on top of the TOON document, which itself ends without one.
func writeAxi(w io.Writer, format string, v any) error {
	format, err := normalizeOutputFormat(format, commandModeAxi)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, v)
	}

	encoded, err := toon.MarshalString(v)
	if err != nil {
		return fmt.Errorf("encode toon output: %w", err)
	}
	_, err = fmt.Fprintln(w, encoded)

	return err
}

func writeUser(w io.Writer, format string, mode commandMode, user *gitlab.User) error {
	if user == nil {
		return errors.New("gitlab api returned an empty current user response")
	}

	if mode == commandModeAxi {
		return writeAxi(w, format, axiWhoamiOutput{
			User: axiUserFromAPI(user),
			Help: whoamiHelp(),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	out := userOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		State:    user.State,
		WebURL:   user.WebURL,
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	_, err = fmt.Fprintf(
		w,
		"id: %d\nusername: %s\nname: %s\nstate: %s\nweb_url: %s\n",
		out.ID,
		out.Username,
		out.Name,
		out.State,
		out.WebURL,
	)

	return err
}

func axiUserFromAPI(user *gitlab.User) axiUserOutput {
	return axiUserOutput{
		ID:       user.ID,
		Username: user.Username,
		Name:     user.Name,
		WebURL:   user.WebURL,
	}
}

func whoamiHelp() []string {
	return []string{
		"Run `project info` to inspect the current project",
		"Run `mr` to list open merge requests",
	}
}

func writeAuthLogin(w io.Writer, format string, mode commandMode, result authLoginResult) error {
	if mode == commandModeAxi {
		return writeAxi(w, format, axiAuthLoginOutput{
			Login: result,
			Help: []string{
				fmt.Sprintf("Credential stored for %s — run `whoami` to verify API access", result.Domain),
				"Run `mr` to list open merge requests",
			},
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
	}

	_, err = fmt.Fprintf(
		w,
		"username: %s\ndomain: %s\nbackend: %s\n",
		result.Username,
		result.Domain,
		result.Backend,
	)

	return err
}

func writeAuthLogout(w io.Writer, format string, mode commandMode, result authLogoutResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == commandModeAxi {
		return writeAxi(w, format, axiAuthLogoutOutput{
			Logout: result,
			Help: []string{
				"Run `auth login <token> --gitlab-base-url <url>` to authenticate again",
			},
		})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
	}

	if result.Noop {
		_, err = fmt.Fprintf(w, "domain: %s\nno stored credential (no-op)\n", result.Domain)
		return err
	}

	_, err = fmt.Fprintf(
		w,
		"domain: %s\nremoved_from: %s\n",
		result.Domain,
		strings.Join(result.Backends, ", "),
	)

	return err
}

func writeAuthStatus(w io.Writer, format string, mode commandMode, result authStatusResult) error {
	if result.Backends == nil {
		result.Backends = []string{}
	}

	if mode == commandModeAxi {
		help := []string{"Run `auth login <token> --gitlab-base-url <url>` to store a credential"}
		if result.Authenticated {
			help = []string{"Run `whoami` to verify the stored token still works"}
		}

		return writeAxi(w, format, axiAuthStatusOutput{Status: result, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, result)
	}

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
}

func writeProject(w io.Writer, format string, mode commandMode, project *gitlab.Project) error {
	if project == nil {
		return errors.New("gitlab api returned an empty project response")
	}

	out := projectToOutput(project)

	// A detail view fully answers the query, so the axi variant carries no
	// help suggestions (axi guide §9: omit when self-contained).
	if mode == commandModeAxi {
		return writeAxi(w, format, axiProjectInfoOutput{Project: out})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeProjectText(w, out)
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

func writeMergeRequest(w io.Writer, format string, mode commandMode, mergeRequest *gitlab.MergeRequest, full bool, hints *mrHintContext) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, truncated := mergeRequestToOutput(mergeRequest, full, mode)

	if mode == commandModeAxi {
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
				hints.projectSuffix(),
			)}
		}

		return writeAxi(w, format, axiMergeRequestViewOutput{MergeRequest: view, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeMergeRequestText(w, out, full)
}

// writeMergeRequestCreated renders a freshly created or updated merge
// request. It reuses the view's merge_request shape so agents parse one
// schema, but unlike the self-contained view a mutation has a genuine next
// step, so the axi variant always suggests checking merge status.
func writeMergeRequestCreated(w io.Writer, format string, mode commandMode, mergeRequest *gitlab.MergeRequest, hints *mrHintContext) error {
	if mode != commandModeAxi {
		return writeMergeRequest(w, format, mode, mergeRequest, false, hints)
	}

	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, truncated := mergeRequestToOutput(mergeRequest, false, mode)

	help := []string{fmt.Sprintf(
		"Run `mr view %d%s` to check merge status and pipeline results",
		out.IID,
		hints.projectSuffix(),
	)}
	if truncated {
		help = append(help, fmt.Sprintf(
			"Run `mr view %d --full%s` for the complete description and all fields",
			out.IID,
			hints.projectSuffix(),
		))
	}

	return writeAxi(w, format, axiMergeRequestViewOutput{
		MergeRequest: compactMergeRequestView(out),
		Help:         help,
	})
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

	_, err = fmt.Fprintf(w, "description:\n%s\n", out.Description)

	return err
}

func writeMergeRequestList(w io.Writer, format string, mode commandMode, mergeRequests []*gitlab.BasicMergeRequest, paging mrListPaging, fields []string, hints *mrHintContext) error {
	if mode == commandModeAxi {
		rows := make([]axiMergeRequestRow, 0, len(mergeRequests))
		for _, mergeRequest := range mergeRequests {
			if mergeRequest == nil {
				continue
			}
			rows = append(rows, axiMergeRequestRowFor(mergeRequest, fields))
		}

		return writeAxi(w, format, axiMergeRequestListOutput{
			MergeRequests: rows,
			Count:         mrListCountLine(len(rows), paging),
			Total:         paging.totalItems,
			Page:          paging.page,
			TotalPages:    paging.totalPages,
			Help:          mrListHelp(len(rows), paging, hints),
		})
	}

	format, err := normalizeOutputFormat(format, mode)
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
		return writeJSON(w, mergeRequestListOutput{
			MergeRequests: rows,
			Count:         len(rows),
			Total:         paging.totalItems,
			Page:          paging.page,
			TotalPages:    paging.totalPages,
		})
	}

	return renderMergeRequestTable(w, rows, paging)
}

// mrListCountLine states the definitive result size, including the explicit
// zero (axi guide §5) and the unknown-total case where GitLab omits X-Total.
func mrListCountLine(count int, paging mrListPaging) string {
	if count > 0 && paging.totalItems == 0 {
		return fmt.Sprintf("%d of unknown total", count)
	}

	return fmt.Sprintf("%d of %d total", count, paging.totalItems)
}

func mrListHelp(count int, paging mrListPaging, hints *mrHintContext) []string {
	suffix := hints.projectSuffix()

	if count == 0 {
		return []string{
			fmt.Sprintf("No merge requests matched — run `mr list --state all%s` to include merged and closed ones, or relax other filters", suffix),
		}
	}

	help := []string{fmt.Sprintf("Run `mr view <iid>%s` for details", suffix)}
	switch {
	case paging.totalPages > paging.page:
		help = append(help, fmt.Sprintf("Run `mr list --page %d%s` for the next page", paging.page+1, suffix))
	case paging.totalItems == 0 && hints != nil && int64(count) >= hints.limit:
		help = append(help, fmt.Sprintf("More results may exist — run `mr list --page %d%s`", paging.page+1, suffix))
	}

	return help
}

// mrHintContext carries invocation context into help hints so suggested
// commands stay runnable as-is (axi guide §9: carry disambiguating flags).
type mrHintContext struct {
	project string
	limit   int64
}

func (c *mrHintContext) projectSuffix() string {
	if c == nil || strings.TrimSpace(c.project) == "" {
		return ""
	}

	return " --project " + strings.TrimSpace(c.project)
}

func mergeRequestToOutput(mergeRequest *gitlab.MergeRequest, full bool, mode commandMode) (mergeRequestOutput, bool) {
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
		out.Description, truncated = truncateDescription(out.Description, descriptionTruncateLimit, mode)
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

func axiMergeRequestRowFor(mergeRequest *gitlab.BasicMergeRequest, fields []string) axiMergeRequestRow {
	full := basicMergeRequestToRow(mergeRequest)
	row := axiMergeRequestRow{
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

func usernamesOf(users []*gitlab.BasicUser) []string {
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

// truncateDescription cuts long descriptions at limit runes and appends an
// explicit size marker. The standard-mode marker keeps the inline --full hint
// (text output has no help channel); the axi marker stays bare because the
// escape hatch is suggested through the structured help field.
func truncateDescription(value string, limit int, mode commandMode) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}

	if mode == commandModeAxi {
		return fmt.Sprintf("%s… (truncated, %d chars total)", string(runes[:limit]), len(runes)), true
	}

	return fmt.Sprintf(
		"%s… (truncated, %d chars total — use --full for the complete description)",
		string(runes[:limit]),
		len(runes),
	), true
}

// writeCommandError renders a failed command. In axi mode the error is
// structured output on the same channel and format as normal results so the
// agent can parse and act on it.
func writeCommandError(w io.Writer, mode commandMode, format string, bin string, err error) {
	if mode != commandModeAxi {
		fmt.Fprintln(w, err)
		return
	}

	code, message, help := classifyError(err, bin)
	out := axiErrorOutput{Error: message, Code: code, Help: help}

	normalized, formatErr := normalizeOutputFormat(format, mode)
	if formatErr != nil {
		normalized = defaultOutputFormat(mode)
	}

	if writeErr := writeAxi(w, normalized, out); writeErr != nil {
		fmt.Fprintln(w, err)
	}
}
