package repo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

var (
	ErrMissingOrigin   = errors.New("missing git remote origin url")
	ErrInvalidOrigin   = errors.New("invalid git remote origin url")
	ErrNoCurrentBranch = errors.New("no current git branch")
)

// Origin describes project coordinates inferred from a local git origin remote.
type Origin struct {
	RemoteURL   string
	BaseURL     string
	ProjectPath string
}

// DiscoverOrigin reads only remote.origin.url from the git repository at workDir.
func DiscoverOrigin(ctx context.Context, workDir string) (Origin, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"config", "--get", "remote.origin.url"}
	if strings.TrimSpace(workDir) != "" {
		args = append([]string{"-C", workDir}, args...)
	}

	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return Origin{}, fmt.Errorf("%w: run git config --get remote.origin.url", ErrMissingOrigin)
	}

	origin, err := ParseOriginURL(string(output))
	if err != nil {
		return Origin{}, err
	}

	return origin, nil
}

// CurrentBranch returns the branch checked out in the git repository at
// workDir. A detached HEAD has no branch and fails with ErrNoCurrentBranch.
func CurrentBranch(ctx context.Context, workDir string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"rev-parse", "--abbrev-ref", "HEAD"}
	if strings.TrimSpace(workDir) != "" {
		args = append([]string{"-C", workDir}, args...)
	}

	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("%w: run git rev-parse --abbrev-ref HEAD", ErrNoCurrentBranch)
	}

	branch := strings.TrimSpace(string(output))
	if branch == "" || branch == "HEAD" {
		return "", fmt.Errorf("%w: detached HEAD", ErrNoCurrentBranch)
	}

	return branch, nil
}

// ParseOriginURL extracts a project path and host base URL from a git remote URL.
func ParseOriginURL(remoteURL string) (Origin, error) {
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return Origin{}, fmt.Errorf("%w: empty remote url", ErrInvalidOrigin)
	}

	if origin, ok, err := parseURLRemote(remoteURL); ok || err != nil {
		return origin, err
	}

	if origin, ok, err := parseSCPRemote(remoteURL); ok || err != nil {
		return origin, err
	}

	return Origin{}, fmt.Errorf("%w: unsupported remote url %q", ErrInvalidOrigin, remoteURL)
}

func parseURLRemote(remoteURL string) (Origin, bool, error) {
	u, err := url.Parse(remoteURL)
	if err != nil || u.Scheme == "" {
		return Origin{}, false, nil
	}
	if u.Hostname() == "" {
		return Origin{}, true, fmt.Errorf("%w: missing remote host in %q", ErrInvalidOrigin, remoteURL)
	}

	project, err := cleanProjectPath(u.Path)
	if err != nil {
		return Origin{}, true, err
	}

	var baseURL string
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		baseURL = u.Scheme + "://" + u.Host
	case "ssh", "git", "git+ssh":
		baseURL = "https://" + u.Hostname()
	default:
		return Origin{}, true, fmt.Errorf("%w: unsupported remote scheme %q", ErrInvalidOrigin, u.Scheme)
	}

	return Origin{
		RemoteURL:   remoteURL,
		BaseURL:     baseURL,
		ProjectPath: project,
	}, true, nil
}

func parseSCPRemote(remoteURL string) (Origin, bool, error) {
	if strings.Contains(remoteURL, "://") {
		return Origin{}, false, nil
	}

	colon := strings.Index(remoteURL, ":")
	if colon <= 0 {
		return Origin{}, false, nil
	}

	host := remoteURL[:colon]
	projectPath := remoteURL[colon+1:]
	if strings.Contains(host, "/") || strings.Contains(host, "\\") {
		return Origin{}, false, nil
	}
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return Origin{}, true, fmt.Errorf("%w: missing remote host in %q", ErrInvalidOrigin, remoteURL)
	}

	project, err := cleanProjectPath(projectPath)
	if err != nil {
		return Origin{}, true, err
	}

	return Origin{
		RemoteURL:   remoteURL,
		BaseURL:     "https://" + host,
		ProjectPath: project,
	}, true, nil
}

func cleanProjectPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")
	if path == "" {
		return "", fmt.Errorf("%w: missing project path", ErrInvalidOrigin)
	}
	if !strings.Contains(path, "/") {
		return "", fmt.Errorf("%w: project path %q must include a namespace", ErrInvalidOrigin, path)
	}

	return path, nil
}
