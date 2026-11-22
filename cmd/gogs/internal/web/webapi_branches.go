package web

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
)

type repoBranch struct {
	Name          string    `json:"name"`
	IsProtected   bool      `json:"isProtected"`
	CommitID      string    `json:"commitID"`
	CommittedAt   time.Time `json:"committedAt"`
	CommitterName string    `json:"committerName"`
}

// repoBranchSection is a group of the branches overview, e.g., the active
// branches.
type repoBranchSection struct {
	// Kind is one of "default", "yours", "active" and "stale".
	Kind     string       `json:"kind"`
	Branches []repoBranch `json:"branches"`
	// HasMore is true when the section shows only the first of its branches,
	// and the view of the same name lists the rest.
	HasMore bool `json:"hasMore"`
}

type repoBranchesResponse struct {
	DefaultBranch string `json:"defaultBranch"`
	View          string `json:"view"`
	// Sections are the groups of the overview, leaving out the empty ones. They
	// are empty for the other views.
	Sections []repoBranchSection `json:"sections"`
	// Branches, Page and HasNext are the page of the views other than the
	// overview. Branches is empty for the overview.
	Branches []repoBranch `json:"branches"`
	Page     int          `json:"page"`
	HasNext  bool         `json:"hasNext"`
	// ViewerCanWrite offers pull requests from the branches, when the
	// repository allows them.
	ViewerCanWrite bool `json:"viewerCanWrite"`
	AllowsPulls    bool `json:"allowsPulls"`
	// ViewerCanChangeDefault links the settings that change the default
	// branch, which mirrors do not have.
	ViewerCanChangeDefault bool `json:"viewerCanChangeDefault"`
}

const (
	branchesPageSize = 20
	// branchesSectionSize is how many branches each overview section shows.
	branchesSectionSize = 5

	branchActiveAge = 30 * 24 * time.Hour
	branchStaleAge  = 90 * 24 * time.Hour
)

// branchesFilter selects the branches of a view of the branches page.
type branchesFilter struct {
	DefaultBranch string
	// Query matches branch names by case-insensitive substring.
	Query string
	Now   time.Time
	// ViewerEmails are the activated email addresses of the viewer, in lower
	// case. A branch is the viewer's when its last commit was committed with
	// one of them.
	ViewerEmails map[string]bool
}

// matches reports whether the branch belongs to the view, which is one of
// "default", "yours", "active", "stale" and "all". Only "all" and "default"
// include the default branch.
func (f branchesFilter) matches(view string, b *git.Branch) bool {
	if f.Query != "" && !strings.Contains(strings.ToLower(b.Name), strings.ToLower(f.Query)) {
		return false
	}
	switch view {
	case "all":
		return true
	case "default":
		return b.Name == f.DefaultBranch
	}
	if b.Name == f.DefaultBranch {
		return false
	}
	switch view {
	case "yours":
		return f.ViewerEmails[strings.ToLower(b.Committer.Email)]
	case "active":
		return f.Now.Sub(b.Committer.When) < branchActiveAge
	case "stale":
		return f.Now.Sub(b.Committer.When) > branchStaleAge
	}
	return false
}

// selectBranches returns the branches of the view from the given ones, skipping
// the first offset of them and stopping at limit. hasMore reports whether more
// of them follow.
func (f branchesFilter) selectBranches(branches []*git.Branch, view string, offset, limit int) (selected []*git.Branch, hasMore bool) {
	selected = []*git.Branch{}
	for _, b := range branches {
		if !f.matches(view, b) {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		if len(selected) == limit {
			return selected, true
		}
		selected = append(selected, b)
	}
	return selected, false
}

// protectedBranchNames returns the names of the protected branches of the
// repository.
func protectedBranchNames(ctx context.Context, repoID int64) (map[string]bool, error) {
	protected, err := database.Handle.ProtectBranches().ListProtected(ctx, repoID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(protected))
	for _, p := range protected {
		names[p.Name] = true
	}
	return names, nil
}

// getRepoBranches lists the branches of a view of the branches page, which is
// given by the "view" query parameter: "overview" (the default), "yours",
// "active", "stale" or "all". Every view but the overview is paged.
// Unauthenticated viewers have no branches of their own, so "yours" is empty for them.
func getRepoBranches(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoBranchesResponse, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	view := c.Query("view")
	if view == "" {
		view = "overview"
	}
	if !slices.Contains([]string{"overview", "yours", "active", "stale", "all"}, view) {
		return http.StatusBadRequest, nil, errors.Newf("unknown view %q", view)
	}

	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	branches, err := gitRepo.BranchesByCommitterDate(ctx)
	if err != nil {
		logger.Error("Failed to list branches", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	protected, err := protectedBranchNames(ctx, repo.ID)
	if err != nil {
		logger.Error("Failed to list protected branches", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list protected branches")
	}

	filter := branchesFilter{
		DefaultBranch: repo.DefaultBranch,
		Query:         strings.TrimSpace(c.Query("query")),
		Now:           time.Now(),
		ViewerEmails:  map[string]bool{},
	}
	if repoCtx.Viewer.Authenticated() && (view == "overview" || view == "yours") {
		emails, err := database.Handle.Users().ListEmails(ctx, repoCtx.Viewer.ID())
		if err != nil {
			logger.Error("Failed to list emails", "userID", repoCtx.Viewer.ID(), "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "list emails")
		}
		for _, email := range emails {
			if email.IsActivated {
				filter.ViewerEmails[strings.ToLower(email.Email)] = true
			}
		}
	}

	toResponse := func(branches []*git.Branch) []repoBranch {
		list := make([]repoBranch, 0, len(branches))
		for _, b := range branches {
			list = append(list, repoBranch{
				Name:          b.Name,
				IsProtected:   protected[b.Name],
				CommitID:      b.CommitID,
				CommittedAt:   b.Committer.When,
				CommitterName: b.Committer.Name,
			})
		}
		return list
	}

	resp = &repoBranchesResponse{
		DefaultBranch:          repo.DefaultBranch,
		View:                   view,
		Sections:               []repoBranchSection{},
		Branches:               []repoBranch{},
		Page:                   1,
		ViewerCanWrite:         repoCtx.Viewer.CanWrite(),
		AllowsPulls:            repo.AllowsPulls(),
		ViewerCanChangeDefault: repoCtx.Viewer.CanAdminister() && !repo.IsMirror,
	}
	if view != "overview" {
		page := pageParam(c)
		selected, hasNext := filter.selectBranches(branches, view, (page-1)*branchesPageSize, branchesPageSize)
		resp.Branches = toResponse(selected)
		resp.Page = page
		resp.HasNext = hasNext
		return http.StatusOK, resp, nil
	}

	kinds := []string{"default", "active", "stale"}
	if repoCtx.Viewer.Authenticated() {
		kinds = []string{"default", "yours", "active", "stale"}
	}
	for _, kind := range kinds {
		selected, hasMore := filter.selectBranches(branches, kind, 0, branchesSectionSize)
		if len(selected) > 0 {
			resp.Sections = append(resp.Sections, repoBranchSection{Kind: kind, Branches: toResponse(selected), HasMore: hasMore})
		}
	}
	return http.StatusOK, resp, nil
}

// withRepoBranchSettings lets through the repositories whose branches can be
// managed. Mirrors have no branch settings.
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

func getRepoBranchSettings(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, rc *repoAdminContext) (statusCode int, resp *repoBranchSettings, err error) {
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
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if resp.Branches, err = gitRepo.Branches(c.Request().Context()); err != nil {
		logger.Error("Failed to list branches", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list branches")
	}
	protected, err := database.Handle.ProtectBranches().ListProtected(c.Request().Context(), repo.ID)
	if err != nil {
		logger.Error("Failed to list protected branches", "repoID", repo.ID, "error", err)
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

func putRepoDefaultBranch(c flamego.Context, logger *logx.Logger, l i18n.Locale, repoCtx *repoContext, rc *repoAdminContext, req repoDefaultBranchRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := rc.Repo
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	if repo.IsEmpty || !gitRepo.HasBranch(ctx, req.Branch) {
		msg := l.Tr("form::target_branch_not_exist")
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"branch": &msg}}, nil
	}
	if repo.DefaultBranch == req.Branch {
		return http.StatusNoContent, nil, nil
	}

	if _, err = gitRepo.SymbolicRef(ctx, git.SymbolicRefOptions{Ref: git.RefsHeads + req.Branch}); err != nil {
		logger.Error("Failed to point HEAD to branch", "repoID", repo.ID, "branch", req.Branch, "error", err)
		return http.StatusInternalServerError, nil, errors.New(l.Tr("repo::settings.update_default_branch_unsupported"))
	}
	repo.DefaultBranch = req.Branch
	if err = database.Handle.Repositories().Update(ctx, logger, repo, false); err != nil {
		logger.Error("Failed to update repository", "repoID", repo.ID, "error", err)
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
	EnableAllowlist    bool    `json:"enableAllowlist"`
	AllowlistUserIDs   []int64 `json:"allowlistUserIDs"`
	AllowlistTeamIDs   []int64 `json:"allowlistTeamIDs"`
	reviewProtection
	// The options are only sent for repositories of organizations, which
	// alone can let people and teams push past the protection.
	UserOptions []repoProtectedBranchUser `json:"userOptions,omitempty"`
	TeamOptions []repoProtectedBranchTeam `json:"teamOptions,omitempty"`
}

// withRepoProtectedBranch injects the protection of the branch in the route,
// which defaults to none.
func withRepoProtectedBranch(c flamego.Context, logger *logx.Logger, repoCtx *repoContext, rc *repoAdminContext) {
	w := c.ResponseWriter()
	repo := rc.Repo
	branch := c.Param("branch")
	if repo.IsEmpty {
		writeErrorResponse(w, http.StatusNotFound, errors.New("branch does not exist"))
		return
	}
	gitRepo, err := openRepoGit(repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}
	if !gitRepo.HasBranch(c.Request().Context(), branch) {
		writeErrorResponse(w, http.StatusNotFound, errors.New("branch does not exist"))
		return
	}

	protectBranch, err := database.Handle.ProtectBranches().GetByName(c.Request().Context(), repo.ID, branch)
	if err != nil {
		if !database.IsErrBranchNotExist(err) {
			logger.Error("Failed to get branch protection", "repoID", repo.ID, "branch", branch, "error", err)
			writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get protected branch"))
			return
		}
		protectBranch = &database.ProtectBranch{RepoID: repo.ID, Name: branch}
	}
	c.Map(protectBranch)
}

func splitIDs(s string) []int64 {
	ids := []int64{}
	for field := range strings.SplitSeq(s, ",") {
		if id, _ := strconv.ParseInt(field, 10, 64); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func getRepoProtectedBranch(c flamego.Context, logger *logx.Logger, rc *repoAdminContext, protectBranch *database.ProtectBranch) (statusCode int, resp *repoProtectedBranch, err error) {
	ctx := c.Request().Context()
	resp = &repoProtectedBranch{
		Name:               protectBranch.Name,
		Protected:          protectBranch.Protected,
		RequirePullRequest: protectBranch.RequirePullRequest,
		EnableAllowlist:    protectBranch.EnableAllowlist,
		AllowlistUserIDs:   splitIDs(protectBranch.AllowlistUserIDs),
		AllowlistTeamIDs:   splitIDs(protectBranch.AllowlistTeamIDs),
		reviewProtection: reviewProtection{
			RequiredApprovals:            protectBranch.RequiredApprovals,
			DismissStaleApprovals:        protectBranch.DismissStaleApprovals,
			RequireResolvedConversations: protectBranch.RequireResolvedConversations,
		},
	}
	if !rc.Owner.IsOrganization() {
		return http.StatusOK, resp, nil
	}

	writers, err := database.Handle.Repositories().ListWriters(ctx, rc.Repo)
	if err != nil {
		logger.Error("Failed to list writers", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list writers")
	}
	resp.UserOptions = make([]repoProtectedBranchUser, 0, len(writers))
	for _, u := range writers {
		resp.UserOptions = append(resp.UserOptions, repoProtectedBranchUser{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURLPath()})
	}
	teams, err := database.Handle.Teams().ListByRepo(ctx, rc.Owner.ID, rc.Repo.ID, database.AccessModeWrite)
	if err != nil {
		logger.Error("Failed to list teams with write access", "repoID", rc.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list teams")
	}
	resp.TeamOptions = make([]repoProtectedBranchTeam, 0, len(teams))
	for _, t := range teams {
		resp.TeamOptions = append(resp.TeamOptions, repoProtectedBranchTeam{ID: t.ID, Name: t.Name})
	}
	return http.StatusOK, resp, nil
}

// reviewProtection is what reviews a pull request needs before it can merge
// into a protected branch.
type reviewProtection struct {
	RequiredApprovals            int  `json:"requiredApprovals" validate:"min=0,max=10"`
	DismissStaleApprovals        bool `json:"dismissStaleApprovals"`
	RequireResolvedConversations bool `json:"requireResolvedConversations"`
}

type repoProtectedBranchRequest struct {
	Protected          bool    `json:"protected"`
	RequirePullRequest bool    `json:"requirePullRequest"`
	EnableAllowlist    bool    `json:"enableAllowlist"`
	AllowlistUserIDs   []int64 `json:"allowlistUserIDs"`
	AllowlistTeamIDs   []int64 `json:"allowlistTeamIDs"`
	reviewProtection
}

func putRepoProtectedBranch(c flamego.Context, logger *logx.Logger, rc *repoAdminContext, protectBranch *database.ProtectBranch, req repoProtectedBranchRequest) (statusCode int, resp any, err error) {
	protectBranch.Protected = req.Protected
	protectBranch.RequirePullRequest = req.RequirePullRequest
	// Reviews only gate pull requests, so without requiring them, writers
	// would push past the reviews.
	// The options of approvals only apply when approvals are required.
	if req.RequirePullRequest {
		protectBranch.RequiredApprovals = req.RequiredApprovals
		protectBranch.DismissStaleApprovals = req.DismissStaleApprovals && req.RequiredApprovals > 0
		protectBranch.RequireResolvedConversations = req.RequireResolvedConversations && req.RequiredApprovals > 0
	} else {
		protectBranch.RequiredApprovals = 0
		protectBranch.DismissStaleApprovals = false
		protectBranch.RequireResolvedConversations = false
	}
	if rc.Owner.IsOrganization() {
		protectBranch.EnableAllowlist = req.EnableAllowlist
		// People and teams who cannot write are dropped.
		err = database.Handle.ProtectBranches().SaveForOrg(c.Request().Context(), logger, protectBranch, req.AllowlistUserIDs, req.AllowlistTeamIDs)
	} else {
		protectBranch.EnableAllowlist = false
		err = database.Handle.ProtectBranches().Save(c.Request().Context(), protectBranch)
	}
	if err != nil {
		logger.Error("Failed to update branch protection", "repoID", rc.Repo.ID, "branch", protectBranch.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update protected branch")
	}
	return http.StatusNoContent, nil, nil
}
