package cli

import (
	"fmt"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// mrListExtraFields are the optional axi list columns that --fields can add
// to the compact default schema (iid, title, state, author).
var mrListExtraFields = []string{"draft", "source_branch", "target_branch", "updated_at", "web_url"}

var mrListDefaultFields = []string{"iid", "title", "state", "author"}

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
		limit: output.DefaultMergeRequestListLimit,
		page:  1,
	}
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

func runMRList(cmd *cobra.Command, rootOpts *rootOptions, projOpts *projectOptions, opts *mrListOptions) error {
	resolved, err := resolveProject(cmd, rootOpts, projOpts)
	if err != nil {
		return err
	}

	mergeRequests, paging, err := fetchMergeRequestList(cmd, rootOpts, resolved, opts)
	if err != nil {
		return err
	}

	hints := &output.MRHintContext{Project: explicitProjectRef(projOpts), Limit: opts.limit}

	return output.WriteMergeRequestList(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequests, paging, opts.fields, hints)
}

func fetchMergeRequestList(cmd *cobra.Command, rootOpts *rootOptions, resolved resolvedProject, opts *mrListOptions) ([]*gitlab.BasicMergeRequest, output.MRListPaging, error) {
	client, err := rootOpts.newGitLabClientWithBaseURLFallback(resolved.baseURL)
	if err != nil {
		return nil, output.MRListPaging{}, err
	}

	mergeRequests, resp, err := client.MergeRequests.ListProjectMergeRequests(
		resolved.ref,
		buildListMergeRequestsOptions(opts),
		gitlab.WithContext(commandContext(cmd)),
	)
	if err != nil {
		return nil, output.MRListPaging{}, fmt.Errorf("list merge requests in project %q: %w", resolved.ref, err)
	}

	paging := output.MRListPaging{Page: opts.page}
	if resp != nil {
		if resp.CurrentPage > 0 {
			paging.Page = resp.CurrentPage
		}
		paging.TotalItems = resp.TotalItems
		paging.TotalPages = resp.TotalPages
	}

	return mergeRequests, paging, nil
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
