package web

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/tool"
)

type repoBranch struct {
	Name          string    `json:"name"`
	IsProtected   bool      `json:"isProtected"`
	CommitID      string    `json:"commitID"`
	CommittedAt   time.Time `json:"committedAt"`
	CommitterName string    `json:"committerName"`
}

type repoBranchesResponse struct {
	DefaultBranch string       `json:"defaultBranch"`
	Branches      []repoBranch `json:"branches"`
	// ViewerCanWrite offers pull requests from the branches, when the
	// repository allows them.
	ViewerCanWrite bool `json:"viewerCanWrite"`
	AllowsPulls    bool `json:"allowsPulls"`
	// ViewerCanChangeDefault links the settings that change the default
	// branch, which mirrors do not have.
	ViewerCanChangeDefault bool `json:"viewerCanChangeDefault"`
}

// protectedBranchNames returns the names of the protected branches of the
// repository.
func protectedBranchNames(repoID int64) (map[string]bool, error) {
	protected, err := database.GetProtectBranchesByRepoID(repoID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(protected))
	for _, p := range protected {
		names[p.Name] = true
	}
	return names, nil
}

func getRepoBranches(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoBranchesResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoBranches: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	names, err := gitRepo.Branches(ctx)
	if err != nil {
		log.Error("getRepoBranches: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	protected, err := protectedBranchNames(repo.ID)
	if err != nil {
		log.Error("getRepoBranches: list protected branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list protected branches")
	}

	resp = &repoBranchesResponse{
		DefaultBranch:          repo.DefaultBranch,
		Branches:               make([]repoBranch, 0, len(names)),
		ViewerCanWrite:         repoCtx.Viewer.CanWrite(),
		AllowsPulls:            repo.AllowsPulls(),
		ViewerCanChangeDefault: repoCtx.Viewer.CanAdminister() && !repo.IsMirror,
	}
	for _, name := range names {
		commit, err := gitRepo.BranchCommit(ctx, name)
		if err != nil {
			log.Error("getRepoBranches: get commit of branch %q in repo %d: %v", name, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get branch commit")
		}
		resp.Branches = append(resp.Branches, repoBranch{
			Name:          name,
			IsProtected:   protected[name],
			CommitID:      commit.ID.String(),
			CommittedAt:   commit.Committer.When,
			CommitterName: commit.Committer.Name,
		})
	}
	return http.StatusOK, resp, nil
}

// withRepoBranchSettings lets through the repositories whose branches can be
// managed. Like the server-rendered pages, mirrors have no branch settings.
func withRepoBranchSettings(c flamego.Context, rc *repoAdminContext) {
	if rc.Repo.IsMirror {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

type repoBranchSettings struct {
	// IsEmpty is true for a repository without commits, which has no
	// branches to manage yet.
	IsEmpty           bool     `json:"isEmpty"`
	DefaultBranch     string   `json:"defaultBranch"`
	Branches          []string `json:"branches"`
	ProtectedBranches []string `json:"protectedBranches"`
}

func getRepoBranchSettings(c flamego.Context, repoCtx *repoContext, rc *repoAdminContext) (statusCode int, resp *repoBranchSettings, err error) {
	repo := rc.Repo
	resp = &repoBranchSettings{
		IsEmpty:           repo.IsEmpty,
		DefaultBranch:     repo.DefaultBranch,
		Branches:          []string{},
		ProtectedBranches: []string{},
	}
	if repo.IsEmpty {
		return http.StatusOK, resp, nil
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("getRepoBranchSettings: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if resp.Branches, err = gitRepo.Branches(c.Request().Context()); err != nil {
		log.Error("getRepoBranchSettings: list branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	protected, err := database.GetProtectBranchesByRepoID(repo.ID)
	if err != nil {
		log.Error("getRepoBranchSettings: list protected branches of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list protected branches")
	}
	// Protection outlives deleted branches, which are not listed.
	for _, p := range protected {
		if slices.Contains(resp.Branches, p.Name) {
			resp.ProtectedBranches = append(resp.ProtectedBranches, p.Name)
		}
	}
	return http.StatusOK, resp, nil
}

type repoDefaultBranchRequest struct {
	Branch string `json:"branch"`
}

func putRepoDefaultBranch(c flamego.Context, l i18n.Locale, repoCtx *repoContext, rc *repoAdminContext, req repoDefaultBranchRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("putRepoDefaultBranch: open repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if repo.IsEmpty || !gitRepo.HasBranch(ctx, req.Branch) {
		msg := l.Tr("form.target_branch_not_exist")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"branch": &msg}}, nil
	}
	if repo.DefaultBranch == req.Branch {
		return http.StatusNoContent, nil, nil
	}

	if _, err = gitRepo.SymbolicRef(ctx, git.SymbolicRefOptions{Ref: git.RefsHeads + req.Branch}); err != nil {
		log.Error("putRepoDefaultBranch: point HEAD of repo %d to %q: %v", repo.ID, req.Branch, err)
		return http.StatusInternalServerError, nil, errors.New(l.Tr("repo.settings.update_default_branch_unsupported"))
	}
	repo.DefaultBranch = req.Branch
	if err = database.UpdateRepository(repo, false); err != nil {
		log.Error("putRepoDefaultBranch: update repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update repository")
	}
	return http.StatusNoContent, nil, nil
}

type repoProtectedBranchUser struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
}

type repoProtectedBranchTeam struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type repoProtectedBranch struct {
	Name               string  `json:"name"`
	Protected          bool    `json:"protected"`
	RequirePullRequest bool    `json:"requirePullRequest"`
	EnableWhitelist    bool    `json:"enableWhitelist"`
	WhitelistUserIDs   []int64 `json:"whitelistUserIDs"`
	WhitelistTeamIDs   []int64 `json:"whitelistTeamIDs"`
	// The options are only sent for repositories of organizations, which
	// alone can let people and teams push past the protection.
	UserOptions []repoProtectedBranchUser `json:"userOptions,omitempty"`
	TeamOptions []repoProtectedBranchTeam `json:"teamOptions,omitempty"`
}

// withRepoProtectedBranch injects the protection of the branch in the route,
// which defaults to none.
func withRepoProtectedBranch(c flamego.Context, repoCtx *repoContext, rc *repoAdminContext) {
	w := c.ResponseWriter()
	repo := rc.Repo
	branch := c.Param("branch")
	if repo.IsEmpty {
		writeErrorResponse(w, http.StatusNotFound, errors.New("branch does not exist"))
		return
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		log.Error("withRepoProtectedBranch: open repository %d: %v", repo.ID, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}
	if !gitRepo.HasBranch(c.Request().Context(), branch) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("branch does not exist"))
		return
	}

	protectBranch, err := database.GetProtectBranchOfRepoByName(repo.ID, branch)
	if err != nil {
		if !database.IsErrBranchNotExist(err) {
			log.Error("withRepoProtectedBranch: get protection of branch %q in repo %d: %v", branch, repo.ID, err)
			writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get protected branch"))
			return
		}
		protectBranch = &database.ProtectBranch{RepoID: repo.ID, Name: branch}
	}
	c.Map(protectBranch)
}

func splitIDs(s string) []int64 {
	ids := []int64{}
	for _, id := range tool.StringsToInt64s(strings.Split(s, ",")) {
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func joinIDs(ids []int64) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(strs, ",")
}

func getRepoProtectedBranch(rc *repoAdminContext, protectBranch *database.ProtectBranch) (statusCode int, resp *repoProtectedBranch, err error) {
	resp = &repoProtectedBranch{
		Name:               protectBranch.Name,
		Protected:          protectBranch.Protected,
		RequirePullRequest: protectBranch.RequirePullRequest,
		EnableWhitelist:    protectBranch.EnableWhitelist,
		WhitelistUserIDs:   splitIDs(protectBranch.WhitelistUserIDs),
		WhitelistTeamIDs:   splitIDs(protectBranch.WhitelistTeamIDs),
	}
	if !rc.Owner.IsOrganization() {
		return http.StatusOK, resp, nil
	}

	writers, err := rc.Repo.GetWriters()
	if err != nil {
		log.Error("getRepoProtectedBranch: list writers of repo %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list writers")
	}
	resp.UserOptions = make([]repoProtectedBranchUser, 0, len(writers))
	for _, u := range writers {
		resp.UserOptions = append(resp.UserOptions, repoProtectedBranchUser{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURLPath()})
	}
	teams, err := rc.Owner.TeamsHaveAccessToRepo(rc.Repo.ID, database.AccessModeWrite)
	if err != nil {
		log.Error("getRepoProtectedBranch: list teams of repo %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list teams")
	}
	resp.TeamOptions = make([]repoProtectedBranchTeam, 0, len(teams))
	for _, t := range teams {
		resp.TeamOptions = append(resp.TeamOptions, repoProtectedBranchTeam{ID: t.ID, Name: t.Name})
	}
	return http.StatusOK, resp, nil
}

type repoProtectedBranchRequest struct {
	Protected          bool    `json:"protected"`
	RequirePullRequest bool    `json:"requirePullRequest"`
	EnableWhitelist    bool    `json:"enableWhitelist"`
	WhitelistUserIDs   []int64 `json:"whitelistUserIDs"`
	WhitelistTeamIDs   []int64 `json:"whitelistTeamIDs"`
}

func putRepoProtectedBranch(rc *repoAdminContext, protectBranch *database.ProtectBranch, req repoProtectedBranchRequest) (statusCode int, resp any, err error) {
	protectBranch.Protected = req.Protected
	protectBranch.RequirePullRequest = req.RequirePullRequest
	if rc.Owner.IsOrganization() {
		protectBranch.EnableWhitelist = req.EnableWhitelist
		// People and teams who cannot write are dropped.
		err = database.UpdateOrgProtectBranch(rc.Repo, protectBranch, joinIDs(req.WhitelistUserIDs), joinIDs(req.WhitelistTeamIDs))
	} else {
		protectBranch.EnableWhitelist = false
		err = database.UpdateProtectBranch(protectBranch)
	}
	if err != nil {
		log.Error("putRepoProtectedBranch: update protection of branch %q in repo %d: %v", protectBranch.Name, rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update protected branch")
	}
	return http.StatusNoContent, nil, nil
}
