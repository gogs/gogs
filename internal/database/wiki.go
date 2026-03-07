package database

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/sync"
)

var wikiWorkingPool = sync.NewExclusivePool()

// WikiBranch returns the branch name used by the wiki repository. It checks if
// "main" branch exists, otherwise falls back to "master".
func WikiBranch(repoPath string) string {
	if git.RepoHasBranch(repoPath, "main") {
		return "main"
	}
	return "master"
}

// ToWikiPageURL formats a string to corresponding wiki URL name.
func ToWikiPageURL(name string) string {
	return url.QueryEscape(name)
}

// ToWikiPageName formats a URL back to corresponding wiki page name. It enforces
// single-level hierarchy by replacing all "/" with spaces.
func ToWikiPageName(urlString string) string {
	name, _ := url.QueryUnescape(urlString)
	name = pathx.Clean(name)
	return strings.ReplaceAll(name, "/", " ")
}

// WikiCloneLink returns clone URLs of repository wiki.
//
// Deprecated: Use repox.NewCloneLink instead.
func (r *Repository) WikiCloneLink() (cl *repox.CloneLink) {
	return r.cloneLink(true)
}

// WikiPath returns wiki data path by given user and repository name.
func WikiPath(userName, repoName string) string {
	return filepath.Join(repox.UserPath(userName), strings.ToLower(repoName)+".wiki.git")
}

func (r *Repository) WikiPath() string {
	return WikiPath(r.MustOwner().Name, r.Name)
}

// HasWiki returns true if repository has wiki.
func (r *Repository) HasWiki() bool {
	return osx.IsDir(r.WikiPath())
}

// InitWiki initializes a wiki for repository,
// it does nothing when repository already has wiki.
func (r *Repository) InitWiki() error {
	if r.HasWiki() {
		return nil
	}

	if err := git.Init(r.WikiPath(), git.InitOptions{Bare: true}); err != nil {
		return errors.Newf("init repository: %v", err)
	} else if err = createDelegateHooks(r.WikiPath()); err != nil {
		return errors.Newf("createDelegateHooks: %v", err)
	}
	return nil
}

func (r *Repository) LocalWikiPath() string {
	return filepath.Join(conf.Server.AppDataPath, "tmp", "local-wiki", strconv.FormatInt(r.ID, 10))
}

// UpdateLocalWiki makes sure the local copy of repository wiki is up-to-date.
func (r *Repository) UpdateLocalWiki() error {
	wikiPath := r.WikiPath()
	return UpdateLocalCopyBranch(wikiPath, r.LocalWikiPath(), WikiBranch(wikiPath), true)
}

func discardLocalWikiChanges(localPath string) error {
	return discardLocalRepoBranchChanges(localPath, WikiBranch(localPath))
}

// updateWikiPage adds new page to repository wiki.
func (r *Repository) updateWikiPage(doer *User, oldTitle, title, content, message string, isNew bool) error {
	wikiWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer wikiWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	if err := r.InitWiki(); err != nil {
		return errors.Newf("InitWiki: %v", err)
	}

	localPath := r.LocalWikiPath()
	if err := discardLocalWikiChanges(localPath); err != nil {
		return errors.Newf("discardLocalWikiChanges: %v", err)
	} else if err = r.UpdateLocalWiki(); err != nil {
		return errors.Newf("UpdateLocalWiki: %v", err)
	}

	title = ToWikiPageName(title)
	filename := path.Join(localPath, title+".md")

	// If not a new file, show perform update not create.
	if isNew {
		if osx.Exist(filename) {
			return ErrWikiAlreadyExist{filename}
		}
	} else {
		oldTitle = ToWikiPageName(oldTitle)
		_ = os.Remove(path.Join(localPath, oldTitle+".md"))
	}

	// SECURITY: if new file is a symlink to non-exist critical file,
	// attack content can be written to the target file (e.g. authorized_keys2)
	// as a new page operation.
	// So we want to make sure the symlink is removed before write anything.
	// The new file we created will be in normal text format.
	_ = os.Remove(filename)

	if err := os.WriteFile(filename, []byte(content), 0o666); err != nil {
		return errors.Newf("WriteFile: %v", err)
	}

	if message == "" {
		message = "Update page '" + title + "'"
	}
	if err := git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return errors.Newf("add all changes: %v", err)
	}

	err := git.CreateCommit(
		localPath,
		&git.Signature{
			Name:  doer.DisplayName(),
			Email: doer.Email,
			When:  time.Now(),
		},
		message,
	)
	if err != nil {
		return errors.Newf("commit changes: %v", err)
	} else if err = git.Push(localPath, "origin", WikiBranch(localPath)); err != nil {
		return errors.Newf("push: %v", err)
	}

	return nil
}

func (r *Repository) AddWikiPage(doer *User, title, content, message string) error {
	return r.updateWikiPage(doer, "", title, content, message, true)
}

func (r *Repository) EditWikiPage(doer *User, oldTitle, title, content, message string) error {
	return r.updateWikiPage(doer, oldTitle, title, content, message, false)
}

func (r *Repository) DeleteWikiPage(doer *User, title string) (err error) {
	wikiWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer wikiWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	localPath := r.LocalWikiPath()
	if err = discardLocalWikiChanges(localPath); err != nil {
		return errors.Newf("discardLocalWikiChanges: %v", err)
	} else if err = r.UpdateLocalWiki(); err != nil {
		return errors.Newf("UpdateLocalWiki: %v", err)
	}

	title = ToWikiPageName(title)
	_ = os.Remove(path.Join(localPath, title+".md"))

	message := "Delete page '" + title + "'"

	if err = git.Add(localPath, git.AddOptions{All: true}); err != nil {
		return errors.Newf("add all changes: %v", err)
	}

	err = git.CreateCommit(
		localPath,
		&git.Signature{
			Name:  doer.DisplayName(),
			Email: doer.Email,
			When:  time.Now(),
		},
		message,
	)
	if err != nil {
		return errors.Newf("commit changes: %v", err)
	} else if err = git.Push(localPath, "origin", WikiBranch(localPath)); err != nil {
		return errors.Newf("push: %v", err)
	}

	return nil
}
