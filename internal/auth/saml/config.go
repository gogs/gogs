package saml

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	crewjamsaml "github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	dsig "github.com/russellhaering/goxmldsig"
)

const (
	metadataCacheLifetime = time.Hour
	maxMetadataSize       = 2 << 20
)

// Config contains configuration for SAML authentication.
//
// WARNING: Field names must preserve their INI key names for backward compatibility.
type Config struct {
	IDPMetadataURL             string `ini:"idp_metadata_url"`
	ServiceProviderIssuer      string `ini:"service_provider_issuer"`
	ServiceProviderCertificate string `ini:"service_provider_certificate"`
	ServiceProviderPrivateKey  string `ini:"service_provider_private_key"`
	LoginAttribute             string `ini:"login_attribute"`
	UsernameAttribute          string `ini:"username_attribute"`
	EmailAttribute             string `ini:"email_attribute"`
	FullNameAttribute          string `ini:"full_name_attribute"`
	SkipVerify                 bool   `ini:"skip_verify"`
}

type metadataCacheEntry struct {
	metadata  *crewjamsaml.EntityDescriptor
	expiresAt time.Time
}

var metadataCache = struct {
	sync.RWMutex
	entries map[string]metadataCacheEntry
}{entries: map[string]metadataCacheEntry{}}

// Validate checks the static SAML configuration without contacting the identity provider.
func (c *Config) Validate() error {
	metadataURL, err := url.Parse(c.IDPMetadataURL)
	if err != nil {
		return errors.Wrap(err, "parse identity provider metadata URL")
	}
	if metadataURL.Scheme != "http" && metadataURL.Scheme != "https" {
		return errors.New("identity provider metadata URL must use HTTP or HTTPS")
	}
	if metadataURL.Host == "" {
		return errors.New("identity provider metadata URL must include a host")
	}

	if strings.TrimSpace(c.LoginAttribute) == "" {
		return errors.New("login attribute is required")
	}
	if strings.TrimSpace(c.UsernameAttribute) == "" {
		return errors.New("username attribute is required")
	}
	if strings.TrimSpace(c.EmailAttribute) == "" {
		return errors.New("email attribute is required")
	}

	_, _, err = c.loadKeyPair()
	return err
}

func (c *Config) loadKeyPair() (crypto.Signer, *x509.Certificate, error) {
	pair, err := tls.LoadX509KeyPair(c.ServiceProviderCertificate, c.ServiceProviderPrivateKey)
	if err != nil {
		return nil, nil, errors.Wrap(err, "load service provider key pair")
	}
	if len(pair.Certificate) == 0 {
		return nil, nil, errors.New("service provider certificate is empty")
	}
	certificate, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, nil, errors.Wrap(err, "parse service provider certificate")
	}
	signer, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return nil, nil, errors.Newf("service provider private key type %T cannot sign requests", pair.PrivateKey)
	}
	return signer, certificate, nil
}

func signatureMethod(signer crypto.Signer) (string, error) {
	switch signer.(type) {
	case *rsa.PrivateKey:
		return dsig.RSASHA256SignatureMethod, nil
	case *ecdsa.PrivateKey:
		return dsig.ECDSASHA256SignatureMethod, nil
	default:
		return "", errors.Newf("unsupported service provider private key type %T", signer)
	}
}

func (c *Config) httpClient() *http.Client {
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	tlsConfig := &tls.Config{}
	if transport.TLSClientConfig != nil {
		tlsConfig = transport.TLSClientConfig.Clone()
	}
	if tlsConfig.MinVersion < tls.VersionTLS12 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}
	tlsConfig.InsecureSkipVerify = c.SkipVerify
	transport.TLSClientConfig = tlsConfig
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}
}

func (c *Config) fetchMetadata(ctx context.Context) (*crewjamsaml.EntityDescriptor, error) {
	cacheKey := fmt.Sprintf("%t:%s", c.SkipVerify, c.IDPMetadataURL)
	metadataCache.RLock()
	entry, ok := metadataCache.entries[cacheKey]
	metadataCache.RUnlock()
	if ok && time.Now().Before(entry.expiresAt) {
		return entry.metadata, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.IDPMetadataURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create identity provider metadata request")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "fetch identity provider metadata")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, errors.Newf("fetch identity provider metadata: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataSize+1))
	if err != nil {
		return nil, errors.Wrap(err, "read identity provider metadata")
	}
	if len(data) > maxMetadataSize {
		return nil, errors.New("identity provider metadata exceeds 2 MiB")
	}
	metadata, err := samlsp.ParseMetadata(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse identity provider metadata")
	}

	metadataCache.Lock()
	metadataCache.entries[cacheKey] = metadataCacheEntry{
		metadata:  metadata,
		expiresAt: time.Now().Add(metadataCacheLifetime),
	}
	metadataCache.Unlock()
	return metadata, nil
}

func (c *Config) serviceProvider(metadataURL, acsURL url.URL) (*crewjamsaml.ServiceProvider, error) {
	signer, certificate, err := c.loadKeyPair()
	if err != nil {
		return nil, err
	}
	signatureMethod, err := signatureMethod(signer)
	if err != nil {
		return nil, err
	}
	issuer := strings.TrimSpace(c.ServiceProviderIssuer)
	if issuer == "" {
		issuer = metadataURL.String()
	}
	return &crewjamsaml.ServiceProvider{
		EntityID:          issuer,
		Key:               signer,
		Certificate:       certificate,
		HTTPClient:        c.httpClient(),
		MetadataURL:       metadataURL,
		AcsURL:            acsURL,
		AuthnNameIDFormat: crewjamsaml.UnspecifiedNameIDFormat,
		SignatureMethod:   signatureMethod,
	}, nil
}

// MetadataServiceProvider returns a service provider that can publish metadata
// without first contacting the identity provider.
func (c *Config) MetadataServiceProvider(metadataURL, acsURL url.URL) (*crewjamsaml.ServiceProvider, error) {
	return c.serviceProvider(metadataURL, acsURL)
}

// ServiceProvider returns a configured service provider for the given public endpoints.
func (c *Config) ServiceProvider(ctx context.Context, metadataURL, acsURL url.URL) (*crewjamsaml.ServiceProvider, error) {
	serviceProvider, err := c.serviceProvider(metadataURL, acsURL)
	if err != nil {
		return nil, err
	}
	serviceProvider.IDPMetadata, err = c.fetchMetadata(ctx)
	if err != nil {
		return nil, err
	}
	return serviceProvider, nil
}
