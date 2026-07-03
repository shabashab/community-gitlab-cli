package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/spf13/cobra"
)

func TestResolveProjectDiscoversOrigin(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	dir := newGitRepoWithOrigin(t, "https://gitlab.example/group/project.git")
	withWorkingDir(t, dir)

	got, err := resolveProject(&cobra.Command{}, &rootOptions{}, &projectOptions{})
	if err != nil {
		t.Fatalf("resolveProject returned error: %v", err)
	}
	if got.ref != "group/project" {
		t.Fatalf("ref = %q, want group/project", got.ref)
	}
	if got.baseURL != "https://gitlab.example" {
		t.Fatalf("baseURL = %q, want https://gitlab.example", got.baseURL)
	}
}

func TestResolveProjectUsesExplicitProjectAndOriginBaseURL(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	dir := newGitRepoWithOrigin(t, "https://self.gitlab.example/team/repo.git")
	withWorkingDir(t, dir)

	got, err := resolveProject(&cobra.Command{}, &rootOptions{}, &projectOptions{project: "other/project"})
	if err != nil {
		t.Fatalf("resolveProject returned error: %v", err)
	}
	if got.ref != "other/project" {
		t.Fatalf("ref = %q, want other/project", got.ref)
	}
	if got.baseURL != "https://self.gitlab.example" {
		t.Fatalf("baseURL = %q, want https://self.gitlab.example", got.baseURL)
	}
}

func TestResolveProjectDoesNotRequireGitForExplicitProject(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	withWorkingDir(t, t.TempDir())

	got, err := resolveProject(&cobra.Command{}, &rootOptions{}, &projectOptions{project: "group/project"})
	if err != nil {
		t.Fatalf("resolveProject returned error: %v", err)
	}
	if got.ref != "group/project" {
		t.Fatalf("ref = %q, want group/project", got.ref)
	}
	if got.baseURL != "" {
		t.Fatalf("baseURL = %q, want empty fallback", got.baseURL)
	}
}

func TestResolveProjectRequiresProjectContext(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "")
	withWorkingDir(t, t.TempDir())

	_, err := resolveProject(&cobra.Command{}, &rootOptions{}, &projectOptions{})
	if !errors.Is(err, errMissingProject) {
		t.Fatalf("resolveProject error = %v, want errMissingProject", err)
	}
}

func TestResolveProjectSkipsDiscoveryWhenBaseURLConfigured(t *testing.T) {
	t.Setenv(gitlabclient.BaseURLEnv, "https://env.gitlab.example")
	withWorkingDir(t, t.TempDir())

	got, err := resolveProject(&cobra.Command{}, &rootOptions{}, &projectOptions{project: "group/project"})
	if err != nil {
		t.Fatalf("resolveProject returned error: %v", err)
	}
	if got.ref != "group/project" {
		t.Fatalf("ref = %q, want group/project", got.ref)
	}
	if got.baseURL != "" {
		t.Fatalf("baseURL = %q, want empty discovered base URL", got.baseURL)
	}
}

func TestRunProjectInfoUsesNamespacePath(t *testing.T) {
	var gotPath string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotToken = r.Header.Get("Private-Token")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, projectJSON(42))
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runProjectInfo(cmd, &rootOptions{
		gitlabToken:   "test-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	}, &projectOptions{project: "group/project"})
	if err != nil {
		t.Fatalf("runProjectInfo returned error: %v", err)
	}
	if gotPath != "/api/v4/projects/group%2Fproject" {
		t.Fatalf("request path = %q, want /api/v4/projects/group%%2Fproject", gotPath)
	}
	if gotToken != "test-token" {
		t.Fatalf("Private-Token header = %q, want test-token", gotToken)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"path_with_namespace": "group/project"`)) {
		t.Fatalf("runProjectInfo output = %q, want path_with_namespace fragment", out.String())
	}
}

func TestRunProjectInfoUsesNumericProject(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, projectJSON(42))
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := runProjectInfo(cmd, &rootOptions{
		gitlabToken:   "test-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	}, &projectOptions{project: "42"})
	if err != nil {
		t.Fatalf("runProjectInfo returned error: %v", err)
	}
	if gotPath != "/api/v4/projects/42" {
		t.Fatalf("request path = %q, want /api/v4/projects/42", gotPath)
	}
}

func projectJSON(id int64) string {
	return fmt.Sprintf(`{
		"id": %d,
		"name": "project",
		"name_with_namespace": "Group / Project",
		"path": "project",
		"path_with_namespace": "group/project",
		"description": "test project",
		"default_branch": "main",
		"visibility": "private",
		"web_url": "https://gitlab.example/group/project",
		"ssh_url_to_repo": "git@gitlab.example:group/project.git",
		"http_url_to_repo": "https://gitlab.example/group/project.git",
		"archived": false,
		"empty_repo": false,
		"open_issues_count": 3,
		"star_count": 7,
		"forks_count": 2,
		"last_activity_at": "2026-07-03T12:00:00Z",
		"namespace": {
			"id": 10,
			"name": "Group",
			"path": "group",
			"kind": "group",
			"full_path": "group",
			"web_url": "https://gitlab.example/groups/group"
		}
	}`, id)
}

func newGitRepoWithOrigin(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is not available")
	}

	dir := t.TempDir()
	runGitCommand(t, dir, "init")
	runGitCommand(t, dir, "remote", "add", "origin", origin)
	return dir
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}
