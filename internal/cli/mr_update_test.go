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
)

// newMRUpdateTestServer stubs the endpoints mr update can touch: the PUT
// itself (body captured), the GET used by --draft/--ready to fetch the
// current title (calls counted), and username lookups.
func newMRUpdateTestServer(t *testing.T, currentTitle string) (*httptest.Server, *map[string]any, *int) {
	t.Helper()

	body := map[string]any{}
	getCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/users":
			switch r.URL.Query().Get("username") {
			case "mona":
				fmt.Fprint(w, `[{"id":7,"username":"mona"}]`)
			case "alice":
				fmt.Fprint(w, `[{"id":8,"username":"alice"}]`)
			default:
				fmt.Fprint(w, "[]")
			}
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode update body: %v", err)
			}
			fmt.Fprint(w, mergeRequestJSON(123, "updated description"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			getCalls++
			fmt.Fprintf(w, `{"id":1123,"iid":123,"title":%q}`, currentTitle)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &body, &getCalls
}

func executeMRUpdateCommand(t *testing.T, baseURL string, stdin io.Reader, extraArgs ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	args := []string{
		"mr", "update", "123",
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

func TestMRUpdatePutsAllParams(t *testing.T) {
	server, body, getCalls := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	out, err := executeMRUpdateCommand(t, server.URL, nil,
		"--title", "Rework search endpoint",
		"--description", "Updated description.",
		"--target-branch", "develop",
		"--label", "bug", "--label", "backend",
		"--assignee", "@mona",
		"--reviewer", "alice",
		"--milestone-id", "3",
		"--squash",
		"--remove-source-branch",
		"--allow-collaboration",
		"--discussion-locked",
	)
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	got := *body
	want := map[string]any{
		"title":                "Rework search endpoint",
		"description":          "Updated description.",
		"target_branch":        "develop",
		"labels":               "bug,backend",
		"milestone_id":         float64(3),
		"squash":               true,
		"remove_source_branch": true,
		"allow_collaboration":  true,
		"discussion_locked":    true,
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("body[%s] = %v, want %v", key, got[key], value)
		}
	}
	assertIDList(t, got, "assignee_ids", 7)
	assertIDList(t, got, "reviewer_ids", 8)

	if *getCalls != 0 {
		t.Fatalf("GET merge request calls = %d, want 0 when --title is passed", *getCalls)
	}
	if !strings.Contains(out, `"iid": 123`) {
		t.Fatalf("output = %q, want updated merge request", out)
	}
}

func TestMRUpdateOmitsUnsetFields(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--title", "New title")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["title"] != "New title" {
		t.Fatalf("body[title] = %v, want New title", (*body)["title"])
	}
	omitted := []string{
		"description", "target_branch", "labels", "add_labels", "remove_labels",
		"assignee_ids", "reviewer_ids", "milestone_id", "state_event",
		"squash", "remove_source_branch", "allow_collaboration", "discussion_locked",
	}
	for _, key := range omitted {
		if _, present := (*body)[key]; present {
			t.Fatalf("body contains %s = %v, want the field omitted when unset", key, (*body)[key])
		}
	}
}

func TestMRUpdateNoFlagsUsageError(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil)
	if !errors.Is(err, errNoUpdateFlags) {
		t.Fatalf("mr update error = %v, want errNoUpdateFlags", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: no_update_flags") {
		t.Fatalf("axi error output = %q, want no_update_flags code", out.String())
	}
}

func TestMRUpdateExplicitClears(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil,
		"--description", "",
		"--assignee", "",
		"--reviewer", "",
		"--label", "",
		"--milestone-id", "0",
	)
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	got := *body
	if value, present := got["description"]; !present || value != "" {
		t.Fatalf("body[description] = %v (present=%t), want empty string sent", value, present)
	}
	if got["labels"] != "" {
		t.Fatalf("body[labels] = %v, want empty string to remove all labels", got["labels"])
	}
	if got["milestone_id"] != float64(0) {
		t.Fatalf("body[milestone_id] = %v, want 0 to unassign", got["milestone_id"])
	}
	assertIDList(t, got, "assignee_ids")
	assertIDList(t, got, "reviewer_ids")
}

func TestMRUpdateLabelReplaceConflict(t *testing.T) {
	for _, extra := range [][]string{
		{"--label", "bug", "--add-label", "backend"},
		{"--label", "bug", "--remove-label", "triage"},
	} {
		_, err := executeMRUpdateCommand(t, "http://localhost:1", nil, extra...)
		if err == nil {
			t.Fatalf("mr update %v returned nil error, want mutual-exclusion usage error", extra)
		}
		if !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("mr update %v error = %v, want mutual-exclusion message", extra, err)
		}
		if exitCodeForError(err) != 2 {
			t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
		}
	}
}

func TestMRUpdateAddRemoveLabelsCombine(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil,
		"--add-label", "backend",
		"--remove-label", "triage",
	)
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["add_labels"] != "backend" {
		t.Fatalf("body[add_labels] = %v, want backend", (*body)["add_labels"])
	}
	if (*body)["remove_labels"] != "triage" {
		t.Fatalf("body[remove_labels] = %v, want triage", (*body)["remove_labels"])
	}
	if _, present := (*body)["labels"]; present {
		t.Fatalf("body contains labels = %v, want the replace field omitted", (*body)["labels"])
	}
}

func TestMRUpdateDraftWithTitle(t *testing.T) {
	server, body, getCalls := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--draft", "--title", "Add search endpoint")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["title"] != "Draft: Add search endpoint" {
		t.Fatalf("body[title] = %v, want Draft: Add search endpoint", (*body)["title"])
	}
	if *getCalls != 0 {
		t.Fatalf("GET merge request calls = %d, want 0 when --title is passed", *getCalls)
	}
}

func TestMRUpdateDraftFetchesCurrentTitle(t *testing.T) {
	server, body, getCalls := newMRUpdateTestServer(t, "Fix auth")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--draft")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if *getCalls != 1 {
		t.Fatalf("GET merge request calls = %d, want 1 to fetch the current title", *getCalls)
	}
	if (*body)["title"] != "Draft: Fix auth" {
		t.Fatalf("body[title] = %v, want Draft: Fix auth", (*body)["title"])
	}
}

func TestMRUpdateReadyStripsPrefix(t *testing.T) {
	for _, currentTitle := range []string{"Draft: Fix auth", "draft:Fix auth"} {
		server, body, _ := newMRUpdateTestServer(t, currentTitle)

		_, err := executeMRUpdateCommand(t, server.URL, nil, "--ready")
		server.Close()
		if err != nil {
			t.Fatalf("mr update --ready with title %q returned error: %v", currentTitle, err)
		}

		if (*body)["title"] != "Fix auth" {
			t.Fatalf("body[title] = %v for current title %q, want Fix auth", (*body)["title"], currentTitle)
		}
	}
}

func TestMRUpdateReadyWithoutPrefixIsIdempotent(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "Fix auth")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--ready")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["title"] != "Fix auth" {
		t.Fatalf("body[title] = %v, want unchanged title still sent", (*body)["title"])
	}
}

func TestMRUpdateDraftReadyConflict(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil, "--draft", "--ready")
	if err == nil {
		t.Fatal("mr update returned nil error, want mutual-exclusion usage error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("mr update error = %v, want mutual-exclusion message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRUpdateDraftFalseIsNotAnUpdate(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil, "--draft=false")
	if !errors.Is(err, errNoUpdateFlags) {
		t.Fatalf("mr update error = %v, want errNoUpdateFlags (--draft=false requests nothing)", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRUpdateEmptyTitleUsageError(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil, "--title", "  ")
	if err == nil {
		t.Fatal("mr update returned nil error, want empty-title usage error")
	}
	if !strings.Contains(err.Error(), "title cannot be empty") {
		t.Fatalf("mr update error = %v, want empty-title message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRUpdateEmptyTargetBranchUsageError(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil, "--target-branch", "")
	if err == nil {
		t.Fatal("mr update returned nil error, want empty-target-branch usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRUpdateBoolFalseSentWhenChanged(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--squash=false")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if value, present := (*body)["squash"]; !present || value != false {
		t.Fatalf("body[squash] = %v (present=%t), want explicit false sent", value, present)
	}
}

func TestMRUpdateDescriptionFromFile(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	path := filepath.Join(t.TempDir(), "description.md")
	if err := os.WriteFile(path, []byte("# Body\n\nfrom file\n"), 0o600); err != nil {
		t.Fatalf("write description file: %v", err)
	}

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--description-file", path)
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["description"] != "# Body\n\nfrom file\n" {
		t.Fatalf("body[description] = %q, want file content preserved verbatim", (*body)["description"])
	}
}

func TestMRUpdateDescriptionFromStdin(t *testing.T) {
	server, body, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, strings.NewReader("body from stdin"), "--description-file", "-")
	if err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	if (*body)["description"] != "body from stdin" {
		t.Fatalf("body[description] = %q, want stdin content", (*body)["description"])
	}
}

func TestMRUpdateDescriptionFlagConflict(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil,
		"--description", "inline",
		"--description-file", "notes.md",
	)
	if err == nil {
		t.Fatal("mr update returned nil error, want mutual-exclusion usage error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("mr update error = %v, want mutual-exclusion message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRUpdateUnreadableDescriptionFile(t *testing.T) {
	_, err := executeMRUpdateCommand(t, "http://localhost:1", nil,
		"--description-file", filepath.Join(t.TempDir(), "missing.md"),
	)
	if err == nil {
		t.Fatal("mr update returned nil error, want file read error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}
}

func TestMRUpdateUserNotFound(t *testing.T) {
	server, _, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--reviewer", "ghost")
	if !errors.Is(err, errUserNotFound) {
		t.Fatalf("mr update error = %v, want errUserNotFound", err)
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

func TestMRUpdateNotFoundErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Not found"}`)
	}))
	defer server.Close()

	_, err := executeMRUpdateCommand(t, server.URL, nil, "--title", "New title")
	if err == nil {
		t.Fatal("mr update returned nil error, want 404 error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	got := out.String()
	if !strings.Contains(got, "code: gitlab_not_found") {
		t.Fatalf("axi error output = %q, want gitlab_not_found code", got)
	}
	if strings.Contains(got, server.URL) {
		t.Fatalf("axi error output = %q, want no raw request URL leak", got)
	}
}

func TestMRUpdateParentDispatchRedirect(t *testing.T) {
	var out bytes.Buffer
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"mr", "123", "update",
		"--gitlab-token", "test-token",
		"--gitlab-base-url", "http://localhost:1",
		"--project", "group/project",
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("mr 123 update returned nil error, want redirect usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}

	var errOut bytes.Buffer
	writeCommandError(&errOut, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(errOut.String(), "mr update !123") {
		t.Fatalf("axi error output = %q, want redirect to `mr update !123`", errOut.String())
	}
}

func TestMRUpdateAxiTOONOutput(t *testing.T) {
	server, _, _ := newMRUpdateTestServer(t, "old title")
	defer server.Close()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl-axi", "test", "test", commandModeAxi)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"mr", "update", "123",
		"--gitlab-token", "test-token",
		"--gitlab-base-url", server.URL,
		"--project", "group/project",
		"--title", "New title",
		"-o", "toon",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("mr update returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_request:\n  iid: 123") {
		t.Fatalf("output = %q, want nested merge_request object", got)
	}
	if !strings.Contains(got, "help[1]: Run `mr view 123 --project group/project` to check merge status and pipeline results") {
		t.Fatalf("output = %q, want next-step hint carrying --project", got)
	}
}

func TestStripDraftTitle(t *testing.T) {
	tests := map[string]string{
		"Draft: Fix auth": "Fix auth",
		"draft:Fix auth":  "Fix auth",
		"DRAFT:  Fix":     "Fix",
		"Fix auth":        "Fix auth",
		"Redraft: x":      "Redraft: x",
		"Fix draft: y":    "Fix draft: y",
	}
	for title, want := range tests {
		if got := stripDraftTitle(title); got != want {
			t.Fatalf("stripDraftTitle(%q) = %q, want %q", title, got, want)
		}
	}
}
