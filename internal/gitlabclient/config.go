package gitlabclient

import (
	"errors"
	"fmt"
	"os"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

const (
	DefaultBaseURL   = "https://gitlab.com"
	DefaultUserAgent = "community-gitlab-cli"

	TokenEnv          = "GITLAB_TOKEN"
	AlternateTokenEnv = "GL_TOKEN"
	BaseURLEnv        = "GITLAB_BASE_URL"
)

var ErrMissingToken = errors.New("missing GitLab token")

// Config contains the GitLab API connection settings used by CLI commands.
type Config struct {
	Token     string
	BaseURL   string
	UserAgent string
}

// NewConfig builds a client config from explicit CLI values, then environment
// variables, then project defaults.
func NewConfig(token, baseURL string) Config {
	return NewConfigWithBaseURLFallback(token, baseURL, DefaultBaseURL)
}

// NewConfigWithBaseURLFallback builds a client config using fallbackBaseURL
// after explicit CLI values and environment variables.
func NewConfigWithBaseURLFallback(token, baseURL, fallbackBaseURL string) Config {
	return Config{
		Token:     firstNonEmpty(token, os.Getenv(TokenEnv), os.Getenv(AlternateTokenEnv)),
		BaseURL:   firstNonEmpty(baseURL, os.Getenv(BaseURLEnv), fallbackBaseURL, DefaultBaseURL),
		UserAgent: DefaultUserAgent,
	}
}

// NewClient creates the official GitLab API client used for all GitLab
// communication in this CLI.
func (c Config) NewClient() (*gitlab.Client, error) {
	c = c.withDefaults()
	if c.Token == "" {
		return nil, fmt.Errorf("%w: set %s, pass --gitlab-token, or run auth login", ErrMissingToken, TokenEnv)
	}

	client, err := gitlab.NewClient(
		c.Token,
		gitlab.WithBaseURL(c.BaseURL),
		gitlab.WithUserAgent(c.UserAgent),
	)
	if err != nil {
		return nil, fmt.Errorf("create GitLab client: %w", err)
	}

	return client, nil
}

func (c Config) withDefaults() Config {
	c.Token = strings.TrimSpace(c.Token)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.UserAgent = strings.TrimSpace(c.UserAgent)

	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}

	return c
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}
