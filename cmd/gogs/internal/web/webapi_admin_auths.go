package web

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/database"
)

type adminAuthSource struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      auth.Type `json:"type"`
	TypeName  string    `json:"typeName"`
	IsActive  bool      `json:"isActive"`
	IsDefault bool      `json:"isDefault"`
	// FromFile is true for sources loaded from a file in the custom
	// configuration, which can be edited but not deleted.
	FromFile bool `json:"fromFile"`
	// CreatedAt is null for sources that do not record it, e.g., ones loaded
	// from a file.
	CreatedAt *time.Time `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

func newAdminAuthSource(s *database.LoginSource) adminAuthSource {
	timeOrNil := func(t time.Time, unix int64) *time.Time {
		if unix == 0 || t.IsZero() {
			return nil
		}
		return &t
	}
	return adminAuthSource{
		ID:        s.ID,
		Name:      s.Name,
		Type:      s.Type,
		TypeName:  s.TypeName(),
		IsActive:  s.IsActived,
		IsDefault: s.IsDefault,
		FromFile:  s.File != nil,
		CreatedAt: timeOrNil(s.Created, s.CreatedUnix),
		UpdatedAt: timeOrNil(s.Updated, s.UpdatedUnix),
	}
}

type adminAuthSourceList struct {
	Items []adminAuthSource `json:"items"`
}

func getAdminAuthSources(c flamego.Context) (statusCode int, resp *adminAuthSourceList, err error) {
	sources, err := database.Handle.LoginSources().List(c.Request().Context(), database.ListLoginSourceOptions{})
	if err != nil {
		log.Error("getAdminAuthSources: list login sources: %v", err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list login sources")
	}
	items := make([]adminAuthSource, 0, len(sources))
	for _, s := range sources {
		items = append(items, newAdminAuthSource(s))
	}
	return http.StatusOK, &adminAuthSourceList{Items: items}, nil
}

// adminAuthSourceConfig holds the settings of every type of authentication
// source. Only the ones of the source's type are used.
type adminAuthSourceConfig struct {
	// LDAP, both via BindDN and simple auth.
	SecurityProtocol  int    `json:"securityProtocol"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	BindDN            string `json:"bindDN"`
	UserBase          string `json:"userBase"`
	UserDN            string `json:"userDN"`
	Filter            string `json:"filter"`
	AdminFilter       string `json:"adminFilter"`
	AttributeUsername string `json:"attributeUsername"`
	AttributeName     string `json:"attributeName"`
	AttributeSurname  string `json:"attributeSurname"`
	AttributeMail     string `json:"attributeMail"`
	AttributesInBind  bool   `json:"attributesInBind"`
	GroupEnabled      bool   `json:"groupEnabled"`
	GroupDN           string `json:"groupDN"`
	GroupFilter       string `json:"groupFilter"`
	GroupMemberUID    string `json:"groupMemberUID"`
	UserUID           string `json:"userUID"`

	// SMTP.
	SMTPAuth       string `json:"smtpAuth"`
	SMTPHost       string `json:"smtpHost"`
	SMTPPort       int    `json:"smtpPort"`
	AllowedDomains string `json:"allowedDomains"`
	TLS            bool   `json:"tls"`

	// PAM.
	PAMServiceName string `json:"pamServiceName"`

	// GitHub.
	GitHubAPIEndpoint string `json:"githubAPIEndpoint"`

	// SkipVerify applies to LDAP over TLS, SMTP and GitHub.
	SkipVerify bool `json:"skipVerify"`
}

type adminAuthSourceDetail struct {
	adminAuthSource
	adminAuthSourceConfig
	// HasBindPassword is true when an LDAP source has a bind password. The
	// password itself is never sent back.
	HasBindPassword bool `json:"hasBindPassword"`
}

func getAdminAuthSource(target *adminTargetAuthSource) (statusCode int, resp *adminAuthSourceDetail, err error) {
	s := target.LoginSource
	d := &adminAuthSourceDetail{adminAuthSource: newAdminAuthSource(s)}
	cfg := &d.adminAuthSourceConfig
	switch {
	case s.IsLDAP(), s.IsDLDAP():
		l := s.LDAP()
		cfg.SecurityProtocol = int(l.SecurityProtocol)
		cfg.Host, cfg.Port = l.Host, l.Port
		cfg.BindDN, cfg.UserBase, cfg.UserDN = l.BindDN, l.UserBase, l.UserDN
		cfg.Filter, cfg.AdminFilter = l.Filter, l.AdminFilter
		cfg.AttributeUsername, cfg.AttributeName = l.AttributeUsername, l.AttributeName
		cfg.AttributeSurname, cfg.AttributeMail = l.AttributeSurname, l.AttributeMail
		cfg.AttributesInBind = l.AttributesInBind
		cfg.GroupEnabled, cfg.GroupDN, cfg.GroupFilter = l.GroupEnabled, l.GroupDN, l.GroupFilter
		cfg.GroupMemberUID, cfg.UserUID = l.GroupMemberUID, l.UserUID
		cfg.SkipVerify = l.SkipVerify
		d.HasBindPassword = l.BindPassword != ""
	case s.IsSMTP():
		m := s.SMTP()
		cfg.SMTPAuth, cfg.SMTPHost, cfg.SMTPPort = m.Auth, m.Host, m.Port
		cfg.AllowedDomains, cfg.TLS, cfg.SkipVerify = m.AllowedDomains, m.TLS, m.SkipVerify
	case s.IsPAM():
		cfg.PAMServiceName = s.PAM().ServiceName
	case s.IsGitHub():
		g := s.GitHub()
		cfg.GitHubAPIEndpoint, cfg.SkipVerify = g.APIEndpoint, g.SkipVerify
	}
	return http.StatusOK, d, nil
}

type adminAuthSourceRequest struct {
	adminAuthSourceConfig
	// Type is only read when creating a source. A source keeps its type.
	Type           auth.Type `json:"type"`
	AuthSourceName string    `json:"name" validate:"required,max=30"`
	IsActive       bool      `json:"isActive"`
	IsDefault      bool      `json:"isDefault"`
	// BindPassword of an LDAP source is left unchanged when empty on update.
	BindPassword string `json:"bindPassword"`
}

// checkAdminAuthSource returns the field errors of the settings a source of
// the type needs.
func checkAdminAuthSource(l i18n.Locale, typ auth.Type, req *adminAuthSourceRequest) fieldErrors {
	fields := fieldErrors{}
	fail := func(field, labelKey, suffixKey string) {
		if _, ok := fields[field]; !ok {
			msg := l.Tr(labelKey) + l.Tr(suffixKey)
			fields[field] = &msg
		}
	}
	require := func(field, labelKey, value string) {
		if strings.TrimSpace(value) == "" {
			fail(field, labelKey, "form.require_error")
		}
	}
	requirePort := func(field, labelKey string, port int) {
		if port <= 0 || port > 65535 {
			fail(field, labelKey, "form.require_error")
		}
	}

	switch typ {
	case auth.LDAP, auth.DLDAP:
		if req.SecurityProtocol < int(ldap.SecurityProtocolUnencrypted) || req.SecurityProtocol > int(ldap.SecurityProtocolStartTLS) {
			fail("securityProtocol", "admin.auths.security_protocol", "form.require_error")
		}
		require("host", "admin.auths.host", req.Host)
		requirePort("port", "admin.auths.port", req.Port)
		if typ == auth.LDAP {
			require("userBase", "admin.auths.user_base", req.UserBase)
		} else {
			require("userDN", "admin.auths.user_dn", req.UserDN)
		}
		require("filter", "admin.auths.filter", req.Filter)
		require("attributeMail", "admin.auths.attribute_mail", req.AttributeMail)
	case auth.SMTP:
		if !slices.Contains(smtp.AuthTypes, req.SMTPAuth) {
			fail("smtpAuth", "admin.auths.smtp_auth", "form.require_error")
		}
		require("smtpHost", "admin.auths.smtphost", req.SMTPHost)
		requirePort("smtpPort", "admin.auths.smtpport", req.SMTPPort)
	case auth.PAM:
		require("pamServiceName", "admin.auths.pam_service_name", req.PAMServiceName)
	case auth.GitHub:
		u, err := url.ParseRequestURI(req.GitHubAPIEndpoint)
		switch {
		case strings.TrimSpace(req.GitHubAPIEndpoint) == "":
			fail("githubAPIEndpoint", "admin.auths.github_api_endpoint", "form.require_error")
		case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
			fail("githubAPIEndpoint", "admin.auths.github_api_endpoint", "form.url_error")
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// adminAuthSourceProvider builds the provider of a source of the type from
// the request. bindPassword is the LDAP bind password to keep.
func adminAuthSourceProvider(typ auth.Type, req *adminAuthSourceRequest, bindPassword string) auth.Provider {
	switch typ {
	case auth.LDAP, auth.DLDAP:
		return ldap.NewProvider(typ == auth.DLDAP, &ldap.Config{
			Host:              req.Host,
			Port:              req.Port,
			SecurityProtocol:  ldap.SecurityProtocol(req.SecurityProtocol),
			SkipVerify:        req.SkipVerify,
			BindDN:            req.BindDN,
			UserDN:            req.UserDN,
			BindPassword:      bindPassword,
			UserBase:          req.UserBase,
			AttributeUsername: req.AttributeUsername,
			AttributeName:     req.AttributeName,
			AttributeSurname:  req.AttributeSurname,
			AttributeMail:     req.AttributeMail,
			AttributesInBind:  req.AttributesInBind,
			Filter:            req.Filter,
			GroupEnabled:      req.GroupEnabled,
			GroupDN:           req.GroupDN,
			GroupFilter:       req.GroupFilter,
			GroupMemberUID:    req.GroupMemberUID,
			UserUID:           req.UserUID,
			AdminFilter:       req.AdminFilter,
		})
	case auth.SMTP:
		return smtp.NewProvider(&smtp.Config{
			Auth:           req.SMTPAuth,
			Host:           req.SMTPHost,
			Port:           req.SMTPPort,
			AllowedDomains: req.AllowedDomains,
			TLS:            req.TLS,
			SkipVerify:     req.SkipVerify,
		})
	case auth.PAM:
		return pam.NewProvider(&pam.Config{ServiceName: req.PAMServiceName})
	case auth.GitHub:
		return github.NewProvider(&github.Config{
			APIEndpoint: strings.TrimSuffix(req.GitHubAPIEndpoint, "/") + "/",
			SkipVerify:  req.SkipVerify,
		})
	}
	return nil
}

// adminAuthSourceNameTaken returns the field errors when another source has
// the name.
func adminAuthSourceNameTaken(c flamego.Context, l i18n.Locale, id int64, name string) (fieldErrors, error) {
	sources, err := database.Handle.LoginSources().List(c.Request().Context(), database.ListLoginSourceOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "list login sources")
	}
	for _, s := range sources {
		if s.ID != id && s.Name == name {
			msg := l.Tr("admin.auths.login_source_exist", name)
			return fieldErrors{"name": &msg}, nil
		}
	}
	return nil, nil
}

type adminAuthSourceCreated struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func postAdminAuthSource(c flamego.Context, l i18n.Locale, admin *database.User, req adminAuthSourceRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	if adminAuthSourceProvider(req.Type, &req, "") == nil {
		msg := l.Tr("admin.auths.auth_type") + l.Tr("form.require_error")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"type": &msg}}, nil
	}
	if fields := checkAdminAuthSource(l, req.Type, &req); fields != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
	}
	// Sources loaded from files are not in the database, so creating checks
	// them here.
	fields, err := adminAuthSourceNameTaken(c, l, 0, req.AuthSourceName)
	if err != nil {
		log.Error("postAdminAuthSource: check name %q: %v", req.AuthSourceName, err)
		return http.StatusInternalServerError, nil, err
	} else if fields != nil {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fields}, nil
	}

	provider := adminAuthSourceProvider(req.Type, &req, req.BindPassword)
	source, err := database.Handle.LoginSources().Create(ctx, database.CreateLoginSourceOptions{
		Type:      req.Type,
		Name:      req.AuthSourceName,
		Activated: req.IsActive,
		Default:   req.IsDefault,
		Config:    provider.Config(),
	})
	if err != nil {
		if database.IsErrLoginSourceAlreadyExist(err) {
			msg := l.Tr("admin.auths.login_source_exist", req.AuthSourceName)
			return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fieldErrors{"name": &msg}}, nil
		}
		log.Error("postAdminAuthSource: create login source %q: %v", req.AuthSourceName, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create login source")
	}
	if source.IsDefault {
		if err = database.Handle.LoginSources().ResetNonDefault(ctx, source); err != nil {
			log.Error("postAdminAuthSource: reset non-default login sources: %v", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "reset non-default login sources")
		}
	}
	log.Trace("Authentication source %q created by admin %q", source.Name, admin.Name)
	return http.StatusCreated, &adminAuthSourceCreated{ID: source.ID, Name: source.Name}, nil
}

// adminTargetAuthSource is the authentication source named by the `id` path
// parameter of an admin route.
type adminTargetAuthSource struct {
	*database.LoginSource
}

func withAdminTargetAuthSource(c flamego.Context) {
	w := c.ResponseWriter()
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	source, err := database.Handle.LoginSources().GetByID(c.Request().Context(), id)
	if err != nil {
		if database.IsErrLoginSourceNotExist(err) {
			writeErrorResponse(w, http.StatusNotFound, errors.New("authentication source not found"))
			return
		}
		log.Error("withAdminTargetAuthSource: get login source %d: %v", id, err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.New("internal server error"))
		return
	}
	c.Map(&adminTargetAuthSource{LoginSource: source})
}

func patchAdminAuthSource(c flamego.Context, l i18n.Locale, admin *database.User, target *adminTargetAuthSource, req adminAuthSourceRequest) (statusCode int, resp any, err error) {
	ctx := c.Request().Context()
	source := target.LoginSource
	if fields := checkAdminAuthSource(l, source.Type, &req); fields != nil {
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fields}, nil
	}
	fields, err := adminAuthSourceNameTaken(c, l, source.ID, req.AuthSourceName)
	if err != nil {
		log.Error("patchAdminAuthSource: check name %q: %v", req.AuthSourceName, err)
		return http.StatusInternalServerError, nil, err
	} else if fields != nil {
		return http.StatusUnprocessableEntity, &bindingErrorResponse{Fields: fields}, nil
	}

	bindPassword := req.BindPassword
	if bindPassword == "" && (source.IsLDAP() || source.IsDLDAP()) {
		bindPassword = source.LDAP().BindPassword
	}
	source.Name = req.AuthSourceName
	source.IsActived = req.IsActive
	source.IsDefault = req.IsDefault
	source.Provider = adminAuthSourceProvider(source.Type, &req, bindPassword)
	if err = database.Handle.LoginSources().Save(ctx, source); err != nil {
		log.Error("patchAdminAuthSource: save login source %d: %v", source.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "save login source")
	}
	if source.IsDefault {
		if err = database.Handle.LoginSources().ResetNonDefault(ctx, source); err != nil {
			log.Error("patchAdminAuthSource: reset non-default login sources: %v", err)
			return http.StatusInternalServerError, nil, errors.Wrap(err, "reset non-default login sources")
		}
	}
	log.Trace("Authentication source %d updated by admin %q", source.ID, admin.Name)
	return http.StatusNoContent, nil, nil
}

func deleteAdminAuthSource(c flamego.Context, l i18n.Locale, admin *database.User, target *adminTargetAuthSource) (statusCode int, resp any, err error) {
	source := target.LoginSource
	if source.File != nil {
		// Deleting the database row would leave the file, which loads the
		// source again on the next start.
		return http.StatusConflict, nil, errors.New("authentication source is loaded from a file")
	}
	if err = database.Handle.LoginSources().DeleteByID(c.Request().Context(), source.ID); err != nil {
		if database.IsErrLoginSourceInUse(err) {
			return http.StatusConflict, &bindingErrorResponse{Error: l.Tr("admin.auths.still_in_used")}, nil
		}
		log.Error("deleteAdminAuthSource: delete login source %d: %v", source.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete login source")
	}
	log.Trace("Authentication source %d deleted by admin %q", source.ID, admin.Name)
	return http.StatusNoContent, nil, nil
}
