package web

import (
	"net/http"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

type repoIssueMilestone struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type repoIssue struct {
	dashboardIssue
	// IsUnread is whether the signed-in viewer has not read the issue since it
	// last changed.
	IsUnread  bool                `json:"isUnread"`
	Milestone *repoIssueMilestone `json:"milestone,omitempty"`
}

type repoIssueAssignee struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarURL"`
}

type repoIssueFilterMilestone struct {
	repoIssueMilestone
	IsClosed bool `json:"isClosed"`
}

type repoIssues struct {
	*listPage[repoIssue]
	OpenCount   int64 `json:"openCount"`
	ClosedCount int64 `json:"closedCount"`
	// Labels, Milestones and Assignees are what the list can be filtered by.
	Labels     []dashboardIssueLabel      `json:"labels"`
	Milestones []repoIssueFilterMilestone `json:"milestones"`
	Assignees  []repoIssueAssignee        `json:"assignees"`
	// NewPullRequestURL is where the viewer proposes a pull request, and is not
	// sent to viewers who cannot propose one here.
	NewPullRequestURL string `json:"newPullRequestURL,omitempty"`
}

func getRepoIssues(c flamego.Context, user *database.User, repoCtx *repoContext) (statusCode int, resp *repoIssues, err error) {
	repo := repoCtx.Repo
	// Like the server-rendered page, guests can list the issues of a
	// repository when its issues are public.
	if !repo.EnableIssues || repo.EnableExternalTracker || (!repoCtx.Viewer.CanRead() && !repo.CanGuestViewIssues()) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	return listRepoIssues(c, user, repoCtx, false)
}

func getRepoPulls(c flamego.Context, user *database.User, repoCtx *repoContext) (statusCode int, resp *repoIssues, err error) {
	if !repoCtx.Viewer.CanRead() || !repoCtx.Repo.AllowsPulls() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	return listRepoIssues(c, user, repoCtx, true)
}

// listRepoIssues lists the issues or pull requests of the repository, filtered
// and sorted like the server-rendered page by the `type`, `labels`,
// `milestone`, `assignee`, `state`, `sort` and `page` query parameters.
func listRepoIssues(c flamego.Context, user *database.User, repoCtx *repoContext, isPull bool) (statusCode int, resp *repoIssues, err error) {
	repo := repoCtx.Repo
	filterMode := database.FilterModeYourRepos
	assigneeID := c.QueryInt64("assignee")
	var posterID int64
	if mode := database.FilterMode(c.Query("type")); slices.Contains([]database.FilterMode{database.FilterModeAssign, database.FilterModeCreate, database.FilterModeMention}, mode) {
		// Lists about the viewer need a signed-in viewer.
		if user == nil {
			return http.StatusUnauthorized, nil, errors.New("authentication required")
		}
		filterMode = mode
		switch mode {
		case database.FilterModeAssign:
			assigneeID = user.ID
		case database.FilterModeCreate:
			posterID = user.ID
		}
	}

	var viewerID int64 = -1
	if user != nil {
		viewerID = user.ID
	}
	labels := c.Query("labels")
	milestoneID := c.QueryInt64("milestone")
	isClosed := c.Query("state") == "closed"
	page := pageParam(c)

	stats := database.GetIssueStats(&database.IssueStatsOptions{
		RepoID:      repo.ID,
		UserID:      viewerID,
		Labels:      labels,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		FilterMode:  filterMode,
		IsPull:      isPull,
	})
	total := stats.OpenCount
	if isClosed {
		total = stats.ClosedCount
	}
	// A page past the end lists the last page, like the server-rendered page.
	pageSize := int64(max(conf.UI.IssuePagingNum, 1))
	page = min(page, max(int((total+pageSize-1)/pageSize), 1))

	issues, err := database.Issues(&database.IssuesOptions{
		UserID:      viewerID,
		AssigneeID:  assigneeID,
		RepoID:      repo.ID,
		PosterID:    posterID,
		MilestoneID: milestoneID,
		Page:        page,
		IsClosed:    isClosed,
		IsMention:   filterMode == database.FilterModeMention,
		IsPull:      isPull,
		Labels:      labels,
		SortType:    c.Query("sort"),
	})
	if err != nil {
		log.Error("listRepoIssues: list issues of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list issues")
	}

	var reads []*database.IssueUser
	if user != nil {
		reads, err = database.GetIssueUsers(repo.ID, user.ID, isClosed)
		if err != nil {
			log.Error("listRepoIssues: get read states in repository %d: %v", repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get read states")
		}
	}

	items := make([]repoIssue, 0, len(issues))
	for _, issue := range issues {
		item := repoIssue{
			dashboardIssue: dashboardIssue{
				Repo:         repo.FullName(),
				Index:        issue.Index,
				Title:        issue.Title,
				IsClosed:     issue.IsClosed,
				CommentCount: issue.NumComments,
				Labels:       make([]dashboardIssueLabel, 0, len(issue.Labels)),
				Poster:       dashboardIssueUser{Name: issue.Poster.Name, AvatarURL: issue.Poster.AvatarURLPath()},
				CreatedAt:    issue.Created.UTC(),
			},
		}
		for _, label := range issue.Labels {
			item.Labels = append(item.Labels, toIssueLabel(label))
		}
		if issue.Assignee != nil {
			item.Assignee = &dashboardIssueUser{Name: issue.Assignee.Name, AvatarURL: issue.Assignee.AvatarURLPath()}
		}
		if issue.Milestone != nil {
			item.Milestone = &repoIssueMilestone{ID: issue.Milestone.ID, Name: issue.Milestone.Name}
		}
		if i := database.PairsContains(reads, issue.ID, viewerID); i > -1 {
			item.IsUnread = !reads[i].IsRead
		}
		items = append(items, item)
	}

	resp = &repoIssues{
		listPage:    newListPage(items, page, total, conf.UI.IssuePagingNum),
		OpenCount:   stats.OpenCount,
		ClosedCount: stats.ClosedCount,
	}

	labelList, err := database.GetLabelsByRepoID(repo.ID)
	if err != nil {
		log.Error("listRepoIssues: list labels of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list labels")
	}
	resp.Labels = make([]dashboardIssueLabel, 0, len(labelList))
	for _, label := range labelList {
		resp.Labels = append(resp.Labels, toIssueLabel(label))
	}

	milestones, err := database.GetMilestonesByRepoID(repo.ID)
	if err != nil {
		log.Error("listRepoIssues: list milestones of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list milestones")
	}
	resp.Milestones = make([]repoIssueFilterMilestone, 0, len(milestones))
	for _, m := range milestones {
		resp.Milestones = append(resp.Milestones, repoIssueFilterMilestone{
			repoIssueMilestone: repoIssueMilestone{ID: m.ID, Name: m.Name},
			IsClosed:           m.IsClosed,
		})
	}

	assignees, err := repo.GetAssignees()
	if err != nil {
		log.Error("listRepoIssues: list assignees of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list assignees")
	}
	resp.Assignees = make([]repoIssueAssignee, 0, len(assignees))
	for _, u := range assignees {
		resp.Assignees = append(resp.Assignees, repoIssueAssignee{ID: u.ID, Name: u.Name, AvatarURL: u.AvatarURLPath()})
	}

	if isPull && user != nil {
		resp.NewPullRequestURL, err = newPullRequestURL(c, user, repoCtx)
		if err != nil {
			log.Error("listRepoIssues: find where to propose a pull request to repository %d: %v", repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "find where to propose a pull request")
		}
	}
	return http.StatusOK, resp, nil
}

func toIssueLabel(label *database.Label) dashboardIssueLabel {
	return dashboardIssueLabel{ID: label.ID, Name: label.Name, Color: label.Color, TextColor: string(label.ForegroundColor())}
}

// newPullRequestURL returns where the viewer proposes a pull request from the
// default branch of the repository, the same way as the server-rendered
// pages, or an empty string when they cannot propose one. Writers propose
// between branches, or to the base repository of a fork, and those who forked
// the repository propose from their fork.
func newPullRequestURL(c flamego.Context, user *database.User, repoCtx *repoContext) (string, error) {
	repo := repoCtx.Repo
	isWriter := repoCtx.Viewer.CanWrite()
	forked := database.Handle.Repositories().HasForkedBy(c.Request().Context(), repo.ID, user.ID)
	if !isWriter && !forked {
		return "", nil
	}
	// Loading the base repository treats a fork of a deleted repository as no
	// fork.
	if err := repo.LoadAttributes(); err != nil {
		return "", errors.Wrap(err, "load base repository")
	}

	allowed := false
	base, head := repo, repo.DefaultBranch
	if repo.IsFork {
		if repo.BaseRepo.AllowsPulls() {
			allowed = true
			if isWriter {
				base, head = repo.BaseRepo, repoCtx.Owner.Name+":"+repo.DefaultBranch
			} else {
				head = user.Name + ":" + repo.DefaultBranch
			}
		}
	} else if repo.AllowsPulls() {
		allowed = true
	}
	// The pull request list prefers the viewer's fork as the head, like the
	// server-rendered page.
	if forked {
		allowed = true
		head = user.Name + ":" + repo.DefaultBranch
	}
	if !allowed {
		return "", nil
	}
	return base.Link() + "/compare/" + repo.DefaultBranch + "..." + head, nil
}

type repoFork struct {
	Owner          string `json:"owner"`
	OwnerAvatarURL string `json:"ownerAvatarURL"`
	Name           string `json:"name"`
	IsPrivate      bool   `json:"isPrivate"`
}

// getRepoForks lists the forks of the repository that the viewer can see.
func getRepoForks(c flamego.Context, user *database.User, repoCtx *repoContext) (statusCode int, resp map[string][]repoFork, err error) {
	repo := repoCtx.Repo
	// Like the server-rendered page, an empty repository has no forks page.
	if !repoCtx.Viewer.CanRead() || repo.IsEmpty {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	forks, err := repo.GetForks()
	if err != nil {
		log.Error("getRepoForks: list forks of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list forks")
	}

	ctx := c.Request().Context()
	items := make([]repoFork, 0, len(forks))
	for _, fork := range forks {
		if fork.IsPrivate && (user == nil || !user.IsAdmin) &&
			database.Handle.Permissions().AccessMode(ctx, repoCtx.Viewer.ID(), fork.ID, database.AccessModeOptions{
				OwnerID: fork.OwnerID,
				Private: true,
			}) < database.AccessModeRead {
			continue
		}
		if err = fork.GetOwner(); err != nil {
			log.Error("getRepoForks: get owner of fork %d: %v", fork.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get owner")
		}
		items = append(items, repoFork{
			Owner:          fork.Owner.Name,
			OwnerAvatarURL: fork.Owner.AvatarURLPath(),
			Name:           fork.Name,
			IsPrivate:      fork.IsPrivate,
		})
	}
	return http.StatusOK, map[string][]repoFork{"items": items}, nil
}
