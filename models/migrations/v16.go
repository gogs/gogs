// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/modules/setting"
)

func updateRepositorySizes(x *xorm.Engine) (err error) {
	type Repository struct {
		ID      int64
		OwnerID int64
		Name    string
		Size    int64
	}
	type User struct {
		ID   int64
		Name string
	}
	if err = x.Sync2(new(Repository)); err != nil {
		return fmt.Errorf("Sync2: %v", err)
	}
	return x.Where("id > 0").Iterate(new(Repository),
		func(idx int, bean interface{}) error {
			repo := bean.(*Repository)
			if repo.Name == "." || repo.Name == ".." {
				return nil
			}

			user := new(User)
			has, err := x.Where("id = ?", repo.OwnerID).Get(user)
			if err != nil {
				return fmt.Errorf("query owner of repository [repo_id: %d, owner_id: %d]: %v", repo.ID, repo.OwnerID, err)
			} else if !has {
				return nil
			}

			repoPath := filepath.Join(setting.RepoRootPath, strings.ToLower(user.Name), strings.ToLower(repo.Name)) + ".git"
			log.Trace("[%04d]: %s", idx, repoPath)

			countObject, err := git.GetRepoSize(repoPath)
			if err != nil {
				log.Warn("GetRepoSize: %v", err)
				return nil
			}

			repo.Size = countObject.Size + countObject.SizePack
			if _, err = x.Id(repo.ID).Cols("size").Update(repo); err != nil {
				return fmt.Errorf("update size: %v", err)
			}
			return nil
		})
}
