package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var errUnknownMergeRequestAction = errors.New("unknown merge request action")

func newMRCommand(rootOpts *rootOptions) *cobra.Command {
	projOpts := &projectOptions{}
	viewOpts := &mrViewOptions{}
	listOpts := newMRListOptions()

	cmd := &cobra.Command{
		Use:   "mr [!<iid>|current] [action]",
		Short: "Work with GitLab merge requests",
		Long: `Work with GitLab merge requests in the current project.

Running mr with no arguments lists open merge requests. Reference a specific
merge request as !<iid> or <iid>, for example "mr !123" or "mr 123". In bash
and zsh, quote the bang form ('!123') to avoid shell history expansion.

The literal reference "current" resolves to the open merge request whose
source branch is the currently checked out git branch.`,
		Args: wrapArgsValidator(cobra.MaximumNArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				listOpts.fields = nil

				return runMRList(cmd, rootOpts, projOpts, listOpts)
			}

			if args[0] == "help" {
				return cmd.Help()
			}

			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			action := "view"
			if len(args) > 1 {
				action = args[1]
			}

			switch action {
			case "view", "info":
				return runMRView(cmd, rootOpts, projOpts, viewOpts, iid)
			case "approvals", "approval":
				return runMRApprovals(cmd, rootOpts, projOpts, &mrApprovalsOptions{full: viewOpts.full}, iid)
			case "approve":
				return newUsageError(
					fmt.Errorf("mr !%d approve takes flags and runs as a subcommand", iid),
					fmt.Sprintf("Run `mr approve !%d` — pass `--sha <sha>` if you need an optimistic head check", iid),
				)
			case "unapprove":
				return newUsageError(
					fmt.Errorf("mr !%d unapprove runs as a subcommand", iid),
					fmt.Sprintf("Run `mr unapprove !%d` to remove your approval", iid),
				)
			case "merge":
				return newUsageError(
					fmt.Errorf("mr !%d merge takes flags and runs as a subcommand", iid),
					fmt.Sprintf("Run `mr merge !%d` — pass `--sha <sha>` if you need an optimistic head check", iid),
				)
			case "close":
				return newUsageError(
					fmt.Errorf("mr !%d close runs as a subcommand", iid),
					fmt.Sprintf("Run `mr close !%d` to close the merge request", iid),
				)
			case "reopen":
				return newUsageError(
					fmt.Errorf("mr !%d reopen runs as a subcommand", iid),
					fmt.Sprintf("Run `mr reopen !%d` to reopen the merge request", iid),
				)
			case "diff", "changes":
				return runMRDiff(cmd, rootOpts, projOpts, newMRDiffListOptions(), iid)
			case "update":
				return newUsageError(
					fmt.Errorf("mr !%d update takes flags and runs as a subcommand", iid),
					fmt.Sprintf("Run `mr update !%d --<flag> <value>` — see `mr update --help` for the flag list", iid),
				)
			case "discussions", "discussion", "threads":
				return newUsageError(
					fmt.Errorf("mr !%d %s runs as a subcommand", iid, action),
					fmt.Sprintf("Run `mr discussions !%d` to list threads, `mr discussion !%d <id>` for one thread, `mr discussion resolve !%d <id>` to resolve one, or `mr discussion react !%d <id> <note-id> <emoji>` to react to a note", iid, iid, iid, iid),
				)
			case "comment":
				return newUsageError(
					fmt.Errorf("mr !%d comment takes flags and runs as a subcommand", iid),
					fmt.Sprintf("Run `mr comment !%d --body <text>` — see `mr comment --help` for position and draft flags", iid),
				)
			case "drafts", "draft":
				return newUsageError(
					fmt.Errorf("mr !%d %s runs as a subcommand", iid, action),
					fmt.Sprintf("Run `mr drafts !%d` to list pending draft notes, or `mr drafts publish !%d --all` to publish them", iid, iid),
				)
			default:
				return newUsageError(fmt.Errorf(
					"%w %q for merge request !%d: supported actions: view (alias: info), approvals, approve (as `mr approve !<iid>`), unapprove (as `mr unapprove !<iid>`), merge (as `mr merge !<iid>`), close (as `mr close !<iid>`), reopen (as `mr reopen !<iid>`), diff, update (as `mr update !<iid>`), discussions (as `mr discussions !<iid>`), discussion resolve/unresolve (as `mr discussion resolve !<iid> <id>`), discussion react/unreact (as `mr discussion react !<iid> <id> <note-id> <emoji>`), comment (as `mr comment !<iid>`), drafts (as `mr drafts !<iid>`)",
					errUnknownMergeRequestAction,
					action,
					iid,
				))
			}
		},
	}

	cmd.PersistentFlags().StringVar(
		&projOpts.project,
		"project",
		"",
		"GitLab project ID or full path (defaults to the current git origin)",
	)
	cmd.Flags().BoolVar(
		&viewOpts.full,
		"full",
		false,
		"Show all merge request fields and the complete description",
	)

	cmd.AddCommand(newMRListCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRViewCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRCreateCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRUpdateCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRApprovalsCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRApproveCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRUnapproveCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRMergeCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRCloseCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRReopenCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiscussionsCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiscussionCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRCommentCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDraftsCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiffCommand(rootOpts, projOpts))

	return cmd
}
