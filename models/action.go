// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

// Operation types of user action.
const (
	OP_CREATE_REPO = iota + 1
	OP_DELETE_REPO
	OP_STAR_REPO
	OP_FOLLOW_REPO
	OP_COMMIT_REPO
	OP_CREATE_ISSUE
	OP_PULL_REQUEST
	OP_TRANSFER_REPO
	OP_PUSH_TAG
	OP_COMMENT_ISSUE
)

// Action represents user operation type and other information to repository.,
// it implemented interface base.Actioner so that can be used in template render.
type Action struct {
	Id           int64
	UserId       int64 // Receiver user id.
	OpType       int
	ActUserId    int64  // Action user id.
	ActUserName  string // Action user name.
	ActEmail     string
	RepoId       int64
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool      `xorm:"NOT NULL DEFAULT false"`
	Content      string    `xorm:"TEXT"`
	Created      time.Time `xorm:"created"`
}

func (a Action) GetOpType() int {
	return a.OpType
}

func (a Action) GetActUserName() string {
	return a.ActUserName
}

func (a Action) GetActEmail() string {
	return a.ActEmail
}

func (a Action) GetRepoUserName() string {
	return a.RepoUserName
}

func (a Action) GetRepoName() string {
	return a.RepoName
}

func (a Action) GetBranch() string {
	return a.RefName
}

func (a Action) GetContent() string {
	return a.Content
}

// CommitRepoAction adds new action for committing repository.
func CommitRepoAction(userId, repoUserId int64, userName, actEmail string,
	repoId int64, repoUserName, repoName string, refFullName string, commit *base.PushCommits) error {
	// log.Trace("action.CommitRepoAction(start): %d/%s", userId, repoName)

	opType := OP_COMMIT_REPO
	// Check it's tag push or branch.
	if strings.HasPrefix(refFullName, "refs/tags/") {
		opType = OP_PUSH_TAG
		commit = &base.PushCommits{}
	}

	refName := git.RefEndName(refFullName)

	bs, err := json.Marshal(commit)
	if err != nil {
		return errors.New("action.CommitRepoAction(json): " + err.Error())
	}

	// Change repository bare status and update last updated time.
	repo, err := GetRepositoryByName(repoUserId, repoName)
	if err != nil {
		return errors.New("action.CommitRepoAction(GetRepositoryByName): " + err.Error())
	}
	repo.IsBare = false
	if err = UpdateRepository(repo); err != nil {
		return errors.New("action.CommitRepoAction(UpdateRepository): " + err.Error())
	}

	if err = NotifyWatchers(&Action{ActUserId: userId, ActUserName: userName, ActEmail: actEmail,
		OpType: opType, Content: string(bs), RepoId: repoId, RepoUserName: repoUserName,
		RepoName: repoName, RefName: refName,
		IsPrivate: repo.IsPrivate}); err != nil {
		return errors.New("action.CommitRepoAction(NotifyWatchers): " + err.Error())

	}
	//qlog.Info("action.CommitRepoAction(end): %d/%s", repoUserId, repoName)

	// New push event hook.
	if err := repo.GetOwner(); err != nil {
		return errors.New("action.CommitRepoAction(GetOwner): " + err.Error())
	}

	ws, err := GetActiveWebhooksByRepoId(repoId)
	if err != nil {
		return errors.New("action.CommitRepoAction(GetWebhooksByRepoId): " + err.Error())
	} else if len(ws) == 0 {
		return nil
	}

	repoLink := fmt.Sprintf("%s%s/%s", setting.AppUrl, repoUserName, repoName)
	commits := make([]*PayloadCommit, len(commit.Commits))
	for i, cmt := range commit.Commits {
		commits[i] = &PayloadCommit{
			Id:      cmt.Sha1,
			Message: cmt.Message,
			Url:     fmt.Sprintf("%s/commit/%s", repoLink, cmt.Sha1),
			Author: &PayloadAuthor{
				Name:  cmt.AuthorName,
				Email: cmt.AuthorEmail,
			},
		}
	}
	p := &Payload{
		Ref:     refFullName,
		Commits: commits,
		Repo: &PayloadRepo{
			Id:          repo.Id,
			Name:        repo.LowerName,
			Url:         repoLink,
			Description: repo.Description,
			Website:     repo.Website,
			Watchers:    repo.NumWatches,
			Owner: &PayloadAuthor{
				Name:  repoUserName,
				Email: actEmail,
			},
			Private: repo.IsPrivate,
		},
		Pusher: &PayloadAuthor{
			Name:  repo.Owner.LowerName,
			Email: repo.Owner.Email,
		},
	}

	for _, w := range ws {
		w.GetEvent()
		if !w.HasPushEvent() {
			continue
		}

		p.Secret = w.Secret
		CreateHookTask(&HookTask{
			Type:        WEBHOOK,
			Url:         w.Url,
			Payload:     p,
			ContentType: w.ContentType,
			IsSsl:       w.IsSsl,
		})
	}
	return nil
}

// NewRepoAction adds new action for creating repository.
func NewRepoAction(u *User, repo *Repository) (err error) {
	if err = NotifyWatchers(&Action{ActUserId: u.Id, ActUserName: u.Name, ActEmail: u.Email,
		OpType: OP_CREATE_REPO, RepoId: repo.Id, RepoUserName: repo.Owner.Name, RepoName: repo.Name,
		IsPrivate: repo.IsPrivate}); err != nil {
		log.Error("action.NewRepoAction(notify watchers): %d/%s", u.Id, repo.Name)
		return err
	}

	log.Trace("action.NewRepoAction: %s/%s", u.LowerName, repo.LowerName)
	return err
}

// TransferRepoAction adds new action for transfering repository.
func TransferRepoAction(user, newUser *User, repo *Repository) (err error) {
	if err = NotifyWatchers(&Action{ActUserId: user.Id, ActUserName: user.Name, ActEmail: user.Email,
		OpType: OP_TRANSFER_REPO, RepoId: repo.Id, RepoName: repo.Name, Content: newUser.Name,
		IsPrivate: repo.IsPrivate}); err != nil {
		log.Error("action.TransferRepoAction(notify watchers): %d/%s", user.Id, repo.Name)
		return err
	}

	log.Trace("action.TransferRepoAction: %s/%s", user.LowerName, repo.LowerName)
	return err
}

// GetFeeds returns action list of given user in given context.
func GetFeeds(userid, offset int64, isProfile bool) ([]*Action, error) {
	actions := make([]*Action, 0, 20)
	sess := x.Limit(20, int(offset)).Desc("id").Where("user_id=?", userid)
	if isProfile {
		sess.Where("is_private=?", false).And("act_user_id=?", userid)
	} else {
		sess.And("act_user_id!=?", userid)
	}
	err := sess.Find(&actions)
	return actions, err
}
