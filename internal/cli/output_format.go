package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	toon "github.com/toon-format/toon-go"
)

func defaultOutputFormat(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon"
	}

	return "text"
}

func outputFormats(mode commandMode) string {
	if mode == commandModeAxi {
		return "toon, json"
	}

	return "text, json"
}

func normalizeOutputFormat(format string, mode commandMode) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return defaultOutputFormat(mode), nil
	}

	switch mode {
	case commandModeAxi:
		if format == "toon" || format == "json" {
			return format, nil
		}
	default:
		if format == "text" || format == "json" {
			return format, nil
		}
	}

	return "", newUsageError(
		fmt.Errorf("unsupported output format %q: use %s", format, outputFormats(mode)),
		fmt.Sprintf("Valid --output values: %s", outputFormats(mode)),
	)
}

func writeJSON(w io.Writer, v any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	return encoder.Encode(v)
}

// writeAxi renders v as TOON (default) or JSON. The trailing newline is a CLI
// convention on top of the TOON document, which itself ends without one.
func writeAxi(w io.Writer, format string, v any) error {
	format, err := normalizeOutputFormat(format, commandModeAxi)
	if err != nil {
		return err
	}

	if format == "json" {
		return writeJSON(w, v)
	}

	encoded, err := toon.MarshalString(v)
	if err != nil {
		return fmt.Errorf("encode toon output: %w", err)
	}
	_, err = fmt.Fprintln(w, encoded)

	return err
}

func encodeTOON(v any) (string, error) {
	encoded, err := toon.MarshalString(v)
	if err != nil {
		return "", fmt.Errorf("encode toon output: %w", err)
	}

	return encoded + "\n", nil
}
