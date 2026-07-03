// Package agenthooks installs SessionStart integrations that surface gl-axi
// ambient context in agent sessions. It follows the axi standard's session
// integration pattern (https://github.com/kunchenguid/axi) and targets the
// three default agent apps: Claude Code, Codex, and OpenCode.
//
// Installs are idempotent and self-repairing: entries are recognized by a
// managed marker inside the hook command, re-runs update a drifted executable
// path, and unmanaged user configuration is never touched.
package agenthooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Marker identifies managed hook entries across all integration targets.
const Marker = "gl-axi"

const defaultTimeoutSeconds = 10

const openCodePluginMarkerPrefix = "community-gitlab-cli managed opencode plugin:"

// TargetResult reports the outcome of one integration target.
type TargetResult struct {
	App    string
	Path   string
	Status string // installed, updated, unchanged, or error: <detail>
}

// Options configures an install run.
type Options struct {
	// HomeDir overrides the user home directory (tests).
	HomeDir string
	// Command is the shell command agents run at session start, for example
	// "gl-axi context". It must contain Marker so installs stay recognizable.
	Command string
	// TimeoutSeconds bounds hook execution; defaults to 10.
	TimeoutSeconds int
}

// PortableCommand returns the hook command for the current executable: the
// bare binary name when PATH resolves it to this same executable (portable
// across reinstalls), the absolute path otherwise.
func PortableCommand(binaryName string, args ...string) string {
	parts := append([]string{portableBinary(binaryName)}, args...)

	return strings.Join(parts, " ")
}

func portableBinary(binaryName string) string {
	exe, err := os.Executable()
	if err != nil {
		return binaryName
	}
	realExe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}

	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate, err := filepath.EvalSymlinks(filepath.Join(dir, binaryName))
		if err != nil {
			continue
		}
		if candidate == realExe {
			return binaryName
		}
	}

	return realExe
}

// InstallSessionStartHooks installs or repairs the ambient-context
// integration for every supported agent app and reports per-target results.
func InstallSessionStartHooks(opts Options) []TargetResult {
	home := opts.HomeDir
	if home == "" {
		if userHome, err := os.UserHomeDir(); err == nil {
			home = userHome
		}
	}
	timeout := opts.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultTimeoutSeconds
	}

	if home == "" {
		return []TargetResult{{App: "all", Status: "error: cannot determine home directory"}}
	}

	results := make([]TargetResult, 0, 4)

	claudePath := filepath.Join(home, ".claude", "settings.json")
	results = append(results, installHookSettings("claude-code", claudePath, opts.Command, timeout))

	codexHooksPath := filepath.Join(home, ".codex", "hooks.json")
	results = append(results, installHookSettings("codex", codexHooksPath, opts.Command, timeout))

	codexConfigPath := filepath.Join(home, ".codex", "config.toml")
	results = append(results, installCodexConfig(codexConfigPath))

	openCodePath := filepath.Join(home, ".config", "opencode", "plugins", "axi-gl-axi.js")
	results = append(results, installOpenCodePlugin(openCodePath, opts.Command, timeout))

	return results
}

// installHookSettings updates a Claude Code / Codex style JSON settings file,
// adding or repairing the managed SessionStart hook while preserving all
// unrelated configuration.
func installHookSettings(app, path, command string, timeoutSeconds int) TargetResult {
	result := TargetResult{App: app, Path: path}

	settings := map[string]any{}
	raw, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !os.IsNotExist(err) {
		result.Status = "error: " + err.Error()
		return result
	}
	if existed {
		if err := json.Unmarshal(raw, &settings); err != nil {
			result.Status = fmt.Sprintf("error: %s is not valid JSON: %v", path, err)
			return result
		}
	}

	changed, installed := applySessionStartHook(settings, command, timeoutSeconds)
	if !changed {
		result.Status = "unchanged"
		return result
	}

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		result.Status = "error: " + err.Error()
		return result
	}
	if err := writeFileWithDirs(path, append(encoded, '\n'), 0o644); err != nil {
		result.Status = "error: " + err.Error()
		return result
	}

	if installed {
		result.Status = "installed"
	} else {
		result.Status = "updated"
	}

	return result
}

// applySessionStartHook mutates settings in place. It returns whether the
// settings changed and whether a new managed entry was added (as opposed to
// an existing one being repaired).
func applySessionStartHook(settings map[string]any, command string, timeoutSeconds int) (changed, installed bool) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}

	groups, ok := hooks["SessionStart"].([]any)
	if !ok {
		groups = []any{}
	}

	for _, groupAny := range groups {
		group, ok := groupAny.(map[string]any)
		if !ok {
			continue
		}
		entries, ok := group["hooks"].([]any)
		if !ok {
			continue
		}
		for _, entryAny := range entries {
			entry, ok := entryAny.(map[string]any)
			if !ok {
				continue
			}
			existing, _ := entry["command"].(string)
			if !strings.Contains(existing, Marker) {
				continue
			}

			entryType, _ := entry["type"].(string)
			timeout, hasTimeout := entry["timeout"].(float64)
			if existing == command && entryType == "command" && hasTimeout && int(timeout) == timeoutSeconds {
				return false, false
			}

			entry["command"] = command
			entry["type"] = "command"
			entry["timeout"] = timeoutSeconds
			hooks["SessionStart"] = groups

			return true, false
		}
	}

	groups = append(groups, map[string]any{
		"matcher": "",
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": command,
			"timeout": timeoutSeconds,
		}},
	})
	hooks["SessionStart"] = groups

	return true, true
}

// installCodexConfig ensures ~/.codex/config.toml enables the hooks feature.
func installCodexConfig(path string) TargetResult {
	result := TargetResult{App: "codex-config", Path: path}

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		result.Status = "error: " + err.Error()
		return result
	}

	updated, changed := ensureCodexHooksFeature(string(raw))
	if !changed {
		result.Status = "unchanged"
		return result
	}

	if err := writeFileWithDirs(path, []byte(updated), 0o644); err != nil {
		result.Status = "error: " + err.Error()
		return result
	}
	result.Status = "updated"

	return result
}

// ensureCodexHooksFeature performs a minimal line-based TOML edit that sets
// hooks = true under [features] without disturbing the rest of the file.
func ensureCodexHooksFeature(content string) (string, bool) {
	if strings.TrimSpace(content) == "" {
		return "[features]\nhooks = true\n", true
	}

	lines := strings.Split(content, "\n")
	inFeatures := false
	sawFeatures := false

	for index, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[") {
			if inFeatures {
				// Leaving [features] without finding a hooks key: insert one.
				updated := append([]string{}, lines[:index]...)
				updated = append(updated, "hooks = true")
				updated = append(updated, lines[index:]...)

				return strings.Join(updated, "\n"), true
			}

			section := strings.Trim(trimmed, "[]")
			inFeatures = strings.TrimSpace(section) == "features"
			sawFeatures = sawFeatures || inFeatures
			continue
		}

		if !inFeatures {
			continue
		}

		key, value, found := strings.Cut(trimmed, "=")
		if !found || strings.TrimSpace(key) != "hooks" {
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(value), "true") {
			return content, false
		}

		lines[index] = strings.Replace(line, "false", "true", 1)

		return strings.Join(lines, "\n"), true
	}

	suffix := ""
	if !strings.HasSuffix(content, "\n") {
		suffix = "\n"
	}
	if sawFeatures {
		return content + suffix + "hooks = true\n", true
	}

	return content + suffix + "\n[features]\nhooks = true\n", true
}

// installOpenCodePlugin writes the managed OpenCode ambient-context plugin.
// Unmanaged files at the target path are never overwritten.
func installOpenCodePlugin(path, command string, timeoutSeconds int) TargetResult {
	result := TargetResult{App: "opencode", Path: path}

	next := buildOpenCodePluginSource(command, timeoutSeconds)

	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		result.Status = "error: " + err.Error()
		return result
	}
	existed := err == nil

	if existed && !strings.Contains(string(raw), openCodePluginMarkerPrefix) {
		result.Status = "error: refusing to overwrite unmanaged plugin file"
		return result
	}
	if existed && string(raw) == next {
		result.Status = "unchanged"
		return result
	}

	if err := writeFileWithDirs(path, []byte(next), 0o644); err != nil {
		result.Status = "error: " + err.Error()
		return result
	}

	if existed {
		result.Status = "updated"
	} else {
		result.Status = "installed"
	}

	return result
}

func buildOpenCodePluginSource(command string, timeoutSeconds int) string {
	parts := strings.Fields(command)
	binary := Marker
	var args []string
	if len(parts) > 0 {
		binary = parts[0]
		args = parts[1:]
	}

	binaryJSON, _ := json.Marshal(binary)
	argsJSON, _ := json.Marshal(args)
	if args == nil {
		argsJSON = []byte("[]")
	}

	return fmt.Sprintf(`// %s %s
// This file is generated by gl-axi. It is safe to edit only if you remove the managed marker above.
import { spawn } from "node:child_process";

const binary = %s;
const args = %s;
const ambientHeader = "## AXI ambient context: %s";
const timeoutMs = %d;

function runAmbientContext(cwd) {
  return new Promise((resolve) => {
    const child = spawn(binary, args, {
      cwd: typeof cwd === "string" && cwd.length > 0 ? cwd : process.cwd(),
      env: process.env,
      shell: false,
      stdio: ["ignore", "pipe", "ignore"],
    });

    let stdout = "";
    let settled = false;

    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill("SIGTERM");
      resolve("");
    }, timeoutMs);

    child.stdout?.setEncoding("utf-8");
    child.stdout?.on("data", (chunk) => {
      stdout += chunk;
    });
    child.on("error", () => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve("");
    });
    child.on("close", (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(code === 0 ? stdout.trim() : "");
    });
  });
}

export const GlAxiAmbientContextPlugin = async ({ directory }) => {
  const sessionCache = new Map();

  return {
    "experimental.chat.system.transform": async (input, output) => {
      const sessionID = input.sessionID ?? "__global__";
      let ambient = sessionCache.get(sessionID);
      if (ambient === undefined) {
        ambient = await runAmbientContext(directory);
        sessionCache.set(sessionID, ambient);
      }

      if (ambient.length === 0) return;
      output.system.push(ambientHeader + "\n" + ambient);
    },
  };
};
`,
		openCodePluginMarkerPrefix,
		Marker,
		binaryJSON,
		argsJSON,
		Marker,
		timeoutSeconds*1000,
	)
}

func writeFileWithDirs(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, perm)
}
