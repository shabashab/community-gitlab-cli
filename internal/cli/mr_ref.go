package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	"github.com/shabashab/community-gitlab-cli/internal/repo"
	"github.com/spf13/cobra"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errInvalidMergeRequestRef       = errors.New("invalid merge request reference")
	errMissingCurrentBranch         = errors.New("cannot determine current git branch")
	errNoCurrentMergeRequest        = errors.New("no open merge request for the current branch")
	errAmbiguousCurrentMergeRequest = errors.New("multiple open merge requests for the current branch")
)

// currentBranchFunc is a test seam over repo.CurrentBranch.
var currentBranchFunc = repo.CurrentBranch

const (

	// currentMergeRequestRef is the literal ref that resolves to the merge
	// request of the currently checked out git branch.
	currentMergeRequestRef = "current"
	// currentMergeRequestLookupPerPage must be at least 2 so an ambiguous
	// branch (several open merge requests) is detectable; 10 also bounds the
	// candidate list echoed in the ambiguity error.
	currentMergeRequestLookupPerPage int64 = 10
)

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
	suffix := (&output.MRHintContext{Project: explicitProjectRef(projOpts)}).ProjectSuffix()

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
