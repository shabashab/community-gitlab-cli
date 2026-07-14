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

	firstResult, firstRuntime := runFakeDockerTrial(t, runner, "first require-provider-env")
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

	t.Run("retained container is sanitized", func(t *testing.T) {
		workDir := writableFakeRepository(t)
		keepRunner := *runner
		keepRunner.KeepContainer = true
		result, runtime, err := keepRunner.Run(context.Background(), fakeAgentConfig(workDir, "require-provider-env"))
		if err != nil {
			t.Fatal(err)
		}
		if result.FinalMessage == "" || !runtime.Cleanup.Retained || runtime.Cleanup.ContainerRemoved {
			t.Fatalf("result=%+v runtime=%+v", result, runtime)
		}
		t.Cleanup(func() {
			remove := exec.Command("docker", "rm", "--force", runtime.ContainerName)
			if output, err := remove.CombinedOutput(); err != nil {
				t.Errorf("remove retained container: %v: %s", err, output)
			}
		})

		inspect := exec.Command("docker", "inspect", "--format", "{{json .Config.Env}}", runtime.ContainerName)
		environment, err := inspect.CombinedOutput()
		if err != nil {
			t.Fatalf("inspect retained environment: %v: %s", err, environment)
		}
		for _, secret := range []string{"fake-account-token", "fake-gitlab-token"} {
			if bytes.Contains(environment, []byte(secret)) {
				t.Fatalf("retained environment leaked %q: %s", secret, environment)
			}
		}

		inspectMount := exec.Command("docker", "inspect", "--format", `{{range .Mounts}}{{if eq .Destination "/run/secrets/benchmark.env"}}{{.Source}}{{end}}{{end}}`, runtime.ContainerName)
		secretSource, err := inspectMount.CombinedOutput()
		if err != nil {
			t.Fatalf("inspect retained credential mount: %v: %s", err, secretSource)
		}
		path := strings.TrimSpace(string(secretSource))
		if path == "" {
			t.Fatal("retained container has no credential mount metadata")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("credential source still exists after retained run: %s: %v", path, err)
		}
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
	if err := os.RemoveAll(workDir); err != nil {
		t.Fatalf("host could not remove container-created workspace content: %v", err)
	}
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists after host removal: %v", err)
	}
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
	if err := os.Mkdir(dir, 0o755); err != nil {
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
