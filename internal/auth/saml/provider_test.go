package saml

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	crewjamsaml "github.com/crewjam/saml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ExternalAccount(t *testing.T) {
	provider := &Provider{config: &Config{
		LoginAttribute:    "subject-id",
		UsernameAttribute: "uid",
		EmailAttribute:    "mail",
		FullNameAttribute: "displayName",
	}}
	assertion := &crewjamsaml.Assertion{
		AttributeStatements: []crewjamsaml.AttributeStatement{{
			Attributes: []crewjamsaml.Attribute{
				{Name: "subject-id", Values: []crewjamsaml.AttributeValue{{Value: "external-123"}}},
				{FriendlyName: "uid", Values: []crewjamsaml.AttributeValue{{Value: "alice"}}},
				{Name: "mail", Values: []crewjamsaml.AttributeValue{{Value: "alice@example.com"}}},
				{Name: "displayName", Values: []crewjamsaml.AttributeValue{{Value: "Alice Doe"}}},
			},
		}},
	}

	account, err := provider.ExternalAccount(assertion)
	require.NoError(t, err)
	assert.Equal(t, "external-123", account.Login)
	assert.Equal(t, "alice", account.Name)
	assert.Equal(t, "alice@example.com", account.Email)
	assert.Equal(t, "Alice Doe", account.FullName)

	provider.config.EmailAttribute = "missing"
	_, err = provider.ExternalAccount(assertion)
	assert.ErrorContains(t, err, `missing email attribute "missing"`)
}

func TestConfig_ServiceProvider(t *testing.T) {
	certificatePath, privateKeyPath := writeTestKeyPair(t)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`))
	}))
	t.Cleanup(idp.Close)

	config := &Config{
		IDPMetadataURL:             idp.URL,
		ServiceProviderCertificate: certificatePath,
		ServiceProviderPrivateKey:  privateKeyPath,
		LoginAttribute:             "uid",
		UsernameAttribute:          "uid",
		EmailAttribute:             "mail",
	}
	require.NoError(t, config.Validate())

	metadataURL := url.URL{Scheme: "https", Host: "gogs.example.com", Path: "/api/web/user/saml/7/metadata"}
	acsURL := url.URL{Scheme: "https", Host: "gogs.example.com", Path: "/api/web/user/saml/7/acs"}
	serviceProvider, err := config.ServiceProvider(t.Context(), metadataURL, acsURL)
	require.NoError(t, err)
	assert.Equal(t, metadataURL.String(), serviceProvider.EntityID)
	assert.Equal(t, "https://idp.example.com/sso", serviceProvider.GetSSOBindingLocation(crewjamsaml.HTTPRedirectBinding))
	assert.NotEmpty(t, serviceProvider.SignatureMethod)
	assert.NotNil(t, serviceProvider.Key)
	assert.NotNil(t, serviceProvider.Certificate)

	metadataProvider, err := config.MetadataServiceProvider(metadataURL, acsURL)
	require.NoError(t, err)
	assert.Nil(t, metadataProvider.IDPMetadata)
}

func TestRequestTracker(t *testing.T) {
	certificatePath, privateKeyPath := writeTestKeyPair(t)
	config := &Config{
		ServiceProviderCertificate: certificatePath,
		ServiceProviderPrivateKey:  privateKeyPath,
	}
	metadataURL := url.URL{Scheme: "https", Host: "gogs.example.com", Path: "/api/web/user/saml/42/metadata"}
	acsURL := url.URL{Scheme: "https", Host: "gogs.example.com", Path: "/api/web/user/saml/42/acs"}
	serviceProvider, err := config.MetadataServiceProvider(metadataURL, acsURL)
	require.NoError(t, err)

	tracker := RequestTracker(serviceProvider, 42)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "https://gogs.example.com/api/web/user/saml/42?redirect_to=%2Falice%2Frepo", nil)
	relayState, err := tracker.TrackRequest(recorder, request, "request-123")
	require.NoError(t, err)

	cookies := recorder.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "gogs_saml_42_"+relayState, cookies[0].Name)
	assert.Equal(t, acsURL.Path, cookies[0].Path)
	assert.True(t, cookies[0].Secure)
	assert.Equal(t, http.SameSiteNoneMode, cookies[0].SameSite)

	acsRequest := httptest.NewRequest(http.MethodPost, acsURL.String(), nil)
	acsRequest.AddCookie(cookies[0])
	tracked, err := tracker.GetTrackedRequest(acsRequest, relayState)
	require.NoError(t, err)
	assert.Equal(t, "request-123", tracked.SAMLRequestID)
	assert.Equal(t, request.URL.String(), tracked.URI)

	stopRecorder := httptest.NewRecorder()
	require.NoError(t, StopTrackingRequest(stopRecorder, acsRequest, tracker, relayState))
	stopCookies := stopRecorder.Result().Cookies()
	require.Len(t, stopCookies, 1)
	assert.Equal(t, cookies[0].Name, stopCookies[0].Name)
	assert.Equal(t, acsURL.Path, stopCookies[0].Path)
	assert.Empty(t, stopCookies[0].Domain)
	assert.Equal(t, -1, stopCookies[0].MaxAge)
	assert.True(t, stopCookies[0].Secure)
	assert.Equal(t, http.SameSiteNoneMode, stopCookies[0].SameSite)
}

func writeTestKeyPair(t *testing.T) (certificatePath, privateKeyPath string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "gogs.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	dir := t.TempDir()
	certificatePath = filepath.Join(dir, "saml.crt")
	privateKeyPath = filepath.Join(dir, "saml.key")
	require.NoError(t, os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	require.NoError(t, os.WriteFile(privateKeyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}), 0o600))
	return certificatePath, privateKeyPath
}
