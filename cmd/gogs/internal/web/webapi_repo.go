package web

import (
	"context"
	"net/http"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/gogs/git-module"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/strx"
	"gogs.io/gogs/internal/tool"
)

type repoHeader struct {
	ID                   int64  `json:"id"`
	Owner                string `json:"owner"`
	Name                 string `json:"name"`
	AvatarURL            string `json:"avatarURL"`
	Visibility           string `json:"visibility"`
	MirrorOf             string `json:"mirrorOf,omitempty"`
	WatchCount           int    `json:"watchCount"`
	StarCount            int    `json:"starCount"`
	ForkCount            int    `json:"forkCount"`
	IssuesEnabled        bool   `json:"issuesEnabled"`
	OpenIssueCount       int    `json:"openIssueCount"`
	PullRequestsEnabled  bool   `json:"pullRequestsEnabled"`
	OpenPullRequestCount int    `json:"openPullRequestCount"`
	WikiEnabled          bool   `json:"wikiEnabled"`

	ViewerCanAdminister bool `json:"viewerCanAdminister"`
	ViewerIsWatching    bool `json:"viewerIsWatching"`
	ViewerIsStarring    bool `json:"viewerIsStarring"`
}

func getRepoHeader(repoCtx *repoContext) (statusCode int, resp *repoHeader, err error) {
	owner := repoCtx.Owner
	repo := repoCtx.Repo

	issuesEnabled := repo.EnableIssues
	wikiEnabled := repo.EnableWiki
	if !repoCtx.ViewerCanRead() {
		if !repo.IsPartialPublic() {
			return http.StatusNotFound, nil, errors.New("repository does not exist")
		}
		issuesEnabled = repo.CanGuestViewIssues()
		wikiEnabled = repo.CanGuestViewWiki()
	}

	visibility := "public"
	if repo.IsPrivate {
		visibility = "private"
	}

	resp = &repoHeader{
		ID:                   repo.ID,
		Owner:                owner.Name,
		Name:                 repo.Name,
		AvatarURL:            strx.Coalesce(repo.AvatarLink(), owner.AvatarURL()),
		Visibility:           visibility,
		WatchCount:           repo.NumWatches,
		StarCount:            repo.NumStars,
		ForkCount:            repo.NumForks,
		IssuesEnabled:        issuesEnabled,
		OpenIssueCount:       repo.NumIssues - repo.NumClosedIssues,
		PullRequestsEnabled:  repo.AllowsPulls(),
		OpenPullRequestCount: repo.NumPulls - repo.NumClosedPulls,
		WikiEnabled:          wikiEnabled,

		ViewerCanAdminister: repoCtx.ViewerCanAdminister(),
		ViewerIsWatching:    database.IsWatching(repoCtx.ViewerID, repo.ID),
		ViewerIsStarring:    database.IsStarring(repoCtx.ViewerID, repo.ID),
	}

	if repo.IsMirror {
		mirror, err := database.GetMirrorByRepoID(repo.ID)
		if err != nil {
			log.Error("getRepoHeader: get mirror by repo ID %d: %v", repo.ID, err)
		} else if mirror != nil {
			resp.MirrorOf = mirror.Address()
		}
	}

	return http.StatusOK, resp, nil
}

type repoCommitSignature struct {
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	When       time.Time `json:"when"`
	AvatarURL  string    `json:"avatarURL"`
	ProfileURL string    `json:"profileURL,omitempty"`
}

type repoCommit struct {
	SHA     string              `json:"sha"`
	Subject string              `json:"subject"`
	Body    string              `json:"body"`
	Author  repoCommitSignature `json:"author"`
	Parents []string            `json:"parents"`
}

func getRepoCommit(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoCommit, err error) {
	if !repoCtx.ViewerCanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}

	ctx := c.Request().Context()
	owner := repoCtx.Owner
	repo := repoCtx.Repo
	commitID := c.Param("sha")

	gitRepo, err := git.Open(repox.RepositoryPath(owner.Name, repo.Name))
	if err != nil {
		log.Error("getRepoCommit: open repository %q/%q: %v", owner.Name, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}

	commit, err := gitRepo.CatFileCommit(commitID)
	if err != nil {
		if gitx.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		log.Error("getRepoCommit: cat-file commit %q in %q/%q: %v", commitID, owner.Name, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "cat-file commit")
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			log.Error("getRepoCommit: parent ID %d for %q in %q/%q: %v", i, commitID, owner.Name, repo.Name, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parent ID")
		}
		parents[i] = sha.String()
	}

	toSignature := func(s *git.Signature) repoCommitSignature {
		sig := repoCommitSignature{
			Name:      s.Name,
			Email:     s.Email,
			When:      s.When.UTC(),
			AvatarURL: tool.AvatarLink(s.Email),
		}
		if u, err := database.Handle.Users().GetByEmail(ctx, s.Email); err == nil && u != nil {
			sig.ProfileURL = conf.Server.Subpath + "/" + u.Name
		}
		return sig
	}

	subject := commit.Summary()
	var body string
	if msg := commit.Message; len(msg) > len(subject) {
		body = msg[len(subject):]
	}

	return http.StatusOK, &repoCommit{
		SHA:     commitID,
		Subject: subject,
		Body:    body,
		Author:  toSignature(commit.Author),
		Parents: parents,
	}, nil
}

type repoWatchResponse struct {
	WatchCount int `json:"watchCount"`
}

func repoWatchAction(ctx context.Context, repoCtx *repoContext, watching bool) (statusCode int, resp *repoWatchResponse, err error) {
	if repoCtx.ViewerCanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}

	repo := repoCtx.Repo

	if watching {
		err = database.Handle.Repositories().Watch(ctx, repoCtx.ViewerID, repo.ID)
	} else {
		err = database.WatchRepo(repoCtx.ViewerID, repo.ID, false)
	}
	if err != nil {
		log.Error("repoWatchAction: set watching=%t for user %d on repo %d: %v", watching, repoCtx.ViewerID, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "watch repo")
	}

	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		log.Error("repoWatchAction: reload repo %d (%q): %v", repo.ID, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoWatchResponse{
		WatchCount: updated.NumWatches,
	}, nil
}

func postRepoWatch(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	return repoWatchAction(c.Request().Context(), repoCtx, true)
}

func deleteRepoWatch(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	return repoWatchAction(c.Request().Context(), repoCtx, false)
}

type repoStarResponse struct {
	StarCount int `json:"starCount"`
}

func repoStarAction(ctx context.Context, repoCtx *repoContext, starring bool) (statusCode int, resp *repoStarResponse, err error) {
	if !repoCtx.ViewerCanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}

	repo := repoCtx.Repo

	if starring {
		err = database.Handle.Repositories().Star(ctx, repoCtx.ViewerID, repo.ID)
	} else {
		err = database.StarRepo(repoCtx.ViewerID, repo.ID, false)
	}
	if err != nil {
		log.Error("repoStarAction: set starred=%t for user %d on repo %d: %v", starring, repoCtx.ViewerID, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "star repo")
	}

	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		log.Error("repoStarAction: reload repo %d (%q): %v", repo.ID, repo.Name, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoStarResponse{
		StarCount: updated.NumStars,
	}, nil
}

func postRepoStar(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoStarResponse, err error) {
	return repoStarAction(c.Request().Context(), repoCtx, true)
}

func deleteRepoStar(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoStarResponse, err error) {
	return repoStarAction(c.Request().Context(), repoCtx, false)
}
