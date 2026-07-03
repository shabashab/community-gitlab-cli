package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestWriteUserSupportsText(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "text", commandModeStandard, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	want := "id: 42\nusername: octocat\nname: Mona Lisa\nstate: active\nweb_url: https://gitlab.example/octocat\n"
	if out.String() != want {
		t.Fatalf("writeUser output = %q, want %q", out.String(), want)
	}
}

func TestRootCommandUsesModeOutputDefaults(t *testing.T) {
	standard := newRootCommand("gl", "short", "long", commandModeStandard)
	standardOutput := standard.PersistentFlags().Lookup("output")
	if standardOutput.DefValue != "text" {
		t.Fatalf("standard output default = %q, want text", standardOutput.DefValue)
	}

	axi := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	axiOutput := axi.PersistentFlags().Lookup("output")
	if axiOutput.DefValue != "toon" {
		t.Fatalf("axi output default = %q, want toon", axiOutput.DefValue)
	}
}

func TestWriteUserSupportsJSON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "json", commandModeStandard, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		`"id": 42`,
		`"username": "octocat"`,
		`"web_url": "https://gitlab.example/octocat"`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteUserRejectsUnknownOutputFormat(t *testing.T) {
	err := writeUser(&bytes.Buffer{}, "yaml", commandModeStandard, &gitlab.User{})
	if err == nil {
		t.Fatal("writeUser returned nil error, want unsupported format error")
	}
}

func TestWriteUserRejectsNilUser(t *testing.T) {
	err := writeUser(&bytes.Buffer{}, "text", commandModeStandard, nil)
	if err == nil {
		t.Fatal("writeUser returned nil error, want nil user error")
	}
}

func TestWriteUserSupportsAxiTOON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "toon", commandModeAxi, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		"user{id,username,name,web_url}:",
		`42,octocat,"Mona Lisa","https://gitlab.example/octocat"`,
		`next: "Use project list when available to inspect accessible projects."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteUserSupportsAxiJSON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "json", commandModeAxi, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		`"user": {`,
		`"username": "octocat"`,
		`"next": "Use project list when available to inspect accessible projects."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser AXI JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteCommandErrorSupportsAxi(t *testing.T) {
	var out bytes.Buffer

	writeCommandError(&out, commandModeAxi, fmt.Errorf("%w: set GITLAB_TOKEN", gitlabclient.ErrMissingToken))

	for _, fragment := range []string{
		"error{code,message}:",
		"missing_gitlab_token",
		`next: "Set GITLAB_TOKEN, pass --gitlab-token, or run auth login <token> --gitlab-base-url <url>, then retry."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeCommandError AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteProjectSupportsText(t *testing.T) {
	var out bytes.Buffer

	if err := writeProject(&out, "text", commandModeStandard, testProject()); err != nil {
		t.Fatalf("writeProject returned error: %v", err)
	}

	for _, fragment := range []string{
		"id: 42",
		"name_with_namespace: Group / Project",
		"path_with_namespace: group/project",
		"namespace_full_path: group",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeProject text = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteProjectSupportsJSON(t *testing.T) {
	var out bytes.Buffer

	if err := writeProject(&out, "json", commandModeStandard, testProject()); err != nil {
		t.Fatalf("writeProject returned error: %v", err)
	}

	for _, fragment := range []string{
		`"id": 42`,
		`"path_with_namespace": "group/project"`,
		`"last_activity_at": "2026-07-03T12:00:00Z"`,
		`"namespace": {`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeProject JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteProjectSupportsAxiTOON(t *testing.T) {
	var out bytes.Buffer

	if err := writeProject(&out, "toon", commandModeAxi, testProject()); err != nil {
		t.Fatalf("writeProject returned error: %v", err)
	}

	for _, fragment := range []string{
		"project{id,name,name_with_namespace,path,path_with_namespace",
		`42,project,"Group / Project",project,group/project`,
		"namespace{id,name,path,kind,full_path,web_url}:",
		`next: "Use --project to inspect another project, or run inside a GitLab repository with origin configured."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeProject AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteProjectSupportsAxiJSON(t *testing.T) {
	var out bytes.Buffer

	if err := writeProject(&out, "json", commandModeAxi, testProject()); err != nil {
		t.Fatalf("writeProject returned error: %v", err)
	}

	for _, fragment := range []string{
		`"project": {`,
		`"path_with_namespace": "group/project"`,
		`"next": "Use --project to inspect another project, or run inside a GitLab repository with origin configured."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeProject AXI JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteProjectRejectsUnknownOutputFormat(t *testing.T) {
	err := writeProject(&bytes.Buffer{}, "yaml", commandModeStandard, &gitlab.Project{})
	if err == nil {
		t.Fatal("writeProject returned nil error, want unsupported format error")
	}
}

func TestWriteProjectRejectsNilProject(t *testing.T) {
	err := writeProject(&bytes.Buffer{}, "text", commandModeStandard, nil)
	if err == nil {
		t.Fatal("writeProject returned nil error, want nil project error")
	}
}

func testProject() *gitlab.Project {
	lastActivityAt := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	return &gitlab.Project{
		ID:                42,
		Name:              "project",
		NameWithNamespace: "Group / Project",
		Path:              "project",
		PathWithNamespace: "group/project",
		Description:       "test project",
		DefaultBranch:     "main",
		Visibility:        gitlab.PrivateVisibility,
		WebURL:            "https://gitlab.example/group/project",
		SSHURLToRepo:      "git@gitlab.example:group/project.git",
		HTTPURLToRepo:     "https://gitlab.example/group/project.git",
		OpenIssuesCount:   3,
		StarCount:         7,
		ForksCount:        2,
		LastActivityAt:    &lastActivityAt,
		Namespace: &gitlab.ProjectNamespace{
			ID:       10,
			Name:     "Group",
			Path:     "group",
			Kind:     "group",
			FullPath: "group",
			WebURL:   "https://gitlab.example/groups/group",
		},
	}
}
