package repo

import (
	"bytes"
	gocontext "context"
	"path"
	"time"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/gitx"
	"gogs.io/gogs/internal/tool"
)

const (
	COMMITS = "repo/commits"
	DIFF    = "repo/diff/page"
)

func RefCommits(c *context.Context) {
	c.Data["PageIsViewFiles"] = true
	switch c.Repo.TreePath {
	case "":
		Commits(c)
	case "search":
		SearchCommits(c)
	default:
		FileHistory(c)
	}
}

// TODO(unknwon)
func RenderIssueLinks(oldCommits []*git.Commit, _ string) []*git.Commit {
	return oldCommits
}

func renderCommits(c *context.Context, filename string) {
	c.Data["Title"] = c.Tr("repo.commits.commit_history") + " · " + c.Repo.Repository.FullName()
	c.Data["PageIsCommits"] = true
	c.Data["FileName"] = filename

	page := max(c.QueryInt("page"), 1)
	pageSize := c.QueryInt("pageSize")
	if pageSize < 1 {
		pageSize = conf.UI.User.CommitsPagingNum
	}

	commits, err := c.Repo.Commit.CommitsByPage(page, pageSize, git.CommitsByPageOptions{Path: filename})
	if err != nil {
		c.Error(err, "paging commits")
		return
	}

	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	c.Data["Commits"] = matchUsersWithCommitEmails(c.Req.Context(), commits)

	if page > 1 {
		c.Data["HasPrevious"] = true
		c.Data["PreviousPage"] = page - 1
	}
	if len(commits) == pageSize {
		c.Data["HasNext"] = true
		c.Data["NextPage"] = page + 1
	}
	c.Data["PageSize"] = pageSize

	c.Data["Username"] = c.Repo.Owner.Name
	c.Data["Reponame"] = c.Repo.Repository.Name
	c.Success(COMMITS)
}

func Commits(c *context.Context) {
	renderCommits(c, "")
}

func SearchCommits(c *context.Context) {
	c.Data["PageIsCommits"] = true

	keyword := c.Query("q")
	if keyword == "" {
		c.Redirect(c.Repo.RepoLink + "/commits/" + c.Repo.BranchName)
		return
	}

	commits, err := c.Repo.Commit.SearchCommits(keyword)
	if err != nil {
		c.Error(err, "search commits")
		return
	}

	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	c.Data["Commits"] = matchUsersWithCommitEmails(c.Req.Context(), commits)

	c.Data["Keyword"] = keyword
	c.Data["Username"] = c.Repo.Owner.Name
	c.Data["Reponame"] = c.Repo.Repository.Name
	c.Data["Branch"] = c.Repo.BranchName
	c.Success(COMMITS)
}

func FileHistory(c *context.Context) {
	renderCommits(c, c.Repo.TreePath)
}

// tryGetUserByEmail returns a non-nil value if the email is corresponding to an
// existing user.
func tryGetUserByEmail(ctx gocontext.Context, email string) *database.User {
	user, _ := database.Handle.Users().GetByEmail(ctx, email)
	return user
}

func Diff(c *context.Context) {
	c.PageIs("Diff")
	c.RequireHighlightJS()

	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	commitID := c.Params(":sha")

	commit, err := c.Repo.GitRepo.CatFileCommit(commitID)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get commit by ID")
		return
	}

	diff, err := gitx.RepoDiff(c.Repo.GitRepo,
		commitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
		git.DiffOptions{Timeout: time.Duration(conf.Git.Timeout.Diff) * time.Second},
	)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get diff")
		return
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			c.NotFound()
			return
		}
		parents[i] = sha.String()
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.RawTitle(commit.Summary() + " · " + tool.ShortSHA1(commitID))
	c.Data["CommitID"] = commitID
	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["IsImageFileByIndex"] = commit.IsImageFileByIndex
	c.Data["Commit"] = commit
	c.Data["Author"] = tryGetUserByEmail(c.Req.Context(), commit.Author.Email)
	c.Data["Diff"] = diff
	c.Data["Parents"] = parents
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", commitID)
	c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", commitID)
	if commit.ParentsCount() > 0 {
		c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", parents[0])
		c.Data["BeforeRawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", parents[0])
	}
	c.Success(DIFF)
}

func RawDiff(c *context.Context) {
	if err := c.Repo.GitRepo.RawDiff(
		c.Params(":sha"),
		git.RawDiffFormat(c.Params(":ext")),
		c.Resp,
	); err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get raw diff")
		return
	}
}

type commitDiffSignature struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl"`
	UserPath  string `json:"userPath,omitempty"`
	When      string `json:"when"`
}

type commitDiffCommit struct {
	SHA       string              `json:"sha"`
	ShortSHA  string              `json:"shortSha"`
	Summary   string              `json:"summary"`
	Message   string              `json:"message"`
	Author    commitDiffSignature `json:"author"`
	Committer commitDiffSignature `json:"committer"`
	Parents   []string            `json:"parents"`
}

type commitDiffPayload struct {
	Commit     commitDiffCommit `json:"commit"`
	Patch      string           `json:"patch"`
	SourcePath string           `json:"sourcePath"`
	RawDiffURL string           `json:"rawDiffUrl"`
}

func DiffJSON(c *context.Context) {
	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	commitID := c.Params(":sha")

	commit, err := c.Repo.GitRepo.CatFileCommit(commitID)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get commit by ID")
		return
	}

	var buf bytes.Buffer
	if err := c.Repo.GitRepo.RawDiff(commitID, git.RawDiffNormal, &buf); err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get raw diff")
		return
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			c.NotFound()
			return
		}
		parents[i] = sha.String()
	}

	ctx := c.Req.Context()
	sig := func(s *git.Signature) commitDiffSignature {
		out := commitDiffSignature{
			Name:      s.Name,
			Email:     s.Email,
			AvatarURL: tool.AvatarLink(s.Email),
			When:      s.When.UTC().Format(time.RFC3339),
		}
		if u, err := database.Handle.Users().GetByEmail(ctx, s.Email); err == nil && u != nil {
			out.UserPath = conf.Server.Subpath + "/" + u.Name
		}
		return out
	}

	body := ""
	if msg := commit.Message; len(msg) > len(commit.Summary()) {
		body = msg[len(commit.Summary()):]
	}

	c.JSONSuccess(commitDiffPayload{
		Commit: commitDiffCommit{
			SHA:       commitID,
			ShortSHA:  tool.ShortSHA1(commitID),
			Summary:   commit.Summary(),
			Message:   body,
			Author:    sig(commit.Author),
			Committer: sig(commit.Committer),
			Parents:   parents,
		},
		Patch:      buf.String(),
		SourcePath: conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", commitID),
		RawDiffURL: conf.Server.Subpath + "/" + path.Join(userName, repoName, "commit", commitID+".diff"),
	})
}

type userCommit struct {
	User *database.User
	*git.Commit
}

// matchUsersWithCommitEmails matches existing users using commit author emails.
func matchUsersWithCommitEmails(ctx gocontext.Context, oldCommits []*git.Commit) []*userCommit {
	emailToUsers := make(map[string]*database.User)
	newCommits := make([]*userCommit, len(oldCommits))
	usersStore := database.Handle.Users()
	for i := range oldCommits {
		var u *database.User
		if v, ok := emailToUsers[oldCommits[i].Author.Email]; !ok {
			u, _ = usersStore.GetByEmail(ctx, oldCommits[i].Author.Email)
			emailToUsers[oldCommits[i].Author.Email] = u
		} else {
			u = v
		}

		newCommits[i] = &userCommit{
			User:   u,
			Commit: oldCommits[i],
		}
	}
	return newCommits
}

func CompareDiff(c *context.Context) {
	c.Data["IsDiffCompare"] = true
	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	beforeCommitID := c.Params(":before")
	afterCommitID := c.Params(":after")

	commit, err := c.Repo.GitRepo.CatFileCommit(afterCommitID)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get head commit")
		return
	}

	diff, err := gitx.RepoDiff(c.Repo.GitRepo,
		afterCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
		git.DiffOptions{Base: beforeCommitID, Timeout: time.Duration(conf.Git.Timeout.Diff) * time.Second},
	)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get diff")
		return
	}

	commits, err := commit.CommitsAfter(beforeCommitID)
	if err != nil {
		c.NotFoundOrError(gitx.NewError(err), "get commits after")
		return
	}

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["CommitRepoLink"] = c.Repo.RepoLink
	c.Data["Commits"] = matchUsersWithCommitEmails(c.Req.Context(), commits)
	c.Data["CommitsCount"] = len(commits)
	c.Data["BeforeCommitID"] = beforeCommitID
	c.Data["AfterCommitID"] = afterCommitID
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["IsImageFileByIndex"] = commit.IsImageFileByIndex
	c.Data["Title"] = "Comparing " + tool.ShortSHA1(beforeCommitID) + "..." + tool.ShortSHA1(afterCommitID) + " · " + userName + "/" + repoName
	c.Data["Commit"] = commit
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", afterCommitID)
	c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", afterCommitID)
	c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", beforeCommitID)
	c.Data["BeforeRawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", beforeCommitID)
	c.Success(DIFF)
}
