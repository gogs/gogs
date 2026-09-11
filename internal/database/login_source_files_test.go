package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/errx"
)

func TestLoadLoginSourceFiles_SAML(t *testing.T) {
	dir := t.TempDir()
	config := `id           = 106
type         = saml
name         = Company SSO
is_activated = true

[config]
idp_metadata_url             = https://idp.example.com/metadata
service_provider_issuer      = https://gogs.example.com/saml
service_provider_certificate = /etc/gogs/saml.crt
service_provider_private_key = /etc/gogs/saml.key
login_attribute              = subject-id
username_attribute           = uid
email_attribute              = mail
full_name_attribute          = cn
skip_verify                  = true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "saml.conf"), []byte(config), 0o600))

	store, err := loadLoginSourceFiles(dir, time.Now)
	require.NoError(t, err)
	source, err := store.GetByID(106)
	require.NoError(t, err)
	assert.True(t, source.IsSAML())
	assert.Equal(t, "https://idp.example.com/metadata", source.SAML().IDPMetadataURL)
	assert.Equal(t, "https://gogs.example.com/saml", source.SAML().ServiceProviderIssuer)
	assert.Equal(t, "subject-id", source.SAML().LoginAttribute)
	assert.True(t, source.SAML().SkipVerify)
}

func TestLoginSourceFiles_GetByID(t *testing.T) {
	store := &loginSourceFiles{
		sources: []*LoginSource{
			{ID: 101},
		},
	}

	t.Run("source does not exist", func(t *testing.T) {
		_, err := store.GetByID(1)
		wantErr := ErrLoginSourceNotExist{args: errx.Args{"id": int64(1)}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("source exists", func(t *testing.T) {
		source, err := store.GetByID(101)
		require.NoError(t, err)
		assert.Equal(t, int64(101), source.ID)
	})
}

func TestLoginSourceFiles_Len(t *testing.T) {
	store := &loginSourceFiles{
		sources: []*LoginSource{
			{ID: 101},
		},
	}

	assert.Equal(t, 1, store.Len())
}

func TestLoginSourceFiles_List(t *testing.T) {
	store := &loginSourceFiles{
		sources: []*LoginSource{
			{ID: 101, IsActived: true},
			{ID: 102, IsActived: false},
		},
	}

	t.Run("list all sources", func(t *testing.T) {
		sources := store.List(ListLoginSourceOptions{})
		assert.Equal(t, 2, len(sources), "number of sources")
	})

	t.Run("list only activated sources", func(t *testing.T) {
		sources := store.List(ListLoginSourceOptions{OnlyActivated: true})
		assert.Equal(t, 1, len(sources), "number of sources")
		assert.Equal(t, int64(101), sources[0].ID)
	})
}

func TestLoginSourceFiles_Update(t *testing.T) {
	store := &loginSourceFiles{
		sources: []*LoginSource{
			{ID: 101, IsActived: true, IsDefault: true},
			{ID: 102, IsActived: false},
		},
		clock: time.Now,
	}

	source102 := &LoginSource{
		ID:        102,
		IsActived: true,
		IsDefault: true,
	}
	store.Update(source102)

	assert.False(t, store.sources[0].IsDefault)

	assert.True(t, store.sources[1].IsActived)
	assert.True(t, store.sources[1].IsDefault)
}
