package cli

import (
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
		Use:   "mr [!<iid>] [action]",
		Short: "Work with GitLab merge requests",
		Long: `Work with GitLab merge requests in the current project.

Running mr with no arguments lists open merge requests. Reference a specific
merge request as !<iid> or <iid>, for example "mr !123" or "mr 123". In bash
and zsh, quote the bang form ('!123') to avoid shell history expansion.`,
		Args: wrapArgsValidator(cobra.MaximumNArgs(2)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				listOpts.fields = nil

				return runMRList(cmd, rootOpts, projOpts, listOpts)
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
				return newUsageError(fmt.Errorf(
					"%w %q for merge request !%d: supported actions: view (alias: info)",
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
		for _, extra := range mrListExtraFields {
			if name == extra {
				known = true
				break
			}
		}
		for _, def := range mrListDefaultFields {
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
					strings.Join(mrListExtraFields, ", "),
					strings.Join(mrListDefaultFields, ", "),
				),
			)
		}

		requested[name] = true
	}

	var fields []string
	for _, extra := range mrListExtraFields {
		if requested[extra] {
			fields = append(fields, extra)
		}
	}

	return fields, nil
}

func newMRViewCommand(rootOpts *rootOptions, projOpts *projectOptions) *cobra.Command {
	opts := &mrViewOptions{}

	cmd := &cobra.Command{
		Use:     "view <!iid|iid>",
		Aliases: []string{"info"},
		Short:   "Show merge request information",
		Args:    wrapArgsValidator(cobra.ExactArgs(1)),
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
		return 0, newUsageError(
			fmt.Errorf("%w %q: expected !<iid> or <iid>, for example !123", errInvalidMergeRequestRef, ref),
		)
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

	mergeRequest, _, err := client.MergeRequests.GetMergeRequest(resolved.ref, iid, nil, gitlab.WithContext(commandContext(cmd)))
	if err != nil {
		return fmt.Errorf("get merge request !%d in project %q: %w", iid, resolved.ref, err)
	}

	hints := &mrHintContext{project: explicitProjectRef(projOpts)}

	return writeMergeRequest(cmd.OutOrStdout(), rootOpts.output, rootOpts.mode, mergeRequest, opts.full, hints)
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
