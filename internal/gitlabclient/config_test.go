package gitlabclient

import (
	"errors"
	"testing"
)

func TestNewConfigPrefersExplicitValues(t *testing.T) {
	t.Setenv(TokenEnv, "env-token")
	t.Setenv(AlternateTokenEnv, "alternate-token")
	t.Setenv(BaseURLEnv, "https://env.gitlab.example")

	cfg := NewConfig("flag-token", "https://flag.gitlab.example")

	if cfg.Token != "flag-token" {
		t.Fatalf("Token = %q, want explicit token", cfg.Token)
	}
	if cfg.BaseURL != "https://flag.gitlab.example" {
		t.Fatalf("BaseURL = %q, want explicit base URL", cfg.BaseURL)
	}
}

func TestNewConfigFallsBackToEnvironmentAndDefaults(t *testing.T) {
	t.Setenv(TokenEnv, "")
	t.Setenv(AlternateTokenEnv, "alternate-token")
	t.Setenv(BaseURLEnv, "")

	cfg := NewConfig("", "")

	if cfg.Token != "alternate-token" {
		t.Fatalf("Token = %q, want alternate env token", cfg.Token)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL = %q, want default base URL", cfg.BaseURL)
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	_, err := Config{BaseURL: DefaultBaseURL}.NewClient()
	if !errors.Is(err, ErrMissingToken) {
		t.Fatalf("NewClient error = %v, want ErrMissingToken", err)
	}
}
