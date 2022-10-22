// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package userutil

import (
	"encoding/hex"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"gogs.io/gogs/internal/avatar"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/tool"
)

// DashboardURLPath returns the URL path to the user or organization dashboard.
func DashboardURLPath(name string, isOrganization bool) string {
	if isOrganization {
		return conf.Server.Subpath + "/org/" + name + "/dashboard/"
	}
	return conf.Server.Subpath + "/"
}

// GenerateActivateCode generates an activate code based on user information and
// the given email.
func GenerateActivateCode(userID int64, email, name, password, rands string) string {
	code := tool.CreateTimeLimitCode(
		fmt.Sprintf("%d%s%s%s%s", userID, email, strings.ToLower(name), password, rands),
		conf.Auth.ActivateCodeLives,
		nil,
	)

	// Add tailing hex username
	code += hex.EncodeToString([]byte(strings.ToLower(name)))
	return code
}

// CustomAvatarPath returns the absolute path of the user custom avatar file.
func CustomAvatarPath(userID int64) string {
	return filepath.Join(conf.Picture.AvatarUploadPath, strconv.FormatInt(userID, 10))
}

// GenerateRandomAvatar generates a random avatar and stores to local file
// system using given user information.
func GenerateRandomAvatar(userID int64, name, email string) error {
	seed := email
	if seed == "" {
		seed = name
	}

	img, err := avatar.RandomImage([]byte(seed))
	if err != nil {
		return errors.Wrap(err, "generate random image")
	}

	avatarPath := CustomAvatarPath(userID)
	err = os.MkdirAll(filepath.Dir(avatarPath), os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create avatar directory")
	}

	f, err := os.Create(avatarPath)
	if err != nil {
		return errors.Wrap(err, "create avatar file")
	}
	defer func() { _ = f.Close() }()

	if err = png.Encode(f, img); err != nil {
		return errors.Wrap(err, "encode avatar image to file")
	}
	return nil
}
