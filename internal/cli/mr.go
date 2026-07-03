package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errInvalidMergeRequestRef    = errors.New("invalid merge request reference")
	errUnknownMergeRequestAction = errors.New("unknown merge request action")
)

const (
	defaultMergeRequestListLimit int64 = 20
	descriptionTruncateLimit           = 500
)

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

	cmd := &cobra.Command{
		Use:   "mr [!<iid>] [action]",
		Short: "Work with GitLab merge requests",
		Long: `Work with GitLab merge requests in the current project.

Running mr with no arguments lists open merge requests. Reference a specific
merge request as !<iid> or <iid>, for example "mr !123" or "mr 123". In bash
and zsh, quote the bang form ('!123') to avoid shell history expansion.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runMRList(cmd, rootOpts, projOpts, newMRListOptions())
			}

			if args[0] == "help" {
				return cmd.Help()
			}

			iid, err := parseMergeRequestRef(args[0])
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
			default:
				return fmt.Errorf(
					"%w %q for merge request !%d: supported actions: view",
					errUnknownMergeRequestAction,
					action,
					iid,
				)
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

	return cmd
}

func newMRListCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := newMRListOptions()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List and search merge requests in a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.draftSet = cmd.Flags().Changed("draft")
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

	return cmd
}

func newMRViewCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrViewOptions{}

	cmd := &cobra.Command{
		Use:     "view <!iid|iid>",
		Aliases: []string{"info"},
		Short:   "Show merge request information",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			iid, err := parseMergeRequestRef(args[0])
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
		return 0, fmt.Errorf("%w %q: expected !<iid> or <iid>, for example !123", errInvalidMergeRequestRef, ref)
	}

	return iid, nil
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

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(resolved.ref, iid, nil, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("get merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	return writeMergeRequest(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequest, opts.full)
}

func runMRList(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrListOptions) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	mergeRequests, resp, err := client.MergeRequests.ListProjectMergeRequests(
		resolved.ref,
		buildListMergeRequestsOptions(opts),
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("list merge requests in project %q: %w", resolved.ref, err)
	}

	paging := mrListPaging{page: opts.page}
	if resp != nil {
		if resp.CurrentPage > 0 {
			paging.page = resp.CurrentPage
		}
		paging.totalItems = resp.TotalItems
		paging.totalPages = resp.TotalPages
	}

	return writeMergeRequestList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequests, paging)
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
