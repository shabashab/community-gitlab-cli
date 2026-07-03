package cli

import (
	"bytes"
	"strings"
	"testing"

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

	if err := writeUser(&out, "text", user); err != nil {
		t.Fatalf("writeUser returned error: %v", err)
	}

	want := "id: 42\nusername: octocat\nname: Mona Lisa\nstate: active\nweb_url: https://gitlab.example/octocat\n"
	if out.String() != want {
		t.Fatalf("writeUser output = %q, want %q", out.String(), want)
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

	if err := writeUser(&out, "json", user); err != nil {
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
	err := writeUser(&bytes.Buffer{}, "yaml", &gitlab.User{})
	if err == nil {
		t.Fatal("writeUser returned nil error, want unsupported format error")
	}
}

func TestWriteUserRejectsNilUser(t *testing.T) {
	err := writeUser(&bytes.Buffer{}, "text", nil)
	if err == nil {
		t.Fatal("writeUser returned nil error, want nil user error")
	}
}
