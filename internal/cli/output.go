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

type mergeRequestApprovalUserOutput struct {
	Username   string `json:"username" toon:"username"`
	ApprovedAt string `json:"approved_at,omitempty" toon:"approved_at,omitempty"`
}

type mergeRequestApprovalCompactOutput struct {
	IID               int64                            `json:"iid,omitempty" toon:"iid,omitempty"`
	Approved          bool                             `json:"approved" toon:"approved"`
	ApprovalsRequired int64                            `json:"approvals_required" toon:"approvals_required"`
	ApprovalsLeft     int64                            `json:"approvals_left" toon:"approvals_left"`
	UserHasApproved   bool                             `json:"user_has_approved" toon:"user_has_approved"`
	UserCanApprove    bool                             `json:"user_can_approve" toon:"user_can_approve"`
	ApprovedBy        []mergeRequestApprovalUserOutput `json:"approved_by" toon:"approved_by"`
}

type mergeRequestApprovalRuleOutput struct {
	ID                int64    `json:"id" toon:"id"`
	Name              string   `json:"name" toon:"name"`
	RuleType          string   `json:"rule_type" toon:"rule_type"`
	ApprovalsRequired int64    `json:"approvals_required" toon:"approvals_required"`
	Approved          bool     `json:"approved" toon:"approved"`
	ApprovedBy        []string `json:"approved_by" toon:"approved_by"`
}

type mergeRequestApprovalFullOutput struct {
	IID                            int64                            `json:"iid" toon:"iid"`
	Title                          string                           `json:"title" toon:"title"`
	State                          string                           `json:"state" toon:"state"`
	MergeStatus                    string                           `json:"merge_status" toon:"merge_status"`
	Approved                       bool                             `json:"approved" toon:"approved"`
	ApprovalsBeforeMerge           int64                            `json:"approvals_before_merge" toon:"approvals_before_merge"`
	ApprovalsRequired              int64                            `json:"approvals_required" toon:"approvals_required"`
	ApprovalsLeft                  int64                            `json:"approvals_left" toon:"approvals_left"`
	RequirePasswordToApprove       bool                             `json:"require_password_to_approve" toon:"require_password_to_approve"`
	UserHasApproved                bool                             `json:"user_has_approved" toon:"user_has_approved"`
	UserCanApprove                 bool                             `json:"user_can_approve" toon:"user_can_approve"`
	ApprovedBy                     []mergeRequestApprovalUserOutput `json:"approved_by" toon:"approved_by"`
	SuggestedApprovers             []string                         `json:"suggested_approvers" toon:"suggested_approvers"`
	Approvers                      []mergeRequestApprovalUserOutput `json:"approvers" toon:"approvers"`
	ApproverGroups                 []string                         `json:"approver_groups" toon:"approver_groups"`
	ApprovalRulesLeft              []mergeRequestApprovalRuleOutput `json:"approval_rules_left" toon:"approval_rules_left"`
	HasApprovalRules               bool                             `json:"has_approval_rules" toon:"has_approval_rules"`
	MergeRequestApproversAvailable bool                             `json:"merge_request_approvers_available" toon:"merge_request_approvers_available"`
	MultipleApprovalRulesAvailable bool                             `json:"multiple_approval_rules_available" toon:"multiple_approval_rules_available"`
}

type axiMergeRequestApprovalOutput struct {
	Approval any      `json:"approval" toon:"approval"`
	Help     []string `json:"help,omitempty" toon:"help,omitempty"`
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

func encodeTOON(v any) (string, error) {
	encoded, err := toon.MarshalString(v)
	if err != nil {
		return "", fmt.Errorf("encode toon output: %w", err)
	}

	return encoded + "\n", nil
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
	return writeMergeRequestWithApprovals(w, format, mode, mergeRequest, nil, full, hints)
}

func writeMergeRequestWithApprovals(w io.Writer, format string, mode commandMode, mergeRequest *gitlab.MergeRequest, approvals *gitlab.MergeRequestApprovals, full bool, hints *mrHintContext) error {
	if mergeRequest == nil {
		return errors.New("gitlab api returned an empty merge request response")
	}

	out, truncated := mergeRequestToOutput(mergeRequest, full, mode)
	if approvals != nil {
		approval := mergeRequestApprovalCompactFromAPI(approvals)
		approval.IID = 0
		out.Approval = &approval
	}

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

func writeMergeRequestApproval(w io.Writer, format string, mode commandMode, approvals *gitlab.MergeRequestApprovals, full bool, help []string) error {
	if approvals == nil {
		return errors.New("gitlab api returned an empty merge request approvals response")
	}

	if mode == commandModeAxi {
		var out any = mergeRequestApprovalCompactFromAPI(approvals)
		if full {
			out = mergeRequestApprovalFullFromAPI(approvals)
		}

		return writeAxi(w, format, axiMergeRequestApprovalOutput{Approval: out, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if full {
		out := mergeRequestApprovalFullFromAPI(approvals)
		if format == "json" {
			return writeJSON(w, out)
		}

		return writeMergeRequestApprovalFullText(w, out)
	}

	out := mergeRequestApprovalCompactFromAPI(approvals)
	if format == "json" {
		return writeJSON(w, out)
	}

	return writeMergeRequestApprovalCompactText(w, out)
}

func writeMergeRequestApprovalCompactText(w io.Writer, out mergeRequestApprovalCompactOutput) error {
	if out.IID > 0 {
		if _, err := fmt.Fprintf(w, "iid: %d\n", out.IID); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(
		w,
		"approved: %t\napprovals_required: %d\napprovals_left: %d\nuser_has_approved: %t\nuser_can_approve: %t\napproved_by: %s\n",
		out.Approved,
		out.ApprovalsRequired,
		out.ApprovalsLeft,
		out.UserHasApproved,
		out.UserCanApprove,
		strings.Join(approvalUserText(out.ApprovedBy), ", "),
	)

	return err
}

func writeMergeRequestApprovalFullText(w io.Writer, out mergeRequestApprovalFullOutput) error {
	if _, err := fmt.Fprintf(
		w,
		"iid: %d\ntitle: %s\nstate: %s\nmerge_status: %s\napproved: %t\napprovals_before_merge: %d\napprovals_required: %d\napprovals_left: %d\nrequire_password_to_approve: %t\nuser_has_approved: %t\nuser_can_approve: %t\napproved_by: %s\nsuggested_approvers: %s\napprovers: %s\napprover_groups: %s\nhas_approval_rules: %t\nmerge_request_approvers_available: %t\nmultiple_approval_rules_available: %t\n",
		out.IID,
		out.Title,
		out.State,
		out.MergeStatus,
		out.Approved,
		out.ApprovalsBeforeMerge,
		out.ApprovalsRequired,
		out.ApprovalsLeft,
		out.RequirePasswordToApprove,
		out.UserHasApproved,
		out.UserCanApprove,
		strings.Join(approvalUserText(out.ApprovedBy), ", "),
		strings.Join(out.SuggestedApprovers, ", "),
		strings.Join(approvalUserText(out.Approvers), ", "),
		strings.Join(out.ApproverGroups, ", "),
		out.HasApprovalRules,
		out.MergeRequestApproversAvailable,
		out.MultipleApprovalRulesAvailable,
	); err != nil {
		return err
	}

	if len(out.ApprovalRulesLeft) == 0 {
		_, err := fmt.Fprintln(w, "approval_rules_left: none")
		return err
	}

	if _, err := fmt.Fprintln(w, "approval_rules_left:"); err != nil {
		return err
	}
	for _, rule := range out.ApprovalRulesLeft {
		if _, err := fmt.Fprintf(
			w,
			"- id=%d name=%q rule_type=%s approvals_required=%d approved=%t approved_by=%s\n",
			rule.ID,
			rule.Name,
			rule.RuleType,
			rule.ApprovalsRequired,
			rule.Approved,
			strings.Join(rule.ApprovedBy, ","),
		); err != nil {
			return err
		}
	}

	return nil
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

func mergeRequestApprovalCompactFromAPI(approvals *gitlab.MergeRequestApprovals) mergeRequestApprovalCompactOutput {
	out := mergeRequestApprovalCompactOutput{
		IID:               approvals.IID,
		Approved:          approvals.Approved,
		ApprovalsRequired: approvals.ApprovalsRequired,
		ApprovalsLeft:     approvals.ApprovalsLeft,
		UserHasApproved:   approvals.UserHasApproved,
		UserCanApprove:    approvals.UserCanApprove,
		ApprovedBy:        approvedByUsers(approvals.ApprovedBy),
	}
	if out.ApprovedBy == nil {
		out.ApprovedBy = []mergeRequestApprovalUserOutput{}
	}

	return out
}

func mergeRequestApprovalFullFromAPI(approvals *gitlab.MergeRequestApprovals) mergeRequestApprovalFullOutput {
	out := mergeRequestApprovalFullOutput{
		IID:                            approvals.IID,
		Title:                          approvals.Title,
		State:                          approvals.State,
		MergeStatus:                    approvals.MergeStatus,
		Approved:                       approvals.Approved,
		ApprovalsBeforeMerge:           approvals.ApprovalsBeforeMerge,
		ApprovalsRequired:              approvals.ApprovalsRequired,
		ApprovalsLeft:                  approvals.ApprovalsLeft,
		RequirePasswordToApprove:       approvals.RequirePasswordToApprove,
		UserHasApproved:                approvals.UserHasApproved,
		UserCanApprove:                 approvals.UserCanApprove,
		ApprovedBy:                     approvedByUsers(approvals.ApprovedBy),
		SuggestedApprovers:             basicUsernames(approvals.SuggestedApprovers),
		Approvers:                      approvedByUsers(approvals.Approvers),
		ApproverGroups:                 approverGroupNames(approvals.ApproverGroups),
		ApprovalRulesLeft:              approvalRuleOutputs(approvals.ApprovalRulesLeft),
		HasApprovalRules:               approvals.HasApprovalRules,
		MergeRequestApproversAvailable: approvals.MergeRequestApproversAvailable,
		MultipleApprovalRulesAvailable: approvals.MultipleApprovalRulesAvailable,
	}
	if out.ApprovedBy == nil {
		out.ApprovedBy = []mergeRequestApprovalUserOutput{}
	}
	if out.SuggestedApprovers == nil {
		out.SuggestedApprovers = []string{}
	}
	if out.Approvers == nil {
		out.Approvers = []mergeRequestApprovalUserOutput{}
	}
	if out.ApproverGroups == nil {
		out.ApproverGroups = []string{}
	}
	if out.ApprovalRulesLeft == nil {
		out.ApprovalRulesLeft = []mergeRequestApprovalRuleOutput{}
	}

	return out
}

func approvedByUsers(users []*gitlab.MergeRequestApproverUser) []mergeRequestApprovalUserOutput {
	out := make([]mergeRequestApprovalUserOutput, 0, len(users))
	for _, user := range users {
		if user == nil || user.User == nil {
			continue
		}
		out = append(out, mergeRequestApprovalUserOutput{
			Username:   basicUsername(user.User),
			ApprovedAt: formatTimeValue(user.ApprovedAt),
		})
	}

	return out
}

func basicUsernames(users []*gitlab.BasicUser) []string {
	names := make([]string, 0, len(users))
	for _, user := range users {
		name := basicUsername(user)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	return names
}

func basicUsername(user *gitlab.BasicUser) string {
	if user == nil {
		return ""
	}
	if user.Username != "" {
		return user.Username
	}

	return user.Name
}

func approverGroupNames(groups []*gitlab.MergeRequestApproverGroup) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}

		nested := group.Group
		name := nested.FullPath
		if name == "" {
			name = nested.FullName
		}
		if name == "" {
			name = nested.Path
		}
		if name == "" {
			name = nested.Name
		}
		if name != "" {
			names = append(names, name)
		}
	}

	return names
}

func approvalRuleOutputs(rules []*gitlab.MergeRequestApprovalRule) []mergeRequestApprovalRuleOutput {
	out := make([]mergeRequestApprovalRuleOutput, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		approvedBy := basicUsernames(rule.ApprovedBy)
		if approvedBy == nil {
			approvedBy = []string{}
		}
		out = append(out, mergeRequestApprovalRuleOutput{
			ID:                rule.ID,
			Name:              rule.Name,
			RuleType:          rule.RuleType,
			ApprovalsRequired: rule.ApprovalsRequired,
			Approved:          rule.Approved,
			ApprovedBy:        approvedBy,
		})
	}

	return out
}

func approvalUserText(users []mergeRequestApprovalUserOutput) []string {
	out := make([]string, 0, len(users))
	for _, user := range users {
		if user.Username == "" {
			continue
		}
		if user.ApprovedAt != "" {
			out = append(out, fmt.Sprintf("%s (%s)", user.Username, user.ApprovedAt))
			continue
		}
		out = append(out, user.Username)
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
	if discussion == nil {
		return errors.New("gitlab api returned an empty discussion response")
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

	detail := discussionDetailOutput{
		ID:    strings.ToLower(discussion.ID),
		State: "none",
		Notes: len(notes),
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

	out := discussionViewOutput{Discussion: detail, Notes: notes}

	if mode == commandModeAxi {
		return writeAxi(w, format, out)
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeDiscussionText(w, out)
}

func writeDiscussionText(w io.Writer, out discussionViewOutput) error {
	if _, err := fmt.Fprintf(w, "discussion: %s\nstate: %s\n", out.Discussion.ID, out.Discussion.State); err != nil {
		return err
	}
	if out.Discussion.File != "" {
		if _, err := fmt.Fprintf(w, "file: %s:%d\n", out.Discussion.File, out.Discussion.Line); err != nil {
			return err
		}
	}
	if out.Discussion.ResolvedBy != "" || out.Discussion.ResolvedAt != "" {
		if _, err := fmt.Fprintf(w, "resolved_by: %s\nresolved_at: %s\n", out.Discussion.ResolvedBy, out.Discussion.ResolvedAt); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "updated_at: %s\nnotes: %d\n", out.Discussion.UpdatedAt, out.Discussion.Notes); err != nil {
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

// formatLocalTime renders a locally computed time value, treating the zero
// time as absent (unlike formatTimeValue it takes a value, not a pointer).
func formatLocalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.Format("2006-01-02T15:04:05Z07:00")
}

// commentCreatedOutput is the compact created-comment view. There is no body
// echo — the caller knows what it wrote; File/Line come from the response
// position so agents see what GitLab actually anchored.
type commentCreatedOutput struct {
	DiscussionID string `json:"discussion_id,omitempty" toon:"discussion_id,omitempty"`
	NoteID       int64  `json:"note_id" toon:"note_id"`
	Author       string `json:"author" toon:"author"`
	Type         string `json:"type" toon:"type"`
	Resolvable   bool   `json:"resolvable" toon:"resolvable"`
	File         string `json:"file,omitempty" toon:"file,omitempty"`
	Line         int64  `json:"line,omitempty" toon:"line,omitempty"`
	CreatedAt    string `json:"created_at" toon:"created_at"`
}

type axiCommentCreatedOutput struct {
	Comment commentCreatedOutput `json:"comment" toon:"comment"`
	Help    []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// commentCreatedFromNote builds the created-comment view from the response
// note. discussionID is empty for plain notes created via the notes API.
func commentCreatedFromNote(discussionID string, note *gitlab.Note) commentCreatedOutput {
	out := commentCreatedOutput{
		DiscussionID: strings.ToLower(discussionID),
		NoteID:       note.ID,
		Author:       note.Author.Username,
		Type:         string(note.Type),
		Resolvable:   note.Resolvable,
		CreatedAt:    formatTimeValue(note.CreatedAt),
	}
	if out.Type == "" {
		out.Type = string(gitlab.GenericNote)
	}
	if position := note.Position; position != nil {
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

func writeCommentCreated(w io.Writer, format string, mode commandMode, out commentCreatedOutput, iid int64, positionRequested bool, hints *mrHintContext) error {
	if mode == commandModeAxi {
		axiOut := out
		axiOut.DiscussionID = shortDiscussionID(out.DiscussionID)

		var help []string
		if axiOut.DiscussionID != "" {
			help = append(help, fmt.Sprintf(
				"Run `%s %d %s%s` for the full thread",
				mrDiscussionViewCommandName,
				iid,
				axiOut.DiscussionID,
				hints.projectSuffix(),
			))
		} else {
			help = append(help, fmt.Sprintf(
				"Run `mr discussions %d --state all%s` to list comments on the merge request",
				iid,
				hints.projectSuffix(),
			))
		}
		if downgraded := commentPositionDowngradeHint(out, iid, positionRequested, hints); downgraded != "" {
			help = append(help, downgraded)
		}

		return writeAxi(w, format, axiCommentCreatedOutput{Comment: axiOut, Help: help})
	}

	format, err := normalizeOutputFormat(format, mode)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, out)
	}

	return writeCommentCreatedText(w, out)
}

// commentPositionDowngradeHint surfaces GitLab's silent position drop: the
// API can answer 201 yet attach the comment to the merge request instead of
// the requested diff line. The mutation succeeded, so this stays a hint —
// never an error an agent would retry into a duplicate.
func commentPositionDowngradeHint(out commentCreatedOutput, iid int64, positionRequested bool, hints *mrHintContext) string {
	if !positionRequested || out.Type == string(gitlab.DiffNote) {
		return ""
	}

	return fmt.Sprintf(
		"GitLab did not anchor the comment to the requested diff position (type %s) — run `%s %d %s%s` to verify",
		out.Type,
		mrDiscussionViewCommandName,
		iid,
		shortDiscussionID(out.DiscussionID),
		hints.projectSuffix(),
	)
}

func writeCommentCreatedText(w io.Writer, out commentCreatedOutput) error {
	if out.DiscussionID != "" {
		if _, err := fmt.Fprintf(w, "discussion: %s\n", out.DiscussionID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(
		w,
		"note: %d\nauthor: %s\ntype: %s\nresolvable: %t\n",
		out.NoteID,
		out.Author,
		out.Type,
		out.Resolvable,
	); err != nil {
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
	_, err := fmt.Fprintf(w, "created_at: %s\n", out.CreatedAt)

	return err
}

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
