// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/strutil"
)

// ActionsStore is the persistent interface for actions.
//
// NOTE: All methods are sorted in alphabetical order.
type ActionsStore interface {
	// ListByOrganization returns actions of the organization viewable by the actor. Results are paginated
	// if `afterID` is given.
	ListByOrganization(ctx context.Context, orgID, actorID, afterID int64) ([]*Action, error)
	// ListByUser returns actions of the user viewable by the actor. Results are paginated if `afterID`
	// is given. The `isProfile` indicates whether repository permissions should be considered.
	ListByUser(ctx context.Context, userID, actorID, afterID int64, isProfile bool) ([]*Action, error)
	// NewRepo creates an action for creating a new repository. The action type could be ActionCreateRepo
	// or ActionForkRepo based on whether the repository is a fork.
	NewRepo(ctx context.Context, doer *User, repo *Repository) error
	// RenameRepo creates an action for renaming a repository.
	RenameRepo(ctx context.Context, doer *User, oldRepoName string, repo *Repository) error
}

var Actions ActionsStore

var _ ActionsStore = (*actions)(nil)

type actions struct {
	*gorm.DB
}

func (db *actions) ListByOrganization(ctx context.Context, orgID, actorID, afterID int64) ([]*Action, error) {
	/*
		Equivalent SQL for Postgres:

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
		LIMIT 20
	*/
	actions := make([]*Action, 0, conf.UI.User.NewsFeedPagingNum)
	return actions, db.WithContext(ctx).
		Where("user_id = ?", orgID).
		Where(db.
			// Not apply when afterID is not given
			Where("?", afterID <= 0).
			Or("id < ?", afterID),
		).
		Where("repo_id IN (?)",
			db.Select("repository.id").
				Table("repository").
				Joins("JOIN team_repo ON repository.id = team_repo.repo_id").
				Where("team_repo.team_id IN (?)",
					db.Select("team_id").
						Table("team_user").
						Where("team_user.org_id = ? AND uid = ?", orgID, actorID),
				).
				Or("repository.is_private = ? AND repository.is_unlisted = ?", false, false),
		).
		Limit(conf.UI.User.NewsFeedPagingNum).
		Order("id DESC").
		Find(&actions).Error
}

func (db *actions) ListByUser(ctx context.Context, userID, actorID, afterID int64, isProfile bool) ([]*Action, error) {
	/*
		Equivalent SQL for Postgres:

		SELECT * FROM "action"
		WHERE
			user_id = @userID
		AND (@skipAfter OR id < @afterID)
		AND (@includePrivate OR (is_private = FALSE AND act_user_id = @actorID))
		ORDER BY id DESC
		LIMIT @limit
	*/
	actions := make([]*Action, 0, conf.UI.User.NewsFeedPagingNum)
	return actions, db.WithContext(ctx).
		Where("user_id = ?", userID).
		Where(db.
			// Not apply when afterID is not given
			Where("?", afterID <= 0).
			Or("id < ?", afterID),
		).
		Where(db.
			// Not apply when in not profile page or the user is viewing own profile
			Where("?", !isProfile || actorID == userID).
			Or("is_private = ? AND act_user_id = ?", false, userID),
		).
		Limit(conf.UI.User.NewsFeedPagingNum).
		Order("id DESC").
		Find(&actions).Error
}

func (db *actions) NewRepo(ctx context.Context, doer *User, repo *Repository) error {
	opType := ActionCreateRepo
	if repo.IsFork {
		opType = ActionForkRepo
	}

	return db.notifyWatchers(ctx, &Action{
		ActUserID:    doer.ID,
		ActUserName:  doer.Name,
		OpType:       opType,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
	})
}

func (db *actions) RenameRepo(ctx context.Context, doer *User, oldRepoName string, repo *Repository) error {
	return db.notifyWatchers(ctx, &Action{
		ActUserID:    doer.ID,
		ActUserName:  doer.Name,
		OpType:       ActionRenameRepo,
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate || repo.IsUnlisted,
		Content:      oldRepoName,
	})
}

// notifyWatchers creates rows in action table for watchers who are able to see the action.
func (db *actions) notifyWatchers(ctx context.Context, act *Action) error {
	watches, err := Watches.ListByRepo(ctx, act.RepoID)
	if err != nil {
		return errors.Wrap(err, "get watches")
	}

	// clone returns a deep copy of the action with UserID assigned.
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

	return db.Create(&actions).Error
}

// ActionType is the type of an action.
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

// Action is a user operation to a repository. It implements template.Actioner interface
// to be able to use it in template rendering.
type Action struct {
	ID           int64 `gorm:"primarykey"`
	UserID       int64 `gorm:"index"` // Receiver user ID
	OpType       ActionType
	ActUserID    int64  // Doer user ID
	ActUserName  string // Doer user name
	ActAvatar    string `xorm:"-" gorm:"-" json:"-"`
	RepoID       int64  `xorm:"INDEX" gorm:"index"`
	RepoUserName string
	RepoName     string
	RefName      string
	IsPrivate    bool   `xorm:"NOT NULL DEFAULT false" gorm:"not null;default:false"`
	Content      string `xorm:"TEXT"`

	Created     time.Time `xorm:"-" gorm:"-" json:"-"`
	CreatedUnix int64
}

// NOTE: This is a GORM create hook.
func (a *Action) BeforeCreate(tx *gorm.DB) error {
	if a.CreatedUnix == 0 {
		a.CreatedUnix = tx.NowFunc().Unix()
	}
	return nil
}

// NOTE: This is a GORM query hook.
func (a *Action) AfterFind(tx *gorm.DB) error {
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
		log.Error("GetIssueByIndex: %v", err)
		return "error getting issue"
	}
	return issue.Title
}

func (a *Action) GetIssueContent() string {
	index, _ := strconv.ParseInt(a.GetIssueInfos()[0], 10, 64)
	issue, err := GetIssueByIndex(a.RepoID, index)
	if err != nil {
		log.Error("GetIssueByIndex: %v", err)
		return "error getting issue"
	}
	return issue.Content
}
