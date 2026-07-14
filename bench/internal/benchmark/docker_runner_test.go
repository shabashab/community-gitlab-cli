package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeDockerCLI struct {
	mu         sync.Mutex
	runs       [][]string
	runFunc    func(context.Context, io.Reader, io.Writer, io.Writer, []string) error
	outputFunc func(context.Context, []string) ([]byte, error)
}

func (f *fakeDockerCLI) Run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	f.mu.Lock()
	f.runs = append(f.runs, append([]string(nil), args...))
	f.mu.Unlock()
	if f.runFunc != nil {
		return f.runFunc(ctx, stdin, stdout, stderr, args)
	}
	return nil
}

func (f *fakeDockerCLI) Output(ctx context.Context, args ...string) ([]byte, error) {
	if f.outputFunc != nil {
		return f.outputFunc(ctx, args)
	}
	return nil, nil
}

func TestDockerRunnerCapturesEventsAndRemovesContainer(t *testing.T) {
	t.Setenv("CODEX_ACCESS_TOKEN", "account-token-not-for-metadata")
	started := time.Now().UTC().Add(-time.Second)
	finished := started.Add(500 * time.Millisecond)
	cli := &fakeDockerCLI{}
	cli.runFunc = func(_ context.Context, _ io.Reader, stdout, stderr io.Writer, args []string) error {
		switch args[0] {
		case "create":
			io.WriteString(stdout, "container-id\n")
		case "start":
			io.WriteString(stdout, `{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`+"\n")
			io.WriteString(stderr, "agent stderr")
		}
		return nil
	}
	cli.outputFunc = func(_ context.Context, args []string) ([]byte, error) {
		if args[0] != "inspect" {
			return nil, errors.New("unexpected output command")
		}
		state := map[string]any{
			"OOMKilled": false, "ExitCode": 0,
			"StartedAt":  started.Format(time.RFC3339Nano),
			"FinishedAt": finished.Format(time.RFC3339Nano),
		}
		return json.Marshal(state)
	}

	workDir := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &DockerRunner{
		CLI: cli, Image: "bench:test", Agent: AgentCodex, Tool: "gl-axi",
		Resources:     defaultContainerResources,
		ImageMetadata: ImageMetadata{Ref: "bench:test", ID: "image-id", AgentVersion: "test", AdapterSHA256: "abc"},
	}
	result, runtime, err := runner.Run(context.Background(), AgentConfig{
		Agent: AgentCodex, Model: "model", Effort: "high", Tool: "gl-axi",
		Prompt: "do it", WorkDir: workDir, Host: "https://gitlab.example", Token: "gitlab-secret",
		RunID: "run", TaskID: TaskFindMR, Trial: 1, Project: "group/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.FinalMessage != "done" || string(result.RawStderr) != "agent stderr" {
		t.Fatalf("result = %+v stderr=%q", result, result.RawStderr)
	}
	if runtime.ContainerID != "container-id" || !runtime.Cleanup.ContainerRemoved || runtime.ExitCode == nil || *runtime.ExitCode != 0 {
		t.Fatalf("runtime = %+v", runtime)
	}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("account-token")) || bytes.Contains(encoded, []byte("gitlab-secret")) {
		t.Fatalf("runtime leaked a credential: %s", encoded)
	}

	joined := make([]string, 0, len(cli.runs))
	for _, call := range cli.runs {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{"create --name gl-bench-", "--memory 2g", "--cpus 2", "--pids-limit 256", "--dangerously-bypass-approvals-and-sandbox", "start --attach --interactive", "rm --force"} {
		if !strings.Contains(all, want) {
			t.Errorf("Docker calls do not contain %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "--sandbox workspace-write") {
		t.Fatalf("Docker Codex command attempted nested sandboxing:\n%s", all)
	}
	if strings.Contains(all, "gitlab-secret") || strings.Contains(all, "account-token") {
		t.Fatalf("Docker argv leaked a credential:\n%s", all)
	}
}

func TestDockerRunnerTimeoutKillsInspectsAndRemoves(t *testing.T) {
	t.Setenv("CODEX_ACCESS_TOKEN", "account-token")
	started := time.Now().UTC()
	cli := &fakeDockerCLI{}
	cli.runFunc = func(ctx context.Context, _ io.Reader, stdout, _ io.Writer, args []string) error {
		if args[0] == "create" {
			io.WriteString(stdout, "container-id\n")
			return nil
		}
		if args[0] == "start" {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	cli.outputFunc = func(_ context.Context, _ []string) ([]byte, error) {
		state := map[string]any{
			"OOMKilled": false, "ExitCode": 137,
			"StartedAt":  started.Format(time.RFC3339Nano),
			"FinishedAt": started.Add(time.Second).Format(time.RFC3339Nano),
		}
		return json.Marshal(state)
	}
	workDir := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &DockerRunner{CLI: cli, Image: "bench:test", Agent: AgentCodex, Tool: "gl-axi", Resources: defaultContainerResources}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, runtime, err := runner.Run(ctx, AgentConfig{
		Agent: AgentCodex, Model: "model", Tool: "gl-axi", Prompt: "wait", WorkDir: workDir,
		Host: "https://gitlab.example", Token: "token", RunID: "run", TaskID: TaskFindMR, Trial: 1,
	})
	if err == nil || !runtime.TimedOut || !runtime.Cleanup.ContainerRemoved {
		t.Fatalf("err=%v runtime=%+v", err, runtime)
	}
	var calls string
	for _, call := range cli.runs {
		calls += strings.Join(call, " ") + "\n"
	}
	if !strings.Contains(calls, "kill ") || !strings.Contains(calls, "rm --force") {
		t.Fatalf("cleanup calls missing:\n%s", calls)
	}
}

func TestProviderAuthSourcePrefersAccountCredentials(t *testing.T) {
	t.Setenv("CODEX_ACCESS_TOKEN", "account")
	t.Setenv("CODEX_API_KEY", "api")
	source, err := providerAuthSource(withConfigDefaults(Config{Agent: AgentCodex}))
	if err != nil || source != "source=CODEX_ACCESS_TOKEN" {
		t.Fatalf("source=%q err=%v", source, err)
	}

	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth")
	t.Setenv("ANTHROPIC_API_KEY", "api")
	source, err = providerAuthSource(Config{Agent: AgentClaude})
	if err != nil || source != "source=CLAUDE_CODE_OAUTH_TOKEN" {
		t.Fatalf("source=%q err=%v", source, err)
	}
}

func TestPreparedCredentialFilesUseScopedPermissions(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source-auth.json")
	if err := os.WriteFile(source, []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_ACCESS_TOKEN", "")
	t.Setenv("CODEX_API_KEY", "")
	authPath, err := prepareCodexAuthSecret(dir, AgentCodex, source)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(authPath)
	authInfo, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if authInfo.Mode().Perm() != 0o444 {
		t.Fatalf("auth mode = %o", authInfo.Mode().Perm())
	}

	t.Setenv("CODEX_ACCESS_TOKEN", "account")
	envPath, err := writeContainerEnv(dir, AgentConfig{Host: "https://gitlab.example", Token: "gitlab"}, AgentCodex, source)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(envPath)
	envInfo, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if envInfo.Mode().Perm() != 0o600 {
		t.Fatalf("env mode = %o", envInfo.Mode().Perm())
	}
}

func TestInspectContainerStateParsesNarrowState(t *testing.T) {
	cli := &fakeDockerCLI{outputFunc: func(_ context.Context, _ []string) ([]byte, error) {
		return []byte(`{"OOMKilled":true,"ExitCode":137,"StartedAt":"2026-07-13T10:00:00Z","FinishedAt":"2026-07-13T10:00:01Z"}`), nil
	}}
	state, err := inspectContainerState(context.Background(), cli, "container")
	if err != nil {
		t.Fatal(err)
	}
	if !state.OOMKilled || state.ExitCode != 137 || state.FinishedAt.Sub(state.StartedAt) != time.Second {
		t.Fatalf("state = %+v", state)
	}
}
