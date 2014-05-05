// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	"github.com/gogits/git"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
)

func Setting(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "repo.Setting", nil)
		return
	}

	ctx.Data["IsRepoToolbarSetting"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - settings"
	ctx.HTML(200, "repo/setting")
}

func SettingPost(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	ctx.Data["IsRepoToolbarSetting"] = true

	switch ctx.Query("action") {
	case "update":
		newRepoName := ctx.Query("name")
		// Check if repository name has been changed.
		if ctx.Repo.Repository.Name != newRepoName {
			isExist, err := models.IsRepositoryExist(ctx.Repo.Owner, newRepoName)
			if err != nil {
				ctx.Handle(500, "repo.SettingPost(update: check existence)", err)
				return
			} else if isExist {
				ctx.RenderWithErr("Repository name has been taken in your repositories.", "repo/setting", nil)
				return
			} else if err = models.ChangeRepositoryName(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName); err != nil {
				ctx.Handle(500, "repo.SettingPost(change repository name)", err)
				return
			}
			log.Trace("%s Repository name changed: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newRepoName)

			ctx.Repo.Repository.Name = newRepoName
		}

		br := ctx.Query("branch")

		if git.IsBranchExist(models.RepoPath(ctx.User.Name, ctx.Repo.Repository.Name), br) {
			ctx.Repo.Repository.DefaultBranch = br
		}
		ctx.Repo.Repository.Description = ctx.Query("desc")
		ctx.Repo.Repository.Website = ctx.Query("site")
		ctx.Repo.Repository.IsPrivate = ctx.Query("private") == "on"
		ctx.Repo.Repository.IsGoget = ctx.Query("goget") == "on"
		if err := models.UpdateRepository(ctx.Repo.Repository); err != nil {
			ctx.Handle(404, "repo.SettingPost(update)", err)
			return
		}
		log.Trace("%s Repository updated: %s/%s", ctx.Req.RequestURI, ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)

		if ctx.Repo.Repository.IsMirror {
			if len(ctx.Query("interval")) > 0 {
				var err error
				ctx.Repo.Mirror.Interval, err = base.StrTo(ctx.Query("interval")).Int()
				if err != nil {
					log.Error("repo.SettingPost(get mirror interval): %v", err)
				} else if err = models.UpdateMirror(ctx.Repo.Mirror); err != nil {
					log.Error("repo.SettingPost(UpdateMirror): %v", err)
				}
			}
		}

		ctx.Flash.Success("Repository options has been successfully updated.")
		ctx.Redirect(fmt.Sprintf("/%s/%s/settings", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
	case "transfer":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		newOwner := ctx.Query("owner")
		// Check if new owner exists.
		isExist, err := models.IsUserExist(newOwner)
		if err != nil {
			ctx.Handle(500, "repo.SettingPost(transfer: check existence)", err)
			return
		} else if !isExist {
			ctx.RenderWithErr("Please make sure you entered owner name is correct.", "repo/setting", nil)
			return
		} else if err = models.TransferOwnership(ctx.User, newOwner, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "repo.SettingPost(transfer repository)", err)
			return
		}
		log.Trace("%s Repository transfered: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newOwner)

		ctx.Redirect("/")
	case "delete":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", "repo/setting", nil)
			return
		}

		if err := models.DeleteRepository(ctx.User.Id, ctx.Repo.Repository.Id, ctx.User.LowerName); err != nil {
			ctx.Handle(500, "repo.Delete", err)
			return
		}
		log.Trace("%s Repository deleted: %s/%s", ctx.Req.RequestURI, ctx.User.LowerName, ctx.Repo.Repository.LowerName)

		ctx.Redirect("/")
	}
}

func Collaboration(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	repoLink := strings.TrimPrefix(ctx.Repo.RepoLink, "/")
	ctx.Data["IsRepoToolbarCollaboration"] = true
	ctx.Data["Title"] = repoLink + " - collaboration"

	// Delete collaborator.
	remove := strings.ToLower(ctx.Query("remove"))
	if len(remove) > 0 && remove != ctx.Repo.Owner.LowerName {
		if err := models.DeleteAccess(&models.Access{UserName: remove, RepoName: repoLink}); err != nil {
			ctx.Handle(500, "repo.Collaboration(DeleteAccess)", err)
			return
		}
		ctx.Flash.Success("Collaborator has been removed.")
		ctx.Redirect(ctx.Repo.RepoLink + "/settings/collaboration")
		return
	}

	names, err := models.GetCollaborators(repoLink)
	if err != nil {
		ctx.Handle(500, "repo.Collaboration(GetCollaborators)", err)
		return
	}

	us := make([]*models.User, len(names))
	for i, name := range names {
		us[i], err = models.GetUserByName(name)
		if err != nil {
			ctx.Handle(500, "repo.Collaboration(GetUserByName)", err)
			return
		}
	}

	ctx.Data["Collaborators"] = us
	ctx.HTML(200, "repo/collaboration")
}

func CollaborationPost(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Error(404)
		return
	}

	repoLink := strings.TrimPrefix(ctx.Repo.RepoLink, "/")
	name := strings.ToLower(ctx.Query("collaborator"))
	if len(name) == 0 || ctx.Repo.Owner.LowerName == name {
		ctx.Redirect(ctx.Req.RequestURI)
		return
	}
	has, err := models.HasAccess(name, repoLink, models.AU_WRITABLE)
	if err != nil {
		ctx.Handle(500, "repo.CollaborationPost(HasAccess)", err)
		return
	} else if has {
		ctx.Redirect(ctx.Req.RequestURI)
		return
	}

	u, err := models.GetUserByName(name)
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Flash.Error("Given user does not exist.")
			ctx.Redirect(ctx.Req.RequestURI)
		} else {
			ctx.Handle(500, "repo.CollaborationPost(GetUserByName)", err)
		}
		return
	}

	if err = models.AddAccess(&models.Access{UserName: name, RepoName: repoLink,
		Mode: models.AU_WRITABLE}); err != nil {
		ctx.Handle(500, "repo.CollaborationPost(AddAccess)", err)
		return
	}

	if base.Service.NotifyMail {
		if err = mailer.SendCollaboratorMail(ctx.Render, u, ctx.User, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "repo.CollaborationPost(SendCollaboratorMail)", err)
			return
		}
	}

	ctx.Flash.Success("New collaborator has been added.")
	ctx.Redirect(ctx.Req.RequestURI)
}

func WebHooks(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "repo.WebHooks", nil)
		return
	}

	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Web Hooks"
	ctx.HTML(200, "repo/hooks")
}

func WebHooksAdd(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "repo.WebHooksAdd", nil)
		return
	}

	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Add Web Hook"
	ctx.HTML(200, "repo/hooks_add")
}

func WebHooksEdit(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner {
		ctx.Handle(404, "repo.WebHooksEdit", nil)
		return
	}

	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Web Hook Name"
	ctx.HTML(200, "repo/hooks_edit")
}
