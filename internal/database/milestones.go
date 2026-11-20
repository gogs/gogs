package database

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// Milestone represents a milestone of repository.
type Milestone struct {
	ID              int64
	RepoID          int64 `gorm:"index:IDX_milestone_repo_id"`
	Name            string
	Content         string `gorm:"type:TEXT"`
	RenderedContent string `json:"-" gorm:"-"`
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `json:"-" gorm:"-"`
	Completeness    int  // Percentage(1-100).
	IsOverDue       bool `json:"-" gorm:"-"`

	DeadlineString string    `json:"-" gorm:"-"`
	Deadline       time.Time `json:"-" gorm:"-"`
	DeadlineUnix   int64
	ClosedDate     time.Time `json:"-" gorm:"-"`
	ClosedDateUnix int64
}

// BeforeCreate implements the GORM create hook.
func (m *Milestone) BeforeCreate(_ *gorm.DB) error {
	if m.DeadlineUnix == 0 {
		m.DeadlineUnix = m.Deadline.Unix()
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (m *Milestone) BeforeUpdate(_ *gorm.DB) error {
	m.DeadlineUnix = m.Deadline.Unix()
	m.ClosedDateUnix = m.ClosedDate.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (m *Milestone) AfterFind(_ *gorm.DB) error {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues

	m.Deadline = time.Unix(m.DeadlineUnix, 0).Local()
	if m.Deadline.Year() != 9999 {
		m.DeadlineString = m.Deadline.Format("2006-01-02")
		if time.Now().Local().After(m.Deadline) {
			m.IsOverDue = true
		}
	}

	m.ClosedDate = time.Unix(m.ClosedDateUnix, 0).Local()
	return nil
}

// State returns string representation of milestone status.
func (m *Milestone) State() apiv1types.IssueStateType {
	if m.IsClosed {
		return apiv1types.IssueStateClosed
	}
	return apiv1types.IssueStateOpen
}

func (m *Milestone) APIFormat() *apiv1types.IssueMilestone {
	apiMilestone := &apiv1types.IssueMilestone{
		ID:           m.ID,
		State:        m.State(),
		Title:        m.Name,
		Description:  m.Content,
		OpenIssues:   m.NumOpenIssues,
		ClosedIssues: m.NumClosedIssues,
	}
	if m.IsClosed {
		apiMilestone.Closed = &m.ClosedDate
	}
	if m.Deadline.Year() < 9999 {
		apiMilestone.Deadline = &m.Deadline
	}
	return apiMilestone
}

var _ errx.NotFound = (*ErrMilestoneNotExist)(nil)

type ErrMilestoneNotExist struct {
	args map[string]any
}

func IsErrMilestoneNotExist(err error) bool {
	_, ok := err.(ErrMilestoneNotExist)
	return ok
}

func (err ErrMilestoneNotExist) Error() string {
	return fmt.Sprintf("milestone does not exist: %v", err.args)
}

func (ErrMilestoneNotExist) NotFound() bool {
	return true
}

// MilestonesStore is the storage layer for milestones of repositories.
type MilestonesStore struct {
	db *gorm.DB
}

func newMilestonesStore(db *gorm.DB) *MilestonesStore {
	return &MilestonesStore{db: db}
}

// Create creates the milestone and counts it in its repository.
func (s *MilestonesStore) Create(ctx context.Context, m *Milestone) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Create(m).Error
		if err != nil {
			return errors.Wrap(err, "create milestone")
		}
		return tx.Exec("UPDATE repository SET num_milestones = num_milestones + 1 WHERE id = ?", m.RepoID).Error
	})
}

// GetByID returns the milestone with given ID of the repository with given
// ID. It returns ErrMilestoneNotExist when not found.
func (s *MilestonesStore) GetByID(ctx context.Context, repoID, id int64) (*Milestone, error) {
	m := new(Milestone)
	err := s.db.WithContext(ctx).Where("repo_id = ? AND id = ?", repoID, id).First(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMilestoneNotExist{args: map[string]any{"repoID": repoID, "milestoneID": id}}
		}
		return nil, err
	}
	return m, nil
}

// ListByRepoID returns all milestones of the repository with given ID.
func (s *MilestonesStore) ListByRepoID(ctx context.Context, repoID int64) ([]*Milestone, error) {
	milestones := make([]*Milestone, 0, 10)
	return milestones, s.db.WithContext(ctx).Where("repo_id = ?", repoID).Order("id ASC").Find(&milestones).Error
}

// List returns the open or closed milestones of the repository with given ID
// on given page, or all of them when page is 0.
func (s *MilestonesStore) List(ctx context.Context, repoID int64, isClosed bool, page int) ([]*Milestone, error) {
	milestones := make([]*Milestone, 0, conf.UI.IssuePagingNum)
	db := s.db.WithContext(ctx).Where("repo_id = ? AND is_closed = ?", repoID, isClosed).Order("id ASC")
	if page > 0 {
		db = db.Limit(conf.UI.IssuePagingNum).Offset((page - 1) * conf.UI.IssuePagingNum)
	}
	return milestones, db.Find(&milestones).Error
}

// Count returns the numbers of open and closed milestones of the repository
// with given ID.
func (s *MilestonesStore) Count(ctx context.Context, repoID int64) (open, closed int64, err error) {
	var counts []struct {
		IsClosed bool
		Count    int64
	}
	err = s.db.WithContext(ctx).Model(new(Milestone)).
		Select("is_closed, COUNT(*) AS count").
		Where("repo_id = ?", repoID).
		Group("is_closed").
		Scan(&counts).Error
	if err != nil {
		return 0, 0, err
	}
	for _, c := range counts {
		if c.IsClosed {
			closed = c.Count
		} else {
			open = c.Count
		}
	}
	return open, closed, nil
}

// CountIssues returns the number of open or closed issues of the milestone
// with given ID, and of its pull requests when includePulls is true.
func (s *MilestonesStore) CountIssues(ctx context.Context, milestoneID int64, isClosed, includePulls bool) (int64, error) {
	db := s.db.WithContext(ctx).Model(new(Issue)).Where("milestone_id = ? AND is_closed = ?", milestoneID, isClosed)
	if !includePulls {
		db = db.Where("is_pull = ?", false)
	}
	var count int64
	return count, db.Count(&count).Error
}

// Update saves the name, description and deadline of the milestone.
func (s *MilestonesStore) Update(ctx context.Context, m *Milestone) error {
	return s.update(ctx, m)
}

func (s *MilestonesStore) update(ctx context.Context, m *Milestone, columns ...string) error {
	columns = append([]string{"name", "content", "deadline_unix"}, columns...)
	return s.db.WithContext(ctx).Model(m).Select(columns).Updates(m).Error
}

// addToCounts adds to the numbers of issues and closed issues of the
// milestone, and computes its completeness again. It returns
// ErrMilestoneNotExist when the milestone is not in its repository, e.g.,
// because it was deleted, and on MySQL also when both numbers to add are 0.
// The database adds to the
// numbers it has rather than saving the ones of the milestone, so that changes
// made since the milestone was read are kept. It must run in a transaction,
// which keeps others from changing the numbers until completeness is saved.
func (s *MilestonesStore) addToCounts(ctx context.Context, m *Milestone, issues, closedIssues int) error {
	db := s.db.WithContext(ctx)
	result := db.Exec(
		"UPDATE milestone SET num_issues = num_issues + ?, num_closed_issues = num_closed_issues + ? WHERE repo_id = ? AND id = ?",
		issues, closedIssues, m.RepoID, m.ID,
	)
	if result.Error != nil {
		return errors.Wrap(result.Error, "add to counts")
	} else if result.RowsAffected == 0 {
		return ErrMilestoneNotExist{args: map[string]any{"repoID": m.RepoID, "milestoneID": m.ID}}
	}

	err := db.Select("num_issues", "num_closed_issues").Where("id = ?", m.ID).Take(m).Error
	if err != nil {
		return errors.Wrap(err, "get counts")
	}
	m.Completeness = 0
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	}
	return db.Model(m).UpdateColumn("completeness", m.Completeness).Error
}

// countInRepo updates the numbers of milestones of the repository with given
// ID.
func (s *MilestonesStore) countInRepo(ctx context.Context, repoID int64) error {
	return s.db.WithContext(ctx).Exec(`
UPDATE repository SET
	num_milestones = (SELECT COUNT(*) FROM milestone WHERE repo_id = ?),
	num_closed_milestones = (SELECT COUNT(*) FROM milestone WHERE repo_id = ? AND is_closed = ?)
WHERE id = ?`,
		repoID, repoID, true, repoID,
	).Error
}

// ChangeStatus saves what Update saves with the milestone closed or reopened,
// and updates the numbers of milestones of its repository. Closing an open
// milestone sets its closed date to now.
func (s *MilestonesStore) ChangeStatus(ctx context.Context, m *Milestone, isClosed bool) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if isClosed && !m.IsClosed {
			m.ClosedDate = tx.NowFunc()
		}
		m.IsClosed = isClosed

		store := newMilestonesStore(tx)
		err := store.update(ctx, m, "is_closed", "closed_date_unix")
		if err != nil {
			return errors.Wrap(err, "update milestone")
		}
		return store.countInRepo(ctx, m.RepoID)
	})
}

// DeleteByID deletes the milestone with given ID of the repository with given
// ID, and removes it from issues. Deleting a milestone that does not exist is
// not an error.
func (s *MilestonesStore) DeleteByID(ctx context.Context, repoID, id int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Closing and moving an issue lock it before its milestone, so the
		// issues are locked first, one by one by their IDs. Any other condition
		// may make MySQL scan and lock the gaps of an index, which creating an
		// issue in the repository waits for while it holds the repository. An
		// issue that a move commits after this keeps the deleted milestone,
		// which counts as no milestone.
		var issueIDs []int64
		err := tx.Model(new(Issue)).Where("repo_id = ? AND milestone_id = ?", repoID, id).Pluck("id", &issueIDs).Error
		if err != nil {
			return errors.Wrap(err, "list issues")
		}
		for _, issueID := range issueIDs {
			err = tx.Exec("UPDATE issue SET milestone_id = 0 WHERE id = ? AND milestone_id = ?", issueID, id).Error
			if err != nil {
				return errors.Wrap(err, "remove from issue")
			}
		}

		result := tx.Where("repo_id = ? AND id = ?", repoID, id).Delete(new(Milestone))
		if result.Error != nil {
			return errors.Wrap(result.Error, "delete milestone")
		} else if result.RowsAffected == 0 {
			return nil
		}

		err = newMilestonesStore(tx).countInRepo(ctx, repoID)
		if err != nil {
			return errors.Wrap(err, "count milestones")
		}
		// In chunks, because a statement can bind only so many parameters.
		for ids := range slices.Chunk(issueIDs, 500) {
			err = tx.Exec("UPDATE issue_user SET milestone_id = 0 WHERE issue_id IN ? AND milestone_id = ?", ids, id).Error
			if err != nil {
				return errors.Wrap(err, "remove from issue users")
			}
		}
		return nil
	})
}

// updateIssueStats moves the issue between the open and closed issues of its
// milestone after the issue has been closed or reopened.
func (s *MilestonesStore) updateIssueStats(ctx context.Context, issue *Issue) error {
	if issue.MilestoneID == 0 {
		return nil
	}

	closedIssues := -1
	if issue.IsClosed {
		closedIssues = 1
	}
	err := s.addToCounts(ctx, &Milestone{ID: issue.MilestoneID, RepoID: issue.RepoID}, 0, closedIssues)
	if IsErrMilestoneNotExist(err) {
		return nil // The milestone was deleted while the issue was moved to it.
	}
	return err
}

// assignIssue moves the issue from the milestone it has in the database, if
// any, to its milestone, if any.
func (s *MilestonesStore) assignIssue(ctx context.Context, issue *Issue) error {
	db := s.db.WithContext(ctx)

	// Closing an issue and deleting a milestone lock the issue before the
	// milestone, and so does this. Reading the issue once it is locked keeps a
	// move or a close that commits meanwhile.
	err := db.Exec("UPDATE issue SET milestone_id = milestone_id WHERE id = ?", issue.ID).Error
	if err != nil {
		return errors.Wrap(err, "lock issue")
	}
	var current struct {
		MilestoneID int64
		IsClosed    bool
	}
	err = db.Table("issue").Select("milestone_id", "is_closed").Where("id = ?", issue.ID).Take(&current).Error
	if err != nil {
		return errors.Wrap(err, "get issue")
	}
	issue.IsClosed = current.IsClosed

	var milestone *Milestone
	if issue.MilestoneID > 0 {
		milestone, err = s.GetByID(ctx, issue.RepoID, issue.MilestoneID)
		if err != nil {
			return err
		}
	}
	err = newIssuesStore(s.db).updateCols(ctx, issue, "milestone_id")
	if err != nil {
		return errors.Wrap(err, "update issue")
	}

	closedIssues := 0
	if issue.IsClosed {
		closedIssues = 1
	}
	counts := make(map[int64][2]int, 2)
	if current.MilestoneID > 0 {
		counts[current.MilestoneID] = [2]int{-1, -closedIssues}
	}
	if issue.MilestoneID > 0 {
		c := counts[issue.MilestoneID]
		counts[issue.MilestoneID] = [2]int{c[0] + 1, c[1] + closedIssues}
	}
	// Locking the milestones in the order of their IDs keeps a move the other
	// way from deadlocking with this one.
	for _, id := range slices.Sorted(maps.Keys(counts)) {
		if counts[id] == [2]int{} {
			continue
		}
		m := &Milestone{ID: id, RepoID: issue.RepoID}
		if id == issue.MilestoneID {
			m = milestone
		}
		err = s.addToCounts(ctx, m, counts[id][0], counts[id][1])
		if IsErrMilestoneNotExist(err) {
			if id == current.MilestoneID {
				continue // The milestone was deleted while the issue was moved to it.
			}
			return err
		} else if err != nil {
			return errors.Wrapf(err, "count issues of milestone %d", id)
		}
	}
	issue.Milestone = milestone

	return db.Exec("UPDATE issue_user SET milestone_id = ? WHERE issue_id = ?", issue.MilestoneID, issue.ID).Error
}

// AssignIssue moves the issue from the milestone it has in the database, if
// any, to its milestone, if any, as the doer.
func (s *MilestonesStore) AssignIssue(ctx context.Context, logger *logx.Logger, doer *User, issue *Issue) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return newMilestonesStore(tx).assignIssue(ctx, issue)
	})
	if err != nil {
		return err
	}

	hookAction := apiv1types.WebhookIssueDemilestoned
	if issue.MilestoneID > 0 {
		hookAction = apiv1types.WebhookIssueMilestoned
	}

	webhooks := newWebhooksStore(s.db)
	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = webhooks.Prepare(ctx, logger, issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      hookAction,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Repository:  issue.Repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = webhooks.Prepare(ctx, logger, issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     hookAction,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "error", err)
	}
	return nil
}
