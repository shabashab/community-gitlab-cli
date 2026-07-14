package benchmark

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	IsolationLocal  = "local"
	IsolationDocker = "docker"

	DefaultCodexImage  = "community-gitlab-cli-bench-codex:local"
	DefaultClaudeImage = "community-gitlab-cli-bench-claude:local"
)

func DefaultCodexAuthFile() string {
	if home := strings.TrimSpace(os.Getenv("CODEX_HOME")); home != "" {
		return filepath.Join(home, "auth.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex", "auth.json")
}

var defaultContainerResources = ResourceLimits{
	Memory:      "2g",
	MemorySwap:  "2g",
	CPUs:        2,
	PIDs:        256,
	HomeTmpfs:   "256m",
	NetworkMode: "bridge",
}

// DockerCLI is deliberately smaller than a Docker SDK. Tests can assert the
// exact argv without needing a daemon, while production honors the active
// Docker CLI context.
type DockerCLI interface {
	Run(context.Context, io.Reader, io.Writer, io.Writer, ...string) error
	Output(context.Context, ...string) ([]byte, error)
}

type execDockerCLI struct{}

func (execDockerCLI) Run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (execDockerCLI) Output(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type infrastructureError struct{ err error }

func (e *infrastructureError) Error() string { return e.err.Error() }
func (e *infrastructureError) Unwrap() error { return e.err }

func newInfrastructureError(format string, args ...any) error {
	return &infrastructureError{err: fmt.Errorf(format, args...)}
}

func isInfrastructureError(err error) bool {
	var target *infrastructureError
	return errors.As(err, &target)
}

// DockerRunner owns a single image selection and immutable runtime profile.
// A new instance container is still created for every call to Run.
type DockerRunner struct {
	CLI           DockerCLI
	Image         string
	Agent         string
	Tool          string
	CodexAuthFile string
	KeepContainer bool
	Resources     ResourceLimits
	Docker        DockerRuntimeMetadata
	ImageMetadata ImageMetadata
}

func NewDockerRunner(ctx context.Context, cfg Config) (*DockerRunner, error) {
	cli := DockerCLI(execDockerCLI{})
	image := cfg.CodexImage
	if cfg.Agent == AgentClaude {
		image = cfg.ClaudeImage
	}
	if image == "" {
		return nil, errors.New("Docker benchmark image is empty")
	}
	dockerMetadata, err := inspectDockerRuntime(ctx, cli)
	if err != nil {
		return nil, err
	}
	imageMetadata, err := inspectDockerImage(ctx, cli, image, cfg.Agent, cfg.Tool)
	if err != nil {
		return nil, err
	}
	return &DockerRunner{
		CLI:           cli,
		Image:         image,
		Agent:         cfg.Agent,
		Tool:          cfg.Tool,
		CodexAuthFile: cfg.CodexAuthFile,
		KeepContainer: cfg.KeepContainer,
		Resources:     defaultContainerResources,
		Docker:        dockerMetadata,
		ImageMetadata: imageMetadata,
	}, nil
}

func (r *DockerRunner) Run(ctx context.Context, cfg AgentConfig) (AgentResult, RuntimeMetadata, error) {
	if r.CLI == nil {
		r.CLI = execDockerCLI{}
	}
	if r.Resources.Memory == "" {
		r.Resources = defaultContainerResources
	}
	runtime := RuntimeMetadata{
		Isolation: IsolationDocker,
		Docker:    r.Docker,
		Image:     r.ImageMetadata,
		Resources: r.Resources,
	}

	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return AgentResult{}, runtime, newInfrastructureError("resolve trial workspace: %w", err)
	}
	if strings.Contains(workDir, ",") {
		return AgentResult{}, runtime, newInfrastructureError("trial workspace cannot contain a comma: %q", workDir)
	}

	name, err := containerName(cfg)
	if err != nil {
		return AgentResult{}, runtime, newInfrastructureError("create container name: %w", err)
	}
	runtime.ContainerName = name

	trialDir := filepath.Dir(workDir)
	envPath, err := writeContainerEnv(trialDir, cfg, r.Agent, r.CodexAuthFile)
	if err != nil {
		return AgentResult{}, runtime, newInfrastructureError("prepare container environment: %w", err)
	}
	defer os.Remove(envPath)

	authPath, err := prepareCodexAuthSecret(trialDir, r.Agent, r.CodexAuthFile)
	if err != nil {
		return AgentResult{}, runtime, newInfrastructureError("prepare Codex account credentials: %w", err)
	}
	if authPath != "" {
		defer os.Remove(authPath)
	}

	commandCfg := cfg
	commandCfg.WorkDir = "/workspace"
	commandCfg.ExternalIsolation = true
	command, commandArgs, err := agentCommand(commandCfg)
	if err != nil {
		return AgentResult{}, runtime, err
	}

	createArgs := []string{
		"create",
		"--name", name,
		"--label", "community-gitlab-cli.benchmark=true",
		"--label", "community-gitlab-cli.run=" + sanitizeLabel(cfg.RunID),
		"--label", "community-gitlab-cli.task=" + sanitizeLabel(string(cfg.TaskID)),
		"--label", "community-gitlab-cli.trial=" + strconv.Itoa(cfg.Trial),
		"--label", "community-gitlab-cli.project=" + sanitizeLabel(cfg.Project),
		"--workdir", "/workspace",
		"--user", "10001:10001",
		"--init",
		"--interactive",
		"--memory", r.Resources.Memory,
		"--memory-swap", r.Resources.MemorySwap,
		"--cpus", strconv.FormatFloat(r.Resources.CPUs, 'f', -1, 64),
		"--pids-limit", strconv.Itoa(r.Resources.PIDs),
		"--tmpfs", "/home/bench:rw,size=" + r.Resources.HomeTmpfs + ",uid=10001,gid=10001,mode=0700",
		"--env-file", envPath,
		"--mount", "type=bind,src=" + workDir + ",dst=/workspace",
		"--network", r.Resources.NetworkMode,
	}
	if authPath != "" {
		if strings.Contains(authPath, ",") {
			return AgentResult{}, runtime, newInfrastructureError("Codex auth path cannot contain a comma")
		}
		createArgs = append(createArgs, "--mount", "type=bind,src="+authPath+",dst=/run/secrets/codex-auth.json,readonly")
	}
	createArgs = append(createArgs, r.Image, command)
	createArgs = append(createArgs, commandArgs...)

	var createOut bytes.Buffer
	var createErr bytes.Buffer
	if err := r.CLI.Run(ctx, nil, &createOut, &createErr, createArgs...); err != nil {
		return AgentResult{}, runtime, newInfrastructureError("create trial container: %w: %s", err, strings.TrimSpace(createErr.String()))
	}
	containerID := strings.TrimSpace(createOut.String())
	runtime.ContainerID = containerID
	_ = os.Remove(envPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	startRequested := time.Now().UTC()
	startErr := r.CLI.Run(ctx, strings.NewReader(cfg.Prompt), &stdout, &stderr, "start", "--attach", "--interactive", name)

	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCleanup()
	if ctx.Err() != nil {
		var ignoredOut bytes.Buffer
		var ignoredErr bytes.Buffer
		_ = r.CLI.Run(cleanupCtx, nil, &ignoredOut, &ignoredErr, "kill", name)
	}

	state, inspectErr := inspectContainerState(cleanupCtx, r.CLI, name)
	if inspectErr == nil {
		runtime.StartedAt = state.StartedAt
		runtime.FinishedAt = state.FinishedAt
		exitCode := state.ExitCode
		runtime.ExitCode = &exitCode
		runtime.OOMKilled = state.OOMKilled
		if !state.StartedAt.IsZero() {
			runtime.ContainerStartupMS = state.StartedAt.Sub(startRequested).Milliseconds()
			if runtime.ContainerStartupMS < 0 {
				runtime.ContainerStartupMS = 0
			}
		}
	}

	if r.KeepContainer {
		runtime.Cleanup.Retained = true
	} else {
		var removeOut bytes.Buffer
		var removeErr bytes.Buffer
		removeRunErr := r.CLI.Run(cleanupCtx, nil, &removeOut, &removeErr, "rm", "--force", name)
		if removeRunErr == nil {
			runtime.Cleanup.ContainerRemoved = true
		} else {
			runtime.Cleanup.Error = strings.TrimSpace(removeErr.String())
		}
		if inspectErr == nil && removeRunErr != nil {
			inspectErr = newInfrastructureError("remove trial container: %w: %s", removeRunErr, strings.TrimSpace(removeErr.String()))
		}
	}

	finished := time.Now().UTC()
	if runtime.StartedAt.IsZero() {
		runtime.StartedAt = startRequested
	}
	if runtime.FinishedAt.IsZero() {
		runtime.FinishedAt = finished
	}
	result := AgentResult{
		RawEvents: stdout.Bytes(),
		RawStderr: stderr.Bytes(),
		Duration:  runtime.FinishedAt.Sub(runtime.StartedAt),
	}
	if result.Duration < 0 {
		result.Duration = finished.Sub(startRequested)
	}
	parseAgentResult(cfg.Agent, cfg.Tool, &result)

	if inspectErr != nil {
		return result, runtime, newInfrastructureError("inspect trial container: %w", inspectErr)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		runtime.TimedOut = true
		return result, runtime, fmt.Errorf("agent timed out: %w", ctx.Err())
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		runtime.Canceled = true
		return result, runtime, fmt.Errorf("agent canceled: %w", ctx.Err())
	}
	if runtime.OOMKilled {
		return result, runtime, errors.New("agent container was OOM-killed")
	}
	if startErr != nil || (runtime.ExitCode != nil && *runtime.ExitCode != 0) {
		if startErr == nil {
			return result, runtime, fmt.Errorf("%s exited unsuccessfully (exit %d)", cfg.Agent, valueOrMinusOne(runtime.ExitCode))
		}
		return result, runtime, fmt.Errorf("%s exited unsuccessfully (exit %d): %w", cfg.Agent, valueOrMinusOne(runtime.ExitCode), startErr)
	}
	if strings.TrimSpace(result.FinalMessage) == "" {
		return result, runtime, errors.New("agent emitted no final message")
	}
	return result, runtime, nil
}

func parseAgentResult(agent, tool string, result *AgentResult) {
	switch agent {
	case AgentClaude:
		parseClaudeEvents(result)
	case AgentCodex:
		parseCodexEvents(result)
	}
	result.PolicyViolation = findPolicyViolation(result.Commands, tool)
}

type containerState struct {
	OOMKilled  bool      `json:"OOMKilled"`
	ExitCode   int       `json:"ExitCode"`
	StartedAt  time.Time `json:"StartedAt"`
	FinishedAt time.Time `json:"FinishedAt"`
}

func inspectContainerState(ctx context.Context, cli DockerCLI, name string) (containerState, error) {
	output, err := cli.Output(ctx, "inspect", "--format", "{{json .State}}", name)
	if err != nil {
		return containerState{}, err
	}
	var state containerState
	if err := json.Unmarshal(bytes.TrimSpace(output), &state); err != nil {
		return containerState{}, fmt.Errorf("decode container state: %w", err)
	}
	return state, nil
}

func inspectDockerRuntime(ctx context.Context, cli DockerCLI) (DockerRuntimeMetadata, error) {
	output, err := cli.Output(ctx, "version", "--format", "{{json .}}")
	if err != nil {
		return DockerRuntimeMetadata{}, newInfrastructureError("read Docker version: %w", err)
	}
	var version struct {
		Client struct{ Version string }
		Server struct {
			Version string
			Os      string
			Arch    string
		}
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &version); err != nil {
		return DockerRuntimeMetadata{}, newInfrastructureError("decode Docker version: %w", err)
	}
	contextOutput, err := cli.Output(ctx, "context", "show")
	if err != nil {
		return DockerRuntimeMetadata{}, newInfrastructureError("read Docker context: %w", err)
	}
	return DockerRuntimeMetadata{
		Context:            strings.TrimSpace(string(contextOutput)),
		ClientVersion:      version.Client.Version,
		ServerVersion:      version.Server.Version,
		ServerOS:           version.Server.Os,
		ServerArchitecture: version.Server.Arch,
	}, nil
}

func inspectDockerImage(ctx context.Context, cli DockerCLI, ref, agent, tool string) (ImageMetadata, error) {
	template := `{"id":{{json .Id}},"repo_digests":{{with .RepoDigests}}{{json .}}{{else}}[]{{end}},"os":{{json .Os}},"architecture":{{json .Architecture}}}`
	output, err := cli.Output(ctx, "image", "inspect", "--format", template, ref)
	if err != nil {
		return ImageMetadata{}, newInfrastructureError("inspect Docker image %q: %w", ref, err)
	}
	metadata := ImageMetadata{Ref: ref}
	if err := json.Unmarshal(bytes.TrimSpace(output), &metadata); err != nil {
		return ImageMetadata{}, newInfrastructureError("decode Docker image metadata: %w", err)
	}
	metadata.Ref = ref
	if labelOutput, labelErr := cli.Output(ctx, "image", "inspect", "--format", "{{json .Config.Labels}}", ref); labelErr == nil {
		_ = json.Unmarshal(bytes.TrimSpace(labelOutput), &metadata.Labels)
	}
	info, err := cli.Output(ctx, "run", "--rm", "--entrypoint", "/usr/local/bin/bench-image-info", ref)
	if err != nil {
		return ImageMetadata{}, newInfrastructureError("read benchmark image versions: %w", err)
	}
	values := parseKeyValues(info)
	metadata.AgentVersion = values["agent_version"]
	key := strings.ReplaceAll(tool, "-", "_")
	metadata.AdapterVersion = values[key+"_version"]
	metadata.AdapterSHA256 = values[key+"_sha256"]
	if values["agent"] != agent {
		return ImageMetadata{}, newInfrastructureError("image %q contains agent %q, want %q", ref, values["agent"], agent)
	}
	if metadata.AdapterSHA256 == "" {
		return ImageMetadata{}, newInfrastructureError("image %q does not report adapter %q", ref, tool)
	}
	return metadata, nil
}

func parseKeyValues(data []byte) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values
}

func writeContainerEnv(dir string, cfg AgentConfig, agent, codexAuthFile string) (string, error) {
	env := map[string]string{
		"HOME":                  "/home/bench",
		"CODEX_HOME":            "/home/bench/.codex",
		"GITLAB_BASE_URL":       cfg.Host,
		"GITLAB_HOST":           cfg.Host,
		"GITLAB_TOKEN":          cfg.Token,
		"GL_TOKEN":              "",
		"GLAB_CONFIG_DIR":       "/home/bench/.config/glab-cli",
		"GLAB_CHECK_UPDATE":     "false",
		"GLAB_NO_PROMPT":        "1",
		"DISABLE_AUTOUPDATER":   "1",
		"NO_COLOR":              "1",
		"GIT_CONFIG_COUNT":      "1",
		"GIT_CONFIG_KEY_0":      "safe.directory",
		"GIT_CONFIG_VALUE_0":    "/workspace",
		"CLAUDE_CODE_SAFE_MODE": "1",
		"CLAUDE_CONFIG_DIR":     "/home/bench/.claude",
	}
	if agent == AgentCodex {
		if value := os.Getenv("CODEX_ACCESS_TOKEN"); value != "" {
			env["CODEX_ACCESS_TOKEN"] = value
		} else if _, err := os.Stat(codexAuthFile); err != nil {
			if value := os.Getenv("CODEX_API_KEY"); value != "" {
				env["CODEX_API_KEY"] = value
			} else {
				return "", fmt.Errorf("no Codex account auth: set CODEX_ACCESS_TOKEN or provide --codex-auth-file")
			}
		}
	} else {
		if value := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); value != "" {
			env["CLAUDE_CODE_OAUTH_TOKEN"] = value
		} else if value := os.Getenv("ANTHROPIC_API_KEY"); value != "" {
			env["ANTHROPIC_API_KEY"] = value
		} else {
			return "", errors.New("no Claude account auth: set CLAUDE_CODE_OAUTH_TOKEN from `claude setup-token`")
		}
	}

	file, err := os.CreateTemp(dir, ".container-env-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	remove := true
	defer func() {
		file.Close()
		if remove {
			os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", err
	}
	for key, value := range env {
		if strings.ContainsAny(value, "\r\n") {
			return "", fmt.Errorf("environment value %s contains a newline", key)
		}
		if _, err := fmt.Fprintf(file, "%s=%s\n", key, value); err != nil {
			return "", err
		}
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	remove = false
	return path, nil
}

func prepareCodexAuthSecret(dir, agent, source string) (string, error) {
	if agent != AgentCodex || os.Getenv("CODEX_ACCESS_TOKEN") != "" {
		return "", nil
	}
	data, err := os.ReadFile(source)
	if err != nil {
		if os.Getenv("CODEX_API_KEY") != "" && errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	file, err := os.CreateTemp(dir, ".codex-auth-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	remove := true
	defer func() {
		file.Close()
		if remove {
			os.Remove(path)
		}
	}()
	// The containing trial directory is mode 0700. The file itself must be
	// readable by the fixed container UID before the entrypoint copies it into
	// the private tmpfs home and restores mode 0600.
	if err := file.Chmod(0o444); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	remove = false
	return path, nil
}

func containerName(cfg AgentConfig) (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	base := fmt.Sprintf("gl-bench-%s-%s-%02d-%s", cfg.RunID, cfg.TaskID, cfg.Trial, hex.EncodeToString(buf))
	var builder strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('-')
		}
	}
	name := strings.Trim(builder.String(), "-_.")
	if len(name) > 120 {
		name = name[:120]
	}
	return name, nil
}

func sanitizeLabel(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if len(value) > 200 {
		return value[:200]
	}
	return value
}

func valueOrMinusOne(value *int) int {
	if value == nil {
		return -1
	}
	return *value
}
