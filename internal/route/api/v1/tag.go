package v1

import "gogs.io/gogs/internal/route/api/v1/types"

type tag struct {
	Name   string                      `json:"name"`
	Commit *types.WebhookPayloadCommit `json:"commit"`
}
