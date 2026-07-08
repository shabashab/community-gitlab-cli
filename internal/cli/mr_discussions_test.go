package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	discussionsListPath = "/api/v4/projects/group%2Fproject/merge_requests/123/discussions"

	discussionIDUnresolvedAlice = "6f9a1c2d0e5b7a9c1d2e3f4a5b6c7d8e9f0a1b2c"
	discussionIDUnresolvedBob   = "6f9b000011112222333344445555666677778888"
	discussionIDResolved        = "aa11bb22cc33dd44ee55ff6677889900aabbccdd"
	discussionIDSystem          = "ffee11cc00000000000000000000000000000000"
)

func discussionNoteJSON(id int64, author, body string, resolvable, resolved, system bool, createdAt, updatedAt string) string {
	return fmt.Sprintf(
		`{"id":%d,"type":"DiscussionNote","body":%q,"author":{"username":%q},"system":%t,"created_at":%q,"updated_at":%q,"resolvable":%t,"resolved":%t}`,
		id, body, author, system, createdAt, updatedAt, resolvable, resolved,
	)
}

func discussionJSON(id string, notes ...string) string {
	return fmt.Sprintf(`{"id":%q,"individual_note":false,"notes":[%s]}`, id, strings.Join(notes, ","))
}

// discussionListStubJSON is the standard mixed fixture: two unresolved
// threads, one resolved thread, and one system discussion.
func discussionListStubJSON() string {
	return "[" + strings.Join([]string{
		discussionJSON(discussionIDUnresolvedAlice,
			discussionNoteJSON(901, "alice", "This branch check looks inverted somewhere", true, false, false, "2026-07-01T08:00:00Z", "2026-07-03T12:00:00Z"),
			discussionNoteJSON(902, "bob", "Agreed - needs a fix", true, false, false, "2026-07-02T09:00:00Z", "2026-07-02T09:00:00Z"),
		),
		discussionJSON(discussionIDUnresolvedBob,
			discussionNoteJSON(903, "bob", "Missing nil guard here", true, false, false, "2026-07-04T10:00:00Z", "2026-07-04T10:00:00Z"),
		),
		discussionJSON(discussionIDResolved,
			fmt.Sprintf(
				`{"id":904,"type":"DiscussionNote","body":"Typo in the docstring","author":{"username":"mona"},"system":false,"created_at":"2026-06-30T08:00:00Z","updated_at":"2026-07-01T08:00:00Z","resolvable":true,"resolved":true,"resolved_by":{"username":"mona"},"resolved_at":"2026-07-01T08:00:00Z"}`,
			),
		),
		discussionJSON(discussionIDSystem,
			`{"id":905,"type":"","body":"added 3 commits","author":{"username":"alice"},"system":true,"created_at":"2026-07-05T08:00:00Z","updated_at":"2026-07-05T08:00:00Z","resolvable":false,"resolved":false}`,
		),
	}, ",") + "]"
}

// newDiscussionListTestServer serves the given JSON body on the discussions
// list path and records each list request's query values.
func newDiscussionListTestServer(t *testing.T, body string) (*httptest.Server, *[]map[string]string) {
	t.Helper()

	var queries []map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath:
			query := map[string]string{}
			for key := range r.URL.Query() {
				query[key] = r.URL.Query().Get(key)
			}
			queries = append(queries, query)
			fmt.Fprint(w, body)
		default:
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return server, &queries
}

func executeDiscussionCommand(t *testing.T, mode commandMode, baseURL string, args ...string) (string, error) {
	t.Helper()

	binName := "gl"
	if mode == commandModeAxi {
		binName = "gl-axi"
	}

	var out bytes.Buffer
	cmd, _ := newRootCommand(binName, "test", "test", mode)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

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

func discussionTestTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse test time %q: %v", value, err)
	}

	return parsed
}

func TestFetchAllMergeRequestDiscussionsPaginates(t *testing.T) {
	var queries []map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != discussionsListPath {
			t.Errorf("unexpected request path %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
			return
		}

		query := map[string]string{
			"page":     r.URL.Query().Get("page"),
			"per_page": r.URL.Query().Get("per_page"),
		}
		queries = append(queries, query)

		if query["page"] == "2" {
			fmt.Fprintf(w, "[%s]", discussionJSON(discussionIDResolved,
				discussionNoteJSON(904, "mona", "Typo", true, true, false, "2026-06-30T08:00:00Z", "2026-07-01T08:00:00Z"),
			))
			return
		}

		w.Header().Set("X-Next-Page", "2")
		fmt.Fprintf(w, "[%s,%s]",
			discussionJSON(discussionIDUnresolvedAlice,
				discussionNoteJSON(901, "alice", "First", true, false, false, "2026-07-01T08:00:00Z", "2026-07-01T08:00:00Z"),
			),
			discussionJSON(discussionIDUnresolvedBob,
				discussionNoteJSON(903, "bob", "Second", true, false, false, "2026-07-02T08:00:00Z", "2026-07-02T08:00:00Z"),
			),
		)
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussions", "123", "--state", "all")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("expected 2 list requests, got %d: %v", len(queries), queries)
	}
	for _, query := range queries {
		if query["per_page"] != "100" {
			t.Errorf("expected per_page=100, got %q", query["per_page"])
		}
	}
	if queries[1]["page"] != "2" {
		t.Errorf("expected second request to fetch page 2, got %q", queries[1]["page"])
	}
	if !strings.Contains(got, "count: 3 of 3 total") {
		t.Errorf("expected exact total across pages, got:\n%s", got)
	}
}

func TestSummarizeDiscussion(t *testing.T) {
	created := discussionTestTime(t, "2026-07-01T08:00:00Z")
	updated := discussionTestTime(t, "2026-07-03T12:00:00Z")
	resolvedAt := discussionTestTime(t, "2026-07-02T10:00:00Z")

	t.Run("resolved thread", func(t *testing.T) {
		summary, ok := summarizeDiscussion(&gitlab.Discussion{
			ID: strings.ToUpper(discussionIDResolved),
			Notes: []*gitlab.Note{
				{
					Author:     gitlab.NoteAuthor{Username: "alice"},
					Type:       gitlab.DiffNote,
					Body:       "Fix this",
					CreatedAt:  &created,
					UpdatedAt:  &updated,
					Resolvable: true,
					Resolved:   true,
					ResolvedBy: gitlab.NoteResolvedBy{Username: "mona"},
					ResolvedAt: &resolvedAt,
					Position:   &gitlab.NotePosition{NewPath: "internal/cli/mr.go", NewLine: 42},
				},
			},
		})
		if !ok {
			t.Fatal("expected ok for a discussion with notes")
		}
		if summary.state != "resolved" || !summary.resolvable || !summary.resolved {
			t.Errorf("expected resolved state, got %+v", summary)
		}
		if summary.id != discussionIDResolved {
			t.Errorf("expected lowercased id, got %q", summary.id)
		}
		if summary.resolvedBy != "mona" || summary.resolvedAt == nil {
			t.Errorf("expected resolver metadata, got %+v", summary)
		}
		if summary.file != "internal/cli/mr.go" || summary.line != 42 {
			t.Errorf("expected diff position, got file=%q line=%d", summary.file, summary.line)
		}
		if summary.noteType != string(gitlab.DiffNote) {
			t.Errorf("expected DiffNote type, got %q", summary.noteType)
		}
	})

	t.Run("updated_at is the newest note update with created_at fallback", func(t *testing.T) {
		later := discussionTestTime(t, "2026-07-05T09:00:00Z")
		summary, ok := summarizeDiscussion(&gitlab.Discussion{
			ID: discussionIDUnresolvedAlice,
			Notes: []*gitlab.Note{
				{Author: gitlab.NoteAuthor{Username: "alice"}, CreatedAt: &created, UpdatedAt: &updated, Resolvable: true},
				{Author: gitlab.NoteAuthor{Username: "bob"}, CreatedAt: &later}, // nil UpdatedAt falls back to CreatedAt
			},
		})
		if !ok {
			t.Fatal("expected ok")
		}
		if !summary.updatedAt.Equal(later) {
			t.Errorf("expected updatedAt %v, got %v", later, summary.updatedAt)
		}
		if !summary.createdAt.Equal(created) {
			t.Errorf("expected createdAt from first note, got %v", summary.createdAt)
		}
		if summary.state != "unresolved" {
			t.Errorf("expected unresolved, got %q", summary.state)
		}
	})

	t.Run("non-resolvable thread has state none", func(t *testing.T) {
		summary, ok := summarizeDiscussion(&gitlab.Discussion{
			ID:    discussionIDSystem,
			Notes: []*gitlab.Note{{Author: gitlab.NoteAuthor{Username: "alice"}, CreatedAt: &created}},
		})
		if !ok {
			t.Fatal("expected ok")
		}
		if summary.state != "none" || summary.resolvable {
			t.Errorf("expected non-resolvable none state, got %+v", summary)
		}
	})

	t.Run("position falls back to old path and line", func(t *testing.T) {
		summary, _ := summarizeDiscussion(&gitlab.Discussion{
			ID: discussionIDUnresolvedAlice,
			Notes: []*gitlab.Note{{
				Author:    gitlab.NoteAuthor{Username: "alice"},
				CreatedAt: &created,
				Position:  &gitlab.NotePosition{OldPath: "legacy.go", OldLine: 7},
			}},
		})
		if summary.file != "legacy.go" || summary.line != 7 {
			t.Errorf("expected old path fallback, got file=%q line=%d", summary.file, summary.line)
		}
	})

	t.Run("all-system detection", func(t *testing.T) {
		summary, _ := summarizeDiscussion(&gitlab.Discussion{
			ID:    discussionIDSystem,
			Notes: []*gitlab.Note{{Author: gitlab.NoteAuthor{Username: "alice"}, System: true, CreatedAt: &created}},
		})
		if !summary.system {
			t.Error("expected all-system discussion to be flagged system")
		}
	})

	t.Run("nil and empty discussions are skipped", func(t *testing.T) {
		if _, ok := summarizeDiscussion(nil); ok {
			t.Error("expected ok=false for nil discussion")
		}
		if _, ok := summarizeDiscussion(&gitlab.Discussion{ID: discussionIDSystem}); ok {
			t.Error("expected ok=false for discussion without notes")
		}
	})
}

func TestDiscussionPreviewFlattensAndTruncates(t *testing.T) {
	long := strings.Repeat("word ", 30) // 150 runes flattened to 149
	got := discussionPreview("line one\n\n  " + long)
	if strings.Contains(got, "\n") {
		t.Errorf("expected single-line preview, got %q", got)
	}
	if runes := []rune(got); len(runes) != discussionPreviewLimit+1 || runes[len(runes)-1] != '…' {
		t.Errorf("expected %d runes ending with ellipsis, got %d: %q", discussionPreviewLimit+1, len(runes), got)
	}

	if got := discussionPreview("short body"); got != "short body" {
		t.Errorf("expected short body unchanged, got %q", got)
	}
	if got := discussionPreview(""); got != "" {
		t.Errorf("expected empty preview for empty body, got %q", got)
	}
}

func TestFilterDiscussionSummaries(t *testing.T) {
	summaries := []discussionSummary{
		{id: "1", author: "Alice", state: "unresolved", resolvable: true},
		{id: "2", author: "bob", state: "resolved", resolvable: true, resolved: true},
		{id: "3", author: "carol", state: "none"},
		{id: "4", author: "dave", state: "none", system: true},
	}

	ids := func(filtered []discussionSummary) string {
		var out []string
		for _, summary := range filtered {
			out = append(out, summary.id)
		}
		return strings.Join(out, ",")
	}

	cases := []struct {
		name          string
		state, author string
		system        bool
		want          string
	}{
		{name: "all without system", state: "all", want: "1,2,3"},
		{name: "all with system", state: "all", system: true, want: "1,2,3,4"},
		{name: "unresolved excludes non-resolvable", state: "unresolved", want: "1"},
		{name: "resolved", state: "resolved", want: "2"},
		{name: "author with at-prefix case-insensitive", state: "all", author: "@ALICE", want: "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(filterDiscussionSummaries(summaries, tc.state, tc.author, tc.system)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSortAndPageDiscussionSummaries(t *testing.T) {
	base := discussionTestTime(t, "2026-07-01T00:00:00Z")
	summaries := make([]discussionSummary, 0, 5)
	for i := range 5 {
		summaries = append(summaries, discussionSummary{
			id:        fmt.Sprintf("%d", i+1),
			createdAt: base.Add(time.Duration(i) * time.Hour),
			updatedAt: base.Add(time.Duration(5-i) * time.Hour),
		})
	}

	sortDiscussionSummaries(summaries, "updated_at", "desc")
	if summaries[0].id != "1" || summaries[4].id != "5" {
		t.Errorf("expected updated_at desc order 1..5, got %+v", summaries)
	}

	sortDiscussionSummaries(summaries, "created_at", "asc")
	rows, paging := pageDiscussionSummaries(summaries, 2, 2)
	if len(rows) != 2 || rows[0].id != "3" || rows[1].id != "4" {
		t.Errorf("expected page 2 to hold items 3 and 4, got %+v", rows)
	}
	if paging.totalItems != 5 || paging.totalPages != 3 || paging.page != 2 {
		t.Errorf("expected exact paging totals, got %+v", paging)
	}

	rows, paging = pageDiscussionSummaries(summaries, 9, 2)
	if len(rows) != 0 || paging.totalItems != 5 {
		t.Errorf("expected empty page past the end with exact totals, got %d rows, %+v", len(rows), paging)
	}
}

func TestMRDiscussionsCommandAxiTOON(t *testing.T) {
	server, queries := newDiscussionListTestServer(t, discussionListStubJSON())

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussions", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	for _, query := range *queries {
		for _, forbidden := range []string{"state", "author", "resolved"} {
			if _, sent := query[forbidden]; sent {
				t.Errorf("filtering must be client-side, but query param %q was sent", forbidden)
			}
		}
	}

	if !strings.Contains(got, "discussions[2]{id,author,state,notes,updated_at,preview}:") {
		t.Errorf("expected compact tabular header, got:\n%s", got)
	}
	if !strings.Contains(got, "6f9a1c2d,alice,unresolved,2,") {
		t.Errorf("expected short-id row for alice's thread, got:\n%s", got)
	}
	if strings.Contains(got, "mona") || strings.Contains(got, "added 3 commits") {
		t.Errorf("expected resolved and system threads filtered out by default, got:\n%s", got)
	}
	if !strings.Contains(got, "count: 2 of 2 total") {
		t.Errorf("expected exact filtered count, got:\n%s", got)
	}
	if !strings.Contains(got, "Run `mr discussion 123 <id> --project group/project` for the full conversation") {
		t.Errorf("expected thread-view hint carrying --project, got:\n%s", got)
	}
}

func TestMRDiscussionsFilterAndSortFlags(t *testing.T) {
	t.Run("state all with system sorted by updated desc", func(t *testing.T) {
		server, _ := newDiscussionListTestServer(t, discussionListStubJSON())

		got, err := executeDiscussionCommand(t, commandModeAxi, server.URL,
			"mr", "discussions", "123", "--state", "all", "--system", "--order-by", "updated_at", "--sort", "desc")
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		if !strings.Contains(got, "discussions[4]{") {
			t.Errorf("expected all 4 threads, got:\n%s", got)
		}
		order := []string{"ffee11cc", "6f9b0000", "6f9a1c2d", "aa11bb22"}
		last := -1
		for _, id := range order {
			index := strings.Index(got, id)
			if index == -1 {
				t.Fatalf("expected id %s in output:\n%s", id, got)
			}
			if index < last {
				t.Errorf("expected updated_at desc order %v, got:\n%s", order, got)
			}
			last = index
		}
	})

	t.Run("author filter", func(t *testing.T) {
		server, _ := newDiscussionListTestServer(t, discussionListStubJSON())

		got, err := executeDiscussionCommand(t, commandModeAxi, server.URL,
			"mr", "discussions", "123", "--state", "all", "--author", "@bob")
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}

		if !strings.Contains(got, "discussions[1]{") || !strings.Contains(got, "6f9b0000,bob,") {
			t.Errorf("expected only bob's thread, got:\n%s", got)
		}
	})
}

func TestMRDiscussionsPagingHintCarriesFilters(t *testing.T) {
	discussions := make([]string, 0, 5)
	for i := range 5 {
		id := fmt.Sprintf("%08d", i+1) + strings.Repeat("0", 32)
		discussions = append(discussions, discussionJSON(id,
			discussionNoteJSON(int64(900+i), "alice", fmt.Sprintf("Thread %d", i+1), true, false, false,
				fmt.Sprintf("2026-07-0%dT08:00:00Z", i+1), fmt.Sprintf("2026-07-0%dT08:00:00Z", i+1)),
		))
	}
	server, _ := newDiscussionListTestServer(t, "["+strings.Join(discussions, ",")+"]")

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL,
		"mr", "discussions", "123", "--limit", "2", "--order-by", "updated_at", "--sort", "desc")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(got, "count: 2 of 5 total") {
		t.Errorf("expected exact filtered totals, got:\n%s", got)
	}
	if !strings.Contains(got, "Run `mr discussions 123 --page 2 --order-by updated_at --sort desc --limit 2 --project group/project` for the next page") {
		t.Errorf("expected next-page hint carrying filters and --project, got:\n%s", got)
	}
}

func TestMRDiscussionsEmptyAfterFilter(t *testing.T) {
	body := "[" + strings.Join([]string{
		discussionJSON(discussionIDSystem,
			`{"id":905,"type":"","body":"added 3 commits","author":{"username":"alice"},"system":true,"created_at":"2026-07-05T08:00:00Z","updated_at":"2026-07-05T08:00:00Z","resolvable":false,"resolved":false}`,
		),
	}, ",") + "]"
	server, _ := newDiscussionListTestServer(t, body)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussions", "123")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(got, "discussions[0]:") {
		t.Errorf("expected explicit empty list, got:\n%s", got)
	}
	if !strings.Contains(got, "count: 0 of 0 total") {
		t.Errorf("expected explicit zero count, got:\n%s", got)
	}
	if !strings.Contains(got, "--state all") {
		t.Errorf("expected filter-relaxation hint, got:\n%s", got)
	}
	if !strings.Contains(got, "1 system discussion(s) were excluded") {
		t.Errorf("expected excluded-system hint, got:\n%s", got)
	}
}

func TestMRDiscussionsUsageErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "bad state", args: []string{"--state", "bogus"}, want: "all, resolved, unresolved"},
		{name: "bad order-by", args: []string{"--order-by", "title"}, want: "created_at, updated_at"},
		{name: "bad sort", args: []string{"--sort", "up"}, want: "asc, desc"},
		{name: "bad fields", args: []string{"--fields", "bogus"}, want: "type, file, line, created_at, id_full"},
		{name: "bad limit", args: []string{"--limit", "0"}, want: "--limit must be at least 1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"mr", "discussions", "123"}, tc.args...)
			_, err := executeDiscussionCommand(t, commandModeAxi, "http://127.0.0.1:1", args...)
			if err == nil {
				t.Fatal("expected a usage error")
			}
			if exitCodeForError(err) != 2 {
				t.Errorf("expected exit code 2, got %d for %v", exitCodeForError(err), err)
			}

			var out bytes.Buffer
			writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("expected rendered error to mention %q, got:\n%s", tc.want, out.String())
			}
		})
	}
}

func TestMRDiscussionViewFullID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath + "/" + discussionIDResolved:
			fmt.Fprint(w, discussionJSON(discussionIDResolved,
				fmt.Sprintf(
					`{"id":901,"type":"DiffNote","body":"Please rename.\nShadows a builtin.","author":{"username":"alice"},"system":false,"created_at":"2026-07-01T08:00:00Z","updated_at":"2026-07-01T08:00:00Z","resolvable":true,"resolved":true,"resolved_by":{"username":"mona"},"resolved_at":"2026-07-02T10:00:00Z","position":{"new_path":"internal/cli/mr.go","new_line":42}}`,
				),
				discussionNoteJSON(902, "bob", "Done in a3f9c", true, true, false, "2026-07-01T09:00:00Z", "2026-07-01T09:00:00Z"),
			))
		default:
			t.Errorf("unexpected request path %s (full-ID view must not hit the list endpoint)", r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", discussionIDResolved)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(got, "discussion:") || !strings.Contains(got, "id: "+discussionIDResolved) {
		t.Errorf("expected discussion object with the full id, got:\n%s", got)
	}
	if !strings.Contains(got, "state: resolved") || !strings.Contains(got, "resolved_by: mona") {
		t.Errorf("expected resolution metadata, got:\n%s", got)
	}
	if !strings.Contains(got, "file: internal/cli/mr.go") || !strings.Contains(got, "line: 42") {
		t.Errorf("expected diff position, got:\n%s", got)
	}
	if !strings.Contains(got, "notes[2]{id,author,created_at,updated_at,system,body}:") {
		t.Errorf("expected tabular notes, got:\n%s", got)
	}
	if !strings.Contains(got, `Please rename.\nShadows a builtin.`) {
		t.Errorf("expected the full multi-line body, got:\n%s", got)
	}
	if strings.Contains(got, "help[") {
		t.Errorf("thread view is self-contained and must carry no hints, got:\n%s", got)
	}
}

func TestMRDiscussionViewShortPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case discussionsListPath:
			fmt.Fprint(w, discussionListStubJSON())
		default:
			t.Errorf("prefix resolution must use the list endpoint only, got %s", r.URL.EscapedPath())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	got, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", "6f9a")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if !strings.Contains(got, "id: "+discussionIDUnresolvedAlice) {
		t.Errorf("expected the prefix to resolve to the full id, got:\n%s", got)
	}
	if !strings.Contains(got, "notes[2]{") {
		t.Errorf("expected both notes of the thread, got:\n%s", got)
	}
}

func TestMRDiscussionViewRefErrors(t *testing.T) {
	t.Run("ambiguous prefix", func(t *testing.T) {
		server, _ := newDiscussionListTestServer(t, discussionListStubJSON())

		_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", "6f9")
		if !errors.Is(err, errAmbiguousDiscussionRef) {
			t.Fatalf("expected errAmbiguousDiscussionRef, got %v", err)
		}
		if exitCodeForError(err) != 2 {
			t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
		}
		if !strings.Contains(err.Error(), "2 discussions match") {
			t.Errorf("expected match count in message, got %q", err.Error())
		}

		var out bytes.Buffer
		writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
		if !strings.Contains(out.String(), "code: ambiguous_discussion_ref") {
			t.Errorf("expected ambiguous_discussion_ref code, got:\n%s", out.String())
		}
	})

	t.Run("no match", func(t *testing.T) {
		server, _ := newDiscussionListTestServer(t, discussionListStubJSON())

		_, err := executeDiscussionCommand(t, commandModeAxi, server.URL, "mr", "discussion", "123", "abcdef01")
		if !errors.Is(err, errDiscussionNotFound) {
			t.Fatalf("expected errDiscussionNotFound, got %v", err)
		}
		if exitCodeForError(err) != 1 {
			t.Errorf("expected exit code 1, got %d", exitCodeForError(err))
		}

		var out bytes.Buffer
		writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
		if !strings.Contains(out.String(), "code: discussion_not_found") {
			t.Errorf("expected discussion_not_found code, got:\n%s", out.String())
		}
		if strings.Contains(out.String(), server.URL) {
			t.Errorf("server URL must not leak into agent-facing errors:\n%s", out.String())
		}
	})

	t.Run("invalid reference", func(t *testing.T) {
		_, err := executeDiscussionCommand(t, commandModeAxi, "http://127.0.0.1:1", "mr", "discussion", "123", "zz")
		if !errors.Is(err, errInvalidDiscussionRef) {
			t.Fatalf("expected errInvalidDiscussionRef, got %v", err)
		}
		if exitCodeForError(err) != 2 {
			t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
		}
	})
}

func TestWriteDiscussionListStandardModes(t *testing.T) {
	created := discussionTestTime(t, "2026-07-01T08:00:00Z")
	updated := discussionTestTime(t, "2026-07-03T12:00:00Z")
	summaries := []discussionSummary{
		{
			id: discussionIDUnresolvedAlice, author: "alice", state: "unresolved", resolvable: true,
			notesCount: 2, noteType: "DiffNote", createdAt: created, updatedAt: updated,
			preview: "This branch check looks inverted",
		},
		{
			id: discussionIDResolved, author: "mona", state: "resolved", resolvable: true, resolved: true,
			notesCount: 1, noteType: "DiscussionNote", createdAt: created, updatedAt: created,
			preview: "Typo in the docstring",
		},
	}
	paging := mrListPaging{page: 1, totalItems: 5, totalPages: 3}

	t.Run("text table", func(t *testing.T) {
		var out bytes.Buffer
		if err := writeDiscussionList(&out, "text", commandModeStandard, summaries, paging, nil, nil); err != nil {
			t.Fatalf("writeDiscussionList returned error: %v", err)
		}
		got := out.String()

		for _, want := range []string{"PREVIEW", "6f9a1c2d", "unresolved", "2026-07-03", "2 of 5 discussions (page 1 of 3)"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected table output to contain %q, got:\n%s", want, got)
			}
		}
		if strings.Contains(got, discussionIDUnresolvedAlice) {
			t.Errorf("expected table to shorten ids, got:\n%s", got)
		}
	})

	t.Run("empty text table", func(t *testing.T) {
		var out bytes.Buffer
		if err := writeDiscussionList(&out, "text", commandModeStandard, nil, mrListPaging{page: 1}, nil, nil); err != nil {
			t.Fatalf("writeDiscussionList returned error: %v", err)
		}
		if !strings.Contains(out.String(), "No discussion threads found") {
			t.Errorf("expected empty-state message, got:\n%s", out.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var out bytes.Buffer
		if err := writeDiscussionList(&out, "json", commandModeStandard, summaries, paging, nil, nil); err != nil {
			t.Fatalf("writeDiscussionList returned error: %v", err)
		}
		got := out.String()

		for _, want := range []string{`"discussions": [`, fmt.Sprintf("%q", discussionIDUnresolvedAlice), `"total": 5`, `"total_pages": 3`} {
			if !strings.Contains(got, want) {
				t.Errorf("expected json output to contain %s, got:\n%s", want, got)
			}
		}
	})
}

func TestMRParentDispatchDiscussionsRedirects(t *testing.T) {
	_, err := executeDiscussionCommand(t, commandModeAxi, "http://127.0.0.1:1", "mr", "123", "discussions")
	if err == nil {
		t.Fatal("expected a usage error redirect")
	}
	if exitCodeForError(err) != 2 {
		t.Errorf("expected exit code 2, got %d", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "mr discussions !123") {
		t.Errorf("expected redirect hint to the subcommand, got:\n%s", out.String())
	}
}
