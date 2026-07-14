package benchmark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPreflightRejectsMissingModelBeforeDockerChecks(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	t.Setenv("PATH", t.TempDir())

	checks := Preflight(context.Background(), Config{
		Host: server.URL, Token: "token", Group: "group",
		Agent: AgentCodex, Tool: "gl-axi", Isolation: IsolationDocker,
	})
	foundModel := false
	for _, check := range checks {
		if check.Name == "model" {
			foundModel = true
			if check.OK {
				t.Fatal("missing model passed preflight")
			}
		}
		if check.Name == "gitlab-auth" || check.Name == "docker-image" || check.Name == "provider-auth" || check.Name == "container-gitlab-auth" || check.Name == "container-provider" || len(check.Name) > 8 && check.Name[:8] == "command:" {
			t.Fatalf("invalid configuration reached live preflight check %q", check.Name)
		}
	}
	if !foundModel {
		t.Fatal("model preflight check not found")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("invalid configuration made %d HTTP request(s)", got)
	}
}
