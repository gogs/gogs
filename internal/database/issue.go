package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/markup"
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

func (issue *Issue) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "deadline_unix":
		issue.Deadline = time.Unix(issue.DeadlineUnix, 0).Local()
	case "created_unix":
		issue.Created = time.Unix(issue.CreatedUnix, 0).Local()
	case "updated_unix":
		issue.Updated = time.Unix(issue.UpdatedUnix, 0).Local()
	}
}

// Deprecated: Use Users.GetByID instead.
func getUserByID(e Engine, id int64) (*User, error) {
	u := new(User)
	has, err := e.ID(id).Get(u)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrUserNotExist{args: errutil.Args{"userID": id}}
	}

	// TODO(unknwon): Rely on AfterFind hook to sanitize user full name.
	u.FullName = markup.Sanitize(u.FullName)
	return u, nil
}

func (issue *Issue) loadAttributes(e Engine) (err error) {
	if issue.Repo == nil {
		issue.Repo, err = getRepositoryByID(e, issue.RepoID)
		if err != nil {
			return errors.Newf("getRepositoryByID [%d]: %v", issue.RepoID, err)
		}
	}

	if issue.Poster == nil {
		issue.Poster, err = getUserByID(e, issue.PosterID)
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
		issue.Labels, err = getLabelsByIssueID(e, issue.ID)
		if err != nil {
			return errors.Newf("getLabelsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Milestone == nil && issue.MilestoneID > 0 {
		issue.Milestone, err = getMilestoneByRepoID(e, issue.RepoID, issue.MilestoneID)
		if err != nil {
			return errors.Newf("getMilestoneByRepoID [repo_id: %d, milestone_id: %d]: %v", issue.RepoID, issue.MilestoneID, err)
		}
	}

	if issue.Assignee == nil && issue.AssigneeID > 0 {
		issue.Assignee, err = getUserByID(e, issue.AssigneeID)
		if err != nil {
			return errors.Newf("getUserByID.(assignee) [%d]: %v", issue.AssigneeID, err)
		}
	}

	if issue.IsPull && issue.PullRequest == nil {
		// It is possible pull request is not yet created.
		issue.PullRequest, err = getPullRequestByIssueID(e, issue.ID)
		if err != nil && !IsErrPullRequestNotExist(err) {
			return errors.Newf("getPullRequestByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Attachments == nil {
		issue.Attachments, err = getAttachmentsByIssueID(e, issue.ID)
		if err != nil {
			return errors.Newf("getAttachmentsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	if issue.Comments == nil {
		issue.Comments, err = getCommentsByIssueID(e, issue.ID)
		if err != nil {
			return errors.Newf("getCommentsByIssueID [%d]: %v", issue.ID, err)
		}
	}

	return nil
}

func (issue *Issue) LoadAttributes() error {
	return issue.loadAttributes(x)
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

func (issue *Issue) hasLabel(e Engine, labelID int64) bool {
	return hasIssueLabel(e, issue.ID, labelID)
}

// HasLabel returns true if issue has been labeled by given ID.
func (issue *Issue) HasLabel(labelID int64) bool {
	return issue.hasLabel(x, labelID)
}

func (issue *Issue) sendLabelUpdatedWebhook(doer *User) {
	var err error
	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			log.Error("LoadIssue: %v", err)
			return
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueLabelUpdated,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     apiv1types.WebhookIssueLabelUpdated,
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

func (issue *Issue) addLabel(e *xorm.Session, label *Label) error {
	return newIssueLabel(e, issue, label)
}

// AddLabel adds a new label to the issue.
func (issue *Issue) AddLabel(doer *User, label *Label) error {
	if err := NewIssueLabel(issue, label); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) addLabels(e *xorm.Session, labels []*Label) error {
	return newIssueLabels(e, issue, labels)
}

// AddLabels adds a list of new labels to the issue.
func (issue *Issue) AddLabels(doer *User, labels []*Label) error {
	if err := NewIssueLabels(issue, labels); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) getLabels(e Engine) (err error) {
	if len(issue.Labels) > 0 {
		return nil
	}

	issue.Labels, err = getLabelsByIssueID(e, issue.ID)
	if err != nil {
		return errors.Newf("getLabelsByIssueID: %v", err)
	}
	return nil
}

func (issue *Issue) removeLabel(e *xorm.Session, label *Label) error {
	return deleteIssueLabel(e, issue, label)
}

// RemoveLabel removes a label from issue by given ID.
func (issue *Issue) RemoveLabel(doer *User, label *Label) error {
	if err := DeleteIssueLabel(issue, label); err != nil {
		return err
	}

	issue.sendLabelUpdatedWebhook(doer)
	return nil
}

func (issue *Issue) clearLabels(e *xorm.Session) (err error) {
	if err = issue.getLabels(e); err != nil {
		return errors.Newf("getLabels: %v", err)
	}

	// NOTE: issue.removeLabel slices issue.Labels, so we need to create another slice to be unaffected.
	labels := make([]*Label, len(issue.Labels))
	copy(labels, issue.Labels)
	for i := range labels {
		if err = issue.removeLabel(e, labels[i]); err != nil {
			return errors.Newf("removeLabel: %v", err)
		}
	}

	return nil
}

func (issue *Issue) ClearLabels(doer *User) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = issue.clearLabels(sess); err != nil {
		return err
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
	}

	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			log.Error("LoadIssue: %v", err)
			return err
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueLabelCleared,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action:     apiv1types.WebhookIssueLabelCleared,
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
func (issue *Issue) ReplaceLabels(labels []*Label) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = issue.clearLabels(sess); err != nil {
		return errors.Newf("clearLabels: %v", err)
	} else if err = issue.addLabels(sess, labels); err != nil {
		return errors.Newf("addLabels: %v", err)
	}

	return sess.Commit()
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

func updateIssueCols(e Engine, issue *Issue, cols ...string) error {
	cols = append(cols, "updated_unix")
	_, err := e.ID(issue.ID).Cols(cols...).Update(issue)
	return err
}

// UpdateIssueCols only updates values of specific columns for given issue.
func UpdateIssueCols(issue *Issue, cols ...string) error {
	return updateIssueCols(x, issue, cols...)
}

func (issue *Issue) changeStatus(e *xorm.Session, doer *User, repo *Repository, isClosed bool) (err error) {
	// Nothing should be performed if current status is same as target status
	if issue.IsClosed == isClosed {
		return nil
	}
	issue.IsClosed = isClosed

	if err = updateIssueCols(e, issue, "is_closed"); err != nil {
		return err
	} else if err = updateIssueUsersByStatus(e, issue.ID, isClosed); err != nil {
		return err
	}

	// Update issue count of labels
	if err = issue.getLabels(e); err != nil {
		return err
	}
	for idx := range issue.Labels {
		if issue.IsClosed {
			issue.Labels[idx].NumClosedIssues++
		} else {
			issue.Labels[idx].NumClosedIssues--
		}
		if err = updateLabel(e, issue.Labels[idx]); err != nil {
			return err
		}
	}

	// Update issue count of milestone
	if err = changeMilestoneIssueStats(e, issue); err != nil {
		return err
	}

	// New action comment
	if _, err = createStatusComment(e, doer, repo, issue); err != nil {
		return err
	}

	return nil
}

// ChangeStatus changes issue status to open or closed.
func (issue *Issue) ChangeStatus(doer *User, repo *Repository, isClosed bool) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = issue.changeStatus(sess, doer, repo, isClosed); err != nil {
		return err
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
	}

	if issue.IsPull {
		// Merge pull request calls issue.changeStatus so we need to handle separately.
		issue.PullRequest.Issue = issue
		apiPullRequest := &apiv1types.WebhookPullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		}
		if isClosed {
			apiPullRequest.Action = apiv1types.WebhookIssueClosed
		} else {
			apiPullRequest.Action = apiv1types.WebhookIssueReopened
		}
		err = PrepareWebhooks(repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &apiv1types.WebhookIssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		}
		if isClosed {
			apiIssues.Action = apiv1types.WebhookIssueClosed
		} else {
			apiIssues.Action = apiv1types.WebhookIssueReopened
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
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueEdited,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Title: &apiv1types.WebhookChangesFromPayload{
					From: oldTitle,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action: apiv1types.WebhookIssueEdited,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Title: &apiv1types.WebhookChangesFromPayload{
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
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &apiv1types.WebhookPullRequestPayload{
			Action:      apiv1types.WebhookIssueEdited,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Body: &apiv1types.WebhookChangesFromPayload{
					From: oldContent,
				},
			},
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
			Action: apiv1types.WebhookIssueEdited,
			Index:  issue.Index,
			Issue:  issue.APIFormat(),
			Changes: &apiv1types.WebhookChangesPayload{
				Body: &apiv1types.WebhookChangesFromPayload{
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
		apiPullRequest := &apiv1types.WebhookPullRequestPayload{
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiPullRequest.Action = apiv1types.WebhookIssueUnassigned
		} else {
			apiPullRequest.Action = apiv1types.WebhookIssueAssigned
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, apiPullRequest)
	} else {
		apiIssues := &apiv1types.WebhookIssuesPayload{
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		}
		if isRemoveAssignee {
			apiIssues.Action = apiv1types.WebhookIssueUnassigned
		} else {
			apiIssues.Action = apiv1types.WebhookIssueAssigned
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

func newIssue(e *xorm.Session, opts NewIssueOptions) (err error) {
	opts.Issue.Title = strings.TrimSpace(opts.Issue.Title)
	opts.Issue.Index = opts.Repo.NextIssueIndex()

	if opts.Issue.MilestoneID > 0 {
		milestone, err := getMilestoneByRepoID(e, opts.Issue.RepoID, opts.Issue.MilestoneID)
		if err != nil && !IsErrMilestoneNotExist(err) {
			return errors.Newf("getMilestoneByID: %v", err)
		}

		// Assume milestone is invalid and drop silently.
		opts.Issue.MilestoneID = 0
		if milestone != nil {
			opts.Issue.MilestoneID = milestone.ID
			opts.Issue.Milestone = milestone
			if err = changeMilestoneAssign(e, opts.Issue, -1); err != nil {
				return err
			}
		}
	}

	if opts.Issue.AssigneeID > 0 {
		assignee, err := getUserByID(e, opts.Issue.AssigneeID)
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
	if _, err = e.Insert(opts.Issue); err != nil {
		return err
	}

	if opts.IsPull {
		_, err = e.Exec("UPDATE `repository` SET num_pulls = num_pulls + 1 WHERE id = ?", opts.Issue.RepoID)
	} else {
		_, err = e.Exec("UPDATE `repository` SET num_issues = num_issues + 1 WHERE id = ?", opts.Issue.RepoID)
	}
	if err != nil {
		return err
	}

	if len(opts.LableIDs) > 0 {
		// During the session, SQLite3 driver cannot handle retrieve objects after update something.
		// So we have to get all needed labels first.
		labels := make([]*Label, 0, len(opts.LableIDs))
		if err = e.In("id", opts.LableIDs).Find(&labels); err != nil {
			return errors.Newf("find all labels [label_ids: %v]: %v", opts.LableIDs, err)
		}

		for _, label := range labels {
			// Silently drop invalid labels.
			if label.RepoID != opts.Repo.ID {
				continue
			}

			if err = opts.Issue.addLabel(e, label); err != nil {
				return errors.Newf("addLabel [id: %d]: %v", label.ID, err)
			}
		}
	}

	if err = newIssueUsers(e, opts.Repo, opts.Issue); err != nil {
		return err
	}

	if len(opts.Attachments) > 0 {
		attachments, err := getAttachmentsByUUIDs(e, opts.Attachments)
		if err != nil {
			return errors.Newf("getAttachmentsByUUIDs [uuids: %v]: %v", opts.Attachments, err)
		}

		for i := range attachments {
			attachments[i].IssueID = opts.Issue.ID
			if _, err = e.ID(attachments[i].ID).Update(attachments[i]); err != nil {
				return errors.Newf("update attachment [id: %d]: %v", attachments[i].ID, err)
			}
		}
	}

	return opts.Issue.loadAttributes(e)
}

// NewIssue creates new issue with labels and attachments for repository.
func NewIssue(repo *Repository, issue *Issue, labelIDs []int64, uuids []string) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssue(sess, NewIssueOptions{
		Repo:        repo,
		Issue:       issue,
		LableIDs:    labelIDs,
		Attachments: uuids,
	}); err != nil {
		return errors.Newf("new issue: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return errors.Newf("commit: %v", err)
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

	if err = PrepareWebhooks(repo, HookEventTypeIssues, &apiv1types.WebhookIssuesPayload{
		Action:     apiv1types.WebhookIssueOpened,
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
	issue := &Issue{
		RepoID: repoID,
		Index:  index,
	}
	has, err := x.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{args: map[string]any{"repoID": repoID, "index": index}}
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

func getRawIssueByID(e Engine, id int64) (*Issue, error) {
	issue := new(Issue)
	has, err := e.ID(id).Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{args: map[string]any{"issueID": id}}
	}
	return issue, nil
}

func getIssueByID(e Engine, id int64) (*Issue, error) {
	issue, err := getRawIssueByID(e, id)
	if err != nil {
		return nil, err
	}
	return issue, issue.loadAttributes(e)
}

// GetIssueByID returns an issue by given ID.
func GetIssueByID(id int64) (*Issue, error) {
	return getIssueByID(x, id)
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
func buildIssuesQuery(opts *IssuesOptions) *xorm.Session {
	sess := x.NewSession()

	if opts.Page <= 0 {
		opts.Page = 1
	}

	if opts.RepoID > 0 {
		sess.Where("issue.repo_id=?", opts.RepoID).And("issue.is_closed=?", opts.IsClosed)
	} else if opts.RepoIDs != nil {
		// In case repository IDs are provided but actually no repository has issue.
		if len(opts.RepoIDs) == 0 {
			return nil
		}
		sess.In("issue.repo_id", opts.RepoIDs).And("issue.is_closed=?", opts.IsClosed)
	} else {
		sess.Where("issue.is_closed=?", opts.IsClosed)
	}

	if opts.AssigneeID > 0 {
		sess.And("issue.assignee_id=?", opts.AssigneeID)
	} else if opts.PosterID > 0 {
		sess.And("issue.poster_id=?", opts.PosterID)
	}

	if opts.MilestoneID > 0 {
		sess.And("issue.milestone_id=?", opts.MilestoneID)
	}

	sess.And("issue.is_pull=?", opts.IsPull)

	switch opts.SortType {
	case "oldest":
		sess.Asc("issue.created_unix")
	case "recentupdate":
		sess.Desc("issue.updated_unix")
	case "leastupdate":
		sess.Asc("issue.updated_unix")
	case "mostcomment":
		sess.Desc("issue.num_comments")
	case "leastcomment":
		sess.Asc("issue.num_comments")
	case "priority":
		sess.Desc("issue.priority")
	default:
		sess.Desc("issue.created_unix")
	}

	if len(opts.Labels) > 0 && opts.Labels != "0" {
		labelIDs := strings.Split(opts.Labels, ",")
		if len(labelIDs) > 0 {
			sess.Join("INNER", "issue_label", "issue.id = issue_label.issue_id").In("issue_label.label_id", labelIDs)
		}
	}

	if opts.IsMention {
		sess.Join("INNER", "issue_user", "issue.id = issue_user.issue_id").And("issue_user.is_mentioned = ?", true)

		if opts.UserID > 0 {
			sess.And("issue_user.uid = ?", opts.UserID)
		}
	}

	return sess
}

// IssuesCount returns the number of issues by given conditions.
func IssuesCount(opts *IssuesOptions) (int64, error) {
	sess := buildIssuesQuery(opts)
	if sess == nil {
		return 0, nil
	}

	return sess.Count(&Issue{})
}

// Issues returns a list of issues by given conditions.
func Issues(opts *IssuesOptions) ([]*Issue, error) {
	sess := buildIssuesQuery(opts)
	if sess == nil {
		return make([]*Issue, 0), nil
	}

	sess.Limit(conf.UI.IssuePagingNum, (opts.Page-1)*conf.UI.IssuePagingNum)

	issues := make([]*Issue, 0, conf.UI.IssuePagingNum)
	if err := sess.Find(&issues); err != nil {
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
	if err := x.Table("comment").Cols("poster_id").
		Where("issue_id = ?", issueID).
		Distinct("poster_id").
		Find(&userIDs); err != nil {
		return nil, errors.Newf("get poster IDs: %v", err)
	}
	if len(userIDs) == 0 {
		return nil, nil
	}

	users := make([]*User, 0, len(userIDs))
	return users, x.In("id", userIDs).Find(&users)
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

func newIssueUsers(e *xorm.Session, repo *Repository, issue *Issue) error {
	assignees, err := repo.getAssignees(e)
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

	if _, err = e.Insert(issueUsers); err != nil {
		return err
	}
	return nil
}

// NewIssueUsers adds new issue-user relations for new issue of repository.
func NewIssueUsers(repo *Repository, issue *Issue) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssueUsers(sess, repo, issue); err != nil {
		return err
	}

	return sess.Commit()
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
	err := x.Where("is_closed=?", isClosed).Find(&ius, &IssueUser{RepoID: rid, UserID: uid})
	return ius, err
}

// GetIssueUserPairsByRepoIds returns issue-user pairs by given repository IDs.
func GetIssueUserPairsByRepoIds(rids []int64, isClosed bool, page int) ([]*IssueUser, error) {
	if len(rids) == 0 {
		return []*IssueUser{}, nil
	}

	ius := make([]*IssueUser, 0, 10)
	sess := x.Limit(20, (page-1)*20).Where("is_closed=?", isClosed).In("repo_id", rids)
	err := sess.Find(&ius)
	return ius, err
}

// GetIssueUserPairsByMode returns issue-user pairs by given repository and user.
func GetIssueUserPairsByMode(userID, repoID int64, filterMode FilterMode, isClosed bool, page int) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	sess := x.Limit(20, (page-1)*20).Where("uid=?", userID).And("is_closed=?", isClosed)
	if repoID > 0 {
		sess.And("repo_id=?", repoID)
	}

	switch filterMode {
	case FilterModeAssign:
		sess.And("is_assigned=?", true)
	case FilterModeCreate:
		sess.And("is_poster=?", true)
	default:
		return ius, nil
	}
	err := sess.Find(&ius)
	return ius, err
}

// updateIssueMentions extracts mentioned people from content and
// updates issue-user relations for them.
func updateIssueMentions(e Engine, issueID int64, mentions []string) error {
	if len(mentions) == 0 {
		return nil
	}

	for i := range mentions {
		mentions[i] = strings.ToLower(mentions[i])
	}
	users := make([]*User, 0, len(mentions))

	if err := e.In("lower_name", mentions).Asc("lower_name").Find(&users); err != nil {
		return errors.Newf("find mentioned users: %v", err)
	}

	ids := make([]int64, 0, len(mentions))
	for _, user := range users {
		ids = append(ids, user.ID)
		if !user.IsOrganization() || user.NumMembers == 0 {
			continue
		}

		memberIDs := make([]int64, 0, user.NumMembers)
		orgUsers, err := getOrgUsersByOrgID(e, user.ID, 0)
		if err != nil {
			return errors.Newf("getOrgUsersByOrgID [%d]: %v", user.ID, err)
		}

		for _, orgUser := range orgUsers {
			memberIDs = append(memberIDs, orgUser.ID)
		}

		ids = append(ids, memberIDs...)
	}

	if err := updateIssueUsersByMentions(e, issueID, ids); err != nil {
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

	countSession := func(opts *IssueStatsOptions) *xorm.Session {
		sess := x.Where("issue.repo_id = ?", opts.RepoID).And("is_pull = ?", opts.IsPull)

		if len(opts.Labels) > 0 && opts.Labels != "0" {
			labelIDs := tool.StringsToInt64s(strings.Split(opts.Labels, ","))
			if len(labelIDs) > 0 {
				sess.Join("INNER", "issue_label", "issue.id = issue_id").In("label_id", labelIDs)
			}
		}

		if opts.MilestoneID > 0 {
			sess.And("issue.milestone_id = ?", opts.MilestoneID)
		}

		if opts.AssigneeID > 0 {
			sess.And("assignee_id = ?", opts.AssigneeID)
		}

		return sess
	}

	switch opts.FilterMode {
	case FilterModeYourRepos, FilterModeAssign:
		stats.OpenCount, _ = countSession(opts).
			And("is_closed = ?", false).
			Count(new(Issue))

		stats.ClosedCount, _ = countSession(opts).
			And("is_closed = ?", true).
			Count(new(Issue))
	case FilterModeCreate:
		stats.OpenCount, _ = countSession(opts).
			And("poster_id = ?", opts.UserID).
			And("is_closed = ?", false).
			Count(new(Issue))

		stats.ClosedCount, _ = countSession(opts).
			And("poster_id = ?", opts.UserID).
			And("is_closed = ?", true).
			Count(new(Issue))
	case FilterModeMention:
		stats.OpenCount, _ = countSession(opts).
			Join("INNER", "issue_user", "issue.id = issue_user.issue_id").
			And("issue_user.uid = ?", opts.UserID).
			And("issue_user.is_mentioned = ?", true).
			And("issue.is_closed = ?", false).
			Count(new(Issue))

		stats.ClosedCount, _ = countSession(opts).
			Join("INNER", "issue_user", "issue.id = issue_user.issue_id").
			And("issue_user.uid = ?", opts.UserID).
			And("issue_user.is_mentioned = ?", true).
			And("issue.is_closed = ?", true).
			Count(new(Issue))
	}
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(repoID, userID int64, repoIDs []int64, filterMode FilterMode, isPull bool) *IssueStats {
	stats := &IssueStats{}
	hasAnyRepo := repoID > 0 || len(repoIDs) > 0
	countSession := func(isClosed, isPull bool, repoID int64, repoIDs []int64) *xorm.Session {
		sess := x.Where("issue.is_closed = ?", isClosed).And("issue.is_pull = ?", isPull)

		if repoID > 0 {
			sess.And("repo_id = ?", repoID)
		} else if len(repoIDs) > 0 {
			sess.In("repo_id", repoIDs)
		}

		return sess
	}

	stats.AssignCount, _ = countSession(false, isPull, repoID, nil).
		And("assignee_id = ?", userID).
		Count(new(Issue))

	stats.CreateCount, _ = countSession(false, isPull, repoID, nil).
		And("poster_id = ?", userID).
		Count(new(Issue))

	if hasAnyRepo {
		stats.YourReposCount, _ = countSession(false, isPull, repoID, repoIDs).
			Count(new(Issue))
	}

	switch filterMode {
	case FilterModeYourRepos:
		if !hasAnyRepo {
			break
		}

		stats.OpenCount, _ = countSession(false, isPull, repoID, repoIDs).
			Count(new(Issue))
		stats.ClosedCount, _ = countSession(true, isPull, repoID, repoIDs).
			Count(new(Issue))
	case FilterModeAssign:
		stats.OpenCount, _ = countSession(false, isPull, repoID, nil).
			And("assignee_id = ?", userID).
			Count(new(Issue))
		stats.ClosedCount, _ = countSession(true, isPull, repoID, nil).
			And("assignee_id = ?", userID).
			Count(new(Issue))
	case FilterModeCreate:
		stats.OpenCount, _ = countSession(false, isPull, repoID, nil).
			And("poster_id = ?", userID).
			Count(new(Issue))
		stats.ClosedCount, _ = countSession(true, isPull, repoID, nil).
			And("poster_id = ?", userID).
			Count(new(Issue))
	}

	return stats
}

// GetRepoIssueStats returns number of open and closed repository issues by given filter mode.
func GetRepoIssueStats(repoID, userID int64, filterMode FilterMode, isPull bool) (numOpen, numClosed int64) {
	countSession := func(isClosed, isPull bool, repoID int64) *xorm.Session {
		sess := x.Where("issue.repo_id = ?", isClosed).
			And("is_pull = ?", isPull).
			And("repo_id = ?", repoID)

		return sess
	}

	openCountSession := countSession(false, isPull, repoID)
	closedCountSession := countSession(true, isPull, repoID)

	switch filterMode {
	case FilterModeAssign:
		openCountSession.And("assignee_id = ?", userID)
		closedCountSession.And("assignee_id = ?", userID)
	case FilterModeCreate:
		openCountSession.And("poster_id = ?", userID)
		closedCountSession.And("poster_id = ?", userID)
	}

	openResult, _ := openCountSession.Count(new(Issue))
	closedResult, _ := closedCountSession.Count(new(Issue))

	return openResult, closedResult
}

func updateIssue(e Engine, issue *Issue) error {
	_, err := e.ID(issue.ID).AllCols().Update(issue)
	return err
}

// UpdateIssue updates all fields of given issue.
func UpdateIssue(issue *Issue) error {
	return updateIssue(x, issue)
}

func updateIssueUsersByStatus(e Engine, issueID int64, isClosed bool) error {
	_, err := e.Exec("UPDATE `issue_user` SET is_closed=? WHERE issue_id=?", isClosed, issueID)
	return err
}

// UpdateIssueUsersByStatus updates issue-user relations by issue status.
func UpdateIssueUsersByStatus(issueID int64, isClosed bool) error {
	return updateIssueUsersByStatus(x, issueID, isClosed)
}

func updateIssueUserByAssignee(e *xorm.Session, issue *Issue) (err error) {
	if _, err = e.Exec("UPDATE `issue_user` SET is_assigned = ? WHERE issue_id = ?", false, issue.ID); err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if issue.AssigneeID > 0 {
		if _, err = e.Exec("UPDATE `issue_user` SET is_assigned = ? WHERE uid = ? AND issue_id = ?", true, issue.AssigneeID, issue.ID); err != nil {
			return err
		}
	}

	return updateIssue(e, issue)
}

// UpdateIssueUserByAssignee updates issue-user relation for assignee.
func UpdateIssueUserByAssignee(issue *Issue) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = updateIssueUserByAssignee(sess, issue); err != nil {
		return err
	}

	return sess.Commit()
}

// UpdateIssueUserByRead updates issue-user relation for reading.
func UpdateIssueUserByRead(uid, issueID int64) error {
	_, err := x.Exec("UPDATE `issue_user` SET is_read=? WHERE uid=? AND issue_id=?", true, uid, issueID)
	return err
}

// updateIssueUsersByMentions updates issue-user pairs by mentioning.
func updateIssueUsersByMentions(e Engine, issueID int64, uids []int64) error {
	for _, uid := range uids {
		iu := &IssueUser{
			UserID:  uid,
			IssueID: issueID,
		}
		has, err := e.Get(iu)
		if err != nil {
			return err
		}

		iu.IsMentioned = true
		if has {
			_, err = e.ID(iu.ID).AllCols().Update(iu)
		} else {
			_, err = e.Insert(iu)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
