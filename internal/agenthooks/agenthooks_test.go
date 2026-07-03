package agenthooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func statusOf(t *testing.T, results []TargetResult, app string) string {
	t.Helper()
	for _, result := range results {
		if result.App == app {
			return result.Status
		}
	}
	t.Fatalf("no result for app %q in %v", app, results)

	return ""
}

func TestInstallSessionStartHooksFreshAndIdempotent(t *testing.T) {
	home := t.TempDir()
	opts := Options{HomeDir: home, Command: "gl-axi context"}

	first := InstallSessionStartHooks(opts)
	for _, app := range []string{"claude-code", "codex", "opencode"} {
		if got := statusOf(t, first, app); got != "installed" {
			t.Fatalf("first install status for %s = %q, want installed", app, got)
		}
	}
	if got := statusOf(t, first, "codex-config"); got != "updated" {
		t.Fatalf("first codex-config status = %q, want updated", got)
	}

	second := InstallSessionStartHooks(opts)
	for _, result := range second {
		if result.Status != "unchanged" {
			t.Fatalf("second install status for %s = %q, want unchanged", result.App, result.Status)
		}
	}
}

func TestInstallSessionStartHooksRepairsPathAndPreservesSettings(t *testing.T) {
	home := t.TempDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	seed := `{"model":"opus","hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"/old/gl-axi context","timeout":10}]}],"PostToolUse":[{"matcher":"x"}]}}`
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	results := InstallSessionStartHooks(Options{HomeDir: home, Command: "/new/gl-axi context"})
	if got := statusOf(t, results, "claude-code"); got != "updated" {
		t.Fatalf("claude-code status = %q, want updated", got)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	if settings["model"] != "opus" {
		t.Fatalf("unrelated settings key lost: %v", settings)
	}
	if !strings.Contains(string(raw), "/new/gl-axi context") {
		t.Fatalf("settings = %s, want repaired command path", raw)
	}
	if strings.Contains(string(raw), "/old/gl-axi context") {
		t.Fatalf("settings = %s, want old path removed", raw)
	}
	if !strings.Contains(string(raw), "PostToolUse") {
		t.Fatalf("settings = %s, want unrelated hooks preserved", raw)
	}
}

func TestInstallSessionStartHooksRefusesUnmanagedOpenCodePlugin(t *testing.T) {
	home := t.TempDir()
	pluginPath := filepath.Join(home, ".config", "opencode", "plugins", "axi-gl-axi.js")
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginPath, []byte("// user-owned plugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	results := InstallSessionStartHooks(Options{HomeDir: home, Command: "gl-axi context"})
	if got := statusOf(t, results, "opencode"); !strings.HasPrefix(got, "error:") {
		t.Fatalf("opencode status = %q, want refusal error", got)
	}

	raw, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "// user-owned plugin\n" {
		t.Fatalf("unmanaged plugin was modified: %s", raw)
	}
}

func TestEnsureCodexHooksFeature(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		changed bool
	}{
		{name: "empty", in: "", want: "[features]\nhooks = true\n", changed: true},
		{name: "already enabled", in: "[features]\nhooks = true\n", want: "[features]\nhooks = true\n", changed: false},
		{name: "disabled", in: "[features]\nhooks = false\n", want: "[features]\nhooks = true\n", changed: true},
		{
			name:    "other sections",
			in:      "[model]\nname = \"gpt\"\n",
			want:    "[model]\nname = \"gpt\"\n\n[features]\nhooks = true\n",
			changed: true,
		},
		{
			name:    "features without hooks followed by section",
			in:      "[features]\nother = 1\n[model]\nname = \"gpt\"\n",
			want:    "[features]\nother = 1\nhooks = true\n[model]\nname = \"gpt\"\n",
			changed: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, changed := ensureCodexHooksFeature(testCase.in)
			if got != testCase.want || changed != testCase.changed {
				t.Fatalf("ensureCodexHooksFeature(%q) = %q, %t, want %q, %t", testCase.in, got, changed, testCase.want, testCase.changed)
			}
		})
	}
}
