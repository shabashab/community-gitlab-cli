package benchmark

import "time"

const (
	CostSourceProviderReported      = "provider_reported"
	CostSourceEstimatedAPIListPrice = "estimated_api_list_price"
	CostSourceUnavailable           = "unavailable"
)

// SummaryUsage is normalized across agent event formats. Claude reports
// uncached, cache-read, and cache-creation input separately, while Codex's
// input count includes cached input.
type SummaryUsage struct {
	InputTokens         int64 `json:"input_tokens"`
	UncachedInputTokens int64 `json:"uncached_input_tokens"`
	CachedInputTokens   int64 `json:"cached_input_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens"`
	Turns               int64 `json:"turns"`
}

// Pricing records the static API list prices used for a Codex estimate so a
// result remains auditable if provider prices change later.
type Pricing struct {
	Model                  string  `json:"model"`
	Currency               string  `json:"currency"`
	AsOf                   string  `json:"as_of"`
	InputPerMillionUSD     float64 `json:"input_per_million_usd"`
	CachedPerMillionUSD    float64 `json:"cached_input_per_million_usd"`
	OutputPerMillionUSD    float64 `json:"output_per_million_usd"`
	SourceURL              string  `json:"source_url"`
	LongContextNotIncluded bool    `json:"long_context_surcharge_not_included"`
}

var codexPricing = map[string]Pricing{
	"gpt-5.6": {
		Model: "gpt-5.6-sol", Currency: "USD", AsOf: "2026-07-12",
		InputPerMillionUSD: 5, CachedPerMillionUSD: 0.5, OutputPerMillionUSD: 30,
		SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
	},
	"gpt-5.6-sol": {
		Model: "gpt-5.6-sol", Currency: "USD", AsOf: "2026-07-12",
		InputPerMillionUSD: 5, CachedPerMillionUSD: 0.5, OutputPerMillionUSD: 30,
		SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-sol",
	},
	"gpt-5.6-terra": {
		Model: "gpt-5.6-terra", Currency: "USD", AsOf: "2026-07-12",
		InputPerMillionUSD: 2.5, CachedPerMillionUSD: 0.25, OutputPerMillionUSD: 15,
		SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-terra",
	},
	"gpt-5.6-luna": {
		Model: "gpt-5.6-luna", Currency: "USD", AsOf: "2026-07-12",
		InputPerMillionUSD: 1, CachedPerMillionUSD: 0.1, OutputPerMillionUSD: 6,
		SourceURL: "https://developers.openai.com/api/docs/models/gpt-5.6-luna",
	},
}

func newSummary(cfg Config, runID, runDir string, startedAt time.Time) Summary {
	summary := Summary{
		RunID: runID, RunDir: runDir, StartedAt: startedAt,
		CostSource: CostSourceUnavailable,
	}
	switch cfg.Agent {
	case AgentClaude:
		summary.CostSource = CostSourceProviderReported
		summary.CostUSD = new(float64)
	case AgentCodex:
		if pricing, ok := codexPricing[cfg.Model]; ok {
			pricing.LongContextNotIncluded = true
			summary.CostSource = CostSourceEstimatedAPIListPrice
			summary.Pricing = &pricing
			summary.CostUSD = new(float64)
		}
	}
	return summary
}

func addTrialToSummary(summary *Summary, agent string, trial TrialResult) {
	usage := trial.Usage
	summary.AgentDurationMS += trial.DurationMS
	summary.Usage.CachedInputTokens += usage.CachedInputTokens
	summary.Usage.CacheCreationTokens += usage.CacheCreationTokens
	summary.Usage.OutputTokens += usage.OutputTokens
	summary.Usage.ReasoningTokens += usage.ReasoningTokens
	summary.Usage.Turns += usage.Turns

	switch agent {
	case AgentClaude:
		// Claude's input_tokens excludes its cache-read and cache-creation
		// counters, so add the three categories for a comparable total.
		summary.Usage.InputTokens += usage.InputTokens + usage.CachedInputTokens + usage.CacheCreationTokens
		summary.Usage.UncachedInputTokens += usage.InputTokens
		*summary.CostUSD += usage.CostUSD
	case AgentCodex:
		// Codex's input_tokens already includes cached_input_tokens.
		summary.Usage.InputTokens += usage.InputTokens
		uncached := usage.InputTokens - usage.CachedInputTokens
		if uncached < 0 {
			uncached = 0
		}
		summary.Usage.UncachedInputTokens += uncached
		if summary.Pricing != nil {
			*summary.CostUSD += estimateCodexCost(usage, *summary.Pricing)
		}
	}
}

func estimateCodexCost(usage Usage, pricing Pricing) float64 {
	uncached := usage.InputTokens - usage.CachedInputTokens
	if uncached < 0 {
		uncached = 0
	}
	return (float64(uncached)*pricing.InputPerMillionUSD +
		float64(usage.CachedInputTokens)*pricing.CachedPerMillionUSD +
		float64(usage.OutputTokens)*pricing.OutputPerMillionUSD) / 1_000_000
}
