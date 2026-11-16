package web

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
)

// pullParent is a pull request that another one depends on, or may depend on.
type pullParent struct {
	Index int64  `json:"index"`
	Title string `json:"title"`
	// State is "open", "closed" or "merged".
	State string `json:"state"`
}

// pullStack is the pull requests connected to one by dependencies.
type pullStack struct {
	// BaseBranch is the branch the pull requests without parents merge into.
	BaseBranch string `json:"baseBranch"`
	// Nodes are parents first.
	Nodes []pullStackNode `json:"nodes"`
}

type pullStackNode struct {
	Index      int64  `json:"index"`
	Title      string `json:"title"`
	HeadBranch string `json:"headBranch"`
	// Status is "merged", "closed", "behind" when it lacks changes of a parent,
	// "blocked" while a parent is not merged, "checking", "mergeable" or
	// "conflict".
	Status string `json:"status"`
	// Parents are the numbers of the pull requests it depends on.
	Parents   []int64   `json:"parents"`
	Poster    issueUser `json:"poster"`
	UpdatedAt time.Time `json:"updatedAt"`
	Comments  int       `json:"comments"`
}

func pullParentState(pr *database.PullRequest) string {
	switch {
	case pr.HasMerged:
		return "merged"
	case pr.Issue.IsClosed:
		return "closed"
	default:
		return "open"
	}
}

// describePullStack adds the dependencies of the pull request to its
// description: its parents, the ones it waits for or lags behind, and the
// graph of the pull requests connected to it.
func describePullStack(ctx context.Context, repoCtx *repoContext, gitRepo *git.Repository, pull *database.PullRequest, info *repoPullInfo) error {
	stack, err := database.LoadPullRequestStack(ctx, pull)
	if err != nil {
		return errors.Wrap(err, "load stack")
	}
	byID := make(map[int64]*database.PullRequest, len(stack.Pulls))
	for _, p := range stack.Pulls {
		byID[p.ID] = p
	}

	// A branch that cannot be read only hides "behind", and the page still
	// shows the rest of the stack.
	behindOf, err := stack.Behind(ctx, gitRepo, pull.BaseBranch)
	if err != nil {
		log.Trace("describePullStack: check which pull requests are behind: %v", err)
	}

	info.Parents = []pullParent{}
	info.Waiting = []int64{}
	info.Behind = []int64{}
	// Agents and people tend to open a whole stack themselves, so most of its
	// pull requests share a few posters.
	posters := make(map[int64]*database.User)
	nodes := make([]pullStackNode, 0, len(stack.Pulls))
	for _, p := range stack.Pulls {
		poster, ok := posters[p.Issue.PosterID]
		if !ok {
			poster, err = database.Handle.Users().GetByID(ctx, p.Issue.PosterID)
			if database.IsErrUserNotExist(err) {
				poster = database.NewGhostUser()
			} else if err != nil {
				return errors.Wrapf(err, "get poster of pull request %d", p.ID)
			}
			posters[p.Issue.PosterID] = poster
		}
		node := pullStackNode{
			Index:      p.Index,
			Title:      p.Issue.Title,
			HeadBranch: p.HeadBranch,
			Parents:    []int64{},
			Poster:     toIssueUser(poster),
			UpdatedAt:  time.Unix(p.Issue.UpdatedUnix, 0),
			Comments:   p.Issue.NumComments,
		}
		var unmerged []*database.PullRequest
		for _, parentID := range stack.Parents[p.ID] {
			parent := byID[parentID]
			if parent == nil {
				continue
			}
			node.Parents = append(node.Parents, parent.Index)
			if !parent.HasMerged {
				unmerged = append(unmerged, parent)
			}
			if p.ID == pull.ID {
				info.Parents = append(info.Parents, pullParent{Index: parent.Index, Title: parent.Issue.Title, State: pullParentState(parent)})
				if !parent.HasMerged {
					info.Waiting = append(info.Waiting, parent.Index)
				}
			}
		}

		switch {
		case p.HasMerged:
			node.Status = "merged"
		case p.Issue.IsClosed:
			node.Status = "closed"
		case len(unmerged) > 0:
			node.Status = "blocked"
			if behind := behindOf[p.ID]; len(behind) > 0 {
				node.Status = "behind"
				if p.ID == pull.ID {
					for _, parent := range behind {
						info.Behind = append(info.Behind, parent.Index)
					}
				}
			}
		case p.IsChecking():
			node.Status = "checking"
		case p.CanAutoMerge():
			node.Status = "mergeable"
		default:
			node.Status = "conflict"
		}
		nodes = append(nodes, node)
	}
	if len(nodes) > 1 {
		info.Stack = &pullStack{BaseBranch: pull.BaseBranch, Nodes: nodes}
	}

	open := !pull.HasMerged && !pull.Issue.IsClosed
	local := pull.HeadRepoID == repoCtx.Repo.ID
	writer := repoCtx.Viewer.CanWrite()
	info.ViewerCanEditParents = writer && open && local
	info.ViewerCanUpdate = writer && open && local && len(info.Behind) > 0
	if open && len(info.Waiting) > 0 {
		info.ViewerCanMergeStack = writer && local
		for _, p := range stack.UnmergedAncestors(pull.ID) {
			if p.Issue.IsClosed || p.BaseBranch != pull.BaseBranch {
				info.ViewerCanMergeStack = false
			}
		}
	}

	if info.ViewerCanEditParents {
		// Offer the open pull requests of the repository, except the ones
		// that depend on this one, which would make a cycle.
		descendants := stack.Descendants(pull.ID)
		candidates, err := database.ListOpenLocalPullRequests(repoCtx.Repo.ID)
		if err != nil {
			return errors.Wrap(err, "list open pull requests")
		}
		info.ParentOptions = []pullParent{}
		for _, p := range candidates {
			if p.ID != pull.ID && !descendants[p.ID] {
				info.ParentOptions = append(info.ParentOptions, pullParent{Index: p.Index, Title: p.Issue.Title, State: "open"})
			}
		}
		// Keep the parents that are merged or closed choosable, so unchecking
		// another parent does not drop them.
		for _, parent := range info.Parents {
			if parent.State != "open" {
				info.ParentOptions = append(info.ParentOptions, parent)
			}
		}
		sort.Slice(info.ParentOptions, func(i, j int) bool { return info.ParentOptions[i].Index < info.ParentOptions[j].Index })
	}
	return nil
}

type repoPullParentsRequest struct {
	// Parents are the numbers of the pull requests to depend on, or none to
	// review the pull request against its base branch again.
	Parents []int64 `json:"parents" validate:"max=50"`
}

// putRepoPullParents replaces the pull requests the pull request depends on.
// Only pull requests whose head branches are in the repository can depend on
// one another, and new parents must be open.
func putRepoPullParents(c flamego.Context, repoCtx *repoContext, issue *database.Issue, req repoPullParentsRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	pull := issue.PullRequest
	pull.Issue = issue
	if issue.IsClosed || pull.HasMerged {
		return http.StatusConflict, &bindingErrorResponse{Error: "only open pull requests can depend on others"}, nil
	} else if pull.HeadRepoID != repo.ID {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: "only pull requests from branches of this repository can depend on others"}, nil
	}

	current, err := database.Handle.PullRequestParents().ListParentIDs(ctx, pull.ID)
	if err != nil {
		log.Error("putRepoPullParents: list parents of pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list parents")
	}
	isCurrent := make(map[int64]bool, len(current))
	for _, id := range current {
		isCurrent[id] = true
	}

	parents := make([]*database.PullRequest, 0, len(req.Parents))
	for _, index := range req.Parents {
		parentIssue, err := database.GetIssueByIndex(repo.ID, index)
		if database.IsErrIssueNotExist(err) || (err == nil && (!parentIssue.IsPull || parentIssue.PullRequest == nil)) {
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: errors.Newf("#%d is not a pull request", index).Error()}, nil
		} else if err != nil {
			log.Error("putRepoPullParents: get issue %d of repository %d: %v", index, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get issue")
		}
		parent := parentIssue.PullRequest
		parent.Issue = parentIssue
		switch {
		case parent.HeadRepoID != repo.ID:
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: errors.Newf("#%d is from another repository", index).Error()}, nil
		case !isCurrent[parent.ID] && (parent.HasMerged || parentIssue.IsClosed):
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: errors.Newf("#%d is not open", index).Error()}, nil
		}
		parents = append(parents, parent)
	}

	if err = pull.LoadAttributes(); err != nil {
		log.Error("putRepoPullParents: load attributes of pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load attributes")
	}
	err = database.SetPullRequestParents(ctx, pull, parents)
	if errors.Is(err, database.ErrPullRequestParentCycle) || errors.Is(err, database.ErrPullRequestParentsConflict) ||
		errors.Is(err, database.ErrPullRequestParentBaseMismatch) {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Error: err.Error()}, nil
	} else if err != nil {
		log.Error("putRepoPullParents: set parents of pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "set parents")
	}
	return http.StatusNoContent, nil, nil
}

// postRepoPullUpdate merges the changes of the parents the head branch lacks
// into it.
func postRepoPullUpdate(c flamego.Context, viewer *database.User, repoCtx *repoContext, issue *database.Issue) (statusCode int, resp any, err error) {
	pull := issue.PullRequest
	pull.Issue = issue
	if issue.IsClosed || pull.HasMerged || pull.HeadRepoID != repoCtx.Repo.ID {
		return http.StatusNotFound, nil, errors.New("pull request cannot be updated")
	}
	_, err = pull.UpdateBranch(c.Request().Context(), viewer)
	if errors.Is(err, database.ErrPullRequestUpdateConflict) || errors.Is(err, database.ErrPullRequestBranchProtected) {
		return http.StatusConflict, &bindingErrorResponse{Error: err.Error()}, nil
	} else if err != nil {
		log.Error("postRepoPullUpdate: update branch of pull request %d: %v", pull.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update branch")
	}
	return http.StatusNoContent, nil, nil
}
