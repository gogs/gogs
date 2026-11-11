package v1

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	routerepo "gogs.io/gogs/internal/route/repo"
)

func getRawFile(c *context.APIContext) {
	if !c.Repo.HasAccess() {
		c.NotFound()
		return
	}

	if c.Repo.Repository.IsEmpty {
		c.NotFound()
		return
	}

	blob, err := c.Repo.Commit.Blob(c.Req.Context(), c.Repo.TreePath)
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get blob")
		return
	}
	if err = routerepo.ServeBlob(c.Context, blob); err != nil {
		c.Error(err, "serve blob")
	}
}

func getArchive(c *context.APIContext) {
	repoPath := database.RepoPath(c.Params(":username"), c.Params(":reponame"))
	gitRepo, err := git.Open(repoPath)
	if err != nil {
		c.Error(err, "open repository")
		return
	}
	c.Repo.GitRepo = gitRepo

	routerepo.Download(c.Context)
}

func getEditorconfig(c *context.APIContext) {
	ec, err := c.Repo.Editorconfig(c.Req.Context())
	if err != nil {
		c.NotFoundOrError(git.NewError(err), "get .editorconfig")
		return
	}

	fileName := c.Params("filename")
	def, err := ec.GetDefinitionForFilename(fileName)
	if err != nil {
		c.Error(err, "get definition for filename")
		return
	}
	if def == nil {
		c.NotFound()
		return
	}
	c.JSONSuccess(def)
}
