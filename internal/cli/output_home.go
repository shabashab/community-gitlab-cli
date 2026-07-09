package cli

// axiHomeRepoOutput is the gl-axi no-args dashboard inside a GitLab repo.
type axiHomeRepoOutput struct {
	Bin           string               `json:"bin" toon:"bin"`
	Description   string               `json:"description" toon:"description"`
	Project       string               `json:"project" toon:"project"`
	MergeRequests []axiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// axiHomeUserOutput is the gl-axi no-args dashboard outside a repo.
type axiHomeUserOutput struct {
	Bin         string        `json:"bin" toon:"bin"`
	Description string        `json:"description" toon:"description"`
	User        axiUserOutput `json:"user" toon:"user"`
	Help        []string      `json:"help,omitempty" toon:"help,omitempty"`
}

// axiContextOutput is the compact session-start ambient context printed by
// `gl-axi context` for agent session hooks.
type axiContextOutput struct {
	Project       string               `json:"project" toon:"project"`
	MergeRequests []axiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

type setupTargetOutput struct {
	App    string `json:"app" toon:"app"`
	Path   string `json:"path" toon:"path"`
	Status string `json:"status" toon:"status"`
}

type axiSetupHooksOutput struct {
	Hooks []setupTargetOutput `json:"hooks" toon:"hooks"`
	Help  []string            `json:"help,omitempty" toon:"help,omitempty"`
}
