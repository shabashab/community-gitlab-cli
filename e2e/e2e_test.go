//go:build e2e

// Package e2e runs testscript-based end-to-end tests against a live GitLab
// instance. Each .txtar script under testdata/ executes the real gl and
// gl-axi commands in-process and asserts on their output and exit codes.
// See docs/e2e-testing.md for provisioning and usage.
package e2e

import (
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/shabashab/community-gitlab-cli/internal/cli"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"gl":     cli.Execute,
		"gl-axi": cli.ExecuteAxi,
	})
}

func TestWhoami(t *testing.T) { testscript.Run(t, params(t, "whoami")) }
func TestAxi(t *testing.T)    { testscript.Run(t, params(t, "axi")) }
