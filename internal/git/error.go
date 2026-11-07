package git

import (
	"errors"
)

var (
	ErrParentNotExist       = errors.New("parent does not exist")
	ErrSubmoduleNotExist    = errors.New("submodule does not exist")
	ErrRevisionNotExist     = errors.New("revision does not exist")
	ErrRemoteNotExist       = errors.New("remote does not exist")
	ErrURLNotExist          = errors.New("URL does not exist")
	ErrExecTimeout          = errors.New("execution was timed out")
	ErrNoMergeBase          = errors.New("no merge based was found")
	ErrNotBlob              = errors.New("the entry is not a blob")
	ErrNotDeleteNonPushURLs = errors.New("will not delete all non-push URLs")
)
