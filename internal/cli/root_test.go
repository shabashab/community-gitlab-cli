package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	standard, _ := newRootCommand("gl", "short", "long", commandModeStandard)
	standardOutput := standard.PersistentFlags().Lookup("output")
	if standardOutput.DefValue != "text" {
		t.Fatalf("standard output default = %q, want text", standardOutput.DefValue)
	}

	axi, _ := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	axiOutput := axi.PersistentFlags().Lookup("output")
	if axiOutput.DefValue != "toon" {
		t.Fatalf("axi output default = %q, want toon", axiOutput.DefValue)
	}
}

func TestRootCommandRejectsInvalidOutputBeforeRunning(t *testing.T) {
	cmd, _ := newRootCommand("gl", "short", "long", commandModeStandard)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"whoami", "-o", "yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want unsupported format usage error")
	}
	if !strings.Contains(err.Error(), `unsupported output format "yaml"`) {
		t.Fatalf("Execute error = %v, want unsupported format message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestRootCommandUnknownFlagListsValidFlags(t *testing.T) {
	cmd, opts := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"mr", "list", "--stat", "closed"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want unknown flag error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for unknown flag", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, opts.output, opts.binName, err)
	got := out.String()
	if !strings.Contains(got, "unknown flag: --stat") {
		t.Fatalf("error output = %q, want unknown flag message", got)
	}
	if !strings.Contains(got, "--state") || !strings.Contains(got, "--help always allowed") {
		t.Fatalf("error output = %q, want inline valid-flag list", got)
	}
}

func TestRootCommandRejectsUnknownCommand(t *testing.T) {
	cmd, _ := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want unknown command error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for unknown command", exitCodeForError(err))
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
		"user:\n  id: 42",
		"username: octocat",
		"name: Mona Lisa",
		"Run `project info` to inspect the current project",
		"Run `mr` to list open merge requests",
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
		`"help": [`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser AXI JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteCommandErrorSupportsAxi(t *testing.T) {
	var out bytes.Buffer

	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", fmt.Errorf("%w: set GITLAB_TOKEN", gitlabclient.ErrMissingToken))

	for _, fragment := range []string{
		"error: ",
		"code: missing_gitlab_token",
		"help[1]: ",
		"Set GITLAB_TOKEN",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeCommandError AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteCommandErrorHonorsJSONFormat(t *testing.T) {
	var out bytes.Buffer

	writeCommandError(&out, commandModeAxi, "json", "gl-axi", fmt.Errorf("%w: set GITLAB_TOKEN", gitlabclient.ErrMissingToken))

	for _, fragment := range []string{
		`"error": `,
		`"code": "missing_gitlab_token"`,
		`"help": [`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeCommandError JSON output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteCommandErrorTranslatesAPIErrors(t *testing.T) {
	respErr := &gitlab.ErrorResponse{StatusCode: 401, Message: "401 Unauthorized"}
	err := fmt.Errorf("get current GitLab user: %w", respErr)

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)

	got := out.String()
	if !strings.Contains(got, "code: gitlab_auth_failed") {
		t.Fatalf("output = %q, want gitlab_auth_failed code", got)
	}
	if !strings.Contains(got, "GitLab rejected the token (401 Unauthorized)") {
		t.Fatalf("output = %q, want translated message", got)
	}
	if strings.Contains(got, "/api/v4/") || strings.Contains(got, "GET http") {
		t.Fatalf("output = %q, want no raw request URL leak", got)
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
		"project:\n  id: 42",
		"path_with_namespace: group/project",
		"namespace:\n    id: 10",
		"full_path: group",
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeProject AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
	// A detail view is self-contained: no help hints expected.
	if strings.Contains(out.String(), "help[") {
		t.Fatalf("writeProject AXI output = %q, want no help hints", out.String())
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

func TestRunAxiHomeOutsideRepoShowsUser(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	t.Setenv(gitlabclient.TokenEnv, "")
	t.Setenv(gitlabclient.AlternateTokenEnv, "")
	withWorkingDir(t, t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Errorf("request path = %q, want /api/v4/user", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":42,"username":"octocat","name":"Mona Lisa","state":"active","web_url":"https://gitlab.example/octocat"}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd, opts := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	cmd.SetOut(&out)
	opts.gitlabToken = "test-token"
	opts.gitlabBaseURL = server.URL

	if err := runAxiHome(cmd, opts); err != nil {
		t.Fatalf("runAxiHome returned error: %v", err)
	}

	got := out.String()
	for _, fragment := range []string{
		"bin: ",
		"description: ",
		"username: octocat",
		"help[",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("home output = %q, want fragment %q", got, fragment)
		}
	}
}

func TestRunAxiHomeInRepoShowsMergeRequests(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	t.Setenv(gitlabclient.TokenEnv, "")
	t.Setenv(gitlabclient.AlternateTokenEnv, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.EscapedPath(), "merge_requests") {
			t.Errorf("request path = %q, want merge request list path", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Total", "1")
		w.Header().Set("X-Total-Pages", "1")
		w.Header().Set("X-Page", "1")
		fmt.Fprint(w, `[{"iid":7,"title":"Fix bug","state":"opened","author":{"username":"octocat"}}]`)
	}))
	defer server.Close()

	dir := newGitRepoWithOrigin(t, server.URL+"/group/project.git")
	withWorkingDir(t, dir)

	var out bytes.Buffer
	cmd, opts := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	cmd.SetOut(&out)
	opts.gitlabToken = "test-token"

	if err := runAxiHome(cmd, opts); err != nil {
		t.Fatalf("runAxiHome returned error: %v", err)
	}

	got := out.String()
	for _, fragment := range []string{
		"bin: ",
		"description: ",
		"project: group/project",
		"merge_requests[1]{iid,title,state,author}:",
		"7,Fix bug,opened,octocat",
		"count: 1 of 1 total open",
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("home output = %q, want fragment %q", got, fragment)
		}
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
