// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/lazyregexp"
	"gogs.io/gogs/internal/repoutil"
	"gogs.io/gogs/internal/strutil"
	"gogs.io/gogs/internal/testutil"
	"gogs.io/gogs/internal/tool"
)

// ActionsStore is the storage layer for actions.
type ActionsStore struct {
	db *gorm.DB
}

func newActionsStore(db *gorm.DB) *ActionsStore {
	return &ActionsStore{db: db}
}

func (s *ActionsStore) listByOrganization(ctx context.Context, orgID, actorID, afterID int64) *gorm.DB {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "action"
		WHERE
			user_id = @userID
		AND (@skipAfter OR id < @afterID)
		AND repo_id IN (
			SELECT repository.id FROM "repository"
			JOIN team_repo ON repository.id = team_repo.repo_id
			WHERE team_repo.team_id IN (
					SELECT team_id FROM "team_user"
					WHERE
						team_user.org_id = @orgID AND uid = @actorID)
					OR  (repository.is_private = FALSE AND repository.is_unlisted = FALSE)
			)
		ORDER BY id DESC
		LIMIT @limit
	*/
	return s.db.WithContext(ctx).
		Where("user_id = ?", orgID).
		Where(s.db.
			// Not apply when afterID is not given
			Where("?", afterID <= 0).
			Or("id < ?", afterID),
		).
		Where("repo_id IN (?)", s.db.
			Select("repository.id").
			Table("repository").
			Joins("JOIN team_repo ON repository.id = team_repo.repo_id").
			Where("team_repo.team_id IN (?)", s.db.
				Select("team_id").
				Table("team_user").
				Where("team_user.org_id = ? AND uid = ?", orgID, actorID),
			).
			Or("repository.is_private = ? AND repository.is_unlisted = ?", false, false),
		).
		Limit(conf.UI.User.NewsFeedPagingNum).
		Order("id DESC")
}

// ListByOrganization returns actions of the organization viewable by the actor.
// Results are paginated if `afterID` is given.
func (s *ActionsStore) ListByOrganization(ctx context.Context, orgID, actorID, afterID int64) ([]*Action, error) {
	actions := make([]*Action, 0, conf.UI.User.NewsFeedPagingNum)
	return actions, s.listByOrganization(ctx, orgID, actorID, afterID).Find(&actions).Error
}

func (s *ActionsStore) listByUser(ctx context.Context, userID, actorID, afterID int64, isProfile bool) *gorm.DB {
	/*
		Equivalent SQL for PostgreSQL:

		SELECT * FROM "action"
		WHERE
			user_id = @userID
		AND (@skipAfter OR id < @afterID)
		AND (@includePrivate OR (is_private = FALSE AND act_user_id = @actorID))
		ORDER BY id DESC
		LIMIT @limit
	*/
	return s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where(s.db.
			// Not apply when afterID is not given
			Where("?", afterID <= 0).
			Or("id < ?", afterID),
		).
		Where(s.db.
			// Not apply when in not profile page or the user is viewing own profile
			Where("?", !isProfile || actorID == userID).
			Or("is_private = ? AND act_user_id = ?", false, userID),
		).
		Limit(conf.UI.User.NewsFeedPagingNum).
		Order("id DESC")
}

// ListByUser returns actions of the user viewable by the actor. Results are
// paginated if `afterID` is given. The `isProfile` indicates whether repository
// permissions should be considered.
func (s *ActionsStore) ListByUser(ctx context.Context, userID, actorID, afterID int64, isProfile bool) ([]*Action, error) {
	actions := make([]*Action, 0, conf.UI.User.NewsFeedPagingNum)
	return actions, s.listByUser(ctx, userID, actorID, afterID, isProfile).Find(&actions).Error
}

// notifyWatchers creates rows in action table for watchers who are able to see the action.
func (s *ActionsStore) notifyWatchers(ctx context.Context, act *Action) error {
	watches, err := newReposStore(s.db).ListWatches(ctx, act.RepoID)
	if err != nil {
		return errors.Wrap(err, "list watches")
	}

	// Clone returns a deep copy of the action with UserID assigned
	clone := func(userID int64) *Action {
		tmp := *act
		tmp.UserID = userID
		return &tmp
	}

	// Plus one for the actor
	actions := make([]*Action, 0, len(watches)+1)
	actions = append(actions, clone(act.ActUserID))

	for _, watch := range watches {
		if act.ActUserID == watch.UserID {
			continue
		}
		actions = append(actions, clone(watch.UserID))
	}

	return s.db.Create(actions).Error
}

// NewRepo creates an action for creating a new repository. The action type
// could be ActionCreateRepo or ActionForkRepo based on whether the repository
// is a fork.
func (s *ActionsStore) NewRepo(ctx context.Context, doer, owner *User, repo *Repository) error {
	opType := ActionCreateRepo
	if repo.IsFork {
		opType = ActionForkRepo
	}

	return s.notifyWatchers(ctx,
		&Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       opType,
			RepoID:       repo.ID,
			RepoUserName: owner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
		},
	)
}

// RenameRepo creates an action for renaming a repository.
func (s *ActionsStore) RenameRepo(ctx context.Context, doer, owner *User, oldRepoName string, repo *Repository) error {
	return s.notifyWatchers(ctx,
		&Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       ActionRenameRepo,
			RepoID:       repo.ID,
			RepoUserName: owner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
			Content:      oldRepoName,
		},
	)
}

func (s *ActionsStore) mirrorSyncAction(ctx context.Context, opType ActionType, owner *User, repo *Repository, refName string, content []byte) error {
	return s.notifyWatchers(ctx,
		&Action{
			ActUserID:    owner.ID,
			ActUserName:  owner.Name,
			OpType:       opType,
			Content:      string(content),
			RepoID:       repo.ID,
			RepoUserName: owner.Name,
			RepoName:     repo.Name,
			RefName:      refName,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
		},
	)
}

type MirrorSyncPushOptions struct {
	Owner       *User
	Repo        *Repository
	RefName     string
	OldCommitID string
	NewCommitID string
	Commits     *PushCommits
}

// MirrorSyncPush creates an action for mirror synchronization of pushed
// commits.
func (s *ActionsStore) MirrorSyncPush(ctx context.Context, opts MirrorSyncPushOptions) error {
	if conf.UI.FeedMaxCommitNum > 0 && len(opts.Commits.Commits) > conf.UI.FeedMaxCommitNum {
		opts.Commits.Commits = opts.Commits.Commits[:conf.UI.FeedMaxCommitNum]
	}

	apiCommits, err := opts.Commits.APIFormat(ctx,
		NewUsersStore(s.db),
		repoutil.RepositoryPath(opts.Owner.Name, opts.Repo.Name),
		repoutil.HTMLURL(opts.Owner.Name, opts.Repo.Name),
	)
	if err != nil {
		return errors.Wrap(err, "convert commits to API format")
	}

	opts.Commits.CompareURL = repoutil.CompareCommitsPath(opts.Owner.Name, opts.Repo.Name, opts.OldCommitID, opts.NewCommitID)
	apiPusher := opts.Owner.APIFormat()
	err = PrepareWebhooks(
		opts.Repo,
		HOOK_EVENT_PUSH,
		&api.PushPayload{
			Ref:        opts.RefName,
			Before:     opts.OldCommitID,
			After:      opts.NewCommitID,
			CompareURL: conf.Server.ExternalURL + opts.Commits.CompareURL,
			Commits:    apiCommits,
			Repo:       opts.Repo.APIFormat(opts.Owner),
			Pusher:     apiPusher,
			Sender:     apiPusher,
		},
	)
	if err != nil {
		return errors.Wrap(err, "prepare webhooks")
	}

	data, err := jsoniter.Marshal(opts.Commits)
	if err != nil {
		return errors.Wrap(err, "marshal JSON")
	}

	return s.mirrorSyncAction(ctx, ActionMirrorSyncPush, opts.Owner, opts.Repo, opts.RefName, data)
}

// MirrorSyncCreate creates an action for mirror synchronization of a new
// reference.
func (s *ActionsStore) MirrorSyncCreate(ctx context.Context, owner *User, repo *Repository, refName string) error {
	return s.mirrorSyncAction(ctx, ActionMirrorSyncCreate, owner, repo, refName, nil)
}

// MirrorSyncDelete creates an action for mirror synchronization of a reference
// deletion.
func (s *ActionsStore) MirrorSyncDelete(ctx context.Context, owner *User, repo *Repository, refName string) error {
	return s.mirrorSyncAction(ctx, ActionMirrorSyncDelete, owner, repo, refName, nil)
}

// MergePullRequest creates an action for merging a pull request.
func (s *ActionsStore) MergePullRequest(ctx context.Context, doer, owner *User, repo *Repository, pull *Issue) error {
	return s.notifyWatchers(ctx,
		&Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       ActionMergePullRequest,
			Content:      fmt.Sprintf("%d|%s", pull.Index, pull.Title),
			RepoID:       repo.ID,
			RepoUserName: owner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
		},
	)
}

// TransferRepo creates an action for transferring a repository to a new owner.
func (s *ActionsStore) TransferRepo(ctx context.Context, doer, oldOwner, newOwner *User, repo *Repository) error {
	return s.notifyWatchers(ctx,
		&Action{
			ActUserID:    doer.ID,
			ActUserName:  doer.Name,
			OpType:       ActionTransferRepo,
			RepoID:       repo.ID,
			RepoUserName: newOwner.Name,
			RepoName:     repo.Name,
			IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
			Content:      oldOwner.Name + "/" + repo.Name,
		},
	)
}

var (
	// Same as GitHub, see https://docs.github.com/en/free-pro-team@latest/github/managing-your-work-on-github/linking-a-pull-request-to-an-issue
	issueCloseKeywords  = []string{"close", "closes", "closed", "fix", "fixes", "fixed", "resolve", "resolves", "resolved"}
	issueReopenKeywords = []string{"reopen", "reopens", "reopened"}

	issueCloseKeywordsPattern  = lazyregexp.New(assembleKeywordsPattern(issueCloseKeywords))
	issueReopenKeywordsPattern = lazyregexp.New(assembleKeywordsPattern(issueReopenKeywords))
	issueReferencePattern      = lazyregexp.New(`(?i)(?:)(^| )\S*#\d+`)
)

func assembleKeywordsPattern(words []string) string {
	return fmt.Sprintf(`(?i)(?:%s) \S+`, strings.Join(words, "|"))
}

// updateCommitReferencesToIssues checks if issues are manipulated by commit message.
func updateCommitReferencesToIssues(doer *User, repo *Repository, commits []*PushCommit) error {
	trimRightNonDigits := func(c rune) bool {
		return !unicode.IsDigit(c)
	}

	// Commits are appended in the reverse order.
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]

		refMarked := make(map[int64]bool)
		for _, ref := range issueReferencePattern.FindAllString(c.Message, -1) {
			ref = strings.TrimSpace(ref)
			ref = strings.TrimRightFunc(ref, trimRightNonDigits)

			if ref == "" {
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
				if IsErrIssueNotExist(err) {
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
		// FIXME: Can merge this and the next for loop to a common function.
		for _, ref := range issueCloseKeywordsPattern.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, trimRightNonDigits)

			if ref == "" {
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

			if err = issue.ChangeStatus(doer, repo, true); err != nil {
				return err
			}
		}

		// It is conflict to have close and reopen at same time, so refsMarkd doesn't need to reinit here.
		for _, ref := range issueReopenKeywordsPattern.FindAllString(c.Message, -1) {
			ref = ref[strings.IndexByte(ref, byte(' '))+1:]
			ref = strings.TrimRightFunc(ref, trimRightNonDigits)

			if ref == "" {
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

			if err = issue.ChangeStatus(doer, repo, false); err != nil {
				return err
			}
		}
	}
	return nil
}

type CommitRepoOptions struct {
	Owner       *User
	Repo        *Repository
	PusherName  string
	RefFullName string
	OldCommitID string
	NewCommitID string
	Commits     *PushCommits
}

// CommitRepo creates actions for pushing commits to the repository. An action
// with the type ActionDeleteBranch is created if the push deletes a branch; an
// action with the type ActionCommitRepo is created for a regular push. If the
// regular push also creates a new branch, then another action with type
// ActionCreateBranch is created.
func (s *ActionsStore) CommitRepo(ctx context.Context, opts CommitRepoOptions) error {
	err := newReposStore(s.db).Touch(ctx, opts.Repo.ID)
	if err != nil {
		return errors.Wrap(err, "touch repository")
	}

	pusher, err := NewUsersStore(s.db).GetByUsername(ctx, opts.PusherName)
	if err != nil {
		return errors.Wrapf(err, "get pusher [name: %s]", opts.PusherName)
	}

	isNewRef := opts.OldCommitID == git.EmptyID
	isDelRef := opts.NewCommitID == git.EmptyID

	// If not the first commit, set the compare URL.
	if !isNewRef && !isDelRef {
		opts.Commits.CompareURL = repoutil.CompareCommitsPath(opts.Owner.Name, opts.Repo.Name, opts.OldCommitID, opts.NewCommitID)
	}

	refName := git.RefShortName(opts.RefFullName)
	action := &Action{
		ActUserID:    pusher.ID,
		ActUserName:  pusher.Name,
		RepoID:       opts.Repo.ID,
		RepoUserName: opts.Owner.Name,
		RepoName:     opts.Repo.Name,
		RefName:      refName,
		IsPrivate:    opts.Repo.IsPrivate || opts.Repo.IsUnlisted,
	}

	apiRepo := opts.Repo.APIFormat(opts.Owner)
	apiPusher := pusher.APIFormat()
	if isDelRef {
		err = PrepareWebhooks(
			opts.Repo,
			HOOK_EVENT_DELETE,
			&api.DeletePayload{
				Ref:        refName,
				RefType:    "branch",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			},
		)
		if err != nil {
			return errors.Wrap(err, "prepare webhooks for delete branch")
		}

		action.OpType = ActionDeleteBranch
		err = s.notifyWatchers(ctx, action)
		if err != nil {
			return errors.Wrap(err, "notify watchers")
		}

		// Delete branch doesn't have anything to push or compare
		return nil
	}

	// Only update issues via commits when internal issue tracker is enabled
	if opts.Repo.EnableIssues && !opts.Repo.EnableExternalTracker {
		if err = updateCommitReferencesToIssues(pusher, opts.Repo, opts.Commits.Commits); err != nil {
			log.Error("update commit references to issues: %v", err)
		}
	}

	if conf.UI.FeedMaxCommitNum > 0 && len(opts.Commits.Commits) > conf.UI.FeedMaxCommitNum {
		opts.Commits.Commits = opts.Commits.Commits[:conf.UI.FeedMaxCommitNum]
	}

	data, err := jsoniter.Marshal(opts.Commits)
	if err != nil {
		return errors.Wrap(err, "marshal JSON")
	}
	action.Content = string(data)

	var compareURL string
	if isNewRef {
		err = PrepareWebhooks(
			opts.Repo,
			HOOK_EVENT_CREATE,
			&api.CreatePayload{
				Ref:           refName,
				RefType:       "branch",
				DefaultBranch: opts.Repo.DefaultBranch,
				Repo:          apiRepo,
				Sender:        apiPusher,
			},
		)
		if err != nil {
			return errors.Wrap(err, "prepare webhooks for new branch")
		}

		action.OpType = ActionCreateBranch
		err = s.notifyWatchers(ctx, action)
		if err != nil {
			return errors.Wrap(err, "notify watchers")
		}
	} else {
		compareURL = conf.Server.ExternalURL + opts.Commits.CompareURL
	}

	commits, err := opts.Commits.APIFormat(ctx,
		NewUsersStore(s.db),
		repoutil.RepositoryPath(opts.Owner.Name, opts.Repo.Name),
		repoutil.HTMLURL(opts.Owner.Name, opts.Repo.Name),
	)
	if err != nil {
		return errors.Wrap(err, "convert commits to API format")
	}

	err = PrepareWebhooks(
		opts.Repo,
		HOOK_EVENT_PUSH,
		&api.PushPayload{
			Ref:        opts.RefFullName,
			Before:     opts.OldCommitID,
			After:      opts.NewCommitID,
			CompareURL: compareURL,
			Commits:    commits,
			Repo:       apiRepo,
			Pusher:     apiPusher,
			Sender:     apiPusher,
		},
	)
	if err != nil {
		return errors.Wrap(err, "prepare webhooks for new commit")
	}

	action.OpType = ActionCommitRepo
	err = s.notifyWatchers(ctx, action)
	if err != nil {
		return errors.Wrap(err, "notify watchers")
	}
	return nil
}

type PushTagOptions struct {
	Owner       *User
	Repo        *Repository
	PusherName  string
	RefFullName string
	NewCommitID string
}

// PushTag creates an action for pushing tags to the repository. An action with
// the type ActionDeleteTag is created if the push deletes a tag. Otherwise, an
// action with the type ActionPushTag is created for a regular push.
func (s *ActionsStore) PushTag(ctx context.Context, opts PushTagOptions) error {
	err := newReposStore(s.db).Touch(ctx, opts.Repo.ID)
	if err != nil {
		return errors.Wrap(err, "touch repository")
	}

	pusher, err := NewUsersStore(s.db).GetByUsername(ctx, opts.PusherName)
	if err != nil {
		return errors.Wrapf(err, "get pusher [name: %s]", opts.PusherName)
	}

	refName := git.RefShortName(opts.RefFullName)
	action := &Action{
		ActUserID:    pusher.ID,
		ActUserName:  pusher.Name,
		RepoID:       opts.Repo.ID,
		RepoUserName: opts.Owner.Name,
		RepoName:     opts.Repo.Name,
		RefName:      refName,
		IsPrivate:    opts.Repo.IsPrivate || opts.Repo.IsUnlisted,
	}

	apiRepo := opts.Repo.APIFormat(opts.Owner)
	apiPusher := pusher.APIFormat()
	if opts.NewCommitID == git.EmptyID {
		err = PrepareWebhooks(
			opts.Repo,
			HOOK_EVENT_DELETE,
			&api.DeletePayload{
				Ref:        refName,
				RefType:    "tag",
				PusherType: api.PUSHER_TYPE_USER,
				Repo:       apiRepo,
				Sender:     apiPusher,
			},
		)
		if err != nil {
			return errors.Wrap(err, "prepare webhooks for delete tag")
		}

		action.OpType = ActionDeleteTag
		err = s.notifyWatchers(ctx, action)
		if err != nil {
			return errors.Wrap(err, "notify watchers")
		}
		return nil
	}

	err = PrepareWebhooks(
		opts.Repo,
		HOOK_EVENT_CREATE,
		&api.CreatePayload{
			Ref:           refName,
			RefType:       "tag",
			Sha:           opts.NewCommitID,
			DefaultBranch: opts.Repo.DefaultBranch,
			Repo:          apiRepo,
			Sender:        apiPusher,
		},
	)
	if err != nil {
		return errors.Wrapf(err, "prepare webhooks for new tag")
	}

	action.OpType = ActionPushTag
	err = s.notifyWatchers(ctx, action)
	if err != nil {
		return errors.Wrap(err, "notify watchers")
	}
	return nil
}

// ActionType is the type of action.
type ActionType int

// ⚠️ WARNING: Only append to the end of list to maintain backward compatibility.
const (
	ActionCreateRepo        ActionType = iota + 1 // 1
	ActionRenameRepo                              // 2
	ActionStarRepo                                // 3
	ActionWatchRepo                               // 4
	ActionCommitRepo                              // 5
	ActionCreateIssue                             // 6
	ActionCreatePullRequest                       // 7
	ActionTransferRepo                            // 8
	ActionPushTag                                 // 9
	ActionCommentIssue                            // 10
	ActionMergePullRequest                        // 11
	ActionCloseIssue                              // 12
	ActionReopenIssue                             // 13
	ActionClosePullRequest                        // 14
	ActionReopenPullRequest                       // 15
	ActionCreateBranch                            // 16
	ActionDeleteBranch                            // 17
	ActionDeleteTag                               // 18
	ActionForkRepo                                // 19
	ActionMirrorSyncPush                          // 20
	ActionMirrorSyncCreate                        // 21
	ActionMirrorSyncDelete                        // 22
)

// Action is a user operation to a repository. It implements template.Actioner
// interface to be able to use it in template rendering.
type Action struct {
	ID           int64 `gorm:"primaryKey"`
	UserID       int64 `gorm:"index"` // Receiver user ID
	OpType       ActionType
	ActUserID    int64  // Doer user ID
	ActUserName  string // Doer user name
	ActAvatar    string `xorm:"-" gorm:"-" json:"-"`
	RepoID       int64  `xorm:"INDEX" gorm:"index"`
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool   `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:FALSE"`
	Content      string `xorm:"TEXT"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (a *Action) BeforeCreate(tx *gorm.DB) error {
	if a.CreatedUnix <= 0 {
		a.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (a *Action) AfterFind(_ *gorm.DB) error {
	a.Created = time.Unix(a.CreatedUnix, 0).Local()
	return nil
}

func (a *Action) GetOpType() int {
	return int(a.OpType)
}

func (a *Action) GetActUserName() string {
	return a.ActUserName
}

func (a *Action) ShortActUserName() string {
	return strutil.Ellipsis(a.ActUserName, 20)
}

func (a *Action) GetRepoUserName() string {
	return a.RepoUserName
}

func (a *Action) ShortRepoUserName() string {
	return strutil.Ellipsis(a.RepoUserName, 20)
}

func (a *Action) GetRepoName() string {
	return a.RepoName
}

func (a *Action) ShortRepoName() string {
	return strutil.Ellipsis(a.RepoName, 33)
}

func (a *Action) GetRepoPath() string {
	return path.Join(a.RepoUserName, a.RepoName)
}

func (a *Action) ShortRepoPath() string {
	return path.Join(a.ShortRepoUserName(), a.ShortRepoName())
}

func (a *Action) GetRepoLink() string {
	if conf.Server.Subpath != "" {
		return path.Join(conf.Server.Subpath, a.GetRepoPath())
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
	index, _ := strconv.ParseInt(a.GetIssueInfos()[0], 10, 64)
	issue, err := GetIssueByIndex(a.RepoID, index)
	if err != nil {
		log.Error("Failed to get issue title [repo_id: %d, index: %d]: %v", a.RepoID, index, err)
		return "error getting issue"
	}
	return issue.Title
}

func (a *Action) GetIssueContent() string {
	index, _ := strconv.ParseInt(a.GetIssueInfos()[0], 10, 64)
	issue, err := GetIssueByIndex(a.RepoID, index)
	if err != nil {
		log.Error("Failed to get issue content [repo_id: %d, index: %d]: %v", a.RepoID, index, err)
		return "error getting issue"
	}
	return issue.Content
}

// PushCommit contains information of a pushed commit.
type PushCommit struct {
	Sha1           string
	Message        string
	AuthorEmail    string
	AuthorName     string
	CommitterEmail string
	CommitterName  string
	Timestamp      time.Time
}

// PushCommits is a list of pushed commits.
type PushCommits struct {
	Len        int
	Commits    []*PushCommit
	CompareURL string

	avatars map[string]string
}

// NewPushCommits returns a new PushCommits.
func NewPushCommits() *PushCommits {
	return &PushCommits{
		avatars: make(map[string]string),
	}
}

func (pcs *PushCommits) APIFormat(ctx context.Context, usersStore UsersStore, repoPath, repoURL string) ([]*api.PayloadCommit, error) {
	// NOTE: We cache query results in case there are many commits in a single push.
	usernameByEmail := make(map[string]string)
	getUsernameByEmail := func(email string) (string, error) {
		username, ok := usernameByEmail[email]
		if ok {
			return username, nil
		}

		user, err := usersStore.GetByEmail(ctx, email)
		if err != nil {
			if IsErrUserNotExist(err) {
				usernameByEmail[email] = ""
				return "", nil
			}
			return "", err
		}

		usernameByEmail[email] = user.Name
		return user.Name, nil
	}

	commits := make([]*api.PayloadCommit, len(pcs.Commits))
	for i, commit := range pcs.Commits {
		authorUsername, err := getUsernameByEmail(commit.AuthorEmail)
		if err != nil {
			return nil, errors.Wrap(err, "get author username")
		}

		committerUsername, err := getUsernameByEmail(commit.CommitterEmail)
		if err != nil {
			return nil, errors.Wrap(err, "get committer username")
		}

		nameStatus := &git.NameStatus{}
		if !testutil.InTest {
			nameStatus, err = git.ShowNameStatus(repoPath, commit.Sha1)
			if err != nil {
				return nil, errors.Wrapf(err, "show name status [commit_sha1: %s]", commit.Sha1)
			}
		}

		commits[i] = &api.PayloadCommit{
			ID:      commit.Sha1,
			Message: commit.Message,
			URL:     fmt.Sprintf("%s/commit/%s", repoURL, commit.Sha1),
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
			Added:     nameStatus.Added,
			Removed:   nameStatus.Removed,
			Modified:  nameStatus.Modified,
			Timestamp: commit.Timestamp,
		}
	}
	return commits, nil
}

// AvatarLink tries to match user in database with email in order to show custom
// avatars, and falls back to general avatar link.
//
// FIXME: This method does not belong to PushCommits, should be a pure template
// function.
func (pcs *PushCommits) AvatarLink(email string) string {
	_, ok := pcs.avatars[email]
	if !ok {
		u, err := Users.GetByEmail(context.Background(), email)
		if err != nil {
			pcs.avatars[email] = tool.AvatarLink(email)
			if !IsErrUserNotExist(err) {
				log.Error("Failed to get user [email: %s]: %v", email, err)
			}
		} else {
			pcs.avatars[email] = u.AvatarURLPath()
		}
	}

	return pcs.avatars[email]
}
