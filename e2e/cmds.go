//go:build e2e

package e2e

import (
	"errors"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

func scriptCmds() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"exitcode":   cmdExitCode,
		"stdout2env": cmdStdout2Env,
		"defer":      cmdDefer,
		"retry":      cmdRetry,
		"mkproject":  cmdMkProject,
		"rmproject":  cmdRmProject,
	}
}

// exitcode runs a program and asserts its exact exit code, because the CLI
// contract distinguishes exit 1 (runtime) from exit 2 (usage) while the
// built-in exec only knows pass/fail. Stdout and stderr stay available to
// subsequent stdout/stderr assertions.
//
//	exitcode 2 gl-axi mr list --bogus-flag
func cmdExitCode(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! exitcode (assert the code you expect instead)")
	}
	if len(args) < 2 {
		ts.Fatalf("usage: exitcode <want> <prog> [args...]")
	}

	want, err := strconv.Atoi(args[0])
	ts.Check(err)

	got := 0
	if err := ts.Exec(args[1], args[2:]...); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			ts.Fatalf("exec %s: %v", args[1], err)
		}
		got = exitErr.ExitCode()
	}

	if got != want {
		ts.Fatalf("exit code = %d, want %d", got, want)
	}
}

// stdout2env extracts a value from the previous command's stdout into an
// environment variable. With a capture group the group is stored, otherwise
// the whole match.
//
//	stdout2env IID 'iid: (\d+)'
func cmdStdout2Env(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: stdout2env VAR PATTERN")
	}

	pattern, err := regexp.Compile(args[1])
	ts.Check(err)

	match := pattern.FindStringSubmatch(ts.ReadFile("stdout"))
	if match == nil {
		ts.Fatalf("stdout2env: pattern %q not found in stdout", args[1])
	}

	value := match[0]
	if len(match) > 1 {
		value = match[1]
	}
	ts.Setenv(args[0], value)
}

const (
	retryAttempts = 10
	retryDelay    = time.Second
)

// retry runs a command, retrying on failure until it succeeds or the attempt
// budget runs out. For eventually-consistent GitLab state right after a
// mutation: diff refs still computing after mr create, a just-pushed branch
// not yet visible to the merge request API. The last attempt's stdout and
// stderr stay available to subsequent assertions.
//
//	retry gl-axi mr diff $IID
func cmdRetry(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: retry <prog> [args...]")
	}

	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if err = ts.Exec(args[0], args[1:]...); err == nil {
			return
		}
		time.Sleep(retryDelay)
	}

	ts.Fatalf("retry %s: still failing after %d attempts: %v", args[0], retryAttempts, err)
}

// defer registers a command to run when the script finishes, pass or fail.
// Cleanup failures are logged, not fatal: the janitor sweeps anything left.
//
//	defer rmproject $SCRIPT_NAME-$RANDOM_STRING
func cmdDefer(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: defer <prog> [args...]")
	}

	prog := args[0]
	rest := slices.Clone(args[1:])
	ts.Defer(func() {
		if err := ts.Exec(prog, rest...); err != nil {
			ts.Logf("defer %s: %v", prog, err)
		}
	})
}

// mkproject creates an ephemeral fixture project named gl-e2e-<suffix> under
// $GL_E2E_GROUP and exports $PROJECT (full path) and $PROJECT_URL. Deletion
// is auto-deferred so even scripts that forget `defer rmproject` self-clean.
//
//	mkproject $SCRIPT_NAME-$RANDOM_STRING
func cmdMkProject(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: mkproject <name-suffix>")
	}

	project, err := createProject(projectNamePrefix + args[0])
	ts.Check(err)

	path := project.PathWithNamespace
	ts.Defer(func() {
		if err := deleteProject(path); err != nil {
			ts.Logf("cleanup %s: %v", path, err)
		}
	})

	ts.Setenv("PROJECT", path)
	ts.Setenv("PROJECT_URL", project.WebURL)
}

// rmproject deletes a fixture project created by mkproject. Deleting a
// project that is already gone is fine, so defer'd cleanup stays quiet when
// the script already removed it.
//
//	rmproject $SCRIPT_NAME-$RANDOM_STRING
func cmdRmProject(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: rmproject <name-suffix>")
	}

	ts.Check(deleteProjectBySuffix(args[0]))
}
