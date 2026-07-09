package cli

// Shared test helpers for the merge request command suites: root-command
// execution wrappers, the current-branch stub, and canned GitLab fixtures.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

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
		case "/api/v4/projects/group%2Fproject/merge_requests/123/approvals":
			fmt.Fprint(w, mergeRequestApprovalsJSON(123))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return server, &listQuery, &viewPath
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

func mergeRequestApprovalsJSON(iid int64) string {
	return fmt.Sprintf(`{
		"id": %d,
		"iid": %d,
		"project_id": 42,
		"title": "Add search endpoint",
		"state": "opened",
		"merge_status": "can_be_merged",
		"approved": false,
		"approvals_before_merge": 2,
		"approvals_required": 2,
		"approvals_left": 1,
		"require_password_to_approve": false,
		"approved_by": [
			{"user": {"id": 1, "username": "alice", "name": "Alice"}, "approved_at": "2026-07-04T10:00:00Z"}
		],
		"suggested_approvers": [
			{"id": 2, "username": "mona", "name": "Mona"}
		],
		"approvers": [
			{"user": {"id": 1, "username": "alice", "name": "Alice"}, "approved_at": "2026-07-04T10:00:00Z"},
			{"user": {"id": 3, "username": "hubot", "name": "Hubot"}}
		],
		"approver_groups": [
			{"group": {"id": 4, "name": "Security", "full_path": "platform/security"}}
		],
		"user_has_approved": true,
		"user_can_approve": false,
		"approval_rules_left": [
			{"id": 7, "name": "Security", "rule_type": "regular", "approvals_required": 1, "approved": false, "approved_by": [{"username": "alice"}]}
		],
		"has_approval_rules": true,
		"merge_request_approvers_available": true,
		"multiple_approval_rules_available": true
	}`, 1000+iid, iid)
}
