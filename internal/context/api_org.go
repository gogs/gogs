package context

import (
	"gogs.io/gogs/internal/database"
)

type APIOrganization struct {
	Organization *database.User
	Team         *database.Team
}
