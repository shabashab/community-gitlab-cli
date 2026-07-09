package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
)

// newMRCreateTestServer stubs the three endpoints mr create can touch and
// captures the POSTed merge request body plus every username lookup.
func newMRCreateTestServer(t *testing.T, defaultBranch string) (*httptest.Server, *map[string]any, *[]string) {
	t.Helper()

	body := map[string]any{}
	var userLookups []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/users":
			username := r.URL.Query().Get("username")
			userLookups = append(userLookups, username)
			switch username {
			case "mona":
				fmt.Fprint(w, `[{"id":7,"username":"mona"}]`)
			case "alice":
				fmt.Fprint(w, `[{"id":8,"username":"alice"}]`)
			default:
				fmt.Fprint(w, "[]")
			}
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			fmt.Fprint(w, mergeRequestJSON(124, "created description"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject" && r.Method == http.MethodGet:
			fmt.Fprintf(w, `{"id":1,"default_branch":%q}`, defaultBranch)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &body, &userLookups
}

func executeMRCreateCommand(t *testing.T, baseURL string, stdin io.Reader, extraArgs ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	args := []string{
		"mr", "create",
		"--gitlab-token", "test-token",
		"--gitlab-base-url", baseURL,
		"--project", "group/project",
		"-o", "json",
	}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func TestMRCreatePostsAllParams(t *testing.T) {
	server, body, _ := newMRCreateTestServer(t, "main")
	defer server.Close()

	out, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--draft",
		"--description", "Adds the search endpoint.",
		"--source-branch", "feature/search",
		"--target-branch", "main",
		"--label", "bug", "--label", "backend",
		"--assignee", "@mona",
		"--reviewer", "alice",
		"--milestone-id", "3",
		"--squash",
		"--remove-source-branch",
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	got := *body
	want := map[string]any{
		"title":                "Draft: Add search endpoint",
		"description":          "Adds the search endpoint.",
		"source_branch":        "feature/search",
		"target_branch":        "main",
		"labels":               "bug,backend",
		"milestone_id":         float64(3),
		"squash":               true,
		"remove_source_branch": true,
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("body[%s] = %v, want %v", key, got[key], value)
		}
	}
	assertIDList(t, got, "assignee_ids", 7)
	assertIDList(t, got, "reviewer_ids", 8)

	if !strings.Contains(out, `"iid": 124`) {
		t.Fatalf("output = %q, want created merge request", out)
	}
}

func TestMRCreateResolvesNumericAndAtPrefixedUsers(t *testing.T) {
	server, body, userLookups := newMRCreateTestServer(t, "main")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
		"--assignee", "42", "--assignee", "@mona",
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	if len(*userLookups) != 1 || (*userLookups)[0] != "mona" {
		t.Fatalf("user lookups = %v, want exactly one for mona", *userLookups)
	}
	assertIDList(t, *body, "assignee_ids", 42, 7)
}

func TestMRCreateUserNotFound(t *testing.T) {
	server, _, _ := newMRCreateTestServer(t, "main")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
		"--reviewer", "ghost",
	)
	if !errors.Is(err, errUserNotFound) {
		t.Fatalf("mr create error = %v, want errUserNotFound", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: user_not_found") {
		t.Fatalf("axi error output = %q, want user_not_found code", out.String())
	}
}

func TestMRCreateDefaultsTargetBranch(t *testing.T) {
	server, body, _ := newMRCreateTestServer(t, "develop")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	if (*body)["target_branch"] != "develop" {
		t.Fatalf("body[target_branch] = %v, want develop from project default branch", (*body)["target_branch"])
	}
}

func TestMRCreateFailsWithoutProjectDefaultBranch(t *testing.T) {
	server, _, _ := newMRCreateTestServer(t, "")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
	)
	if !errors.Is(err, errMissingTargetBranch) {
		t.Fatalf("mr create error = %v, want errMissingTargetBranch", err)
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: missing_target_branch") {
		t.Fatalf("axi error output = %q, want missing_target_branch code", out.String())
	}
}

func TestMRCreateBoolsOmittedWhenUnset(t *testing.T) {
	server, body, _ := newMRCreateTestServer(t, "main")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	for _, key := range []string{"squash", "remove_source_branch", "allow_collaboration", "description", "labels", "assignee_ids", "reviewer_ids", "milestone_id", "target_project_id"} {
		if _, present := (*body)[key]; present {
			t.Fatalf("body contains %s = %v, want the field omitted when unset", key, (*body)[key])
		}
	}
}

func TestMRCreateMissingTitle(t *testing.T) {
	_, err := executeMRCreateCommand(t, "http://localhost:1", nil,
		"--source-branch", "feature/search",
		"--target-branch", "main",
	)
	if err == nil {
		t.Fatal("mr create returned nil error, want missing --title usage error")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Fatalf("mr create error = %v, want message naming --title", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRCreateDescriptionFlagConflict(t *testing.T) {
	_, err := executeMRCreateCommand(t, "http://localhost:1", nil,
		"--title", "Add search endpoint",
		"--description", "inline",
		"--description-file", "notes.md",
	)
	if err == nil {
		t.Fatal("mr create returned nil error, want mutual-exclusion usage error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("mr create error = %v, want mutual-exclusion message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRCreateDescriptionFromFile(t *testing.T) {
	server, body, _ := newMRCreateTestServer(t, "main")
	defer server.Close()

	path := filepath.Join(t.TempDir(), "description.md")
	if err := os.WriteFile(path, []byte("# Body\n\nfrom file\n"), 0o600); err != nil {
		t.Fatalf("write description file: %v", err)
	}

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
		"--description-file", path,
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	if (*body)["description"] != "# Body\n\nfrom file\n" {
		t.Fatalf("body[description] = %q, want file content preserved verbatim", (*body)["description"])
	}
}

func TestMRCreateDescriptionFromStdin(t *testing.T) {
	server, body, _ := newMRCreateTestServer(t, "main")
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, strings.NewReader("body from stdin"),
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
		"--description-file", "-",
	)
	if err != nil {
		t.Fatalf("mr create returned error: %v", err)
	}

	if (*body)["description"] != "body from stdin" {
		t.Fatalf("body[description] = %q, want stdin content", (*body)["description"])
	}
}

func TestMRCreateUnreadableDescriptionFile(t *testing.T) {
	_, err := executeMRCreateCommand(t, "http://localhost:1", nil,
		"--title", "Add search endpoint",
		"--description-file", filepath.Join(t.TempDir(), "missing.md"),
	)
	if err == nil {
		t.Fatal("mr create returned nil error, want file read error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}
}

func TestApplyDraftTitle(t *testing.T) {
	tests := map[string]string{
		"Add search endpoint":        "Draft: Add search endpoint",
		"Draft: Add search endpoint": "Draft: Add search endpoint",
		"draft: add search endpoint": "draft: add search endpoint",
	}
	for title, want := range tests {
		if got := applyDraftTitle(title); got != want {
			t.Fatalf("applyDraftTitle(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestWriteMergeRequestCreatedAxiTOON(t *testing.T) {
	mergeRequest := testMergeRequest(124, "short description")

	var out bytes.Buffer
	hints := &output.MRHintContext{Project: "group/project"}
	if err := output.WriteMergeRequestCreated(&out, "toon", commandModeAxi, mergeRequest, hints); err != nil {
		t.Fatalf("output.WriteMergeRequestCreated returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_request:\n  iid: 124") {
		t.Fatalf("output = %q, want nested merge_request object", got)
	}
	if !strings.Contains(got, `web_url: "https://gitlab.example/group/project/-/merge_requests/124"`) {
		t.Fatalf("output = %q, want web_url in compact view", got)
	}
	if !strings.Contains(got, "help[1]: Run `mr view 124 --project group/project` to check merge status and pipeline results") {
		t.Fatalf("output = %q, want next-step hint carrying --project", got)
	}
}

func TestMRCreateConflictErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Another open merge request already exists for this source branch: !5"}`)
	}))
	defer server.Close()

	_, err := executeMRCreateCommand(t, server.URL, nil,
		"--title", "Add search endpoint",
		"--source-branch", "feature/search",
		"--target-branch", "main",
	)
	if err == nil {
		t.Fatal("mr create returned nil error, want 409 conflict error")
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	got := out.String()
	if !strings.Contains(got, "code: gitlab_conflict") {
		t.Fatalf("axi error output = %q, want gitlab_conflict code", got)
	}
	if strings.Contains(got, server.URL) {
		t.Fatalf("axi error output = %q, want no raw request URL leak", got)
	}
}

func assertIDList(t *testing.T, body map[string]any, key string, want ...float64) {
	t.Helper()

	values, ok := body[key].([]any)
	if !ok {
		t.Fatalf("body[%s] = %v (%T), want a JSON array", key, body[key], body[key])
	}
	if len(values) != len(want) {
		t.Fatalf("body[%s] = %v, want %d entries", key, values, len(want))
	}
	for i, value := range want {
		if values[i] != value {
			t.Fatalf("body[%s][%d] = %v, want %v", key, i, values[i], value)
		}
	}
}
