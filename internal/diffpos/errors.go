package diffpos

import "errors"

// Sentinel errors for position resolution. internal/cli maps them to stable
// axi error codes in classifyError, so exactly one variable must exist per
// sentinel — never redeclare these messages elsewhere.
var (
	ErrDiffNotReady  = errors.New("merge request diff is not ready yet")
	ErrFileNotInDiff = errors.New("file is not part of the merge request diff")
	ErrLineNotInDiff = errors.New("line is not part of the merge request diff")
	ErrDiffTooLarge  = errors.New("merge request diff for this file is collapsed or too large")
)

// hintedError is the diffpos analog of cli's helpError: a runtime error
// carrying next-step hints built at the error site, extracted by cli through
// the HelpHints method. It never changes the exit code.
type hintedError struct {
	err   error
	hints []string
}

func (e *hintedError) Error() string { return e.err.Error() }

func (e *hintedError) Unwrap() error { return e.err }

func (e *hintedError) HelpHints() []string { return e.hints }

func withHints(err error, hints ...string) error {
	return &hintedError{err: err, hints: hints}
}
