package database

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/tool"
)

var ErrMissingIssueNumber = errors.New("no issue number specified")

// Issue represents an issue or pull request of repository.
type Issue struct {
	ID              int64       `gorm:"primaryKey"`
	RepoID          int64       `xorm:"INDEX UNIQUE(repo_index)" gorm:"index;uniqueIndex:issue_repo_index_unique;not null"`
	Repo            *Repository `xorm:"-" json:"-" gorm:"-"`
	Index           int64       `xorm:"UNIQUE(repo_index)" gorm:"uniqueIndex:issue_repo_index_unique;not null"` // Index in one repository.
	PosterID        int64       `gorm:"index"`
	Poster          *User       `xorm:"-" json:"-" gorm:"-"`
	Title           string      `xorm:"name" gorm:"column:name"`
	Content         string      `xorm:"TEXT" gorm:"type:TEXT"`
	RenderedContent string      `xorm:"-" json:"-" gorm:"-"`
	Labels          []*Label    `xorm:"-" json:"-" gorm:"-"`
	MilestoneID     int64       `gorm:"index"`
	Milestone       *Milestone  `xorm:"-" json:"-" gorm:"-"`
	Priority        int
	AssigneeID      int64 `gorm:"index"`
	Assignee        *User `xorm:"-" json:"-" gorm:"-"`
	IsClosed        bool
	IsRead          bool         `xorm:"-" json:"-" gorm:"-"`
	IsPull          bool         // Indicates whether is a pull request or not.
	PullRequest     *PullRequest `xorm:"-" json:"-" gorm:"-"`
	NumComments     int

	Deadline     time.Time `xorm:"-" json:"-" gorm:"-"`
	DeadlineUnix int64
	Created      time.Time `xorm:"-" json:"-" gorm:"-"`
	CreatedUnix  int64
	Updated      time.Time `xorm:"-" json:"-" gorm:"-"`
	UpdatedUnix  int64

	Attachments []*Attachment `xorm:"-" json:"-" gorm:"-"`
	Comments    []*Comment    `xorm:"-" json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (issue *Issue) BeforeCreate(tx *gorm.DB) error {
	if issue.CreatedUnix == 0 {
		issue.CreatedUnix = tx.NowFunc().Unix()
		issue.UpdatedUnix = issue.CreatedUnix
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (issue *Issue) BeforeUpdate(tx *gorm.DB) error {
	issue.UpdatedUnix = tx.NowFunc().Unix()
	issue.DeadlineUnix = issue.Deadline.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (issue *Issue) AfterFind(_ *gorm.DB) error {
	issue.Deadline = time.Unix(issue.DeadlineUnix, 0).Local()
	issue.Created = time.Unix(issue.CreatedUnix, 0).Local()
	issue.Updated = time.Unix(issue.UpdatedUnix, 0).Local()
	return nil
}

func (issue *Issue) loadAttributes(tx *gorm.DB) (err error) {
	if issue.Repo == nil {
		issue.Repo, err = getRepositoryByID(tx, issue.RepoID)
		if err != nil {
			return errors.Newf("getRepositoryByID [%d]: %v", issue.RepoID, err)
		}
	}

	if issue.Poster == nil {
		issue.Poster, err = newUsersStore(tx).GetByID(context.TODO(), issue.PosterID)
		if err != nil {
			if IsErrUserNotExist(err) {
				issue.PosterID = -1
				issue.Poster = NewGhostUser()
			} else {
				return errors.Newf("getUserByID.(Poster) [%d]: %v", issue.PosterID, err)
			}
		}
	}

	if issue.Labels == nil {
		issue.Labels, err = newLabelsStore(tx).ListByIssueID(context.TODO(), issue.ID)
		if err != nil {
			return errors.Wrap(err, "list labels")
		}
	}

	if issue.Milestone == nil && issue.MilestoneID > 0 {
		issue.Milestone, err = newMilestonesStore(tx).GetByID(context.TODO(), issue.RepoID, issue.MilestoneID)
		if err != nil {
			return errors.Wrapf(err, "get milestone %d", issue.MilestoneID)
		}
	}

	if issue.Assignee == nil && issue.AssigneeID > 0 {
		issue.Assignee, err = newUsersStore(tx).GetByID(context.TODO(), issue.AssigneeID)
		if err != nil {
			return errors.Newf("getUserByID.(assignee) [%d]: %v", issue.AssigneeID, err)
		}
	}

	if issue.IsPull && issue.PullRequest == nil {
		// It is possible pull request is not yet created.
		issue.PullRequest, err = getPullRequestByIssueID(tx, issue.ID)
		if err != nil && !IsErrPullRequestNotExist(err) {
			return errors.Newf("getPullRequestByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Attachments == nil {
		issue.Attachments, err = newAttachmentsStore(tx).ListByIssueID(context.TODO(), issue.ID)
		if err != nil {
			return errors.Wrap(err, "list attachments")
		}
	}

	if issue.Comments == nil {
		issue.Comments, err = getCommentsByIssueID(tx, issue.ID)
		if err != nil {
			return errors.Newf("getCommentsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	return nil
}

func (issue *Issue) LoadAttributes() error {
	return issue.loadAttributes(Handle.db)
}

func (issue *Issue) HTMLURL() string {
	var path string
	if issue.IsPull {
		path = "pulls"
	} else {
		path = "issues"
	}
	return fmt.Sprintf("%s/%s/%d", issue.Repo.HTMLURL(), path, issue.Index)
}

// State returns string representation of issue status.
func (issue *Issue) State() apiv1types.IssueStateType {
	if issue.IsClosed {
		return apiv1types.IssueStateClosed
	}
	return apiv1types.IssueStateOpen
}

// This method assumes some fields assigned with values:
// Required - Poster, Labels,
// Optional - Milestone, Assignee, PullRequest
func (issue *Issue) APIFormat() *apiv1types.Issue {
	apiLabels := make([]*apiv1types.IssueLabel, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}

	apiIssue := &apiv1types.Issue{
		ID:       issue.ID,
		Index:    issue.Index,
		Poster:   issue.Poster.APIFormat(),
		Title:    issue.Title,
		Body:     issue.Content,
		Labels:   apiLabels,
		State:    issue.State(),
		Comments: issue.NumComments,
		Created:  issue.Created,
		Updated:  issue.Updated,
	}

	if issue.Milestone != nil {
		apiIssue.Milestone = issue.Milestone.APIFormat()
	}
	if issue.Assignee != nil {
		apiIssue.Assignee = issue.Assignee.APIFormat()
	}
	if issue.IsPull {
		apiIssue.PullRequest = &apiv1types.PullRequestMeta{
			HasMerged: issue.PullRequest.HasMerged,
		}
		if issue.PullRequest.HasMerged {
			apiIssue.PullRequest.Merged = &issue.PullRequest.Merged
		}
	}

	return apiIssue
}

// HashTag returns unique hash tag for issue.
func (issue *Issue) HashTag() string {
	return "issue-" + strconv.FormatInt(issue.ID, 10)
}

// IsPoster returns true if given user by ID is the poster.
func (issue *Issue) IsPoster(uid int64) bool {
	return issue.PosterID == uid
}

func (issue *Issue) sendLabelUpdatedWebhook(logger *logx.Logger, doer *User) {
	var err error
	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			logger.Error("Failed to load issue", "error", err)
			return
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueLabelUpdated,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Repository:  issue.Repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     apiv1types.WebhookIssueLabelUpdated,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "error", err)
	}
}

// AddLabel adds the label to the issue.
func (issue *Issue) AddLabel(logger *logx.Logger, doer *User, label *Label) error {
	return issue.AddLabels(logger, doer, []*Label{label})
}

func (issue *Issue) addLabels(tx *gorm.DB, labels []*Label) error {
	return newLabelsStore(tx).addToIssue(context.TODO(), issue, labels...)
}

// AddLabels adds a list of new labels to the issue.
func (issue *Issue) AddLabels(logger *logx.Logger, doer *User, labels []*Label) error {
	if err := Handle.Labels().AddToIssue(context.TODO(), issue, labels...); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(logger, doer)
	return nil
}

func (issue *Issue) getLabels(tx *gorm.DB) (err error) {
	if len(issue.Labels) > 0 {
		return nil
	}

	issue.Labels, err = newLabelsStore(tx).ListByIssueID(context.TODO(), issue.ID)
	if err != nil {
		return errors.Wrap(err, "list labels")
	}
	return nil
}

// RemoveLabel removes a label from issue by given ID.
func (issue *Issue) RemoveLabel(logger *logx.Logger, doer *User, label *Label) error {
	if err := Handle.Labels().RemoveFromIssue(context.TODO(), issue, label); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(logger, doer)
	return nil
}

func (issue *Issue) clearLabels(tx *gorm.DB) (err error) {
	if err = issue.getLabels(tx); err != nil {
		return errors.Newf("getLabels: %v", err)
	}

	// NOTE: Removing a label deletes it from issue.Labels in place, so iterate over a copy.
	labels := make([]*Label, len(issue.Labels))
	copy(labels, issue.Labels)
	for i := range labels {
		if err = newLabelsStore(tx).removeFromIssue(context.TODO(), issue, labels[i]); err != nil {
			return errors.Wrapf(err, "remove label %d", labels[i].ID)
		}
	}

	return nil
}

func (issue *Issue) ClearLabels(logger *logx.Logger, doer *User) (err error) {
	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		return issue.clearLabels(tx)
	})
	if err != nil {
		return err
	}

	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			logger.Error("Failed to load issue", "error", err)
			return err
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueLabelCleared,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Repository:  issue.Repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     apiv1types.WebhookIssueLabelCleared,
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

// ReplaceLabels removes all current labels and add new labels to the issue.
func (issue *Issue) ReplaceLabels(labels []*Label) (err error) {
	return Handle.db.Transaction(func(tx *gorm.DB) error {
		if err := issue.clearLabels(tx); err != nil {
			return errors.Newf("clearLabels: %v", err)
		} else if err = issue.addLabels(tx, labels); err != nil {
			return errors.Newf("addLabels: %v", err)
		}
		return nil
	})
}

func (issue *Issue) GetAssignee() (err error) {
	if issue.AssigneeID == 0 || issue.Assignee != nil {
		return nil
	}

	issue.Assignee, err = Handle.Users().GetByID(context.TODO(), issue.AssigneeID)
	if IsErrUserNotExist(err) {
		return nil
	}
	return err
}

// ReadBy sets issue to be read by given user.
func (issue *Issue) ReadBy(uid int64) error {
	return UpdateIssueUserByRead(uid, issue.ID)
}

func updateIssueCols(tx *gorm.DB, issue *Issue, cols ...string) error {
	cols = append(cols, "updated_unix")
	return tx.Model(issue).Where("id = ?", issue.ID).Select(cols).Updates(issue).Error
}

// UpdateIssueCols only updates values of specific columns for given issue.
func UpdateIssueCols(issue *Issue, cols ...string) error {
	return updateIssueCols(Handle.db, issue, cols...)
}

func (issue *Issue) changeStatus(logger *logx.Logger, tx *gorm.DB, doer *User, repo *Repository, isClosed bool) (err error) {
	// Nothing should be performed if current status is same as target status
	if issue.IsClosed == isClosed {
		return nil
	}
	issue.IsClosed = isClosed

	if err = updateIssueCols(tx, issue, "is_closed"); err != nil {
		return err
	} else if err = updateIssueUsersByStatus(tx, issue.ID, isClosed); err != nil {
		return err
	}

	// Update issue count of labels
	if err = issue.getLabels(tx); err != nil {
		return err
	}
	for idx := range issue.Labels {
		if issue.IsClosed {
			issue.Labels[idx].NumClosedIssues++
		} else {
			issue.Labels[idx].NumClosedIssues--
		}
		if err = newLabelsStore(tx).Update(context.TODO(), issue.Labels[idx]); err != nil {
			return err
		}
	}

	// Update issue count of milestone
	if err = newMilestonesStore(tx).updateIssueStats(context.TODO(), issue); err != nil {
		return err
	}

	// New action comment
	if _, err = createStatusComment(logger, tx, doer, repo, issue); err != nil {
		return err
	}

	return nil
}

// ChangeStatus changes issue status to open or closed.
func (issue *Issue) ChangeStatus(logger *logx.Logger, doer *User, repo *Repository, isClosed bool) (err error) {
	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		return issue.changeStatus(logger, tx, doer, repo, isClosed)
	})
	if err != nil {
		return err
	}

	if issue.IsPull {
		// Merge pull request calls issue.changeStatus so we need to handle separately.
		issue.PullRequest.Issue = issue
		apiPullRequest := &apiv1types.WebhookPullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Repository:  repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		}
		if isClosed {
			apiPullRequest.Action = apiv1types.WebhookIssueClosed
		} else {
			apiPullRequest.Action = apiv1types.WebhookIssueReopened
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &apiv1types.WebhookIssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		}
		if isClosed {
			apiIssues.Action = apiv1types.WebhookIssueClosed
		} else {
			apiIssues.Action = apiv1types.WebhookIssueReopened
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, repo, HookEventTypeIssues, apiIssues)
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "isClosed", isClosed, "error", err)
	}

	return nil
}

func (issue *Issue) ChangeTitle(logger *logx.Logger, doer *User, title string) (err error) {
	oldTitle := issue.Title
	issue.Title = title
	if err = UpdateIssueCols(issue, "name"); err != nil {
		return errors.Newf("UpdateIssueCols: %v", err)
	}

	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueEdited,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Changes: &apiv1types.WebhookChangesPayload{
				Title: &apiv1types.WebhookChangesFromPayload{
					From: oldTitle,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action: apiv1types.WebhookIssueEdited,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Title: &apiv1types.WebhookChangesFromPayload{
					From: oldTitle,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "error", err)
	}

	return nil
}

func (issue *Issue) ChangeContent(logger *logx.Logger, doer *User, content string) (err error) {
	oldContent := issue.Content
	issue.Content = content
	if err = UpdateIssueCols(issue, "content"); err != nil {
		return errors.Newf("UpdateIssueCols: %v", err)
	}

	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueEdited,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Changes: &apiv1types.WebhookChangesPayload{
				Body: &apiv1types.WebhookChangesFromPayload{
					From: oldContent,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action: apiv1types.WebhookIssueEdited,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Body: &apiv1types.WebhookChangesFromPayload{
					From: oldContent,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "error", err)
	}

	return nil
}

func (issue *Issue) ChangeAssignee(logger *logx.Logger, doer *User, assigneeID int64) (err error) {
	issue.AssigneeID = assigneeID
	if err = UpdateIssueUserByAssignee(issue); err != nil {
		return errors.Newf("UpdateIssueUserByAssignee: %v", err)
	}

	issue.Assignee, err = Handle.Users().GetByID(context.TODO(), issue.AssigneeID)
	if err != nil && !IsErrUserNotExist(err) {
		logger.Error("Failed to get user by ID", "error", err)
		return nil
	}

	// Error not nil here means user does not exist, which is remove assignee.
	isRemoveAssignee := err != nil
	if issue.IsPull {
		issue.PullRequest.Issue = issue
		apiPullRequest := &apiv1types.WebhookPullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(logger),
			Repository:  issue.Repo.APIFormatLegacy(logger, nil),
			Sender:      doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiPullRequest.Action = apiv1types.WebhookIssueUnassigned
		} else {
			apiPullRequest.Action = apiv1types.WebhookIssueAssigned
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &apiv1types.WebhookIssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(logger, nil),
			Sender:     doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiIssues.Action = apiv1types.WebhookIssueUnassigned
		} else {
			apiIssues.Action = apiv1types.WebhookIssueAssigned
		}
		err = Handle.Webhooks().Prepare(context.TODO(), logger, issue.Repo, HookEventTypeIssues, apiIssues)
	}
	if err != nil {
		logger.Error("Failed to prepare webhooks", "isPull", issue.IsPull, "removeAssignee", isRemoveAssignee, "error", err)
	}

	return nil
}

type NewIssueOptions struct {
	Repo        *Repository
	Issue       *Issue
	LableIDs    []int64
	Attachments []string // In UUID format.
	IsPull      bool
}

func newIssue(tx *gorm.DB, opts NewIssueOptions) (err error) {
	opts.Issue.Title = strings.TrimSpace(opts.Issue.Title)
	opts.Issue.Index = opts.Repo.NextIssueIndex()

	if opts.Issue.MilestoneID > 0 {
		milestones := newMilestonesStore(tx)
		milestone, err := milestones.GetByID(context.TODO(), opts.Issue.RepoID, opts.Issue.MilestoneID)
		if err != nil && !IsErrMilestoneNotExist(err) {
			return errors.Wrap(err, "get milestone")
		}

		// Assume milestone is invalid and drop silently.
		opts.Issue.MilestoneID = 0
		if milestone != nil {
			opts.Issue.MilestoneID = milestone.ID
			opts.Issue.Milestone = milestone
			if err = milestones.assignIssue(context.TODO(), opts.Issue, -1); err != nil {
				return err
			}
		}
	}

	if opts.Issue.AssigneeID > 0 {
		assignee, err := newUsersStore(tx).GetByID(context.TODO(), opts.Issue.AssigneeID)
		if err != nil && !IsErrUserNotExist(err) {
			return errors.Newf("get user by ID: %v", err)
		}

		if assignee != nil {
			opts.Issue.AssigneeID = assignee.ID
			opts.Issue.Assignee = assignee
		} else {
			// The assignee does not exist, drop it
			opts.Issue.AssigneeID = 0
		}
	}

	// Milestone and assignee validation should happen before insert actual object.
	if err = tx.Create(opts.Issue).Error; err != nil {
		return err
	}

	if opts.IsPull {
		err = tx.Exec("UPDATE repository SET num_pulls = num_pulls + 1 WHERE id = ?", opts.Issue.RepoID).Error
	} else {
		err = tx.Exec("UPDATE repository SET num_issues = num_issues + 1 WHERE id = ?", opts.Issue.RepoID).Error
	}
	if err != nil {
		return err
	}

	if len(opts.LableIDs) > 0 {
		// During the session, SQLite3 driver cannot handle retrieve objects after update something.
		// So we have to get all needed labels first.
		labels := make([]*Label, 0, len(opts.LableIDs))
		if err = tx.Where("id IN ?", opts.LableIDs).Find(&labels).Error; err != nil {
			return errors.Newf("find all labels [label_ids: %v]: %v", opts.LableIDs, err)
		}

		for _, label := range labels {
			// Silently drop invalid labels.
			if label.RepoID != opts.Repo.ID {
				continue
			}

			if err = newLabelsStore(tx).addToIssue(context.TODO(), opts.Issue, label); err != nil {
				return errors.Wrapf(err, "add label %d", label.ID)
			}
		}
	}

	if err = newIssueUsers(tx, opts.Repo, opts.Issue); err != nil {
		return err
	}

	err = newAttachmentsStore(tx).Bind(context.TODO(), opts.Issue.ID, 0, opts.Attachments)
	if err != nil {
		return errors.Wrap(err, "attach files")
	}

	return opts.Issue.loadAttributes(tx)
}

// NewIssue creates new issue with labels and attachments for repository.
func NewIssue(logger *logx.Logger, repo *Repository, issue *Issue, labelIDs []int64, uuids []string) (err error) {
	err = Handle.db.Transaction(func(tx *gorm.DB) error {
		return newIssue(tx, NewIssueOptions{
			Repo:        repo,
			Issue:       issue,
			LableIDs:    labelIDs,
			Attachments: uuids,
		})
	})
	if err != nil {
		return errors.Newf("new issue: %v", err)
	}

	if err = NotifyWatchers(&Action{
		ActUserID:    issue.Poster.ID,
		ActUserName:  issue.Poster.Name,
		OpType:       ActionCreateIssue,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Title),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}); err != nil {
		logger.Error("Failed to notify watchers", "error", err)
	}
	if err = issue.MailParticipants(logger); err != nil {
		logger.Error("Failed to mail participants", "error", err)
	}

	if err = Handle.Webhooks().Prepare(context.TODO(), logger, repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
		Action:     apiv1types.WebhookIssueOpened,
		Index:      issue.Index,
		Issue:      issue.APIFormat(),
		Repository: repo.APIFormatLegacy(logger, nil),
		Sender:     issue.Poster.APIFormat(),
	}); err != nil {
		logger.Error("Failed to prepare webhooks", "error", err)
	}

	return nil
}

var _ errx.NotFound = (*ErrIssueNotExist)(nil)

type ErrIssueNotExist struct {
	args map[string]any
}

func IsErrIssueNotExist(err error) bool {
	_, ok := err.(ErrIssueNotExist)
	return ok
}

func (err ErrIssueNotExist) Error() string {
	return fmt.Sprintf("issue does not exist: %v", err.args)
}

func (ErrIssueNotExist) NotFound() bool {
	return true
}

// GetIssueByRef returns an Issue specified by a GFM reference, e.g. owner/repo#123.
func GetIssueByRef(ref string) (*Issue, error) {
	before, after, ok := strings.Cut(ref, "#")
	if !ok {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	index, _ := strconv.ParseInt(after, 10, 64)
	if index == 0 {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	repo, err := GetRepositoryByRef(before)
	if err != nil {
		return nil, err
	}

	issue, err := GetIssueByIndex(repo.ID, index)
	if err != nil {
		return nil, err
	}

	return issue, issue.LoadAttributes()
}

// GetRawIssueByIndex returns raw issue without loading attributes by index in a repository.
func GetRawIssueByIndex(repoID, index int64) (*Issue, error) {
	issue := new(Issue)
	err := Handle.db.Where(&Issue{RepoID: repoID, Index: index}).First(issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIssueNotExist{args: map[string]any{"repoID": repoID, "index": index}}
		}
		return nil, err
	}
	return issue, nil
}

// GetIssueByIndex returns issue by index in a repository.
func GetIssueByIndex(repoID, index int64) (*Issue, error) {
	issue, err := GetRawIssueByIndex(repoID, index)
	if err != nil {
		return nil, err
	}
	return issue, issue.LoadAttributes()
}

func getRawIssueByID(tx *gorm.DB, id int64) (*Issue, error) {
	issue := new(Issue)
	err := tx.Where("id = ?", id).First(issue).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIssueNotExist{args: map[string]any{"issueID": id}}
		}
		return nil, err
	}
	return issue, nil
}

func getIssueByID(tx *gorm.DB, id int64) (*Issue, error) {
	issue, err := getRawIssueByID(tx, id)
	if err != nil {
		return nil, err
	}
	return issue, issue.loadAttributes(tx)
}

// GetIssueByID returns an issue by given ID.
func GetIssueByID(id int64) (*Issue, error) {
	return getIssueByID(Handle.db, id)
}

type IssuesOptions struct {
	UserID      int64
	AssigneeID  int64
	RepoID      int64
	PosterID    int64
	MilestoneID int64
	RepoIDs     []int64
	Page        int
	IsClosed    bool
	IsMention   bool
	IsPull      bool
	Labels      string
	SortType    string
}

// buildIssuesQuery returns nil if it foresees there won't be any value returned.
func buildIssuesQuery(opts *IssuesOptions) *gorm.DB {
	query := Handle.db.Model(new(Issue))

	if opts.Page <= 0 {
		opts.Page = 1
	}

	if opts.RepoID > 0 {
		query = query.Where("issue.repo_id = ? AND issue.is_closed = ?", opts.RepoID, opts.IsClosed)
	} else if opts.RepoIDs != nil {
		// In case repository IDs are provided but actually no repository has issue.
		if len(opts.RepoIDs) == 0 {
			return nil
		}
		query = query.Where("issue.repo_id IN ? AND issue.is_closed = ?", opts.RepoIDs, opts.IsClosed)
	} else {
		query = query.Where("issue.is_closed = ?", opts.IsClosed)
	}

	if opts.AssigneeID > 0 {
		query = query.Where("issue.assignee_id = ?", opts.AssigneeID)
	} else if opts.PosterID > 0 {
		query = query.Where("issue.poster_id = ?", opts.PosterID)
	}

	if opts.MilestoneID > 0 {
		query = query.Where("issue.milestone_id = ?", opts.MilestoneID)
	}

	query = query.Where("issue.is_pull = ?", opts.IsPull)

	switch opts.SortType {
	case "oldest":
		query = query.Order("issue.created_unix ASC")
	case "recentupdate":
		query = query.Order("issue.updated_unix DESC")
	case "leastupdate":
		query = query.Order("issue.updated_unix ASC")
	case "mostcomment":
		query = query.Order("issue.num_comments DESC")
	case "leastcomment":
		query = query.Order("issue.num_comments ASC")
	case "priority":
		query = query.Order("issue.priority DESC")
	default:
		query = query.Order("issue.created_unix DESC")
	}

	if len(opts.Labels) > 0 && opts.Labels != "0" {
		labelIDs := strings.Split(opts.Labels, ",")
		if len(labelIDs) > 0 {
			query = query.Joins("INNER JOIN issue_label ON issue.id = issue_label.issue_id").Where("issue_label.label_id IN ?", labelIDs)
		}
	}

	if opts.IsMention {
		query = query.Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").Where("issue_user.is_mentioned = ?", true)

		if opts.UserID > 0 {
			query = query.Where("issue_user.uid = ?", opts.UserID)
		}
	}

	return query
}

// IssuesCount returns the number of issues by given conditions.
func IssuesCount(opts *IssuesOptions) (int64, error) {
	query := buildIssuesQuery(opts)
	if query == nil {
		return 0, nil
	}

	var count int64
	return count, query.Count(&count).Error
}

// Issues returns a list of issues by given conditions.
func Issues(opts *IssuesOptions) ([]*Issue, error) {
	query := buildIssuesQuery(opts)
	if query == nil {
		return make([]*Issue, 0), nil
	}

	issues := make([]*Issue, 0, conf.UI.IssuePagingNum)
	err := query.Select("issue.*").
		Limit(conf.UI.IssuePagingNum).
		Offset((opts.Page - 1) * conf.UI.IssuePagingNum).
		Find(&issues).Error
	if err != nil {
		return nil, errors.Newf("find: %v", err)
	}

	// FIXME: use IssueList to improve performance.
	for i := range issues {
		if err := issues[i].LoadAttributes(); err != nil {
			return nil, errors.Newf("LoadAttributes [%d]: %v", issues[i].ID, err)
		}
	}

	return issues, nil
}

// SuggestIssuesOptions is the options for SuggestIssues.
type SuggestIssuesOptions struct {
	RepoID int64
	// Keyword matches the issues whose number starts with it or whose title
	// contains it, ignoring case. An empty keyword matches every issue.
	Keyword       string
	IncludeIssues bool
	IncludePulls  bool
	Limit         int
}

// SuggestIssues returns the issues and pull requests to suggest when
// referencing one, without loading their attributes. The issue whose number is
// the keyword comes first, then the most recently updated ones.
func SuggestIssues(opts SuggestIssuesOptions) ([]*Issue, error) {
	if !opts.IncludeIssues && !opts.IncludePulls {
		return []*Issue{}, nil
	}

	tx := Handle.db.Where("repo_id = ?", opts.RepoID)
	if opts.IncludeIssues != opts.IncludePulls {
		tx = tx.Where("is_pull = ?", opts.IncludePulls)
	}

	keyword := strings.TrimSpace(opts.Keyword)
	if keyword != "" {
		conds := []string{"LOWER(name) LIKE ?"}
		args := []any{"%" + strings.ToLower(keyword) + "%"}
		if index, err := strconv.ParseInt(keyword, 10, 64); err == nil && index > 0 {
			// Numbers starting with n are in [n, n+1), [10n, 10n+10) and so on.
			indexCol := tx.Statement.Quote("index")
			for lo, hi := index, index+1; ; lo, hi = lo*10, hi*10 {
				conds = append(conds, fmt.Sprintf("(%[1]s >= ? AND %[1]s < ?)", indexCol))
				args = append(args, lo, hi)
				if lo > math.MaxInt64/10 || hi > math.MaxInt64/10 {
					break
				}
			}
			tx = tx.Order(fmt.Sprintf("CASE WHEN %s = %d THEN 0 ELSE 1 END", indexCol, index))
		}
		tx = tx.Where("("+strings.Join(conds, " OR ")+")", args...)
	}

	issues := make([]*Issue, 0, opts.Limit)
	if err := tx.Order("updated_unix DESC").Limit(opts.Limit).Find(&issues).Error; err != nil {
		return nil, errors.Wrap(err, "find")
	}
	return issues, nil
}

// GetParticipantsByIssueID returns all users who are participated in comments of an issue.
func GetParticipantsByIssueID(tx *gorm.DB, issueID int64) ([]*User, error) {
	userIDs := make([]int64, 0, 5)
	if err := tx.Model(new(Comment)).
		Where("issue_id = ?", issueID).
		Distinct().
		Pluck("poster_id", &userIDs).Error; err != nil {
		return nil, errors.Newf("get poster IDs: %v", err)
	}
	if len(userIDs) == 0 {
		return nil, nil
	}

	users := make([]*User, 0, len(userIDs))
	return users, tx.Where("id IN ?", userIDs).Find(&users).Error
}

// .___                             ____ ___
// |   | ______ ________ __   ____ |    |   \______ ___________
// |   |/  ___//  ___/  |  \_/ __ \|    |   /  ___// __ \_  __ \
// |   |\___ \ \___ \|  |  /\  ___/|    |  /\___ \\  ___/|  | \/
// |___/____  >____  >____/  \___  >______//____  >\___  >__|
//          \/     \/            \/             \/     \/

// IssueUser represents an issue-user relation.
type IssueUser struct {
	ID          int64 `gorm:"primary_key"`
	UserID      int64 `xorm:"uid INDEX" gorm:"column:uid;index"`
	IssueID     int64
	RepoID      int64 `xorm:"INDEX" gorm:"index"`
	MilestoneID int64
	IsRead      bool
	IsAssigned  bool
	IsMentioned bool
	IsPoster    bool
	IsClosed    bool
}

func newIssueUsers(tx *gorm.DB, repo *Repository, issue *Issue) error {
	assignees, err := repo.getAssignees(tx)
	if err != nil {
		return errors.Newf("getAssignees: %v", err)
	}

	// Poster can be anyone, append later if not one of assignees.
	isPosterAssignee := false

	// Leave a seat for poster itself to append later, but if poster is one of assignee
	// and just waste 1 unit is cheaper than re-allocate memory once.
	issueUsers := make([]*IssueUser, 0, len(assignees)+1)
	for _, assignee := range assignees {
		isPoster := assignee.ID == issue.PosterID
		issueUsers = append(issueUsers, &IssueUser{
			IssueID:    issue.ID,
			RepoID:     repo.ID,
			UserID:     assignee.ID,
			IsPoster:   isPoster,
			IsAssigned: assignee.ID == issue.AssigneeID,
		})
		if !isPosterAssignee && isPoster {
			isPosterAssignee = true
		}
	}
	if !isPosterAssignee {
		issueUsers = append(issueUsers, &IssueUser{
			IssueID:  issue.ID,
			RepoID:   repo.ID,
			UserID:   issue.PosterID,
			IsPoster: true,
		})
	}

	return tx.Create(issueUsers).Error
}

// NewIssueUsers adds new issue-user relations for new issue of repository.
func NewIssueUsers(repo *Repository, issue *Issue) (err error) {
	return Handle.db.Transaction(func(tx *gorm.DB) error {
		return newIssueUsers(tx, repo, issue)
	})
}

// PairsContains returns true when pairs list contains given issue.
func PairsContains(ius []*IssueUser, issueID, uid int64) int {
	for i := range ius {
		if ius[i].IssueID == issueID &&
			ius[i].UserID == uid {
			return i
		}
	}
	return -1
}

// GetIssueUsers returns issue-user pairs by given repository and user.
func GetIssueUsers(rid, uid int64, isClosed bool) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	err := Handle.db.Where("is_closed = ?", isClosed).Where(&IssueUser{RepoID: rid, UserID: uid}).Find(&ius).Error
	return ius, err
}

// GetIssueUserPairsByRepoIds returns issue-user pairs by given repository IDs.
func GetIssueUserPairsByRepoIds(rids []int64, isClosed bool, page int) ([]*IssueUser, error) {
	if len(rids) == 0 {
		return []*IssueUser{}, nil
	}

	ius := make([]*IssueUser, 0, 10)
	err := Handle.db.Where("is_closed = ? AND repo_id IN ?", isClosed, rids).
		Limit(20).Offset((page - 1) * 20).
		Find(&ius).Error
	return ius, err
}

// GetIssueUserPairsByMode returns issue-user pairs by given repository and user.
func GetIssueUserPairsByMode(userID, repoID int64, filterMode FilterMode, isClosed bool, page int) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	query := Handle.db.Where("uid = ? AND is_closed = ?", userID, isClosed).Limit(20).Offset((page - 1) * 20)
	if repoID > 0 {
		query = query.Where("repo_id = ?", repoID)
	}

	switch filterMode {
	case FilterModeAssign:
		query = query.Where("is_assigned = ?", true)
	case FilterModeCreate:
		query = query.Where("is_poster = ?", true)
	default:
		return ius, nil
	}
	err := query.Find(&ius).Error
	return ius, err
}

// updateIssueMentions extracts mentioned people from content and
// updates issue-user relations for them.
func updateIssueMentions(tx *gorm.DB, issueID int64, mentions []string) error {
	if len(mentions) == 0 {
		return nil
	}

	for i := range mentions {
		mentions[i] = strings.ToLower(mentions[i])
	}
	users := make([]*User, 0, len(mentions))

	if err := tx.Where("lower_name IN ?", mentions).Order("lower_name ASC").Find(&users).Error; err != nil {
		return errors.Newf("find mentioned users: %v", err)
	}

	ids := make([]int64, 0, len(mentions))
	for _, user := range users {
		ids = append(ids, user.ID)
		if !user.IsOrganization() || user.NumMembers == 0 {
			continue
		}

		memberIDs := make([]int64, 0, user.NumMembers)
		orgUsers, err := getOrgUsersByOrgID(tx, user.ID, 0)
		if err != nil {
			return errors.Newf("getOrgUsersByOrgID [%d]: %v", user.ID, err)
		}

		for _, orgUser := range orgUsers {
			memberIDs = append(memberIDs, orgUser.ID)
		}

		ids = append(ids, memberIDs...)
	}

	if err := updateIssueUsersByMentions(tx, issueID, ids); err != nil {
		return errors.Newf("UpdateIssueUsersByMentions: %v", err)
	}

	return nil
}

// IssueStats represents issue statistic information.
type IssueStats struct {
	OpenCount, ClosedCount int64
	YourReposCount         int64
	AssignCount            int64
	CreateCount            int64
	MentionCount           int64
}

type FilterMode string

const (
	FilterModeYourRepos FilterMode = "your_repositories"
	FilterModeAssign    FilterMode = "assigned"
	FilterModeCreate    FilterMode = "created_by"
	FilterModeMention   FilterMode = "mentioned"
)

func parseCountResult(results []map[string][]byte) int64 {
	if len(results) == 0 {
		return 0
	}
	for _, result := range results[0] {
		count, _ := strconv.ParseInt(string(result), 10, 64)
		return count
	}
	return 0
}

type IssueStatsOptions struct {
	RepoID      int64
	UserID      int64
	Labels      string
	MilestoneID int64
	AssigneeID  int64
	FilterMode  FilterMode
	IsPull      bool
}

// GetIssueStats returns issue statistic information by given conditions.
func GetIssueStats(opts *IssueStatsOptions) *IssueStats {
	stats := &IssueStats{}

	countSession := func(opts *IssueStatsOptions) *gorm.DB {
		query := Handle.db.Model(new(Issue)).Where("issue.repo_id = ? AND is_pull = ?", opts.RepoID, opts.IsPull)

		if len(opts.Labels) > 0 && opts.Labels != "0" {
			labelIDs := tool.StringsToInt64s(strings.Split(opts.Labels, ","))
			if len(labelIDs) > 0 {
				query = query.Joins("INNER JOIN issue_label ON issue.id = issue_id").Where("label_id IN ?", labelIDs)
			}
		}

		if opts.MilestoneID > 0 {
			query = query.Where("issue.milestone_id = ?", opts.MilestoneID)
		}

		if opts.AssigneeID > 0 {
			query = query.Where("assignee_id = ?", opts.AssigneeID)
		}

		return query
	}
	count := func(query *gorm.DB) int64 {
		var n int64
		_ = query.Count(&n).Error
		return n
	}

	switch opts.FilterMode {
	case FilterModeYourRepos, FilterModeAssign:
		stats.OpenCount = count(countSession(opts).
			Where("is_closed = ?", false))

		stats.ClosedCount = count(countSession(opts).
			Where("is_closed = ?", true))
	case FilterModeCreate:
		stats.OpenCount = count(countSession(opts).
			Where("poster_id = ?", opts.UserID).
			Where("is_closed = ?", false))

		stats.ClosedCount = count(countSession(opts).
			Where("poster_id = ?", opts.UserID).
			Where("is_closed = ?", true))
	case FilterModeMention:
		stats.OpenCount = count(countSession(opts).
			Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").
			Where("issue_user.uid = ?", opts.UserID).
			Where("issue_user.is_mentioned = ?", true).
			Where("issue.is_closed = ?", false))

		stats.ClosedCount = count(countSession(opts).
			Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").
			Where("issue_user.uid = ?", opts.UserID).
			Where("issue_user.is_mentioned = ?", true).
			Where("issue.is_closed = ?", true))
	}
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(repoID, userID int64, repoIDs []int64, filterMode FilterMode, isPull bool) *IssueStats {
	stats := &IssueStats{}
	hasAnyRepo := repoID > 0 || len(repoIDs) > 0
	countSession := func(isClosed, isPull bool, repoID int64, repoIDs []int64) *gorm.DB {
		query := Handle.db.Model(new(Issue)).Where("issue.is_closed = ? AND issue.is_pull = ?", isClosed, isPull)

		if repoID > 0 {
			query = query.Where("repo_id = ?", repoID)
		} else if len(repoIDs) > 0 {
			query = query.Where("repo_id IN ?", repoIDs)
		}

		return query
	}
	count := func(query *gorm.DB) int64 {
		var n int64
		_ = query.Count(&n).Error
		return n
	}

	stats.AssignCount = count(countSession(false, isPull, repoID, nil).
		Where("assignee_id = ?", userID))

	stats.CreateCount = count(countSession(false, isPull, repoID, nil).
		Where("poster_id = ?", userID))

	if hasAnyRepo {
		stats.YourReposCount = count(countSession(false, isPull, repoID, repoIDs))
	}

	switch filterMode {
	case FilterModeYourRepos:
		if !hasAnyRepo {
			break
		}

		stats.OpenCount = count(countSession(false, isPull, repoID, repoIDs))
		stats.ClosedCount = count(countSession(true, isPull, repoID, repoIDs))
	case FilterModeAssign:
		stats.OpenCount = count(countSession(false, isPull, repoID, nil).
			Where("assignee_id = ?", userID))
		stats.ClosedCount = count(countSession(true, isPull, repoID, nil).
			Where("assignee_id = ?", userID))
	case FilterModeCreate:
		stats.OpenCount = count(countSession(false, isPull, repoID, nil).
			Where("poster_id = ?", userID))
		stats.ClosedCount = count(countSession(true, isPull, repoID, nil).
			Where("poster_id = ?", userID))
	}

	return stats
}

// GetRepoIssueStats returns number of open and closed repository issues by given filter mode.
func GetRepoIssueStats(repoID, userID int64, filterMode FilterMode, isPull bool) (numOpen, numClosed int64) {
	countSession := func(isClosed, isPull bool, repoID int64) *gorm.DB {
		return Handle.db.Model(new(Issue)).
			Where("issue.repo_id = ?", isClosed).
			Where("is_pull = ?", isPull).
			Where("repo_id = ?", repoID)
	}

	openCountSession := countSession(false, isPull, repoID)
	closedCountSession := countSession(true, isPull, repoID)

	switch filterMode {
	case FilterModeAssign:
		openCountSession = openCountSession.Where("assignee_id = ?", userID)
		closedCountSession = closedCountSession.Where("assignee_id = ?", userID)
	case FilterModeCreate:
		openCountSession = openCountSession.Where("poster_id = ?", userID)
		closedCountSession = closedCountSession.Where("poster_id = ?", userID)
	}

	_ = openCountSession.Count(&numOpen).Error
	_ = closedCountSession.Count(&numClosed).Error
	return numOpen, numClosed
}

func updateIssue(tx *gorm.DB, issue *Issue) error {
	// NOTE: The explicit condition keeps updating an issue that is not yet
	// inserted (i.e., ID is 0) a no-op instead of an error.
	return tx.Model(issue).Where("id = ?", issue.ID).Select("*").Updates(issue).Error
}

// UpdateIssue updates all fields of given issue.
func UpdateIssue(issue *Issue) error {
	return updateIssue(Handle.db, issue)
}

func updateIssueUsersByStatus(tx *gorm.DB, issueID int64, isClosed bool) error {
	return tx.Exec("UPDATE issue_user SET is_closed = ? WHERE issue_id = ?", isClosed, issueID).Error
}

// UpdateIssueUsersByStatus updates issue-user relations by issue status.
func UpdateIssueUsersByStatus(issueID int64, isClosed bool) error {
	return updateIssueUsersByStatus(Handle.db, issueID, isClosed)
}

func updateIssueUserByAssignee(tx *gorm.DB, issue *Issue) (err error) {
	if err = tx.Exec("UPDATE issue_user SET is_assigned = ? WHERE issue_id = ?", false, issue.ID).Error; err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if issue.AssigneeID > 0 {
		if err = tx.Exec("UPDATE issue_user SET is_assigned = ? WHERE uid = ? AND issue_id = ?", true, issue.AssigneeID, issue.ID).Error; err != nil {
			return err
		}
	}

	return updateIssue(tx, issue)
}

// UpdateIssueUserByAssignee updates issue-user relation for assignee.
func UpdateIssueUserByAssignee(issue *Issue) (err error) {
	return Handle.db.Transaction(func(tx *gorm.DB) error {
		return updateIssueUserByAssignee(tx, issue)
	})
}

// UpdateIssueUserByRead updates issue-user relation for reading.
func UpdateIssueUserByRead(uid, issueID int64) error {
	return Handle.db.Exec("UPDATE issue_user SET is_read = ? WHERE uid = ? AND issue_id = ?", true, uid, issueID).Error
}

// updateIssueUsersByMentions updates issue-user pairs by mentioning.
func updateIssueUsersByMentions(tx *gorm.DB, issueID int64, uids []int64) error {
	for _, uid := range uids {
		iu := new(IssueUser)
		err := tx.Where(&IssueUser{UserID: uid, IssueID: issueID}).First(iu).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			iu.IsMentioned = true
			err = tx.Model(iu).Select("*").Updates(iu).Error
		} else {
			err = tx.Create(&IssueUser{UserID: uid, IssueID: issueID, IsMentioned: true}).Error
		}
		if err != nil {
			return err
		}
	}
	return nil
}
