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
	"strings"
	"testing"
)

func executeMRFinalizeCommand(t *testing.T, baseURL, command, ref string, stdin io.Reader, extraArgs ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	args := []string{
		"mr", command, ref,
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

func executeMRFinalizeAxiCommand(t *testing.T, baseURL, command, ref string, stdin io.Reader, extraArgs ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl-axi", "test", "test", commandModeAxi)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	args := []string{
		"mr", command, ref,
		"--gitlab-token", "test-token",
		"--gitlab-base-url", baseURL,
		"--project", "group/project",
		"-o", "toon",
	}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return out.String(), err
}

func TestMRMergeSendsFullOptions(t *testing.T) {
	squashMessageFile, err := os.CreateTemp(t.TempDir(), "squash-message-*")
	if err != nil {
		t.Fatalf("create temp squash message file: %v", err)
	}
	if _, err := squashMessageFile.WriteString("squash message\n"); err != nil {
		t.Fatalf("write temp squash message file: %v", err)
	}
	if err := squashMessageFile.Close(); err != nil {
		t.Fatalf("close temp squash message file: %v", err)
	}

	var body map[string]any
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123/merge" && r.Method == http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode merge body: %v", err)
			}
			fmt.Fprint(w, mergeRequestStateJSON(123, "merged"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := executeMRFinalizeCommand(t, server.URL, "merge", "123", strings.NewReader("merge message from stdin"),
		"--sha", "f5b0c3d2e1",
		"--auto-merge",
		"--squash=false",
		"--remove-source-branch",
		"--merge-commit-message-file", "-",
		"--squash-commit-message-file", squashMessageFile.Name(),
	)
	if err != nil {
		t.Fatalf("mr merge returned error: %v", err)
	}

	wantPaths := []string{
		"GET /api/v4/projects/group%2Fproject/merge_requests/123",
		"PUT /api/v4/projects/group%2Fproject/merge_requests/123/merge",
	}
	if fmt.Sprint(paths) != fmt.Sprint(wantPaths) {
		t.Fatalf("request paths = %v, want %v", paths, wantPaths)
	}

	want := map[string]any{
		"sha":                         "f5b0c3d2e1",
		"auto_merge":                  true,
		"squash":                      false,
		"should_remove_source_branch": true,
		"merge_commit_message":        "merge message from stdin",
		"squash_commit_message":       "squash message\n",
	}
	for key, value := range want {
		if body[key] != value {
			t.Fatalf("body[%s] = %#v, want %#v (full body: %#v)", key, body[key], value, body)
		}
	}
	if !strings.Contains(out, `"action": "merge"`) || !strings.Contains(out, `"state": "merged"`) {
		t.Fatalf("output = %q, want merge action and merged state", out)
	}
}

func TestMRMergeRejectsEmptySHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("GitLab API should not be called for empty --sha")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := executeMRFinalizeCommand(t, server.URL, "merge", "123", nil, "--sha", "")
	if err == nil {
		t.Fatal("mr merge returned nil error, want usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "--sha cannot be empty") {
		t.Fatalf("error = %v, want empty sha message", err)
	}
}

func TestMRMergeRejectsConflictingMessageInputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("GitLab API should not be called for conflicting content flags")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := executeMRFinalizeCommand(t, server.URL, "merge", "123", strings.NewReader("from stdin"),
		"--merge-commit-message", "inline",
		"--merge-commit-message-file", "-",
	)
	if err == nil {
		t.Fatal("mr merge returned nil error, want usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "--merge-commit-message and --merge-commit-message-file are mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive message", err)
	}
}

func TestMRCloseSendsStateEvent(t *testing.T) {
	body := map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode close body: %v", err)
			}
			fmt.Fprint(w, mergeRequestStateJSON(123, "closed"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := executeMRFinalizeCommand(t, server.URL, "close", "123", nil)
	if err != nil {
		t.Fatalf("mr close returned error: %v", err)
	}
	if body["state_event"] != "close" {
		t.Fatalf("body[state_event] = %#v, want close", body["state_event"])
	}
	if !strings.Contains(out, `"action": "close"`) || !strings.Contains(out, `"state": "closed"`) {
		t.Fatalf("output = %q, want close action and closed state", out)
	}
}

func TestMRReopenSendsStateEvent(t *testing.T) {
	body := map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "closed"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode reopen body: %v", err)
			}
			fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := executeMRFinalizeCommand(t, server.URL, "reopen", "123", nil)
	if err != nil {
		t.Fatalf("mr reopen returned error: %v", err)
	}
	if body["state_event"] != "reopen" {
		t.Fatalf("body[state_event] = %#v, want reopen", body["state_event"])
	}
	if !strings.Contains(out, `"action": "reopen"`) || !strings.Contains(out, `"state": "opened"`) {
		t.Fatalf("output = %q, want reopen action and opened state", out)
	}
}

func TestMRFinalizeCurrentResolvesIID(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	var listQuery string
	closed := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests" && r.Method == http.MethodGet:
			listQuery = r.URL.Query().Encode()
			fmt.Fprint(w, "["+mergeRequestStateJSON(123, "opened")+"]")
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodPut:
			closed = true
			fmt.Fprint(w, mergeRequestStateJSON(123, "closed"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if _, err := executeMRFinalizeCommand(t, server.URL, "close", "current", nil); err != nil {
		t.Fatalf("mr close current returned error: %v", err)
	}
	if !strings.Contains(listQuery, "source_branch=feature%2Fsearch") || !strings.Contains(listQuery, "state=opened") {
		t.Fatalf("list query = %q, want current-ref source branch lookup", listQuery)
	}
	if !closed {
		t.Fatal("close endpoint was not called after resolving current")
	}
}

func TestMRFinalizeNoops(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		initialState string
	}{
		{name: "merge already merged", command: "merge", initialState: "merged"},
		{name: "close already closed", command: "close", initialState: "closed"},
		{name: "reopen already opened", command: "reopen", initialState: "opened"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutationCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch {
				case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
					fmt.Fprint(w, mergeRequestStateJSON(123, tt.initialState))
				case r.Method == http.MethodPut:
					mutationCalls++
					t.Errorf("unexpected mutation request for no-op: %s", r.URL.EscapedPath())
					w.WriteHeader(http.StatusInternalServerError)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			out, err := executeMRFinalizeCommand(t, server.URL, tt.command, "123", nil)
			if err != nil {
				t.Fatalf("mr %s returned error: %v", tt.command, err)
			}
			if mutationCalls != 0 {
				t.Fatalf("mutationCalls = %d, want 0", mutationCalls)
			}
			if !strings.Contains(out, `"noop": true`) {
				t.Fatalf("output = %q, want noop true", out)
			}
		})
	}
}

func TestMRMergeVerifiesRaceNoopAfterMethodNotAllowed(t *testing.T) {
	getCalls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			getCalls++
			if getCalls == 1 {
				fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
				return
			}
			fmt.Fprint(w, mergeRequestStateJSON(123, "merged"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123/merge" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, `{"message":"Method Not Allowed"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := executeMRFinalizeCommand(t, server.URL, "merge", "123", nil)
	if err != nil {
		t.Fatalf("mr merge returned error: %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("GET calls = %d, want initial read plus verification read", getCalls)
	}
	if !strings.Contains(out, `"noop": true`) || !strings.Contains(out, `"state": "merged"`) {
		t.Fatalf("output = %q, want verified merged no-op", out)
	}
}

func TestMRFinalizeParentDispatchRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	for _, action := range []string{"merge", "close", "reopen"} {
		t.Run(action, func(t *testing.T) {
			_, err := executeMRRootCommandErr(t, server.URL, "!123", action)
			if err == nil {
				t.Fatal("Execute returned nil error, want redirect usage error")
			}
			if exitCodeForError(err) != 2 {
				t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("mr !123 %s", action)) {
				t.Fatalf("error = %v, want parent-dispatch redirect context", err)
			}
		})
	}
}

func TestMRMergeAxiTOONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "opened"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123/merge" && r.Method == http.MethodPut:
			fmt.Fprint(w, mergeRequestStateJSON(123, "merged"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out, err := executeMRFinalizeAxiCommand(t, server.URL, "merge", "123", nil)
	if err != nil {
		t.Fatalf("mr merge returned error: %v", err)
	}

	if !strings.Contains(out, "merge_request:\n  iid: 123") {
		t.Fatalf("output = %q, want nested merge_request object", out)
	}
	if !strings.Contains(out, "action: merge") {
		t.Fatalf("output = %q, want merge action", out)
	}
	if !strings.Contains(out, "help[1]: Run `mr view 123 --project group/project` to verify the merge request state") {
		t.Fatalf("output = %q, want project-carrying help", out)
	}
}

func TestMRFinalizeIncompatibleStateSurfacesGitLabError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123" && r.Method == http.MethodGet:
			fmt.Fprint(w, mergeRequestStateJSON(123, "closed"))
		case r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests/123/merge" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprint(w, `{"message":"Method Not Allowed"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := executeMRFinalizeCommand(t, server.URL, "merge", "123", nil)
	if err == nil {
		t.Fatal("mr merge returned nil error, want GitLab runtime error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1", exitCodeForError(err))
	}
	if errors.Is(err, errNoCurrentMergeRequest) {
		t.Fatalf("error = %v, should not be converted to a current-ref error", err)
	}
}

func mergeRequestStateJSON(iid int64, state string) string {
	return strings.Replace(mergeRequestJSON(iid, "short description"), `"state": "opened"`, fmt.Sprintf(`"state": %q`, state), 1)
}
