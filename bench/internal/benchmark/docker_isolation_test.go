//go:build dockerintegration

package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeDockerImage = "community-gitlab-cli-bench-fake:test"

func TestDockerIsolation(t *testing.T) {
	t.Setenv("CODEX_ACCESS_TOKEN", "fake-account-token")
	root := benchmarkRepositoryRoot(t)
	build := exec.Command("docker", "build", "--file", filepath.Join(root, "bench/docker/testdata/Dockerfile.fake"), "--tag", fakeDockerImage, filepath.Join(root, "bench/docker/testdata"))
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake benchmark image: %v\n%s", err, output)
	}

	runner, err := NewDockerRunner(context.Background(), withConfigDefaults(Config{
		Agent: AgentCodex, Tool: "gl-axi", CodexImage: fakeDockerImage,
	}))
	if err != nil {
		t.Fatal(err)
	}

	firstResult, firstRuntime := runFakeDockerTrial(t, runner, "first")
	secondResult, secondRuntime := runFakeDockerTrial(t, runner, "second")
	if firstRuntime.ContainerID == secondRuntime.ContainerID || firstRuntime.ContainerName == secondRuntime.ContainerName {
		t.Fatalf("trials reused a container: first=%+v second=%+v", firstRuntime, secondRuntime)
	}
	if !firstRuntime.Cleanup.ContainerRemoved || !secondRuntime.Cleanup.ContainerRemoved {
		t.Fatalf("container cleanup failed: first=%+v second=%+v", firstRuntime.Cleanup, secondRuntime.Cleanup)
	}
	for _, result := range []AgentResult{firstResult, secondResult} {
		if result.FinalMessage != "benchmark fake complete" || !bytes.Contains(result.RawStderr, []byte("fake stderr")) {
			t.Fatalf("unexpected fake result: %+v stderr=%q", result, result.RawStderr)
		}
	}

	t.Run("account auth file", func(t *testing.T) {
		t.Setenv("CODEX_ACCESS_TOKEN", "")
		authFile := filepath.Join(t.TempDir(), "auth.json")
		if err := os.WriteFile(authFile, []byte(`{"access_token":"fake"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		authRunner := *runner
		authRunner.CodexAuthFile = authFile
		result, runtime := runFakeDockerTrial(t, &authRunner, "require-auth")
		if result.FinalMessage == "" || !runtime.Cleanup.ContainerRemoved {
			t.Fatalf("result=%+v runtime=%+v", result, runtime)
		}
	})

	t.Run("parallel", func(t *testing.T) {
		for _, name := range []string{"a", "b"} {
			name := name
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				result, runtime := runFakeDockerTrial(t, runner, "parallel-"+name)
				if result.FinalMessage == "" || !runtime.Cleanup.ContainerRemoved {
					t.Fatalf("result=%+v runtime=%+v", result, runtime)
				}
			})
		}
	})

	t.Run("timeout", func(t *testing.T) {
		workDir := writableFakeRepository(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, runtime, err := runner.Run(ctx, fakeAgentConfig(workDir, "timeout"))
		if err == nil || !runtime.TimedOut || !runtime.Cleanup.ContainerRemoved {
			t.Fatalf("err=%v runtime=%+v", err, runtime)
		}
		assertNoContainer(t, runtime.ContainerName)
	})

	t.Run("crash", func(t *testing.T) {
		workDir := writableFakeRepository(t)
		_, runtime, err := runner.Run(context.Background(), fakeAgentConfig(workDir, "crash"))
		if err == nil || runtime.ExitCode == nil || *runtime.ExitCode != 9 || !runtime.Cleanup.ContainerRemoved {
			t.Fatalf("err=%v runtime=%+v", err, runtime)
		}
		assertNoContainer(t, runtime.ContainerName)
	})

	t.Run("oom", func(t *testing.T) {
		oomRunner := *runner
		oomRunner.Resources = ResourceLimits{
			Memory: "16m", MemorySwap: "16m", CPUs: 1, PIDs: 32,
			HomeTmpfs: "8m", NetworkMode: "bridge",
		}
		workDir := writableFakeRepository(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, runtime, err := oomRunner.Run(ctx, fakeAgentConfig(workDir, "oom"))
		if err == nil || !runtime.OOMKilled || !runtime.Cleanup.ContainerRemoved {
			t.Fatalf("err=%v runtime=%+v", err, runtime)
		}
		assertNoContainer(t, runtime.ContainerName)
	})
}

func runFakeDockerTrial(t *testing.T, runner *DockerRunner, prompt string) (AgentResult, RuntimeMetadata) {
	t.Helper()
	workDir := writableFakeRepository(t)
	result, runtime, err := runner.Run(context.Background(), fakeAgentConfig(workDir, prompt))
	if err != nil {
		t.Fatal(err)
	}
	assertNoContainer(t, runtime.ContainerName)
	return result, runtime
}

func fakeAgentConfig(workDir, prompt string) AgentConfig {
	return AgentConfig{
		Agent: AgentCodex, Model: "fake", Tool: "gl-axi", Prompt: prompt,
		WorkDir: workDir, Host: "https://gitlab.example", Token: "fake-gitlab-token",
		RunID: "isolation", TaskID: TaskFindMR, Trial: 1, Project: "group/project",
	}
}

func writableFakeRepository(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.Mkdir(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertNoContainer(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "name=^/"+name+"$")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query container %s: %v: %s", name, err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("container %s leaked: %s", name, output)
	}
}

func benchmarkRepositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal(fmt.Errorf("repository root not found"))
		}
		dir = parent
	}
}
