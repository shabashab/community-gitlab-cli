package benchmark

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	fixturePrefix = "gl-bench-"
	fixtureWait   = 30 * time.Second
)

// Fixture is the isolated GitLab and local-repository state for one trial.
type Fixture struct {
	Client       *gitlab.Client
	Project      *gitlab.Project
	MergeRequest *gitlab.MergeRequest
	RepoDir      string
	Marker       string
	MRTitle      string
	SourceBranch string
	TargetBranch string
	ChangedPath  string
	CommentBody  string
}

type FixtureConfig struct {
	Host    string
	Token   string
	Group   string
	WorkDir string
}

func ProvisionFixture(ctx context.Context, cfg FixtureConfig) (*Fixture, error) {
	client, err := gitlab.NewClient(cfg.Token, gitlab.WithBaseURL(cfg.Host), gitlab.WithUserAgent("community-gitlab-cli-benchmark"))
	if err != nil {
		return nil, fmt.Errorf("create benchmark GitLab client: %w", err)
	}

	group, _, err := client.Groups.GetGroup(cfg.Group, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("resolve benchmark group %q: %w", cfg.Group, err)
	}

	marker, err := randomMarker()
	if err != nil {
		return nil, err
	}
	name := fixturePrefix + marker
	project, _, err := client.Projects.CreateProject(&gitlab.CreateProjectOptions{
		Name:                 gitlab.Ptr(name),
		Path:                 gitlab.Ptr(name),
		NamespaceID:          gitlab.Ptr(group.ID),
		Visibility:           gitlab.Ptr(gitlab.PrivateVisibility),
		InitializeWithReadme: gitlab.Ptr(true),
		DefaultBranch:        gitlab.Ptr("main"),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("create benchmark project %q: %w", name, err)
	}

	fixture := &Fixture{
		Client:       client,
		Project:      project,
		Marker:       marker,
		MRTitle:      "Benchmark target " + marker,
		SourceBranch: "bench-feature",
		TargetBranch: "main",
		ChangedPath:  "benchmark.txt",
		CommentBody:  "benchmark-comment-" + marker,
	}

	if err := fixture.finishProvisioning(ctx, cfg.WorkDir); err != nil {
		_ = fixture.Cleanup(context.Background())
		return nil, err
	}
	return fixture, nil
}

func (f *Fixture) finishProvisioning(ctx context.Context, workDir string) error {
	if err := waitFor(ctx, "default branch", func() error {
		_, _, err := f.Client.Branches.GetBranch(f.Project.ID, f.TargetBranch, gitlab.WithContext(ctx))
		return err
	}); err != nil {
		return err
	}

	_, _, err := f.Client.Branches.CreateBranch(f.Project.ID, &gitlab.CreateBranchOptions{
		Branch: gitlab.Ptr(f.SourceBranch),
		Ref:    gitlab.Ptr(f.TargetBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create fixture branch: %w", err)
	}

	_, _, err = f.Client.RepositoryFiles.CreateFile(f.Project.ID, f.ChangedPath, &gitlab.CreateFileOptions{
		Branch:        gitlab.Ptr(f.SourceBranch),
		Content:       gitlab.Ptr("benchmark-marker=" + f.Marker + "\n"),
		CommitMessage: gitlab.Ptr("add benchmark marker"),
		AuthorName:    gitlab.Ptr("gl benchmark"),
		AuthorEmail:   gitlab.Ptr("gl-benchmark@example.invalid"),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create fixture file: %w", err)
	}

	f.MergeRequest, _, err = f.Client.MergeRequests.CreateMergeRequest(f.Project.ID, &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(f.MRTitle),
		Description:  gitlab.Ptr("Disposable LLM benchmark fixture " + f.Marker),
		SourceBranch: gitlab.Ptr(f.SourceBranch),
		TargetBranch: gitlab.Ptr(f.TargetBranch),
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create fixture merge request: %w", err)
	}

	if err := waitFor(ctx, "merge request diff", func() error {
		diffs, _, err := f.Client.MergeRequests.ListMergeRequestDiffs(
			f.Project.ID,
			f.MergeRequest.IID,
			&gitlab.ListMergeRequestDiffsOptions{},
			gitlab.WithContext(ctx),
		)
		if err != nil {
			return err
		}
		if len(diffs) == 0 {
			return errors.New("diff is empty")
		}
		return nil
	}); err != nil {
		return err
	}

	f.RepoDir = filepath.Join(workDir, "repo")
	if err := os.MkdirAll(f.RepoDir, 0o755); err != nil {
		return fmt.Errorf("create fixture repo directory: %w", err)
	}
	for _, args := range [][]string{
		{"init"},
		{"remote", "add", "origin", f.Project.HTTPURLToRepo},
		{"checkout", "-b", f.SourceBranch},
	} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = f.RepoDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func (f *Fixture) GradeExactComment() Grade {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	notes, _, err := f.Client.Notes.ListMergeRequestNotes(
		f.Project.ID,
		f.MergeRequest.IID,
		&gitlab.ListMergeRequestNotesOptions{ListOptions: gitlab.ListOptions{PerPage: 100}},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return Grade{Failures: []string{fmt.Sprintf("list merge request notes: %v", err)}}
	}

	count := 0
	for _, note := range notes {
		if note != nil && note.Body == f.CommentBody {
			count++
		}
	}
	assertion := fmt.Sprintf("merge request has exactly one comment with body %q", f.CommentBody)
	if count == 1 {
		return Grade{Passed: true, Assertions: []string{assertion}}
	}
	return Grade{Failures: []string{fmt.Sprintf("%s (found %d)", assertion, count)}}
}

func (f *Fixture) Cleanup(ctx context.Context) error {
	if f == nil || f.Client == nil || f.Project == nil {
		return nil
	}
	_, err := f.Client.Projects.DeleteProject(f.Project.ID, nil, gitlab.WithContext(ctx))
	if err == nil || isGitLabStatus(err, 404) || isAlreadyMarkedForDeletion(err) {
		return nil
	}
	return fmt.Errorf("delete benchmark project %q: %w", f.Project.PathWithNamespace, err)
}

func waitFor(ctx context.Context, resource string, check func() error) error {
	deadline := time.NewTimer(fixtureWait)
	defer deadline.Stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("%s not ready: %w", resource, lastErr)
		case <-ticker.C:
		}
	}
}

func randomMarker() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate fixture marker: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func isGitLabStatus(err error, status int) bool {
	var responseErr *gitlab.ErrorResponse
	return errors.As(err, &responseErr) && responseErr.StatusCode == status
}

func isAlreadyMarkedForDeletion(err error) bool {
	return isGitLabStatus(err, 400) && strings.Contains(err.Error(), "marked for deletion")
}
