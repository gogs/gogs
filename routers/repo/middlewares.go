package repo

import (
	"fmt"

	"github.com/gogits/git-module"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
)

func setEditorconfigIfExists(ctx *context.Context) {
	ec, err := ctx.Repo.GetEditorconfig()

	if err != nil && !git.IsErrNotExist(err) {
		description := fmt.Sprintf("Error while getting .editorconfig file: %v", err)
		if err := models.CreateRepositoryNotice(description); err != nil {
			ctx.Handle(500, "ErrCreatingReporitoryNotice", err)
		}
		return
	}

	ctx.Data["Editorconfig"] = ec
}
