package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/tool"
)

// requireAdmin turns away users who are not site admins. Mount it after
// requireAuthenticated.
func requireAdmin(c flamego.Context, viewer *database.User) {
	if !viewer.IsAdmin {
		writeErrorResponse(c.ResponseWriter(), http.StatusForbidden, errors.New("admin required"))
	}
}

// adminListPage is one page of a list in the admin panel, with the number of
// all matches.
type adminListPage[T any] struct {
	*listPage[T]
	Total int64 `json:"total"`
}

func newAdminListPage[T any](items []T, page int, total int64, pageSize int) *adminListPage[T] {
	return &adminListPage[T]{listPage: newListPage(items, page, total, pageSize), Total: total}
}

// unixTime returns the time of a Unix timestamp column in UTC.
func unixTime(unix int64) time.Time {
	return time.Unix(unix, 0).UTC()
}

// searchAdminUsers lists users or organizations by ID, or searches them by
// name when the keyword is not empty, like the server-rendered admin pages.
func searchAdminUsers(ctx context.Context, typ database.UserType, keyword string, page, pageSize int) ([]*database.User, int64, error) {
	if keyword != "" {
		if typ == database.UserTypeOrganization {
			return database.Handle.Organizations().SearchByName(ctx, keyword, page, pageSize, "id ASC")
		}
		return database.Handle.Users().SearchByName(ctx, keyword, page, pageSize, "id ASC")
	}

	if typ == database.UserTypeOrganization {
		orgs, err := database.Handle.Organizations().ListByPage(ctx, page, pageSize)
		return orgs, database.Handle.Organizations().Count(ctx), err
	}
	users, err := database.Handle.Users().List(ctx, page, pageSize)
	return users, database.Handle.Users().Count(ctx), err
}

type adminUser struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatarURL"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"isActive"`
	IsAdmin   bool      `json:"isAdmin"`
	RepoCount int       `json:"repoCount"`
	CreatedAt time.Time `json:"createdAt"`
}

func getAdminUsers(logger *logx.Logger, c flamego.Context) (statusCode int, resp *adminListPage[adminUser], err error) {
	page, pageSize := pageParam(c), conf.UI.Admin.UserPagingNum
	users, total, err := searchAdminUsers(c.Request().Context(), database.UserTypeIndividual, c.Query("q"), page, pageSize)
	if err != nil {
		logger.Error("Failed to search users", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search users")
	}

	items := make([]adminUser, 0, len(users))
	for _, u := range users {
		items = append(items, adminUser{
			ID:        u.ID,
			Name:      u.Name,
			AvatarURL: u.AvatarURLPath(),
			Email:     u.Email,
			IsActive:  u.IsActive,
			IsAdmin:   u.IsAdmin,
			RepoCount: u.NumRepos,
			CreatedAt: unixTime(u.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

type adminOrg struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	AvatarURL   string    `json:"avatarURL"`
	TeamCount   int       `json:"teamCount"`
	MemberCount int       `json:"memberCount"`
	RepoCount   int       `json:"repoCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

func getAdminOrgs(logger *logx.Logger, c flamego.Context) (statusCode int, resp *adminListPage[adminOrg], err error) {
	page, pageSize := pageParam(c), conf.UI.Admin.OrgPagingNum
	orgs, total, err := searchAdminUsers(c.Request().Context(), database.UserTypeOrganization, c.Query("q"), page, pageSize)
	if err != nil {
		logger.Error("Failed to search organizations", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "search organizations")
	}

	items := make([]adminOrg, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, adminOrg{
			ID:          org.ID,
			Name:        org.Name,
			AvatarURL:   org.AvatarURLPath(),
			TeamCount:   org.NumTeams,
			MemberCount: org.NumMembers,
			RepoCount:   org.NumRepos,
			CreatedAt:   unixTime(org.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

type adminRepo struct {
	ID         int64  `json:"id"`
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"isPrivate"`
	WatchCount int    `json:"watchCount"`
	StarCount  int    `json:"starCount"`
	IssueCount int    `json:"issueCount"`
	// Size is the size of the repository on disk, formatted like "1.2 MB".
	Size      string    `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

func getAdminRepos(logger *logx.Logger, c flamego.Context) (statusCode int, resp *adminListPage[adminRepo], err error) {
	ctx := c.Request().Context()
	page, pageSize := pageParam(c), conf.UI.Admin.RepoPagingNum
	var (
		repos []*database.Repository
		total int64
	)
	if keyword := c.Query("q"); keyword == "" {
		repos, err = database.Handle.Repositories().ListByPage(ctx, page, pageSize)
		if err == nil {
			total, err = database.Handle.Repositories().Count(ctx, 0, true)
		}
	} else {
		repos, total, err = database.Handle.Repositories().Search(ctx, database.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  "id ASC",
			Private:  true,
			Page:     page,
			PageSize: pageSize,
		})
	}
	if err != nil {
		logger.Error("Failed to list repositories", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list repositories")
	}
	if err = database.Handle.Repositories().LoadAttributes(c.Request().Context(), repos...); err != nil {
		logger.Error("Failed to load repository attributes", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load repository attributes")
	}

	items := make([]adminRepo, 0, len(repos))
	for _, repo := range repos {
		items = append(items, adminRepo{
			ID:         repo.ID,
			Owner:      repo.Owner.Name,
			Name:       repo.Name,
			IsPrivate:  repo.IsPrivate,
			WatchCount: repo.NumWatches,
			StarCount:  repo.NumStars,
			IssueCount: repo.NumIssues,
			Size:       tool.FileSize(repo.Size),
			CreatedAt:  unixTime(repo.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, total, pageSize), nil
}

func deleteAdminRepo(logger *logx.Logger, c flamego.Context, viewer *database.User) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	repo, err := database.Handle.Repositories().GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrRepoNotExist(err) {
			return http.StatusNotFound, nil, errors.New("repository not found")
		}
		logger.Error("Failed to get repository", "repoID", id, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get repository")
	}
	if err = database.Handle.Repositories().Delete(ctx, logger, repo.OwnerID, repo.ID); err != nil {
		logger.Error("Failed to delete repository", "repoID", repo.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete repository")
	}
	logger.Debug("Repository deleted by admin", "repoID", repo.ID, "adminName", viewer.Name)
	return http.StatusNoContent, nil, nil
}

type adminNotice struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

func getAdminNotices(logger *logx.Logger, c flamego.Context, l i18n.Locale) (statusCode int, resp *adminListPage[adminNotice], err error) {
	ctx := c.Request().Context()
	page, pageSize := pageParam(c), conf.UI.Admin.NoticePagingNum
	notices, err := database.Handle.Notices().List(ctx, page, pageSize)
	if err != nil {
		logger.Error("Failed to list notices", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list notices")
	}

	items := make([]adminNotice, 0, len(notices))
	for _, n := range notices {
		items = append(items, adminNotice{
			ID:          n.ID,
			Type:        l.Tr(n.Type.TrStr()),
			Description: n.Description,
			CreatedAt:   unixTime(n.CreatedUnix),
		})
	}
	return http.StatusOK, newAdminListPage(items, page, database.Handle.Notices().Count(ctx), pageSize), nil
}

type adminNoticesDeleteRequest struct {
	IDs []int64 `json:"ids" validate:"required,min=1,dive,min=1"`
}

func deleteAdminNotices(logger *logx.Logger, c flamego.Context, req adminNoticesDeleteRequest) (statusCode int, resp any, err error) {
	if err = database.Handle.Notices().DeleteByIDs(c.Request().Context(), req.IDs...); err != nil {
		logger.Error("Failed to delete notices", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete notices")
	}
	return http.StatusNoContent, nil, nil
}

func deleteAllAdminNotices(logger *logx.Logger, c flamego.Context, viewer *database.User) (statusCode int, resp any, err error) {
	if err = database.Handle.Notices().DeleteAll(c.Request().Context()); err != nil {
		logger.Error("Failed to delete all notices", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete all notices")
	}
	logger.Debug("System notices deleted by admin", "adminName", viewer.Name)
	return http.StatusNoContent, nil, nil
}

type adminLoginSource struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type adminUserOptions struct {
	// LoginSources are the authentication sources a user can sign in through,
	// besides local accounts.
	LoginSources []adminLoginSource `json:"loginSources"`
	// CanSendEmail is true when the site can send email, e.g., to tell a new
	// user about their account.
	CanSendEmail bool `json:"canSendEmail"`
	// LocalPathMigrationEnabled is true when users may be allowed to import
	// repositories from the local disk.
	LocalPathMigrationEnabled bool `json:"localPathMigrationEnabled"`
}

func getAdminUserOptions(logger *logx.Logger, c flamego.Context) (statusCode int, resp *adminUserOptions, err error) {
	sources, err := database.Handle.LoginSources().List(c.Request().Context(), database.ListLoginSourceOptions{})
	if err != nil {
		logger.Error("Failed to list login sources", "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list login sources")
	}
	items := make([]adminLoginSource, 0, len(sources))
	for _, s := range sources {
		items = append(items, adminLoginSource{ID: s.ID, Name: s.Name})
	}
	return http.StatusOK, &adminUserOptions{
		LoginSources:              items,
		CanSendEmail:              conf.Email.Enabled,
		LocalPathMigrationEnabled: conf.Repository.EnableLocalPathMigration,
	}, nil
}

// checkAdminLoginSource returns the field errors of the authentication source
// and login name of a user: the source must exist, and users of a source need
// the name they sign in with there.
func checkAdminLoginSource(ctx context.Context, l i18n.Locale, sourceID int64, loginName string) (fieldErrors, error) {
	if sourceID == 0 {
		return nil, nil
	}
	if _, err := database.Handle.LoginSources().GetByID(ctx, sourceID); err != nil {
		if database.IsErrLoginSourceNotExist(err) {
			msg := l.Tr("admin.users.auth_source_not_exist")
			return fieldErrors{"loginSource": &msg}, nil
		}
		return nil, errors.Wrap(err, "get login source")
	}
	if loginName == "" {
		msg := l.Tr("form.LoginName") + l.Tr("form.require_error")
		return fieldErrors{"loginName": &msg}, nil
	}
	return nil, nil
}

type adminUserCreateRequest struct {
	LoginSource int64  `json:"loginSource"`
	LoginName   string `json:"loginName" validate:"max=254"`
	UserName    string `json:"name" validate:"required,alphadashdot,max=35"`
	Email       string `json:"email" validate:"required,email,max=254"`
	Password    string `json:"password" validate:"max=255"`
	SendNotify  bool   `json:"sendNotify"`
}

type adminUserCreated struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func postAdminUser(logger *logx.Logger, c flamego.Context, l i18n.Locale, admin *database.User, req adminUserCreateRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	fields, err := checkAdminLoginSource(ctx, l, req.LoginSource, req.LoginName)
	if err != nil {
		logger.Error("Failed to check login source", "loginSourceID", req.LoginSource, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if fields == nil && req.LoginSource == 0 && req.Password == "" {
		// Local users sign in with a password, so they need one to start with.
		msg := l.Tr("form.Password") + l.Tr("form.require_error")
		fields = fieldErrors{"password": &msg}
	}
	if fields != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
	}

	opts := database.CreateUserOptions{Activated: true}
	if req.LoginSource == 0 {
		opts.Password = req.Password
	} else {
		opts.LoginSource = req.LoginSource
		opts.LoginName = req.LoginName
	}
	user, err := database.Handle.Users().Create(ctx, req.UserName, req.Email, opts)
	if err != nil {
		var msg string
		switch {
		case database.IsErrUserAlreadyExist(err):
			msg = l.Tr("form.username_been_taken")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
		case database.IsErrEmailAlreadyUsed(err):
			msg = l.Tr("form.email_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
		case database.IsErrNameNotAllowed(err):
			msg = l.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
			return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
		}
		logger.Error("Failed to create user", "userName", req.UserName, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create user")
	}
	logger.Debug("Account created by admin", "userName", user.Name, "adminName", admin.Name)

	if req.SendNotify && conf.Email.Enabled {
		if err = email.SendRegisterNotifyMail(logger, l, database.NewMailerUser(user)); err != nil {
			logger.Error("Failed to send register notify mail", "userID", user.ID, "error", err)
		}
	}
	return http.StatusCreated, &adminUserCreated{ID: user.ID, Name: user.Name}, nil
}

// adminTargetUser is the individual user named by the `id` path parameter of
// an admin route.
type adminTargetUser struct {
	*database.User
}

// withAdminTargetUser maps the user named by the `id` path parameter as
// *adminTargetUser. Organizations are not found, since they have their own
// settings.
func withAdminTargetUser(logger *logx.Logger, c flamego.Context) {
	w := c.ResponseWriter()
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	user, err := database.Handle.Users().GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("user not found"))
			return
		}
		logger.Error("Failed to get user", "userID", id, "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	if user.IsOrganization() {
		writeErrorResponse(w, http.StatusNotFound, errors.New("user not found"))
		return
	}
	c.Map(&adminTargetUser{User: user})
}

type adminUserDetail struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	AvatarURL        string `json:"avatarURL"`
	LoginSource      int64  `json:"loginSource"`
	LoginName        string `json:"loginName"`
	FullName         string `json:"fullName"`
	Email            string `json:"email"`
	Website          string `json:"website"`
	Location         string `json:"location"`
	MaxRepoCreation  int    `json:"maxRepoCreation"`
	IsActive         bool   `json:"isActive"`
	IsAdmin          bool   `json:"isAdmin"`
	AllowGitHook     bool   `json:"allowGitHook"`
	AllowImportLocal bool   `json:"allowImportLocal"`
	ProhibitLogin    bool   `json:"prohibitLogin"`
}

func getAdminUser(target *adminTargetUser) (statusCode int, resp *adminUserDetail, err error) {
	u := target.User
	return http.StatusOK, &adminUserDetail{
		ID:               u.ID,
		Name:             u.Name,
		AvatarURL:        u.AvatarURLPath(),
		LoginSource:      u.LoginSource,
		LoginName:        u.LoginName,
		FullName:         u.FullName,
		Email:            u.Email,
		Website:          u.Website,
		Location:         u.Location,
		MaxRepoCreation:  u.MaxRepoCreation,
		IsActive:         u.IsActive,
		IsAdmin:          u.IsAdmin,
		AllowGitHook:     u.AllowGitHook,
		AllowImportLocal: u.AllowImportLocal,
		ProhibitLogin:    u.ProhibitLogin,
	}, nil
}

type adminUserUpdateRequest struct {
	LoginSource int64  `json:"loginSource"`
	LoginName   string `json:"loginName" validate:"max=254"`
	FullName    string `json:"fullName" validate:"max=100"`
	Email       string `json:"email" validate:"required,email,max=254"`
	// Password is left unchanged when empty.
	Password         string `json:"password" validate:"max=255"`
	Website          string `json:"website" validate:"omitempty,url,max=100"`
	Location         string `json:"location" validate:"max=50"`
	MaxRepoCreation  int    `json:"maxRepoCreation"`
	IsActive         bool   `json:"isActive"`
	IsAdmin          bool   `json:"isAdmin"`
	AllowGitHook     bool   `json:"allowGitHook"`
	AllowImportLocal bool   `json:"allowImportLocal"`
	ProhibitLogin    bool   `json:"prohibitLogin"`
}

func patchAdminUser(logger *logx.Logger, c flamego.Context, l i18n.Locale, admin *database.User, target *adminTargetUser, req adminUserUpdateRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	u := target.User
	fields, err := checkAdminLoginSource(ctx, l, req.LoginSource, req.LoginName)
	if err != nil {
		logger.Error("Failed to check login source", "loginSourceID", req.LoginSource, "error", err)
		return http.StatusInternalServerError, nil, err
	}
	if fields != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
	}

	opts := database.UpdateUserOptions{
		LoginName:        &req.LoginName,
		FullName:         &req.FullName,
		Website:          &req.Website,
		Location:         &req.Location,
		MaxRepoCreation:  &req.MaxRepoCreation,
		IsActivated:      &req.IsActive,
		IsAdmin:          &req.IsAdmin,
		AllowGitHook:     &req.AllowGitHook,
		AllowImportLocal: &req.AllowImportLocal,
		ProhibitLogin:    &req.ProhibitLogin,
	}
	if u.LoginSource != req.LoginSource {
		opts.LoginSource = &req.LoginSource
	}
	if req.Password != "" {
		opts.Password = &req.Password
	}
	if u.Email != req.Email {
		opts.Email = &req.Email
	}
	if err = database.Handle.Users().Update(ctx, u.ID, opts); err != nil {
		if database.IsErrEmailAlreadyUsed(err) {
			msg := l.Tr("form.email_been_used")
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"email": &msg}}, nil
		}
		logger.Error("Failed to update user", "userID", u.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update user")
	}
	logger.Debug("Account updated by admin", "userName", u.Name, "adminName", admin.Name)
	return http.StatusNoContent, nil, nil
}

func deleteAdminUser(logger *logx.Logger, c flamego.Context, l i18n.Locale, admin *database.User, target *adminTargetUser) (statusCode int, resp any, err error) {
	u := target.User
	if err = database.Handle.Users().DeleteByID(c.Request().Context(), u.ID, false); err != nil {
		switch {
		case database.IsErrUserOwnRepos(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("admin.users.still_own_repo")}, nil
		case database.IsErrUserHasOrgs(err):
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("admin.users.still_has_org")}, nil
		}
		logger.Error("Failed to delete user", "userID", u.ID, "error", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete user")
	}
	logger.Debug("Account deleted by admin", "userName", u.Name, "adminName", admin.Name)
	return http.StatusNoContent, nil, nil
}
