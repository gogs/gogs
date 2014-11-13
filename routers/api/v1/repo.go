// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"fmt"
	"path"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

type ApiPermission struct {
	Admin bool `json:"admin"`
	Push  bool `json:"push"`
	Pull  bool `json:"pull"`
}

type ApiRepository struct {
	Id          int64         `json:"id"`
	Owner       ApiUser       `json:"owner"`
	FullName    string        `json:"full_name"`
	Private     bool          `json:"private"`
	Fork        bool          `json:"fork"`
	HtmlUrl     string        `json:"html_url"`
	CloneUrl    string        `json:"clone_url"`
	SshUrl      string        `json:"ssh_url"`
	Permissions ApiPermission `json:"permissions"`
}

func SearchRepos(ctx *middleware.Context) {
	opt := models.SearchOption{
		Keyword: path.Base(ctx.Query("q")),
		Uid:     com.StrTo(ctx.Query("uid")).MustInt64(),
		Limit:   com.StrTo(ctx.Query("limit")).MustInt(),
	}
	if opt.Limit == 0 {
		opt.Limit = 10
	}

	// Check visibility.
	if ctx.IsSigned && opt.Uid > 0 {
		if ctx.User.Id == opt.Uid {
			opt.Private = true
		} else {
			u, err := models.GetUserById(opt.Uid)
			if err != nil {
				ctx.JSON(500, map[string]interface{}{
					"ok":    false,
					"error": err.Error(),
				})
				return
			}
			if u.IsOrganization() && u.IsOrgOwner(ctx.User.Id) {
				opt.Private = true
			}
			// FIXME: how about collaborators?
		}
	}

	repos, err := models.SearchRepositoryByName(opt)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	results := make([]*ApiRepository, len(repos))
	for i := range repos {
		if err = repos[i].GetOwner(); err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		results[i] = &ApiRepository{
			Id:       repos[i].Id,
			FullName: path.Join(repos[i].Owner.Name, repos[i].Name),
		}
	}

	ctx.Render.JSON(200, map[string]interface{}{
		"ok":   true,
		"data": results,
	})
}

func Migrate(ctx *middleware.Context, form auth.MigrateRepoForm) {
	u, err := models.GetUserByName(ctx.Query("username"))
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	if !u.ValidtePassword(ctx.Query("password")) {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": "username or password is not correct",
		})
		return
	}

	ctxUser := u
	// Not equal means current user is an organization.
	if form.Uid != u.Id {
		org, err := models.GetUserById(form.Uid)
		if err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		ctxUser = org
	}

	if ctx.HasError() {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": ctx.GetErrMsg(),
		})
		return
	}

	if ctxUser.IsOrganization() {
		// Check ownership of organization.
		if !ctxUser.IsOrgOwner(u.Id) {
			ctx.JSON(403, map[string]interface{}{
				"ok":    false,
				"error": "given user is not owner of organization",
			})
			return
		}
	}

	authStr := strings.Replace(fmt.Sprintf("://%s:%s",
		form.AuthUserName, form.AuthPasswd), "@", "%40", -1)
	url := strings.Replace(form.HttpsUrl, "://", authStr+"@", 1)
	repo, err := models.MigrateRepository(ctxUser, form.RepoName, form.Description, form.Private,
		form.Mirror, url)
	if err == nil {
		log.Trace("Repository migrated: %s/%s", ctxUser.Name, form.RepoName)
		ctx.JSON(200, map[string]interface{}{
			"ok":   true,
			"data": "/" + ctxUser.Name + "/" + form.RepoName,
		})
		return
	}

	if repo != nil {
		if errDelete := models.DeleteRepository(ctxUser.Id, repo.Id, ctxUser.Name); errDelete != nil {
			log.Error(4, "DeleteRepository: %v", errDelete)
		}
	}

	ctx.JSON(500, map[string]interface{}{
		"ok":    false,
		"error": err.Error(),
	})
}

// /user/repos: https://developer.github.com/v3/repos/#list-your-repositories
func ListMyRepos(ctx *middleware.Context) {
	if !ctx.IsSigned {
		ctx.Error(403)
		return
	}

	ownRepos, err := models.GetRepositories(ctx.User.Id, true)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	numOwnRepos := len(ownRepos)

	collaRepos, err := models.GetCollaborativeRepos(ctx.User.Name)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	sshUrlFmt := "%s@%s:%s/%s.git"
	if setting.SshPort != 22 {
		sshUrlFmt = "ssh://%s@%s:%d/%s/%s.git"
	}

	repos := make([]*ApiRepository, numOwnRepos+len(collaRepos))
	// FIXME: make only one loop
	for i := range ownRepos {
		repos[i] = &ApiRepository{
			Id: ownRepos[i].Id,
			Owner: ApiUser{
				Id:        ctx.User.Id,
				UserName:  ctx.User.Name,
				AvatarUrl: string(setting.Protocol) + ctx.User.AvatarLink(),
			},
			FullName:    ctx.User.Name + "/" + ownRepos[i].Name,
			Private:     ownRepos[i].IsPrivate,
			Fork:        ownRepos[i].IsFork,
			HtmlUrl:     setting.AppUrl + ctx.User.Name + "/" + ownRepos[i].Name,
			SshUrl:      fmt.Sprintf(sshUrlFmt, setting.RunUser, setting.Domain, ctx.User.LowerName, ownRepos[i].LowerName),
			Permissions: ApiPermission{true, true, true},
		}
		repos[i].CloneUrl = repos[i].HtmlUrl + ".git"
	}
	for i := range collaRepos {
		if err = collaRepos[i].GetOwner(); err != nil {
			ctx.JSON(500, map[string]interface{}{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}
		j := i + numOwnRepos
		repos[j] = &ApiRepository{
			Id: collaRepos[i].Id,
			Owner: ApiUser{
				Id:        collaRepos[i].Owner.Id,
				UserName:  collaRepos[i].Owner.Name,
				AvatarUrl: string(setting.Protocol) + collaRepos[i].Owner.AvatarLink(),
			},
			FullName:    collaRepos[i].Owner.Name + "/" + collaRepos[i].Name,
			Private:     collaRepos[i].IsPrivate,
			Fork:        collaRepos[i].IsFork,
			HtmlUrl:     setting.AppUrl + collaRepos[i].Owner.Name + "/" + collaRepos[i].Name,
			SshUrl:      fmt.Sprintf(sshUrlFmt, setting.RunUser, setting.Domain, collaRepos[i].Owner.LowerName, collaRepos[i].LowerName),
			Permissions: ApiPermission{false, collaRepos[i].CanPush, true},
		}
		repos[j].CloneUrl = repos[j].HtmlUrl + ".git"

		// FIXME: cache result to reduce DB query?
		if collaRepos[i].Owner.IsOrganization() && collaRepos[i].Owner.IsOrgOwner(ctx.User.Id) {
			repos[j].Permissions.Admin = true
		}
	}

	ctx.JSON(200, &repos)
}
