package web

import (
	"bytes"
	"encoding/xml"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cockroachdb/errors"
	crewjamsaml "github.com/crewjam/saml"
	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	samlauth "gogs.io/gogs/internal/auth/saml"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/urlx"
)

const maxSAMLResponseSize = 1 << 20

func mountSAMLRoutes(f *flamego.Flame) {
	f.Group("/api/web/user/saml/{sourceID}", func() {
		f.Get("", getUserSAMLStart)
		f.Get("/metadata", getUserSAMLMetadata)
		f.Post("/acs", limitSAMLResponseSize, postUserSAMLACS)
	})
}

func limitSAMLResponseSize(c flamego.Context) {
	r := c.Request().Request
	r.Body = http.MaxBytesReader(c.ResponseWriter(), r.Body, maxSAMLResponseSize)
}

func samlEndpoints(sourceID int64) (metadataURL, acsURL url.URL) {
	prefix := "api/web/user/saml/" + strconv.FormatInt(sourceID, 10)
	metadataURL = *conf.Server.URL.ResolveReference(&url.URL{Path: prefix + "/metadata"})
	acsURL = *conf.Server.URL.ResolveReference(&url.URL{Path: prefix + "/acs"})
	return metadataURL, acsURL
}

func loadSAMLServiceProvider(r *http.Request, sourceID int64) (*samlauth.Provider, *crewjamsaml.ServiceProvider, error) {
	source, err := database.Handle.LoginSources().GetByID(r.Context(), sourceID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "get login source")
	}
	if !source.IsActived {
		return nil, nil, errors.Newf("login source %d is not activated", sourceID)
	}
	provider, ok := source.Provider.(*samlauth.Provider)
	if !ok || source.Type != auth.SAML {
		return nil, nil, errors.Newf("login source %d is not a SAML provider", sourceID)
	}
	metadataURL, acsURL := samlEndpoints(sourceID)
	serviceProvider, err := source.SAML().ServiceProvider(r.Context(), metadataURL, acsURL)
	if err != nil {
		return nil, nil, errors.Wrap(err, "configure SAML service provider")
	}
	return provider, serviceProvider, nil
}

func parseSAMLSourceID(c flamego.Context) (int64, error) {
	sourceID, err := strconv.ParseInt(c.Param("sourceID"), 10, 64)
	if err != nil || sourceID <= 0 {
		return 0, errors.New("invalid SAML login source ID")
	}
	return sourceID, nil
}

func getUserSAMLStart(c flamego.Context, w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSAMLSourceID(c)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, serviceProvider, err := loadSAMLServiceProvider(r, sourceID)
	if err != nil {
		log.Error("getUserSAMLStart: load SAML source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", r.URL.Query().Get("redirect_to"))
		return
	}

	binding := crewjamsaml.HTTPRedirectBinding
	bindingLocation := serviceProvider.GetSSOBindingLocation(binding)
	if bindingLocation == "" {
		binding = crewjamsaml.HTTPPostBinding
		bindingLocation = serviceProvider.GetSSOBindingLocation(binding)
	}
	if bindingLocation == "" {
		log.Error("getUserSAMLStart: SAML source %d has no supported SSO binding", sourceID)
		redirectSAMLFailure(w, r, "failed", r.URL.Query().Get("redirect_to"))
		return
	}

	authRequest, err := serviceProvider.MakeAuthenticationRequest(bindingLocation, binding, crewjamsaml.HTTPPostBinding)
	if err != nil {
		log.Error("getUserSAMLStart: create authentication request for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", r.URL.Query().Get("redirect_to"))
		return
	}
	tracker := samlauth.RequestTracker(serviceProvider, sourceID)
	relayState, err := tracker.TrackRequest(w, r, authRequest.ID)
	if err != nil {
		log.Error("getUserSAMLStart: track authentication request for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", r.URL.Query().Get("redirect_to"))
		return
	}

	if binding == crewjamsaml.HTTPRedirectBinding {
		redirectURL, err := authRequest.Redirect(relayState, serviceProvider)
		if err != nil {
			log.Error("getUserSAMLStart: build redirect for source %d: %v", sourceID, err)
			redirectSAMLFailure(w, r, "failed", r.URL.Query().Get("redirect_to"))
			return
		}
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'sha256-AjPdJSbZmeWHnEc5ykvJFay8FTWeTeRbs9dutfZ0HqE='; form-action 'self' "+bindingLocation+"; base-uri 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var body bytes.Buffer
	body.WriteString("<!doctype html><html><head><meta name=\"referrer\" content=\"no-referrer\"></head><body>")
	body.Write(authRequest.Post(relayState))
	body.WriteString("</body></html>")
	if _, err := w.Write(body.Bytes()); err != nil {
		log.Error("getUserSAMLStart: write authentication form for source %d: %v", sourceID, err)
	}
}

func getUserSAMLMetadata(c flamego.Context, w http.ResponseWriter, r *http.Request) {
	sourceID, err := parseSAMLSourceID(c)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	source, err := database.Handle.LoginSources().GetByID(r.Context(), sourceID)
	if err != nil {
		if database.IsErrLoginSourceNotExist(err) {
			http.NotFound(w, r)
			return
		}
		log.Error("getUserSAMLMetadata: get SAML source %d: %v", sourceID, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if !source.IsActived || source.Type != auth.SAML {
		http.NotFound(w, r)
		return
	}
	metadataURL, acsURL := samlEndpoints(sourceID)
	serviceProvider, err := source.SAML().MetadataServiceProvider(metadataURL, acsURL)
	if err != nil {
		log.Error("getUserSAMLMetadata: configure SAML source %d: %v", sourceID, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	data, err := xml.MarshalIndent(serviceProvider.Metadata(), "", "  ")
	if err != nil {
		log.Error("getUserSAMLMetadata: marshal metadata for source %d: %v", sourceID, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	if _, err := w.Write(data); err != nil {
		log.Error("getUserSAMLMetadata: write metadata for source %d: %v", sourceID, err)
	}
}

func postUserSAMLACS(c flamego.Context, w http.ResponseWriter, r *http.Request, sess session.Session, mc *macaron.Context) {
	sourceID, err := parseSAMLSourceID(c)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	provider, serviceProvider, err := loadSAMLServiceProvider(r, sourceID)
	if err != nil {
		log.Error("postUserSAMLACS: load SAML source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", "")
		return
	}
	if err := r.ParseForm(); err != nil {
		log.Warn("postUserSAMLACS: parse response for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", "")
		return
	}

	tracker := samlauth.RequestTracker(serviceProvider, sourceID)
	relayState := r.PostForm.Get("RelayState")
	trackedRequest, err := tracker.GetTrackedRequest(r, relayState)
	if err != nil {
		log.Warn("postUserSAMLACS: get tracked request for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", "")
		return
	}
	redirectTo := samlRedirectTarget(trackedRequest.URI)
	assertion, err := serviceProvider.ParseResponse(r, []string{trackedRequest.SAMLRequestID})
	if err != nil {
		log.Warn("postUserSAMLACS: validate response for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", redirectTo)
		return
	}
	if err := samlauth.StopTrackingRequest(w, r, tracker, relayState); err != nil {
		log.Warn("postUserSAMLACS: stop tracking request for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", redirectTo)
		return
	}

	externalAccount, err := provider.ExternalAccount(assertion)
	if err != nil {
		log.Warn("postUserSAMLACS: map assertion for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "provisioning", redirectTo)
		return
	}
	user, err := database.Handle.Users().AuthenticateExternalAccount(r.Context(), externalAccount, sourceID)
	if err != nil {
		if database.IsErrUserAlreadyExist(err) || database.IsErrEmailAlreadyUsed(err) || database.IsErrNameNotAllowed(err) {
			log.Warn("postUserSAMLACS: provision account for source %d: %v", sourceID, err)
			redirectSAMLFailure(w, r, "provisioning", redirectTo)
			return
		}
		log.Error("postUserSAMLACS: authenticate external account for source %d: %v", sourceID, err)
		redirectSAMLFailure(w, r, "failed", redirectTo)
		return
	}

	if database.Handle.TwoFactors().IsEnabled(r.Context(), user.ID) {
		sess.Set("mfaUserID", user.ID)
		target := conf.Server.Subpath + "/user/mfa"
		if redirectTo != "" {
			target += "?redirect_to=" + url.QueryEscape(redirectTo)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}

	completeSignIn(sess, mc, user)
	if redirectTo == "" {
		redirectTo = conf.Server.Subpath + "/"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func samlRedirectTarget(trackedURI string) string {
	trackedURL, err := url.Parse(trackedURI)
	if err != nil {
		return ""
	}
	target := trackedURL.Query().Get("redirect_to")
	if !urlx.IsSameSite(target) {
		return ""
	}
	return target
}

func redirectSAMLFailure(w http.ResponseWriter, r *http.Request, code, redirectTo string) {
	query := url.Values{"saml_error": []string{code}}
	if urlx.IsSameSite(redirectTo) {
		query.Set("redirect_to", redirectTo)
	}
	target := conf.Server.Subpath + "/user/sign-in?" + query.Encode()
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func isSAMLSource(source *database.LoginSource) bool {
	return source != nil && source.Type == auth.SAML
}

func partitionLoginSources(sources []*database.LoginSource) (password, saml []*database.LoginSource) {
	for _, source := range sources {
		if isSAMLSource(source) {
			saml = append(saml, source)
		} else {
			password = append(password, source)
		}
	}
	return password, saml
}
