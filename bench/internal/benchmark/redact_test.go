package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactAgentResultRemovesTrialCredentials(t *testing.T) {
	authFile := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(authFile, []byte(`{"tokens":{"access_token":"codex-access","refresh_token":"codex-refresh"},"account_id":"not-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_ACCESS_TOKEN", "")
	t.Setenv("CODEX_API_KEY", "")
	result := AgentResult{
		RawEvents:    []byte(`gitlab-secret codex-access codex-refresh`),
		RawStderr:    []byte(`codex-access`),
		FinalMessage: `gitlab-secret`,
		Commands:     []string{`printf codex-refresh`},
	}
	redactAgentResult(&result, benchmarkSecrets(Config{
		Agent: AgentCodex, Token: "gitlab-secret", CodexAuthFile: authFile,
	}))
	combined := string(result.RawEvents) + string(result.RawStderr) + result.FinalMessage + strings.Join(result.Commands, " ")
	for _, secret := range []string{"gitlab-secret", "codex-access", "codex-refresh"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("result still contains %q: %s", secret, combined)
		}
	}
	if !strings.Contains(combined, redactedValue) {
		t.Fatalf("result has no redaction marker: %s", combined)
	}
}
