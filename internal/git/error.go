package git

import (
	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/errx"
)

var (
	ErrParentNotExist    = errors.New("parent does not exist")
	ErrSubmoduleNotExist = errors.New("submodule does not exist")
	ErrRevisionNotExist  = errors.New("revision does not exist")
	ErrRemoteNotExist    = errors.New("remote does not exist")
	ErrExecTimeout       = errors.New("execution was timed out")
	ErrNoMergeBase       = errors.New("no merge based was found")
	ErrNotBlob           = errors.New("the entry is not a blob")
	ErrNotSymbolicRef    = errors.New("the reference is not a symbolic reference")
)

var _ errx.NotFound = (*Error)(nil)

// Error is a wrapper of a Git error, which handles not found.
type Error struct {
	error
}

func (e Error) NotFound() bool {
	return errors.Is(e.error, ErrSubmoduleNotExist) ||
		IsErrRevisionNotExist(e.error)
}

// NewError wraps given error.
func NewError(err error) error {
	return Error{error: err}
}

// IsErrRevisionNotExist returns true if the error is ErrRevisionNotExist.
func IsErrRevisionNotExist(err error) bool {
	return errors.Is(err, ErrRevisionNotExist)
}

// IsErrNoMergeBase returns true if the error is ErrNoMergeBase.
func IsErrNoMergeBase(err error) bool {
	return errors.Is(err, ErrNoMergeBase)
}
