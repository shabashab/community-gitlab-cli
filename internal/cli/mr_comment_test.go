package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	commentMRPath     = "/api/v4/projects/group%2Fproject/merge_requests/123"
	commentDiffsPath  = "/api/v4/projects/group%2Fproject/merge_requests/123/diffs"
	commentNotesPath  = "/api/v4/projects/group%2Fproject/merge_requests/123/notes"
	commentDraftsPath = "/api/v4/projects/group%2Fproject/merge_requests/123/draft_notes"

	// appGoLineCode is sha1("src/app.go"), pinning fabricated line_codes.
	appGoLineCode = "186661a929ba829946a73d5054834e5bf9c3153e"
)

func commentMergeRequestJSON() string {
	return `{"iid":123,"title":"t","state":"opened","diff_refs":{"base_sha":"base000","head_sha":"head000","start_sha":"start000"}}`
}

// commentDiffsJSON changes src/app.go (context/removed/added mix with shifted
// numbering, new side 12-16) and renames old/name.go to new/name.go.
func commentDiffsJSON() string {
	return `[` +
		`{"old_path":"src/app.go","new_path":"src/app.go","diff":"@@ -10,4 +12,5 @@\n ctx1\n ctx2\n-rm1\n+add1\n+add2\n ctx3\n"},` +
		`{"old_path":"old/name.go","new_path":"new/name.go","renamed_file":true,"diff":"@@ -1,2 +1,2 @@\n ctx\n-x\n+y\n"}` +
		`]`
}

func createdDiscussionJSON(discussionID string, noteID int64, noteType, positionJSON string) string {
	position := ""
	if positionJSON != "" {
		position = fmt.Sprintf(`,"position":%s`, positionJSON)
	}

	return fmt.Sprintf(
		`{"id":%q,"individual_note":false,"notes":[{"id":%d,"type":%q,"body":"x","author":{"username":"rev"},"created_at":"2026-07-08T10:00:00Z","resolvable":true%s}]}`,
		discussionID, noteID, noteType, position,
	)
}

func executeCommentCommand(t *testing.T, mode commandMode, baseURL string, stdin io.Reader, args ...string) (string, error) {
	t.Helper()

	binName := "gl"
	if mode == commandModeAxi {
		binName = "gl-axi"
	}

	var out bytes.Buffer
	cmd, _ := newRootCommand(binName, "test", "test", mode)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if stdin != nil {
		cmd.SetIn(stdin)
	}

	fullArgs := append([]string{}, args...)
	fullArgs = append(fullArgs,
		"--gitlab-token", "test-token",
		"--gitlab-base-url", baseURL,
		"--project", "group/project",
	)
	cmd.SetArgs(fullArgs)

	err := cmd.Execute()

	return out.String(), err
}

// newCommentPositionServer serves the merge request and diffs fixtures and
// captures the decoded JSON body of the first POST to postPath.
func newCommentPositionServer(t *testing.T, postPath, postResponse string) (*httptest.Server, *map[string]any) {
	t.Helper()

	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == commentMRPath:
			fmt.Fprint(w, commentMergeRequestJSON())
		case r.Method == http.MethodGet && r.URL.EscapedPath() == commentDiffsPath:
			fmt.Fprint(w, commentDiffsJSON())
		case r.Method == http.MethodPost && r.URL.EscapedPath() == postPath:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, postResponse)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server, &body
}

func requestPosition(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	position, ok := body["position"].(map[string]any)
	if !ok {
		t.Fatalf("request body carries no position object: %v", body)
	}

	return position
}

func TestMRCommentPlainCreatesDiscussionThread(t *testing.T) {
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.EscapedPath() == discussionsListPath:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, createdDiscussionJSON(discussionIDUnresolvedAlice, 9001, "DiscussionNote", ""))
		default:
			// A plain comment must stay single-request: no merge request or
			// diff lookups.
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--body", "LGTM overall")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if body["body"] != "LGTM overall" {
		t.Fatalf("request body = %v, want body LGTM overall", body)
	}
	for _, fragment := range []string{
		"note_id: 9001",
		"type: DiscussionNote",
		"resolvable: true",
		"discussion_id: 6f9a1c2d",
		"mr discussion 123 6f9a1c2d --project group/project",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, out)
		}
	}
}

func TestMRCommentBodyFromStdin(t *testing.T) {
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, createdDiscussionJSON(discussionIDUnresolvedAlice, 9001, "DiscussionNote", ""))
	}))
	t.Cleanup(server.Close)

	stdin := strings.NewReader("multi-line\nreview body\n")
	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, stdin, "mr", "comment", "123", "--body-file", "-"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if body["body"] != "multi-line\nreview body\n" {
		t.Fatalf("stdin body not passed through verbatim: %q", body["body"])
	}
}

func TestMRCommentFlagValidationMatrix(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		fragment string
	}{
		{"line and old-line", []string{"--file", "f", "--line", "5", "--old-line", "3", "--body", "x"}, "mutually exclusive"},
		{"line without file", []string{"--line", "5", "--body", "x"}, "require --file"},
		{"reply-to with file", []string{"--reply-to", "abc123", "--file", "f", "--body", "x"}, "--reply-to cannot be combined"},
		{"reply-to with note", []string{"--reply-to", "abc123", "--note", "--body", "x"}, "--reply-to cannot be combined with --note"},
		{"note with draft", []string{"--note", "--draft", "--body", "x"}, "--note cannot be combined with --draft"},
		{"note with file", []string{"--note", "--file", "f", "--body", "x"}, "--note cannot be combined"},
		{"resolve without reply-to", []string{"--resolve", "--body", "x"}, "--resolve requires --reply-to"},
		{"resolve without draft", []string{"--resolve", "--reply-to", "abc123", "--body", "x"}, "--resolve requires --draft"},
		{"body and body-file", []string{"--body", "x", "--body-file", "-"}, "mutually exclusive"},
		{"missing body", nil, "missing required flag --body"},
		{"blank body", []string{"--body", "   "}, "missing required flag --body"},
		{"empty file path", []string{"--file", "", "--body", "x"}, "--file requires a non-empty path"},
		{"zero line", []string{"--file", "f", "--line", "0", "--body", "x"}, "positive integers"},
		{"inverted range", []string{"--file", "f", "--line", "9:3", "--body", "x"}, "range start 9 is after end 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"mr", "comment", "123"}, tt.args...)
			_, err := executeCommentCommand(t, commandModeAxi, "http://127.0.0.1:1", nil, args...)
			if err == nil {
				t.Fatal("Execute returned nil error, want usage error")
			}
			if exitCodeForError(err) != 2 {
				t.Fatalf("exitCodeForError = %d, want 2 (%v)", exitCodeForError(err), err)
			}
			if !strings.Contains(err.Error(), tt.fragment) {
				t.Fatalf("error %q missing fragment %q", err.Error(), tt.fragment)
			}
		})
	}
}

func TestMRCommentPositionedAddedLine(t *testing.T) {
	positionJSON := `{"position_type":"text","new_path":"src/app.go","old_path":"src/app.go","new_line":15,"base_sha":"base000","head_sha":"head000","start_sha":"start000"}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9002, "DiffNote", positionJSON))

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "15", "--body", "typo")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	for key, want := range map[string]any{
		"base_sha":      "base000",
		"head_sha":      "head000",
		"start_sha":     "start000",
		"position_type": "text",
		"new_path":      "src/app.go",
		"old_path":      "src/app.go",
		"new_line":      float64(15),
	} {
		if position[key] != want {
			t.Fatalf("position[%s] = %v, want %v", key, position[key], want)
		}
	}
	if _, hasOldLine := position["old_line"]; hasOldLine {
		t.Fatalf("added line must not carry old_line: %v", position)
	}
	if !strings.Contains(out, "file: src/app.go") || !strings.Contains(out, "line: 15") {
		t.Fatalf("output missing anchored file/line:\n%s", out)
	}
}

func TestMRCommentPositionedContextLineCarriesBothSides(t *testing.T) {
	positionJSON := `{"position_type":"text","new_path":"src/app.go","old_path":"src/app.go","old_line":11,"new_line":13}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9003, "DiffNote", positionJSON))

	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "13", "--body", "context"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	if position["old_line"] != float64(11) || position["new_line"] != float64(13) {
		t.Fatalf("context line must carry both differing sides, got %v", position)
	}
}

func TestMRCommentPositionedRemovedLine(t *testing.T) {
	positionJSON := `{"position_type":"text","new_path":"src/app.go","old_path":"src/app.go","old_line":12}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9004, "DiffNote", positionJSON))

	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--old-line", "12", "--body", "why removed?"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	if position["old_line"] != float64(12) {
		t.Fatalf("position[old_line] = %v, want 12", position["old_line"])
	}
	if _, hasNewLine := position["new_line"]; hasNewLine {
		t.Fatalf("removed line must not carry new_line: %v", position)
	}
}

func TestMRCommentRenamedFileSendsBothPaths(t *testing.T) {
	positionJSON := `{"position_type":"text","new_path":"new/name.go","old_path":"old/name.go","new_line":2}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9005, "DiffNote", positionJSON))

	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "new/name.go", "--line", "2", "--body", "rename"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	if position["old_path"] != "old/name.go" || position["new_path"] != "new/name.go" {
		t.Fatalf("renamed file must send both paths, got %v", position)
	}
}

func TestMRCommentRangeFabricatesLineCodes(t *testing.T) {
	positionJSON := `{"position_type":"text","new_path":"src/app.go","old_path":"src/app.go","new_line":15}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9006, "DiffNote", positionJSON))

	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "13:15", "--body", "range"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	if position["new_line"] != float64(15) {
		t.Fatalf("top-level new_line must come from the range end, got %v", position)
	}

	lineRange, ok := position["line_range"].(map[string]any)
	if !ok {
		t.Fatalf("position carries no line_range: %v", position)
	}
	start, _ := lineRange["start"].(map[string]any)
	end, _ := lineRange["end"].(map[string]any)

	// Start is a context line (old 11 / new 13): both sides, no type.
	if start["line_code"] != appGoLineCode+"_11_13" {
		t.Fatalf("start line_code = %v, want %s_11_13", start["line_code"], appGoLineCode)
	}
	if _, hasType := start["type"]; hasType {
		t.Fatalf("context range start must carry no type: %v", start)
	}
	if start["old_line"] != float64(11) || start["new_line"] != float64(13) {
		t.Fatalf("start lines = %v, want old 11 / new 13", start)
	}

	// End is an added line (old cursor 13 / new 15): type new, new side only.
	if end["line_code"] != appGoLineCode+"_13_15" {
		t.Fatalf("end line_code = %v, want %s_13_15", end["line_code"], appGoLineCode)
	}
	if end["type"] != "new" {
		t.Fatalf("end type = %v, want new", end["type"])
	}
	if end["new_line"] != float64(15) {
		t.Fatalf("end new_line = %v, want 15", end["new_line"])
	}
	if _, hasOldLine := end["old_line"]; hasOldLine {
		t.Fatalf("added range end must not carry old_line: %v", end)
	}
}

func TestMRCommentFileLevelPosition(t *testing.T) {
	positionJSON := `{"position_type":"file","new_path":"src/app.go","old_path":"src/app.go"}`
	server, body := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9007, "DiffNote", positionJSON))

	if _, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--body", "rename this file"); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	position := requestPosition(t, *body)
	if position["position_type"] != "file" {
		t.Fatalf("position_type = %v, want file", position["position_type"])
	}
	if _, hasNewLine := position["new_line"]; hasNewLine {
		t.Fatalf("file-level position must carry no lines: %v", position)
	}
	if _, hasOldLine := position["old_line"]; hasOldLine {
		t.Fatalf("file-level position must carry no lines: %v", position)
	}
}

func TestMRCommentDraftPositioned(t *testing.T) {
	response := `{"id":77,"note":"looks wrong","position":{"position_type":"text","new_path":"src/app.go","old_path":"src/app.go","new_line":15}}`
	server, body := newCommentPositionServer(t, commentDraftsPath, response)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--draft", "--file", "src/app.go", "--line", "15", "--body", "looks wrong")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if (*body)["note"] != "looks wrong" {
		t.Fatalf("draft body must use the note field, got %v", *body)
	}
	position := requestPosition(t, *body)
	if position["new_line"] != float64(15) {
		t.Fatalf("draft position new_line = %v, want 15", position["new_line"])
	}
	for _, fragment := range []string{
		"draft_note:",
		"id: 77",
		"file: src/app.go",
		"line: 15",
		"mr drafts publish 123 77 --project group/project",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, out)
		}
	}
}

func TestMRCommentDraftPlainSendsNoPosition(t *testing.T) {
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.EscapedPath() != commentDraftsPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":78,"note":"overall pending"}`)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--draft", "--body", "overall pending")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if _, hasPosition := body["position"]; hasPosition {
		t.Fatalf("plain draft must not send a position: %v", body)
	}
	if !strings.Contains(out, "id: 78") {
		t.Fatalf("output missing draft id:\n%s", out)
	}
}

func TestMRCommentReplyToFullDiscussionID(t *testing.T) {
	discussionPath := discussionsListPath + "/" + discussionIDUnresolvedAlice
	body := map[string]any{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == discussionPath:
			fmt.Fprint(w, discussionJSON(discussionIDUnresolvedAlice,
				discussionNoteJSON(901, "alice", "original", true, false, false, "2026-07-01T08:00:00Z", "2026-07-01T08:00:00Z"),
			))
		case r.Method == http.MethodPost && r.URL.EscapedPath() == discussionPath+"/notes":
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":9100,"type":"DiscussionNote","body":"agreed","author":{"username":"rev"},"created_at":"2026-07-08T10:00:00Z","resolvable":true}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--reply-to", discussionIDUnresolvedAlice, "--body", "agreed")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if body["body"] != "agreed" {
		t.Fatalf("reply body = %v, want agreed", body)
	}
	if !strings.Contains(out, "note_id: 9100") || !strings.Contains(out, "discussion_id: 6f9a1c2d") {
		t.Fatalf("output missing reply identifiers:\n%s", out)
	}
}

func TestMRCommentDraftReplyWithResolveByPrefix(t *testing.T) {
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == discussionsListPath:
			fmt.Fprint(w, discussionListStubJSON())
		case r.Method == http.MethodPost && r.URL.EscapedPath() == commentDraftsPath:
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"id":79,"note":"fixed","discussion_id":%q,"resolve_discussion":true}`, discussionIDUnresolvedAlice)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--draft", "--reply-to", "6f9a1c", "--resolve", "--body", "fixed")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if body["in_reply_to_discussion_id"] != discussionIDUnresolvedAlice {
		t.Fatalf("in_reply_to_discussion_id = %v, want full ID", body["in_reply_to_discussion_id"])
	}
	if body["resolve_discussion"] != true {
		t.Fatalf("resolve_discussion = %v, want true", body["resolve_discussion"])
	}
	if !strings.Contains(out, "resolve_discussion: true") || !strings.Contains(out, "discussion_id: 6f9a1c2d") {
		t.Fatalf("output missing draft reply fields:\n%s", out)
	}
}

func TestMRCommentNoteUsesNotesAPI(t *testing.T) {
	body := map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.EscapedPath() != commentNotesPath {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":9200,"body":"heads up","author":{"username":"rev"},"created_at":"2026-07-08T10:00:00Z"}`)
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--note", "--body", "heads up")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if body["body"] != "heads up" {
		t.Fatalf("note body = %v, want heads up", body)
	}
	for _, fragment := range []string{
		"note_id: 9200",
		"type: Note",
		"mr discussions 123 --state all --project group/project",
	} {
		if !strings.Contains(out, fragment) {
			t.Fatalf("output missing %q:\n%s", fragment, out)
		}
	}
	if strings.Contains(out, "discussion_id") {
		t.Fatalf("plain note must carry no discussion id:\n%s", out)
	}
}

func TestMRCommentDiffNotReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.EscapedPath() == commentMRPath {
			fmt.Fprint(w, `{"iid":123,"title":"t","state":"opened"}`)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "15", "--body", "x")
	if !errors.Is(err, errMergeRequestDiffNotReady) {
		t.Fatalf("expected errMergeRequestDiffNotReady, got %v", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1", exitCodeForError(err))
	}

	var buf bytes.Buffer
	writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)
	got := buf.String()
	if !strings.Contains(got, "code: merge_request_diff_not_ready") {
		t.Fatalf("rendered error missing code:\n%s", got)
	}
	if strings.Contains(got, server.URL) {
		t.Fatalf("rendered error leaks the server URL:\n%s", got)
	}
}

func TestMRCommentFileNotInDiff(t *testing.T) {
	server, _ := newCommentPositionServer(t, discussionsListPath, "{}")

	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "missing.go", "--line", "3", "--body", "x")
	if !errors.Is(err, errFileNotInDiff) {
		t.Fatalf("expected errFileNotInDiff, got %v", err)
	}

	var buf bytes.Buffer
	writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)
	got := buf.String()
	if !strings.Contains(got, "code: file_not_in_diff") || !strings.Contains(got, "src/app.go") {
		t.Fatalf("rendered error must list changed files:\n%s", got)
	}
}

func TestMRCommentLineNotInDiff(t *testing.T) {
	server, _ := newCommentPositionServer(t, discussionsListPath, "{}")

	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "99", "--body", "x")
	if !errors.Is(err, errLineNotInDiff) {
		t.Fatalf("expected errLineNotInDiff, got %v", err)
	}

	var buf bytes.Buffer
	writeCommandError(&buf, commandModeAxi, "toon", "gl-axi", err)
	got := buf.String()
	if !strings.Contains(got, "code: line_not_in_diff") || !strings.Contains(got, "12-16") {
		t.Fatalf("rendered error must show the commentable ranges:\n%s", got)
	}
}

func TestMRCommentLineNotInDiffSuggestsOtherSide(t *testing.T) {
	server, _ := newCommentPositionServer(t, discussionsListPath, "{}")

	// New side spans 12-16; line 10 exists only on the old side (ctx1).
	_, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "10", "--body", "x")
	if !errors.Is(err, errLineNotInDiff) {
		t.Fatalf("expected errLineNotInDiff, got %v", err)
	}

	hintFound := false
	for _, hint := range helpFromError(err) {
		if strings.Contains(hint, "--old-line 10") {
			hintFound = true
		}
	}
	if !hintFound {
		t.Fatalf("expected a cross-side suggestion, got %v", helpFromError(err))
	}
}

func TestMRCommentPositionDowngradeSurfacesHint(t *testing.T) {
	// GitLab answers 201 but the note came back unanchored (DiscussionNote,
	// no position): the command still succeeds and warns through help.
	server, _ := newCommentPositionServer(t, discussionsListPath, createdDiscussionJSON(discussionIDUnresolvedAlice, 9008, "DiscussionNote", ""))

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "123", "--file", "src/app.go", "--line", "15", "--body", "x")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, "did not anchor the comment") {
		t.Fatalf("output missing the downgrade hint:\n%s", out)
	}
}

func TestMRCommentCurrentRef(t *testing.T) {
	stubCurrentBranch(t, "feature-x", nil)

	listCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/merge_requests":
			listCalled = true
			if got := r.URL.Query().Get("source_branch"); got != "feature-x" {
				t.Errorf("source_branch = %q, want feature-x", got)
			}
			fmt.Fprint(w, `[{"iid":123,"title":"t","state":"opened","author":{"username":"a"}}]`)
		case r.Method == http.MethodPost && r.URL.EscapedPath() == discussionsListPath:
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, createdDiscussionJSON(discussionIDUnresolvedAlice, 9009, "DiscussionNote", ""))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	out, err := executeCommentCommand(t, commandModeAxi, server.URL, nil, "mr", "comment", "current", "--body", "from current")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !listCalled {
		t.Fatal("current ref must resolve through the merge request list")
	}
	if !strings.Contains(out, "note_id: 9009") {
		t.Fatalf("output missing created note:\n%s", out)
	}
}

func TestMRCommentStandardJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, createdDiscussionJSON(discussionIDUnresolvedAlice, 9010, "DiscussionNote", ""))
	}))
	t.Cleanup(server.Close)

	out, err := executeMRRootCommandErr(t, server.URL, "comment", "123", "--body", "x")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !strings.Contains(out, `"note_id": 9010`) {
		t.Fatalf("standard json output missing note_id:\n%s", out)
	}
	if !strings.Contains(out, fmt.Sprintf("%q", discussionIDUnresolvedAlice)) {
		t.Fatalf("standard json output must carry the full discussion id:\n%s", out)
	}
}

func TestMRCommentParentDispatchRedirects(t *testing.T) {
	_, err := executeCommentCommand(t, commandModeAxi, "http://127.0.0.1:1", nil, "mr", "!123", "comment")
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
