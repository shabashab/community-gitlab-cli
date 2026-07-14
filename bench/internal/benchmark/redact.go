package benchmark

import (
	"bytes"
	"encoding/json"
	"errors"
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
	result.RawEvents = redactBytes(result.RawEvents, secrets)
	result.RawStderr = redactBytes(result.RawStderr, secrets)
	result.FinalMessage = redactString(result.FinalMessage, secrets)
	result.PolicyViolation = redactString(result.PolicyViolation, secrets)
	redactStrings(result.Commands, secrets)
}

func redactTrialResult(result *TrialResult, secrets []string) {
	result.FinalMessage = redactString(result.FinalMessage, secrets)
	result.PolicyViolation = redactString(result.PolicyViolation, secrets)
	result.Error = redactString(result.Error, secrets)
	result.Runtime.Cleanup.Error = redactString(result.Runtime.Cleanup.Error, secrets)
	redactStrings(result.Commands, secrets)
	redactStrings(result.Grade.Assertions, secrets)
	redactStrings(result.Grade.Failures, secrets)
}

func redactError(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	message := redactString(err.Error(), secrets)
	if message == err.Error() {
		return err
	}
	if isInfrastructureError(err) {
		return newInfrastructureError("%s", message)
	}
	return errors.New(message)
}

func redactBytes(value []byte, secrets []string) []byte {
	for _, secret := range secrets {
		value = bytes.ReplaceAll(value, []byte(secret), []byte(redactedValue))
	}
	return value
}

func redactString(value string, secrets []string) string {
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret, redactedValue)
	}
	return value
}

func redactStrings(values []string, secrets []string) {
	for i := range values {
		values[i] = redactString(values[i], secrets)
	}
}
