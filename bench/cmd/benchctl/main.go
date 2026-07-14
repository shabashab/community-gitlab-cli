package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shabashab/community-gitlab-cli/bench/internal/benchmark"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "benchmark:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: benchctl <list|preflight|run|clean> [flags]")
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}

	switch args[0] {
	case "list":
		for _, task := range benchmark.Tasks() {
			fmt.Printf("%-14s %s\n", task.ID, task.Description)
		}
		return nil
	case "preflight":
		cfg, _, err := parseConfig(root, "preflight", args[1:])
		if err != nil {
			return err
		}
		checks := benchmark.Preflight(ctx, cfg)
		failed := false
		for _, check := range checks {
			if !check.OK {
				failed = true
			}
		}
		encoded, err := json.MarshalIndent(checks, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		if failed {
			return errors.New("one or more preflight checks failed")
		}
		return nil
	case "run":
		cfg, failOnTaskFailure, err := parseConfig(root, "run", args[1:])
		if err != nil {
			return err
		}
		summary, err := benchmark.Run(ctx, cfg)
		if err != nil {
			return err
		}
		cost := "n/a"
		if summary.CostUSD != nil {
			cost = fmt.Sprintf("%.6f", *summary.CostUSD)
		}
		fmt.Printf("\nrun=%s passed=%d failed=%d total=%d input_tokens=%d output_tokens=%d agent_duration=%s wall_duration=%s cost_usd=%s cost_source=%s results=%s\n",
			summary.RunID,
			summary.Passed,
			summary.Failed,
			summary.Total,
			summary.Usage.InputTokens,
			summary.Usage.OutputTokens,
			time.Duration(summary.AgentDurationMS)*time.Millisecond,
			time.Duration(summary.WallDurationMS)*time.Millisecond,
			cost,
			summary.CostSource,
			summary.RunDir,
		)
		if failOnTaskFailure && summary.Failed != 0 {
			return fmt.Errorf("%d benchmark trial(s) failed", summary.Failed)
		}
		return nil
	case "clean":
		flags := flag.NewFlagSet("clean", flag.ContinueOnError)
		maxAge := flags.Duration("max-age", time.Hour, "only remove resources older than this")
		dryRun := flags.Bool("dry-run", false, "list matching resources without removing them")
		hard := flags.Bool("hard", false, "permanently remove projects already pending deletion")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		return benchmark.Clean(ctx, benchmark.CleanConfig{
			Host:   firstNonEmpty(os.Getenv("GL_BENCH_HOST"), os.Getenv("GL_E2E_HOST")),
			Token:  firstNonEmpty(os.Getenv("GL_BENCH_TOKEN"), os.Getenv("GL_E2E_TOKEN")),
			Group:  firstNonEmpty(os.Getenv("GL_BENCH_GROUP"), os.Getenv("GL_E2E_GROUP")),
			MaxAge: *maxAge,
			DryRun: *dryRun,
			Hard:   *hard,
			Out:    os.Stdout,
		})
	default:
		return fmt.Errorf("unknown command %q (use list, preflight, run, or clean)", args[0])
	}
}

func parseConfig(root, name string, args []string) (benchmark.Config, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	agent := flags.String("agent", benchmark.AgentCodex, "agent harness: claude or codex")
	model := flags.String("model", "", "exact model ID")
	effort := flags.String("effort", "high", "reasoning effort")
	tool := flags.String("tool", "gl-axi", "GitLab adapter: gl, gl-axi, or glab")
	helper := flags.String("helper", benchmark.HelperNone, "helper mode: none or native")
	helperFile := flags.String("helper-file", "", "custom helper material to append to the prompt")
	taskList := flags.String("tasks", "", "comma-separated task IDs (default: all MVP tasks)")
	trials := flags.Int("trials", 1, "trials per task")
	timeout := flags.Duration("timeout", 5*time.Minute, "per-agent trial timeout")
	maxTurns := flags.Int("max-turns", 25, "Claude Code turn limit")
	resultsDir := flags.String("results-dir", filepath.Join(root, "bench", "results"), "result directory")
	isolation := flags.String("isolation", benchmark.IsolationDocker, "trial isolation: docker or local")
	codexImage := flags.String("codex-image", benchmark.DefaultCodexImage, "Codex benchmark image")
	claudeImage := flags.String("claude-image", benchmark.DefaultClaudeImage, "Claude benchmark image")
	codexAuthFile := flags.String("codex-auth-file", benchmark.DefaultCodexAuthFile(), "file-backed Codex account credentials")
	keepContainer := flags.Bool("keep-container", false, "retain the sanitized stopped container and workspace for inspection (not restartable)")
	failOnTaskFailure := flags.Bool("fail-on-task-failure", false, "exit nonzero when a graded trial fails")
	if err := flags.Parse(args); err != nil {
		return benchmark.Config{}, false, err
	}
	if flags.NArg() != 0 {
		return benchmark.Config{}, false, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	var taskIDs []string
	if strings.TrimSpace(*taskList) != "" {
		taskIDs = strings.Split(*taskList, ",")
	}
	cfg := benchmark.Config{
		Host:          firstNonEmpty(os.Getenv("GL_BENCH_HOST"), os.Getenv("GL_E2E_HOST")),
		Token:         firstNonEmpty(os.Getenv("GL_BENCH_TOKEN"), os.Getenv("GL_E2E_TOKEN")),
		Group:         firstNonEmpty(os.Getenv("GL_BENCH_GROUP"), os.Getenv("GL_E2E_GROUP")),
		Agent:         *agent,
		Model:         *model,
		Effort:        *effort,
		Tool:          *tool,
		HelperMode:    *helper,
		HelperFile:    *helperFile,
		TaskIDs:       taskIDs,
		Trials:        *trials,
		Timeout:       *timeout,
		MaxTurns:      *maxTurns,
		RootDir:       root,
		ResultsDir:    *resultsDir,
		Out:           os.Stdout,
		Isolation:     *isolation,
		CodexImage:    *codexImage,
		ClaudeImage:   *claudeImage,
		CodexAuthFile: *codexAuthFile,
		KeepContainer: *keepContainer,
	}
	return cfg, *failOnTaskFailure, nil
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run benchctl from inside the community-gitlab-cli repository")
		}
		dir = parent
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
