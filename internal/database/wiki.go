package database

import (
	"context"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/sync"
)

var wikiWorkingPool = sync.NewExclusivePool()

// WikiBranch returns the branch name used by the wiki repository. It checks if
// "main" branch exists, otherwise falls back to "master".
func WikiBranch(ctx context.Context, repoPath string) string {
	if git.RepoHasBranch(ctx, repoPath, "main") {
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
	// 🚨 SECURITY: Prevent path traversal in repository name.
	return filepath.Join(repox.UserPath(userName), pathx.Clean(strings.ToLower(repoName))+".wiki.git")
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
func (r *Repository) InitWiki(ctx context.Context) error {
	// Stopping midway, e.g., when the client disconnects, would leave a
	// partial change behind.
	ctx = context.WithoutCancel(ctx)

	if r.HasWiki() {
		return nil
	}

	if err := git.Init(ctx, r.WikiPath(), git.InitBare()); err != nil {
		return errors.Newf("init repository: %v", err)
	} else if err = createDelegateHooks(r.WikiPath()); err != nil {
		return errors.Newf("createDelegateHooks: %v", err)
	}
	return nil
}

// wikiParent returns the branch that wiki pages are committed to and its
// latest commit, which is nil when the wiki has no pages yet.
func (r *Repository) wikiParent(ctx context.Context) (string, *git.Commit, error) {
	wikiPath := r.WikiPath()
	wikiRepo, err := git.Open(wikiPath)
	if err != nil {
		return "", nil, errors.Wrap(err, "open wiki repository")
	}

	branch := WikiBranch(ctx, wikiPath)
	if wikiRepo.HasBranch(ctx, branch) {
		parent, err := wikiRepo.BranchCommit(ctx, branch)
		if err != nil {
			return "", nil, errors.Wrapf(err, "get commit of branch %q", branch)
		}
		return branch, parent, nil
	}

	// The first page goes to the branch that HEAD names, so that cloning the wiki
	// checks it out.
	ref, err := wikiRepo.SymbolicRef(ctx)
	if err == nil && strings.HasPrefix(ref, git.RefsHeads) {
		branch = strings.TrimPrefix(ref, git.RefsHeads)
	}
	return branch, nil, nil
}

// updateWikiPage adds new page to repository wiki.
func (r *Repository) updateWikiPage(ctx context.Context, doer *User, oldTitle, title, content, message string, isNew bool) error {
	wikiWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer wikiWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	if err := r.InitWiki(ctx); err != nil {
		return errors.Newf("InitWiki: %v", err)
	}

	branch, parent, err := r.wikiParent(ctx)
	if err != nil {
		return err
	}

	title = ToWikiPageName(title)
	filename := title + ".md"
	if message == "" {
		message = "Update page '" + title + "'"
	}
	sig := &git.Signature{Name: doer.DisplayName(), Email: doer.Email}
	return commitAndPush(ctx, r.WikiPath(), parent, branch, message, sig, nil, func(tmpRepo *git.Repository) error {
		if parent != nil {
			if isNew {
				entry, err := treeEntry(ctx, parent, filename)
				if err != nil {
					return err
				} else if entry != nil {
					return ErrWikiAlreadyExist{filename}
				}
			} else {
				oldFilename := ToWikiPageName(oldTitle) + ".md"
				entry, err := treeEntry(ctx, parent, oldFilename)
				if err != nil {
					return err
				} else if entry != nil && !entry.IsTree() {
					if err = tmpRepo.RemoveFromIndex(ctx, oldFilename); err != nil {
						return errors.Wrapf(err, "remove %q", oldFilename)
					}
				}
			}
		}

		err := tmpRepo.AddToIndex(ctx, git.IndexFile{Path: filename, Content: strings.NewReader(content)})
		return errors.Wrapf(err, "add %q", filename)
	})
}

func (r *Repository) AddWikiPage(ctx context.Context, doer *User, title, content, message string) error {
	return r.updateWikiPage(ctx, doer, "", title, content, message, true)
}

func (r *Repository) EditWikiPage(ctx context.Context, doer *User, oldTitle, title, content, message string) error {
	return r.updateWikiPage(ctx, doer, oldTitle, title, content, message, false)
}

func (r *Repository) DeleteWikiPage(ctx context.Context, doer *User, title string) (err error) {
	wikiWorkingPool.CheckIn(strconv.FormatInt(r.ID, 10))
	defer wikiWorkingPool.CheckOut(strconv.FormatInt(r.ID, 10))

	branch, parent, err := r.wikiParent(ctx)
	if err != nil {
		return err
	} else if parent == nil {
		return errors.New("wiki has no pages")
	}

	title = ToWikiPageName(title)
	filename := title + ".md"
	sig := &git.Signature{Name: doer.DisplayName(), Email: doer.Email}
	return commitAndPush(ctx, r.WikiPath(), parent, branch, "Delete page '"+title+"'", sig, nil, func(tmpRepo *git.Repository) error {
		entry, err := treeEntry(ctx, parent, filename)
		if err != nil {
			return err
		} else if entry == nil || entry.IsTree() {
			return nil
		}
		if err = tmpRepo.RemoveFromIndex(ctx, filename); err != nil {
			return errors.Wrapf(err, "remove %q", filename)
		}
		return nil
	})
}
