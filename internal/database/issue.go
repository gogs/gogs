package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/com"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/markup"
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
	Title           string      `xorm:"name" gorm:"name"`
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

func (issue *Issue) BeforeInsert() {
	issue.CreatedUnix = time.Now().Unix()
	issue.UpdatedUnix = issue.CreatedUnix
}

func (issue *Issue) BeforeUpdate() {
	issue.UpdatedUnix = time.Now().Unix()
	issue.DeadlineUnix = issue.Deadline.Unix()
}

func (issue *Issue) AfterFind(tx *gorm.DB) error {
	issue.Deadline = time.Unix(issue.DeadlineUnix, 0).Local()
	issue.Created = time.Unix(issue.CreatedUnix, 0).Local()
	issue.Updated = time.Unix(issue.UpdatedUnix, 0).Local()
	return nil
}

// Deprecated: Use Users.GetByID instead.
func getUserByID(db *gorm.DB, id int64) (*User, error) {
	u := new(User)
	err := db.First(u, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotExist{args: errutil.Args{"userID": id}}
		}
		return nil, err
	}

	// TODO(unknwon): Rely on AfterFind hook to sanitize user full name.
	u.FullName = markup.Sanitize(u.FullName)
	return u, nil
}

func (issue *Issue) loadAttributes(db *gorm.DB) (err error) {
	if issue.Repo == nil {
		issue.Repo, err = getRepositoryByID(db, issue.RepoID)
		if err != nil {
			return errors.Newf("getRepositoryByID [%d]: %v", issue.RepoID, err)
		}
	}

	if issue.Poster == nil {
		issue.Poster, err = getUserByID(db, issue.PosterID)
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
		issue.Labels, err = getLabelsByIssueID(db, issue.ID)
		if err != nil {
			return errors.Newf("getLabelsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Milestone == nil && issue.MilestoneID > 0 {
		issue.Milestone, err = getMilestoneByRepoID(db, issue.RepoID, issue.MilestoneID)
		if err != nil {
			return errors.Newf("getMilestoneByRepoID [repo_id: %d, milestone_id: %d]: %v", issue.RepoID, issue.MilestoneID, err)
		}
	}

	if issue.Assignee == nil && issue.AssigneeID > 0 {
		issue.Assignee, err = getUserByID(db, issue.AssigneeID)
		if err != nil {
			return errors.Newf("getUserByID.(assignee) [%d]: %v", issue.AssigneeID, err)
		}
	}

	if issue.IsPull && issue.PullRequest == nil {
		// It is possible pull request is not yet created.
		issue.PullRequest, err = getPullRequestByIssueID(db, issue.ID)
		if err != nil && !IsErrPullRequestNotExist(err) {
			return errors.Newf("getPullRequestByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Attachments == nil {
		issue.Attachments, err = getAttachmentsByIssueID(db, issue.ID)
		if err != nil {
			return errors.Newf("getAttachmentsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Comments == nil {
		issue.Comments, err = getCommentsByIssueID(db, issue.ID)
		if err != nil {
			return errors.Newf("getCommentsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	return nil
}

func (issue *Issue) LoadAttributes() error {
	return issue.loadAttributes(db)
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
func (issue *Issue) State() api.StateType {
	if issue.IsClosed {
		return api.STATE_CLOSED
	}
	return api.STATE_OPEN
}

// This method assumes some fields assigned with values:
// Required - Poster, Labels,
// Optional - Milestone, Assignee, PullRequest
func (issue *Issue) APIFormat() *api.Issue {
	apiLabels := make([]*api.Label, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}

	apiIssue := &api.Issue{
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
		apiIssue.PullRequest = &api.PullRequestMeta{
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
	return "issue-" + com.ToStr(issue.ID)
}

// IsPoster returns true if given user by ID is the poster.
func (issue *Issue) IsPoster(uid int64) bool {
	return issue.PosterID == uid
}

func (issue *Issue) hasLabel(db *gorm.DB, labelID int64) bool {
	return hasIssueLabel(db, issue.ID, labelID)
}

// HasLabel returns true if issue has been labeled by given ID.
func (issue *Issue) HasLabel(labelID int64) bool {
	return issue.hasLabel(db, labelID)
}

func (issue *Issue) sendLabelUpdatedWebhook(doer *User) {
	var err error
	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			log.Error("LoadIssue: %v", err)
			return
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &api.PullRequestPayload{
			Action:      api.HOOK_ISSUE_LABEL_UPDATED,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &api.IssuesPayload{
			Action:     api.HOOK_ISSUE_LABEL_UPDATED,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
	}
}

func (issue *Issue) addLabel(tx *gorm.DB, label *Label) error {
	return newIssueLabel(tx, issue, label)
}

// AddLabel adds a new label to the issue.
func (issue *Issue) AddLabel(doer *User, label *Label) error {
	if err := NewIssueLabel(issue, label); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) addLabels(tx *gorm.DB, labels []*Label) error {
	return newIssueLabels(tx, issue, labels)
}

// AddLabels adds a list of new labels to the issue.
func (issue *Issue) AddLabels(doer *User, labels []*Label) error {
	if err := NewIssueLabels(issue, labels); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) getLabels(db *gorm.DB) (err error) {
	if len(issue.Labels) > 0 {
		return nil
	}

	issue.Labels, err = getLabelsByIssueID(db, issue.ID)
	if err != nil {
		return errors.Newf("getLabelsByIssueID: %v", err)
	}
	return nil
}

func (issue *Issue) removeLabel(tx *gorm.DB, label *Label) error {
	return deleteIssueLabel(tx, issue, label)
}

// RemoveLabel removes a label from issue by given ID.
func (issue *Issue) RemoveLabel(doer *User, label *Label) error {
	if err := DeleteIssueLabel(issue, label); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) clearLabels(tx *gorm.DB) (err error) {
	if err = issue.getLabels(tx); err != nil {
		return errors.Newf("getLabels: %v", err)
	}

	// NOTE: issue.removeLabel slices issue.Labels, so we need to create another slice to be unaffected.
	labels := make([]*Label, len(issue.Labels))
	copy(labels, issue.Labels)
	for i := range labels {
		if err = issue.removeLabel(tx, labels[i]); err != nil {
			return errors.Newf("removeLabel: %v", err)
		}
	}

	return nil
}

func (issue *Issue) ClearLabels(doer *User) (err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		return issue.clearLabels(tx)
	})
	if err != nil {
		return err
	}

	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			log.Error("LoadIssue: %v", err)
			return err
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &api.PullRequestPayload{
			Action:      api.HOOK_ISSUE_LABEL_CLEARED,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &api.IssuesPayload{
			Action:     api.HOOK_ISSUE_LABEL_CLEARED,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
	}

	return nil
}

// ReplaceLabels removes all current labels and add new labels to the issue.
func (issue *Issue) ReplaceLabels(labels []*Label) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := issue.clearLabels(tx); err != nil {
			return errors.Newf("clearLabels: %v", err)
		}
		if err := issue.addLabels(tx, labels); err != nil {
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

func updateIssueCols(db *gorm.DB, issue *Issue, cols ...string) error {
	updates := make(map[string]any)
	for _, col := range cols {
		switch col {
		case "is_closed":
			updates["is_closed"] = issue.IsClosed
		case "priority":
			updates["priority"] = issue.Priority
		case "milestone_id":
			updates["milestone_id"] = issue.MilestoneID
		case "assignee_id":
			updates["assignee_id"] = issue.AssigneeID
		case "num_comments":
			updates["num_comments"] = issue.NumComments
		case "deadline_unix":
			updates["deadline_unix"] = issue.DeadlineUnix
		case "title":
			updates["title"] = issue.Title
		case "content":
			updates["content"] = issue.Content
		}
	}
	updates["updated_unix"] = time.Now().Unix()
	return db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(updates).Error
}

// UpdateIssueCols only updates values of specific columns for given issue.
func UpdateIssueCols(issue *Issue, cols ...string) error {
	return updateIssueCols(db, issue, cols...)
}

func (issue *Issue) changeStatus(tx *gorm.DB, doer *User, repo *Repository, isClosed bool) (err error) {
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
		if err = updateLabel(tx, issue.Labels[idx]); err != nil {
			return err
		}
	}

	// Update issue count of milestone
	if err = changeMilestoneIssueStats(tx, issue); err != nil {
		return err
	}

	// New action comment
	if _, err = createStatusComment(tx, doer, repo, issue); err != nil {
		return err
	}

	return nil
}

// ChangeStatus changes issue status to open or closed.
func (issue *Issue) ChangeStatus(doer *User, repo *Repository, isClosed bool) (err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		return issue.changeStatus(tx, doer, repo, isClosed)
	})
	if err != nil {
		return err
	}

	if issue.IsPull {
		// Merge pull request calls issue.changeStatus so we need to handle separately.
		issue.PullRequest.Issue = issue
		apiPullRequest := &api.PullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		}
		if isClosed {
			apiPullRequest.Action = api.HOOK_ISSUE_CLOSED
		} else {
			apiPullRequest.Action = api.HOOK_ISSUE_REOPENED
		}
		err = PrepareWebhooks(repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &api.IssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		}
		if isClosed {
			apiIssues.Action = api.HOOK_ISSUE_CLOSED
		} else {
			apiIssues.Action = api.HOOK_ISSUE_REOPENED
		}
		err = PrepareWebhooks(repo, HookEventTypeIssues, apiIssues)
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v, is_closed: %v]: %v", issue.IsPull, isClosed, err)
	}

	return nil
}

func (issue *Issue) ChangeTitle(doer *User, title string) (err error) {
	oldTitle := issue.Title
	issue.Title = title
	if err = UpdateIssueCols(issue, "name"); err != nil {
		return errors.Newf("UpdateIssueCols: %v", err)
	}

	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &api.PullRequestPayload{
			Action:      api.HOOK_ISSUE_EDITED,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Changes: &api.ChangesPayload{
				Title: &api.ChangesFromPayload{
					From: oldTitle,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &api.IssuesPayload{
			Action: api.HOOK_ISSUE_EDITED,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &api.ChangesPayload{
				Title: &api.ChangesFromPayload{
					From: oldTitle,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
	}

	return nil
}

func (issue *Issue) ChangeContent(doer *User, content string) (err error) {
	oldContent := issue.Content
	issue.Content = content
	if err = UpdateIssueCols(issue, "content"); err != nil {
		return errors.Newf("UpdateIssueCols: %v", err)
	}

	if issue.IsPull {
		issue.PullRequest.Issue = issue
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &api.PullRequestPayload{
			Action:      api.HOOK_ISSUE_EDITED,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Changes: &api.ChangesPayload{
				Body: &api.ChangesFromPayload{
					From: oldContent,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &api.IssuesPayload{
			Action: api.HOOK_ISSUE_EDITED,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &api.ChangesPayload{
				Body: &api.ChangesFromPayload{
					From: oldContent,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
	}

	return nil
}

func (issue *Issue) ChangeAssignee(doer *User, assigneeID int64) (err error) {
	issue.AssigneeID = assigneeID
	if err = UpdateIssueUserByAssignee(issue); err != nil {
		return errors.Newf("UpdateIssueUserByAssignee: %v", err)
	}

	issue.Assignee, err = Handle.Users().GetByID(context.TODO(), issue.AssigneeID)
	if err != nil && !IsErrUserNotExist(err) {
		log.Error("Failed to get user by ID: %v", err)
		return nil
	}

	// Error not nil here means user does not exist, which is remove assignee.
	isRemoveAssignee := err != nil
	if issue.IsPull {
		issue.PullRequest.Issue = issue
		apiPullRequest := &api.PullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiPullRequest.Action = api.HOOK_ISSUE_UNASSIGNED
		} else {
			apiPullRequest.Action = api.HOOK_ISSUE_ASSIGNED
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &api.IssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiIssues.Action = api.HOOK_ISSUE_UNASSIGNED
		} else {
			apiIssues.Action = api.HOOK_ISSUE_ASSIGNED
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, apiIssues)
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v, remove_assignee: %v]: %v", issue.IsPull, isRemoveAssignee, err)
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
		milestone, err := getMilestoneByRepoID(tx, opts.Issue.RepoID, opts.Issue.MilestoneID)
		if err != nil && !IsErrMilestoneNotExist(err) {
			return errors.Newf("getMilestoneByID: %v", err)
		}

		// Assume milestone is invalid and drop silently.
		opts.Issue.MilestoneID = 0
		if milestone != nil {
			opts.Issue.MilestoneID = milestone.ID
			opts.Issue.Milestone = milestone
			if err = changeMilestoneAssign(tx, opts.Issue, -1); err != nil {
				return err
			}
		}
	}

	if opts.Issue.AssigneeID > 0 {
		assignee, err := getUserByID(tx, opts.Issue.AssigneeID)
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
		err = tx.Exec("UPDATE `repository` SET num_pulls = num_pulls + 1 WHERE id = ?", opts.Issue.RepoID).Error
	} else {
		err = tx.Exec("UPDATE `repository` SET num_issues = num_issues + 1 WHERE id = ?", opts.Issue.RepoID).Error
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

			if err = opts.Issue.addLabel(tx, label); err != nil {
				return errors.Newf("addLabel [id: %d]: %v", label.ID, err)
			}
		}
	}

	if err = newIssueUsers(tx, opts.Repo, opts.Issue); err != nil {
		return err
	}

	if len(opts.Attachments) > 0 {
		attachments, err := getAttachmentsByUUIDs(tx, opts.Attachments)
		if err != nil {
			return errors.Newf("getAttachmentsByUUIDs [uuids: %v]: %v", opts.Attachments, err)
		}

		for i := 0; i < len(attachments); i++ {
			attachments[i].IssueID = opts.Issue.ID
			if err = tx.Model(&Attachment{}).Where("id = ?", attachments[i].ID).Updates(attachments[i]).Error; err != nil {
				return errors.Newf("update attachment [id: %d]: %v", attachments[i].ID, err)
			}
		}
	}

	return opts.Issue.loadAttributes(tx)
}

// NewIssue creates new issue with labels and attachments for repository.
func NewIssue(repo *Repository, issue *Issue, labelIDs []int64, uuids []string) (err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
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
		log.Error("NotifyWatchers: %v", err)
	}
	if err = issue.MailParticipants(); err != nil {
		log.Error("MailParticipants: %v", err)
	}

	if err = PrepareWebhooks(repo, HookEventTypeIssues, &api.IssuesPayload{
		Action:     api.HOOK_ISSUE_OPENED,
		Index:      issue.Index,
		Issue:      issue.APIFormat(),
		Repository: repo.APIFormatLegacy(nil),
		Sender:     issue.Poster.APIFormat(),
	}); err != nil {
		log.Error("PrepareWebhooks: %v", err)
	}

	return nil
}

var _ errutil.NotFound = (*ErrIssueNotExist)(nil)

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
	n := strings.IndexByte(ref, byte('#'))
	if n == -1 {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	index := com.StrTo(ref[n+1:]).MustInt64()
	if index == 0 {
		return nil, ErrIssueNotExist{args: map[string]any{"ref": ref}}
	}

	repo, err := GetRepositoryByRef(ref[:n])
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
	issue := &Issue{
		RepoID: repoID,
		Index:  index,
	}
	err := db.Where("repo_id = ? AND `index` = ?", repoID, index).First(issue).Error
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

func getRawIssueByID(db *gorm.DB, id int64) (*Issue, error) {
	issue := new(Issue)
	err := db.First(issue, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIssueNotExist{args: map[string]any{"issueID": id}}
		}
		return nil, err
	}
	return issue, nil
}

func getIssueByID(db *gorm.DB, id int64) (*Issue, error) {
	issue, err := getRawIssueByID(db, id)
	if err != nil {
		return nil, err
	}
	return issue, issue.loadAttributes(db)
}

// GetIssueByID returns an issue by given ID.
func GetIssueByID(id int64) (*Issue, error) {
	return getIssueByID(db, id)
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
	query := db.Model(&Issue{})

	if opts.Page <= 0 {
		opts.Page = 1
	}

	if opts.RepoID > 0 {
		query = query.Where("issue.repo_id = ?", opts.RepoID).Where("issue.is_closed = ?", opts.IsClosed)
	} else if opts.RepoIDs != nil {
		// In case repository IDs are provided but actually no repository has issue.
		if len(opts.RepoIDs) == 0 {
			return nil
		}
		query = query.Where("issue.repo_id IN ?", opts.RepoIDs).Where("issue.is_closed = ?", opts.IsClosed)
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
	err := query.Count(&count).Error
	return count, err
}

// Issues returns a list of issues by given conditions.
func Issues(opts *IssuesOptions) ([]*Issue, error) {
	query := buildIssuesQuery(opts)
	if query == nil {
		return make([]*Issue, 0), nil
	}

	query = query.Limit(conf.UI.IssuePagingNum).Offset((opts.Page - 1) * conf.UI.IssuePagingNum)

	issues := make([]*Issue, 0, conf.UI.IssuePagingNum)
	if err := query.Find(&issues).Error; err != nil {
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

// GetParticipantsByIssueID returns all users who are participated in comments of an issue.
func GetParticipantsByIssueID(issueID int64) ([]*User, error) {
	userIDs := make([]int64, 0, 5)
	if err := db.Table("comment").
		Select("DISTINCT poster_id").
		Where("issue_id = ?", issueID).
		Pluck("poster_id", &userIDs).Error; err != nil {
		return nil, errors.Newf("get poster IDs: %v", err)
	}
	if len(userIDs) == 0 {
		return nil, nil
	}

	users := make([]*User, 0, len(userIDs))
	return users, db.Where("id IN ?", userIDs).Find(&users).Error
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

	if err = tx.Create(issueUsers).Error; err != nil {
		return err
	}
	return nil
}

// NewIssueUsers adds new issue-user relations for new issue of repository.
func NewIssueUsers(repo *Repository, issue *Issue) (err error) {
	return db.Transaction(func(tx *gorm.DB) error {
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
	err := db.Where("repo_id = ? AND uid = ? AND is_closed = ?", rid, uid, isClosed).Find(&ius).Error
	return ius, err
}

// GetIssueUserPairsByRepoIds returns issue-user pairs by given repository IDs.
func GetIssueUserPairsByRepoIds(rids []int64, isClosed bool, page int) ([]*IssueUser, error) {
	if len(rids) == 0 {
		return []*IssueUser{}, nil
	}

	ius := make([]*IssueUser, 0, 10)
	err := db.Limit(20).Offset((page-1)*20).Where("is_closed = ? AND repo_id IN ?", isClosed, rids).Find(&ius).Error
	return ius, err
}

// GetIssueUserPairsByMode returns issue-user pairs by given repository and user.
func GetIssueUserPairsByMode(userID, repoID int64, filterMode FilterMode, isClosed bool, page int) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	query := db.Limit(20).Offset((page-1)*20).Where("uid = ? AND is_closed = ?", userID, isClosed)
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
func updateIssueMentions(db *gorm.DB, issueID int64, mentions []string) error {
	if len(mentions) == 0 {
		return nil
	}

	for i := range mentions {
		mentions[i] = strings.ToLower(mentions[i])
	}
	users := make([]*User, 0, len(mentions))

	if err := db.Where("lower_name IN ?", mentions).Order("lower_name ASC").Find(&users).Error; err != nil {
		return errors.Newf("find mentioned users: %v", err)
	}

	ids := make([]int64, 0, len(mentions))
	for _, user := range users {
		ids = append(ids, user.ID)
		if !user.IsOrganization() || user.NumMembers == 0 {
			continue
		}

		memberIDs := make([]int64, 0, user.NumMembers)
		orgUsers, err := getOrgUsersByOrgID(db, user.ID, 0)
		if err != nil {
			return errors.Newf("getOrgUsersByOrgID [%d]: %v", user.ID, err)
		}

		for _, orgUser := range orgUsers {
			memberIDs = append(memberIDs, orgUser.ID)
		}

		ids = append(ids, memberIDs...)
	}

	if err := updateIssueUsersByMentions(db, issueID, ids); err != nil {
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
		query := db.Table("issue").Where("issue.repo_id = ? AND is_pull = ?", opts.RepoID, opts.IsPull)

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

	switch opts.FilterMode {
	case FilterModeYourRepos, FilterModeAssign:
		countSession(opts).Where("is_closed = ?", false).Count(&stats.OpenCount)
		countSession(opts).Where("is_closed = ?", true).Count(&stats.ClosedCount)
	case FilterModeCreate:
		countSession(opts).Where("poster_id = ? AND is_closed = ?", opts.UserID, false).Count(&stats.OpenCount)
		countSession(opts).Where("poster_id = ? AND is_closed = ?", opts.UserID, true).Count(&stats.ClosedCount)
	case FilterModeMention:
		countSession(opts).
			Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").
			Where("issue_user.uid = ? AND issue_user.is_mentioned = ? AND issue.is_closed = ?", opts.UserID, true, false).
			Count(&stats.OpenCount)
		countSession(opts).
			Joins("INNER JOIN issue_user ON issue.id = issue_user.issue_id").
			Where("issue_user.uid = ? AND issue_user.is_mentioned = ? AND issue.is_closed = ?", opts.UserID, true, true).
			Count(&stats.ClosedCount)
	}
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(repoID, userID int64, repoIDs []int64, filterMode FilterMode, isPull bool) *IssueStats {
	stats := &IssueStats{}
	hasAnyRepo := repoID > 0 || len(repoIDs) > 0
	countSession := func(isClosed, isPull bool, repoID int64, repoIDs []int64) *gorm.DB {
		query := db.Table("issue").Where("issue.is_closed = ? AND issue.is_pull = ?", isClosed, isPull)

		if repoID > 0 {
			query = query.Where("repo_id = ?", repoID)
		} else if len(repoIDs) > 0 {
			query = query.Where("repo_id IN ?", repoIDs)
		}

		return query
	}

	countSession(false, isPull, repoID, nil).Where("assignee_id = ?", userID).Count(&stats.AssignCount)
	countSession(false, isPull, repoID, nil).Where("poster_id = ?", userID).Count(&stats.CreateCount)

	if hasAnyRepo {
		countSession(false, isPull, repoID, repoIDs).Count(&stats.YourReposCount)
	}

	switch filterMode {
	case FilterModeYourRepos:
		if !hasAnyRepo {
			break
		}
		countSession(false, isPull, repoID, repoIDs).Count(&stats.OpenCount)
		countSession(true, isPull, repoID, repoIDs).Count(&stats.ClosedCount)
	case FilterModeAssign:
		countSession(false, isPull, repoID, nil).Where("assignee_id = ?", userID).Count(&stats.OpenCount)
		countSession(true, isPull, repoID, nil).Where("assignee_id = ?", userID).Count(&stats.ClosedCount)
	case FilterModeCreate:
		countSession(false, isPull, repoID, nil).Where("poster_id = ?", userID).Count(&stats.OpenCount)
		countSession(true, isPull, repoID, nil).Where("poster_id = ?", userID).Count(&stats.ClosedCount)
	}

	return stats
}

// GetRepoIssueStats returns number of open and closed repository issues by given filter mode.
func GetRepoIssueStats(repoID, userID int64, filterMode FilterMode, isPull bool) (numOpen, numClosed int64) {
	countSession := func(isClosed, isPull bool, repoID int64) *gorm.DB {
		return db.Table("issue").Where("issue.is_closed = ? AND is_pull = ? AND repo_id = ?", isClosed, isPull, repoID)
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

	openCountSession.Count(&numOpen)
	closedCountSession.Count(&numClosed)

	return numOpen, numClosed
}

func updateIssue(db *gorm.DB, issue *Issue) error {
	return db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(issue).Error
}

// UpdateIssue updates all fields of given issue.
func UpdateIssue(issue *Issue) error {
	return updateIssue(db, issue)
}

func updateIssueUsersByStatus(db *gorm.DB, issueID int64, isClosed bool) error {
	return db.Exec("UPDATE `issue_user` SET is_closed = ? WHERE issue_id = ?", isClosed, issueID).Error
}

// UpdateIssueUsersByStatus updates issue-user relations by issue status.
func UpdateIssueUsersByStatus(issueID int64, isClosed bool) error {
	return updateIssueUsersByStatus(db, issueID, isClosed)
}

func updateIssueUserByAssignee(tx *gorm.DB, issue *Issue) (err error) {
	if err = tx.Exec("UPDATE `issue_user` SET is_assigned = ? WHERE issue_id = ?", false, issue.ID).Error; err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if issue.AssigneeID > 0 {
		if err = tx.Exec("UPDATE `issue_user` SET is_assigned = ? WHERE uid = ? AND issue_id = ?", true, issue.AssigneeID, issue.ID).Error; err != nil {
			return err
		}
	}

	return updateIssue(tx, issue)
}

// UpdateIssueUserByAssignee updates issue-user relation for assignee.
func UpdateIssueUserByAssignee(issue *Issue) (err error) {
	return db.Transaction(func(tx *gorm.DB) error {
		return updateIssueUserByAssignee(tx, issue)
	})
}

// UpdateIssueUserByRead updates issue-user relation for reading.
func UpdateIssueUserByRead(uid, issueID int64) error {
	return db.Exec("UPDATE `issue_user` SET is_read = ? WHERE uid = ? AND issue_id = ?", true, uid, issueID).Error
}

// updateIssueUsersByMentions updates issue-user pairs by mentioning.
func updateIssueUsersByMentions(db *gorm.DB, issueID int64, uids []int64) error {
	for _, uid := range uids {
		iu := &IssueUser{
			UserID:  uid,
			IssueID: issueID,
		}
		err := db.Where("uid = ? AND issue_id = ?", uid, issueID).First(iu).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		iu.IsMentioned = true
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = db.Create(iu).Error
		} else {
			err = db.Model(&IssueUser{}).Where("id = ?", iu.ID).Updates(iu).Error
		}
		if err != nil {
			return err
		}
	}
	return nil
}
