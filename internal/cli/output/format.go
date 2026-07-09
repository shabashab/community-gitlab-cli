package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	toon "github.com/toon-format/toon-go"
)

// Mode selects the presentation contract: standard gl output or agent-oriented
// gl-axi output. The output package owns the mode because every writer
// branches on it; cli aliases the type for its own dispatch.
type Mode string

const (
	ModeStandard Mode = "standard"
	ModeAxi      Mode = "axi"
)

// UsageError marks errors caused by invalid invocation (unknown flags, bad
// arguments, unsupported formats). Commands exit with code 2 for usage errors
// and 1 for everything else, matching the axi exit-code contract. It lives
// here because format negotiation reports invalid --output values; cli
// aliases the type so errors.As identity is preserved.
type UsageError struct {
	Err  error
	Help []string
}

func (e *UsageError) Error() string { return e.Err.Error() }

func (e *UsageError) Unwrap() error { return e.Err }

func NewUsageError(err error, help ...string) error {
	return &UsageError{Err: err, Help: help}
}

func DefaultFormat(mode Mode) string {
	if mode == ModeAxi {
		return "toon"
	}

	return "text"
}

func Formats(mode Mode) string {
	if mode == ModeAxi {
		return "toon, json"
	}

	return "text, json"
}

func NormalizeFormat(format string, mode Mode) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return DefaultFormat(mode), nil
	}

	switch mode {
	case ModeAxi:
		if format == "toon" || format == "json" {
			return format, nil
		}
	default:
		if format == "text" || format == "json" {
			return format, nil
		}
	}

	return "", NewUsageError(
		fmt.Errorf("unsupported output format %q: use %s", format, Formats(mode)),
		fmt.Sprintf("Valid --output values: %s", Formats(mode)),
	)
}

func WriteJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(v)
}

// WriteAxi renders v as TOON (default) or JSON. The trailing newline is a CLI
// convention on top of the TOON document, which itself ends without one.
func WriteAxi(w io.Writer, format string, v any) error {
	format, err := NormalizeFormat(format, ModeAxi)
	if err != nil {
		return err
	}

	if format == "json" {
		return WriteJSON(w, v)
	}

	encoded, err := toon.MarshalString(v)
	if err != nil {
		return fmt.Errorf("encode toon output: %w", err)
	}
	_, err = fmt.Fprintln(w, encoded)

	return err
}

func EncodeTOON(v any) (string, error) {
	encoded, err := toon.MarshalString(v)
	if err != nil {
		return "", fmt.Errorf("encode toon output: %w", err)
	}

	return encoded + "\n", nil
}
