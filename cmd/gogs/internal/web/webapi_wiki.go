package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/repox"
)

// withRepoWiki lets through the viewers who can read the wiki of the
// repository. Signed-out visitors and others without access can read it when
// the repository allows public wikis. A repository that uses an external wiki
// has none here.
func withRepoWiki(c flamego.Context, repoCtx *repoContext) {
	repo := repoCtx.Repo
	if !repo.EnableWiki || repo.EnableExternalWiki || (!repoCtx.Viewer.CanRead() && !repo.CanGuestViewWiki()) {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("wiki does not exist"))
	}
}

// viewerCanWriteWiki reports whether the viewer can change the wiki. The wiki
// of a mirror follows its source, so it is not changed here.
func viewerCanWriteWiki(repoCtx *repoContext) bool {
	return repoCtx.Viewer.CanWrite() && !repoCtx.Repo.IsMirror
}

// withWikiWriter lets through the viewers who can change the wiki. Like the
// server-rendered pages, the wiki is not found for others.
func withWikiWriter(c flamego.Context, repoCtx *repoContext) {
	if !viewerCanWriteWiki(repoCtx) {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("wiki does not exist"))
	}
}

// wikiPageName returns the name of the page in the query, defaulting to the
// home page.
func wikiPageName(c flamego.Context) string {
	name := database.CleanWikiPageName(c.Query("page"))
	if name == "" {
		return "Home"
	}
	return name
}

// openWiki returns the wiki repository and the latest commit of its branch.
func openWiki(c flamego.Context, repo *database.Repository) (*git.Repository, string, *git.Commit, error) {
	wikiPath := repo.WikiPath()
	wikiRepo, err := git.Open(wikiPath)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "open wiki repository")
	}
	branch := database.WikiBranch(c.Request().Context(), wikiPath)
	commit, err := wikiRepo.BranchCommit(c.Request().Context(), branch)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "get branch commit")
	}
	return wikiRepo, branch, commit, nil
}

// wikiPageNames returns the names of the pages at the commit, in the order of
// its tree.
func wikiPageNames(c flamego.Context, commit *git.Commit) ([]string, error) {
	entries, err := commit.Entries(c.Request().Context())
	if err != nil {
		return nil, errors.Wrap(err, "list entries")
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Type() == git.ObjectBlob && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	return names, nil
}

// readWikiPage returns the content of the page, and false when there is no
// such page.
func readWikiPage(c flamego.Context, commit *git.Commit, name string) ([]byte, bool, error) {
	blob, err := commit.Blob(c.Request().Context(), name+".md")
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return nil, false, nil
		}
		return nil, false, errors.Wrap(err, "get blob")
	}
	p, err := blob.Bytes(c.Request().Context())
	if err != nil {
		return nil, false, errors.Wrap(err, "read blob")
	}
	return p, true, nil
}

type wikiCloneLinks struct {
	HTTPS string `json:"https,omitempty"`
	SSH   string `json:"ssh,omitempty"`
}

type wikiPageAuthor struct {
	Name string    `json:"name"`
	When time.Time `json:"when"`
}

type wikiPage struct {
	Name   string         `json:"name"`
	HTML   string         `json:"html"`
	Author wikiPageAuthor `json:"author"`
}

type repoWikiResponse struct {
	// HasWiki is false until the first page is created, and the other fields
	// are then left out.
	HasWiki        bool            `json:"hasWiki"`
	Page           *wikiPage       `json:"page,omitempty"`
	Pages          []string        `json:"pages,omitempty"`
	CloneLinks     *wikiCloneLinks `json:"cloneLinks,omitempty"`
	ViewerCanWrite bool            `json:"viewerCanWrite"`
}

func getRepoWiki(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWikiResponse, err error) {
	repo := repoCtx.Repo
	resp = &repoWikiResponse{ViewerCanWrite: viewerCanWriteWiki(repoCtx)}
	if !repo.HasWiki() {
		return http.StatusOK, resp, nil
	}

	wikiRepo, branch, commit, err := openWiki(c, repo)
	if err != nil {
		log.Error("getRepoWiki: open wiki of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	name := wikiPageName(c)
	content, found, err := readWikiPage(c, commit, name)
	if err != nil {
		log.Error("getRepoWiki: read page %q of repo %d: %v", name, repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if !found {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	commits, err := wikiRepo.Log(c.Request().Context(), git.RefsHeads+branch, git.LogOptions{MaxCount: 1, Path: name + ".md"})
	if err != nil || len(commits) == 0 {
		log.Error("getRepoWiki: get last commit of page %q of repo %d: %v", name, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get last commit")
	}
	pages, err := wikiPageNames(c, commit)
	if err != nil {
		log.Error("getRepoWiki: list pages of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}

	links := repox.NewCloneLink(repoCtx.Owner.Name, repo.Name, true)
	resp.HasWiki = true
	resp.Page = &wikiPage{
		Name:   name,
		HTML:   string(markup.Markdown(content, repo.Link(), repo.ComposeMetas())),
		Author: wikiPageAuthor{Name: commits[0].Author.Name, When: commits[0].Author.When},
	}
	resp.Pages = pages
	resp.CloneLinks = &wikiCloneLinks{}
	if !conf.Repository.DisableHTTPGit {
		resp.CloneLinks.HTTPS = links.HTTPS
	}
	if !conf.SSH.Disabled {
		resp.CloneLinks.SSH = links.SSH
	}
	return http.StatusOK, resp, nil
}

type repoWikiPageItem struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type repoWikiPagesResponse struct {
	Pages          []repoWikiPageItem `json:"pages"`
	ViewerCanWrite bool               `json:"viewerCanWrite"`
}

func getRepoWikiPages(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWikiPagesResponse, err error) {
	repo := repoCtx.Repo
	resp = &repoWikiPagesResponse{Pages: []repoWikiPageItem{}, ViewerCanWrite: viewerCanWriteWiki(repoCtx)}
	if !repo.HasWiki() {
		return http.StatusOK, resp, nil
	}

	wikiRepo, branch, commit, err := openWiki(c, repo)
	if err != nil {
		log.Error("getRepoWikiPages: open wiki of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	names, err := wikiPageNames(c, commit)
	if err != nil {
		log.Error("getRepoWikiPages: list pages of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	for _, name := range names {
		commits, err := wikiRepo.Log(c.Request().Context(), git.RefsHeads+branch, git.LogOptions{MaxCount: 1, Path: name + ".md"})
		if err != nil || len(commits) == 0 {
			log.Error("getRepoWikiPages: get last commit of page %q of repo %d: %v", name, repo.ID, err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "get last commit")
		}
		resp.Pages = append(resp.Pages, repoWikiPageItem{Name: name, UpdatedAt: commits[0].Author.When})
	}
	return http.StatusOK, resp, nil
}

type repoWikiPageSource struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func getRepoWikiPageSource(c flamego.Context, repoCtx *repoContext) (statusCode int, resp *repoWikiPageSource, err error) {
	repo := repoCtx.Repo
	if !repo.HasWiki() {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	_, _, commit, err := openWiki(c, repo)
	if err != nil {
		log.Error("getRepoWikiPageSource: open wiki of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	name := wikiPageName(c)
	content, found, err := readWikiPage(c, commit, name)
	if err != nil {
		log.Error("getRepoWikiPageSource: read page %q of repo %d: %v", name, repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if !found {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	return http.StatusOK, &repoWikiPageSource{Name: name, Content: string(content)}, nil
}

type repoWikiPageRequest struct {
	// OldTitle is the page being changed, and is left out for a new page.
	OldTitle string `json:"oldTitle"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Message  string `json:"message"`
}

// wikiPageRequestError returns the response for a request that has no title or
// content, or nil. It trims the title.
func wikiPageRequestError(l i18n.Locale, req *repoWikiPageRequest) *bindingErrorResponse {
	req.Title = strings.TrimSpace(req.Title)
	if database.CleanWikiPageName(req.Title) == "" {
		msg := l.Tr("repo.wiki.page_title") + l.Tr("form.require_error")
		return &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}
	}
	if strings.TrimSpace(req.Content) == "" {
		msg := l.Tr("repo.wiki.page_content") + l.Tr("form.require_error")
		return &bindingErrorResponse{Fields: fieldErrors{"content": &msg}}
	}
	return nil
}

type repoWikiPageResult struct {
	// Name is the name the page was saved under.
	Name string `json:"name"`
}

func postRepoWikiPage(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoWikiPageRequest) (statusCode int, resp any, err error) {
	if resp := wikiPageRequestError(l, &req); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	repo := repoCtx.Repo
	if err = repo.AddWikiPage(c.Request().Context(), user, req.Title, req.Content, req.Message); err != nil {
		if database.IsErrWikiAlreadyExist(err) {
			msg := l.Tr("repo.wiki.page_already_exists")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}, nil
		}
		log.Error("postRepoWikiPage: add page %q to repo %d: %v", req.Title, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "add page")
	}
	return http.StatusCreated, &repoWikiPageResult{Name: database.CleanWikiPageName(req.Title)}, nil
}

func patchRepoWikiPage(c flamego.Context, l i18n.Locale, user *database.User, repoCtx *repoContext, req repoWikiPageRequest) (statusCode int, resp any, err error) {
	if resp := wikiPageRequestError(l, &req); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	repo := repoCtx.Repo
	if !repo.HasWiki() {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	_, _, commit, err := openWiki(c, repo)
	if err != nil {
		log.Error("patchRepoWikiPage: open wiki of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	_, found, err := readWikiPage(c, commit, database.CleanWikiPageName(req.OldTitle))
	if err != nil {
		log.Error("patchRepoWikiPage: read page %q of repo %d: %v", req.OldTitle, repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if !found {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}

	if err = repo.EditWikiPage(c.Request().Context(), user, req.OldTitle, req.Title, req.Content, req.Message); err != nil {
		if database.IsErrWikiAlreadyExist(err) {
			msg := l.Tr("repo.wiki.page_already_exists")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"title": &msg}}, nil
		}
		log.Error("patchRepoWikiPage: edit page %q of repo %d: %v", req.OldTitle, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "edit page")
	}
	return http.StatusOK, &repoWikiPageResult{Name: database.CleanWikiPageName(req.Title)}, nil
}

func deleteRepoWikiPage(c flamego.Context, user *database.User, repoCtx *repoContext) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	if !repo.HasWiki() {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	_, _, commit, err := openWiki(c, repo)
	if err != nil {
		log.Error("deleteRepoWikiPage: open wiki of repo %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	name := wikiPageName(c)
	_, found, err := readWikiPage(c, commit, name)
	if err != nil {
		log.Error("deleteRepoWikiPage: read page %q of repo %d: %v", name, repo.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if !found {
		return http.StatusNotFound, nil, errors.New("page does not exist")
	}
	if err = repo.DeleteWikiPage(c.Request().Context(), user, name); err != nil {
		log.Error("deleteRepoWikiPage: delete page %q of repo %d: %v", name, repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete page")
	}
	return http.StatusNoContent, nil, nil
}
