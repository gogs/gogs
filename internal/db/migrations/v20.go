// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"github.com/pkg/errors"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/cryptoutil"
)

func migrateAccessTokenSHA1(x *xorm.Engine) (err error) {
	exist, err := x.IsTableExist("access_token")
	if err != nil {
		return errors.Wrap(err, "is table exists")
	} else if !exist {
		return nil
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	type accessToken struct {
		ID     int64
		Sha1   string
		SHA256 string
	}
	var accessTokens []*accessToken
	err = sess.Table("access_token").Where("sha256 IS NULL").Find(&accessTokens)
	if err != nil {
		return errors.Wrap(err, "query access_token")
	}

	for _, t := range accessTokens {
		sha256 := cryptoutil.SHA256(t.Sha1)
		if _, err := sess.Table("access_token").ID(t.ID).Update(map[string]interface{}{"sha256": sha256}); err != nil {
			return errors.Wrap(err, "update sha256")
		}
	}

	return sess.Commit()
}
