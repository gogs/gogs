// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package importer

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"time"

	"github.com/gogits/gogs/models"
)

func ImportRedmine(baseUrl *url.URL, token string) error {
	if err := importRedmineUsers(baseUrl, token); err != nil {
		return err
	}
	return nil
}

// http://www.redmine.org/projects/redmine/wiki/Rest_api
// http://www.redmine.org/projects/redmine/wiki/Rest_Users

type RedmineUser struct {
	Id        int64     `json:"id"`
	Name      string    `json:"login"`
	FirstName string    `json:"firstname"`
	LastName  string    `json:"lastname"`
	Email     string    `json:"mail"`
	Passwd    string    `json:"password_hash"` // this requires API patch (returns bcrypt hash)
	Created   time.Time `json:"created_on"`
}
type RedmineUsers []RedmineUser

func (slice RedmineUsers) Len() int           { return len(slice) }
func (slice RedmineUsers) Less(i, j int) bool { return slice[i].Id < slice[j].Id }
func (slice RedmineUsers) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

func importRedmineUsers(baseUrl *url.URL, token string) error {
	var remoteUsers RedmineUsers
	if err := fetchObjects("/users.json", baseUrl, token, &remoteUsers); err != nil {
		return err
	}
	sort.Sort(remoteUsers)

	for _, remoteUser := range remoteUsers {
		if user, _ := models.GetUserByName(remoteUser.Name); user == nil {
			fullName := fmt.Sprintf("%s %s", remoteUser.FirstName, remoteUser.LastName)
			log.Printf("User: %s (%s)", remoteUser.Name, fullName)
			user := &models.User{
				Name:      remoteUser.Name,
				FullName:  fullName,
				Email:     remoteUser.Email,
				IsActive:  true,
				IsAdmin:   true,
				Created:   remoteUser.Created,
				LoginType: models.PLAIN,
			}
			err := models.CreateUser(user)
			if err != nil {
				log.Fatalf("Cannot create user: %s", err)
				return err
			}
		} else {
			log.Printf("User %s already exists!", remoteUser.Name)
		}
	}
	return nil
}
