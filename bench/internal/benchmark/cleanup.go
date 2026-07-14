package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type CleanConfig struct {
	Host   string
	Token  string
	Group  string
	MaxAge time.Duration
	DryRun bool
	Hard   bool
	Out    io.Writer
	Docker DockerCLI
}

func Clean(ctx context.Context, cfg CleanConfig) error {
	if cfg.MaxAge < 0 {
		return errors.New("--max-age cannot be negative")
	}
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.Docker == nil {
		cfg.Docker = execDockerCLI{}
	}
	cutoff := time.Now().Add(-cfg.MaxAge)
	if err := cleanContainers(ctx, cfg, cutoff); err != nil {
		return err
	}
	if err := cleanWorkspaces(cfg, cutoff); err != nil {
		return err
	}
	return cleanProjects(ctx, cfg, cutoff)
}

func cleanContainers(ctx context.Context, cfg CleanConfig, cutoff time.Time) error {
	output, err := cfg.Docker.Output(ctx, "ps", "--all", "--quiet", "--filter", "label=community-gitlab-cli.benchmark=true")
	if err != nil {
		return fmt.Errorf("list benchmark containers: %w", err)
	}
	for _, id := range strings.Fields(string(output)) {
		template := `{"id":{{json .Id}},"name":{{json .Name}},"created":{{json .Created}},"status":{{json .State.Status}}}`
		inspect, err := cfg.Docker.Output(ctx, "inspect", "--format", template, id)
		if err != nil {
			return fmt.Errorf("inspect benchmark container %s: %w", id, err)
		}
		var container struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Created string `json:"created"`
			Status  string `json:"status"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(inspect), &container); err != nil {
			return fmt.Errorf("decode benchmark container %s: %w", id, err)
		}
		created, err := time.Parse(time.RFC3339Nano, container.Created)
		if err != nil || created.After(cutoff) {
			continue
		}
		name := strings.TrimPrefix(container.Name, "/")
		if cfg.DryRun {
			fmt.Fprintf(cfg.Out, "would remove container %s status=%s\n", name, container.Status)
			continue
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if err := cfg.Docker.Run(ctx, nil, &stdout, &stderr, "rm", "--force", container.ID); err != nil {
			return fmt.Errorf("remove benchmark container %s: %w: %s", name, err, strings.TrimSpace(stderr.String()))
		}
		fmt.Fprintf(cfg.Out, "removed container %s\n", name)
	}
	return nil
}

func cleanWorkspaces(cfg CleanConfig, cutoff time.Time) error {
	root := filepath.Join(os.TempDir(), "community-gitlab-cli-bench")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list benchmark workspaces: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "trial-") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if cfg.DryRun {
			fmt.Fprintf(cfg.Out, "would remove workspace %s\n", path)
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove benchmark workspace %s: %w", path, err)
		}
		fmt.Fprintf(cfg.Out, "removed workspace %s\n", path)
	}
	return nil
}

func cleanProjects(ctx context.Context, cfg CleanConfig, cutoff time.Time) error {
	if cfg.Host == "" || cfg.Token == "" || cfg.Group == "" {
		return errors.New("GL_BENCH_HOST, GL_BENCH_TOKEN, and GL_BENCH_GROUP must be set")
	}
	client, err := gitlab.NewClient(cfg.Token, gitlab.WithBaseURL(cfg.Host), gitlab.WithUserAgent("community-gitlab-cli-benchmark-janitor"))
	if err != nil {
		return err
	}
	opts := &gitlab.ListGroupProjectsOptions{
		Search:      gitlab.Ptr(fixturePrefix),
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}
	for {
		projects, resp, err := client.Groups.ListGroupProjects(cfg.Group, opts, gitlab.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("list benchmark projects in group %q: %w", cfg.Group, err)
		}
		for _, project := range projects {
			if project == nil || !strings.HasPrefix(project.Path, fixturePrefix) {
				continue
			}
			if project.MarkedForDeletionOn != nil {
				if !cfg.Hard {
					continue
				}
				if cfg.DryRun {
					fmt.Fprintf(cfg.Out, "would permanently remove project %s\n", project.PathWithNamespace)
					continue
				}
				_, err := client.Projects.DeleteProject(project.ID, &gitlab.DeleteProjectOptions{
					FullPath:          gitlab.Ptr(project.PathWithNamespace),
					PermanentlyRemove: gitlab.Ptr(true),
				}, gitlab.WithContext(ctx))
				if err != nil {
					return fmt.Errorf("permanently remove benchmark project %s: %w", project.PathWithNamespace, err)
				}
				fmt.Fprintf(cfg.Out, "permanently removed project %s\n", project.PathWithNamespace)
				continue
			}
			if project.CreatedAt != nil && project.CreatedAt.After(cutoff) {
				continue
			}
			if cfg.DryRun {
				fmt.Fprintf(cfg.Out, "would delete project %s\n", project.PathWithNamespace)
				continue
			}
			if _, err := client.Projects.DeleteProject(project.ID, nil, gitlab.WithContext(ctx)); err != nil {
				return fmt.Errorf("delete benchmark project %s: %w", project.PathWithNamespace, err)
			}
			fmt.Fprintf(cfg.Out, "deleted project %s\n", project.PathWithNamespace)
		}
		if resp.NextPage == 0 {
			return nil
		}
		opts.Page = resp.NextPage
	}
}
