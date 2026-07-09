package output

// AxiHomeRepoOutput is the gl-axi no-args dashboard inside a GitLab repo.
type AxiHomeRepoOutput struct {
	Bin           string               `json:"bin" toon:"bin"`
	Description   string               `json:"description" toon:"description"`
	Project       string               `json:"project" toon:"project"`
	MergeRequests []AxiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

// AxiHomeUserOutput is the gl-axi no-args dashboard outside a repo.
type AxiHomeUserOutput struct {
	Bin         string        `json:"bin" toon:"bin"`
	Description string        `json:"description" toon:"description"`
	User        AxiUserOutput `json:"user" toon:"user"`
	Help        []string      `json:"help,omitempty" toon:"help,omitempty"`
}

// AxiContextOutput is the compact session-start ambient context printed by
// `gl-axi context` for agent session hooks.
type AxiContextOutput struct {
	Project       string               `json:"project" toon:"project"`
	MergeRequests []AxiMergeRequestRow `json:"merge_requests" toon:"merge_requests"`
	Count         string               `json:"count" toon:"count"`
	Help          []string             `json:"help,omitempty" toon:"help,omitempty"`
}

type SetupTargetOutput struct {
	App    string `json:"app" toon:"app"`
	Path   string `json:"path" toon:"path"`
	Status string `json:"status" toon:"status"`
}

type AxiSetupHooksOutput struct {
	Hooks []SetupTargetOutput `json:"hooks" toon:"hooks"`
	Help  []string            `json:"help,omitempty" toon:"help,omitempty"`
}
