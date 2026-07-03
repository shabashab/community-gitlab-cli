package credstore

import (
	"errors"
	"testing"
)

func TestCanonicalDomain(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "lowercases host", baseURL: "https://GitLab.Example.com/", want: "gitlab.example.com"},
		{name: "strips default https port", baseURL: "https://gitlab.example.com:443", want: "gitlab.example.com"},
		{name: "strips default http port", baseURL: "http://gitlab.example.com:80", want: "gitlab.example.com"},
		{name: "keeps non-standard port", baseURL: "https://gitlab.example.com:8443", want: "gitlab.example.com:8443"},
		{name: "keeps loopback port", baseURL: "http://127.0.0.1:39000", want: "127.0.0.1:39000"},
		{name: "bare host without scheme", baseURL: "gitlab.com", want: "gitlab.com"},
		{name: "ignores path", baseURL: "https://gitlab.example.com/api/v4", want: "gitlab.example.com"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := CanonicalDomain(testCase.baseURL)
			if err != nil {
				t.Fatalf("CanonicalDomain(%q) error = %v", testCase.baseURL, err)
			}
			if got != testCase.want {
				t.Fatalf("CanonicalDomain(%q) = %q, want %q", testCase.baseURL, got, testCase.want)
			}
		})
	}
}

func TestCanonicalDomainRejectsInvalidInput(t *testing.T) {
	for _, baseURL := range []string{"", "   ", "https://"} {
		if _, err := CanonicalDomain(baseURL); !errors.Is(err, ErrInvalidDomain) {
			t.Fatalf("CanonicalDomain(%q) error = %v, want ErrInvalidDomain", baseURL, err)
		}
	}
}
