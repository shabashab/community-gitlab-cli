package benchmark

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestAddCodexTrialToSummary(t *testing.T) {
	summary := newSummary(Config{Agent: AgentCodex, Model: "gpt-5.6-sol"}, "run", "/tmp/run", time.Now())
	addTrialToSummary(&summary, AgentCodex, TrialResult{
		DurationMS: 12_345,
		Usage: Usage{
			InputTokens: 172_116, CachedInputTokens: 147_712,
			OutputTokens: 1_153, ReasoningTokens: 40,
		},
	})

	if summary.Usage.InputTokens != 172_116 || summary.Usage.UncachedInputTokens != 24_404 {
		t.Fatalf("usage = %+v", summary.Usage)
	}
	if summary.AgentDurationMS != 12_345 {
		t.Fatalf("agent duration = %d", summary.AgentDurationMS)
	}
	if summary.CostUSD == nil {
		t.Fatal("cost is unavailable")
	}
	if diff := math.Abs(*summary.CostUSD - 0.230466); diff > 1e-12 {
		t.Fatalf("cost = %.12f", *summary.CostUSD)
	}
	if summary.CostSource != CostSourceEstimatedAPIListPrice || summary.Pricing == nil {
		t.Fatalf("cost metadata = %q, %+v", summary.CostSource, summary.Pricing)
	}
}

func TestAddClaudeTrialToSummary(t *testing.T) {
	summary := newSummary(Config{Agent: AgentClaude, Model: "claude-opus-4-8"}, "run", "/tmp/run", time.Now())
	addTrialToSummary(&summary, AgentClaude, TrialResult{
		DurationMS: 2_000,
		Usage: Usage{
			InputTokens: 100, CachedInputTokens: 80, CacheCreationTokens: 20,
			OutputTokens: 10, CostUSD: 0.12, Turns: 2,
		},
	})

	if summary.Usage.InputTokens != 200 || summary.Usage.UncachedInputTokens != 100 {
		t.Fatalf("usage = %+v", summary.Usage)
	}
	if summary.CostUSD == nil || *summary.CostUSD != 0.12 || summary.CostSource != CostSourceProviderReported {
		t.Fatalf("cost = %v source = %q", summary.CostUSD, summary.CostSource)
	}
}

func TestUnknownCodexPricingIsExplicitlyUnavailable(t *testing.T) {
	summary := newSummary(Config{Agent: AgentCodex, Model: "future-model"}, "run", "/tmp/run", time.Now())
	addTrialToSummary(&summary, AgentCodex, TrialResult{Usage: Usage{InputTokens: 100, OutputTokens: 10}})

	if summary.CostSource != CostSourceUnavailable || summary.Pricing != nil || summary.CostUSD != nil {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestSummaryJSONUsesStableFieldNames(t *testing.T) {
	summary := newSummary(Config{Agent: AgentCodex, Model: "gpt-5.6-luna"}, "run", "/tmp/run", time.Now())
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, field := range []string{`"run_id"`, `"usage"`, `"agent_duration_ms"`, `"wall_duration_ms"`, `"cost_usd"`, `"cost_source"`, `"pricing"`} {
		if !strings.Contains(encoded, field) {
			t.Errorf("summary JSON %s does not contain %s", encoded, field)
		}
	}
}
