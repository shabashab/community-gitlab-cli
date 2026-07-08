package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestMRApprovalsCommandFetchesStatus(t *testing.T) {
	var gotPath string
	var gotMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotMethod = r.Method

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestApprovalsJSON(123))
	}))
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "approvals", "123")

	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123/approvals" {
		t.Fatalf("request path = %q, want approvals path", gotPath)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("request method = %q, want GET", gotMethod)
	}
	if !strings.Contains(out, `"approvals_left": 1`) {
		t.Fatalf("output = %q, want approvals_left fragment", out)
	}
	if !strings.Contains(out, `"username": "alice"`) {
		t.Fatalf("output = %q, want approved_by user", out)
	}
}

func TestMRParentDispatchesApprovalsAction(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestApprovalsJSON(123))
	}))
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "123", "approvals", "--full")

	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123/approvals" {
		t.Fatalf("request path = %q, want approvals path", gotPath)
	}
	if !strings.Contains(out, `"user_has_approved": true`) {
		t.Fatalf("output = %q, want approval status", out)
	}
	if !strings.Contains(out, `"suggested_approvers": [`) {
		t.Fatalf("output = %q, want full approval metadata", out)
	}
}

func TestMRApproveSendsSHA(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, mergeRequestApprovalsJSON(123))
	}))
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "approve", "123", "--sha", "f5b0c3d2e1")

	if gotPath != "/api/v4/projects/group%2Fproject/merge_requests/123/approve" {
		t.Fatalf("request path = %q, want approve path", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("request method = %q, want POST", gotMethod)
	}
	if gotBody["sha"] != "f5b0c3d2e1" {
		t.Fatalf("request body sha = %v, want f5b0c3d2e1", gotBody["sha"])
	}
	if !strings.Contains(out, `"approvals_required": 2`) {
		t.Fatalf("output = %q, want approval status", out)
	}
}

func TestMRApproveRejectsEmptySHA(t *testing.T) {
	cmd, _ := newRootCommand("gl", "test", "test", commandModeStandard)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"mr", "approve", "123", "--sha", ""})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute returned nil error, want usage error")
	}
	if exitCodeForError(err) != 2 {
		t.Fatalf("exitCodeForError = %d, want 2", exitCodeForError(err))
	}
	if !strings.Contains(err.Error(), "--sha cannot be empty") {
		t.Fatalf("error = %v, want empty sha message", err)
	}
}

func TestMRUnapproveRefreshesStatus(t *testing.T) {
	var gotPaths []string
	var gotMethods []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.EscapedPath())
		gotMethods = append(gotMethods, r.Method)

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/group%2Fproject/merge_requests/123/unapprove":
			fmt.Fprint(w, `{}`)
		case "/api/v4/projects/group%2Fproject/merge_requests/123/approvals":
			fmt.Fprint(w, mergeRequestApprovalsJSON(123))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out := executeMRRootCommand(t, server.URL, "unapprove", "123")

	wantPaths := []string{
		"/api/v4/projects/group%2Fproject/merge_requests/123/unapprove",
		"/api/v4/projects/group%2Fproject/merge_requests/123/approvals",
	}
	if fmt.Sprint(gotPaths) != fmt.Sprint(wantPaths) {
		t.Fatalf("request paths = %v, want %v", gotPaths, wantPaths)
	}
	if fmt.Sprint(gotMethods) != fmt.Sprint([]string{http.MethodPost, http.MethodGet}) {
		t.Fatalf("request methods = %v, want POST then GET", gotMethods)
	}
	if !strings.Contains(out, `"approvals_left": 1`) {
		t.Fatalf("output = %q, want refreshed approval status", out)
	}
}

func TestMRParentApproveAndUnapproveRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("parent approve/unapprove redirects should not call GitLab")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	for _, action := range []string{"approve", "unapprove"} {
		_, err := executeMRRootCommandErr(t, server.URL, "123", action)
		if err == nil {
			t.Fatalf("mr 123 %s returned nil error, want usage redirect", action)
		}
		if exitCodeForError(err) != 2 {
			t.Fatalf("mr 123 %s exit code = %d, want 2", action, exitCodeForError(err))
		}
		if !strings.Contains(err.Error(), action) {
			t.Fatalf("mr 123 %s error = %v, want action in redirect", action, err)
		}
	}
}

func TestWriteMergeRequestApprovalAxiTOONFull(t *testing.T) {
	approvedAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	approvals := &gitlab.MergeRequestApprovals{
		IID:                            123,
		Title:                          "Add search endpoint",
		State:                          "opened",
		MergeStatus:                    "can_be_merged",
		Approved:                       false,
		ApprovalsBeforeMerge:           2,
		ApprovalsRequired:              2,
		ApprovalsLeft:                  1,
		UserHasApproved:                true,
		UserCanApprove:                 false,
		ApprovedBy:                     []*gitlab.MergeRequestApproverUser{{User: &gitlab.BasicUser{Username: "alice"}, ApprovedAt: &approvedAt}},
		SuggestedApprovers:             []*gitlab.BasicUser{{Username: "mona"}},
		Approvers:                      []*gitlab.MergeRequestApproverUser{{User: &gitlab.BasicUser{Username: "alice"}, ApprovedAt: &approvedAt}},
		ApproverGroups:                 []*gitlab.MergeRequestApproverGroup{{Group: gitlab.MergeRequestApproverNestedGroup{FullPath: "platform/security"}}},
		ApprovalRulesLeft:              []*gitlab.MergeRequestApprovalRule{{ID: 7, Name: "Security", RuleType: "regular", ApprovalsRequired: 1, Approved: false, ApprovedBy: []*gitlab.BasicUser{{Username: "alice"}}}},
		HasApprovalRules:               true,
		MergeRequestApproversAvailable: true,
		MultipleApprovalRulesAvailable: true,
	}

	var out bytes.Buffer
	if err := writeMergeRequestApproval(&out, "toon", commandModeAxi, approvals, true, nil); err != nil {
		t.Fatalf("writeMergeRequestApproval returned error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "approval:\n  iid: 123") {
		t.Fatalf("output = %q, want approval root", got)
	}
	if !strings.Contains(got, "approved_by[1]{username,approved_at}:") {
		t.Fatalf("output = %q, want approved_by rows", got)
	}
	if !strings.Contains(got, "approval_rules_left[1]:") || !strings.Contains(got, "name: Security") {
		t.Fatalf("output = %q, want approval rule details", got)
	}
}
