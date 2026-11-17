package web

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
)

type repoMentionSuggestion struct {
	Name      string `json:"name"`
	FullName  string `json:"fullName"`
	AvatarURL string `json:"avatarURL"`
}

type repoMentionSuggestions struct {
	Items []repoMentionSuggestion `json:"items"`
}

// getRepoMentionSuggestions lists the users to suggest when mentioning someone
// in the Markdown of the repository, i.e., the users who can read it.
func getRepoMentionSuggestions(logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoMentionSuggestions, err error) {
	users, err := repoCtx.Repo.GetAssignees()
	if err != nil {
		logger.Error("Failed to list repository users", "repoID", repoCtx.Repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list users")
	}
	resp = &repoMentionSuggestions{Items: make([]repoMentionSuggestion, 0, len(users))}
	for _, u := range users {
		resp.Items = append(resp.Items, repoMentionSuggestion{Name: u.Name, FullName: u.FullName, AvatarURL: u.AvatarURLPath()})
	}
	return http.StatusOK, resp, nil
}

type repoIssueSuggestion struct {
	Index    int64  `json:"index"`
	Title    string `json:"title"`
	IsPull   bool   `json:"isPull"`
	IsClosed bool   `json:"isClosed"`
}

type repoIssueSuggestions struct {
	Items []repoIssueSuggestion `json:"items"`
}

// getRepoIssueSuggestions lists the issues and pull requests the viewer can see
// to suggest when referencing one by the `q` query parameter. The repository
// is not found when it uses an external issue tracker, because the numbers in
// its Markdown link there.
func getRepoIssueSuggestions(c flamego.Context, logger *logx.Logger, repoCtx *repoContext) (statusCode int, resp *repoIssueSuggestions, err error) {
	repo := repoCtx.Repo
	includeIssues := repo.EnableIssues && (repoCtx.Viewer.CanRead() || repo.CanGuestViewIssues())
	includePulls := repoCtx.Viewer.CanRead() && repo.AllowsPulls()
	if repo.EnableExternalTracker || (!includeIssues && !includePulls) {
		return http.StatusNotFound, nil, errors.New("repository does not exist")
	}

	issues, err := database.SuggestIssues(database.SuggestIssuesOptions{
		RepoID:        repo.ID,
		Keyword:       c.Query("q"),
		IncludeIssues: includeIssues,
		IncludePulls:  includePulls,
		Limit:         10,
	})
	if err != nil {
		logger.Error("Failed to suggest issues", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "suggest issues")
	}
	resp = &repoIssueSuggestions{Items: make([]repoIssueSuggestion, 0, len(issues))}
	for _, issue := range issues {
		resp.Items = append(resp.Items, repoIssueSuggestion{Index: issue.Index, Title: issue.Title, IsPull: issue.IsPull, IsClosed: issue.IsClosed})
	}
	return http.StatusOK, resp, nil
}
