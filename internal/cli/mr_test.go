package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/shabashab/community-gitlab-cli/internal/repo"
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

	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
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
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRCommandRejectsExtraArguments(t *testing.T) {
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"mr", "!123", "view", "extra"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want usage error for extra arguments")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRListRejectsUnknownFields(t *testing.T) {
	cmd, _ := newRootCommand("gl-axi", "test", "test", commandModeAxi)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"mr", "list", "--fields", "bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want usage error for unknown field")
	}
	if !strings.Contains(err.Error(), `unknown field "bogus"`) {
		t.Fatalf("Execute error = %v, want unknown field message", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestParseMRListFieldsCanonicalOrder(t *testing.T) {
	fields, err := parseMRListFields("web_url, draft")
	if err != nil {
		t.Fatalf("parseMRListFields returned error: %v", err)
	}
	if len(fields) != 2 || fields[0] != "draft" || fields[1] != "web_url" {
		t.Fatalf("parseMRListFields = %v, want [draft web_url]", fields)
	}

	if fields, err := parseMRListFields("title"); err != nil || fields != nil {
		t.Fatalf("parseMRListFields(default field) = %v, %v, want nil, nil", fields, err)
	}
}

func executeMRRootCommand(t *testing.T, baseURL string, extraArgs ...string) string {
	t.Helper()

	out, err := executeMRRootCommandErr(t, baseURL, extraArgs...)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	return out
}

func executeMRRootCommandErr(t *testing.T, baseURL string, extraArgs ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
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

	err := cmd.Execute()

	return out.String(), err
}

// stubCurrentBranch overrides the currentBranchFunc test seam for the duration
// of the test, so cli tests never shell out to the real repository.
func stubCurrentBranch(t *testing.T, branch string, err error) {
	t.Helper()

	original := currentBranchFunc
	currentBranchFunc = func(context.Context, string) (string, error) { return branch, err }
	t.Cleanup(func() { currentBranchFunc = original })
}

// newMRCurrentTestServer stubs the two endpoints a "current" ref touches: the
// list lookup by source branch (query captured) and the single-MR GET (path
// recorded). listBody is the raw JSON array served by the list endpoint.
func newMRCurrentTestServer(t *testing.T, listBody string) (*httptest.Server, *url.Values, *string) {
	t.Helper()

	var listQuery url.Values
	var viewPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.EscapedPath() {
		case "/api/v4/projects/group%2Fproject/merge_requests":
			listQuery = r.URL.Query()
			fmt.Fprint(w, listBody)
		case "/api/v4/projects/group%2Fproject/merge_requests/123":
			viewPath = r.URL.EscapedPath()
			fmt.Fprint(w, mergeRequestJSON(123, "short description"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &listQuery, &viewPath
}

func TestResolveMergeRequestRefDelegatesNumeric(t *testing.T) {
	cmd := &cobra.Command{}

	for ref, want := range map[string]int64{"123": 123, "!123": 123} {
		got, err := resolveMergeRequestRef(cmd, &rootOptions{}, &projectOptions{}, ref)
		if err != nil {
			t.Fatalf("resolveMergeRequestRef(%q) returned error: %v", ref, err)
		}
		if got != want {
			t.Fatalf("resolveMergeRequestRef(%q) = %d, want %d", ref, got, want)
		}
	}

	_, err := resolveMergeRequestRef(cmd, &rootOptions{}, &projectOptions{}, "abc")
	if !errors.Is(err, errInvalidMergeRequestRef) {
		t.Fatalf("resolveMergeRequestRef(abc) error = %v, want errInvalidMergeRequestRef", err)
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
}

func TestMRCommandDispatchesCurrent(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	server, listQuery, viewPath := newMRCurrentTestServer(t, "["+mergeRequestJSON(123, "short description")+"]")
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "current", "--full")

	want := map[string]string{
		"source_branch": "feature/search",
		"state":         "opened",
		"per_page":      "10",
	}
	for key, value := range want {
		if got := listQuery.Get(key); got != value {
			t.Fatalf("list query param %s = %q, want %q", key, got, value)
		}
	}
	if *viewPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("view path = %q, want resolved single merge request path", *viewPath)
	}
	if !strings.Contains(out, `"iid": 123`) {
		t.Fatalf("output = %q, want iid fragment", out)
	}
}

func TestMRCommandDispatchesBangCurrent(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	server, listQuery, viewPath := newMRCurrentTestServer(t, "["+mergeRequestJSON(123, "short description")+"]")
	defer server.Close()

	executeMRRootCommand(t, server.URL, "!current")

	if got := listQuery.Get("source_branch"); got != "feature/search" {
		t.Fatalf("list query param source_branch = %q, want feature/search", got)
	}
	if *viewPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("view path = %q, want resolved single merge request path", *viewPath)
	}
}

func TestMRViewCurrentResolvesSourceBranch(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	server, listQuery, viewPath := newMRCurrentTestServer(t, "["+mergeRequestJSON(123, "short description")+"]")
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "view", "current")

	if got := listQuery.Get("source_branch"); got != "feature/search" {
		t.Fatalf("list query param source_branch = %q, want feature/search", got)
	}
	if *viewPath != "/api/v4/projects/group%2Fproject/merge_requests/123" {
		t.Fatalf("view path = %q, want resolved single merge request path", *viewPath)
	}
	if !strings.Contains(out, `"iid": 123`) {
		t.Fatalf("output = %q, want iid fragment", out)
	}
}

func TestMRCurrentNoOpenMergeRequest(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	server, _, _ := newMRCurrentTestServer(t, "[]")
	defer server.Close()

	_, err := executeMRRootCommandErr(t, server.URL, "current")
	if !errors.Is(err, errNoCurrentMergeRequest) {
		t.Fatalf("Execute error = %v, want errNoCurrentMergeRequest", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	got := out.String()
	if !strings.Contains(got, "code: no_current_merge_request") {
		t.Fatalf("rendered error = %q, want no_current_merge_request code", got)
	}
	if !strings.Contains(got, "--source-branch feature/search") {
		t.Fatalf("rendered error = %q, want source-branch hint", got)
	}
	if !strings.Contains(got, "--project group/project") {
		t.Fatalf("rendered error = %q, want hint carrying --project", got)
	}
	if strings.Contains(got, server.URL) {
		t.Fatalf("rendered error = %q, must not leak the server URL", got)
	}
}

func TestMRCurrentAmbiguousListsCandidates(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	listBody := "[" + mergeRequestJSON(123, "one") + "," + mergeRequestJSON(120, "two") + "]"
	server, _, _ := newMRCurrentTestServer(t, listBody)
	defer server.Close()

	_, err := executeMRRootCommandErr(t, server.URL, "current")
	if !errors.Is(err, errAmbiguousCurrentMergeRequest) {
		t.Fatalf("Execute error = %v, want errAmbiguousCurrentMergeRequest", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "!123") || !strings.Contains(err.Error(), "!120") {
		t.Fatalf("error message = %q, want both candidate iids", err.Error())
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	got := out.String()
	if !strings.Contains(got, "code: ambiguous_current_merge_request") {
		t.Fatalf("rendered error = %q, want ambiguous_current_merge_request code", got)
	}
	if !strings.Contains(got, "mr view 123 --project group/project") {
		t.Fatalf("rendered error = %q, want explicit-iid hint carrying --project", got)
	}
}

func TestMRCurrentDetachedHeadFails(t *testing.T) {
	stubCurrentBranch(t, "", fmt.Errorf("%w: detached HEAD", repo.ErrNoCurrentBranch))

	handlerCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	_, err := executeMRRootCommandErr(t, server.URL, "current")
	if !errors.Is(err, errMissingCurrentBranch) {
		t.Fatalf("Execute error = %v, want errMissingCurrentBranch", err)
	}
	if exitCodeForError(err) != 1 {
		t.Fatalf("exitCodeForError = %d, want 1 for runtime error", exitCodeForError(err))
	}
	if handlerCalled {
		t.Fatal("GitLab API was called although the branch lookup failed")
	}

	var out bytes.Buffer
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", err)
	if !strings.Contains(out.String(), "code: missing_current_branch") {
		t.Fatalf("rendered error = %q, want missing_current_branch code", out.String())
	}
}

func TestMRCurrentUpdateRedirect(t *testing.T) {
	stubCurrentBranch(t, "feature/search", nil)

	server, _, _ := newMRCurrentTestServer(t, "["+mergeRequestJSON(123, "one")+"]")
	defer server.Close()

	_, err := executeMRRootCommandErr(t, server.URL, "current", "update")
	if err == nil {
		t.Fatal("Execute returned nil error, want update-redirect usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2 for usage error", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "!123") {
		t.Fatalf("error message = %q, want the resolved iid in the redirect", err.Error())
	}
}

func TestWriteMergeRequestAxiTOONTruncatesDescription(t *testing.T) {
	mergeRequest := testMergeRequest(123, strings.Repeat("x", 600))

	var out bytes.Buffer
	if err := writeMergeRequest(&out, "toon", commandModeAxi, mergeRequest, false, nil); err != nil {
		t.Fatalf("writeMergeRequest returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_request:\n  iid: 123") {
		t.Fatalf("output = %q, want nested merge_request object", got)
	}
	if !strings.Contains(got, "pipeline_status: success") {
		t.Fatalf("output = %q, want pipeline_status in compact view", got)
	}
	if !strings.Contains(got, "(truncated, 600 chars total)") {
		t.Fatalf("output = %q, want truncation marker", got)
	}
	if !strings.Contains(got, "help[1]: Run `mr view 123 --full`") {
		t.Fatalf("output = %q, want --full escape hatch hint", got)
	}
}

func TestWriteMergeRequestAxiTOONShortDescriptionOmitsHelp(t *testing.T) {
	mergeRequest := testMergeRequest(123, "short description")

	var out bytes.Buffer
	if err := writeMergeRequest(&out, "toon", commandModeAxi, mergeRequest, false, nil); err != nil {
		t.Fatalf("writeMergeRequest returned error: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "help[") {
		t.Fatalf("output = %q, want no help hints when nothing is truncated", got)
	}
	if strings.Contains(got, "(truncated,") {
		t.Fatalf("output = %q, want no truncation marker for short description", got)
	}
}

func TestWriteMergeRequestAxiTOONFullFields(t *testing.T) {
	description := strings.Repeat("x", 600)
	mergeRequest := testMergeRequest(123, description)

	var out bytes.Buffer
	if err := writeMergeRequest(&out, "toon", commandModeAxi, mergeRequest, true, nil); err != nil {
		t.Fatalf("writeMergeRequest returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "changes_count: ") {
		t.Fatalf("output = %q, want full view fields", got)
	}
	if !strings.Contains(got, "labels[2]: backend,search") {
		t.Fatalf("output = %q, want labels as TOON inline array", got)
	}
	if !strings.Contains(got, "assignees[2]: mona,hubot") {
		t.Fatalf("output = %q, want assignees as TOON inline array", got)
	}
	if strings.Contains(got, "(truncated,") {
		t.Fatalf("output = %q, want complete description without truncation hint", got)
	}
	if !strings.Contains(got, description) {
		t.Fatalf("output = %q, want complete description", got)
	}
	if strings.Contains(got, "help[") {
		t.Fatalf("output = %q, want no help hints on the self-contained full view", got)
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
	}, nil, nil)
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_requests[2]{iid,title,state,author}:") {
		t.Fatalf("output = %q, want compact TOON list header", got)
	}
	if !strings.Contains(got, "  123,") || !strings.Contains(got, "  120,") {
		t.Fatalf("output = %q, want both rows", got)
	}
	if !strings.Contains(got, "count: 2 of 57 total") {
		t.Fatalf("output = %q, want count line", got)
	}
	if !strings.Contains(got, "Run `mr view <iid>` for details") {
		t.Fatalf("output = %q, want view hint", got)
	}
	if !strings.Contains(got, "Run `mr list --page 2` for the next page") {
		t.Fatalf("output = %q, want next-page hint", got)
	}
}

func TestWriteMergeRequestListAxiTOONExtraFields(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, mergeRequests, mrListPaging{
		page: 1, totalItems: 1, totalPages: 1,
	}, []string{"source_branch", "updated_at"}, nil)
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_requests[1]{iid,title,state,author,source_branch,updated_at}:") {
		t.Fatalf("output = %q, want header with extra fields", got)
	}
	if strings.Contains(got, "for the next page") {
		t.Fatalf("output = %q, want no page hint on a single page", got)
	}
}

func TestWriteMergeRequestListAxiCarriesProjectFlag(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, mergeRequests, mrListPaging{
		page: 1, totalItems: 1, totalPages: 1,
	}, nil, &mrHintContext{project: "group/project"})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Run `mr view <iid> --project group/project` for details") {
		t.Fatalf("output = %q, want hint carrying --project", out.String())
	}
}

func TestWriteMergeRequestListAxiTOONEmpty(t *testing.T) {
	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, nil, mrListPaging{page: 1}, nil, nil)
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "merge_requests[0]:") {
		t.Fatalf("output = %q, want empty TOON list header", got)
	}
	if !strings.Contains(got, "count: 0 of 0 total") {
		t.Fatalf("output = %q, want zero count line", got)
	}
	if !strings.Contains(got, "No merge requests matched") {
		t.Fatalf("output = %q, want definitive empty-state hint", got)
	}
}

func TestWriteMergeRequestListAxiTOONUnknownTotal(t *testing.T) {
	mergeRequests := []*gitlab.BasicMergeRequest{
		&testMergeRequest(123, "one").BasicMergeRequest,
	}

	var out bytes.Buffer
	err := writeMergeRequestList(&out, "toon", commandModeAxi, mergeRequests, mrListPaging{page: 1}, nil, &mrHintContext{limit: 1})
	if err != nil {
		t.Fatalf("writeMergeRequestList returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "count: 1 of unknown total") {
		t.Fatalf("output = %q, want unknown-total count line", got)
	}
	if !strings.Contains(got, "More results may exist") {
		t.Fatalf("output = %q, want full-page pagination hint", got)
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
	}, nil, nil)
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
	err := writeMergeRequestList(&out, "text", commandModeStandard, nil, mrListPaging{page: 1}, nil, nil)
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
	}, nil, nil)
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
	writeCommandError(&out, commandModeAxi, "toon", "gl-axi", fmt.Errorf("wrap: %w", errInvalidMergeRequestRef))

	if !strings.Contains(out.String(), "code: invalid_merge_request_ref") {
		t.Fatalf("output = %q, want invalid_merge_request_ref code", out.String())
	}
}

func TestTruncateDescription(t *testing.T) {
	if got, truncated := truncateDescription("short", 500, commandModeAxi); got != "short" || truncated {
		t.Fatalf("truncateDescription(short) = %q, %t, want unchanged and not truncated", got, truncated)
	}

	long := strings.Repeat("é", 501)
	got, truncated := truncateDescription(long, 500, commandModeAxi)
	if !truncated {
		t.Fatal("truncateDescription reported not truncated for a long value")
	}
	if !strings.HasPrefix(got, strings.Repeat("é", 500)) {
		t.Fatalf("truncateDescription output does not preserve first 500 runes: %q", got[:50])
	}
	if !strings.Contains(got, "(truncated, 501 chars total)") {
		t.Fatalf("truncateDescription output = %q, want size marker with rune total", got)
	}
	if strings.Contains(got, strings.Repeat("é", 501)) {
		t.Fatalf("truncateDescription output still contains the full value")
	}

	standard, _ := truncateDescription(long, 500, commandModeStandard)
	if !strings.Contains(standard, "use --full for the complete description") {
		t.Fatalf("standard-mode marker = %q, want inline --full hint", standard)
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
