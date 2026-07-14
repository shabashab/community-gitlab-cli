package benchmark

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

type AgentConfig struct {
	Agent    string
	Model    string
	Effort   string
	Tool     string
	Prompt   string
	WorkDir  string
	RepoRoot string
	Host     string
	Token    string
	MaxTurns int
	RunID    string
	TaskID   TaskID
	Trial    int
	Project  string

	// ExternalIsolation is set only by a runner that supplies its own process
	// boundary. It lets Codex avoid trying to nest its bwrap sandbox inside the
	// trial container; LocalRunner intentionally leaves it false.
	ExternalIsolation bool
}

// AgentRunner executes one normalized Claude Code or Codex invocation.
type AgentRunner interface {
	Run(context.Context, AgentConfig) (AgentResult, RuntimeMetadata, error)
}

// LocalRunner preserves the original host-process runner for harness work.
type LocalRunner struct{}

func (LocalRunner) Run(ctx context.Context, cfg AgentConfig) (AgentResult, RuntimeMetadata, error) {
	name, args, err := agentCommand(cfg)
	if err != nil {
		return AgentResult{}, RuntimeMetadata{Isolation: IsolationLocal}, err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cfg.WorkDir
	cmd.Stdin = strings.NewReader(cfg.Prompt)
	cmd.Env = append(os.Environ(),
		"GITLAB_BASE_URL="+cfg.Host,
		"GITLAB_TOKEN="+cfg.Token,
		"GL_TOKEN=",
		"DISABLE_AUTOUPDATER=1",
		"PATH="+filepath.Join(cfg.RepoRoot, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := time.Now().UTC()
	runErr := cmd.Run()
	finished := time.Now().UTC()

	result := AgentResult{
		RawEvents: stdout.Bytes(),
		RawStderr: stderr.Bytes(),
		Duration:  finished.Sub(started),
	}
	runtime := RuntimeMetadata{
		Isolation:  IsolationLocal,
		StartedAt:  started,
		FinishedAt: finished,
		Cleanup:    CleanupMetadata{},
	}
	if runErr == nil {
		exitCode := 0
		runtime.ExitCode = &exitCode
	} else {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode := exitErr.ExitCode()
			runtime.ExitCode = &exitCode
		}
	}
	parseAgentResult(cfg.Agent, cfg.Tool, &result)

	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			runtime.TimedOut = true
			return result, runtime, fmt.Errorf("agent timed out: %w", ctx.Err())
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			runtime.Canceled = true
		}
		return result, runtime, fmt.Errorf("%s exited unsuccessfully: %w", cfg.Agent, runErr)
	}
	if strings.TrimSpace(result.FinalMessage) == "" {
		return result, runtime, errors.New("agent emitted no final message")
	}
	return result, runtime, nil
}

// ExecuteAgent is retained as the local-runner compatibility seam.
func ExecuteAgent(ctx context.Context, cfg AgentConfig) (AgentResult, error) {
	result, _, err := (LocalRunner{}).Run(ctx, cfg)
	return result, err
}

func agentCommand(cfg AgentConfig) (string, []string, error) {
	effort := cfg.Effort
	if effort == "" {
		effort = "high"
	}
	maxTurns := cfg.MaxTurns
	if maxTurns == 0 {
		maxTurns = 25
	}

	switch cfg.Agent {
	case AgentClaude:
		allowedWithArgs := fmt.Sprintf("Bash(%s *)", cfg.Tool)
		allowedBare := fmt.Sprintf("Bash(%s)", cfg.Tool)
		return "claude", []string{
			"-p",
			"--safe-mode",
			"--output-format", "stream-json",
			"--verbose",
			"--no-session-persistence",
			"--model", cfg.Model,
			"--effort", effort,
			"--max-turns", strconv.Itoa(maxTurns),
			"--tools", "Bash",
			"--permission-mode", "dontAsk",
			"--allowedTools", allowedWithArgs, allowedBare,
		}, nil
	case AgentCodex:
		args := []string{"--ask-for-approval", "never", "exec"}
		if cfg.ExternalIsolation {
			args = []string{"exec", "--dangerously-bypass-approvals-and-sandbox"}
		}
		args = append(args,
			"--ephemeral",
			"--ignore-user-config",
			"--json",
			"--model", cfg.Model,
			"--config", fmt.Sprintf("model_reasoning_effort=%q", effort),
		)
		if !cfg.ExternalIsolation {
			args = append(args,
				"--config", "sandbox_workspace_write.network_access=true",
				"--sandbox", "workspace-write",
			)
		}
		args = append(args, "--cd", cfg.WorkDir, "-")
		return "codex", args, nil
	default:
		return "", nil, fmt.Errorf("unsupported agent %q (use %s or %s)", cfg.Agent, AgentClaude, AgentCodex)
	}
}

func parseCodexEvents(result *AgentResult) {
	forEachJSONLine(result.RawEvents, func(event map[string]any) {
		typeName, _ := event["type"].(string)
		if typeName == "turn.completed" {
			if usage, ok := event["usage"].(map[string]any); ok {
				result.Usage.InputTokens = intValue(usage["input_tokens"])
				result.Usage.CachedInputTokens = intValue(usage["cached_input_tokens"])
				result.Usage.OutputTokens = intValue(usage["output_tokens"])
				result.Usage.ReasoningTokens = intValue(usage["reasoning_output_tokens"])
			}
		}

		item, ok := event["item"].(map[string]any)
		if !ok {
			return
		}
		switch item["type"] {
		case "agent_message":
			if text, ok := item["text"].(string); ok && strings.TrimSpace(text) != "" {
				result.FinalMessage = text
			}
		case "command_execution":
			if typeName == "item.completed" {
				command, ok := item["command"].(string)
				if !ok {
					return
				}
				result.Commands = append(result.Commands, command)
			}
		}
	})
}

func parseClaudeEvents(result *AgentResult) {
	forEachJSONLine(result.RawEvents, func(event map[string]any) {
		typeName, _ := event["type"].(string)
		if typeName == "result" {
			if text, ok := event["result"].(string); ok && strings.TrimSpace(text) != "" {
				result.FinalMessage = text
			}
			result.Usage.CostUSD = floatValue(event["total_cost_usd"])
			result.Usage.Turns = intValue(event["num_turns"])
			if usage, ok := event["usage"].(map[string]any); ok {
				result.Usage.InputTokens = intValue(usage["input_tokens"])
				result.Usage.CachedInputTokens = intValue(usage["cache_read_input_tokens"])
				result.Usage.CacheCreationTokens = intValue(usage["cache_creation_input_tokens"])
				result.Usage.OutputTokens = intValue(usage["output_tokens"])
			}
		}

		message, ok := event["message"].(map[string]any)
		if !ok {
			return
		}
		content, ok := message["content"].([]any)
		if !ok {
			return
		}
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			switch block["type"] {
			case "text":
				if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
					result.FinalMessage = text
				}
			case "tool_use":
				if block["name"] != "Bash" {
					continue
				}
				input, _ := block["input"].(map[string]any)
				if command, ok := input["command"].(string); ok {
					result.Commands = append(result.Commands, command)
				}
			}
		}
	})
}

func forEachJSONLine(data []byte, visit func(map[string]any)) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			visit(event)
		}
	}
}

func intValue(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	if typed, ok := value.(float64); ok {
		return typed
	}
	return 0
}

func findPolicyViolation(commands []string, selectedTool string) string {
	toolPattern := regexp.MustCompile(`(^|[[:space:];|&'\"])(gl-axi|glab|gl)([[:space:];|&'\"]|$)`)
	for _, command := range commands {
		lower := strings.ToLower(command)
		if strings.Contains(lower, "/api/v4") || regexp.MustCompile(`(^|[[:space:];|&])(?:curl|wget)([[:space:];|&]|$)`).MatchString(lower) {
			return "used raw HTTP instead of the selected adapter: " + command
		}
		matches := toolPattern.FindAllStringSubmatch(lower, -1)
		for _, match := range matches {
			if len(match) > 2 && match[2] != selectedTool {
				return fmt.Sprintf("used GitLab adapter %q instead of %q: %s", match[2], selectedTool, command)
			}
		}
	}
	return ""
}
