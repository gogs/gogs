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

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

type ActionType int

const (
	ACTION_CREATE_REPO         ActionType = iota + 1 // 1
	ACTION_RENAME_REPO                               // 2
	ACTION_STAR_REPO                                 // 3
	ACTION_WATCH_REPO                                // 4
	ACTION_COMMIT_REPO                               // 5
	ACTION_CREATE_ISSUE                              // 6
	ACTION_CREATE_PULL_REQUEST                       // 7
	ACTION_TRANSFER_REPO                             // 8
	ACTION_PUSH_TAG                                  // 9
	ACTION_COMMENT_ISSUE                             // 10
	ACTION_MERGE_PULL_REQUEST                        // 11
	ACTION_CLOSE_ISSUE                               // 12
	ACTION_REOPEN_ISSUE                              // 13
	ACTION_CLOSE_PULL_REQUEST                        // 14
	ACTION_REOPEN_PULL_REQUEST                       // 15
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
	ID           int64 `xorm:"pk autoincr"`
	UserID       int64 // Receiver user id.
	OpType       ActionType
	ActUserID    int64  // Action user id.
	ActUserName  string // Action user name.
	ActEmail     string
	ActAvatar    string `xorm:"-"`
	RepoID       int64
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool      `xorm:"NOT NULL DEFAULT false"`
	Content      string    `xorm:"TEXT"`
	Created      time.Time `xorm:"-"`
	CreatedUnix  int64
}

func (a *Action) BeforeInsert() {
	a.CreatedUnix = time.Now().UTC().Unix()
}

func (a *Action) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		a.Created = time.Unix(a.CreatedUnix, 0).Local()
	}
}

func (a *Action) GetOpType() int {
	return int(a.OpType)
}

func (a *Action) GetActUserName() string {
	return a.ActUserName
}

func (a *Action) ShortActUserName() string {
	return base.EllipsisString(a.ActUserName, 20)
}

func (a *Action) GetActEmail() string {
	return a.ActEmail
}

func (a *Action) GetRepoUserName() string {
	return a.RepoUserName
}

func (a *Action) ShortRepoUserName() string {
	return base.EllipsisString(a.RepoUserName, 20)
}

func (a *Action) GetRepoName() string {
	return a.RepoName
}

func (a *Action) ShortRepoName() string {
	return base.EllipsisString(a.RepoName, 33)
}

func (a *Action) GetRepoPath() string {
	return path.Join(a.RepoUserName, a.RepoName)
}

func (a *Action) ShortRepoPath() string {
	return path.Join(a.ShortRepoUserName(), a.ShortRepoName())
}

func (a *Action) GetRepoLink() string {
	if len(setting.AppSubUrl) > 0 {
		return path.Join(setting.AppSubUrl, a.GetRepoPath())
	}
	return "/" + a.GetRepoPath()
}

func (a *Action) GetBranch() string {
	return a.RefName
}

func (a *Action) GetContent() string {
	return a.Content
}

func (a *Action) GetCreate() time.Time {
	return a.Created
}

func (a *Action) GetIssueInfos() []string {
	return strings.SplitN(a.Content, "|", 2)
}

func (a *Action) GetIssueTitle() string {
	index := com.StrTo(a.GetIssueInfos()[0]).MustInt64()
	issue, err := GetIssueByIndex(a.RepoID, index)
	if err != nil {
		log.Error(4, "GetIssueByIndex: %v", err)
		return "500 when get issue"
	}
	return issue.Name
}

func (a *Action) GetIssueContent() string {
	index := com.StrTo(a.GetIssueInfos()[0]).MustInt64()
	issue, err := GetIssueByIndex(a.RepoID, index)
	if err != nil {
		log.Error(4, "GetIssueByIndex: %v", err)
		return "500 when get issue"
	}
	return issue.Content
}

func newRepoAction(e Engine, u *User, repo *Repository) (err error) {
	if err = notifyWatchers(e, &Action{
		ActUserID:    u.Id,
		ActUserName:  u.Name,
		ActEmail:     u.Email,
		OpType:       ACTION_CREATE_REPO,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}); err != nil {
		return fmt.Errorf("notify watchers '%d/%d': %v", u.Id, repo.ID, err)
	}

	log.Trace("action.newRepoAction: %s/%s", u.Name, repo.Name)
	return err
}

// NewRepoAction adds new action for creating repository.
func NewRepoAction(u *User, repo *Repository) (err error) {
	return newRepoAction(x, u, repo)
}

func renameRepoAction(e Engine, actUser *User, oldRepoName string, repo *Repository) (err error) {
	if err = notifyWatchers(e, &Action{
		ActUserID:    actUser.Id,
		ActUserName:  actUser.Name,
		ActEmail:     actUser.Email,
		OpType:       ACTION_RENAME_REPO,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
		Content:      oldRepoName,
	}); err != nil {
		return fmt.Errorf("notify watchers: %v", err)
	}

	log.Trace("action.renameRepoAction: %s/%s", actUser.Name, repo.Name)
	return nil
}

// RenameRepoAction adds new action for renaming a repository.
func RenameRepoAction(actUser *User, oldRepoName string, repo *Repository) error {
	return renameRepoAction(x, actUser, oldRepoName, repo)
}

func issueIndexTrimRight(c rune) bool {
	return !unicode.IsDigit(c)
}

type PushCommit struct {
	Sha1        string
	Message     string
	AuthorEmail string
	AuthorName  string
}

type PushCommits struct {
	Len        int
	Commits    []*PushCommit
	CompareUrl string

	avatars map[string]string
}

func NewPushCommits() *PushCommits {
	return &PushCommits{
		avatars: make(map[string]string),
	}
}

func (pc *PushCommits) ToApiPayloadCommits(repoLink string) []*api.PayloadCommit {
	commits := make([]*api.PayloadCommit, len(pc.Commits))
	for i, cmt := range pc.Commits {
		author_username := ""
		author, err := GetUserByEmail(cmt.AuthorEmail)
		if err == nil {
			author_username = author.Name
		}
		commits[i] = &api.PayloadCommit{
			ID:      cmt.Sha1,
			Message: cmt.Message,
			URL:     fmt.Sprintf("%s/commit/%s", repoLink, cmt.Sha1),
			Author: &api.PayloadAuthor{
				Name:     cmt.AuthorName,
				Email:    cmt.AuthorEmail,
				UserName: author_username,
			},
		}
	}
	return commits
}

// AvatarLink tries to match user in database with e-mail
// in order to show custom avatar, and falls back to general avatar link.
func (push *PushCommits) AvatarLink(email string) string {
	_, ok := push.avatars[email]
	if !ok {
		u, err := GetUserByEmail(email)
		if err != nil {
			push.avatars[email] = base.AvatarLink(email)
			if !IsErrUserNotExist(err) {
				log.Error(4, "GetUserByEmail: %v", err)
			}
		} else {
			push.avatars[email] = u.AvatarLink()
		}
	}

	return push.avatars[email]
}

// updateIssuesCommit checks if issues are manipulated by commit message.
func updateIssuesCommit(u *User, repo *Repository, repoUserName, repoName string, commits []*PushCommit) error {
	// Commits are appended in the reverse order.
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]

		refMarked := make(map[int64]bool)
		for _, ref := range IssueReferenceKeywordsPat.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if !strings.Contains(ref, "/") {
				// FIXME: We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			url := fmt.Sprintf("%s/%s/%s/commit/%s", setting.AppSubUrl, repoUserName, repoName, c.Sha1)
			message := fmt.Sprintf(`<a href="%s">%s</a>`, url, c.Message)
			if err = CreateRefComment(u, repo, issue, message, c.Sha1); err != nil {
				return err
			}
		}

		refMarked = make(map[int64]bool)
		// FIXME: can merge this one and next one to a common function.
		for _, ref := range IssueCloseKeywordsPat.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if !strings.Contains(ref, "/") {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			if issue.RepoID != repo.ID || issue.IsClosed {
				continue
			}

			if err = issue.ChangeStatus(u, repo, true); err != nil {
				return err
			}
		}

		// It is conflict to have close and reopen at same time, so refsMarkd doesn't need to reinit here.
		for _, ref := range IssueReopenKeywordsPat.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, issueIndexTrimRight)

			if len(ref) == 0 {
				continue
			}

			// Add repo name if missing
			if ref[0] == '#' {
				ref = fmt.Sprintf("%s/%s%s", repoUserName, repoName, ref)
			} else if !strings.Contains(ref, "/") {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if IsErrIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			if issue.RepoID != repo.ID || !issue.IsClosed {
				continue
			}

			if err = issue.ChangeStatus(u, repo, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// CommitRepoAction adds new action for committing repository.
func CommitRepoAction(
	userID, repoUserID int64,
	userName, actEmail string,
	repoID int64,
	repoUserName, repoName string,
	refFullName string,
	commit *PushCommits,
	oldCommitID string, newCommitID string) error {

	u, err := GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("GetUserByID: %v", err)
	}

	repo, err := GetRepositoryByName(repoUserID, repoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName: %v", err)
	} else if err = repo.GetOwner(); err != nil {
		return fmt.Errorf("GetOwner: %v", err)
	}

	// Change repository bare status and update last updated time.
	repo.IsBare = false
	if err = UpdateRepository(repo, false); err != nil {
		return fmt.Errorf("UpdateRepository: %v", err)
	}

	isNewBranch := false
	opType := ACTION_COMMIT_REPO
	// Check it's tag push or branch.
	if strings.HasPrefix(refFullName, "refs/tags/") {
		opType = ACTION_PUSH_TAG
		commit = &PushCommits{}
	} else {
		// if not the first commit, set the compareUrl
		if !strings.HasPrefix(oldCommitID, "0000000") {
			commit.CompareUrl = repo.ComposeCompareURL(oldCommitID, newCommitID)
		} else {
			isNewBranch = true
		}

		if err = updateIssuesCommit(u, repo, repoUserName, repoName, commit.Commits); err != nil {
			log.Error(4, "updateIssuesCommit: %v", err)
		}
	}

	if len(commit.Commits) > setting.FeedMaxCommitNum {
		commit.Commits = commit.Commits[:setting.FeedMaxCommitNum]
	}

	bs, err := json.Marshal(commit)
	if err != nil {
		return fmt.Errorf("Marshal: %v", err)
	}

	refName := git.RefEndName(refFullName)

	if err = NotifyWatchers(&Action{
		ActUserID:    u.Id,
		ActUserName:  userName,
		ActEmail:     actEmail,
		OpType:       opType,
		Content:      string(bs),
		RepoID:       repo.ID,
		RepoUserName: repoUserName,
		RepoName:     repoName,
		RefName:      refName,
		IsPrivate:    repo.IsPrivate,
	}); err != nil {
		return fmt.Errorf("NotifyWatchers: %v", err)
	}

	payloadRepo := repo.ComposePayload()

	pusher_email, pusher_name := "", ""
	pusher, err := GetUserByName(userName)
	if err == nil {
		pusher_email = pusher.Email
		pusher_name = pusher.DisplayName()
	}
	payloadSender := &api.PayloadUser{
		UserName:  pusher.Name,
		ID:        pusher.Id,
		AvatarUrl: pusher.AvatarLink(),
	}

	switch opType {
	case ACTION_COMMIT_REPO: // Push
		p := &api.PushPayload{
			Ref:        refFullName,
			Before:     oldCommitID,
			After:      newCommitID,
			CompareUrl: setting.AppUrl + commit.CompareUrl,
			Commits:    commit.ToApiPayloadCommits(repo.FullRepoLink()),
			Repo:       payloadRepo,
			Pusher: &api.PayloadAuthor{
				Name:     pusher_name,
				Email:    pusher_email,
				UserName: userName,
			},
			Sender: payloadSender,
		}
		if err = PrepareWebhooks(repo, HOOK_EVENT_PUSH, p); err != nil {
			return fmt.Errorf("PrepareWebhooks: %v", err)
		}

		if isNewBranch {
			return PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
				Ref:     refName,
				RefType: "branch",
				Repo:    payloadRepo,
				Sender:  payloadSender,
			})
		}

	case ACTION_PUSH_TAG: // Create
		return PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
			Ref:     refName,
			RefType: "tag",
			Repo:    payloadRepo,
			Sender:  payloadSender,
		})
	}

	return nil
}

func transferRepoAction(e Engine, actUser, oldOwner, newOwner *User, repo *Repository) (err error) {
	if err = notifyWatchers(e, &Action{
		ActUserID:    actUser.Id,
		ActUserName:  actUser.Name,
		ActEmail:     actUser.Email,
		OpType:       ACTION_TRANSFER_REPO,
		RepoID:       repo.ID,
		RepoUserName: newOwner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
		Content:      path.Join(oldOwner.LowerName, repo.LowerName),
	}); err != nil {
		return fmt.Errorf("notify watchers '%d/%d': %v", actUser.Id, repo.ID, err)
	}

	// Remove watch for organization.
	if repo.Owner.IsOrganization() {
		if err = watchRepo(e, repo.Owner.Id, repo.ID, false); err != nil {
			return fmt.Errorf("watch repository: %v", err)
		}
	}

	log.Trace("action.transferRepoAction: %s/%s", actUser.Name, repo.Name)
	return nil
}

// TransferRepoAction adds new action for transferring repository.
func TransferRepoAction(actUser, oldOwner, newOwner *User, repo *Repository) error {
	return transferRepoAction(x, actUser, oldOwner, newOwner, repo)
}

func mergePullRequestAction(e Engine, actUser *User, repo *Repository, pull *Issue) error {
	return notifyWatchers(e, &Action{
		ActUserID:    actUser.Id,
		ActUserName:  actUser.Name,
		ActEmail:     actUser.Email,
		OpType:       ACTION_MERGE_PULL_REQUEST,
		Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Name),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	})
}

// MergePullRequestAction adds new action for merging pull request.
func MergePullRequestAction(actUser *User, repo *Repository, pull *Issue) error {
	return mergePullRequestAction(x, actUser, repo, pull)
}

// GetFeeds returns action list of given user in given context.
// userID is the user who's requesting, ctxUserID is the user/org that is requested.
// userID can be -1, if isProfile is true or in order to skip the permission check.
func GetFeeds(ctxUserID, userID, offset int64, isProfile bool) ([]*Action, error) {
	actions := make([]*Action, 0, 20)
	sess := x.Limit(20, int(offset)).Desc("id").Where("user_id=?", ctxUserID)
	if isProfile {
		sess.And("is_private=?", false).And("act_user_id=?", ctxUserID)
	} else if ctxUserID != -1 {
		ctxUser := &User{Id: ctxUserID}
		if err := ctxUser.GetUserRepositories(userID); err != nil {
			return nil, err
		}

		var repoIDs []int64
		for _, repo := range ctxUser.Repos {
			repoIDs = append(repoIDs, repo.ID)
		}

		if len(repoIDs) > 0 {
			sess.In("repo_id", repoIDs)
		}
	}

	err := sess.Find(&actions)
	return actions, err
}
