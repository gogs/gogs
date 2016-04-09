// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Unknwon/com"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/modules/setting"
)

// workingPool represents a pool of working status which makes sure
// that only one instance of same task is performing at a time.
// However, different type of tasks can performing at the same time.
type workingPool struct {
	lock  sync.Mutex
	pool  map[string]*sync.Mutex
	count map[string]int
}

// CheckIn checks in a task and waits if others are running.
func (p *workingPool) CheckIn(name string) {
	p.lock.Lock()

	lock, has := p.pool[name]
	if !has {
		lock = &sync.Mutex{}
		p.pool[name] = lock
	}
	p.count[name]++

	p.lock.Unlock()
	lock.Lock()
}

// CheckOut checks out a task to let other tasks run.
func (p *workingPool) CheckOut(name string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool[name].Unlock()
	if p.count[name] == 1 {
		delete(p.pool, name)
		delete(p.count, name)
	} else {
		p.count[name]--
	}
}

var wikiWorkingPool = &workingPool{
	pool:  make(map[string]*sync.Mutex),
	count: make(map[string]int),
}

// ToWikiPageURL formats a string to corresponding wiki URL name.
func ToWikiPageURL(name string) string {
	return url.QueryEscape(strings.Replace(name, " ", "-", -1))
}

// ToWikiPageName formats a URL back to corresponding wiki page name.
func ToWikiPageName(urlString string) string {
	name, _ := url.QueryUnescape(strings.Replace(urlString, "-", " ", -1))
	return name
}

// WikiCloneLink returns clone URLs of repository wiki.
func (repo *Repository) WikiCloneLink() (cl *CloneLink) {
	return repo.cloneLink(true)
}

// WikiPath returns wiki data path by given user and repository name.
func WikiPath(userName, repoName string) string {
	return filepath.Join(UserPath(userName), strings.ToLower(repoName)+".wiki.git")
}

func (repo *Repository) WikiPath() string {
	return WikiPath(repo.MustOwner().Name, repo.Name)
}

// HasWiki returns true if repository has wiki.
func (repo *Repository) HasWiki() bool {
	return com.IsDir(repo.WikiPath())
}

// InitWiki initializes a wiki for repository,
// it does nothing when repository already has wiki.
func (repo *Repository) InitWiki() error {
	if repo.HasWiki() {
		return nil
	}

	if err := git.InitRepository(repo.WikiPath(), true); err != nil {
		return fmt.Errorf("InitRepository: %v", err)
	}
	return nil
}

func (repo *Repository) LocalWikiPath() string {
	return path.Join(setting.AppDataPath, "tmp/local-wiki", com.ToStr(repo.ID))
}

// UpdateLocalWiki makes sure the local copy of repository wiki is up-to-date.
func (repo *Repository) UpdateLocalWiki() error {
	return updateLocalCopy(repo.WikiPath(), repo.LocalWikiPath())
}

// discardLocalWikiChanges discards local commits make sure
// it is even to remote branch when local copy exists.
func discardLocalWikiChanges(localPath string) error {
	if !com.IsExist(localPath) {
		return nil
	}
	// No need to check if nothing in the repository.
	if !git.IsBranchExist(localPath, "master") {
		return nil
	}

	if err := git.ResetHEAD(localPath, true, "origin/master"); err != nil {
		return fmt.Errorf("ResetHEAD: %v", err)
	}
	return nil
}

// updateWikiPage adds new page to repository wiki.
func (repo *Repository) updateWikiPage(doer *User, oldTitle, title, content, message string, isNew bool) (err error) {
	wikiWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer wikiWorkingPool.CheckOut(com.ToStr(repo.ID))

	if err = repo.InitWiki(); err != nil {
		return fmt.Errorf("InitWiki: %v", err)
	}

	localPath := repo.LocalWikiPath()
	if err = discardLocalWikiChanges(localPath); err != nil {
		return fmt.Errorf("discardLocalWikiChanges: %v", err)
	} else if err = repo.UpdateLocalWiki(); err != nil {
		return fmt.Errorf("UpdateLocalWiki: %v", err)
	}

	title = ToWikiPageName(strings.Replace(title, "/", " ", -1))
	filename := path.Join(localPath, title+".md")

	// If not a new file, show perform update not create.
	if isNew {
		if com.IsExist(filename) {
			return ErrWikiAlreadyExist{filename}
		}
	} else {
		os.Remove(path.Join(localPath, oldTitle+".md"))
	}

	if err = ioutil.WriteFile(filename, []byte(content), 0666); err != nil {
		return fmt.Errorf("WriteFile: %v", err)
	}

	if len(message) == 0 {
		message = "Update page '" + title + "'"
	}
	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("AddChanges: %v", err)
	} else if err = git.CommitChanges(localPath, message, doer.NewGitSig()); err != nil {
		return fmt.Errorf("CommitChanges: %v", err)
	} else if err = git.Push(localPath, "origin", "master"); err != nil {
		return fmt.Errorf("Push: %v", err)
	}

	return nil
}

func (repo *Repository) AddWikiPage(doer *User, title, content, message string) error {
	return repo.updateWikiPage(doer, "", title, content, message, true)
}

func (repo *Repository) EditWikiPage(doer *User, oldTitle, title, content, message string) error {
	return repo.updateWikiPage(doer, oldTitle, title, content, message, false)
}

func (repo *Repository) DeleteWikiPage(doer *User, title string) (err error) {
	wikiWorkingPool.CheckIn(com.ToStr(repo.ID))
	defer wikiWorkingPool.CheckOut(com.ToStr(repo.ID))

	localPath := repo.LocalWikiPath()
	if err = discardLocalWikiChanges(localPath); err != nil {
		return fmt.Errorf("discardLocalWikiChanges: %v", err)
	} else if err = repo.UpdateLocalWiki(); err != nil {
		return fmt.Errorf("UpdateLocalWiki: %v", err)
	}

	title = ToWikiPageName(strings.Replace(title, "/", " ", -1))
	filename := path.Join(localPath, title+".md")
	os.Remove(filename)

	message := "Delete page '" + title + "'"

	if err = git.AddChanges(localPath, true); err != nil {
		return fmt.Errorf("AddChanges: %v", err)
	} else if err = git.CommitChanges(localPath, message, doer.NewGitSig()); err != nil {
		return fmt.Errorf("CommitChanges: %v", err)
	} else if err = git.Push(localPath, "origin", "master"); err != nil {
		return fmt.Errorf("Push: %v", err)
	}

	return nil
}
