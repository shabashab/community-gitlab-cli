package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestParseMergeRequestRef(t *testing.T) {
	valid := map[string]int64{
		"!123": 123,
		"123":  123,
		" !5 ": 5,
	}
	for ref, want := range valid {
		got, err := parseMergeRequestRef(ref)
		if err != nil {
			t.Fatalf("parseMergeRequestRef(%q) returned error: %v", ref, err)
		}
		if got != want {
			t.Fatalf("parseMergeRequestRef(%q) = %d, want %d", ref, got, want)
		}
	}

	for _, ref := range []string{"abc", "!0", "-3", "", "!12a"} {
		if _, err := parseMergeRequestRef(ref); !errors.Is(err, errInvalidMergeRequestRef) {
			t.Fatalf("parseMergeRequestRef(%q) error = %v, want errInvalidMergeRequestRef", ref, err)
		}
	}
}

func TestRunMRViewFetchesMergeRequest(t *testing.T) {
	var gotPath string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotToken = r.Header.Get("Private-Token")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestJSON(123, "short description"))
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runMRView(cmd, &rootOptions{
		gitlabToken:   "test-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	}, &projectOptions{project: "group/project"}, &mrViewOptions{}, 123)
	if err != nil {
		t.Fatalf("runMRView returned error: %v", err)
	}
	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("request path = %q, want /api/v4/projects/group%%2Fproject/merge_requests/123", gotPath)
	}
	if gotToken != "test-token" {
		t.Fatalf("Private-Token header = %q, want test-token", gotToken)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"iid": 123`)) {
		t.Fatalf("runMRView output = %q, want iid fragment", out.String())
	}
}

func TestRunMRListSendsFilterParams(t *testing.T) {
	var gotQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	opts := &mrListOptions{
		state:        "opened",
		search:       "search endpoint",
		labels:       []string{"bug", "backend"},
		author:       "octocat",
		reviewer:     "mona",
		sourceBranch: "feature/search",
		targetBranch: "main",
		draft:        true,
		draftSet:     true,
		milestone:    "16.1",
		orderBy:      "updated_at",
		sort:         "asc",
		limit:        20,
		page:         1,
	}

	err := runMRList(cmd, &rootOptions{
		gitlabToken:   "test-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	}, &projectOptions{project: "group/project"}, opts)
	if err != nil {
		t.Fatalf("runMRList returned error: %v", err)
	}

	want := map[string]string{
		"state":             "opened",
		"search":            "search endpoint",
		"labels":            "bug,backend",
		"author_username":   "octocat",
		"reviewer_username": "mona",
		"source_branch":     "feature/search",
		"target_branch":     "main",
		"draft":             "true",
		"milestone":         "16.1",
		"order_by":          "updated_at",
		"sort":              "asc",
		"per_page":          "20",
		"page":              "1",
	}
	for key, value := range want {
		if got := gotQuery.Get(key); got != value {
			t.Fatalf("query param %s = %q, want %q", key, got, value)
		}
	}
}

func TestMRCommandDispatchesBangRef(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestJSON(123, "short description"))
	}))
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "!123")
	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("request path = %q, want single merge request path", gotPath)
	}
	if !strings.Contains(out, `"iid": 123`) {
		t.Fatalf("output = %q, want iid fragment", out)
	}
}

func TestMRCommandDispatchesPlainIID(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestJSON(123, "short description"))
	}))
	defer server.Close()

	executeMRRootCommand(t, server.URL, "123")
	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("request path = %q, want single merge request path", gotPath)
	}
}

func TestMRCommandBareRunsList(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotQuery = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer server.Close()

	executeMRRootCommand(t, server.URL)
	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests" {
		t.Fatalf("request path = %q, want merge request list path", gotPath)
	}
	if got := gotQuery.Get("state"); got != "opened" {
		t.Fatalf("query param state = %q, want opened", got)
	}
	if got := gotQuery.Get("per_page"); got != "20" {
		t.Fatalf("query param per_page = %q, want 20", got)
	}
}

func TestMRCommandRejectsUnknownAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	cmd := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"mr",
		"--gitlab-token", "test-token",
		"--gitlab-base-url", server.URL,
		"--project", "group/project",
		"!123", "diff",
	})

	err := cmd.Execute()
	if !errors.Is(err, errUnknownMergeRequestAction) {
		t.Fatalf("Execute error = %v, want errUnknownMergeRequestAction", err)
	}
}

func executeMRRootCommand(t *testing.T, baseURL string, extraArgs ...string) string {
	t.Helper()

	var out bytes.Buffer
	cmd := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	args := []string{
		"mr",
		"--gitlab-token", "test-token",
		"--gitlab-base-url", baseURL,
		"--project", "group/project",
		"-o", "json",
	}
	args = append(args, extraArgs...)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	return out.String()
}

func TestWriteMergeRequestAxiTOONTruncatesDescription(t *testing.T) {
	mergeRequest := testMergeRequest(123, strings.Repeat("x", 600))

	var out bytes.Buffer
	if err := writeMergeRequest(&out, "toon", commandModeAxi, mergeRequest, false); err != nil {
		t.Fatalf("writeMergeRequest returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_request{iid,title,state,draft,author,source_branch,target_branch,detailed_merge_status,has_conflicts,user_notes_count,updated_at,web_url}:") {
		t.Fatalf("output = %q, want compact TOON header", got)
	}
	if !strings.Contains(got, "(truncated, 600 chars total — use --full") {
		t.Fatalf("output = %q, want truncation hint", got)
	}
	if !strings.Contains(got, "next: ") {
		t.Fatalf("output = %q, want next hint", got)
	}
}

func TestWriteMergeRequestAxiTOONFullFields(t *testing.T) {
	description := strings.Repeat("x", 600)
	mergeRequest := testMergeRequest(123, description)

	var out bytes.Buffer
	if err := writeMergeRequest(&out, "toon", commandModeAxi, mergeRequest, true); err != nil {
		t.Fatalf("writeMergeRequest returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "changes_count,pipeline_status") {
		t.Fatalf("output = %q, want full TOON header fields", got)
	}
	if !strings.Contains(got, "backend;search") {
		t.Fatalf("output = %q, want ;-joined labels", got)
	}
	if strings.Contains(got, "(truncated,") {
		t.Fatalf("output = %q, want complete description without truncation hint", got)
	}
	if !strings.Contains(got, description) {
		t.Fatalf("output = %q, want complete description", got)
	}
}

func TestWriteMergeRequestListAxiTOON(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
		&testMergeRequest(120, "two").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, mergeRequests, mrListPaging{
		page:       1,
		totalItems: 57,
		totalPages: 3,
	})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_requests[2]{iid,title,state,draft,author,source_branch,target_branch,updated_at}:") {
		t.Fatalf("output = %q, want TOON list header", got)
	}
	if !strings.Contains(got, "  123,") || !strings.Contains(got, "  120,") {
		t.Fatalf("output = %q, want both rows", got)
	}
	if !strings.Contains(got, "count: 2 of 57 total") {
		t.Fatalf("output = %q, want count line", got)
	}
	if !strings.Contains(got, "next: ") {
		t.Fatalf("output = %q, want next hint", got)
	}
}

func TestWriteMergeRequestListAxiTOONEmpty(t *testing.T) {
	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, nil, mrListPaging{page: 1})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_requests[0]{") {
		t.Fatalf("output = %q, want empty TOON list header", got)
	}
	if !strings.Contains(got, "count: 0 of 0 total") {
		t.Fatalf("output = %q, want zero count line", got)
	}
	if !strings.Contains(got, "No merge requests matched") {
		t.Fatalf("output = %q, want definitive empty-state hint", got)
	}
}

func TestWriteMergeRequestListStandardTable(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
		&testMergeRequest(120, "two").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "text", commandModeStandard, mergeRequests, mrListPaging{
		page:       1,
		totalItems: 57,
		totalPages: 3,
	})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "TITLE") {
		t.Fatalf("output = %q, want table header", got)
	}
	if !strings.Contains(got, "!123") {
		t.Fatalf("output = %q, want bang-formatted iid", got)
	}
	if !strings.Contains(got, "2 of 57 merge requests (page 1 of 3)") {
		t.Fatalf("output = %q, want summary line", got)
	}
}

func TestWriteMergeRequestListStandardTableEmpty(t *testing.T) {
	var out bytes.Buffer
	err := writeMergeRequestList(&out, "text", commandModeStandard, nil, mrListPaging{page: 1})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	if !strings.Contains(out.String(), "No merge requests found") {
		t.Fatalf("output = %q, want empty-state message", out.String())
	}
}

func TestWriteMergeRequestListStandardJSON(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "json", commandModeStandard, mergeRequests, mrListPaging{
		page:       1,
		totalItems: 57,
		totalPages: 3,
	})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `"merge_requests": [`) {
		t.Fatalf("output = %q, want merge_requests array", got)
	}
	if !strings.Contains(got, `"total": 57`) {
		t.Fatalf("output = %q, want total field", got)
	}
}

func TestWriteCommandErrorInvalidMergeRequestRef(t *testing.T) {
	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, fmt.Errorf("wrap: %w", errInvalidMergeRequestRef))

	if !strings.Contains(out.String(), "invalid_merge_request_ref") {
		t.Fatalf("output = %q, want invalid_merge_request_ref code", out.String())
	}
}

func TestTruncateWithHint(t *testing.T) {
	if got := truncateWithHint("short", 500); got != "short" {
		t.Fatalf("truncateWithHint(short) = %q, want unchanged", got)
	}

	long := strings.Repeat("é", 501)
	got := truncateWithHint(long, 500)
	if !strings.HasPrefix(got, strings.Repeat("é", 500)) {
		t.Fatalf("truncateWithHint output does not preserve first 500 runes: %q", got[:50])
	}
	if !strings.Contains(got, "(truncated, 501 chars total — use --full for the complete description)") {
		t.Fatalf("truncateWithHint output = %q, want hint with rune total", got)
	}
	if strings.Contains(got, strings.Repeat("é", 501)) {
		t.Fatalf("truncateWithHint output still contains the full value")
	}
}

func testMergeRequest(iid int64, description string) *gitlab.MergeRequest {
	updatedAt := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)

	return &gitlab.MergeRequest{
		BasicMergeRequest: gitlab.BasicMergeRequest{
			ID:                          1000 + iid,
			IID:                         iid,
			Title:                       "Add search endpoint",
			State:                       "opened",
			Draft:                       false,
			Author:                      &gitlab.BasicUser{Username: "octocat"},
			Assignees:                   []*gitlab.BasicUser{{Username: "mona"}, {Username: "hubot"}},
			Reviewers:                   []*gitlab.BasicUser{{Username: "alice"}},
			SourceBranch:                "feature/search",
			TargetBranch:                "main",
			Labels:                      gitlab.Labels{"backend", "search"},
			Milestone:                   &gitlab.Milestone{Title: "16.1"},
			Description:                 description,
			DetailedMergeStatus:         "mergeable",
			HasConflicts:                false,
			BlockingDiscussionsResolved: true,
			UserNotesCount:              4,
			SHA:                         "f5b0c3d2e1",
			CreatedAt:                   &createdAt,
			UpdatedAt:                   &updatedAt,
			WebURL:                      fmt.Sprintf("https://gitlab.example/group/project/-/merge_requests/%d", iid),
		},
		ChangesCount: "12",
		HeadPipeline: &gitlab.Pipeline{Status: "success"},
	}
}

func mergeRequestJSON(iid int64, description string) string {
	return fmt.Sprintf(`{
		"id": %d,
		"iid": %d,
		"title": "Add search endpoint",
		"state": "opened",
		"draft": false,
		"author": {"username": "octocat"},
		"assignees": [{"username": "mona"}, {"username": "hubot"}],
		"reviewers": [{"username": "alice"}],
		"source_branch": "feature/search",
		"target_branch": "main",
		"labels": ["backend", "search"],
		"milestone": {"title": "16.1"},
		"description": %q,
		"detailed_merge_status": "mergeable",
		"has_conflicts": false,
		"blocking_discussions_resolved": true,
		"user_notes_count": 4,
		"changes_count": "12",
		"head_pipeline": {"status": "success"},
		"sha": "f5b0c3d2e1",
		"created_at": "2026-07-01T08:00:00Z",
		"updated_at": "2026-07-03T12:00:00Z",
		"web_url": "https://gitlab.example/group/project/-/merge_requests/%d"
	}`, 1000+iid, iid, description, iid)
}
