package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunWhoamiUsesGitLabClient(t *testing.T) {
	var gotPath string
	var gotToken string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("Private-Token")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":42,"username":"octocat","name":"Mona Lisa","state":"active","web_url":"https://gitlab.example/octocat"}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	err := runWhoami(cmd, &rootOptions{
		gitlabToken:   "test-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runWhoami returned error: %v", err)
	}
	if gotPath != "/api/v4/user" {
		t.Fatalf("request path = %q, want /api/v4/user", gotPath)
	}
	if gotToken != "test-token" {
		t.Fatalf("Private-Token header = %q, want test-token", gotToken)
	}

	want := `"username": "octocat"`
	if !bytes.Contains(out.Bytes(), []byte(want)) {
		t.Fatalf("runWhoami output = %q, want fragment %q", out.String(), want)
	}
}
