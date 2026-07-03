package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

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

func writeCommandError(w io.Writer, mode commandMode, err error) {
	if mode != commandModeAxi {
		fmt.Fprintln(w, err)
		return
	}

	code := "command_failed"
	next := "Inspect the error message, fix the input or GitLab configuration, then retry."
	if errors.Is(err, gitlabclient.ErrMissingToken) {
		code = "missing_gitlab_token"
		next = "Set GITLAB_TOKEN or pass --gitlab-token, then retry."
	} else if errors.Is(err, errMissingProject) {
		code = "missing_gitlab_project"
		next = "Run inside a git repository with remote origin configured or pass --project."
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
