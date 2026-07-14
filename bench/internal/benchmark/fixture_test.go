package benchmark

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestGradeExactComment(t *testing.T) {
	tests := []struct {
		name string
		body string
		pass bool
	}{
		{name: "one exact comment", body: `[{"id":1,"body":"benchmark-comment-abc"}]`, pass: true},
		{name: "duplicate comment", body: `[{"id":1,"body":"benchmark-comment-abc"},{"id":2,"body":"benchmark-comment-abc"}]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got, want := r.URL.EscapedPath(), "/api/v4/projects/42/merge_requests/7/notes"; got != want {
					t.Errorf("path = %q, want %q", got, want)
				}
				if got := r.Header.Get("Private-Token"); got != "token" {
					t.Errorf("Private-Token = %q", got)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()

			client, err := gitlab.NewClient("token", gitlab.WithBaseURL(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			fixture := &Fixture{
				Client:       client,
				Project:      &gitlab.Project{ID: 42},
				MergeRequest: &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: 7}},
				CommentBody:  "benchmark-comment-abc",
			}

			grade := fixture.GradeExactComment()
			if grade.Passed != test.pass {
				t.Fatalf("grade = %+v, want pass=%v", grade, test.pass)
			}
		})
	}
}
