package cli

import (
	"fmt"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

type mrListPaging struct {
	page       int64
	totalItems int64
	totalPages int64
}

// mrListCountLine states the definitive result size, including the explicit
// zero (axi guide §5) and the unknown-total case where GitLab omits X-Total.
func mrListCountLine(count int, paging mrListPaging) string {
	if count > 0 && paging.totalItems == 0 {
		return fmt.Sprintf("%d of unknown total", count)
	}

	return fmt.Sprintf("%d of %d total", count, paging.totalItems)
}

// mrHintContext carries invocation context into help hints so suggested
// commands stay runnable as-is (axi guide §9: carry disambiguating flags).
type mrHintContext struct {
	project string
	limit   int64
}

func (c *mrHintContext) projectSuffix() string {
	if c == nil || strings.TrimSpace(c.project) == "" {
		return ""
	}

	return " --project " + strings.TrimSpace(c.project)
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

// truncateDescription cuts long descriptions at limit runes and appends an
// explicit size marker. The standard-mode marker keeps the inline --full hint
// (text output has no help channel); the axi marker stays bare because the
// escape hatch is suggested through the structured help field.
func truncateDescription(value string, limit int, mode commandMode) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}

	if mode == commandModeAxi {
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
