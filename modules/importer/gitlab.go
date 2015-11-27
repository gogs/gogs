// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package importer

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"time"

	"github.com/gogits/gogs/models"
)

func ImportGitLab(baseUrl *url.URL, token string) error {
	if err := importGitLabUsers(baseUrl, token); err != nil {
		return err
	}
	if err := importGitLabOrgs(baseUrl, token); err != nil {
		return err
	}
	if err := importGitLabRepos(baseUrl, token); err != nil {
		return err
	}
	return nil
}

// https://gitlab.com/gitlab-org/gitlab-ce/tree/master/doc/api
// https://gitlab.com/gitlab-org/gitlab-ce/blob/master/doc/api/users.md

type GitLabUser struct {
	Id       int64     `json:"id"`
	Name     string    `json:"username"`
	FullName string    `json:"name"`
	Email    string    `json:"email"`
	Passwd   string    `json:"password_hash"` // this requires API patch (returns bcrypt hash)
	State    string    `json:"state"`         // "active", "inactive"
	IsAdmin  bool      `json:"is_admin"`
	Website  string    `json:"website_url"`
	Created  time.Time `json:"created_at"`
}
type GitLabUsers []GitLabUser

func (slice GitLabUsers) Len() int           { return len(slice) }
func (slice GitLabUsers) Less(i, j int) bool { return slice[i].Id < slice[j].Id }
func (slice GitLabUsers) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

type GitLabKey struct {
	Id      int64     `json:"id"`
	Name    string    `json:"title"`
	Key     string    `json:"key"`
	Created time.Time `json:"created_at"`
}
type GitLabKeys []GitLabKey

func (slice GitLabKeys) Len() int           { return len(slice) }
func (slice GitLabKeys) Less(i, j int) bool { return slice[i].Id < slice[j].Id }
func (slice GitLabKeys) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

func importGitLabUsers(baseUrl *url.URL, token string) error {
	var remoteUsers GitLabUsers
	if err := fetchObjects("/api/v3/users", baseUrl, token, &remoteUsers); err != nil {
		return err
	}
	sort.Sort(remoteUsers)

	for _, remoteUser := range remoteUsers {
		if user, _ := models.GetUserByName(remoteUser.Name); user == nil {
			log.Printf("User: %s (%s)", remoteUser.Name, remoteUser.FullName)
			user := &models.User{
				Name:      remoteUser.Name,
				FullName:  remoteUser.FullName,
				Email:     remoteUser.Email,
				IsActive:  remoteUser.State == "active",
				IsAdmin:   remoteUser.IsAdmin,
				Website:   remoteUser.Website,
				Created:   remoteUser.Created,
				LoginType: models.PLAIN,
			}
			err := models.CreateUser(user)
			if err != nil {
				log.Fatalf("Cannot create user: %s", err)
				return err
			}
			// extra step needed to put bcrypt hash directly
			user.Passwd = remoteUser.Passwd
			models.UpdateUser(user)
			var remoteKeys GitLabKeys
			if err := fetchObjects(fmt.Sprintf("/api/v3/users/%d/keys", remoteUser.Id), baseUrl, token, &remoteKeys); err != nil {
				return err
			} else {
				sort.Sort(remoteKeys)
				for _, remoteKey := range remoteKeys {
					if err := models.AddPublicKeyCreated(user.Id, remoteKey.Name, remoteKey.Key, remoteKey.Created); err != nil {
						if models.IsErrKeyNameAlreadyUsed(err) {
							log.Printf("Duplicate key: %s", remoteKey.Name, remoteUser.Name, err)
						} else {
							log.Fatalf("Cannot add public key %s to user %s: %s", remoteKey.Name, remoteUser.Name, err)
							return err
						}
					} else {
						log.Printf("  Keys <- %s", remoteKey.Name)
					}
				}
			}
		} else {
			log.Printf("User %s already exists!", remoteUser.Name)
		}
	}
	return nil
}

// https://gitlab.com/gitlab-org/gitlab-ce/blob/master/doc/api/groups.md

type GitLabOrg struct {
	Id          int64  `json:"id"`
	Name        string `json:"path"`
	FullName    string `json:"name"`
	Email       string `json:"email"`
	Description string `json:"description"`
}
type GitLabOrgs []GitLabOrg

func (slice GitLabOrgs) Len() int           { return len(slice) }
func (slice GitLabOrgs) Less(i, j int) bool { return slice[i].Id < slice[j].Id }
func (slice GitLabOrgs) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

type GitLabAccessLevel int

const (
	GitLabNoAccess  GitLabAccessLevel = iota
	GitLabGuest                       = 10
	GitLabReporter                    = 20
	GitLabDeveloper                   = 30
	GitLabMaster                      = 40
	GitLabOwner                       = 50
)

type GitLabMember struct {
	Id          int64             `json:"id"`
	Name        string            `json:"username"`
	FullName    string            `json:"name"`
	Email       string            `json:"email"`
	AccessLevel GitLabAccessLevel `json:"access_level"`
}
type GitLabMembers []GitLabMember

func (slice GitLabMembers) Len() int           { return len(slice) }
func (slice GitLabMembers) Less(i, j int) bool { return slice[i].AccessLevel > slice[j].AccessLevel }
func (slice GitLabMembers) Swap(i, j int)      { slice[i], slice[j] = slice[j], slice[i] }

func importGitLabOrgs(baseUrl *url.URL, token string) error {
	var remoteOrgs GitLabOrgs
	if err := fetchObjects("/api/v3/groups", baseUrl, token, &remoteOrgs); err != nil {
		return err
	}
	sort.Sort(remoteOrgs)

	for _, remoteOrg := range remoteOrgs {
		if org, _ := models.GetOrgByName(remoteOrg.Name); org == nil {
			var remoteMembers GitLabMembers
			if err := fetchObjects(fmt.Sprintf("/api/v3/groups/%d/members", remoteOrg.Id), baseUrl, token, &remoteMembers); err != nil {
				return err
			}
			if len(remoteMembers) == 0 {
				log.Fatalf("No members in org: %s! Skipping", remoteOrg.Name)
				continue
			}
			if owner, _ := models.GetUserByName(remoteMembers[0].Name); owner == nil {
				log.Fatalf("Inexistent owner %s (%s) for org: %s!", remoteMembers[0].FullName, remoteMembers[0].Name, remoteOrg.Name)
			} else {
				log.Printf("Org: %s (%s)", remoteOrg.Name, remoteOrg.FullName)
				log.Printf("  %s (%s) -> Owners", owner.FullName, owner.Name)
				org := &models.User{
					Name:     remoteOrg.Name,
					FullName: remoteOrg.FullName,
					Created:  owner.Created, // GitLab does not expose creation time, use owner's
					IsActive: true,
					Type:     models.ORGANIZATION,
				}
				err := models.CreateOrganization(org, owner)
				if err != nil {
					log.Fatalf("Cannot create org: %s", err)
					return err
				}
				ownersTeam, err := org.GetOwnerTeam()
				if err != nil {
					log.Fatalf("Cannot get org owners team: %s", err)
					return err
				}
				var adminsTeam, writersTeam, readersTeam *models.Team
				for index, remoteMember := range remoteMembers {
					if index == 0 {
						continue
					}
					if member, _ := models.GetUserByName(remoteMember.Name); owner == nil {
						log.Fatalf("Inexistent member %s (%s) for org: %s!", remoteMember.FullName, remoteMember.Name, remoteOrg.Name)
					} else {
						var team *models.Team
						switch {
						case remoteMember.AccessLevel >= GitLabOwner:
							team = ownersTeam
							err = nil
						case remoteMember.AccessLevel >= GitLabMaster:
							team, err = getOrgTeam(org, &adminsTeam, "Admins", models.ACCESS_MODE_ADMIN)
						case remoteMember.AccessLevel >= GitLabDeveloper:
							team, err = getOrgTeam(org, &writersTeam, "Writers", models.ACCESS_MODE_WRITE)
						case remoteMember.AccessLevel >= GitLabGuest:
							team, err = getOrgTeam(org, &readersTeam, "Readers", models.ACCESS_MODE_READ)
						}
						if err != nil {
							return err
						}
						if err := team.AddMember(member.Id); err != nil {
							log.Fatalf("Cannot add member %s (%s) to org: %s, %s", member.FullName, member.Name, remoteOrg.Name, err)
						}
						log.Printf("  %s (%s) -> %s", member.FullName, member.Name, team.Name)
					}
				}
			}
		} else {
			log.Printf("Org %s already exists!", remoteOrg.Name)
		}
	}
	return nil
}

// https://gitlab.com/gitlab-org/gitlab-ce/blob/master/doc/api/projects.md

type GitLabNamespace struct {
	Id          int64  `json:"id"`
	Name        string `json:"path"`
	FullName    string `json:"name"`
	Description string `json:"description"`
}
type GitLabRepo struct {
	Id              int64           `json:"id"`
	Name            string          `json:"path"`
	Description     string          `json:"description"`
	Created         time.Time       `json:"created_at"`
	IsPublic        bool            `json:"public"`
	VisibilityLevel int             `json:"visibility_level"` // (0) private, (10) internal, (20) public
	Owner           GitLabUser      `json:"owner"`
	Namespace       GitLabNamespace `json:"namespace"`
	DefaultBranch   string
}
type GitLabRepos []GitLabRepo

func importGitLabRepos(baseUrl *url.URL, token string) error {
	var remoteRepos GitLabRepos
	if err := fetchObjects("/api/v3/projects/all?order_by=id&sort=asc", baseUrl, token, &remoteRepos); err != nil {
		return err
	}

	for _, remoteRepo := range remoteRepos {
		if owner, _ := models.GetUserByName(remoteRepo.Namespace.Name); owner == nil {
			log.Fatalf("Cannot find owner: %s for repo: %s", remoteRepo.Namespace.Name, remoteRepo.Name)
			return errors.New("Cannot find repo owner")
		} else {
			if repo, _ := models.GetRepositoryByName(owner.Id, remoteRepo.Name); repo == nil {
				log.Printf("Repo: %s (%s)", remoteRepo.Name, remoteRepo.Namespace.Name)
				repo, err := models.CreateRepository(owner, models.CreateRepoOptions{
					Name:        remoteRepo.Name,
					Description: remoteRepo.Description,
					IsPrivate:   remoteRepo.VisibilityLevel == 0,
					Created:     remoteRepo.Created,
					IsMirror:    false,
					AutoInit:    true,
					Readme:      "Default",
				})
				if err != nil {
					log.Fatalf("Cannot create repo: %s", err)
					return err
				}
				// NOTE: Gogs only supports plain list of collaborators in project, while GitLab has fine grained access control
				var remoteMembers GitLabMembers
				if err := fetchObjects(fmt.Sprintf("/api/v3/projects/%d/members", remoteRepo.Id), baseUrl, token, &remoteMembers); err != nil {
					return err
				}
				for _, remoteMember := range remoteMembers {
					if member, _ := models.GetUserByName(remoteMember.Name); owner == nil {
						log.Fatalf("Inexistent member %s (%s) for repo: %s!", remoteMember.FullName, remoteMember.Name, remoteRepo.Name)
					} else {
						if member.Id == owner.Id {
							continue // skip if we got owner here
						}
						if err := repo.AddCollaborator(member); err != nil {
							log.Fatalf("Cannot add collaborator %s (%s) to repo: %s, %s", member.FullName, member.Name, remoteRepo.Name, err)
							return err
						}
						log.Printf("  %s (%s) -> Collaborators", member.FullName, member.Name)
					}
				}
			} else {
				log.Printf("Repo %s already exists!", remoteRepo.Name)
			}
		}
	}
	return nil
}
