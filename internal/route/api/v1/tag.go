package v1

import (
	"fmt"

	"gogs.io/gogs/internal/route/api/v1/types"
)

type tag struct {
	Name   string                      `json:"name"`
	Commit *types.WebhookPayloadCommit `json:"commit"`
}

type errTagAlreadyExists string

func (e errTagAlreadyExists) Error() string {
	return fmt.Sprintf("tag already exists [name: %s]", string(e))
}
