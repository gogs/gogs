// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/errutil"
)

func TestLoginSourceFiles_GetByID(t *testing.T) {
	store := &loginSourceFiles{
		sources: []*LoginSource{
			{ID: 101},
		},
	}

	t.Run("source does not exist", func(t *testing.T) {
		_, err := store.GetByID(1)
		expErr := ErrLoginSourceNotExist{args: errutil.Args{"id": int64(1)}}
		assert.Equal(t, expErr, err)
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
		sources := store.List(ListLoginSourceOpts{})
		assert.Equal(t, 2, len(sources), "number of sources")
	})

	t.Run("list only activated sources", func(t *testing.T) {
		sources := store.List(ListLoginSourceOpts{OnlyActivated: true})
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
