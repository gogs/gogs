package v1

import "gogs.io/gogs/internal/api/v1types"

type tag struct {
	Name   string                        `json:"name"`
	Commit *v1types.WebhookPayloadCommit `json:"commit"`
}
