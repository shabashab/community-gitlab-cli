//go:build e2e

package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// Environment contract for the E2E suite. All three are required; running
// with -tags e2e and missing values fails loudly instead of skipping so an
// opted-in run can never fake green.
const (
	envHost     = "GL_E2E_HOST"      // instance base URL, e.g. https://gitlab.example.com
	envToken    = "GL_E2E_TOKEN"     // api-scope PAT of the primary test account
	envGroup    = "GL_E2E_GROUP"     // group path fixture projects are created under
	envTokenAlt = "GL_E2E_TOKEN_ALT" // optional: second account for [second-token] scripts
	envPremium  = "GL_E2E_PREMIUM"   // optional: "1" enables [premium] scripts
)

func params(t *testing.T, dir string) testscript.Params {
	host := requireEnv(t, envHost)
	token := requireEnv(t, envToken)
	group := requireEnv(t, envGroup)

	return testscript.Params{
		Dir:                 filepath.Join("testdata", dir),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Cmds:                scriptCmds(),
		Setup:               setupScriptEnv(host, token, group),
		Condition:           condition,
	}
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()

	value := os.Getenv(key)
	if value == "" {
		t.Fatalf("%s must be set to run the e2e suite (see docs/e2e-testing.md)", key)
	}

	return value
}

// setupScriptEnv isolates each script from the machine it runs on: HOME moves
// into the script workdir (credential file, agent hooks, git global config)
// and the CLI is pointed at the test instance through the normal env vars.
func setupScriptEnv(host, token, group string) func(env *testscript.Env) error {
	return func(env *testscript.Env) error {
		home := filepath.Join(env.WorkDir, "home")
		if err := os.MkdirAll(home, 0o700); err != nil {
			return err
		}

		env.Setenv("HOME", home)
		env.Setenv("GL_CREDSTORE", "file")
		env.Setenv("GITLAB_BASE_URL", host)
		env.Setenv("GITLAB_TOKEN", token)
		env.Setenv(envHost, host)
		env.Setenv(envGroup, group)
		if alt := os.Getenv(envTokenAlt); alt != "" {
			env.Setenv(envTokenAlt, alt)
		}
		env.Setenv("SCRIPT_NAME", scriptName(env.WorkDir))
		env.Setenv("RANDOM_STRING", randomString())

		// Git pushes authenticate through a credential helper so the token
		// never appears in remote URLs, which testscript echoes into logs.
		env.Setenv("GIT_CONFIG_NOSYSTEM", "1")

		return os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitConfig(token)), 0o600)
	}
}

// scriptName derives the script file name from the per-script workdir, which
// testscript names "script-<file base name>".
func scriptName(workDir string) string {
	return strings.TrimPrefix(filepath.Base(workDir), "script-")
}

func randomString() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("generate random string: %v", err))
	}

	return hex.EncodeToString(buf)
}

func gitConfig(token string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(token)

	return fmt.Sprintf(
		"[user]\n"+
			"\tname = gl-e2e\n"+
			"\temail = gl-e2e@example.invalid\n"+
			"[init]\n"+
			"\tdefaultBranch = main\n"+
			"[credential]\n"+
			"\thelper = \"!f() { echo username=oauth2; echo password=%s; }; f\"\n",
		escaped,
	)
}

func condition(cond string) (bool, error) {
	switch cond {
	case "second-token":
		return os.Getenv(envTokenAlt) != "", nil
	case "premium":
		return os.Getenv(envPremium) == "1", nil
	}

	return false, fmt.Errorf("unknown condition %q", cond)
}
