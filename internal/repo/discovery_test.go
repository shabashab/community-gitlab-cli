package repo

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

func TestParseOriginURL(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		baseURL   string
		project   string
	}{
		{
			name:      "https",
			remoteURL: "https://gitlab.example/group/project.git",
			baseURL:   "https://gitlab.example",
			project:   "group/project",
		},
		{
			name:      "http with port and nested namespace",
			remoteURL: "http://gitlab.example:8080/group/subgroup/project.git",
			baseURL:   "http://gitlab.example:8080",
			project:   "group/subgroup/project",
		},
		{
			name:      "scp style ssh",
			remoteURL: "git@gitlab.example:group/project.git",
			baseURL:   "https://gitlab.example",
			project:   "group/project",
		},
		{
			name:      "ssh url",
			remoteURL: "ssh://git@gitlab.example:2222/group/project.git",
			baseURL:   "https://gitlab.example",
			project:   "group/project",
		},
		{
			name:      "without trailing git suffix",
			remoteURL: "https://gitlab.example/group/project",
			baseURL:   "https://gitlab.example",
			project:   "group/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOriginURL(tt.remoteURL)
			if err != nil {
				t.Fatalf("ParseOriginURL returned error: %v", err)
			}
			if got.BaseURL != tt.baseURL {
				t.Fatalf("BaseURL = %q, want %q", got.BaseURL, tt.baseURL)
			}
			if got.ProjectPath != tt.project {
				t.Fatalf("ProjectPath = %q, want %q", got.ProjectPath, tt.project)
			}
		})
	}
}

func TestParseOriginURLRejectsInvalidRemotes(t *testing.T) {
	for _, remoteURL := range []string{
		"",
		"ftp://gitlab.example/group/project.git",
		"https://gitlab.example/project.git",
		"not-a-remote",
	} {
		_, err := ParseOriginURL(remoteURL)
		if !errors.Is(err, ErrInvalidOrigin) {
			t.Fatalf("ParseOriginURL(%q) error = %v, want ErrInvalidOrigin", remoteURL, err)
		}
	}
}

func TestDiscoverOriginReadsOnlyOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is not available")
	}

	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "remote", "add", "origin", "git@gitlab.example:group/project.git")
	runGit(t, dir, "remote", "add", "upstream", "git@gitlab.example:other/repo.git")

	got, err := DiscoverOrigin(context.Background(), dir)
	if err != nil {
		t.Fatalf("DiscoverOrigin returned error: %v", err)
	}
	if got.BaseURL != "https://gitlab.example" {
		t.Fatalf("BaseURL = %q, want https://gitlab.example", got.BaseURL)
	}
	if got.ProjectPath != "group/project" {
		t.Fatalf("ProjectPath = %q, want group/project", got.ProjectPath)
	}
}

func TestDiscoverOriginRequiresOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is not available")
	}

	dir := t.TempDir()
	runGit(t, dir, "init")

	_, err := DiscoverOrigin(context.Background(), dir)
	if !errors.Is(err, ErrMissingOrigin) {
		t.Fatalf("DiscoverOrigin error = %v, want ErrMissingOrigin", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
