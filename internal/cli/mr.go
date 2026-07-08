package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/repo"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errInvalidMergeRequestRef       = errors.New("invalid merge request reference")
	errUnknownMergeRequestAction    = errors.New("unknown merge request action")
	errMissingCurrentBranch         = errors.New("cannot determine current git branch")
	errNoCurrentMergeRequest        = errors.New("no open merge request for the current branch")
	errAmbiguousCurrentMergeRequest = errors.New("multiple open merge requests for the current branch")
)

// currentBranchFunc is a test seam over repo.CurrentBranch.
var currentBranchFunc = repo.CurrentBranch

const (
	defaultMergeRequestListLimit int64 = 20
	descriptionTruncateLimit           = 500

	// currentMergeRequestRef is the literal ref that resolves to the merge
	// request of the currently checked out git branch.
	currentMergeRequestRef = "current"
	// currentMergeRequestLookupPerPage must be at least 2 so an ambiguous
	// branch (several open merge requests) is detectable; 10 also bounds the
	// candidate list echoed in the ambiguity error.
	currentMergeRequestLookupPerPage int64 = 10
)

// mrListExtraFields are the optional axi list columns that --fields can add
// to the compact default schema (iid, title, state, author).
var mrListExtraFields = []string{"draft", "source_branch", "target_branch", "updated_at", "web_url"}

var mrListDefaultFields = []string{"iid", "title", "state", "author"}

type mrViewOptions struct {
	full bool
}

type mrListOptions struct {
	state        string
	search       string
	labels       []string
	author       string
	reviewer     string
	sourceBranch string
	targetBranch string
	draft        bool
	draftSet     bool
	milestone    string
	orderBy      string
	sort         string
	limit        int64
	page         int64
	fields       []string
}

func newMRListOptions() *mrListOptions {
	return &mrListOptions{
		state: "opened",
		limit: defaultMergeRequestListLimit,
		page:  1,
	}
}

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
					fmt.Sprintf("Run `mr discussions !%d` to list threads, or `mr discussion !%d <id>` for one thread", iid, iid),
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
					"%w %q for merge request !%d: supported actions: view (alias: info), approvals, approve (as `mr approve !<iid>`), unapprove (as `mr unapprove !<iid>`), diff, update (as `mr update !<iid>`), discussions (as `mr discussions !<iid>`), comment (as `mr comment !<iid>`), drafts (as `mr drafts !<iid>`)",
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
	cmd.AddCommand(newMRDiscussionsCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiscussionCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRCommentCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDraftsCommand(rootOpts, projOpts))
	cmd.AddCommand(newMRDiffCommand(rootOpts, projOpts))

	return cmd
}

func newMRListCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := newMRListOptions()
	var fieldsFlag string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List and search merge requests in a project",
		Args:  wrapArgsValidator(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.draftSet = cmd.Flags().Changed("draft")

			fields, err := parseMRListFields(fieldsFlag)
			if err != nil {
				return err
			}
			opts.fields = fields

			return runMRList(cmd, rootOpts, projOpts, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.state, "state", opts.state, "Filter by state: opened, closed, locked, merged, all")
	flags.StringVar(&opts.search, "search", "", "Search in title and description")
	flags.StringSliceVar(&opts.labels, "label", nil, "Filter by label (repeatable or comma-separated)")
	flags.StringVar(&opts.author, "author", "", "Filter by author username")
	flags.StringVar(&opts.reviewer, "reviewer", "", "Filter by reviewer username")
	flags.StringVar(&opts.sourceBranch, "source-branch", "", "Filter by source branch")
	flags.StringVar(&opts.targetBranch, "target-branch", "", "Filter by target branch")
	flags.BoolVar(&opts.draft, "draft", false, "Filter drafts only; use --draft=false for non-drafts (omit for both)")
	flags.StringVar(&opts.milestone, "milestone", "", "Filter by milestone title (also accepts None or Any)")
	flags.StringVar(&opts.orderBy, "order-by", "", "Order by: created_at, updated_at, or title (API default created_at)")
	flags.StringVar(&opts.sort, "sort", "", "Sort direction: asc or desc (API default desc)")
	flags.Int64Var(&opts.limit, "limit", opts.limit, "Results per page (GitLab max 100)")
	flags.Int64Var(&opts.page, "page", opts.page, "Result page to fetch")
	flags.StringVar(
		&fieldsFlag,
		"fields",
		"",
		fmt.Sprintf("Comma-separated extra columns to add to the compact schema: %s", strings.Join(mrListExtraFields, ", ")),
	)

	return cmd
}

// parseMRListFields validates a --fields value and returns the extra columns
// in canonical order. Unknown names fail loud with the valid set inline.
func parseMRListFields(value string) ([]string, error) {
	return parseExtraFields(value, mrListExtraFields, mrListDefaultFields)
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

func newMRViewCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrViewOptions{}

	cmd := &cobra.Command{
		Use:     "view <!iid|iid|current>",
		Aliases: []string{"info"},
		Short:   "Show merge request information",
		Args:    wrapArgsValidator(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := resolveMergeRequestRef(cmd, rootOpts, projOpts, args[0])
			if err != nil {
				return err
			}

			return runMRView(cmd, rootOpts, projOpts, opts, iid)
		},
	}

	cmd.Flags().BoolVar(
		&opts.full,
		"full",
		false,
		"Show all merge request fields and the complete description",
	)

	return cmd
}

func parseMergeRequestRef(ref string) (int64, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(ref), "!")

	iid, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || iid <= 0 {
		return 0, newUsageError(
			fmt.Errorf("%w %q: expected !<iid>, <iid>, or current, for example !123", errInvalidMergeRequestRef, ref),
		)
	}

	return iid, nil
}

// resolveMergeRequestRef turns a merge request reference into an iid. The
// literal ref "current" (or "!current") resolves to the single open merge
// request whose source branch is the currently checked out git branch;
// anything else must parse as !<iid> or <iid>.
func resolveMergeRequestRef(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, ref string) (int64, error) {
	if strings.TrimPrefix(strings.TrimSpace(ref), "!") != currentMergeRequestRef {
		return parseMergeRequestRef(ref)
	}

	return resolveCurrentMergeRequestIID(cmd, rootOpts, projOpts)
}

func resolveCurrentMergeRequestIID(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions) (int64, error) {
	branch, err := currentBranchFunc(commandContext(cmd), "")
	if err != nil {
		return 0, fmt.Errorf("%w (%v): pass an explicit merge request iid", errMissingCurrentBranch, err)
	}

	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return 0, err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return 0, err
	}

	mergeRequests, _, err := client.MergeRequests.ListProjectMergeRequests(
		resolved.ref,
		&gitlab.ListProjectMergeRequestsOptions{
			ListOptions: gitlab.ListOptions{
				PerPage: currentMergeRequestLookupPerPage,
				Page:    1,
			},
			SourceBranch: gitlab.Ptr(branch),
			State:        gitlab.Ptr("opened"),
		},
		gitlab.WithContext(commandContext(cmd)),
	)
	if err != nil {
		return 0, fmt.Errorf("resolve merge request %q in project %q: %w", currentMergeRequestRef, resolved.ref, err)
	}

	bin := rootOpts.binName
	suffix := (&mrHintContext{project: explicitProjectRef(projOpts)}).projectSuffix()

	switch len(mergeRequests) {
	case 0:
		return 0, newHelpError(
			fmt.Errorf("%w: source branch %q has no open merge request in project %q", errNoCurrentMergeRequest, branch, resolved.ref),
			fmt.Sprintf("Run `%s mr list --source-branch %s --state all%s` to see merge requests for this branch (it may be merged or closed)", bin, branch, suffix),
			fmt.Sprintf("Pass an explicit iid, e.g. `%s mr view <iid>%s`", bin, suffix),
		)
	case 1:
		return mergeRequests[0].IID, nil
	default:
		candidates := make([]string, len(mergeRequests))
		for i, mergeRequest := range mergeRequests {
			candidates[i] = fmt.Sprintf("!%d", mergeRequest.IID)
		}

		return 0, newHelpError(
			fmt.Errorf("%w: source branch %q matches %s in project %q", errAmbiguousCurrentMergeRequest, branch, strings.Join(candidates, ", "), resolved.ref),
			fmt.Sprintf("Pass one of the matching iids explicitly, e.g. `%s mr view %d%s`", bin, mergeRequests[0].IID, suffix),
			fmt.Sprintf("Run `%s mr list --source-branch %s%s` to compare the candidates", bin, branch, suffix),
		)
	}
}

func runMRView(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrViewOptions, iid int64) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(resolved.ref, iid, nil, gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("get merge request !%d in project %q: %w", iid, resolved.ref, err)
	}
	approvals, err := fetchMergeRequestApprovals(cmd, client, resolved.ref, iid)
	if err != nil {
		return err
	}

	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	return writeMergeRequestWithApprovals(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequest, approvals, opts.full, hints)
}

func runMRList(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrListOptions) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	mergeRequests, paging, err := fetchMergeRequestList(cmd, rootOpts, resolved, opts)
	if err != nil {
		return err
	}

	hints := &mrHintContext{project: explicitProjectRef(projOpts), limit: opts.limit}

	return writeMergeRequestList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequests, paging, opts.fields, hints)
}

func fetchMergeRequestList(cmd *cobra.Command, rootOpts *rootOptions, resolved resolvedProject, opts *mrListOptions) ([]*gitlab.BasicMergeRequest, mrListPaging, error) {
	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return nil, mrListPaging{}, err
	}

	mergeRequests, resp, err := client.MergeRequests.ListProjectMergeRequests(
		resolved.ref,
		buildListMergeRequestsOptions(opts),
		gitlab.WithContext(commandContext(cmd)),
	)
	if err != nil {
		return nil, mrListPaging{}, fmt.Errorf("list merge requests in project %q: %w", resolved.ref, err)
	}

	paging := mrListPaging{page: opts.page}
	if resp != nil {
		if resp.CurrentPage > 0 {
			paging.page = resp.CurrentPage
		}
		paging.totalItems = resp.TotalItems
		paging.totalPages = resp.TotalPages
	}

	return mergeRequests, paging, nil
}

// explicitProjectRef reports the --project value when one was passed, so help
// hints can carry the flag forward into suggested commands.
func explicitProjectRef(projOpts *projectOptions) string {
	if projOpts == nil {
		return ""
	}

	return strings.TrimSpace(projOpts.project)
}

func buildListMergeRequestsOptions(opts *mrListOptions) *gitlab.ListProjectMergeRequestsOptions {
	listOpts := &gitlab.ListProjectMergeRequestsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: opts.limit,
			Page:    opts.page,
		},
	}

	if opts.state != "" {
		listOpts.State = gitlab.Ptr(opts.state)
	}
	if opts.search != "" {
		listOpts.Search = gitlab.Ptr(opts.search)
	}
	if len(opts.labels) > 0 {
		labels := gitlab.LabelOptions(opts.labels)
		listOpts.Labels = &labels
	}
	if opts.author != "" {
		listOpts.AuthorUsername = gitlab.Ptr(opts.author)
	}
	if opts.reviewer != "" {
		listOpts.ReviewerUsername = gitlab.Ptr(opts.reviewer)
	}
	if opts.sourceBranch != "" {
		listOpts.SourceBranch = gitlab.Ptr(opts.sourceBranch)
	}
	if opts.targetBranch != "" {
		listOpts.TargetBranch = gitlab.Ptr(opts.targetBranch)
	}
	if opts.draftSet {
		listOpts.Draft = gitlab.Ptr(opts.draft)
	}
	if opts.milestone != "" {
		listOpts.Milestone = gitlab.Ptr(opts.milestone)
	}
	if opts.orderBy != "" {
		listOpts.OrderBy = gitlab.Ptr(opts.orderBy)
	}
	if opts.sort != "" {
		listOpts.Sort = gitlab.Ptr(opts.sort)
	}

	return listOpts
}
