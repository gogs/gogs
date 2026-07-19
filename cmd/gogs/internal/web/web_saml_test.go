package web

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/database"
)

func TestPartitionLoginSources(t *testing.T) {
	sources := []*database.LoginSource{
		{ID: 1, Type: auth.LDAP},
		{ID: 2, Type: auth.SAML},
		{ID: 3, Type: auth.SMTP},
		{ID: 4, Type: auth.SAML},
	}
	passwordSources, samlSources := partitionLoginSources(sources)
	assert.Equal(t, []int64{1, 3}, []int64{passwordSources[0].ID, passwordSources[1].ID})
	assert.Equal(t, []int64{2, 4}, []int64{samlSources[0].ID, samlSources[1].ID})
}

func TestSAMLRedirectTarget(t *testing.T) {
	tests := []struct {
		name       string
		trackedURI string
		want       string
	}{
		{name: "same-site path", trackedURI: "/api/web/user/saml/1?redirect_to=%2Falice%2Frepo%3Ftab%3Dissues", want: "/alice/repo?tab=issues"},
		{name: "external URL", trackedURI: "/api/web/user/saml/1?redirect_to=https%3A%2F%2Fevil.example.com", want: ""},
		{name: "protocol-relative URL", trackedURI: "/api/web/user/saml/1?redirect_to=%2F%2Fevil.example.com", want: ""},
		{name: "invalid tracked URI", trackedURI: "%", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, samlRedirectTarget(test.trackedURI))
		})
	}
}
