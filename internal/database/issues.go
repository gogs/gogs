package database

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/tool"
)

// IssuesStore is the storage layer for issues and pull requests of
// repositories, and the users related to them.
type IssuesStore struct {
	db *gorm.DB
}

func newIssuesStore(db *gorm.DB) *IssuesStore {
	return &IssuesStore{db: db}
}

// Create creates the issue with given labels and attachments, notifies the
// watchers of the repository and the users it mentions, and prepares
// webhooks. Labels of other repositories and attachments that are already
// attached are skipped, and so are a milestone and an assignee that do not
// exist.
func (s *IssuesStore) Create(ctx context.Context, logger *logx.Logger, repo *Repository, issue *Issue, labelIDs []int64, uuids []string) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newIssuesStore(tx).create(ctx, NewIssueOptions{
			Repo:        repo,
			Issue:       issue,
			LableIDs:    labelIDs,
			Attachments: uuids,
		})
	})
	if err != nil {
		return errors.Wrap(err, "create issue")
	}

	err = newActionsStore(s.db).notifyWatchers(ctx, &Action{
		ActUserID:    issue.Poster.ID,
		ActUserName:  issue.Poster.Name,
		OpType:       ActionCreateIssue,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	})
	if err != nil {
		logger.Error("Failed to notify watchers", "error", err)
	}
	if err = s.mailParticipants(ctx, logger, issue); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}

	err = newWebhooksStore(s.db).Prepare(ctx, logger, repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
		Action:     apiv1types.WebhookIssueOpened,
		Index:      issue.Index,
		Issue:      issue.APIFormat(),
		Repository: repo.APIFormatLegacy(logger, nil),
		Sender:     issue.Poster.APIFormat(),
	})
	if err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
	}
	return nil
}

// create is Create without a transaction of its own, notifications and
// webhooks.
func (s *IssuesStore) create(ctx context.Context, opts NewIssueOptions) error {
	db := s.db.WithContext(ctx)
	opts.Issue.Title = strings.TrimSpace(opts.Issue.Title)
	opts.Issue.Index = opts.Repo.NextIssueIndex()

	if opts.Issue.MilestoneID > 0 {
		milestones := newMilestonesStore(s.db)
		milestone, err := milestones.GetByID(ctx, opts.Issue.RepoID, opts.Issue.MilestoneID)
		if err != nil && !IsErrMilestoneNotExist(err) {
			return errors.Wrap(err, "get milestone")
		}

		opts.Issue.MilestoneID = 0
		if milestone != nil {
			opts.Issue.MilestoneID = milestone.ID
			opts.Issue.Milestone = milestone
			if err = milestones.assignIssue(ctx, opts.Issue, -1); err != nil {
				return errors.Wrap(err, "assign milestone")
			}
		}
	}

	if opts.Issue.AssigneeID > 0 {
		assignee, err := newUsersStore(s.db).GetByID(ctx, opts.Issue.AssigneeID)
		if err != nil && !IsErrUserNotExist(err) {
			return errors.Wrap(err, "get assignee")
		}

		opts.Issue.AssigneeID = 0
		if assignee != nil {
			opts.Issue.AssigneeID = assignee.ID
			opts.Issue.Assignee = assignee
		}
	}

	// The milestone and the assignee are checked before the issue is inserted.
	err := db.Create(opts.Issue).Error
	if err != nil {
		return errors.Wrap(err, "insert issue")
	}

	if opts.IsPull {
		err = db.Exec("UPDATE repository SET num_pulls = num_pulls + 1 WHERE id = ?", opts.Issue.RepoID).Error
	} else {
		err = db.Exec("UPDATE repository SET num_issues = num_issues + 1 WHERE id = ?", opts.Issue.RepoID).Error
	}
	if err != nil {
		return errors.Wrap(err, "count issue")
	}

	if len(opts.LableIDs) > 0 {
		labels, err := newLabelsStore(s.db).ListByIDs(ctx, opts.Repo.ID, opts.LableIDs)
		if err != nil {
			return errors.Wrap(err, "list labels")
		}
		err = newLabelsStore(s.db).addToIssue(ctx, opts.Issue, labels...)
		if err != nil {
			return errors.Wrap(err, "add labels")
		}
	}

	if err = s.createIssueUsers(ctx, opts.Repo, opts.Issue); err != nil {
		return errors.Wrap(err, "create issue users")
	}

	err = newAttachmentsStore(s.db).Bind(ctx, opts.Issue.ID, 0, opts.Attachments)
	if err != nil {
		return errors.Wrap(err, "attach files")
	}
	return s.LoadAttributes(ctx, opts.Issue)
}

// createIssueUsers creates the relations of the new issue to the users who
// can be assigned in its repository and to its poster.
func (s *IssuesStore) createIssueUsers(ctx context.Context, repo *Repository, issue *Issue) error {
	assignees, err := repo.getAssignees(s.db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "list assignees")
	}

	issueUsers := make([]*IssueUser, 0, len(assignees)+1)
	isPosterAssignee := false
	for _, assignee := range assignees {
		isPoster := assignee.ID == issue.PosterID
		issueUsers = append(issueUsers, &IssueUser{
			IssueID:    issue.ID,
			RepoID:     repo.ID,
			UserID:     assignee.ID,
			IsPoster:   isPoster,
			IsAssigned: assignee.ID == issue.AssigneeID,
		})
		isPosterAssignee = isPosterAssignee || isPoster
	}
	if !isPosterAssignee {
		issueUsers = append(issueUsers, &IssueUser{
			IssueID:  issue.ID,
			RepoID:   repo.ID,
			UserID:   issue.PosterID,
			IsPoster: true,
		})
	}
	return s.db.WithContext(ctx).Create(issueUsers).Error
}

func (s *IssuesStore) getRaw(ctx context.Context, args map[string]any, query string, values ...any) (*Issue, error) {
	issue := new(Issue)
	err := s.db.WithContext(ctx).Where(query, values...).First(issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIssueNotExist{args: args}
		}
		return nil, err
	}
	return issue, nil
}

// GetByID returns the issue with given ID and its attributes. It returns
// ErrIssueNotExist when not found.
func (s *IssuesStore) GetByID(ctx context.Context, id int64) (*Issue, error) {
	issue, err := s.getRaw(ctx, map[string]any{"issueID": id}, "id = ?", id)
	if err != nil {
		return nil, err
	}
	return issue, s.LoadAttributes(ctx, issue)
}

// GetByIndex returns the issue with given index of the repository with given
// ID and its attributes. It returns ErrIssueNotExist when not found.
func (s *IssuesStore) GetByIndex(ctx context.Context, repoID, index int64) (*Issue, error) {
	issue, err := s.getRaw(ctx, map[string]any{"repoID": repoID, "index": index}, "repo_id = ? AND "+s.db.Statement.Quote("index")+" = ?", repoID, index)
	if err != nil {
		return nil, err
	}
	return issue, s.LoadAttributes(ctx, issue)
}

// getByRef returns the issue of given reference, e.g., "owner/repo#123", and
// its attributes. It returns ErrIssueNotExist when the reference is malformed
// or the issue is not found.
func (s *IssuesStore) getByRef(ctx context.Context, ref string) (*Issue, error) {
	before, after, ok := strings.Cut(ref, "#")
	if !ok {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	index, _ := strconv.ParseInt(after, 10, 64)
	if index <= 0 {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	repo, err := GetRepositoryByRef(before)
	if err != nil {
		return nil, err
	}
	return s.GetByIndex(ctx, repo.ID, index)
}

// LoadAttributes sets the repository, the poster, the labels, the milestone,
// the assignee, the pull request, the attachments and the comments of given
// issues that are not set yet. Issues whose poster no longer exists get the
// ghost user.
func (s *IssuesStore) LoadAttributes(ctx context.Context, issues ...*Issue) (err error) {
	for _, issue := range issues {
		if issue.Repo == nil {
			issue.Repo, err = getRepositoryByID(s.db.WithContext(ctx), issue.RepoID)
			if err != nil {
				return errors.Wrapf(err, "get repository %d", issue.RepoID)
			}
		}

		if issue.Poster == nil {
			issue.Poster, err = newUsersStore(s.db).GetByID(ctx, issue.PosterID)
			if err != nil {
				if !IsErrUserNotExist(err) {
					return errors.Wrapf(err, "get poster %d", issue.PosterID)
				}
				issue.PosterID = -1
				issue.Poster = NewGhostUser()
			}
		}

		if issue.Labels == nil {
			issue.Labels, err = newLabelsStore(s.db).ListByIssueID(ctx, issue.ID)
			if err != nil {
				return errors.Wrap(err, "list labels")
			}
		}

		if issue.Milestone == nil && issue.MilestoneID > 0 {
			issue.Milestone, err = newMilestonesStore(s.db).GetByID(ctx, issue.RepoID, issue.MilestoneID)
			if err != nil {
				return errors.Wrapf(err, "get milestone %d", issue.MilestoneID)
			}
		}

		if issue.Assignee == nil && issue.AssigneeID > 0 {
			issue.Assignee, err = newUsersStore(s.db).GetByID(ctx, issue.AssigneeID)
			if err != nil {
				return errors.Wrapf(err, "get assignee %d", issue.AssigneeID)
			}
		}

		if issue.IsPull && issue.PullRequest == nil {
			// The pull request is not created yet while its issue is.
			issue.PullRequest, err = newPullRequestsStore(s.db).GetByIssueID(ctx, issue.ID)
			if err != nil && !IsErrPullRequestNotExist(err) {
				return errors.Wrap(err, "get pull request")
			}
		}

		if issue.Attachments == nil {
			issue.Attachments, err = newAttachmentsStore(s.db).ListByIssueID(ctx, issue.ID)
			if err != nil {
				return errors.Wrap(err, "list attachments")
			}
		}

		if issue.Comments == nil {
			issue.Comments, err = newCommentsStore(s.db).ListByIssueID(ctx, issue.ID, 0)
			if err != nil {
				return errors.Wrap(err, "list comments")
			}
		}
	}
	return nil
}

// buildListQuery returns nil when no issue can match.
func (s *IssuesStore) buildListQuery(ctx context.Context, opts *IssuesOptions) *gorm.DB {
	db := s.db.WithContext(ctx).Model(new(Issue))

	if opts.RepoID > 0 {
		db = db.Where("issue.repo_id = ? AND issue.is_closed = ?", opts.RepoID, opts.IsClosed)
	} else if opts.RepoIDs != nil {
		if len(opts.RepoIDs) == 0 {
			return nil
		}
		db = db.Where("issue.repo_id IN ? AND issue.is_closed = ?", opts.RepoIDs, opts.IsClosed)
	} else {
		db = db.Where("issue.is_closed = ?", opts.IsClosed)
	}

	if opts.AssigneeID > 0 {
		db = db.Where("issue.assignee_id = ?", opts.AssigneeID)
	} else if opts.PosterID > 0 {
		db = db.Where("issue.poster_id = ?", opts.PosterID)
	}

	if opts.MilestoneID > 0 {
		db = db.Where("issue.milestone_id = ?", opts.MilestoneID)
	}

	db = db.Where("issue.is_pull = ?", opts.IsPull)

	if len(opts.Labels) > 0 && opts.Labels != "0" {
		labelIDs := tool.StringsToInt64s(strings.Split(opts.Labels, ","))
		db = db.Where("issue.id IN (?)", s.db.Model(new(IssueLabel)).Select("issue_id").Where("label_id IN ?", labelIDs))
	}

	if opts.IsMention {
		db = db.Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").Where("issue_user.is_mentioned = ?", true)
		if opts.UserID > 0 {
			db = db.Where("issue_user.uid = ?", opts.UserID)
		}
	}
	return db
}

// Count returns the number of issues that match given options.
func (s *IssuesStore) Count(ctx context.Context, opts *IssuesOptions) (int64, error) {
	db := s.buildListQuery(ctx, opts)
	if db == nil {
		return 0, nil
	}

	var count int64
	return count, db.Count(&count).Error
}

// List returns the issues that match given options on the page of the
// options, and their attributes.
func (s *IssuesStore) List(ctx context.Context, opts *IssuesOptions) ([]*Issue, error) {
	db := s.buildListQuery(ctx, opts)
	if db == nil {
		return []*Issue{}, nil
	}

	// Issues that sort the same are listed by ID in the same direction.
	order := "DESC"
	switch opts.SortType {
	case "oldest":
		db, order = db.Order("issue.created_unix ASC"), "ASC"
	case "recentupdate":
		db = db.Order("issue.updated_unix DESC")
	case "leastupdate":
		db, order = db.Order("issue.updated_unix ASC"), "ASC"
	case "mostcomment":
		db = db.Order("issue.num_comments DESC")
	case "leastcomment":
		db, order = db.Order("issue.num_comments ASC"), "ASC"
	case "priority":
		db = db.Order("issue.priority DESC")
	default:
		db = db.Order("issue.created_unix DESC")
	}

	page := max(opts.Page, 1)
	issues := make([]*Issue, 0, conf.UI.IssuePagingNum)
	err := db.Select("issue.*").
		Order("issue.id " + order).
		Limit(conf.UI.IssuePagingNum).
		Offset((page - 1) * conf.UI.IssuePagingNum).
		Find(&issues).Error
	if err != nil {
		return nil, errors.Wrap(err, "list issues")
	}
	return issues, s.LoadAttributes(ctx, issues...)
}

// Suggest returns the issues and pull requests to suggest when referencing
// one, without their attributes. The issue whose number is the keyword comes
// first, then the most recently updated ones.
func (s *IssuesStore) Suggest(ctx context.Context, opts SuggestIssuesOptions) ([]*Issue, error) {
	if !opts.IncludeIssues && !opts.IncludePulls {
		return []*Issue{}, nil
	}

	db := s.db.WithContext(ctx).Where("repo_id = ?", opts.RepoID)
	if opts.IncludeIssues != opts.IncludePulls {
		db = db.Where("is_pull = ?", opts.IncludePulls)
	}

	keyword := strings.TrimSpace(opts.Keyword)
	if keyword != "" {
		conds := []string{"LOWER(name) LIKE ?"}
		args := []any{"%" + strings.ToLower(keyword) + "%"}
		if index, err := strconv.ParseInt(keyword, 10, 64); err == nil && index > 0 {
			// Numbers starting with n are in [n, n+1), [10n, 10n+10) and so on.
			indexCol := db.Statement.Quote("index")
			for lo, hi := index, index+1; ; lo, hi = lo*10, hi*10 {
				conds = append(conds, fmt.Sprintf("(%[1]s >= ? AND %[1]s < ?)", indexCol))
				args = append(args, lo, hi)
				if lo > math.MaxInt64/10 || hi > math.MaxInt64/10 {
					break
				}
			}
			db = db.Order(fmt.Sprintf("CASE WHEN %s = %d THEN 0 ELSE 1 END", indexCol, index))
		}
		db = db.Where("("+strings.Join(conds, " OR ")+")", args...)
	}

	issues := make([]*Issue, 0, opts.Limit)
	err := db.Order("updated_unix DESC").Order("id DESC").Limit(opts.Limit).Find(&issues).Error
	if err != nil {
		return nil, errors.Wrap(err, "list issues")
	}
	return issues, nil
}

// countOpenClosed returns the numbers of open and closed issues that match
// the query.
func countOpenClosed(db *gorm.DB) (open, closed int64, err error) {
	if err = db.Session(&gorm.Session{}).Where("issue.is_closed = ?", false).Count(&open).Error; err != nil {
		return 0, 0, errors.Wrap(err, "count open issues")
	}
	if err = db.Session(&gorm.Session{}).Where("issue.is_closed = ?", true).Count(&closed).Error; err != nil {
		return 0, 0, errors.Wrap(err, "count closed issues")
	}
	return open, closed, nil
}

// Stats returns the numbers of open and closed issues of a repository that
// match given options.
func (s *IssuesStore) Stats(ctx context.Context, opts *IssueStatsOptions) (*IssueStats, error) {
	db := s.db.WithContext(ctx).Model(new(Issue)).Where("issue.repo_id = ? AND issue.is_pull = ?", opts.RepoID, opts.IsPull)
	if len(opts.Labels) > 0 && opts.Labels != "0" {
		labelIDs := tool.StringsToInt64s(strings.Split(opts.Labels, ","))
		db = db.Where("issue.id IN (?)", s.db.Model(new(IssueLabel)).Select("issue_id").Where("label_id IN ?", labelIDs))
	}
	if opts.MilestoneID > 0 {
		db = db.Where("issue.milestone_id = ?", opts.MilestoneID)
	}
	if opts.AssigneeID > 0 {
		db = db.Where("issue.assignee_id = ?", opts.AssigneeID)
	}

	switch opts.FilterMode {
	case FilterModeYourRepos, FilterModeAssign:
	case FilterModeCreate:
		db = db.Where("issue.poster_id = ?", opts.UserID)
	case FilterModeMention:
		db = db.Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").
			Where("issue_user.uid = ? AND issue_user.is_mentioned = ?", opts.UserID, true)
	default:
		return &IssueStats{}, nil
	}

	stats := &IssueStats{}
	var err error
	stats.OpenCount, stats.ClosedCount, err = countOpenClosed(db)
	return stats, err
}

// UserStats returns the numbers of open issues assigned to and created by the
// user with given ID, and of those in given repositories, and the numbers of
// open and closed issues for given filter mode. A repository ID that is not 0
// takes precedence over repository IDs.
func (s *IssuesStore) UserStats(ctx context.Context, repoID, userID int64, repoIDs []int64, filterMode FilterMode, isPull bool) (*IssueStats, error) {
	inRepos := func(repoIDs []int64) *gorm.DB {
		db := s.db.WithContext(ctx).Model(new(Issue)).Where("issue.is_pull = ?", isPull)
		if repoID > 0 {
			return db.Where("issue.repo_id = ?", repoID)
		} else if len(repoIDs) > 0 {
			return db.Where("issue.repo_id IN ?", repoIDs)
		}
		return db
	}
	assigned := inRepos(nil).Where("issue.assignee_id = ?", userID)
	created := inRepos(nil).Where("issue.poster_id = ?", userID)
	hasAnyRepo := repoID > 0 || len(repoIDs) > 0

	stats := &IssueStats{}
	var err error
	if stats.AssignCount, _, err = countOpenClosed(assigned); err != nil {
		return nil, errors.Wrap(err, "count assigned")
	}
	if stats.CreateCount, _, err = countOpenClosed(created); err != nil {
		return nil, errors.Wrap(err, "count created")
	}
	var yourRepos struct{ open, closed int64 }
	if hasAnyRepo {
		yourRepos.open, yourRepos.closed, err = countOpenClosed(inRepos(repoIDs))
		if err != nil {
			return nil, errors.Wrap(err, "count in repositories")
		}
		stats.YourReposCount = yourRepos.open
	}

	switch filterMode {
	case FilterModeYourRepos:
		stats.OpenCount, stats.ClosedCount = yourRepos.open, yourRepos.closed
	case FilterModeAssign:
		stats.OpenCount, stats.ClosedCount, err = countOpenClosed(assigned)
	case FilterModeCreate:
		stats.OpenCount, stats.ClosedCount, err = countOpenClosed(created)
	}
	return stats, err
}

// ListIssueUsers returns the relations of the user with given ID to the open
// or closed issues of the repository with given ID.
func (s *IssuesStore) ListIssueUsers(ctx context.Context, repoID, userID int64, isClosed bool) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	return ius, s.db.WithContext(ctx).Where("repo_id = ? AND uid = ? AND is_closed = ?", repoID, userID, isClosed).Find(&ius).Error
}

// MarkRead marks the issue with given ID as read by the user with given ID.
func (s *IssuesStore) MarkRead(ctx context.Context, userID, issueID int64) error {
	return s.db.WithContext(ctx).Exec("UPDATE issue_user SET is_read = ? WHERE uid = ? AND issue_id = ?", true, userID, issueID).Error
}

// Update saves all fields of the issue.
func (s *IssuesStore) Update(ctx context.Context, issue *Issue) error {
	// NOTE: The explicit condition keeps updating an issue that is not yet
	// inserted (i.e., ID is 0) a no-op instead of an error.
	return s.db.WithContext(ctx).Model(issue).Where("id = ?", issue.ID).Select("*").Updates(issue).Error
}

// updateCols saves given columns and the updated time of the issue.
func (s *IssuesStore) updateCols(ctx context.Context, issue *Issue, cols ...string) error {
	cols = append(cols, "updated_unix")
	return s.db.WithContext(ctx).Model(issue).Where("id = ?", issue.ID).Select(cols).Updates(issue).Error
}

// UpdateAssignee saves all fields of the issue, and marks its assignee, if
// any, as the only user assigned to it.
func (s *IssuesStore) UpdateAssignee(ctx context.Context, issue *Issue) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Exec("UPDATE issue_user SET is_assigned = ? WHERE issue_id = ?", false, issue.ID).Error
		if err != nil {
			return errors.Wrap(err, "unassign users")
		}
		if issue.AssigneeID > 0 {
			err = tx.Exec("UPDATE issue_user SET is_assigned = ? WHERE uid = ? AND issue_id = ?", true, issue.AssigneeID, issue.ID).Error
			if err != nil {
				return errors.Wrap(err, "assign user")
			}
		}
		return newIssuesStore(tx).Update(ctx, issue)
	})
}

// prepareWebhooks prepares the deliveries of given action on the issue, or on
// its pull request, of given repository as the doer. Failing to prepare them
// is logged.
func (s *IssuesStore) prepareWebhooks(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, action apiv1types.WebhookIssueAction, changes *apiv1types.WebhookChangesPayload) {
	webhooks := newWebhooksStore(s.db)
	var err error
	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = webhooks.Prepare(ctx, logger, repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      action,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Changes:     changes,
			Repository:  repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = webhooks.Prepare(ctx, logger, repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     action,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Changes:    changes,
			Repository: repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "issueID", issue.ID, "action", action, "error", err)
	}
}

// ChangeStatus closes or reopens the issue of given repository as the doer,
// updates the numbers of closed issues of its labels and milestone, and
// comments on it. Changing the issue to the status it has is a no-op.
func (s *IssuesStore) ChangeStatus(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, isClosed bool) error {
	if issue.IsClosed == isClosed {
		return nil
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newIssuesStore(tx).changeStatus(ctx, logger, doer, repo, issue, isClosed)
	})
	if err != nil {
		return err
	}

	action := apiv1types.WebhookIssueReopened
	if isClosed {
		action = apiv1types.WebhookIssueClosed
	}
	s.prepareWebhooks(ctx, logger, doer, repo, issue, action, nil)
	return nil
}

// changeStatus is ChangeStatus without a transaction of its own and webhooks.
func (s *IssuesStore) changeStatus(ctx context.Context, logger *logx.Logger, doer *User, repo *Repository, issue *Issue, isClosed bool) (err error) {
	if issue.IsClosed == isClosed {
		return nil
	}
	issue.IsClosed = isClosed

	if err = s.updateCols(ctx, issue, "is_closed"); err != nil {
		return errors.Wrap(err, "update issue")
	}
	err = s.db.WithContext(ctx).Exec("UPDATE issue_user SET is_closed = ? WHERE issue_id = ?", isClosed, issue.ID).Error
	if err != nil {
		return errors.Wrap(err, "update issue users")
	}

	labels := newLabelsStore(s.db)
	if len(issue.Labels) == 0 {
		issue.Labels, err = labels.ListByIssueID(ctx, issue.ID)
		if err != nil {
			return errors.Wrap(err, "list labels")
		}
	}
	for _, label := range issue.Labels {
		if isClosed {
			label.NumClosedIssues++
		} else {
			label.NumClosedIssues--
		}
		if err = labels.Update(ctx, label); err != nil {
			return errors.Wrapf(err, "update label %d", label.ID)
		}
	}

	if err = newMilestonesStore(s.db).updateIssueStats(ctx, issue); err != nil {
		return errors.Wrap(err, "update milestone")
	}

	_, err = newCommentsStore(s.db).createStatus(ctx, logger, doer, repo, issue)
	if err != nil {
		return errors.Wrap(err, "create comment")
	}
	return nil
}

// ChangeTitle changes the title of the issue as the doer.
func (s *IssuesStore) ChangeTitle(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue, title string) error {
	oldTitle := issue.Title
	issue.Title = title
	if err := s.updateCols(ctx, issue, "name"); err != nil {
		return errors.Wrap(err, "update issue")
	}

	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, apiv1types.WebhookIssueEdited, &apiv1types.WebhookChangesPayload{
		Title: &apiv1types.WebhookChangesFromPayload{From: oldTitle},
	})
	return nil
}

// ChangeContent changes the content of the issue as the doer.
func (s *IssuesStore) ChangeContent(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue, content string) error {
	oldContent := issue.Content
	issue.Content = content
	if err := s.updateCols(ctx, issue, "content"); err != nil {
		return errors.Wrap(err, "update issue")
	}

	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, apiv1types.WebhookIssueEdited, &apiv1types.WebhookChangesPayload{
		Body: &apiv1types.WebhookChangesFromPayload{From: oldContent},
	})
	return nil
}

// ChangeAssignee assigns the issue to the user with given ID as the doer, or
// unassigns it when the user does not exist, e.g., when the ID is 0.
func (s *IssuesStore) ChangeAssignee(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue, assigneeID int64) error {
	issue.AssigneeID = assigneeID
	if err := s.UpdateAssignee(ctx, issue); err != nil {
		return errors.Wrap(err, "update assignee")
	}

	assignee, err := newUsersStore(s.db).GetByID(ctx, assigneeID)
	if err != nil && !IsErrUserNotExist(err) {
		logger.Error("Failed to get assignee", "userID", assigneeID, "error", err)
		return nil
	}
	issue.Assignee = assignee

	action := apiv1types.WebhookIssueAssigned
	if assignee == nil {
		action = apiv1types.WebhookIssueUnassigned
	}
	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, action, nil)
	return nil
}

// AddLabels adds given labels to the issue as the doer. Labels that the issue
// has are skipped.
func (s *IssuesStore) AddLabels(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue, labels ...*Label) error {
	err := newLabelsStore(s.db).AddToIssue(ctx, issue, labels...)
	if err != nil {
		return err
	}

	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, apiv1types.WebhookIssueLabelUpdated, nil)
	return nil
}

// RemoveLabel removes the label from the issue as the doer.
func (s *IssuesStore) RemoveLabel(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue, label *Label) error {
	err := newLabelsStore(s.db).RemoveFromIssue(ctx, issue, label)
	if err != nil {
		return err
	}

	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, apiv1types.WebhookIssueLabelUpdated, nil)
	return nil
}

// clearLabels removes all labels from the issue.
func (s *IssuesStore) clearLabels(ctx context.Context, issue *Issue) error {
	labels := newLabelsStore(s.db)
	current, err := labels.ListByIssueID(ctx, issue.ID)
	if err != nil {
		return errors.Wrap(err, "list labels")
	}
	issue.Labels = slices.Clone(current)
	for _, label := range current {
		if err = labels.removeFromIssue(ctx, issue, label); err != nil {
			return errors.Wrapf(err, "remove label %d", label.ID)
		}
	}
	return nil
}

// ClearLabels removes all labels from the issue as the doer.
func (s *IssuesStore) ClearLabels(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newIssuesStore(tx).clearLabels(ctx, issue)
	})
	if err != nil {
		return err
	}

	s.prepareWebhooks(ctx, logger, doer, issue.Repo, issue, apiv1types.WebhookIssueLabelCleared, nil)
	return nil
}

// ReplaceLabels replaces the labels of the issue with given labels.
func (s *IssuesStore) ReplaceLabels(ctx context.Context, issue *Issue, labels ...*Label) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := newIssuesStore(tx).clearLabels(ctx, issue)
		if err != nil {
			return errors.Wrap(err, "clear labels")
		}
		return newLabelsStore(tx).addToIssue(ctx, issue, labels...)
	})
}

// listParticipants returns the users who commented on the issue with given
// ID.
func (s *IssuesStore) listParticipants(ctx context.Context, issueID int64) ([]*User, error) {
	db := s.db.WithContext(ctx)
	users := make([]*User, 0, 5)
	return users, db.Where("id IN (?)", db.Model(new(Comment)).Select("poster_id").Where("issue_id = ?", issueID)).Find(&users).Error
}

// findLowerMentions returns the lowercase names that the content mentions.
func findLowerMentions(content string) []string {
	mentions := markup.FindAllMentions(content)
	for i := range mentions {
		mentions[i] = strings.ToLower(mentions[i])
	}
	return mentions
}

// updateMentions marks the issue with given ID as mentioning the users with
// given lowercase names, and the members of the organizations with them.
func (s *IssuesStore) updateMentions(ctx context.Context, issueID int64, lowerNames []string) error {
	if len(lowerNames) == 0 {
		return nil
	}

	db := s.db.WithContext(ctx)
	users := make([]*User, 0, len(lowerNames))
	err := db.Where("lower_name IN ?", lowerNames).Order("lower_name ASC").Find(&users).Error
	if err != nil {
		return errors.Wrap(err, "list mentioned users")
	}

	userIDs := make([]int64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
		if !user.IsOrganization() || user.NumMembers == 0 {
			continue
		}

		members, err := newOrganizationsStore(s.db).listOrgUsers(ctx, user.ID, 0)
		if err != nil {
			return errors.Wrapf(err, "list members of organization %d", user.ID)
		}
		for _, member := range members {
			userIDs = append(userIDs, member.UID)
		}
	}

	for _, userID := range userIDs {
		iu := new(IssueUser)
		err := db.Where("uid = ? AND issue_id = ?", userID, issueID).First(iu).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.Wrapf(err, "get issue user %d", userID)
		}

		if err == nil {
			err = db.Model(iu).Update("is_mentioned", true).Error
		} else {
			err = db.Create(&IssueUser{UserID: userID, IssueID: issueID, IsMentioned: true}).Error
		}
		if err != nil {
			return errors.Wrapf(err, "mention user %d", userID)
		}
	}
	return nil
}

// mailParticipants marks the users the issue mentions as mentioned, and sends
// emails about the issue to them and to the participants of the issue.
func (s *IssuesStore) mailParticipants(ctx context.Context, logger *logx.Logger, issue *Issue) error {
	mentions := findLowerMentions(issue.Content)
	err := s.updateMentions(ctx, issue.ID, mentions)
	if err != nil {
		return errors.Wrap(err, "update mentions")
	}

	if err = mailIssueCommentToParticipants(logger, s.db.WithContext(ctx), issue, issue.Poster, mentions); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}
	return nil
}
