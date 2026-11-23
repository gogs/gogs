package web

import (
	"bytes"
	stdctx "context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/unknwon/com"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/mimex"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
	"gogs.io/gogs/internal/userx"
)

// webAppPages is the web app, which renders the pages of the site.
type webAppPages struct {
	http.Handler
	// isProdMode hides the errors of the error page from viewers who are not
	// site admins.
	isProdMode bool
	// version is the version of Gogs that site admins see on the error page.
	version string
}

// NotFound renders the page that is not found, with a 404 status, like
// visiting a path that is no page at all.
func (p webAppPages) NotFound(c flamego.Context) {
	r := c.Request().Request
	wc := context.WebContextFrom(r)
	wc.StatusCode = http.StatusNotFound
	p.ServeHTTP(c.ResponseWriter(), r.WithContext(stdctx.WithValue(r.Context(), context.WebContextKey{}, wc)))
}

// Error renders the page of a server error, with a 500 status. Only site
// admins, or anyone outside production, see the error.
func (p webAppPages) Error(w http.ResponseWriter, r *http.Request, viewer *database.User, err error) {
	wc := context.WebContextFrom(r)
	wc.StatusCode = http.StatusInternalServerError
	wc.Page = context.WebPageServerError
	isAdmin := viewer != nil && viewer.IsAdmin
	if !p.isProdMode || isAdmin {
		wc.Error = err.Error()
	}
	if isAdmin {
		wc.Version = p.version
	}
	p.ServeHTTP(w, r.WithContext(stdctx.WithValue(r.Context(), context.WebContextKey{}, wc)))
}

// getAttachment serves an attachment of an issue, a comment or a release to
// viewers who can read the repository.
func getAttachment(c flamego.Context, att conf.AttachmentOpts, logger *logx.Logger, viewer *database.User, pages webAppPages) {
	w := c.ResponseWriter()
	ctx := c.Request().Context()
	attach, err := database.Handle.Attachments().GetByUUID(ctx, c.Param("uuid"))
	if err != nil {
		if errx.IsNotFound(err) {
			pages.NotFound(c)
			return
		}
		logger.Error("Failed to get attachment by UUID", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get attachment"))
		return
	}

	// Resolve the repository that owns this attachment so we can enforce
	// repository-level read permission. Without this check, anyone with the UUID
	// could download files belonging to private repositories.
	var repo *database.Repository
	switch {
	case attach.IssueID > 0:
		issue, err := database.Handle.Issues().GetByID(ctx, attach.IssueID)
		if err != nil {
			if errx.IsNotFound(err) {
				pages.NotFound(c)
				return
			}
			logger.Error("Failed to get issue by ID", "issueID", attach.IssueID, "error", err)
			writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get issue"))
			return
		}
		repo = issue.Repo
	case attach.ReleaseID > 0:
		release, err := database.Handle.Releases().GetByID(ctx, attach.ReleaseID)
		if err == nil {
			err = database.Handle.Releases().LoadAttributes(ctx, release)
		}
		if err != nil {
			if errx.IsNotFound(err) {
				pages.NotFound(c)
				return
			}
			logger.Error("Failed to get release by ID", "releaseID", attach.ReleaseID, "error", err)
			writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get release"))
			return
		}
		repo = release.Repo
	}
	var viewerID int64
	if viewer != nil {
		viewerID = viewer.ID
	}
	// A file that is not attached was either just uploaded, e.g., to preview it
	// before the issue or comment is created, or left out of a release by an
	// earlier edit. Only its uploader can download it.
	if (repo == nil && (viewerID == 0 || attach.UploaderID != viewerID)) ||
		(repo != nil && !repo.HasAccess(logger, viewerID)) ||
		!osx.IsFile(attach.LocalPath(att.Path)) {
		pages.NotFound(c)
		return
	}

	fr, err := os.Open(attach.LocalPath(att.Path))
	if err != nil {
		logger.Error("Failed to open attachment file", "attachmentID", attach.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open attachment"))
		return
	}
	defer func() { _ = fr.Close() }()

	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "private,max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, attach.Name))
	if _, err = io.Copy(w, fr); err != nil {
		logger.Error("Failed to copy attachment to response", "attachmentID", attach.ID, "error", err)
	}
}

// getUserAvatar serves the uploaded or generated avatar of a user or an
// organization. Unlike the pages, it does not turn away viewers who cannot sign
// in or have not activated their account, because the pages that tell them so
// show the avatar of the viewer.
func getUserAvatar(c flamego.Context, server conf.ServerOpts, picture conf.PictureOpts, auth conf.AuthOpts, logger *logx.Logger, viewer *database.User, pages webAppPages) {
	if viewer == nil && auth.RequireSigninView {
		pages.NotFound(c)
		return
	}

	id := c.ParamInt64("id")
	// The uploaded avatar file also holds identicons saved by earlier versions.
	avatarPath := userx.CustomAvatarPath(picture.AvatarUploadPath, id)
	if !osx.IsFile(avatarPath) {
		avatarPath = userx.GeneratedAvatarPath(server.AppDataPath, id, avatar.Person)
	}
	if !osx.IsFile(avatarPath) {
		avatarPath = userx.GeneratedAvatarPath(server.AppDataPath, id, avatar.Organization)
	}
	if !osx.IsFile(avatarPath) {
		u, err := database.Handle.Users().GetByID(c.Request().Context(), id)
		if err != nil {
			if errx.IsNotFound(err) {
				pages.NotFound(c)
				return
			}
			logger.Error("Failed to get user by ID", "userID", id, "error", err)
			writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get user"))
			return
		}
		kind := avatar.Person
		if u.IsOrganization() {
			kind = avatar.Organization
		}
		avatarPath = userx.GeneratedAvatarPath(server.AppDataPath, id, kind)
		if err = userx.SaveGeneratedAvatar(server.AppDataPath, id, kind); err != nil {
			logger.Error("Failed to save generated user avatar", "userID", id, "error", err)
			writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, err)
			return
		}
	}
	if err := serveAvatar(c, pages, avatarPath); err != nil {
		logger.Error("Failed to serve user avatar", "userID", id, "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, err)
	}
}

// getGeneratedAvatar serves the avatar generated from a seed, which is used for
// people who have no user, e.g., commit authors. It is not saved, because
// anyone can request any seed.
func getGeneratedAvatar(c flamego.Context, app conf.AppOpts, auth conf.AuthOpts, logger *logx.Logger, viewer *database.User, pages webAppPages) {
	if viewer == nil && auth.RequireSigninView {
		pages.NotFound(c)
		return
	}
	seed := c.Param("seed")
	if len(seed) > 64 {
		pages.NotFound(c)
		return
	}
	if err := serveGeneratedAvatar(c, app.Version, seed); err != nil {
		logger.Error("Failed to generate avatar", "seed", seed, "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, err)
	}
}

func serveGeneratedAvatar(c flamego.Context, version, seed string) error {
	// The avatar depends only on the seed and the version that draws it, so a
	// revalidation is answered without drawing it again.
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(version+"\x00"+seed)))
	h := c.ResponseWriter().Header()
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("ETag", etag)
	if strings.Contains(c.Request().Header.Get("If-None-Match"), etag) {
		c.ResponseWriter().WriteHeader(http.StatusNotModified)
		return nil
	}

	img, err := avatar.GeneratePNG(avatar.Person, seed, avatar.DefaultSize)
	if err != nil {
		return errors.Wrap(err, "generate avatar")
	}
	h.Set("Content-Type", "image/png")
	http.ServeContent(c.ResponseWriter(), c.Request().Request, "", time.Time{}, bytes.NewReader(img))
	return nil
}

// getRepoAvatar serves the uploaded avatar of a repository to the viewers who
// can read the repository.
func getRepoAvatar(c flamego.Context, picture conf.PictureOpts, auth conf.AuthOpts, logger *logx.Logger, viewer *database.User, pages webAppPages) {
	if viewer == nil && auth.RequireSigninView {
		pages.NotFound(c)
		return
	}

	ctx := c.Request().Context()
	id := c.ParamInt64("id")
	repo, err := database.Handle.Repositories().GetByID(ctx, id)
	if err != nil {
		if errx.IsNotFound(err) {
			pages.NotFound(c)
			return
		}
		logger.Error("Failed to get repository by ID", "repoID", id, "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get repository"))
		return
	}
	owner, err := database.Handle.Users().GetByID(ctx, repo.OwnerID)
	if err != nil {
		logger.Error("Failed to get repository owner by ID", "repoID", id, "ownerID", repo.OwnerID, "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get owner"))
		return
	}
	if !newViewer(ctx, logger, auth.RequireEmailConfirmation, viewer, owner, repo).CanRead() {
		pages.NotFound(c)
		return
	}
	if err := serveAvatar(c, pages, repo.CustomAvatarPath(picture.RepositoryAvatarUploadPath)); err != nil {
		logger.Error("Failed to serve repository avatar", "repoID", id, "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, err)
	}
}

// serveAvatar serves the avatar file at the path, or the page that is not
// found when there is no such file. It returns the error of opening an
// existing file.
func serveAvatar(c flamego.Context, pages webAppPages, avatarPath string) error {
	fi, err := os.Stat(avatarPath)
	if err != nil || !fi.Mode().IsRegular() {
		pages.NotFound(c)
		return nil
	}

	fr, err := os.Open(avatarPath)
	if err != nil {
		return errors.Wrap(err, "open avatar")
	}
	defer func() { _ = fr.Close() }()

	// Avatars can be private to authenticated viewers or to the readers of a
	// repository, so shared caches must not keep them.
	c.ResponseWriter().Header().Set("Cache-Control", "private")
	http.ServeContent(c.ResponseWriter(), c.Request().Request, "", fi.ModTime(), fr)
	return nil
}

// getUserPublicKeys serves the public SSH keys of a user as plain text.
func getUserPublicKeys(c flamego.Context, logger *logx.Logger, pages webAppPages) {
	w := c.ResponseWriter()
	name := c.Param("username")
	user, err := database.Handle.Users().GetByUsername(c.Request().Context(), name)
	if err != nil {
		if errx.IsNotFound(err) {
			pages.NotFound(c)
			return
		}
		logger.Error("Failed to get user by name", "userName", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get user"))
		return
	}

	keys, err := database.Handle.PublicKeys().List(c.Request().Context(), user.ID)
	if err != nil {
		logger.Error("Failed to list public keys", "userID", user.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "list public keys"))
		return
	}
	var buf bytes.Buffer
	for _, key := range keys {
		buf.WriteString(key.OmitEmail())
		buf.WriteString("\n")
	}
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// activateEmail activates the email address of an activation link, and sends
// the user to their email settings, which confirm it.
func activateEmail(c flamego.Context, sec conf.SecurityOpts, auth conf.AuthOpts, server conf.ServerOpts, logger *logx.Logger) {
	ctx := c.Request().Context()
	if email := verifyActivateEmailCode(ctx, logger, sec.SecretKey, auth.ActivateCodeLives, c.Query("code"), c.Query("email")); email != nil {
		if err := database.Handle.Users().MarkEmailActivated(ctx, email.UserID, email.Email); err != nil {
			logger.Error("Failed to mark email activated", "userID", email.UserID, "email", email.Email, "error", err)
			writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "mark email activated"))
			return
		}
		logger.Debug("Email activated", "email", email.Email)
		c.Redirect(server.Subpath + "/user/settings/email?activated=1")
		return
	}
	c.Redirect(server.Subpath + "/user/settings/email")
}

// verifyActivateEmailCode returns the email address the code activates, or nil
// when the code is not valid for it, i.e., not signed with secretKey or older
// than minutes.
func verifyActivateEmailCode(ctx stdctx.Context, logger *logx.Logger, secretKey string, minutes int, code, email string) *database.EmailAddress {
	user := parseUserFromCode(ctx, logger, code)
	if user == nil || !userx.VerifyConfirmEmailCode(secretKey, minutes, code, user.ID, email, user.LowerName, user.Password, user.Rands) {
		return nil
	}
	emailAddress, err := database.Handle.Users().GetEmail(ctx, user.ID, email, false)
	if err != nil {
		return nil
	}
	return emailAddress
}

// email2User sends the viewer to the profile of the user with the email
// address in the query.
func email2User(c flamego.Context, server conf.ServerOpts, logger *logx.Logger, pages webAppPages) {
	u, err := database.Handle.Users().GetByEmail(c.Request().Context(), c.Query("email"))
	if err != nil {
		if errx.IsNotFound(err) {
			pages.NotFound(c)
			return
		}
		logger.Error("Failed to get user by email", "error", err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get user"))
		return
	}
	c.Redirect(u.HomeURLPath(server.Subpath))
}

// getRepoArchive serves the archive of a branch, a tag or a commit, e.g.,
// "master.zip".
func getRepoArchive(c flamego.Context, server conf.ServerOpts, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext, pages webAppPages) {
	w := c.ResponseWriter()
	repo := repoCtx.Repo
	if !repoCtx.Viewer.CanRead() || repo.IsEmpty {
		pages.NotFound(c)
		return
	}

	gitRepo, err := git.Open(repo.RepoPath(repoOpts.Root))
	if err != nil {
		logger.Error("Failed to open repository", "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open repository"))
		return
	}

	archivePath, name, err := repox.PrepareArchive(c.Request().Context(), server.AppDataPath, gitRepo, repo.Name, c.Param("**"))
	if err != nil {
		if errx.IsNotFound(err) {
			pages.NotFound(c)
			return
		}
		logger.Error("Failed to prepare archive", "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "prepare archive"))
		return
	}
	repox.ServeArchive(w, c.Request().Request, archivePath, name)
}

// isWikiPageName returns true if the name in a wiki path is a page, which the
// web app renders, rather than another file of the wiki, e.g., an image.
func isWikiPageName(name string) bool {
	ext := path.Ext(name)
	return ext == "" || strings.EqualFold(ext, ".md")
}

// getRepoWikiFile serves a file of the wiki that is not a page, e.g., an image
// that a page links to relatively. It writes nothing when the wiki has no such
// file, so the web app renders the page of that name.
func getRepoWikiFile(c flamego.Context, repoOpts conf.RepositoryOpts, logger *logx.Logger, repoCtx *repoContext) {
	w := c.ResponseWriter()
	repo := repoCtx.Repo
	name := c.Param("page")
	if !repo.HasWikiPages(c.Request().Context(), repoOpts.Root) {
		return
	}

	ctx := c.Request().Context()
	wikiPath := repo.WikiPath(repoOpts.Root)
	wikiRepo, err := git.Open(wikiPath)
	if err != nil {
		logger.Error("Failed to open wiki repository", "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "open wiki repository"))
		return
	}
	commit, err := wikiRepo.BranchCommit(ctx, database.WikiBranch(ctx, wikiPath))
	if err != nil {
		logger.Error("Failed to get wiki branch commit", "repoID", repo.ID, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get wiki branch commit"))
		return
	}
	blob, err := commit.Blob(ctx, name)
	if err != nil {
		if git.IsErrRevisionNotExist(err) {
			return
		}
		logger.Error("Failed to get wiki file", "repoID", repo.ID, "fileName", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "get wiki file"))
		return
	}
	data, err := blob.Bytes(ctx)
	if err != nil {
		logger.Error("Failed to read wiki file", "repoID", repo.ID, "fileName", name, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "read wiki file"))
		return
	}

	// Like raw repository files, only images are shown inline, and text is
	// served as plain text so it never runs as a page of this site.
	if !mimex.IsText(data) {
		if !mimex.IsImage(data) {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
			w.Header().Set("Content-Transfer-Encoding", "binary")
		}
	} else if !repoOpts.EnableRawFileRenderMode || !c.QueryBool("render") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// serveGoGet answers "go get" requests for a repository with the go-import
// meta, whether or not the viewer may see the repository, or the repository
// exists at all, because "go get" does not send credentials from ".netrc". It
// writes nothing for other requests.
func serveGoGet(c flamego.Context, server conf.ServerOpts) {
	if c.Query("go-get") != "1" {
		return
	}

	ownerName := c.Param("owner")
	repoName := c.Param("repo")
	branchName := "master"
	owner, err := database.Handle.Users().GetByUsername(c.Request().Context(), ownerName)
	if err == nil {
		repo, err := database.Handle.Repositories().GetByName(c.Request().Context(), owner.ID, repoName)
		if err == nil && repo.DefaultBranch != "" {
			branchName = repo.DefaultBranch
		}
	}

	prefix := server.ExternalURL + path.Join(ownerName, repoName, "src", branchName)
	insecureFlag := ""
	if !strings.HasPrefix(server.ExternalURL, "https://") {
		insecureFlag = "--insecure "
	}
	w := c.ResponseWriter()
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(com.Expand(`<!doctype html>
<html>
	<head>
		<meta name="go-import" content="{GoGetImport} git {CloneLink}">
		<meta name="go-source" content="{GoGetImport} _ {GoDocDirectory} {GoDocFile}">
	</head>
	<body>
		go get {InsecureFlag}{GoGetImport}
	</body>
</html>
`,
		map[string]string{
			"GoGetImport":    path.Join(server.URL.Host, server.Subpath, ownerName, repoName),
			"CloneLink":      repox.HTTPSCloneURL(server.ExternalURL, ownerName, repoName),
			"GoDocDirectory": prefix + "{/dir}",
			"GoDocFile":      prefix + "{/dir}/{file}#L{line}",
			"InsecureFlag":   insecureFlag,
		},
	)))
}
