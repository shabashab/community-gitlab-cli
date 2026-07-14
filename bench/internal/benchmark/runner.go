package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	HelperNone   = "none"
	HelperNative = "native"
)

type Config struct {
	Host          string
	Token         string
	Group         string
	Agent         string
	Model         string
	Effort        string
	Tool          string
	HelperMode    string
	HelperFile    string
	TaskIDs       []string
	Trials        int
	Timeout       time.Duration
	MaxTurns      int
	RootDir       string
	ResultsDir    string
	Out           io.Writer
	Isolation     string
	CodexImage    string
	ClaudeImage   string
	CodexAuthFile string
	KeepContainer bool
	Runner        AgentRunner
}

type Summary struct {
	RunID           string       `json:"run_id"`
	RunDir          string       `json:"run_dir"`
	Total           int          `json:"total"`
	Passed          int          `json:"passed"`
	Failed          int          `json:"failed"`
	Usage           SummaryUsage `json:"usage"`
	AgentDurationMS int64        `json:"agent_duration_ms"`
	WallDurationMS  int64        `json:"wall_duration_ms"`
	CostUSD         *float64     `json:"cost_usd"`
	CostSource      string       `json:"cost_source"`
	Pricing         *Pricing     `json:"pricing,omitempty"`
	StartedAt       time.Time    `json:"started_at"`
	EndedAt         time.Time    `json:"ended_at"`
}

type helperMaterial struct {
	Path    string
	Content string
	SHA256  string
	Bytes   int
}

func Run(ctx context.Context, cfg Config) (Summary, error) {
	cfg = withConfigDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return Summary{}, err
	}
	secrets := benchmarkSecrets(cfg)
	tasks, err := SelectTasks(cfg.TaskIDs)
	if err != nil {
		return Summary{}, redactError(err, secrets)
	}
	helper, err := loadHelper(cfg)
	if err != nil {
		return Summary{}, redactError(err, secrets)
	}
	runner, err := newAgentRunner(ctx, cfg)
	if err != nil {
		return Summary{}, redactError(err, secrets)
	}
	toolVersion, err := runnerToolVersion(ctx, cfg, runner)
	if err != nil {
		return Summary{}, redactError(err, secrets)
	}

	runID := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + cfg.Agent + "-" + cfg.Tool
	runDir := filepath.Join(cfg.ResultsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Summary{}, redactError(fmt.Errorf("create benchmark result directory: %w", err), secrets)
	}
	startedAt := time.Now().UTC()
	manifest, err := newRunManifest(ctx, cfg, runID, helper, runner, startedAt)
	if err != nil {
		return Summary{}, redactError(err, secrets)
	}
	if err := writeManifest(runDir, manifest); err != nil {
		return Summary{}, redactError(fmt.Errorf("write benchmark manifest: %w", err), secrets)
	}
	summary := newSummary(cfg, runID, runDir, startedAt)

	resultsFile, err := os.OpenFile(filepath.Join(runDir, "trials.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return summary, finishFailedRun(runDir, &manifest, &summary, secrets, fmt.Errorf("open benchmark results: %w", err))
	}
	defer resultsFile.Close()
	encoder := json.NewEncoder(resultsFile)

	for _, task := range tasks {
		for trial := 1; trial <= cfg.Trials; trial++ {
			result, trialErr := runTrial(ctx, cfg, runner, runID, runDir, toolVersion, helper, task, trial, secrets)
			if result.RunID != "" {
				// Enforce redaction again at the serialization and terminal-output
				// boundary in case a future caller mutates a diagnostic late.
				redactTrialResult(&result, secrets)
				if err := encoder.Encode(result); err != nil {
					return summary, finishFailedRun(runDir, &manifest, &summary, secrets, fmt.Errorf("write benchmark trial result: %w", err))
				}
				addResultToRun(cfg, &summary, result)
			}
			if trialErr != nil {
				return summary, finishFailedRun(runDir, &manifest, &summary, secrets, trialErr)
			}
			if result.RunID == "" {
				return summary, finishFailedRun(runDir, &manifest, &summary, secrets, errors.New("trial completed without a result"))
			}
			if err := resultsFile.Sync(); err != nil {
				return summary, finishFailedRun(runDir, &manifest, &summary, secrets, fmt.Errorf("sync benchmark trial result: %w", err))
			}
		}
	}
	summary.EndedAt = time.Now().UTC()
	summary.WallDurationMS = summary.EndedAt.Sub(summary.StartedAt).Milliseconds()
	if err := writeSummary(runDir, summary); err != nil {
		return summary, finishFailedRun(runDir, &manifest, &summary, secrets, err)
	}
	manifest.Status = "complete"
	manifest.EndedAt = summary.EndedAt
	if err := writeManifest(runDir, manifest); err != nil {
		return summary, redactError(fmt.Errorf("finalize benchmark manifest: %w", err), secrets)
	}
	return summary, nil
}

func addResultToRun(cfg Config, summary *Summary, result TrialResult) {
	summary.Total++
	addTrialToSummary(summary, cfg.Agent, result)
	if result.Grade.Passed {
		summary.Passed++
		fmt.Fprintf(cfg.Out, "PASS %-14s trial=%d duration=%s\n", result.TaskID, result.Trial, time.Duration(result.DurationMS)*time.Millisecond)
		return
	}
	summary.Failed++
	fmt.Fprintf(cfg.Out, "FAIL %-14s trial=%d duration=%s error=%s failures=%s\n",
		result.TaskID,
		result.Trial,
		time.Duration(result.DurationMS)*time.Millisecond,
		result.Error,
		strings.Join(result.Grade.Failures, "; "),
	)
}

func finishFailedRun(runDir string, manifest *RunManifest, summary *Summary, secrets []string, runErr error) error {
	runErr = redactError(runErr, secrets)
	endedAt := time.Now().UTC()
	summary.EndedAt = endedAt
	summary.WallDurationMS = endedAt.Sub(summary.StartedAt).Milliseconds()
	_ = writeSummary(runDir, *summary)
	manifest.Status = "failed"
	manifest.Error = runErr.Error()
	manifest.EndedAt = endedAt
	_ = writeManifest(runDir, *manifest)
	return runErr
}

func runTrial(
	ctx context.Context,
	cfg Config,
	runner AgentRunner,
	runID string,
	runDir string,
	toolVersion string,
	helper helperMaterial,
	task Task,
	trial int,
	secrets []string,
) (TrialResult, error) {
	tempRoot := filepath.Join(os.TempDir(), "community-gitlab-cli-bench")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return TrialResult{}, fmt.Errorf("create benchmark temp root: %w", err)
	}
	workDir, err := os.MkdirTemp(tempRoot, "trial-")
	if err != nil {
		return TrialResult{}, fmt.Errorf("create trial work directory: %w", err)
	}
	workspaceCleaned := false
	retainWorkspace := false
	defer func() {
		if !workspaceCleaned && !retainWorkspace {
			_ = os.RemoveAll(workDir)
		}
	}()

	fixture, err := ProvisionFixture(ctx, FixtureConfig{
		Host:    cfg.Host,
		Token:   cfg.Token,
		Group:   cfg.Group,
		WorkDir: workDir,
	})
	if err != nil {
		_ = os.RemoveAll(workDir)
		workspaceCleaned = true
		return TrialResult{}, fmt.Errorf("provision task %s trial %d: %w", task.ID, trial, err)
	}
	prompt := BuildPrompt(cfg.Tool, task, fixture, helper.Content)
	promptSum := sha256.Sum256([]byte(prompt))
	started := time.Now().UTC()
	trialCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	agentResult, runtimeMetadata, agentErr := runner.Run(trialCtx, AgentConfig{
		Agent:    cfg.Agent,
		Model:    cfg.Model,
		Effort:   cfg.Effort,
		Tool:     cfg.Tool,
		Prompt:   prompt,
		WorkDir:  fixture.RepoDir,
		RepoRoot: cfg.RootDir,
		Host:     cfg.Host,
		Token:    cfg.Token,
		MaxTurns: cfg.MaxTurns,
		RunID:    runID,
		TaskID:   task.ID,
		Trial:    trial,
		Project:  fixture.Project.PathWithNamespace,
	})
	cancel()
	redactAgentResult(&agentResult, secrets)

	traceName := fmt.Sprintf("%s-trial-%02d.jsonl", task.ID, trial)
	stderrName := fmt.Sprintf("%s-trial-%02d.stderr", task.ID, trial)
	tracePath := filepath.Join(runDir, traceName)
	stderrPath := filepath.Join(runDir, stderrName)
	var infrastructureErr error
	if err := os.WriteFile(tracePath, agentResult.RawEvents, 0o600); err != nil {
		infrastructureErr = fmt.Errorf("write agent trace: %w", err)
	}
	if err := os.WriteFile(stderrPath, agentResult.RawStderr, 0o600); err != nil && infrastructureErr == nil {
		infrastructureErr = fmt.Errorf("write agent stderr: %w", err)
	}

	grade := Grade{Failures: []string{"agent execution failed before grading"}}
	if agentErr == nil {
		grade = task.Grade(fixture, agentResult)
	}
	if agentResult.PolicyViolation != "" {
		grade.Passed = false
		grade.Failures = append(grade.Failures, agentResult.PolicyViolation)
	}

	result := TrialResult{
		RunID:           runID,
		TaskID:          task.ID,
		Trial:           trial,
		Agent:           cfg.Agent,
		Model:           cfg.Model,
		Effort:          cfg.Effort,
		Tool:            cfg.Tool,
		ToolVersion:     toolVersion,
		HelperPath:      helper.Path,
		HelperSHA256:    helper.SHA256,
		HelperBytes:     helper.Bytes,
		PromptSHA256:    hex.EncodeToString(promptSum[:]),
		Project:         fixture.Project.Path,
		MergeRequestIID: fixture.MergeRequest.IID,
		StartedAt:       started,
		DurationMS:      agentResult.Duration.Milliseconds(),
		Runtime:         runtimeMetadata,
		Usage:           agentResult.Usage,
		Commands:        agentResult.Commands,
		FinalMessage:    agentResult.FinalMessage,
		Grade:           grade,
		PolicyViolation: agentResult.PolicyViolation,
		TracePath:       traceName,
		StderrPath:      stderrName,
	}
	if agentErr != nil {
		result.Error = agentErr.Error()
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	fixtureErr := fixture.Cleanup(cleanupCtx)
	cleanupCancel()
	if fixtureErr == nil {
		result.Runtime.Cleanup.FixtureRemoved = true
	} else {
		result.Runtime.Cleanup.Error = fixtureErr.Error()
		if infrastructureErr == nil {
			infrastructureErr = fmt.Errorf("cleanup fixture %s: %w", fixture.Project.PathWithNamespace, fixtureErr)
		}
	}
	if cfg.KeepContainer && result.Runtime.ContainerID != "" {
		retainWorkspace = true
		result.Runtime.Cleanup.Retained = true
		fmt.Fprintf(cfg.Out, "RETAIN container=%s workspace=%s credentials_removed=true restartable=false\n", result.Runtime.ContainerName, workDir)
	} else if err := os.RemoveAll(workDir); err != nil {
		result.Runtime.Cleanup.Error = err.Error()
		if infrastructureErr == nil {
			infrastructureErr = fmt.Errorf("remove trial workspace: %w", err)
		}
	} else {
		workspaceCleaned = true
		result.Runtime.Cleanup.WorkspaceRemoved = true
	}

	if isInfrastructureError(agentErr) && infrastructureErr == nil {
		infrastructureErr = agentErr
	}
	redactTrialResult(&result, secrets)
	return result, redactError(infrastructureErr, secrets)
}

func withConfigDefaults(cfg Config) Config {
	if cfg.Isolation == "" {
		cfg.Isolation = IsolationDocker
	}
	if cfg.CodexImage == "" {
		cfg.CodexImage = DefaultCodexImage
	}
	if cfg.ClaudeImage == "" {
		cfg.ClaudeImage = DefaultClaudeImage
	}
	if cfg.CodexAuthFile == "" {
		cfg.CodexAuthFile = DefaultCodexAuthFile()
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	return cfg
}

func newAgentRunner(ctx context.Context, cfg Config) (AgentRunner, error) {
	if cfg.Runner != nil {
		return cfg.Runner, nil
	}
	if cfg.Isolation == IsolationLocal {
		return LocalRunner{}, nil
	}
	return NewDockerRunner(ctx, cfg)
}

func runnerToolVersion(ctx context.Context, cfg Config, runner AgentRunner) (string, error) {
	if dockerRunner, ok := runner.(*DockerRunner); ok {
		return strings.TrimSpace(dockerRunner.ImageMetadata.AdapterVersion + " sha256=" + dockerRunner.ImageMetadata.AdapterSHA256), nil
	}
	return commandVersion(ctx, cfg)
}

func validateConfig(cfg Config) error {
	missing := make([]string, 0, 3)
	if strings.TrimSpace(cfg.Host) == "" {
		missing = append(missing, "GL_BENCH_HOST")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		missing = append(missing, "GL_BENCH_TOKEN")
	}
	if strings.TrimSpace(cfg.Group) == "" {
		missing = append(missing, "GL_BENCH_GROUP")
	}
	if len(missing) != 0 {
		return fmt.Errorf("missing benchmark environment: %s", strings.Join(missing, ", "))
	}
	if cfg.Agent != AgentClaude && cfg.Agent != AgentCodex {
		return fmt.Errorf("unsupported agent %q", cfg.Agent)
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return errors.New("--model is required")
	}
	switch cfg.Tool {
	case "gl", "gl-axi", "glab":
	default:
		return fmt.Errorf("unsupported MVP tool %q (use gl, gl-axi, or glab)", cfg.Tool)
	}
	if cfg.Trials < 1 {
		return errors.New("--trials must be at least 1")
	}
	if cfg.Timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if cfg.Isolation != IsolationDocker && cfg.Isolation != IsolationLocal {
		return fmt.Errorf("unsupported isolation %q (use docker or local)", cfg.Isolation)
	}
	if cfg.KeepContainer && cfg.Isolation != IsolationDocker {
		return errors.New("--keep-container requires --isolation docker")
	}
	return nil
}

func loadHelper(cfg Config) (helperMaterial, error) {
	path := strings.TrimSpace(cfg.HelperFile)
	mode := strings.TrimSpace(cfg.HelperMode)
	if mode == "" || mode == HelperNone {
		if path == "" {
			return helperMaterial{}, nil
		}
	} else if mode == HelperNative {
		if path == "" && cfg.Tool == "gl-axi" {
			path = filepath.Join(cfg.RootDir, ".agents", "skills", "gl-axi", "SKILL.md")
		}
		if path == "" {
			return helperMaterial{}, fmt.Errorf("tool %q has no native helper configured in the MVP; use --helper-file", cfg.Tool)
		}
	} else {
		return helperMaterial{}, fmt.Errorf("unsupported helper mode %q", mode)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return helperMaterial{}, fmt.Errorf("read helper material %q: %w", path, err)
	}
	sum := sha256.Sum256(content)
	return helperMaterial{
		Path:    path,
		Content: string(content),
		SHA256:  hex.EncodeToString(sum[:]),
		Bytes:   len(content),
	}, nil
}

func commandVersion(ctx context.Context, cfg Config) (string, error) {
	path, err := toolPath(cfg.RootDir, cfg.Tool)
	if err != nil {
		return "", err
	}
	if cfg.Tool == "gl" || cfg.Tool == "gl-axi" {
		binary, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s binary: %w", cfg.Tool, err)
		}
		sum := sha256.Sum256(binary)
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
		cmd.Dir = cfg.RootDir
		commit, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("read repository commit: %w", err)
		}
		return fmt.Sprintf("commit=%s sha256=%s", strings.TrimSpace(string(commit)), hex.EncodeToString(sum[:])), nil
	}
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read %s version: %w: %s", cfg.Tool, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func toolPath(rootDir, tool string) (string, error) {
	if tool == "gl" || tool == "gl-axi" {
		path := filepath.Join(rootDir, "bin", tool)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath(tool)
	if err != nil {
		return "", fmt.Errorf("find tool %q: %w", tool, err)
	}
	return path, nil
}

func writeSummary(runDir string, summary Summary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), data, 0o600); err != nil {
		return fmt.Errorf("write benchmark summary: %w", err)
	}
	return nil
}

type PreflightCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Details string `json:"details"`
}

func Preflight(ctx context.Context, cfg Config) []PreflightCheck {
	cfg = withConfigDefaults(cfg)
	checks := []PreflightCheck{
		{Name: "GL_BENCH_HOST", OK: cfg.Host != "", Details: "configured=" + fmt.Sprint(cfg.Host != "")},
		{Name: "GL_BENCH_TOKEN", OK: cfg.Token != "", Details: "configured=" + fmt.Sprint(cfg.Token != "")},
		{Name: "GL_BENCH_GROUP", OK: cfg.Group != "", Details: "configured=" + fmt.Sprint(cfg.Group != "")},
		{Name: "model", OK: strings.TrimSpace(cfg.Model) != "", Details: "configured=" + fmt.Sprint(strings.TrimSpace(cfg.Model) != "")},
		{Name: "agent", OK: cfg.Agent == AgentClaude || cfg.Agent == AgentCodex, Details: cfg.Agent},
		{Name: "tool", OK: cfg.Tool == "gl" || cfg.Tool == "gl-axi" || cfg.Tool == "glab", Details: cfg.Tool},
		{Name: "isolation", OK: cfg.Isolation == IsolationDocker || cfg.Isolation == IsolationLocal, Details: cfg.Isolation},
	}
	configurationOK := true
	for _, check := range checks {
		if !check.OK {
			configurationOK = false
		}
	}
	if !configurationOK {
		return checks
	}
	commands := []string{"git"}
	if cfg.Isolation == IsolationDocker {
		commands = append(commands, "docker")
	} else {
		commands = append(commands, cfg.Agent, cfg.Tool)
	}
	for _, command := range commands {
		path, err := exec.LookPath(command)
		if cfg.Isolation == IsolationLocal && command == cfg.Tool {
			path, err = toolPath(cfg.RootDir, command)
		}
		check := PreflightCheck{Name: "command:" + command, OK: err == nil, Details: path}
		if err != nil {
			check.Details = err.Error()
		}
		checks = append(checks, check)
	}

	if cfg.Isolation == IsolationDocker {
		runner, err := NewDockerRunner(ctx, cfg)
		imageCheck := PreflightCheck{Name: "docker-image", OK: err == nil}
		if err != nil {
			imageCheck.Details = err.Error()
		} else {
			imageCheck.Details = fmt.Sprintf("id=%s os=%s arch=%s agent=%s adapter_sha256=%s",
				runner.ImageMetadata.ID,
				runner.ImageMetadata.OS,
				runner.ImageMetadata.Architecture,
				runner.ImageMetadata.AgentVersion,
				runner.ImageMetadata.AdapterSHA256,
			)
		}
		checks = append(checks, imageCheck)

		authSource, authErr := providerAuthSource(cfg)
		authCheck := PreflightCheck{Name: "provider-auth", OK: authErr == nil, Details: authSource}
		if authErr != nil {
			authCheck.Details = authErr.Error()
		}
		checks = append(checks, authCheck)

		if err == nil && authErr == nil && cfg.Host != "" && cfg.Token != "" {
			adapterErr := dockerAdapterPreflight(ctx, cfg, runner)
			adapterCheck := PreflightCheck{Name: "container-gitlab-auth", OK: adapterErr == nil, Details: "authenticated=true"}
			if adapterErr != nil {
				adapterCheck.Details = adapterErr.Error()
			}
			checks = append(checks, adapterCheck)

			providerErr := dockerProviderPreflight(ctx, cfg, runner)
			providerCheck := PreflightCheck{Name: "container-provider", OK: providerErr == nil, Details: "request=ok"}
			if providerErr != nil {
				providerCheck.Details = providerErr.Error()
			}
			checks = append(checks, providerCheck)
		}
	}

	if cfg.Host != "" && cfg.Token != "" {
		_, err := gitLabCurrentUser(ctx, cfg.Host, cfg.Token)
		check := PreflightCheck{Name: "gitlab-auth", OK: err == nil}
		if err != nil {
			check.Details = err.Error()
		} else {
			check.Details = "authenticated=true"
		}
		checks = append(checks, check)
	}
	return checks
}

func providerAuthSource(cfg Config) (string, error) {
	if cfg.Agent == AgentCodex {
		if os.Getenv("CODEX_ACCESS_TOKEN") != "" {
			return "source=CODEX_ACCESS_TOKEN", nil
		}
		if _, err := os.Stat(cfg.CodexAuthFile); err == nil {
			return "source=codex_auth_file", nil
		}
		if os.Getenv("CODEX_API_KEY") != "" {
			return "source=CODEX_API_KEY", nil
		}
		return "", errors.New("set CODEX_ACCESS_TOKEN or provide --codex-auth-file")
	}
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		return "source=CLAUDE_CODE_OAUTH_TOKEN", nil
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "source=ANTHROPIC_API_KEY", nil
	}
	return "", errors.New("set CLAUDE_CODE_OAUTH_TOKEN from `claude setup-token`")
}

func dockerAdapterPreflight(ctx context.Context, cfg Config, runner *DockerRunner) error {
	dir, err := os.MkdirTemp("", "gl-bench-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	secretPath, err := writeContainerSecrets(dir, AgentConfig{Host: cfg.Host, Token: cfg.Token}, cfg.Agent, cfg.CodexAuthFile, false)
	if err != nil {
		return err
	}
	defer os.Remove(secretPath)
	envArgs, err := containerEnvironmentArgs(AgentConfig{Host: cfg.Host})
	if err != nil {
		return err
	}
	args := []string{
		"run", "--rm",
		"--network", defaultContainerResources.NetworkMode,
	}
	args = append(args, containerIdentityArgs(runner.Identity, "256m")...)
	args = append(args, envArgs...)
	args = append(args, "--mount", "type=bind,src="+secretPath+",dst="+containerSecretPath+",readonly", runner.Image)
	switch cfg.Tool {
	case "gl", "gl-axi":
		args = append(args, cfg.Tool, "--output", "json", "whoami")
	case "glab":
		args = append(args, "glab", "api", "user", "--silent")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runner.CLI.Run(ctx, nil, &stdout, &stderr, args...); err != nil {
		return fmt.Errorf("selected adapter could not authenticate in image: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func dockerProviderPreflight(ctx context.Context, cfg Config, runner *DockerRunner) error {
	dir, err := os.MkdirTemp("", "gl-bench-provider-preflight-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	repoDir := filepath.Join(dir, "repo")
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		return err
	}
	gitCmd := exec.CommandContext(ctx, "git", "init")
	gitCmd.Dir = repoDir
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("initialize provider preflight repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	preflightCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	result, _, err := runner.Run(preflightCtx, AgentConfig{
		Agent: cfg.Agent, Model: cfg.Model, Effort: cfg.Effort, Tool: cfg.Tool,
		Prompt:  "Reply with exactly benchmark-preflight-ok. Do not use any tools.",
		WorkDir: repoDir, RepoRoot: cfg.RootDir, Host: cfg.Host, Token: cfg.Token,
		MaxTurns: 1, RunID: "preflight", TaskID: TaskID("provider"), Trial: 0, Project: "preflight",
	})
	if err != nil {
		return err
	}
	if !strings.Contains(result.FinalMessage, "benchmark-preflight-ok") {
		return fmt.Errorf("provider preflight returned unexpected final message %q", result.FinalMessage)
	}
	return nil
}

func gitLabCurrentUser(ctx context.Context, host, token string) (string, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(host), gitlab.WithUserAgent("community-gitlab-cli-benchmark"))
	if err != nil {
		return "", err
	}
	user, _, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return "", err
	}
	return user.Username, nil
}
