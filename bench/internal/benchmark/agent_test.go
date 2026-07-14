package benchmark

import (
	"slices"
	"strings"
	"testing"
)

func TestAgentCommand(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		name, args, err := agentCommand(AgentConfig{
			Agent:   AgentCodex,
			Model:   "gpt-5.6-sol",
			Effort:  "high",
			Tool:    "gl-axi",
			WorkDir: "/tmp/repo",
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "codex" {
			t.Fatalf("name = %q", name)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"--ephemeral", "--ignore-user-config", "--json", "gpt-5.6-sol", "model_reasoning_effort=\"high\""} {
			if !strings.Contains(joined, want) {
				t.Errorf("args %q do not contain %q", joined, want)
			}
		}
		approval := slices.Index(args, "--ask-for-approval")
		execSubcommand := slices.Index(args, "exec")
		if approval < 0 || execSubcommand < 0 || approval > execSubcommand {
			t.Fatalf("global --ask-for-approval must precede exec: %#v", args)
		}
		if !slices.Contains(args, "--sandbox") || slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("local Codex command must retain its internal sandbox: %#v", args)
		}
	})

	t.Run("codex with external isolation", func(t *testing.T) {
		_, args, err := agentCommand(AgentConfig{
			Agent: AgentCodex, Model: "gpt-5.6-sol", Effort: "high",
			WorkDir: "/workspace", ExternalIsolation: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("externally isolated Codex command does not bypass its nested sandbox: %#v", args)
		}
		for _, forbidden := range []string{"--ask-for-approval", "--sandbox", "sandbox_workspace_write.network_access=true"} {
			if slices.Contains(args, forbidden) {
				t.Fatalf("externally isolated Codex command contains %q: %#v", forbidden, args)
			}
		}
	})

	t.Run("claude", func(t *testing.T) {
		name, args, err := agentCommand(AgentConfig{
			Agent:  AgentClaude,
			Model:  "claude-opus-4-8",
			Effort: "high",
			Tool:   "gl-axi",
		})
		if err != nil {
			t.Fatal(err)
		}
		if name != "claude" {
			t.Fatalf("name = %q", name)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"--safe-mode", "stream-json", "claude-opus-4-8", "Bash(gl-axi *)", "dontAsk"} {
			if !strings.Contains(joined, want) {
				t.Errorf("args %q do not contain %q", joined, want)
			}
		}
	})
}

func TestParseCodexEvents(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"item.started","item":{"type":"command_execution","command":"/bin/zsh -lc 'gl-axi mr list'"}}`,
		`{"type":"item.completed","item":{"type":"command_execution","command":"/bin/zsh -lc 'gl-axi mr list'"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"MR !7 uses bench-feature"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":40,"output_tokens":20,"reasoning_output_tokens":5}}`,
	}, "\n")
	result := AgentResult{RawEvents: []byte(raw)}
	parseCodexEvents(&result)

	if result.FinalMessage != "MR !7 uses bench-feature" {
		t.Fatalf("final message = %q", result.FinalMessage)
	}
	if result.Usage.InputTokens != 100 || result.Usage.ReasoningTokens != 5 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if len(result.Commands) != 1 {
		t.Fatalf("commands = %#v", result.Commands)
	}
}

func TestParseClaudeEvents(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"gl-axi mr 7"}}]}}`,
		`{"type":"result","result":"MR !7 uses bench-feature","num_turns":2,"total_cost_usd":0.12,"usage":{"input_tokens":90,"cache_read_input_tokens":30,"cache_creation_input_tokens":10,"output_tokens":15}}`,
	}, "\n")
	result := AgentResult{RawEvents: []byte(raw)}
	parseClaudeEvents(&result)

	if result.FinalMessage != "MR !7 uses bench-feature" {
		t.Fatalf("final message = %q", result.FinalMessage)
	}
	if result.Usage.CostUSD != 0.12 || result.Usage.Turns != 2 {
		t.Fatalf("usage = %+v", result.Usage)
	}
	if len(result.Commands) != 1 || result.Commands[0] != "gl-axi mr 7" {
		t.Fatalf("commands = %#v", result.Commands)
	}
}

func TestFindPolicyViolation(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		tool     string
		want     bool
	}{
		{name: "selected tool", commands: []string{"gl-axi mr list"}, tool: "gl-axi"},
		{name: "other adapter", commands: []string{"glab mr list"}, tool: "gl-axi", want: true},
		{name: "raw http", commands: []string{"curl https://gitlab.example/api/v4/projects"}, tool: "gl", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := findPolicyViolation(test.commands, test.tool) != ""
			if got != test.want {
				t.Fatalf("violation = %v, want %v", got, test.want)
			}
		})
	}
}
