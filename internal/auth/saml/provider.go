package saml

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	crewjamsaml "github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"

	"gogs.io/gogs/internal/auth"
)

// Provider contains configuration for a SAML authentication provider.
type Provider struct {
	config *Config
}

// NewProvider creates a SAML authentication provider.
func NewProvider(cfg *Config) auth.Provider {
	return &Provider{config: cfg}
}

// Authenticate rejects password authentication because SAML uses a browser redirect flow.
func (*Provider) Authenticate(login, _ string) (*auth.ExternalAccount, error) {
	return nil, auth.ErrBadCredentials{Args: map[string]any{"login": login}}
}

func (p *Provider) Config() any {
	return p.config
}

func (*Provider) HasTLS() bool {
	return true
}

func (p *Provider) UseTLS() bool {
	metadataURL, err := url.Parse(p.config.IDPMetadataURL)
	return err == nil && metadataURL.Scheme == "https"
}

func (p *Provider) SkipTLSVerify() bool {
	return p.config.SkipVerify
}

// RequestTracker returns a source-specific, signed cookie tracker for pending SAML requests.
func RequestTracker(serviceProvider *crewjamsaml.ServiceProvider, sourceID int64) samlsp.CookieRequestTracker {
	sameSite := http.SameSiteLaxMode
	if serviceProvider.AcsURL.Scheme == "https" {
		sameSite = http.SameSiteNoneMode
	}
	opts := samlsp.Options{
		URL:            serviceProvider.MetadataURL,
		Key:            serviceProvider.Key,
		Certificate:    serviceProvider.Certificate,
		CookieSameSite: sameSite,
	}
	tracker := samlsp.DefaultRequestTracker(opts, serviceProvider)
	tracker.NamePrefix = "gogs_saml_" + strconv.FormatInt(sourceID, 10) + "_"
	return tracker
}

// StopTrackingRequest validates and removes a pending SAML request cookie.
func StopTrackingRequest(w http.ResponseWriter, r *http.Request, tracker samlsp.CookieRequestTracker, index string) error {
	if _, err := tracker.GetTrackedRequest(r, index); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     tracker.NamePrefix + index,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: tracker.SameSite,
		Secure:   tracker.ServiceProvider.AcsURL.Scheme == "https",
		Path:     tracker.ServiceProvider.AcsURL.Path,
	})
	return nil
}

// ExternalAccount maps a validated SAML assertion to a Gogs external account.
func (p *Provider) ExternalAccount(assertion *crewjamsaml.Assertion) (*auth.ExternalAccount, error) {
	login := assertionAttribute(assertion, p.config.LoginAttribute)
	if login == "" {
		return nil, errors.Newf("SAML assertion is missing login attribute %q", p.config.LoginAttribute)
	}
	username := assertionAttribute(assertion, p.config.UsernameAttribute)
	if username == "" {
		return nil, errors.Newf("SAML assertion is missing username attribute %q", p.config.UsernameAttribute)
	}
	email := assertionAttribute(assertion, p.config.EmailAttribute)
	if email == "" {
		return nil, errors.Newf("SAML assertion is missing email attribute %q", p.config.EmailAttribute)
	}

	return &auth.ExternalAccount{
		Login:    login,
		Name:     username,
		Email:    email,
		FullName: assertionAttribute(assertion, p.config.FullNameAttribute),
	}, nil
}

func assertionAttribute(assertion *crewjamsaml.Assertion, name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	for _, statement := range assertion.AttributeStatements {
		for _, attribute := range statement.Attributes {
			if attribute.Name != name && attribute.FriendlyName != name {
				continue
			}
			for _, value := range attribute.Values {
				if value := strings.TrimSpace(value.Value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}
