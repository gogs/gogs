// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/markup"
)

// CommentType defines whether a comment is just a simple comment, an action (like close) or a reference.
type CommentType int

const (
	// Plain comment, can be associated with a commit (CommitID > 0) and a line (LineNum > 0)
	COMMENT_TYPE_COMMENT CommentType = iota
	COMMENT_TYPE_REOPEN
	COMMENT_TYPE_CLOSE

	// References.
	COMMENT_TYPE_ISSUE_REF
	// Reference from a commit (not part of a pull request)
	COMMENT_TYPE_COMMIT_REF
	// Reference from a comment
	COMMENT_TYPE_COMMENT_REF
	// Reference from a pull request
	COMMENT_TYPE_PULL_REF

	// Labels
	COMMENT_TYPE_LABEL_CHANGE
	// Milestones
	COMMENT_TYPE_MILESTONE_CHANGE
	// Title
	COMMENT_TYPE_TITLE_CHANGE
	// Assignee
	COMMENT_TYPE_ASSIGNEE_CHANGE
)

type CommentTag int

const (
	COMMENT_TAG_NONE CommentTag = iota
	COMMENT_TAG_POSTER
	COMMENT_TAG_WRITER
	COMMENT_TAG_OWNER
)

// Comment represents a comment in commit and issue page.
type Comment struct {
	ID              int64
	Type            CommentType
	PosterID        int64
	Poster          *User  `xorm:"-" json:"-"`
	IssueID         int64  `xorm:"INDEX"`
	Issue           *Issue `xorm:"-" json:"-"`
	CommitID        int64
	Line            int64
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-" json:"-"`

	AddedLabelID   int64  `xorm:"INDEX" json:"-"`
	AddedLabel     *Label `xorm:"-" json:"-"`
	RemovedLabelID int64  `xorm:"INDEX" json:"-"`
	RemovedLabel   *Label `xorm:"-" json:"-"`

	MilestoneID    int64      `xorm:"INDEX" json:"-"`
	Milestone      *Milestone `xorm:"-" json:"-"`
	OldMilestoneID int64      `xorm:"INDEX" json:"-"`
	OldMilestone   *Milestone `xorm:"-" json:"-"`

	AssigneeID    int64 `xorm:"-" json:"-"`
	Assignee      *User `xorm:"-" json:"-"`
	OldAssigneeID int64 `xorm:"-" json:"-"`
	OldAssignee   *User `xorm:"-" json:"-"`

	Title    string `xorm:"TEXT"`
	OldTitle string `xorm:"TEXT"`

	Created     time.Time `xorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-" json:"-"`
	UpdatedUnix int64

	// Reference issue in commit message
	CommitSHA string `xorm:"VARCHAR(40)"`

	Attachments []*Attachment `xorm:"-" json:"-"`

	// For view issue page.
	ShowTag CommentTag `xorm:"-" json:"-"`
}

func (c *Comment) BeforeInsert() {
	c.CreatedUnix = time.Now().Unix()
	c.UpdatedUnix = c.CreatedUnix
}

func (c *Comment) BeforeUpdate() {
	c.UpdatedUnix = time.Now().Unix()
}

func (c *Comment) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		c.Created = time.Unix(c.CreatedUnix, 0).Local()
	case "updated_unix":
		c.Updated = time.Unix(c.UpdatedUnix, 0).Local()
	}
}

func (c *Comment) loadAttributes(e Engine) (err error) {
	if c.Poster == nil {
		c.Poster, err = GetUserByID(c.PosterID)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.PosterID = -1
				c.Poster = NewGhostUser()
			} else {
				return fmt.Errorf("getUserByID.(Poster) [%d]: %v", c.PosterID, err)
			}
		}
	}

	if c.Issue == nil {
		c.Issue, err = getRawIssueByID(e, c.IssueID)
		if err != nil {
			return fmt.Errorf("getIssueByID [%d]: %v", c.IssueID, err)
		}
		if c.Issue.Repo == nil {
			c.Issue.Repo, err = getRepositoryByID(e, c.Issue.RepoID)
			if err != nil {
				return fmt.Errorf("getRepositoryByID [%d]: %v", c.Issue.RepoID, err)
			}
		}
	}

	if c.Attachments == nil {
		c.Attachments, err = getAttachmentsByCommentID(e, c.ID)
		if err != nil {
			return fmt.Errorf("getAttachmentsByCommentID [%d]: %v", c.ID, err)
		}
	}

	if c.Assignee == nil && c.AssigneeID > 0 {
		c.Assignee, err = GetUserByID(c.AssigneeID)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.AssigneeID = -1
				c.Assignee = NewGhostUser()
			} else {
				return fmt.Errorf("getUserByID.(Assignee) [%d]: %v", c.AssigneeID, err)
			}
		}
	}

	if c.OldAssignee == nil && c.OldAssigneeID > 0 {
		c.OldAssignee, err = GetUserByID(c.OldAssigneeID)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.OldAssigneeID = -1
				c.OldAssignee = NewGhostUser()
			} else {
				return fmt.Errorf("getUserByID.(OldAssignee) [%d]: %v", c.OldAssigneeID, err)
			}
		}
	}

	if c.Milestone == nil && c.MilestoneID > 0 {
		c.Milestone, err = c.Issue.Repo.GetMilestoneByID(c.MilestoneID)
		if err != nil {
			if _, ok := err.(ErrMilestoneNotExist); ok {
				c.MilestoneID = -1
				c.Milestone = &Milestone{ID: -1, RepoID: c.Issue.RepoID, Name: "Ghost"}
			} else {
				return fmt.Errorf("GetMilestoneByID.(Milestone) [%d]: %v", c.MilestoneID, err)
			}
		}
	}

	if c.OldMilestone == nil && c.OldMilestoneID > 0 {
		c.OldMilestone, err = c.Issue.Repo.GetMilestoneByID(c.OldMilestoneID)
		if err != nil {
			if _, ok := err.(ErrMilestoneNotExist); ok {
				c.OldMilestoneID = -1
				c.OldMilestone = &Milestone{ID: -1, RepoID: c.Issue.RepoID, Name: "Ghost"}
			} else {
				return fmt.Errorf("GetMilestoneByID.(OldMilestone) [%d]: %v", c.OldMilestoneID, err)
			}
		}
	}

	if c.AddedLabel == nil && c.AddedLabelID > 0 {
		c.AddedLabel, err = GetLabelByID(c.AddedLabelID)
		if err != nil {
			if _, ok := err.(ErrLabelNotExist); ok {
				c.AddedLabelID = -1
				c.AddedLabel = &Label{ID: -1, RepoID: c.Issue.RepoID, Name: "Ghost"}
			} else {
				return fmt.Errorf("GetLabelByID.(AddedLabel) [%d]: %v", c.AddedLabelID, err)
			}
		}
	}

	if c.RemovedLabel == nil && c.RemovedLabelID > 0 {
		c.RemovedLabel, err = GetLabelByID(c.RemovedLabelID)
		if err != nil {
			if _, ok := err.(ErrLabelNotExist); ok {
				c.RemovedLabelID = -1
				c.RemovedLabel = &Label{ID: -1, RepoID: c.Issue.RepoID, Name: "Ghost"}
			} else {
				return fmt.Errorf("GetLabelByID.(RemovedLabel) [%d]: %v", c.RemovedLabelID, err)
			}
		}
	}

	return nil
}

func (c *Comment) LoadAttributes() error {
	return c.loadAttributes(x)
}

func (c *Comment) HTMLURL() string {
	return fmt.Sprintf("%s#issuecomment-%d", c.Issue.HTMLURL(), c.ID)
}

// This method assumes following fields have been assigned with valid values:
// Required - Poster, Issue
func (c *Comment) APIFormat() *api.Comment {
	return &api.Comment{
		ID:      c.ID,
		HTMLURL: c.HTMLURL(),
		Poster:  c.Poster.APIFormat(),
		Body:    c.Content,
		Created: c.Created,
		Updated: c.Updated,
	}
}

func CommentHashTag(id int64) string {
	return "issuecomment-" + com.ToStr(id)
}

// HashTag returns unique hash tag for comment.
func (c *Comment) HashTag() string {
	return CommentHashTag(c.ID)
}

// EventTag returns unique event hash tag for comment.
func (c *Comment) EventTag() string {
	return "event-" + com.ToStr(c.ID)
}

// mailParticipants sends new comment emails to repository watchers
// and mentioned people.
func (cmt *Comment) mailParticipants(e Engine, opType ActionType, issue *Issue) (err error) {
	mentions := markup.FindAllMentions(cmt.Content)
	if err = updateIssueMentions(e, cmt.IssueID, mentions); err != nil {
		return fmt.Errorf("UpdateIssueMentions [%d]: %v", cmt.IssueID, err)
	}

	switch opType {
	case ACTION_COMMENT_ISSUE:
		issue.Content = cmt.Content
	case ACTION_CLOSE_ISSUE:
		issue.Content = fmt.Sprintf("Closed #%d", issue.Index)
	case ACTION_REOPEN_ISSUE:
		issue.Content = fmt.Sprintf("Reopened #%d", issue.Index)
	case ACTION_LABEL_ISSUE_CHANGE:
		switch {
		case cmt.AddedLabel != nil:
			issue.Content = fmt.Sprintf("Added label %s", cmt.AddedLabel.Name)
		case cmt.RemovedLabel != nil:
			issue.Content = fmt.Sprintf("Removed label %s", cmt.RemovedLabel.Name)
		default:
			issue.Content = "Cleared labels"
		}
	case ACTION_MILESTONE_ISSUE_CHANGE:
		switch {
		case cmt.OldMilestone == nil:
			issue.Content = fmt.Sprintf("Added milestone %s", cmt.Milestone.Name)
		case cmt.Milestone == nil:
			issue.Content = fmt.Sprintf("Cleared milestone %s", cmt.OldMilestone.Name)
		default:
			issue.Content = fmt.Sprintf("Changed milestone from %s to %s", cmt.OldMilestone.Name, cmt.Milestone.Name)
		}
	case ACTION_TITLE_ISSUE_CHANGE:
		issue.Content = fmt.Sprintf("Changed title from '%s' to '%s'", cmt.OldTitle, cmt.Title)
	case ACTION_ASSIGNEE_ISSUE_CHANGE:
		switch {
		case cmt.OldAssignee == nil:
			issue.Content = fmt.Sprintf("Added assignee %s", cmt.Assignee.Name)
		case cmt.Assignee == nil:
			issue.Content = fmt.Sprintf("Cleared milestone %s", cmt.OldAssignee.Name)
		default:
			issue.Content = fmt.Sprintf("Changed assignee from %s to %s", cmt.OldAssignee.Name, cmt.Assignee.Name)
		}
	}
	if err = mailIssueCommentToParticipants(issue, cmt.Poster, mentions); err != nil {
		log.Error(2, "mailIssueCommentToParticipants: %v", err)
	}

	return nil
}

func createComment(e *xorm.Session, opts *CreateCommentOptions) (_ *Comment, err error) {
	comment := &Comment{
		Type:           opts.Type,
		PosterID:       opts.Doer.ID,
		Poster:         opts.Doer,
		IssueID:        opts.Issue.ID,
		CommitID:       opts.CommitID,
		CommitSHA:      opts.CommitSHA,
		Line:           opts.LineNum,
		Content:        opts.Content,
		MilestoneID:    opts.MilestoneID,
		OldMilestoneID: opts.OldMilestoneID,
		AssigneeID:     opts.AssigneeID,
		OldAssigneeID:  opts.OldAssigneeID,
		AddedLabelID:   opts.AddedLabelID,
		RemovedLabelID: opts.RemovedLabelID,
		Title:          opts.Title,
		OldTitle:       opts.OldTitle,
	}
	if _, err = e.Insert(comment); err != nil {
		return nil, err
	}

	// Compose comment action, could be plain comment, close or reopen issue/pull request.
	// This object will be used to notify watchers in the end of function.
	act := &Action{
		ActUserID:    opts.Doer.ID,
		ActUserName:  opts.Doer.Name,
		Content:      fmt.Sprintf("%d|%s", opts.Issue.Index, strings.Split(opts.Content, "\n")[0]),
		RepoID:       opts.Repo.ID,
		RepoUserName: opts.Repo.Owner.Name,
		RepoName:     opts.Repo.Name,
		IsPrivate:    opts.Repo.IsPrivate,
	}

	// Check comment type.
	switch opts.Type {
	case COMMENT_TYPE_COMMENT:
		act.OpType = ACTION_COMMENT_ISSUE

		if _, err = e.Exec("UPDATE `issue` SET num_comments=num_comments+1 WHERE id=?", opts.Issue.ID); err != nil {
			return nil, err
		}

		// Check attachments
		attachments := make([]*Attachment, 0, len(opts.Attachments))
		for _, uuid := range opts.Attachments {
			attach, err := getAttachmentByUUID(e, uuid)
			if err != nil {
				if IsErrAttachmentNotExist(err) {
					continue
				}
				return nil, fmt.Errorf("getAttachmentByUUID [%s]: %v", uuid, err)
			}
			attachments = append(attachments, attach)
		}

		for i := range attachments {
			attachments[i].IssueID = opts.Issue.ID
			attachments[i].CommentID = comment.ID
			// No assign value could be 0, so ignore AllCols().
			if _, err = e.Id(attachments[i].ID).Update(attachments[i]); err != nil {
				return nil, fmt.Errorf("update attachment [%d]: %v", attachments[i].ID, err)
			}
		}

	case COMMENT_TYPE_REOPEN:
		act.OpType = ACTION_REOPEN_ISSUE
		if opts.Issue.IsPull {
			act.OpType = ACTION_REOPEN_PULL_REQUEST
		}

		if opts.Issue.IsPull {
			_, err = e.Exec("UPDATE `repository` SET num_closed_pulls=num_closed_pulls-1 WHERE id=?", opts.Repo.ID)
		} else {
			_, err = e.Exec("UPDATE `repository` SET num_closed_issues=num_closed_issues-1 WHERE id=?", opts.Repo.ID)
		}
		if err != nil {
			return nil, err
		}

	case COMMENT_TYPE_CLOSE:
		act.OpType = ACTION_CLOSE_ISSUE
		if opts.Issue.IsPull {
			act.OpType = ACTION_CLOSE_PULL_REQUEST
		}

		if opts.Issue.IsPull {
			_, err = e.Exec("UPDATE `repository` SET num_closed_pulls=num_closed_pulls+1 WHERE id=?", opts.Repo.ID)
		} else {
			_, err = e.Exec("UPDATE `repository` SET num_closed_issues=num_closed_issues+1 WHERE id=?", opts.Repo.ID)
		}
		if err != nil {
			return nil, err
		}

	case COMMENT_TYPE_LABEL_CHANGE:
		act.OpType = ACTION_LABEL_ISSUE_CHANGE
		act.Content = fmt.Sprintf("%d|%d", opts.Issue.Index, comment.ID)

	case COMMENT_TYPE_MILESTONE_CHANGE:
		act.OpType = ACTION_MILESTONE_ISSUE_CHANGE
		act.Content = fmt.Sprintf("%d|%d", opts.Issue.Index, comment.ID)

	case COMMENT_TYPE_TITLE_CHANGE:
		act.OpType = ACTION_TITLE_ISSUE_CHANGE
		act.Content = fmt.Sprintf("%d|%d", opts.Issue.Index, comment.ID)

	case COMMENT_TYPE_ASSIGNEE_CHANGE:
		act.OpType = ACTION_ASSIGNEE_ISSUE_CHANGE
		act.Content = fmt.Sprintf("%d|%d", opts.Issue.Index, comment.ID)
	}

	if _, err = e.Exec("UPDATE `issue` SET updated_unix = ? WHERE id = ?", time.Now().Unix(), opts.Issue.ID); err != nil {
		return nil, fmt.Errorf("update issue 'updated_unix': %v", err)
	}

	if err := comment.loadAttributes(e); err != nil {
		return nil, fmt.Errorf("loadAttributes: %v", err)
	}

	// Notify watchers for whatever action comes in, ignore if no action type.
	if act.OpType > 0 {
		if err = notifyWatchers(e, act); err != nil {
			log.Error(2, "notifyWatchers: %v", err)
		}
		if err = comment.mailParticipants(e, act.OpType, opts.Issue); err != nil {
			log.Error(2, "MailParticipants: %v", err)
		}
	}

	return comment, nil
}

func createStatusComment(e *xorm.Session, doer *User, repo *Repository, issue *Issue) (*Comment, error) {
	cmtType := COMMENT_TYPE_CLOSE
	if !issue.IsClosed {
		cmtType = COMMENT_TYPE_REOPEN
	}
	return createComment(e, &CreateCommentOptions{
		Type:  cmtType,
		Doer:  doer,
		Repo:  repo,
		Issue: issue,
	})
}

type CreateCommentOptions struct {
	Type  CommentType
	Doer  *User
	Repo  *Repository
	Issue *Issue

	CommitID                     int64
	CommitSHA                    string
	LineNum                      int64
	Content                      string
	Attachments                  []string // UUIDs of attachments
	MilestoneID, OldMilestoneID  int64
	AssigneeID, OldAssigneeID    int64
	AddedLabelID, RemovedLabelID int64
	Title, OldTitle              string
}

// CreateComment creates comment of issue or commit.
func CreateComment(opts *CreateCommentOptions) (comment *Comment, err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return nil, err
	}

	comment, err = createComment(sess, opts)
	if err != nil {
		return nil, err
	}

	return comment, sess.Commit()
}

// CreateIssueComment creates a plain issue comment.
func CreateIssueComment(doer *User, repo *Repository, issue *Issue, content string, attachments []string) (*Comment, error) {
	comment, err := CreateComment(&CreateCommentOptions{
		Type:        COMMENT_TYPE_COMMENT,
		Doer:        doer,
		Repo:        repo,
		Issue:       issue,
		Content:     content,
		Attachments: attachments,
	})
	if err != nil {
		return nil, fmt.Errorf("CreateComment: %v", err)
	}

	comment.Issue = issue
	if err = PrepareWebhooks(repo, HOOK_EVENT_ISSUE_COMMENT, &api.IssueCommentPayload{
		Action:     api.HOOK_ISSUE_COMMENT_CREATED,
		Issue:      issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: repo.APIFormat(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error(2, "PrepareWebhooks [comment_id: %d]: %v", comment.ID, err)
	}

	return comment, nil
}

// CreateRefComment creates a commit reference comment to issue.
func CreateRefComment(doer *User, repo *Repository, issue *Issue, content, commitSHA string) error {
	if len(commitSHA) == 0 {
		return fmt.Errorf("cannot create reference with empty commit SHA")
	}

	// Check if same reference from same commit has already existed.
	has, err := x.Get(&Comment{
		Type:      COMMENT_TYPE_COMMIT_REF,
		IssueID:   issue.ID,
		CommitSHA: commitSHA,
	})
	if err != nil {
		return fmt.Errorf("check reference comment: %v", err)
	} else if has {
		return nil
	}

	_, err = CreateComment(&CreateCommentOptions{
		Type:      COMMENT_TYPE_COMMIT_REF,
		Doer:      doer,
		Repo:      repo,
		Issue:     issue,
		CommitSHA: commitSHA,
		Content:   content,
	})
	return err
}

// CreateAssigneeComment creates an assignee comment on the issue.
func CreateAssigneeComment(doer *User, repo *Repository, issue *Issue, oldAssigneeID int64) error {
	_, err := CreateComment(&CreateCommentOptions{
		Type:          COMMENT_TYPE_ASSIGNEE_CHANGE,
		Doer:          doer,
		Repo:          repo,
		Issue:         issue,
		AssigneeID:    issue.AssigneeID,
		OldAssigneeID: oldAssigneeID,
	})
	return err
}

// CreateTitleComment creates a title change comment on the issue.
func CreateTitleComment(doer *User, repo *Repository, issue *Issue, oldTitle string) error {
	_, err := CreateComment(&CreateCommentOptions{
		Type:     COMMENT_TYPE_TITLE_CHANGE,
		Doer:     doer,
		Repo:     repo,
		Issue:    issue,
		Title:    issue.Title,
		OldTitle: oldTitle,
	})
	return err
}

// CreateMilestoneComment creates a milestone change comment on the issue.
func CreateMilestoneComment(doer *User, repo *Repository, issue *Issue, oldMilestoneID int64) error {
	_, err := CreateComment(&CreateCommentOptions{
		Type:           COMMENT_TYPE_MILESTONE_CHANGE,
		Doer:           doer,
		Repo:           repo,
		Issue:          issue,
		MilestoneID:    issue.MilestoneID,
		OldMilestoneID: oldMilestoneID,
	})
	return err
}

// CreateLabelComment creates a milestone change comment on the issue.
func CreateLabelComment(doer *User, repo *Repository, issue *Issue, removedLabelID, addedLabelID int64) error {
	_, err := CreateComment(&CreateCommentOptions{
		Type:           COMMENT_TYPE_LABEL_CHANGE,
		Doer:           doer,
		Repo:           repo,
		Issue:          issue,
		AddedLabelID:   addedLabelID,
		RemovedLabelID: removedLabelID,
	})
	return err
}

// GetCommentByID returns the comment by given ID.
func GetCommentByID(id int64) (*Comment, error) {
	c := new(Comment)
	has, err := x.Id(id).Get(c)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrCommentNotExist{id, 0}
	}
	return c, c.LoadAttributes()
}

// FIXME: use CommentList to improve performance.
func loadCommentsAttributes(e Engine, comments []*Comment) (err error) {
	for i := range comments {
		if err = comments[i].loadAttributes(e); err != nil {
			return fmt.Errorf("loadAttributes [%d]: %v", comments[i].ID, err)
		}
	}

	return nil
}

func getCommentsByIssueIDSince(e Engine, issueID, since int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	sess := e.Where("issue_id = ?", issueID).Asc("created_unix")
	if since > 0 {
		sess.And("updated_unix >= ?", since)
	}

	if err := sess.Find(&comments); err != nil {
		return nil, err
	}
	return comments, loadCommentsAttributes(e, comments)
}

func getCommentsByRepoIDSince(e Engine, repoID, since int64) ([]*Comment, error) {
	comments := make([]*Comment, 0, 10)
	sess := e.Where("issue.repo_id = ?", repoID).Join("INNER", "issue", "issue.id = comment.issue_id").Asc("comment.created_unix")
	if since > 0 {
		sess.And("comment.updated_unix >= ?", since)
	}
	if err := sess.Find(&comments); err != nil {
		return nil, err
	}
	return comments, loadCommentsAttributes(e, comments)
}

func getCommentsByIssueID(e Engine, issueID int64) ([]*Comment, error) {
	return getCommentsByIssueIDSince(e, issueID, -1)
}

// GetCommentsByIssueID returns all comments of an issue.
func GetCommentsByIssueID(issueID int64) ([]*Comment, error) {
	return getCommentsByIssueID(x, issueID)
}

// GetCommentsByIssueIDSince returns a list of comments of an issue since a given time point.
func GetCommentsByIssueIDSince(issueID, since int64) ([]*Comment, error) {
	return getCommentsByIssueIDSince(x, issueID, since)
}

// GetCommentsByRepoIDSince returns a list of comments for all issues in a repo since a given time point.
func GetCommentsByRepoIDSince(repoID, since int64) ([]*Comment, error) {
	return getCommentsByRepoIDSince(x, repoID, since)
}

// UpdateComment updates information of comment.
func UpdateComment(doer *User, c *Comment, oldContent string) (err error) {
	if _, err = x.Id(c.ID).AllCols().Update(c); err != nil {
		return err
	}

	if err = c.Issue.LoadAttributes(); err != nil {
		log.Error(2, "Issue.LoadAttributes [issue_id: %d]: %v", c.IssueID, err)
	} else if err = PrepareWebhooks(c.Issue.Repo, HOOK_EVENT_ISSUE_COMMENT, &api.IssueCommentPayload{
		Action:  api.HOOK_ISSUE_COMMENT_EDITED,
		Issue:   c.Issue.APIFormat(),
		Comment: c.APIFormat(),
		Changes: &api.ChangesPayload{
			Body: &api.ChangesFromPayload{
				From: oldContent,
			},
		},
		Repository: c.Issue.Repo.APIFormat(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error(2, "PrepareWebhooks [comment_id: %d]: %v", c.ID, err)
	}

	return nil
}

// DeleteCommentByID deletes the comment by given ID.
func DeleteCommentByID(doer *User, id int64) error {
	comment, err := GetCommentByID(id)
	if err != nil {
		if IsErrCommentNotExist(err) {
			return nil
		}
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.ID(comment.ID).Delete(new(Comment)); err != nil {
		return err
	}

	if comment.Type == COMMENT_TYPE_COMMENT {
		if _, err = sess.Exec("UPDATE `issue` SET num_comments = num_comments - 1 WHERE id = ?", comment.IssueID); err != nil {
			return err
		}
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("commit: %v", err)
	}

	_, err = DeleteAttachmentsByComment(comment.ID, true)
	if err != nil {
		log.Error(2, "Failed to delete attachments by comment[%d]: %v", comment.ID, err)
	}

	if err = comment.Issue.LoadAttributes(); err != nil {
		log.Error(2, "Issue.LoadAttributes [issue_id: %d]: %v", comment.IssueID, err)
	} else if err = PrepareWebhooks(comment.Issue.Repo, HOOK_EVENT_ISSUE_COMMENT, &api.IssueCommentPayload{
		Action:     api.HOOK_ISSUE_COMMENT_DELETED,
		Issue:      comment.Issue.APIFormat(),
		Comment:    comment.APIFormat(),
		Repository: comment.Issue.Repo.APIFormat(nil),
		Sender:     doer.APIFormat(),
	}); err != nil {
		log.Error(2, "PrepareWebhooks [comment_id: %d]: %v", comment.ID, err)
	}
	return nil
}
