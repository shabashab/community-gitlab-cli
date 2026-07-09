package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shabashab/community-gitlab-cli/internal/cli/output"
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

var (
	errInvalidDiscussionRef   = errors.New("invalid discussion reference")
	errAmbiguousDiscussionRef = errors.New("ambiguous discussion reference")
	errDiscussionNotFound     = errors.New("discussion not found")
)

const (
	discussionFetchPageSize int64 = 100
)

// discussionRefPattern matches a full 40-character hex discussion ID or any
// prefix of one.
var discussionRefPattern = regexp.MustCompile(`^[0-9a-f]{1,40}$`)

// fetchAllMergeRequestDiscussions pages through the full discussion list. The
// discussions API has no filters or sorting, so every command works from the
// complete set and totals stay exact.
func fetchAllMergeRequestDiscussions(ctx context.Context, client *gitlab.Client, projectRef any, iid int64) ([]*gitlab.Discussion, error) {
	opt := &gitlab.ListMergeRequestDiscussionsOptions{
		ListOptions: gitlab.ListOptions{PerPage: discussionFetchPageSize, Page: 1},
	}

	var all []*gitlab.Discussion
	for {
		discussions, resp, err := client.Discussions.ListMergeRequestDiscussions(projectRef, iid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list discussions on merge request !%d in project %q: %w", iid, projectRef, err)
		}
		all = append(all, discussions...)

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return all, nil
}

// resolveDiscussionRef turns a user-supplied discussion reference — a full
// 40-character hex ID or a unique prefix — into the discussion it names.
func resolveDiscussionRef(ctx context.Context, client *gitlab.Client, projectRef any, iid int64, ref string) (*gitlab.Discussion, error) {
	normalized := strings.ToLower(strings.TrimSpace(ref))
	if !discussionRefPattern.MatchString(normalized) {
		return nil, newUsageError(
			fmt.Errorf("%w %q: expected a 40-character hex ID or a unique prefix of one", errInvalidDiscussionRef, ref),
		)
	}

	if len(normalized) == 40 {
		discussion, _, err := client.Discussions.GetMergeRequestDiscussion(projectRef, iid, normalized, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("get discussion %s on merge request !%d in project %q: %w", normalized, iid, projectRef, err)
		}

		return discussion, nil
	}

	discussions, err := fetchAllMergeRequestDiscussions(ctx, client, projectRef, iid)
	if err != nil {
		return nil, err
	}

	var matches []*gitlab.Discussion
	for _, discussion := range discussions {
		if discussion != nil && strings.HasPrefix(strings.ToLower(discussion.ID), normalized) {
			matches = append(matches, discussion)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("%w: no discussion on merge request !%d matches prefix %q", errDiscussionNotFound, iid, normalized)
	default:
		return nil, newUsageError(
			fmt.Errorf("%w %q: %d discussions match", errAmbiguousDiscussionRef, normalized, len(matches)),
		)
	}
}

func filterDiscussionSummaries(summaries []output.DiscussionSummary, state, author string, includeSystem bool) []output.DiscussionSummary {
	author = strings.TrimPrefix(strings.TrimSpace(author), "@")

	filtered := make([]output.DiscussionSummary, 0, len(summaries))
	for _, summary := range summaries {
		if summary.System && !includeSystem {
			continue
		}
		if author != "" && !strings.EqualFold(summary.Author, author) {
			continue
		}

		switch state {
		case "resolved":
			if !summary.Resolved {
				continue
			}
		case "unresolved":
			// glab semantics: unresolved means resolvable and not resolved;
			// non-resolvable threads match only --state all.
			if !summary.Resolvable || summary.Resolved {
				continue
			}
		}

		filtered = append(filtered, summary)
	}

	return filtered
}

func sortDiscussionSummaries(summaries []output.DiscussionSummary, orderBy, sortDir string) {
	key := func(summary output.DiscussionSummary) time.Time {
		if orderBy == "updated_at" {
			return summary.UpdatedAt
		}

		return summary.CreatedAt
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if sortDir == "desc" {
			return key(summaries[j]).Before(key(summaries[i]))
		}

		return key(summaries[i]).Before(key(summaries[j]))
	})
}

// pageDiscussionSummaries slices the filtered result into the requested page.
// Totals are exact because the whole set was fetched and filtered locally.
func pageDiscussionSummaries(summaries []output.DiscussionSummary, page, limit int64) ([]output.DiscussionSummary, output.MRListPaging) {
	total := int64(len(summaries))
	paging := output.MRListPaging{
		Page:       page,
		TotalItems: total,
		TotalPages: (total + limit - 1) / limit,
	}

	start := (page - 1) * limit
	if start >= total {
		return nil, paging
	}

	end := start + limit
	if end > total {
		end = total
	}

	return summaries[start:end], paging
}
