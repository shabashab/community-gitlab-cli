package cli

import (
	"errors"
	"fmt"
	"io"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

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
