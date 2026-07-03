package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/credstore"
	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/shabashab/community-gitlab-cli/internal/repo"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errMissingExplicitBaseURL = errors.New("missing explicit --gitlab-base-url")
	errTokenVerification      = errors.New("gitlab token verification failed")
)

func newAuthCommand(rootOpts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage stored GitLab credentials",
	}

	cmd.AddCommand(newAuthLoginCommand(rootOpts))
	cmd.AddCommand(newAuthLogoutCommand(rootOpts))
	cmd.AddCommand(newAuthStatusCommand(rootOpts))

	return cmd
}

func newAuthLoginCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "login <token>",
		Short: "Store a GitLab personal access token for a host",
		Long: `Verify a personal access token against a GitLab host and store it durably.

The credential lands in the OS keychain when one is available, otherwise in an
encrypted file under ~/.gl keyed by the host, so later commands can
authenticate without GITLAB_TOKEN.

--gitlab-base-url is required and must be passed explicitly; the
GITLAB_BASE_URL environment variable is not accepted for login. Passing the
token as an argument may leave it in shell history — prefer reading it from a
password manager, for example: gl auth login "$(pass show gitlab/token)" ...`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthLogin(cmd, rootOpts, args[0])
		},
	}
}

func newAuthLogoutCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored credential for a GitLab host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthLogout(cmd, rootOpts)
		},
	}
}

func newAuthStatusCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show stored credential state for a GitLab host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthStatus(cmd, rootOpts)
		},
	}
}

func runAuthLogin(cmd *cobra.Command, rootOpts *rootOptions, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("%w: empty token", errTokenVerification)
	}

	baseURL := strings.TrimSpace(rootOpts.gitlabBaseURL)
	if baseURL == "" {
		return fmt.Errorf(
			"%w: auth login stores a credential per GitLab host and needs the host stated explicitly",
			errMissingExplicitBaseURL,
		)
	}

	domain, err := credstore.CanonicalDomain(baseURL)
	if err != nil {
		return err
	}

	client, err := gitlabclient.Config{Token: token, BaseURL: baseURL}.NewClient()
	if err != nil {
		return err
	}

	user, _, err := client.Users.CurrentUser(gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("%w for %s: %v", errTokenVerification, domain, err)
	}

	backend, err := credstore.New().Set(domain, token)
	if err != nil {
		return err
	}

	return writeAuthLogin(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, authLoginResult{
		Username: user.Username,
		Domain:   domain,
		Backend:  string(backend),
	})
}

func runAuthLogout(cmd *cobra.Command, rootOpts *rootOptions) error {
	domain, err := resolveAuthDomain(cmd, rootOpts)
	if err != nil {
		return err
	}

	backends, err := credstore.New().Delete(domain)
	if err != nil {
		return fmt.Errorf("remove credential for %s: %w", domain, err)
	}

	return writeAuthLogout(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, authLogoutResult{
		Domain:   domain,
		Backends: backendNames(backends),
	})
}

func runAuthStatus(cmd *cobra.Command, rootOpts *rootOptions) error {
	domain, err := resolveAuthDomain(cmd, rootOpts)
	if err != nil {
		return err
	}

	status := credstore.New().Status(domain)

	return writeAuthStatus(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, authStatusResult{
		Domain:        domain,
		Authenticated: len(status.Backends) > 0,
		Backends:      backendNames(status.Backends),
		Warnings:      status.Warnings,
	})
}

// resolveAuthDomain mirrors the base URL resolution used by API commands so
// auth status answers "would a command run from here find a credential?".
func resolveAuthDomain(cmd *cobra.Command, rootOpts *rootOptions) (string, error) {
	baseURL := strings.TrimSpace(rootOpts.gitlabBaseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv(gitlabclient.BaseURLEnv))
	}
	if baseURL == "" {
		if origin, err := repo.DiscoverOrigin(commandContext(cmd), ""); err == nil {
			baseURL = origin.BaseURL
		}
	}
	if baseURL == "" {
		baseURL = gitlabclient.DefaultBaseURL
	}

	return credstore.CanonicalDomain(baseURL)
}

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}

	return context.Background()
}

func backendNames(backends []credstore.Backend) []string {
	names := make([]string, 0, len(backends))
	for _, backend := range backends {
		names = append(names, string(backend))
	}

	return names
}
