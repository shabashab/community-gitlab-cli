package output

// ErrorOutput is the structured error payload rendered on stdout in axi mode
// (fields per the axi error contract: error, code, help[]).
type ErrorOutput struct {
	Error string   `json:"error" toon:"error"`
	Code  string   `json:"code" toon:"code"`
	Help  []string `json:"help,omitempty" toon:"help,omitempty"`
}
