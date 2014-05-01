// Copyright github.com/juju2013. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"

	"github.com/gogits/gogs/modules/auth/ldap"
	"github.com/gogits/gogs/modules/log"
)

// Query if name/passwd can login against the LDAP direcotry pool
// Create a local user if success
// Return the same LoginUserPlain semantic
func LoginUserLdap(name, passwd string) (*User, error) {
	mail, logged := ldap.LoginUser(name, passwd)
	if !logged {
		// user not in LDAP, do nothing
		return nil, ErrUserNotExist
	}
	// fake a local user creation
	user := User{
		LowerName: strings.ToLower(name),
		Name:      strings.ToLower(name),
		LoginType: 389,
		IsActive:  true,
		Passwd:    passwd,
		Email:     mail}
	_, err := RegisterUser(&user)
	if err != nil {
		log.Debug("LDAP local user %s fond (%s) ", name, err)
	}
	// simulate local user login
	localUser, err2 := GetUserByName(user.Name)
	return localUser, err2
}
