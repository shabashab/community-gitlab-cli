package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func draftNoteJSON(id int64, note string, extra string) string {
	if extra != "" {
		extra = "," + extra
	}

	return fmt.Sprintf(`{"id":%d,"note":%q%s}`, id, note, extra)
}

func TestMRDraftsListPaginatesAndRenders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.EscapedPath() != commentDraftsPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.URL.Query().Get("page") == "2" {
			fmt.Fprint(w, "["+draftNoteJSON(7, "third", "")+"]")
			return
		}
		w.Header().Set("X-Next-Page", "2")
		fmt.Fprint(w, "["+strings.Join([]string{
			draftNoteJSON(5, "first draft", `"position":{"position_type":"text","new_path":"src/app.go","new_line":3}`),
			draftNoteJSON(6, "reply draft", fmt.Sprintf(`"discussion_id":%q`, discussionIDResolved)),
		}, ",")+"]")
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, fragment := range []string{
		"draft_notes[3]{id,file,line,preview}:",
		"5,src/app.go,3,first draft",
		"count: 3 of 3 total",
		"mr drafts publish 123 --all --project group/project",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, out)
		}
	}
}

func TestMRDraftsListEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "count: 0 of 0 total") || !strings.Contains(out, "No pending draft notes") {
		t.Fatalf("empty list output wrong:\n%s", out)
	}
}

func TestMRDraftsListExtraFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "["+draftNoteJSON(6, "reply draft", fmt.Sprintf(`"discussion_id":%q`, discussionIDResolved))+"]")
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "123", "--fields", "discussion_id")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "draft_notes[1]{id,file,line,preview,discussion_id}:") || !strings.Contains(out, "aa11bb22") {
		t.Fatalf("output missing discussion_id column:\n%s", out)
	}
}

func TestMRDraftsListStandardJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "["+draftNoteJSON(5, "first draft", "")+"]")
	}))
	t.Cleanup(server.Close)

	out, err := executeMRRootCommandErr(t, server.URL, "drafts", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, `"draft_notes"`) || !strings.Contains(out, `"id": 5`) {
		t.Fatalf("standard json output wrong:\n%s", out)
	}
}

func TestMRDraftsPublishSingle(t *testing.T) {
	published := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.EscapedPath() == commentDraftsPath+"/5/publish" {
			published = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "publish", "123", "5")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !published {
		t.Fatal("publish endpoint was not called")
	}
	if !strings.Contains(out, "published:") || !strings.Contains(out, "id: 5") || !strings.Contains(out, "count: 1") {
		t.Fatalf("publish output wrong:\n%s", out)
	}
}

func TestMRDraftsPublishAll(t *testing.T) {
	bulkCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == commentDraftsPath:
			fmt.Fprint(w, "["+draftNoteJSON(5, "a", "")+","+draftNoteJSON(6, "b", "")+"]")
		case r.Method == http.MethodPost && r.URL.EscapedPath() == commentDraftsPath+"/bulk_publish":
			bulkCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "publish", "123", "--all")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !bulkCalled {
		t.Fatal("bulk_publish endpoint was not called")
	}
	if !strings.Contains(out, "all: true") || !strings.Contains(out, "count: 2") {
		t.Fatalf("publish --all output wrong:\n%s", out)
	}
	if !strings.Contains(out, "mr discussions 123 --project group/project") {
		t.Fatalf("publish --all output missing the next-step hint:\n%s", out)
	}
}

func TestMRDraftsPublishAllEmptyIsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.EscapedPath() == commentDraftsPath {
			fmt.Fprint(w, "[]")
			return
		}
		// The provably-empty set must not trigger a bulk publish.
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "publish", "123", "--all")
	if err != nil {
		t.Fatalf("Execute must be a no-op success, got error: %v", err)
	}
	if !strings.Contains(out, "noop: true") || !strings.Contains(out, "count: 0") {
		t.Fatalf("noop output wrong:\n%s", out)
	}
}

func TestMRDraftsPublishArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"id and --all", []string{"mr", "drafts", "publish", "123", "5", "--all"}},
		{"neither id nor --all", []string{"mr", "drafts", "publish", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeCommentCommand(t, commandModeAxi, "http://127.0.0.1:1", nil, tt.args...)
			if err == nil {
				t.Fatal("Execute returned nil error, want usage error")
			}
			if exitCodeForError(err) != 2 {
				t.Fatalf("exitCodeForError = %d, want 2 (%v)", exitCodeForError(err), err)
			}
		})
	}
}

func TestMRDraftsInvalidDraftNoteID(t *testing.T) {
	_, err := executeCommentCommand(t, commandModeAxi, "http://127.0.0.1:1", nil, "mr", "drafts", "publish", "123", "abc")
	if !errors.Is(err, errInvalidDraftNoteID) {
		t.Fatalf("expected errInvalidDraftNoteID, got %v", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
	}

	var buf bytes.Buffer
	writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(buf.String(), "code: invalid_draft_note_id") {
		t.Fatalf("rendered error missing code:\n%s", buf.String())
	}
}

func TestMRDraftsPublishMissingDraftGetsDraftsHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 Not found"}`)
	}))
	t.Cleanup(server.Close)

	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "publish", "123", "99")
	if err == nil {
		t.Fatal("Execute returned nil error, want 404 error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1", exitCodeForError(err))
	}

	code, _, help := classifyError(err, "gl-axi")
	if code != "gitlab_not_found" {
		t.Fatalf("code = %q, want gitlab_not_found", code)
	}
	if len(help) == 0 || !strings.Contains(help[0], "gl-axi mr drafts 123") {
		t.Fatalf("404 help must point at the drafts list, got %v", help)
	}
}

func TestMRDraftsDelete(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.EscapedPath() == commentDraftsPath+"/5" {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "delete", "123", "5")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !deleted {
		t.Fatal("delete endpoint was not called")
	}
	if !strings.Contains(out, "deleted:") || !strings.Contains(out, "id: 5") {
		t.Fatalf("delete output wrong:\n%s", out)
	}
}

func TestMRDraftsDeleteVerified404IsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == commentDraftsPath+"/5":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not found"}`)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == commentDraftsPath:
			// The list succeeds and proves ID 5 is absent.
			fmt.Fprint(w, "["+draftNoteJSON(9, "other", "")+"]")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "delete", "123", "5")
	if err != nil {
		t.Fatalf("verified-absent delete must be a no-op success, got: %v", err)
	}
	if !strings.Contains(out, "noop: true") {
		t.Fatalf("noop output wrong:\n%s", out)
	}
}

func TestMRDraftsDelete404StaysErrorWhenListFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodDelete && r.URL.EscapedPath() == commentDraftsPath+"/5":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"404 Not found"}`)
		case r.Method == http.MethodGet && r.URL.EscapedPath() == commentDraftsPath:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"boom"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "drafts", "delete", "123", "5")
	if err == nil {
		t.Fatal("unverifiable 404 must stay an error")
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1", exitCodeForError(err))
	}
}

func TestMRDraftsParentDispatchRedirects(t *testing.T) {
	_, err := executeCommentCommand(t, commandModeAxi, "http://127.0.0.1:1", nil, "mr", "!123", "drafts")
	if err == nil {
		t.Fatal("Execute returned nil error, want usage redirect")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "runs as a subcommand") {
		t.Fatalf("error %q missing subcommand redirect", err.Error())
	}
}
