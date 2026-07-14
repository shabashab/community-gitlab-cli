package benchmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		Commands:     []string{`curl -H 'Private-Token: codex-refresh'`},
		PolicyViolation: `used raw HTTP instead of the selected adapter: ` +
			`curl -H 'Private-Token: codex-refresh'`,
	}
	redactAgentResult(&result, benchmarkSecrets(Config{
		Agent: AgentCodex, Token: "gitlab-secret", CodexAuthFile: authFile,
	}))
	combined := string(result.RawEvents) + string(result.RawStderr) + result.FinalMessage + strings.Join(result.Commands, " ") + result.PolicyViolation
	for _, secret := range []string{"gitlab-secret", "codex-access", "codex-refresh"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("result still contains %q: %s", secret, combined)
		}
	}
	if !strings.Contains(combined, redactedValue) {
		t.Fatalf("result has no redaction marker: %s", combined)
	}
}

func TestRedactTrialResultCoversEveryDiagnosticSink(t *testing.T) {
	const secret = "sentinel-secret"
	result := TrialResult{
		Commands:        []string{"command " + secret},
		FinalMessage:    "message " + secret,
		PolicyViolation: "policy " + secret,
		Error:           "execution " + secret,
		Runtime: RuntimeMetadata{Cleanup: CleanupMetadata{
			Error: "cleanup " + secret,
		}},
		Grade: Grade{
			Assertions: []string{"assertion " + secret},
			Failures:   []string{"failure " + secret},
		},
	}
	redactTrialResult(&result, []string{secret})

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secret)) {
		t.Fatalf("serialized result leaked secret: %s", encoded)
	}
	if bytes.Count(encoded, []byte(redactedValue)) < 7 {
		t.Fatalf("expected all diagnostic fields to be redacted: %s", encoded)
	}

	var output bytes.Buffer
	addResultToRun(Config{Out: &output}, &Summary{}, result)
	if strings.Contains(output.String(), secret) || !strings.Contains(output.String(), redactedValue) {
		t.Fatalf("terminal output was not redacted: %s", output.String())
	}
}

func TestFinishFailedRunRedactsManifestAndReturnedError(t *testing.T) {
	const secret = "sentinel-secret"
	runDir := t.TempDir()
	started := time.Now().UTC().Add(-time.Second)
	manifest := RunManifest{RunID: "run", Status: "running", StartedAt: started}
	summary := Summary{RunID: "run", RunDir: runDir, StartedAt: started}

	err := finishFailedRun(runDir, &manifest, &summary, []string{secret}, errors.New("infrastructure "+secret))
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), redactedValue) {
		t.Fatalf("returned error was not redacted: %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(runDir, "manifest.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if bytes.Contains(data, []byte(secret)) || !bytes.Contains(data, []byte(redactedValue)) {
		t.Fatalf("manifest error was not redacted: %s", data)
	}
}
