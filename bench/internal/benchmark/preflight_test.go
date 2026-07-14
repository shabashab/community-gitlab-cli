package benchmark

import (
	"context"
	"testing"
)

func TestPreflightRejectsMissingModelBeforeDockerChecks(t *testing.T) {
	checks := Preflight(context.Background(), Config{
		Host: "https://gitlab.example", Token: "token", Group: "group",
		Agent: AgentCodex, Tool: "gl-axi", Isolation: IsolationDocker,
	})
	for _, check := range checks {
		if check.Name == "model" {
			if check.OK {
				t.Fatal("missing model passed preflight")
			}
			return
		}
	}
	t.Fatal("model preflight check not found")
}
