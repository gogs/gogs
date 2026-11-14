package web

import (
	"net/http"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

// gitHooksDir is where the hooks that users edit live in a repository, apart
// from the hooks that Gogs installs itself.
const gitHooksDir = "custom_hooks"

// withRepoGitHooks lets through the users who may edit Git hooks, which run
// arbitrary code on the server: site admins and the users they allow.
func withRepoGitHooks(c flamego.Context, user *database.User, repoCtx *repoContext) {
	w := c.ResponseWriter()
	if !user.CanEditGitHook() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("page does not exist"))
		return
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("withRepoGitHooks: open repository %d: %v", repoCtx.Repo.ID, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}
	c.Map(gitRepo)
}

type repoGitHook struct {
	Name string `json:"name"`
	// IsActive is false when the hook only has its sample content.
	IsActive bool `json:"isActive"`
}

type repoGitHooks struct {
	Items []repoGitHook `json:"items"`
}

func getRepoGitHooks(gitRepo *git.Repository) (statusCode int, resp *repoGitHooks, err error) {
	hooks, err := gitRepo.Hooks(gitHooksDir)
	if err != nil {
		log.Error("getRepoGitHooks: list hooks of %q: %v", gitRepo.Path(), err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list hooks")
	}
	items := make([]repoGitHook, 0, len(hooks))
	for _, h := range hooks {
		items = append(items, repoGitHook{Name: string(h.Name()), IsActive: !h.IsSample()})
	}
	return http.StatusOK, &repoGitHooks{Items: items}, nil
}

// withRepoGitHook injects the server-side hook named in the route.
func withRepoGitHook(c flamego.Context, gitRepo *git.Repository) {
	w := c.ResponseWriter()
	name := git.HookName(c.Param("name"))
	if !slices.Contains(git.ServerSideHooks, name) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("hook does not exist"))
		return
	}
	hook, err := gitRepo.Hook(gitHooksDir, name)
	if err != nil {
		log.Error("withRepoGitHook: get hook %q of %q: %v", name, gitRepo.Path(), err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get hook"))
		return
	}
	c.Map(hook)
}

type repoGitHookContent struct {
	Name     string `json:"name"`
	IsActive bool   `json:"isActive"`
	Content  string `json:"content"`
}

func getRepoGitHook(hook *git.Hook) (statusCode int, resp *repoGitHookContent, err error) {
	return http.StatusOK, &repoGitHookContent{Name: string(hook.Name()), IsActive: !hook.IsSample(), Content: hook.Content()}, nil
}

type repoGitHookRequest struct {
	Content string `json:"content"`
}

// putRepoGitHook saves the content of the hook. Empty content leaves the
// hook in place but makes it do nothing.
func putRepoGitHook(hook *git.Hook, req repoGitHookRequest) (statusCode int, resp any, err error) {
	if err = hook.Update(req.Content); err != nil {
		log.Error("putRepoGitHook: update hook %q: %v", hook.Path(), err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update hook")
	}
	return http.StatusNoContent, nil, nil
}
