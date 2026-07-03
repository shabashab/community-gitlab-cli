package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// contextMergeRequestLimit keeps session-start ambient context small: this
// output loads on every agent session, so it stays ruthlessly minimal.
const contextMergeRequestLimit int64 = 5

func newContextCommand(rootOpts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "context",
		Short: "Print session-start ambient context (used by setup hooks)",
		Long: `Print compact ambient context for agent session-start hooks.

Shows the open merge requests of the GitLab repository in the current
directory. Prints nothing and exits 0 when the directory has no GitLab
origin, no credential resolves, or GitLab is unreachable — ambient context
must never break or spam a session.`,
		Args: wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runContext(cmd, rootOpts)
		},
	}
}

func runContext(cmd *cobra.Command, rootOpts *rootOptions) error {
	resolved, err := resolveProject(cmd, rootOpts, nil)
	if err != nil {
		return nil
	}

	listOpts := newMRListOptions()
	listOpts.limit = contextMergeRequestLimit

	mergeRequests, paging, err := fetchMergeRequestList(cmd, rootOpts, resolved, listOpts)
	if err != nil {
		return nil
	}

	rows := make([]axiMergeRequestRow, 0, len(mergeRequests))
	for _, mergeRequest := range mergeRequests {
		if mergeRequest == nil {
			continue
		}
		rows = append(rows, axiMergeRequestRowFor(mergeRequest, nil))
	}

	help := []string{fmt.Sprintf("Run `%s mr view <iid>` for merge request details", rootOpts.binName)}
	if paging.totalItems > int64(len(rows)) {
		help = append(help, fmt.Sprintf("Run `%s mr` for all %d open merge requests", rootOpts.binName, paging.totalItems))
	}

	return writeAxi(cmd.OutOrStdout(), rootOpts.output, axiContextOutput{
		Project:       resolved.ref,
		MergeRequests: rows,
		Count:         fmt.Sprintf("%s open", mrListCountLine(len(rows), paging)),
		Help:          help,
	})
}
