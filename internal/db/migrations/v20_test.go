// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
)

type accessTokenPreV20 struct {
	ID          int64
	UserID      int64 `gorm:"COLUMN:uid;INDEX"`
	Name        string
	Sha1        string `gorm:"TYPE:VARCHAR(40);UNIQUE"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (*accessTokenPreV20) TableName() string {
	return "access_token"
}

type accessTokenV20 struct {
	ID          int64
	UserID      int64 `gorm:"column:uid;index"`
	Name        string
	Sha1        string `gorm:"type:VARCHAR(40);unique"`
	SHA256      string `gorm:"type:VARCHAR(64);unique;not null"`
	CreatedUnix int64
	UpdatedUnix int64
}

func (*accessTokenV20) TableName() string {
	return "access_token"
}

func TestMigrateAccessTokenToSHA256(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := dbtest.NewDB(t, "migrateAccessTokenToSHA256", new(accessTokenPreV20))
	err := db.Create(
		&accessTokenPreV20{
			ID:          1,
			UserID:      1,
			Name:        "test",
			Sha1:        "73da7bb9d2a475bbc2ab79da7d4e94940cb9f9d5",
			CreatedUnix: db.NowFunc().Unix(),
			UpdatedUnix: db.NowFunc().Unix(),
		},
	).Error
	require.NoError(t, err)

	err = migrateAccessTokenToSHA256(db)
	require.NoError(t, err)

	var got accessTokenV20
	err = db.Where("id = ?", 1).First(&got).Error
	require.NoError(t, err)
	assert.Equal(t, "73da7bb9d2a475bbc2ab79da7d4e94940cb9f9d5", got.Sha1)
	assert.Equal(t, "ab144c7bd170691bb9bb995f1541c608e33a78b40174f30fc8a1616c0bc3a477", got.SHA256)
}
