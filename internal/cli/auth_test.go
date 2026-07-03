package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shabashab/community-gitlab-cli/internal/credstore"
	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// setupAuthTest isolates every credential path: the keyring mock is
// process-global, HOME redirects the file backend, and the env vars must not
// leak real credentials into resolution.
func setupAuthTest(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(gitlabclient.TokenEnv, "")
	t.Setenv(gitlabclient.AlternateTokenEnv, "")
	t.Setenv(gitlabclient.BaseURLEnv, "")
}

func newAuthTestCommand(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)

	return cmd
}

func newUserServer(t *testing.T, wantToken string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Errorf("request path = %q, want /api/v4/user", r.URL.Path)
		}
		if got := r.Header.Get("Private-Token"); got != wantToken {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"message":"401 Unauthorized"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":42,"username":"octocat","name":"Mona Lisa","state":"active","web_url":"https://gitlab.example/octocat"}`)
	}))
	t.Cleanup(server.Close)

	return server
}

func TestRunAuthLoginStoresVerifiedToken(t *testing.T) {
	setupAuthTest(t)
	server := newUserServer(t, "glpat-secret")

	var out bytes.Buffer
	err := runAuthLogin(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: server.URL,
		output:        "text",
		mode:          commandModeStandard,
	}, "glpat-secret")
	if err != nil {
		t.Fatalf("runAuthLogin returned error: %v", err)
	}

	for _, fragment := range []string{"username: octocat", "backend: keyring"} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("runAuthLogin output = %q, want fragment %q", out.String(), fragment)
		}
	}

	domain, err := credstore.CanonicalDomain(server.URL)
	if err != nil {
		t.Fatalf("CanonicalDomain error = %v", err)
	}
	token, _, err := credstore.New().Get(domain)
	if err != nil {
		t.Fatalf("stored credential lookup error = %v", err)
	}
	if token != "glpat-secret" {
		t.Fatalf("stored token = %q, want verified token", token)
	}
}

func TestRunAuthLoginRequiresExplicitBaseURL(t *testing.T) {
	setupAuthTest(t)
	t.Setenv(gitlabclient.BaseURLEnv, "https://env.gitlab.example")

	var out bytes.Buffer
	err := runAuthLogin(newAuthTestCommand(&out), &rootOptions{
		output: "text",
		mode:   commandModeStandard,
	}, "glpat-secret")
	if !errors.Is(err, errMissingExplicitBaseURL) {
		t.Fatalf("runAuthLogin error = %v, want errMissingExplicitBaseURL", err)
	}
}

func TestRunAuthLoginRejectsInvalidToken(t *testing.T) {
	setupAuthTest(t)
	server := newUserServer(t, "glpat-valid")

	var out bytes.Buffer
	err := runAuthLogin(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: server.URL,
		output:        "text",
		mode:          commandModeStandard,
	}, "glpat-wrong")
	if !errors.Is(err, errTokenVerification) {
		t.Fatalf("runAuthLogin error = %v, want errTokenVerification", err)
	}

	domain, err := credstore.CanonicalDomain(server.URL)
	if err != nil {
		t.Fatalf("CanonicalDomain error = %v", err)
	}
	if _, _, err := credstore.New().Get(domain); !errors.Is(err, credstore.ErrNotFound) {
		t.Fatalf("credential lookup error = %v, want ErrNotFound after failed login", err)
	}
}

func TestRunAuthLogoutRemovesCredential(t *testing.T) {
	setupAuthTest(t)

	if _, err := credstore.New().Set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("seed credential error = %v", err)
	}

	var out bytes.Buffer
	err := runAuthLogout(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: "https://gitlab.example.com",
		output:        "text",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runAuthLogout returned error: %v", err)
	}
	if !strings.Contains(out.String(), "removed_from: keyring") {
		t.Fatalf("runAuthLogout output = %q, want removed_from: keyring", out.String())
	}

	if _, _, err := credstore.New().Get("gitlab.example.com"); !errors.Is(err, credstore.ErrNotFound) {
		t.Fatalf("credential lookup error = %v, want ErrNotFound after logout", err)
	}
}

func TestRunAuthLogoutWithoutCredentialIsNoop(t *testing.T) {
	setupAuthTest(t)

	var out bytes.Buffer
	err := runAuthLogout(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: "https://gitlab.example.com",
		output:        "text",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runAuthLogout error = %v, want idempotent no-op success", err)
	}
	if !strings.Contains(out.String(), "no stored credential (no-op)") {
		t.Fatalf("runAuthLogout output = %q, want no-op acknowledgment", out.String())
	}
}

func TestRunAuthLogoutWithoutCredentialAxiNoop(t *testing.T) {
	setupAuthTest(t)

	var out bytes.Buffer
	err := runAuthLogout(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: "https://gitlab.example.com",
		output:        "toon",
		mode:          commandModeAxi,
	})
	if err != nil {
		t.Fatalf("runAuthLogout error = %v, want idempotent no-op success", err)
	}
	got := out.String()
	if !strings.Contains(got, "noop: true") {
		t.Fatalf("runAuthLogout axi output = %q, want noop marker", got)
	}
	if !strings.Contains(got, "backends[0]:") {
		t.Fatalf("runAuthLogout axi output = %q, want empty backends array", got)
	}
}

func TestRunAuthStatusShapes(t *testing.T) {
	setupAuthTest(t)

	if _, err := credstore.New().Set("gitlab.example.com", "glpat-secret"); err != nil {
		t.Fatalf("seed credential error = %v", err)
	}

	cases := []struct {
		name      string
		output    string
		mode      commandMode
		fragments []string
	}{
		{
			name:   "text found",
			output: "text",
			mode:   commandModeStandard,
			fragments: []string{
				"domain: gitlab.example.com",
				"authenticated: true",
				"backends: keyring",
			},
		},
		{
			name:   "json found",
			output: "json",
			mode:   commandModeStandard,
			fragments: []string{
				`"domain": "gitlab.example.com"`,
				`"authenticated": true`,
			},
		},
		{
			name:   "toon found",
			output: "toon",
			mode:   commandModeAxi,
			fragments: []string{
				"status:\n  domain: gitlab.example.com",
				"authenticated: true",
				"backends[1]: keyring",
				"help[1]: ",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runAuthStatus(newAuthTestCommand(&out), &rootOptions{
				gitlabBaseURL: "https://gitlab.example.com",
				output:        testCase.output,
				mode:          testCase.mode,
			})
			if err != nil {
				t.Fatalf("runAuthStatus returned error: %v", err)
			}
			for _, fragment := range testCase.fragments {
				if !strings.Contains(out.String(), fragment) {
					t.Fatalf("runAuthStatus output = %q, want fragment %q", out.String(), fragment)
				}
			}
		})
	}
}

func TestRunAuthStatusWithoutCredential(t *testing.T) {
	setupAuthTest(t)

	var out bytes.Buffer
	err := runAuthStatus(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: "https://gitlab.example.com",
		output:        "text",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runAuthStatus returned error: %v", err)
	}
	if !strings.Contains(out.String(), "authenticated: false") {
		t.Fatalf("runAuthStatus output = %q, want authenticated: false", out.String())
	}
}

func TestStoredCredentialUsedByAPICommands(t *testing.T) {
	setupAuthTest(t)
	server := newUserServer(t, "stored-token")

	domain, err := credstore.CanonicalDomain(server.URL)
	if err != nil {
		t.Fatalf("CanonicalDomain error = %v", err)
	}
	if _, err := credstore.New().Set(domain, "stored-token"); err != nil {
		t.Fatalf("seed credential error = %v", err)
	}

	var out bytes.Buffer
	err = runWhoami(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runWhoami with stored credential returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"username": "octocat"`) {
		t.Fatalf("runWhoami output = %q, want octocat", out.String())
	}
}

func TestExplicitTokenBeatsStoredCredential(t *testing.T) {
	setupAuthTest(t)
	server := newUserServer(t, "flag-token")

	domain, err := credstore.CanonicalDomain(server.URL)
	if err != nil {
		t.Fatalf("CanonicalDomain error = %v", err)
	}
	if _, err := credstore.New().Set(domain, "stored-token"); err != nil {
		t.Fatalf("seed credential error = %v", err)
	}

	var out bytes.Buffer
	err = runWhoami(newAuthTestCommand(&out), &rootOptions{
		gitlabToken:   "flag-token",
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runWhoami with flag token returned error: %v", err)
	}
}

func TestEnvTokenBeatsStoredCredential(t *testing.T) {
	setupAuthTest(t)
	server := newUserServer(t, "env-token")
	t.Setenv(gitlabclient.TokenEnv, "env-token")

	domain, err := credstore.CanonicalDomain(server.URL)
	if err != nil {
		t.Fatalf("CanonicalDomain error = %v", err)
	}
	if _, err := credstore.New().Set(domain, "stored-token"); err != nil {
		t.Fatalf("seed credential error = %v", err)
	}

	var out bytes.Buffer
	err = runWhoami(newAuthTestCommand(&out), &rootOptions{
		gitlabBaseURL: server.URL,
		output:        "json",
		mode:          commandModeStandard,
	})
	if err != nil {
		t.Fatalf("runWhoami with env token returned error: %v", err)
	}
}

func TestWriteCommandErrorAuthCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code string
	}{
		{name: "missing base url", err: fmt.Errorf("wrap: %w", errMissingExplicitBaseURL), code: "missing_gitlab_base_url"},
		{name: "invalid token", err: fmt.Errorf("wrap: %w", errTokenVerification), code: "invalid_gitlab_token"},
		{name: "no stored credential", err: fmt.Errorf("wrap: %w", credstore.ErrNotFound), code: "no_stored_credential"},
		{name: "corrupt store", err: fmt.Errorf("wrap: %w", credstore.ErrCorruptCredentials), code: "credential_store_unreadable"},
		{name: "unsupported version", err: fmt.Errorf("wrap: %w", credstore.ErrUnsupportedVersion), code: "credential_store_unreadable"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			writeCommandError(&out, commandModeAxi, "toon", "gl-axi", testCase.err)
			if !strings.Contains(out.String(), "code: "+testCase.code) {
				t.Fatalf("writeCommandError output = %q, want code %q", out.String(), testCase.code)
			}
		})
	}
}
