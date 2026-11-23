package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/api/v1types"
	"gogs.io/gogs/internal/database"
)

func TestToRepository(t *testing.T) {
	t.Run("parent of a fork of a fork", func(t *testing.T) {
		alice := &database.User{ID: 1, Name: "alice"}
		bob := &database.User{ID: 2, Name: "bob"}
		// Listing the forks of a repository sets it as the parent of each fork,
		// without loading its own parent.
		fork1 := &database.Repository{ID: 2, OwnerID: bob.ID, Owner: bob, Name: "fork1", IsFork: true, ForkID: 1}
		fork2 := &database.Repository{ID: 3, OwnerID: alice.ID, Owner: alice, Name: "fork2", IsFork: true, ForkID: 2, BaseRepo: fork1}

		got := toRepository(database.LinkOptions{}, fork2, nil, &v1types.RepositoryPermission{Pull: true})
		require.NotNil(t, got.Parent)
		assert.Equal(t, "bob/fork1", got.Parent.FullName)
		assert.True(t, got.Parent.Fork)
		assert.Nil(t, got.Parent.Parent)
	})
}
