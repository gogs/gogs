package repo

import (
	"gogs.io/gogs/internal/context"
)

func MustEnableWiki(c *context.Context) {
	if !c.Repo.Repository.EnableWiki {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalWiki {
		c.Redirect(c.Repo.Repository.ExternalWikiURL)
		return
	}
}
