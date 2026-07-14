package benchmark

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
)

const redactedValue = "[REDACTED]"

func benchmarkSecrets(cfg Config) []string {
	values := []string{
		cfg.Token,
		os.Getenv("CODEX_ACCESS_TOKEN"),
		os.Getenv("CODEX_API_KEY"),
		os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"),
		os.Getenv("ANTHROPIC_API_KEY"),
	}
	if cfg.Agent == AgentCodex && strings.TrimSpace(cfg.CodexAuthFile) != "" {
		if data, err := os.ReadFile(cfg.CodexAuthFile); err == nil {
			values = append(values, string(data))
			var document any
			if json.Unmarshal(data, &document) == nil {
				collectSensitiveJSONValues(document, false, &values)
			}
		}
	}

	seen := make(map[string]struct{}, len(values))
	secrets := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		secrets = append(secrets, value)
	}
	// Replace longer values first in case one credential contains another.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func collectSensitiveJSONValues(value any, sensitive bool, values *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			key = strings.ToLower(key)
			childSensitive := sensitive || strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "api_key")
			collectSensitiveJSONValues(child, childSensitive, values)
		}
	case []any:
		for _, child := range typed {
			collectSensitiveJSONValues(child, sensitive, values)
		}
	case string:
		if sensitive && typed != "" {
			*values = append(*values, typed)
		}
	}
}

func redactAgentResult(result *AgentResult, secrets []string) {
	for _, secret := range secrets {
		result.RawEvents = bytes.ReplaceAll(result.RawEvents, []byte(secret), []byte(redactedValue))
		result.RawStderr = bytes.ReplaceAll(result.RawStderr, []byte(secret), []byte(redactedValue))
		result.FinalMessage = strings.ReplaceAll(result.FinalMessage, secret, redactedValue)
		for i := range result.Commands {
			result.Commands[i] = strings.ReplaceAll(result.Commands[i], secret, redactedValue)
		}
	}
}
