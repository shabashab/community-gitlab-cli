package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// resolveContentFlag implements the repo-wide dual-input convention for
// content-bearing flags: --<thing> carries inline text, --<thing>-file <path>
// reads a file, and --<thing>-file - reads stdin. Passing both flags is a
// usage error. File and stdin content is returned as-is, without trimming, so
// intentional whitespace (for example trailing newlines in a merge request
// description) survives.
func resolveContentFlag(cmd *cobra.Command, inline, filePath, inlineFlag, fileFlag string) (string, error) {
	inlineSet := cmd.Flags().Changed(inlineFlag)
	fileSet := cmd.Flags().Changed(fileFlag)

	if inlineSet && fileSet {
		return "", newUsageError(
			fmt.Errorf("--%s and --%s are mutually exclusive", inlineFlag, fileFlag),
			fmt.Sprintf("Pass the content inline with --%s, or from a file (or stdin via -) with --%s", inlineFlag, fileFlag),
		)
	}

	if !fileSet {
		return inline, nil
	}

	if filePath == "-" {
		content, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read --%s from stdin: %w", fileFlag, err)
		}

		return string(content), nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read --%s %q: %w", fileFlag, filePath, err)
	}

	return string(content), nil
}

// parseExtraFields validates a --fields value against a command's extra and
// default column sets and returns the extras in canonical order. Unknown
// names fail loud with the valid set inline.
func parseExtraFields(value string, extraFields, defaultFields []string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	requested := map[string]bool{}
	for _, name := range strings.Split(value, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		known := false
		for _, extra := range extraFields {
			if name == extra {
				known = true
				break
			}
		}
		for _, def := range defaultFields {
			if name == def {
				known = true // defaults are always emitted; requesting them is a no-op
				break
			}
		}
		if !known {
			return nil, newUsageError(
				fmt.Errorf("unknown field %q for --fields", name),
				fmt.Sprintf(
					"Valid --fields values: %s (defaults always included: %s)",
					strings.Join(extraFields, ", "),
					strings.Join(defaultFields, ", "),
				),
			)
		}

		requested[name] = true
	}

	var fields []string
	for _, extra := range extraFields {
		if requested[extra] {
			fields = append(fields, extra)
		}
	}

	return fields, nil
}
