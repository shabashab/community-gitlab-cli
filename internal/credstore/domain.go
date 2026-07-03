package credstore

import (
	"fmt"
	"net/url"
	"strings"
)

// CanonicalDomain reduces a GitLab base URL to the canonical domain used as
// the credential key: lowercased host, keeping the port only when it is not
// the scheme default. Login and lookup must both key through this function so
// the two can never disagree.
func CanonicalDomain(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty base URL", ErrInvalidDomain)
	}

	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidDomain, err)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: no host in %q", ErrInvalidDomain, baseURL)
	}

	port := parsed.Port()
	if port == "" || isDefaultPort(parsed.Scheme, port) {
		return host, nil
	}

	return host + ":" + port, nil
}

func isDefaultPort(scheme, port string) bool {
	switch strings.ToLower(scheme) {
	case "https":
		return port == "443"
	case "http":
		return port == "80"
	default:
		return false
	}
}
