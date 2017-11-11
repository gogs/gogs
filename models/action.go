// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/tool"
)

type ActionType int

// To maintain backward compatibility only append to the end of list
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
	ACTION_CREATE_BRANCH                             // 16
	ACTION_DELETE_BRANCH                             // 17
	ACTION_DELETE_TAG                                // 18
	ACTION_FORK_REPO                                 // 19
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

// Action represents user operation type and other information to repository,
// it implemented interface base.Actioner so that can be used in template render.
type Action struct {
	ID           int64
	UserID       int64 // Receiver user ID
	OpType       ActionType
	ActUserID    int64  // Doer user ID
	ActUserName  string // Doer user name
	ActAvatar    string `xorm:"-"`
	RepoID       int64  `xorm:"INDEX"`
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool      `xorm:"NOT NULL DEFAULT false"`
	Content      string    `xorm:"TEXT"`
	Created      time.Time `xorm:"-"`
	CreatedUnix  int64
}

func (a *Action) BeforeInsert() {
	a.CreatedUnix = time.Now().Unix()
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
	return tool.EllipsisString(a.ActUserName, 20)
}

func (a *Action) GetRepoUserName() string {
	return a.RepoUserName
}

func (a *Action) ShortRepoUserName() string {
	return tool.EllipsisString(a.RepoUserName, 20)
}

func (a *Action) GetRepoName() string {
	return a.RepoName
}

func (a *Action) ShortRepoName() string {
	return tool.EllipsisString(a.RepoName, 33)
}

func (a *Action) GetRepoPath() string {
	return path.Join(a.RepoUserName, a.RepoName)
}

func (a *Action) ShortRepoPath() string {
	return path.Join(a.ShortRepoUserName(), a.ShortRepoName())
}

func (a *Action) GetRepoLink() string {
	if len(setting.AppSubURL) > 0 {
		return path.Join(setting.AppSubURL, a.GetRepoPath())
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
	return issue.Title
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

func newRepoAction(e Engine, doer, owner *User, repo *Repository) (err error) {
	opType := ACTION_CREATE_REPO
	if repo.IsFork {
		opType = ACTION_FORK_REPO
	}

	return notifyWatchers(e, &Action{
		ActUserID:    doer.ID,
		ActUserName:  doer.Name,
		OpType:       opType,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	})
}

// NewRepoAction adds new action for creating repository.
func NewRepoAction(doer, owner *User, repo *Repository) (err error) {
	return newRepoAction(x, doer, owner, repo)
}

func renameRepoAction(e Engine, actUser *User, oldRepoName string, repo *Repository) (err error) {
	if err = notifyWatchers(e, &Action{
		ActUserID:    actUser.ID,
		ActUserName:  actUser.Name,
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
	Sha1           string
	Message        string
	AuthorEmail    string
	AuthorName     string
	CommitterEmail string
	CommitterName  string
	Timestamp      time.Time
}

type PushCommits struct {
	Len        int
	Commits    []*PushCommit
	CompareURL string

	avatars map[string]string
}

func NewPushCommits() *PushCommits {
	return &PushCommits{
		avatars: make(map[string]string),
	}
}

func (pc *PushCommits) ToApiPayloadCommits(repoPath, repoLink string) ([]*api.PayloadCommit, error) {
	commits := make([]*api.PayloadCommit, len(pc.Commits))
	for i, commit := range pc.Commits {
		authorUsername := ""
		author, err := GetUserByEmail(commit.AuthorEmail)
		if err == nil {
			authorUsername = author.Name
		} else if !errors.IsUserNotExist(err) {
			return nil, fmt.Errorf("GetUserByEmail: %v", err)
		}

		committerUsername := ""
		committer, err := GetUserByEmail(commit.CommitterEmail)
		if err == nil {
			committerUsername = committer.Name
		} else if !errors.IsUserNotExist(err) {
			return nil, fmt.Errorf("GetUserByEmail: %v", err)
		}

		fileStatus, err := git.GetCommitFileStatus(repoPath, commit.Sha1)
		if err != nil {
			return nil, fmt.Errorf("FileStatus [commit_sha1: %s]: %v", commit.Sha1, err)
		}

		commits[i] = &api.PayloadCommit{
			ID:      commit.Sha1,
			Message: commit.Message,
			URL:     fmt.Sprintf("%s/commit/%s", repoLink, commit.Sha1),
			Author: &api.PayloadUser{
				Name:     commit.AuthorName,
				Email:    commit.AuthorEmail,
				UserName: authorUsername,
			},
			Committer: &api.PayloadUser{
				Name:     commit.CommitterName,
				Email:    commit.CommitterEmail,
				UserName: committerUsername,
			},
			Added:     fileStatus.Added,
			Removed:   fileStatus.Removed,
			Modified:  fileStatus.Modified,
			Timestamp: commit.Timestamp,
		}
	}
	return commits, nil
}

// AvatarLink tries to match user in database with e-mail
// in order to show custom avatar, and falls back to general avatar link.
func (push *PushCommits) AvatarLink(email string) string {
	_, ok := push.avatars[email]
	if !ok {
		u, err := GetUserByEmail(email)
		if err != nil {
			push.avatars[email] = tool.AvatarLink(email)
			if !errors.IsUserNotExist(err) {
				log.Error(4, "GetUserByEmail: %v", err)
			}
		} else {
			push.avatars[email] = u.RelAvatarLink()
		}
	}

	return push.avatars[email]
}

// UpdateIssuesCommit checks if issues are manipulated by commit message.
func UpdateIssuesCommit(doer *User, repo *Repository, commits []*PushCommit) error {
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
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// FIXME: We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if errors.IsIssueNotExist(err) {
					continue
				}
				return err
			}

			if refMarked[issue.ID] {
				continue
			}
			refMarked[issue.ID] = true

			msgLines := strings.Split(c.Message, "\n")
			shortMsg := msgLines[0]
			if len(msgLines) > 2 {
				shortMsg += "..."
			}
			message := fmt.Sprintf(`<a href="%s/commit/%s">%s</a>`, repo.Link(), c.Sha1, shortMsg)
			if err = CreateRefComment(doer, repo, issue, message, c.Sha1); err != nil {
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
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// FIXME: We don't support User#ID syntax yet
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if errors.IsIssueNotExist(err) {
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

			if err = issue.ChangeStatus(doer, repo, true); err != nil {
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
				ref = fmt.Sprintf("%s%s", repo.FullName(), ref)
			} else if !strings.Contains(ref, "/") {
				// We don't support User#ID syntax yet
				// return ErrNotImplemented
				continue
			}

			issue, err := GetIssueByRef(ref)
			if err != nil {
				if errors.IsIssueNotExist(err) {
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

			if err = issue.ChangeStatus(doer, repo, false); err != nil {
				return err
			}
		}
	}
	return nil
}

type CommitRepoActionOptions struct {
	PusherName  string
	RepoOwnerID int64
	RepoName    string
	RefFullName string
	OldCommitID string
	NewCommitID string
	Commits     *PushCommits
}

// CommitRepoAction adds new commit actio to the repository, and prepare corresponding webhooks.
func CommitRepoAction(opts CommitRepoActionOptions) error {
	pusher, err := GetUserByName(opts.PusherName)
	if err != nil {
		return fmt.Errorf("GetUserByName [%s]: %v", opts.PusherName, err)
	}

	repo, err := GetRepositoryByName(opts.RepoOwnerID, opts.RepoName)
	if err != nil {
		return fmt.Errorf("GetRepositoryByName [owner_id: %d, name: %s]: %v", opts.RepoOwnerID, opts.RepoName, err)
	}

	// Change repository bare status and update last updated time.
	repo.IsBare = false
	if err = UpdateRepository(repo, false); err != nil {
		return fmt.Errorf("UpdateRepository: %v", err)
	}

	isNewRef := opts.OldCommitID == git.EMPTY_SHA
	isDelRef := opts.NewCommitID == git.EMPTY_SHA

	opType := ACTION_COMMIT_REPO
	// Check if it's tag push or branch.
	if strings.HasPrefix(opts.RefFullName, git.TAG_PREFIX) {
		opType = ACTION_PUSH_TAG
	} else {
		// if not the first commit, set the compare URL.
		if !isNewRef && !isDelRef {
			opts.Commits.CompareURL = repo.ComposeCompareURL(opts.OldCommitID, opts.NewCommitID)
		}

		if err = UpdateIssuesCommit(pusher, repo, opts.Commits.Commits); err != nil {
			log.Error(2, "UpdateIssuesCommit: %v", err)
		}
	}

	if len(opts.Commits.Commits) > setting.UI.FeedMaxCommitNum {
		opts.Commits.Commits = opts.Commits.Commits[:setting.UI.FeedMaxCommitNum]
	}

	data, err := json.Marshal(opts.Commits)
	if err != nil {
		return fmt.Errorf("Marshal: %v", err)
	}

	refName := git.RefEndName(opts.RefFullName)
	action := &Action{
		ActUserID:    pusher.ID,
		ActUserName:  pusher.Name,
		Content:      string(data),
		RepoID:       repo.ID,
		RepoUserName: repo.MustOwner().Name,
		RepoName:     repo.Name,
		RefName:      refName,
		IsPrivate:    repo.IsPrivate,
	}

	apiRepo := repo.APIFormat(nil)
	apiPusher := pusher.APIFormat()
	switch opType {
	case ACTION_COMMIT_REPO: // Push
		if isDelRef {
			if err = PrepareWebhooks(repo, HOOK_EVENT_DELETE, &api.DeletePayload{
				Ref:        refName,
				RefType:    "branch",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(delete branch): %v", err)
			}

			action.OpType = ACTION_DELETE_BRANCH
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(delete branch): %v", err)
			}

			// Delete branch doesn't have anything to push or compare
			return nil
		}

		compareURL := setting.AppURL + opts.Commits.CompareURL
		if isNewRef {
			compareURL = ""
			if err = PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
				Ref:           refName,
				RefType:       "branch",
				DefaultBranch: repo.DefaultBranch,
				Repo:          apiRepo,
				Sender:        apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(new branch): %v", err)
			}

			action.OpType = ACTION_CREATE_BRANCH
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(new branch): %v", err)
			}
		}

		commits, err := opts.Commits.ToApiPayloadCommits(repo.RepoPath(), repo.HTMLURL())
		if err != nil {
			return fmt.Errorf("ToApiPayloadCommits: %v", err)
		}

		if err = PrepareWebhooks(repo, HOOK_EVENT_PUSH, &api.PushPayload{
			Ref:        opts.RefFullName,
			Before:     opts.OldCommitID,
			After:      opts.NewCommitID,
			CompareURL: compareURL,
			Commits:    commits,
			Repo:       apiRepo,
			Pusher:     apiPusher,
			Sender:     apiPusher,
		}); err != nil {
			return fmt.Errorf("PrepareWebhooks.(new commit): %v", err)
		}

		action.OpType = ACTION_COMMIT_REPO
		if err = NotifyWatchers(action); err != nil {
			return fmt.Errorf("NotifyWatchers.(new commit): %v", err)
		}

	case ACTION_PUSH_TAG: // Tag
		if isDelRef {
			if err = PrepareWebhooks(repo, HOOK_EVENT_DELETE, &api.DeletePayload{
				Ref:        refName,
				RefType:    "tag",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			}); err != nil {
				return fmt.Errorf("PrepareWebhooks.(delete tag): %v", err)
			}

			action.OpType = ACTION_DELETE_TAG
			if err = NotifyWatchers(action); err != nil {
				return fmt.Errorf("NotifyWatchers.(delete tag): %v", err)
			}
			return nil
		}

		if err = PrepareWebhooks(repo, HOOK_EVENT_CREATE, &api.CreatePayload{
			Ref:           refName,
			RefType:       "tag",
			DefaultBranch: repo.DefaultBranch,
			Repo:          apiRepo,
			Sender:        apiPusher,
		}); err != nil {
			return fmt.Errorf("PrepareWebhooks.(new tag): %v", err)
		}

		action.OpType = ACTION_PUSH_TAG
		if err = NotifyWatchers(action); err != nil {
			return fmt.Errorf("NotifyWatchers.(new tag): %v", err)
		}
	}

	return nil
}

func transferRepoAction(e Engine, doer, oldOwner *User, repo *Repository) (err error) {
	if err = notifyWatchers(e, &Action{
		ActUserID:    doer.ID,
		ActUserName:  doer.Name,
		OpType:       ACTION_TRANSFER_REPO,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
		Content:      path.Join(oldOwner.Name, repo.Name),
	}); err != nil {
		return fmt.Errorf("notifyWatchers: %v", err)
	}

	// Remove watch for organization.
	if oldOwner.IsOrganization() {
		if err = watchRepo(e, oldOwner.ID, repo.ID, false); err != nil {
			return fmt.Errorf("watchRepo [false]: %v", err)
		}
	}

	return nil
}

// TransferRepoAction adds new action for transferring repository,
// the Owner field of repository is assumed to be new owner.
func TransferRepoAction(doer, oldOwner *User, repo *Repository) error {
	return transferRepoAction(x, doer, oldOwner, repo)
}

func mergePullRequestAction(e Engine, doer *User, repo *Repository, issue *Issue) error {
	return notifyWatchers(e, &Action{
		ActUserID:    doer.ID,
		ActUserName:  doer.Name,
		OpType:       ACTION_MERGE_PULL_REQUEST,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Title),
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
// actorID is the user who's requesting, ctxUserID is the user/org that is requested.
// actorID can be -1 when isProfile is true or to skip the permission check.
func GetFeeds(ctxUser *User, actorID, afterID int64, isProfile bool) ([]*Action, error) {
	actions := make([]*Action, 0, setting.UI.User.NewsFeedPagingNum)
	sess := x.Limit(setting.UI.User.NewsFeedPagingNum).Where("user_id = ?", ctxUser.ID).Desc("id")
	if afterID > 0 {
		sess.And("id < ?", afterID)
	}
	if isProfile {
		sess.And("is_private = ?", false).And("act_user_id = ?", ctxUser.ID)
	} else if actorID != -1 && ctxUser.IsOrganization() {
		// FIXME: only need to get IDs here, not all fields of repository.
		repos, _, err := ctxUser.GetUserRepositories(actorID, 1, ctxUser.NumRepos)
		if err != nil {
			return nil, fmt.Errorf("GetUserRepositories: %v", err)
		}

		var repoIDs []int64
		for _, repo := range repos {
			repoIDs = append(repoIDs, repo.ID)
		}

		if len(repoIDs) > 0 {
			sess.In("repo_id", repoIDs)
		}
	}

	err := sess.Find(&actions)
	return actions, err
}
