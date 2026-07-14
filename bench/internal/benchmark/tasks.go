package benchmark

import (
	"fmt"
	"regexp"
	"strings"
)

const taskPreamble = `Complete this GitLab task autonomously.

Use only the GitLab command named %[1]q for GitLab operations. Do not use curl,
raw HTTP, another GitLab CLI, or an MCP server. You may use %[1]s --help when
needed. Do not modify local files. Verify the result before finishing. Keep the
final answer brief and include the requested values.`

func Tasks() []Task {
	return []Task{
		{
			ID:          TaskFindMR,
			Description: "find a uniquely titled open merge request and report its identity",
			Prompt: func(f *Fixture) string {
				return fmt.Sprintf(
					"Find the open merge request whose title is exactly %q. Report its IID, source branch, and target branch.",
					f.MRTitle,
				)
			},
			Grade: func(f *Fixture, result AgentResult) Grade {
				grade := gradeContains(result.FinalMessage, f.SourceBranch, f.TargetBranch)
				pattern := regexp.MustCompile(fmt.Sprintf(`(?i)(!%d\b|iid\D{0,8}%d\b)`, f.MergeRequest.IID, f.MergeRequest.IID))
				assertion := fmt.Sprintf("final answer identifies merge request IID %d", f.MergeRequest.IID)
				if pattern.MatchString(result.FinalMessage) {
					grade.Assertions = append(grade.Assertions, assertion)
				} else {
					grade.Passed = false
					grade.Failures = append(grade.Failures, assertion)
				}
				return grade
			},
		},
		{
			ID:          TaskInspectDiff,
			Description: "inspect an MR diff and recover an exact marker from the changed file",
			Prompt: func(f *Fixture) string {
				return fmt.Sprintf(
					"Inspect merge request !%d. Report the changed file path and the exact benchmark marker added by the diff.",
					f.MergeRequest.IID,
				)
			},
			Grade: func(f *Fixture, result AgentResult) Grade {
				return gradeContains(result.FinalMessage, f.ChangedPath, f.Marker)
			},
		},
		{
			ID:          TaskCommentMR,
			Description: "add one exact MR comment without creating duplicates",
			Prompt: func(f *Fixture) string {
				return fmt.Sprintf(
					"Add exactly one top-level comment to merge request !%d with this exact body: %s",
					f.MergeRequest.IID,
					f.CommentBody,
				)
			},
			Grade: func(f *Fixture, _ AgentResult) Grade {
				return f.GradeExactComment()
			},
		},
	}
}

func SelectTasks(ids []string) ([]Task, error) {
	all := Tasks()
	if len(ids) == 0 {
		return all, nil
	}

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[strings.TrimSpace(id)] = true
	}

	selected := make([]Task, 0, len(wanted))
	for _, task := range all {
		if wanted[string(task.ID)] {
			selected = append(selected, task)
			delete(wanted, string(task.ID))
		}
	}
	if len(wanted) != 0 {
		unknown := make([]string, 0, len(wanted))
		for id := range wanted {
			unknown = append(unknown, id)
		}
		return nil, fmt.Errorf("unknown benchmark task(s): %s", strings.Join(unknown, ", "))
	}

	return selected, nil
}

func BuildPrompt(tool string, task Task, fixture *Fixture, helper string) string {
	prompt := fmt.Sprintf(taskPreamble, tool) + "\n\nTask:\n" + task.Prompt(fixture)
	if strings.TrimSpace(helper) != "" {
		prompt += "\n\nTool helper reference:\n<tool-helper>\n" + helper + "\n</tool-helper>"
	}
	return prompt
}

func gradeContains(message string, values ...string) Grade {
	grade := Grade{Passed: true}
	for _, value := range values {
		assertion := fmt.Sprintf("final answer contains %q", value)
		if strings.Contains(message, value) {
			grade.Assertions = append(grade.Assertions, assertion)
			continue
		}
		grade.Passed = false
		grade.Failures = append(grade.Failures, assertion)
	}
	return grade
}
