package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/shabashab/community-gitlab-cli/internal/gitlabclient"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestWriteUserSupportsText(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "text", commandModeStandard, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	want := "id: 42\nusername: octocat\nname: Mona Lisa\nstate: active\nweb_url: https://gitlab.example/octocat\n"
	if out.String() != want {
		t.Fatalf("writeUser output = %q, want %q", out.String(), want)
	}
}

func TestRootCommandUsesModeOutputDefaults(t *testing.T) {
	standard := newRootCommand("gl", "short", "long", commandModeStandard)
	standardOutput := standard.PersistentFlags().Lookup("output")
	if standardOutput.DefValue != "text" {
		t.Fatalf("standard output default = %q, want text", standardOutput.DefValue)
	}

	axi := newRootCommand("gl-axi", "short", "long", commandModeAxi)
	axiOutput := axi.PersistentFlags().Lookup("output")
	if axiOutput.DefValue != "toon" {
		t.Fatalf("axi output default = %q, want toon", axiOutput.DefValue)
	}
}

func TestWriteUserSupportsJSON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "json", commandModeStandard, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		`"id": 42`,
		`"username": "octocat"`,
		`"web_url": "https://gitlab.example/octocat"`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteUserRejectsUnknownOutputFormat(t *testing.T) {
	err := writeUser(&bytes.Buffer{}, "yaml", commandModeStandard, &gitlab.User{})
	if err == nil {
		t.Fatal("writeUser returned nil error, want unsupported format error")
	}
}

func TestWriteUserRejectsNilUser(t *testing.T) {
	err := writeUser(&bytes.Buffer{}, "text", commandModeStandard, nil)
	if err == nil {
		t.Fatal("writeUser returned nil error, want nil user error")
	}
}

func TestWriteUserSupportsAxiTOON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "toon", commandModeAxi, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		"user{id,username,name,web_url}:",
		`42,octocat,"Mona Lisa","https://gitlab.example/octocat"`,
		`next: "Use project list when available to inspect accessible projects."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteUserSupportsAxiJSON(t *testing.T) {
	var out bytes.Buffer
	user := &gitlab.User{
		ID:       42,
		Username: "octocat",
		Name:     "Mona Lisa",
		State:    "active",
		WebURL:   "https://gitlab.example/octocat",
	}

	if err := writeUser(&out, "json", commandModeAxi, user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	for _, fragment := range []string{
		`"user": {`,
		`"username": "octocat"`,
		`"next": "Use project list when available to inspect accessible projects."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeUser AXI JSON = %q, want fragment %q", out.String(), fragment)
		}
	}
}

func TestWriteCommandErrorSupportsAxi(t *testing.T) {
	var out bytes.Buffer

	writeCommandError(&out, commandModeAxi, fmt.Errorf("%w: set GITLAB_TOKEN", gitlabclient.ErrMissingToken))

	for _, fragment := range []string{
		"error{code,message}:",
		"missing_gitlab_token",
		`next: "Set GITLAB_TOKEN or pass --gitlab-token, then retry."`,
	} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("writeCommandError AXI output = %q, want fragment %q", out.String(), fragment)
		}
	}
}
