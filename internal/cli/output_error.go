package cli

import (
	"fmt"
	"io"
)

type axiErrorOutput struct {
	Error string   `json:"error" toon:"error"`
	Code  string   `json:"code" toon:"code"`
	Help  []string `json:"help,omitempty" toon:"help,omitempty"`
}

// writeCommandError renders a failed command. In axi mode the error is
// structured output on the same channel and format as normal results so the
// agent can parse and act on it.
func writeCommandError(w io.Writer, mode commandMode, format string, bin string, err error) {
	if mode != commandModeAxi {
		fmt.Fprintln(w, err)
		return
	}

	code, message, help := classifyError(err, bin)
	out := axiErrorOutput{Error: message, Code: code, Help: help}

	normalized, formatErr := normalizeOutputFormat(format, mode)
	if formatErr != nil {
		normalized = defaultOutputFormat(mode)
	}

	if writeErr := writeAxi(w, normalized, out); writeErr != nil {
		fmt.Fprintln(w, err)
	}
}
