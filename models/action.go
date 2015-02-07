// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type ActionType int

const (
	CREATE_REPO   ActionType = iota + 1 // 1
	DELETE_REPO                         // 2
	STAR_REPO                           // 3
	FOLLOW_REPO                         // 4
	COMMIT_REPO                         // 5
	CREATE_ISSUE                        // 6
	PULL_REQUEST                        // 7
	TRANSFER_REPO                       // 8
	PUSH_TAG                            // 9
	COMMENT_ISSUE                       // 10
)

var (
	ErrNotImplemented = errors.New("Not implemented yet")
)

var (
	// Same as Github. See https://help.github.com/articles/closing-issues-via-commit-messages
	IssueCloseKeywords  = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}
	IssueReopenKeywords = []string{"reopen", "reopens", "reopened"}

	IssueCloseKeywordsPat, IssueReopenKeywordsPat *regexp.Regexp
	IssueReferenceKeywordsPat                     *regexp.Regexp
)

func assembleKeywordsPattern(words []string) string {
	return fmt.Sprintf(`(?i)(?:%s) \S+`, strings.Join(words, "|"))
}

func init() {
	IssueCloseKeywordsPat = regexp.MustCompile(assembleKeywordsPattern(IssueCloseKeywords))
	IssueReopenKeywordsPat = regexp.MustCompile(assembleKeywordsPattern(IssueReopenKeywords))
	IssueReferenceKeywordsPat = regexp.MustCompile(`(?i)(?:)(^| )\S+`)
}

// Action represents user operation type and other information to repository.,
// it implemented interface base.Actioner so that can be used in template render.
type Action struct {
	Id           int64
	UserId       int64 // Receiver user id.
	OpType       ActionType
	ActUserId    int64  // Action user id.
	ActUserName  string // Action user name.
	ActEmail     string
	ActAvatar    string `xorm:"-"`
	RepoId       int64
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool      `xorm:"NOT NULL DEFAULT false"`
	Content      string    `xorm:"TEXT"`
	Created      time.Time `xorm:"created"`
}

func (a Action) GetOpType() int {
	return int(a.OpType)
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

func (a Action) GetRepoLink() string {
	return path.Join(a.RepoUserName, a.RepoName)
}

func (a Action) GetBranch() string {
	return a.RefName
}

func (a Action) GetContent() string {
	return a.Content
}

func (a Action) GetCreate() time.Time {
	return a.Created
}

func (a Action) GetIssueInfos() []string {
	return strings.SplitN(a.Content, "|", 2)
}

func updateIssuesCommit(userId, repoId int64, repoUserName, repoName string, commits []*base.PushCommit) error {
	for _, c := range commits {
		for _, ref := range IssueReferenceKeywordsPat.FindAllString(c.Message, -1) {
			ref := ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, func(c rune) bool {
				return !unicode.IsDigit(c)
			})

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if strings.Contains(ref, "/") == false {
				// FIXME: We don't support User#ID syntax yet
				// return ErrNotImplemented

				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				return err
			}

			url := fmt.Sprintf("%s/%s/%s/commit/%s", setting.AppSubUrl, repoUserName, repoName, c.Sha1)
			message := fmt.Sprintf(`<a href="%s">%s</a>`, url, c.Message)
			if _, err = CreateComment(userId, issue.RepoId, issue.Id, 0, 0, COMMENT_TYPE_COMMIT, message, nil); err != nil {
				return err
			}
		}

		for _, ref := range IssueCloseKeywordsPat.FindAllString(c.Message, -1) {
			ref := ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, func(c rune) bool {
				return !unicode.IsDigit(c)
			})

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if strings.Contains(ref, "/") == false {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented

				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				return err
			}

			if issue.RepoId == repoId {
				if issue.IsClosed {
					continue
				}
				issue.IsClosed = true

				if err = UpdateIssue(issue); err != nil {
					return err
				} else if err = UpdateIssueUserPairsByStatus(issue.Id, issue.IsClosed); err != nil {
					return err
				}

				if err = ChangeMilestoneIssueStats(issue); err != nil {
					return err
				}

				// If commit happened in the referenced repository, it means the issue can be closed.
				if _, err = CreateComment(userId, repoId, issue.Id, 0, 0, COMMENT_TYPE_CLOSE, "", nil); err != nil {
					return err
				}
			}
		}

		for _, ref := range IssueReopenKeywordsPat.FindAllString(c.Message, -1) {
			ref := ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, func(c rune) bool {
				return !unicode.IsDigit(c)
			})

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if strings.Contains(ref, "/") == false {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented

				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				return err
			}

			if issue.RepoId == repoId {
				if !issue.IsClosed {
					continue
				}
				issue.IsClosed = false

				if err = UpdateIssue(issue); err != nil {
					return err
				} else if err = UpdateIssueUserPairsByStatus(issue.Id, issue.IsClosed); err != nil {
					return err
				}

				if err = ChangeMilestoneIssueStats(issue); err != nil {
					return err
				}

				// If commit happened in the referenced repository, it means the issue can be closed.
				if _, err = CreateComment(userId, repoId, issue.Id, 0, 0, COMMENT_TYPE_REOPEN, "", nil); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// CommitRepoAction adds new action for committing repository.
func CommitRepoAction(userId, repoUserId int64, userName, actEmail string,
	repoId int64, repoUserName, repoName string, refFullName string, commit *base.PushCommits, oldCommitId string, newCommitId string) error {

	opType := COMMIT_REPO
	// Check it's tag push or branch.
	if strings.HasPrefix(refFullName, "refs/tags/") {
		opType = PUSH_TAG
		commit = &base.PushCommits{}
	}

	repoLink := fmt.Sprintf("%s%s/%s", setting.AppUrl, repoUserName, repoName)
	// if not the first commit, set the compareUrl
	if !strings.HasPrefix(oldCommitId, "0000000") {
		commit.CompareUrl = fmt.Sprintf("%s/compare/%s...%s", repoLink, oldCommitId, newCommitId)
	}

	bs, err := json.Marshal(commit)
	if err != nil {
		return errors.New("action.CommitRepoAction(json): " + err.Error())
	}

	refName := git.RefEndName(refFullName)

	// Change repository bare status and update last updated time.
	repo, err := GetRepositoryByName(repoUserId, repoName)
	if err != nil {
		return errors.New("action.CommitRepoAction(GetRepositoryByName): " + err.Error())
	}
	repo.IsBare = false
	if err = UpdateRepository(repo); err != nil {
		return errors.New("action.CommitRepoAction(UpdateRepository): " + err.Error())
	}

	err = updateIssuesCommit(userId, repoId, repoUserName, repoName, commit.Commits)

	if err != nil {
		log.Debug("action.CommitRepoAction(updateIssuesCommit): ", err)
	}

	if err = NotifyWatchers(&Action{ActUserId: userId, ActUserName: userName, ActEmail: actEmail,
		OpType: opType, Content: string(bs), RepoId: repoId, RepoUserName: repoUserName,
		RepoName: repoName, RefName: refName,
		IsPrivate: repo.IsPrivate}); err != nil {
		return errors.New("action.CommitRepoAction(NotifyWatchers): " + err.Error())

	}

	// New push event hook.
	if err := repo.GetOwner(); err != nil {
		return errors.New("action.CommitRepoAction(GetOwner): " + err.Error())
	}

	ws, err := GetActiveWebhooksByRepoId(repoId)
	if err != nil {
		return errors.New("action.CommitRepoAction(GetActiveWebhooksByRepoId): " + err.Error())
	}

	// check if repo belongs to org and append additional webhooks
	if repo.Owner.IsOrganization() {
		// get hooks for org
		orgws, err := GetActiveWebhooksByOrgId(repo.OwnerId)
		if err != nil {
			return errors.New("action.CommitRepoAction(GetActiveWebhooksByOrgId): " + err.Error())
		}
		ws = append(ws, orgws...)
	}

	if len(ws) == 0 {
		return nil
	}

	pusher_email, pusher_name := "", ""
	pusher, err := GetUserByName(userName)
	if err == nil {
		pusher_email = pusher.Email
		pusher_name = pusher.GetFullNameFallback()
	}

	commits := make([]*PayloadCommit, len(commit.Commits))
	for i, cmt := range commit.Commits {
		author_username := ""
		author, err := GetUserByEmail(cmt.AuthorEmail)
		if err == nil {
			author_username = author.Name
		}
		commits[i] = &PayloadCommit{
			Id:      cmt.Sha1,
			Message: cmt.Message,
			Url:     fmt.Sprintf("%s/commit/%s", repoLink, cmt.Sha1),
			Author: &PayloadAuthor{
				Name:     cmt.AuthorName,
				Email:    cmt.AuthorEmail,
				UserName: author_username,
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
				Name:     repo.Owner.GetFullNameFallback(),
				Email:    repo.Owner.Email,
				UserName: repo.Owner.Name,
			},
			Private: repo.IsPrivate,
		},
		Pusher: &PayloadAuthor{
			Name:     pusher_name,
			Email:    pusher_email,
			UserName: userName,
		},
		Before:     oldCommitId,
		After:      newCommitId,
		CompareUrl: commit.CompareUrl,
	}

	for _, w := range ws {
		w.GetEvent()
		if !w.HasPushEvent() {
			continue
		}

		switch w.HookTaskType {
		case SLACK:
			{
				s, err := GetSlackPayload(p, w.Meta)
				if err != nil {
					return errors.New("action.GetSlackPayload: " + err.Error())
				}
				CreateHookTask(&HookTask{
					Type:        w.HookTaskType,
					Url:         w.Url,
					BasePayload: s,
					ContentType: w.ContentType,
					IsSsl:       w.IsSsl,
				})
			}
		default:
			{
				p.Secret = w.Secret
				CreateHookTask(&HookTask{
					Type:        w.HookTaskType,
					Url:         w.Url,
					BasePayload: p,
					ContentType: w.ContentType,
					IsSsl:       w.IsSsl,
				})
			}
		}
	}

	return nil
}

// NewRepoAction adds new action for creating repository.
func NewRepoAction(u *User, repo *Repository) (err error) {
	if err = NotifyWatchers(&Action{ActUserId: u.Id, ActUserName: u.Name, ActEmail: u.Email,
		OpType: CREATE_REPO, RepoId: repo.Id, RepoUserName: repo.Owner.Name, RepoName: repo.Name,
		IsPrivate: repo.IsPrivate}); err != nil {
		log.Error(4, "NotifyWatchers: %d/%s", u.Id, repo.Name)
		return err
	}

	log.Trace("action.NewRepoAction: %s/%s", u.Name, repo.Name)
	return err
}

// TransferRepoAction adds new action for transferring repository.
func TransferRepoAction(u, newUser *User, repo *Repository) (err error) {
	action := &Action{
		ActUserId:    u.Id,
		ActUserName:  u.Name,
		ActEmail:     u.Email,
		OpType:       TRANSFER_REPO,
		RepoId:       repo.Id,
		RepoUserName: newUser.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
		Content:      path.Join(repo.Owner.LowerName, repo.LowerName),
	}
	if err = NotifyWatchers(action); err != nil {
		log.Error(4, "NotifyWatchers: %d/%s", u.Id, repo.Name)
		return err
	}

	// Remove watch for organization.
	if repo.Owner.IsOrganization() {
		if err = WatchRepo(repo.Owner.Id, repo.Id, false); err != nil {
			log.Error(4, "WatchRepo", err)
		}
	}

	log.Trace("action.TransferRepoAction: %s/%s", u.Name, repo.Name)
	return err
}

// GetFeeds returns action list of given user in given context.
func GetFeeds(uid, offset int64, isProfile bool) ([]*Action, error) {
	actions := make([]*Action, 0, 20)
	sess := x.Limit(20, int(offset)).Desc("id").Where("user_id=?", uid)
	if isProfile {
		sess.And("is_private=?", false).And("act_user_id=?", uid)
	}
	err := sess.Find(&actions)
	return actions, err
}
