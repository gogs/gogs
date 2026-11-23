package web

import (
	"cmp"
	"context"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/pathx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/userx"
)

type repoHeader struct {
	ID         int64  `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	AvatarURL  string `json:"avatarURL"`
	Visibility string `json:"visibility"`
	MirrorOf   string `json:"mirrorOf,omitempty"`
	// ForkOf is the base repository of a fork, for viewers who can read both.
	ForkOf               *repoHeaderBase `json:"forkOf,omitempty"`
	WatchCount           int             `json:"watchCount"`
	StarCount            int             `json:"starCount"`
	ForkCount            int             `json:"forkCount"`
	IssuesEnabled        bool            `json:"issuesEnabled"`
	OpenIssueCount       int             `json:"openIssueCount"`
	PullRequestsEnabled  bool            `json:"pullRequestsEnabled"`
	OpenPullRequestCount int             `json:"openPullRequestCount"`
	WikiEnabled          bool            `json:"wikiEnabled"`
	// ExternalTrackerURL and ExternalWikiURL are set when the issues or the
	// wiki are kept elsewhere, for viewers who can read the code.
	ExternalTrackerURL string `json:"externalTrackerURL,omitempty"`
	ExternalWikiURL    string `json:"externalWikiURL,omitempty"`

	// ViewerCanRead is false for visitors who only see the public issues or
	// wiki of a repository.
	ViewerCanRead       bool `json:"viewerCanRead"`
	ViewerCanAdminister bool `json:"viewerCanAdminister"`
	ViewerCanWrite      bool `json:"viewerCanWrite"`
	ViewerIsWatching    bool `json:"viewerIsWatching"`
	ViewerIsStarring    bool `json:"viewerIsStarring"`
	// DefaultBranch and CloneLinks are set for viewers who can read the
	// code.
	DefaultBranch string          `json:"defaultBranch,omitempty"`
	CloneLinks    *repoCloneLinks `json:"cloneLinks,omitempty"`
}

type repoHeaderBase struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

func getRepoHeader(c flamego.Context, auth conf.AuthOpts, picture conf.PictureOpts, sshOpts conf.SSHOpts, repoOpts conf.RepositoryOpts, links database.LinkOptions, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoHeader, err error) {
	owner := repoCtx.Owner
	repo := repoCtx.Repo

	issuesEnabled := repo.EnableIssues
	wikiEnabled := repo.EnableWiki
	if !repoCtx.Viewer.CanRead() {
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
		AvatarURL:            cmp.Or(repo.AvatarLink(links, picture.RepositoryAvatarUploadPath), owner.AvatarURL(links)),
		Visibility:           visibility,
		WatchCount:           repo.NumWatches,
		StarCount:            repo.NumStars,
		ForkCount:            repo.NumForks,
		IssuesEnabled:        issuesEnabled,
		OpenIssueCount:       repo.NumIssues - repo.NumClosedIssues,
		PullRequestsEnabled:  repo.AllowsPulls(),
		OpenPullRequestCount: repo.NumPulls - repo.NumClosedPulls,
		WikiEnabled:          wikiEnabled,

		ViewerCanRead:       repoCtx.Viewer.CanRead(),
		ViewerCanAdminister: repoCtx.Viewer.CanAdminister(),
		ViewerCanWrite:      repoCtx.Viewer.CanWrite(),
		ViewerIsWatching:    database.Handle.Repositories().IsWatching(c.Request().Context(), repoCtx.Viewer.ID(), repo.ID),
		ViewerIsStarring:    database.Handle.Repositories().IsStarring(c.Request().Context(), repoCtx.Viewer.ID(), repo.ID),
	}
	if repoCtx.Viewer.CanRead() {
		resp.DefaultBranch = repo.DefaultBranch
		resp.CloneLinks = repoCloneLinksOf(links, repoOpts.DisableHTTPGit, sshOpts.Disabled, owner.Name, repo.Name, false)
		resp.ExternalTrackerURL = repo.ExternalTrackerLink()
		resp.ExternalWikiURL = repo.ExternalWikiLink()

		if repo.IsFork {
			resp.ForkOf, err = readableBaseRepo(c.Request().Context(), logger, auth.RequireEmailConfirmation, repoCtx)
			if err != nil {
				logger.Error("Failed to get base repository", "repoID", repo.ID, "error", err)
				return http.StatusInternalServerError, nil, errors.Wrap(err, "get base repository")
			}
		}
	}

	if repo.IsMirror {
		mirror, err := database.Handle.Mirrors().GetByRepoID(c.Request().Context(), repo.ID)
		if err != nil {
			logger.Error("Failed to get mirror by repository ID", "repoID", repo.ID, "error", err)
		} else if mirror != nil {
			resp.MirrorOf = mirror.Address(repoOpts.Root)
		}
	}

	return http.StatusOK, resp, nil
}

// readableBaseRepo returns the base repository of the fork, or nil when it is
// gone or the viewer cannot read it.
func readableBaseRepo(ctx context.Context, logger *logx.Logger, requireEmailConfirmation bool, repoCtx *repoContext) (*repoHeaderBase, error) {
	base, err := database.Handle.Repositories().GetByID(ctx, repoCtx.Repo.ForkID)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get repository")
	}
	if err = base.GetOwner(); err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	if !newViewer(ctx, logger, requireEmailConfirmation, repoCtx.Viewer.User, base.Owner, base).CanRead() {
		return nil, nil
	}
	return &repoHeaderBase{Owner: base.Owner.Name, Name: base.Name}, nil
}

type repoCommitSignature struct {
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	When       time.Time `json:"when"`
	AvatarURL  string    `json:"avatarURL"`
	ProfileURL string    `json:"profileURL,omitempty"`
}

type repoCommit struct {
	SHA string `json:"sha"`
	// Subject is the first line of the message as plain text, for the page
	// title.
	Subject string `json:"subject"`
	// SubjectHTML and BodyHTML are the first line and the rest of the message,
	// with links to the users and issues they mention.
	SubjectHTML string              `json:"subjectHTML"`
	BodyHTML    string              `json:"bodyHTML"`
	Author      repoCommitSignature `json:"author"`
	Parents     []string            `json:"parents"`
	// MarkdownExtensions are the extensions of files that show a rich diff.
	MarkdownExtensions []string `json:"markdownExtensions"`
}

func getRepoCommit(logger *logx.Logger, c flamego.Context, markupOpts markup.Options, markdown conf.MarkdownOpts, links database.LinkOptions, repoOpts conf.RepositoryOpts, repoCtx *repoContext) (statusCode int, resp *repoCommit, err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}

	ctx := c.Request().Context()
	owner := repoCtx.Owner
	repo := repoCtx.Repo
	commitID := c.Param("sha")

	commit, err := git.RepoCatFileCommit(ctx, repox.RepositoryPath(repoOpts.Root, owner.Name, repo.Name), commitID)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, nil
		}
		logger.Error("Failed to get commit", "commitID", commitID, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "cat-file commit")
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			logger.Error("Failed to get commit parent ID", "parentIndex", i, "commitID", commitID, "ownerName", owner.Name, "repoName", repo.Name, "error", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "parent ID")
		}
		parents[i] = sha.String()
	}

	toSignature := func(s *git.Signature) repoCommitSignature {
		sig := repoCommitSignature{
			Name:      s.Name,
			Email:     s.Email,
			When:      s.When.UTC(),
			AvatarURL: userx.GeneratedAvatarURLPath(links.SecretKey, links.Subpath, s.Email),
		}
		if u, err := database.Handle.Users().GetByEmail(ctx, s.Email); err == nil && u != nil {
			sig.ProfileURL = u.HomeURLPath(links.Subpath)
		}
		return sig
	}

	subject := commit.Summary()
	var body string
	if msg := commit.Message; len(msg) > len(subject) {
		body = msg[len(subject):]
	}

	metas := repo.ComposeMetas(markupOpts.Subpath, repoOpts.Root)
	return http.StatusOK, &repoCommit{
		SHA:                commitID,
		Subject:            subject,
		SubjectHTML:        markup.RenderCommitMessage(markupOpts, subject, repo.Link(markupOpts.Subpath), metas),
		BodyHTML:           markup.RenderCommitMessage(markupOpts, strings.TrimSpace(body), repo.Link(markupOpts.Subpath), metas),
		Author:             toSignature(commit.Author),
		Parents:            parents,
		MarkdownExtensions: markdown.FileExtensions,
	}, nil
}

type repoMarkdownFile struct {
	HTML string `json:"html"`
}

// getRepoMarkdownFile renders the Markdown file at the path in the commit the
// way the file view does, e.g., to show the rich diff of a commit.
func getRepoMarkdownFile(c flamego.Context, markupOpts markup.Options, markdown conf.MarkdownOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoMarkdownFile, err error) {
	ctx := c.Request().Context()
	repo := repoCtx.Repo
	sha := c.Param("sha")
	treePath := pathx.Clean(c.Param("path"))
	if !markup.IsMarkdownFile(markdown.FileExtensions, treePath) {
		return http.StatusNotFound, nil, errors.New("file is not Markdown")
	}

	gitRepo, err := openRepoGit(repoOpts.Root, repoCtx)
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "open repository")
	}
	commit, err := gitRepo.CatFileCommit(ctx, sha)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return http.StatusNotFound, nil, errors.New("commit does not exist")
		}
		logger.Error("Failed to get commit", "commitID", sha, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get commit")
	}
	blob, err := commit.Blob(ctx, treePath)
	if err != nil {
		if git.IsErrRevisionNotExist(err) || errors.Is(err, git.ErrNotBlob) {
			return http.StatusNotFound, nil, errors.New("file does not exist")
		}
		logger.Error("Failed to get blob", "commitID", sha, "treePath", treePath, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get blob")
	}
	p, err := blob.Bytes(ctx)
	if err != nil {
		logger.Error("Failed to read blob", "commitID", sha, "treePath", treePath, "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "read blob")
	}

	fileLink := repo.Link(markupOpts.Subpath) + "/src/" + commit.ID.String() + "/" + escapePath(treePath)
	return http.StatusOK, &repoMarkdownFile{
		HTML: markAppLinks(markupOpts, markup.MarkdownFile(markupOpts, p, path.Dir(fileLink), repo.ComposeMetas(markupOpts.Subpath, repoOpts.Root)), repo),
	}, nil
}

type repoWatchResponse struct {
	WatchCount int `json:"watchCount"`
}

func repoWatchAction(ctx context.Context, logger *logx.Logger, repoCtx *repoContext, watching bool) (statusCode int, resp *repoWatchResponse, err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if !repoCtx.Viewer.Authenticated() {
		return http.StatusUnauthorized, nil, errors.New("authentication required")
	}

	repo := repoCtx.Repo

	if watching {
		err = database.Handle.Repositories().Watch(ctx, repoCtx.Viewer.ID(), repo.ID)
	} else {
		err = database.Handle.Repositories().Unwatch(ctx, repoCtx.Viewer.ID(), repo.ID)
	}
	if err != nil {
		logger.Error("Failed to update repository watch status", "watching", watching, "userID", repoCtx.Viewer.ID(), "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "watch repo")
	}

	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		logger.Error("Failed to reload repository", "repoID", repo.ID, "repoName", repo.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoWatchResponse{
		WatchCount: updated.NumWatches,
	}, nil
}

func postRepoWatch(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	return repoWatchAction(c.Request().Context(), logger, repoCtx, true)
}

func deleteRepoWatch(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWatchResponse, err error) {
	return repoWatchAction(c.Request().Context(), logger, repoCtx, false)
}

type repoStarResponse struct {
	StarCount int `json:"starCount"`
}

func repoStarAction(ctx context.Context, logger *logx.Logger, repoCtx *repoContext, starring bool) (statusCode int, resp *repoStarResponse, err error) {
	if !repoCtx.Viewer.CanRead() {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}
	if !repoCtx.Viewer.Authenticated() {
		return http.StatusUnauthorized, nil, errors.New("authentication required")
	}

	repo := repoCtx.Repo

	if starring {
		err = database.Handle.Repositories().Star(ctx, repoCtx.Viewer.ID(), repo.ID)
	} else {
		err = database.Handle.Repositories().Unstar(ctx, repoCtx.Viewer.ID(), repo.ID)
	}
	if err != nil {
		logger.Error("Failed to update repository star status", "starred", starring, "userID", repoCtx.Viewer.ID(), "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "star repo")
	}

	updated, err := database.Handle.Repositories().GetByName(ctx, repo.OwnerID, repo.Name)
	if err != nil {
		logger.Error("Failed to reload repository", "repoID", repo.ID, "repoName", repo.Name, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reload repo")
	}
	return http.StatusOK, &repoStarResponse{
		StarCount: updated.NumStars,
	}, nil
}

func postRepoStar(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoStarResponse, err error) {
	return repoStarAction(c.Request().Context(), logger, repoCtx, true)
}

func deleteRepoStar(logger *logx.Logger, c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoStarResponse, err error) {
	return repoStarAction(c.Request().Context(), logger, repoCtx, false)
}
