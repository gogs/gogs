// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/pkg/setting"
)

func generateAndMigrateGitHooks(x *xorm.Engine) (err error) {
	type Repository struct {
		ID      int64
		OwnerID int64
		Name    string
	}
	type User struct {
		ID   int64
		Name string
	}
	var (
		hookNames = []string{"pre-receive", "update", "post-receive"}
		hookTpls  = []string{
			fmt.Sprintf("#!/usr/bin/env %s\n\"%s\" hook --config='%s' pre-receive\n", setting.ScriptType, setting.AppPath, setting.CustomConf),
			fmt.Sprintf("#!/usr/bin/env %s\n\"%s\" hook --config='%s' update $1 $2 $3\n", setting.ScriptType, setting.AppPath, setting.CustomConf),
			fmt.Sprintf("#!/usr/bin/env %s\n\"%s\" hook --config='%s' post-receive\n", setting.ScriptType, setting.AppPath, setting.CustomConf),
		}
	)

	// Cleanup old update.log and http.log files.
	filepath.Walk(setting.LogRootPath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() &&
			(strings.HasPrefix(filepath.Base(path), "update.log") ||
				strings.HasPrefix(filepath.Base(path), "http.log")) {
			os.Remove(path)
		}
		return nil
	})

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

			repoBase := filepath.Join(setting.RepoRootPath, strings.ToLower(user.Name), strings.ToLower(repo.Name))
			repoPath := repoBase + ".git"
			wikiPath := repoBase + ".wiki.git"
			log.Trace("[%04d]: %s", idx, repoPath)

			// Note: we should not create hookDir here because update hook file should already exists inside this direcotry,
			// if this directory does not exist, the current setup is not correct anyway.
			hookDir := filepath.Join(repoPath, "hooks")
			customHookDir := filepath.Join(repoPath, "custom_hooks")
			wikiHookDir := filepath.Join(wikiPath, "hooks")

			for i, hookName := range hookNames {
				oldHookPath := filepath.Join(hookDir, hookName)
				newHookPath := filepath.Join(customHookDir, hookName)

				// Gogs didn't allow user to set custom update hook thus no migration for it.
				// In case user runs this migration multiple times, and custom hook exists,
				// we assume it's been migrated already.
				if hookName != "update" && com.IsFile(oldHookPath) && !com.IsExist(customHookDir) {
					os.MkdirAll(customHookDir, os.ModePerm)
					if err = os.Rename(oldHookPath, newHookPath); err != nil {
						return fmt.Errorf("move hook file to custom directory '%s' -> '%s': %v", oldHookPath, newHookPath, err)
					}
				}

				if err = ioutil.WriteFile(oldHookPath, []byte(hookTpls[i]), os.ModePerm); err != nil {
					return fmt.Errorf("write hook file '%s': %v", oldHookPath, err)
				}

				if com.IsDir(wikiPath) {
					os.MkdirAll(wikiHookDir, os.ModePerm)
					wikiHookPath := filepath.Join(wikiHookDir, hookName)
					if err = ioutil.WriteFile(wikiHookPath, []byte(hookTpls[i]), os.ModePerm); err != nil {
						return fmt.Errorf("write wiki hook file '%s': %v", wikiHookPath, err)
					}
				}
			}
			return nil
		})
}
