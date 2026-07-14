package benchmark

import "time"

// TaskID identifies one benchmark task.
type TaskID string

const (
	TaskFindMR      TaskID = "find-mr"
	TaskInspectDiff TaskID = "inspect-diff"
	TaskCommentMR   TaskID = "comment-mr"
)

// Task describes one prompt and its outcome grader.
type Task struct {
	ID          TaskID
	Description string
	Prompt      func(*Fixture) string
	Grade       func(*Fixture, AgentResult) Grade
}

// Grade is the execution-grade result for a trial.
type Grade struct {
	Passed     bool     `json:"passed"`
	Assertions []string `json:"assertions"`
	Failures   []string `json:"failures,omitempty"`
}

// Usage is the provider-reported usage available in an agent event stream.
type Usage struct {
	InputTokens         int64   `json:"input_tokens,omitempty"`
	CachedInputTokens   int64   `json:"cached_input_tokens,omitempty"`
	CacheCreationTokens int64   `json:"cache_creation_input_tokens,omitempty"`
	OutputTokens        int64   `json:"output_tokens,omitempty"`
	ReasoningTokens     int64   `json:"reasoning_tokens,omitempty"`
	CostUSD             float64 `json:"cost_usd,omitempty"`
	Turns               int64   `json:"turns,omitempty"`
}

// AgentResult is the normalized result of one Claude Code or Codex run.
type AgentResult struct {
	FinalMessage    string
	RawEvents       []byte
	RawStderr       []byte
	Commands        []string
	Duration        time.Duration
	Usage           Usage
	PolicyViolation string
}

// ResourceLimits is the fixed container profile used for comparable trials.
type ResourceLimits struct {
	Memory      string  `json:"memory"`
	MemorySwap  string  `json:"memory_swap"`
	CPUs        float64 `json:"cpus"`
	PIDs        int     `json:"pids"`
	HomeTmpfs   string  `json:"home_tmpfs"`
	NetworkMode string  `json:"network_mode"`
}

// CleanupMetadata records whether disposable trial resources were removed.
type CleanupMetadata struct {
	ContainerRemoved bool   `json:"container_removed,omitempty"`
	WorkspaceRemoved bool   `json:"workspace_removed,omitempty"`
	FixtureRemoved   bool   `json:"fixture_removed,omitempty"`
	Retained         bool   `json:"retained,omitempty"`
	Error            string `json:"error,omitempty"`
}

// DockerRuntimeMetadata identifies the Docker engine used for a trial.
type DockerRuntimeMetadata struct {
	Context            string `json:"context,omitempty"`
	ClientVersion      string `json:"client_version,omitempty"`
	ServerVersion      string `json:"server_version,omitempty"`
	ServerOS           string `json:"server_os,omitempty"`
	ServerArchitecture string `json:"server_architecture,omitempty"`
}

// ImageMetadata identifies the immutable image and binaries used by a trial.
type ImageMetadata struct {
	Ref            string            `json:"ref,omitempty"`
	ID             string            `json:"id,omitempty"`
	RepoDigests    []string          `json:"repo_digests,omitempty"`
	OS             string            `json:"os,omitempty"`
	Architecture   string            `json:"architecture,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	AgentVersion   string            `json:"agent_version,omitempty"`
	AdapterVersion string            `json:"adapter_version,omitempty"`
	AdapterSHA256  string            `json:"adapter_sha256,omitempty"`
}

// RuntimeMetadata is persisted without environment values or raw Docker
// inspection data so a trial can be explained without leaking credentials.
type RuntimeMetadata struct {
	Isolation          string                `json:"isolation"`
	Docker             DockerRuntimeMetadata `json:"docker,omitempty"`
	Image              ImageMetadata         `json:"image,omitempty"`
	ContainerID        string                `json:"container_id,omitempty"`
	ContainerName      string                `json:"container_name,omitempty"`
	Resources          ResourceLimits        `json:"resources,omitempty"`
	StartedAt          time.Time             `json:"started_at,omitempty"`
	FinishedAt         time.Time             `json:"finished_at,omitempty"`
	ContainerStartupMS int64                 `json:"container_startup_ms,omitempty"`
	ExitCode           *int                  `json:"exit_code,omitempty"`
	OOMKilled          bool                  `json:"oom_killed,omitempty"`
	TimedOut           bool                  `json:"timed_out,omitempty"`
	Canceled           bool                  `json:"canceled,omitempty"`
	Cleanup            CleanupMetadata       `json:"cleanup"`
}

// TrialResult is persisted as one JSON object per trial.
type TrialResult struct {
	RunID           string          `json:"run_id"`
	TaskID          TaskID          `json:"task_id"`
	Trial           int             `json:"trial"`
	Agent           string          `json:"agent"`
	Model           string          `json:"model"`
	Effort          string          `json:"effort"`
	Tool            string          `json:"tool"`
	ToolVersion     string          `json:"tool_version,omitempty"`
	HelperPath      string          `json:"helper_path,omitempty"`
	HelperSHA256    string          `json:"helper_sha256,omitempty"`
	HelperBytes     int             `json:"helper_bytes,omitempty"`
	PromptSHA256    string          `json:"prompt_sha256,omitempty"`
	Project         string          `json:"project"`
	MergeRequestIID int64           `json:"merge_request_iid"`
	StartedAt       time.Time       `json:"started_at"`
	DurationMS      int64           `json:"duration_ms"`
	Runtime         RuntimeMetadata `json:"runtime"`
	Usage           Usage           `json:"usage"`
	Commands        []string        `json:"commands,omitempty"`
	FinalMessage    string          `json:"final_message,omitempty"`
	Grade           Grade           `json:"grade"`
	PolicyViolation string          `json:"policy_violation,omitempty"`
	Error           string          `json:"error,omitempty"`
	TracePath       string          `json:"trace_path,omitempty"`
	StderrPath      string          `json:"stderr_path,omitempty"`
}
