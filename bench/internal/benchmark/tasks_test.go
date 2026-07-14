package benchmark

import (
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

func TestMVPTasks(t *testing.T) {
	tasks := Tasks()
	if len(tasks) != 3 {
		t.Fatalf("len(Tasks()) = %d, want 3", len(tasks))
	}

	fixture := &Fixture{
		MergeRequest: &gitlab.MergeRequest{BasicMergeRequest: gitlab.BasicMergeRequest{IID: 7}},
		MRTitle:      "Benchmark target abc123",
		SourceBranch: "bench-feature",
		TargetBranch: "main",
		ChangedPath:  "benchmark.txt",
		Marker:       "abc123",
	}

	for _, task := range tasks[:2] {
		prompt := BuildPrompt("gl-axi", task, fixture, "helper material")
		for _, want := range []string{"gl-axi", "helper material", task.Prompt(fixture)} {
			if !strings.Contains(prompt, want) {
				t.Errorf("task %s prompt does not contain %q", task.ID, want)
			}
		}
	}

	for _, answer := range []string{"MR !7 bench-feature -> main", "IID: **7** bench-feature -> main"} {
		findGrade := tasks[0].Grade(fixture, AgentResult{FinalMessage: answer})
		if !findGrade.Passed {
			t.Fatalf("find grade for %q = %+v", answer, findGrade)
		}
	}
	diffGrade := tasks[1].Grade(fixture, AgentResult{FinalMessage: "benchmark.txt contains abc123"})
	if !diffGrade.Passed {
		t.Fatalf("diff grade = %+v", diffGrade)
	}
}

func TestSelectTasksRejectsUnknown(t *testing.T) {
	if _, err := SelectTasks([]string{"not-a-task"}); err == nil {
		t.Fatal("SelectTasks succeeded for an unknown task")
	}
}
