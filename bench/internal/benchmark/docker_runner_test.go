package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/user"
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
	var stagedSecretPath string
	cli.runFunc = func(_ context.Context, _ io.Reader, stdout, stderr io.Writer, args []string) error {
		switch args[0] {
		case "create":
			stagedSecretPath = mountedSource(args, containerSecretPath)
			data, err := os.ReadFile(stagedSecretPath)
			if err != nil {
				t.Fatalf("read staged credential: %v", err)
			}
			if !bytes.Contains(data, []byte("gitlab-secret")) || !bytes.Contains(data, []byte("account-token-not-for-metadata")) {
				t.Fatalf("staged credential is incomplete: %s", data)
			}
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
		Identity:      ContainerIdentity{UID: 501, GID: 20},
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
	if _, err := os.Stat(stagedSecretPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged credential still exists after trial: %v", err)
	}

	joined := make([]string, 0, len(cli.runs))
	for _, call := range cli.runs {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{"create --name gl-bench-", "--memory 2g", "--cpus 2", "--pids-limit 256", "--user 501:20", "uid=501,gid=20", "dst=" + containerSecretPath + ",readonly", "--dangerously-bypass-approvals-and-sandbox", "start --attach --interactive", "rm --force"} {
		if !strings.Contains(all, want) {
			t.Errorf("Docker calls do not contain %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "--sandbox workspace-write") {
		t.Fatalf("Docker Codex command attempted nested sandboxing:\n%s", all)
	}
	if strings.Contains(all, "--env-file") {
		t.Fatalf("Docker create persisted credentials through --env-file:\n%s", all)
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
	runner := &DockerRunner{
		CLI: cli, Image: "bench:test", Agent: AgentCodex, Tool: "gl-axi",
		Resources: defaultContainerResources, Identity: ContainerIdentity{UID: 501, GID: 20},
	}
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
	if authInfo.Mode().Perm() != 0o600 {
		t.Fatalf("auth mode = %o", authInfo.Mode().Perm())
	}

	t.Setenv("CODEX_ACCESS_TOKEN", "account")
	envPath, err := writeContainerSecrets(dir, AgentConfig{Host: "https://gitlab.example", Token: "gitlab"}, AgentCodex, source, true)
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

func TestContainerEnvironmentNeverContainsCredentials(t *testing.T) {
	args, err := containerEnvironmentArgs(AgentConfig{
		Host: "https://gitlab.example", Token: "gitlab-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "gitlab-secret") || strings.Contains(joined, "GITLAB_TOKEN") {
		t.Fatalf("non-secret environment contains a credential: %s", joined)
	}
}

func TestDockerAdapterPreflightUsesSharedIdentityAndSecretMount(t *testing.T) {
	const gitLabSecret = "adapter-gitlab-secret"
	cli := &fakeDockerCLI{}
	var stagedPath string
	cli.runFunc = func(_ context.Context, _ io.Reader, _ io.Writer, _ io.Writer, args []string) error {
		stagedPath = mountedSource(args, containerSecretPath)
		data, err := os.ReadFile(stagedPath)
		if err != nil {
			t.Fatalf("read adapter credential: %v", err)
		}
		if !bytes.Contains(data, []byte(gitLabSecret)) {
			t.Fatalf("adapter credential was not staged: %s", data)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"--user 501:20", "uid=501,gid=20", "dst=" + containerSecretPath + ",readonly"} {
			if !strings.Contains(joined, want) {
				t.Errorf("adapter preflight does not contain %q: %s", want, joined)
			}
		}
		if strings.Contains(joined, "--env-file") || strings.Contains(joined, gitLabSecret) {
			t.Fatalf("adapter preflight persisted a credential: %s", joined)
		}
		return nil
	}
	runner := &DockerRunner{
		CLI: cli, Image: "bench:test", Identity: ContainerIdentity{UID: 501, GID: 20},
	}
	err := dockerAdapterPreflight(context.Background(), Config{
		Agent: AgentCodex, Tool: "gl-axi", Host: "https://gitlab.example", Token: gitLabSecret,
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adapter credential still exists after preflight: %v", err)
	}
}

func TestParseContainerIdentityRequiresNonRootPOSIXUser(t *testing.T) {
	identity, err := parseContainerIdentity(&user.User{Uid: "1000", Gid: "100"})
	if err != nil || identity != (ContainerIdentity{UID: 1000, GID: 100}) {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	for _, current := range []*user.User{
		{Uid: "0", Gid: "0"},
		{Uid: "S-1-5-21", Gid: "S-1-5-32"},
	} {
		if _, err := parseContainerIdentity(current); err == nil {
			t.Fatalf("identity %+v unexpectedly passed", current)
		}
	}
}

func mountedSource(args []string, destination string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != "--mount" || !strings.Contains(args[i+1], "dst="+destination) {
			continue
		}
		for _, part := range strings.Split(args[i+1], ",") {
			if strings.HasPrefix(part, "src=") {
				return strings.TrimPrefix(part, "src=")
			}
		}
	}
	return ""
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
