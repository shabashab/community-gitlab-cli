package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunManifest captures harness and runtime identity separately from measured
// trial results. It intentionally contains no environment values.
type RunManifest struct {
	RunID                string                `json:"run_id"`
	Status               string                `json:"status"`
	Error                string                `json:"error,omitempty"`
	Isolation            string                `json:"isolation"`
	Agent                string                `json:"agent"`
	Model                string                `json:"model"`
	Effort               string                `json:"effort"`
	Tool                 string                `json:"tool"`
	GitLabHost           string                `json:"gitlab_host"`
	ProviderAuthSource   string                `json:"provider_auth_source,omitempty"`
	HelperMode           string                `json:"helper_mode"`
	HelperSHA256         string                `json:"helper_sha256,omitempty"`
	PromptTemplateSHA256 string                `json:"prompt_template_sha256"`
	RepositoryCommit     string                `json:"repository_commit"`
	RepositoryDirty      bool                  `json:"repository_dirty"`
	Docker               DockerRuntimeMetadata `json:"docker,omitempty"`
	Image                ImageMetadata         `json:"image,omitempty"`
	Resources            ResourceLimits        `json:"resources,omitempty"`
	ContainerIdentity    *ContainerIdentity    `json:"container_identity,omitempty"`
	StartedAt            time.Time             `json:"started_at"`
	EndedAt              time.Time             `json:"ended_at,omitempty"`
}

func newRunManifest(ctx context.Context, cfg Config, runID string, helper helperMaterial, runner AgentRunner, started time.Time) (RunManifest, error) {
	commit, dirty, err := repositoryState(ctx, cfg.RootDir)
	if err != nil {
		return RunManifest{}, err
	}
	templateSum := sha256.Sum256([]byte(taskPreamble))
	manifest := RunManifest{
		RunID:                runID,
		Status:               "running",
		Isolation:            cfg.Isolation,
		Agent:                cfg.Agent,
		Model:                cfg.Model,
		Effort:               cfg.Effort,
		Tool:                 cfg.Tool,
		GitLabHost:           cfg.Host,
		HelperMode:           cfg.HelperMode,
		HelperSHA256:         helper.SHA256,
		PromptTemplateSHA256: hex.EncodeToString(templateSum[:]),
		RepositoryCommit:     commit,
		RepositoryDirty:      dirty,
		StartedAt:            started,
	}
	if dockerRunner, ok := runner.(*DockerRunner); ok {
		manifest.Docker = dockerRunner.Docker
		manifest.Image = dockerRunner.ImageMetadata
		manifest.Resources = dockerRunner.Resources
		identity := dockerRunner.Identity
		manifest.ContainerIdentity = &identity
	}
	if source, err := providerAuthSource(cfg); err == nil {
		manifest.ProviderAuthSource = strings.TrimPrefix(source, "source=")
	}
	return manifest, nil
}

func repositoryState(ctx context.Context, root string) (string, bool, error) {
	commitCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	commitCmd.Dir = root
	commit, err := commitCmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("read repository commit: %w", err)
	}
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain", "--untracked-files=normal")
	statusCmd.Dir = root
	status, err := statusCmd.Output()
	if err != nil {
		return "", false, fmt.Errorf("read repository status: %w", err)
	}
	return strings.TrimSpace(string(commit)), len(status) != 0, nil
}

func writeManifest(runDir string, manifest RunManifest) error {
	return writeJSONAtomically(filepath.Join(runDir, "manifest.json"), manifest)
}

func writeJSONAtomically(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".manifest-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
