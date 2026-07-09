package output

import (
	"fmt"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type MRListPaging struct {
	Page       int64
	TotalItems int64
	TotalPages int64
}

// MRListCountLine states the definitive result size, including the explicit
// zero (axi guide §5) and the unknown-total case where GitLab omits X-Total.
func MRListCountLine(count int, paging MRListPaging) string {
	if count > 0 && paging.TotalItems == 0 {
		return fmt.Sprintf("%d of unknown total", count)
	}

	return fmt.Sprintf("%d of %d total", count, paging.TotalItems)
}

// MRHintContext carries invocation context into help hints so suggested
// commands stay runnable as-is (axi guide §9: carry disambiguating flags).
type MRHintContext struct {
	Project string
	Limit   int64
}

func (c *MRHintContext) ProjectSuffix() string {
	if c == nil || strings.TrimSpace(c.Project) == "" {
		return ""
	}

	return " --project " + strings.TrimSpace(c.Project)
}

func basicUsernames(users []*gitlab.BasicUser) []string {
	names := make([]string, 0, len(users))
	for _, user := range users {
		name := basicUsername(user)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	return names
}

func basicUsername(user *gitlab.BasicUser) string {
	if user == nil {
		return ""
	}
	if user.Username != "" {
		return user.Username
	}

	return user.Name
}

func usernamesOf(users []*gitlab.BasicUser) []string {
	names := make([]string, 0, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		names = append(names, user.Username)
	}

	return names
}

func formatTimeValue(t *time.Time) string {
	if t == nil {
		return ""
	}

	return t.Format("2006-01-02T15:04:05Z07:00")
}

// TruncateDescription cuts long descriptions at limit runes and appends an
// explicit size marker. The standard-mode marker keeps the inline --full hint
// (text output has no help channel); the axi marker stays bare because the
// escape hatch is suggested through the structured help field.
func TruncateDescription(value string, limit int, mode Mode) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}

	if mode == ModeAxi {
		return fmt.Sprintf("%s… (truncated, %d chars total)", string(runes[:limit]), len(runes)), true
	}

	return fmt.Sprintf(
		"%s… (truncated, %d chars total — use --full for the complete description)",
		string(runes[:limit]),
		len(runes),
	), true
}

// formatLocalTime renders a locally computed time value, treating the zero
// time as absent (unlike formatTimeValue it takes a value, not a pointer).
func formatLocalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	return t.Format("2006-01-02T15:04:05Z07:00")
}

// DefaultMergeRequestListLimit is the shared default page size for merge
// request style lists; cli flag defaults and hint text both reference it.
const DefaultMergeRequestListLimit int64 = 20
