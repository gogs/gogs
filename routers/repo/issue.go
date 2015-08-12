// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	ISSUES     base.TplName = "repo/issue/list"
	ISSUE_NEW  base.TplName = "repo/issue/new"
	ISSUE_VIEW base.TplName = "repo/issue/view"

	LABELS base.TplName = "repo/issue/labels"

	MILESTONE      base.TplName = "repo/issue/milestones"
	MILESTONE_NEW  base.TplName = "repo/issue/milestone_new"
	MILESTONE_EDIT base.TplName = "repo/issue/milestone_edit"
)

var (
	ErrFileTypeForbidden = errors.New("File type is not allowed")
	ErrTooManyFiles      = errors.New("Maximum number of files to upload exceeded")
)

func RetrieveLabels(ctx *middleware.Context) {
	labels, err := models.GetLabelsByRepoID(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "RetrieveLabels.GetLabels: %v", err)
		return
	}
	for _, l := range labels {
		l.CalOpenIssues()
	}
	ctx.Data["Labels"] = labels
	ctx.Data["NumLabels"] = len(labels)
}

func Issues(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.issues")
	ctx.Data["PageIsIssueList"] = true

	viewType := ctx.Query("type")
	types := []string{"assigned", "created_by", "mentioned"}
	if !com.IsSliceContainsStr(types, viewType) {
		viewType = "all"
	}

	// Must sign in to see issues about you.
	if viewType != "all" && !ctx.IsSigned {
		ctx.SetCookie("redirect_to", "/"+url.QueryEscape(setting.AppSubUrl+ctx.Req.RequestURI), 0, setting.AppSubUrl)
		ctx.Redirect(setting.AppSubUrl + "/user/login")
		return
	}

	var assigneeID, posterID int64
	filterMode := models.FM_ALL
	switch viewType {
	case "assigned":
		assigneeID = ctx.User.Id
		filterMode = models.FM_ASSIGN
	case "created_by":
		posterID = ctx.User.Id
		filterMode = models.FM_CREATE
	case "mentioned":
		filterMode = models.FM_MENTION
	}

	var uid int64 = -1
	if ctx.IsSigned {
		uid = ctx.User.Id
	}

	repo := ctx.Repo.Repository
	selectLabels := ctx.Query("labels")
	milestoneID := ctx.QueryInt64("milestone")
	isShowClosed := ctx.Query("state") == "closed"
	issueStats := models.GetIssueStats(repo.ID, uid, com.StrTo(selectLabels).MustInt64(), milestoneID, isShowClosed, filterMode)

	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(issueStats.OpenCount)
	} else {
		total = int(issueStats.ClosedCount)
	}
	ctx.Data["Page"] = paginater.New(total, setting.IssuePagingNum, page, 5)

	// Get issues.
	issues, err := models.Issues(uid, assigneeID, repo.ID, posterID, milestoneID,
		page, isShowClosed, filterMode == models.FM_MENTION, selectLabels, ctx.Query("sortType"))
	if err != nil {
		ctx.Handle(500, "Issues: %v", err)
		return
	}

	// Get issue-user relations.
	pairs, err := models.GetIssueUsers(repo.ID, posterID, isShowClosed)
	if err != nil {
		ctx.Handle(500, "GetIssueUsers: %v", err)
		return
	}

	// Get posters.
	for i := range issues {
		if err = issues[i].GetPoster(); err != nil {
			ctx.Handle(500, "GetPoster", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if err = issues[i].GetLabels(); err != nil {
			ctx.Handle(500, "GetLabels", fmt.Errorf("[#%d]%v", issues[i].ID, err))
			return
		}

		if !ctx.IsSigned {
			issues[i].IsRead = true
			continue
		}

		// Check read status.
		idx := models.PairsContains(pairs, issues[i].ID, ctx.User.Id)
		if idx > -1 {
			issues[i].IsRead = pairs[idx].IsRead
		} else {
			issues[i].IsRead = true
		}
	}
	ctx.Data["Issues"] = issues

	// Get milestones.
	miles, err := models.GetAllRepoMilestones(repo.ID)
	if err != nil {
		ctx.Handle(500, "GetAllRepoMilestones: %v", err)
		return
	}
	ctx.Data["Milestones"] = miles

	ctx.Data["IssueStats"] = issueStats
	ctx.Data["SelectLabels"] = com.StrTo(selectLabels).MustInt64()
	ctx.Data["ViewType"] = viewType
	ctx.Data["MilestoneID"] = milestoneID
	ctx.Data["IsShowClosed"] = isShowClosed
	if isShowClosed {
		ctx.Data["State"] = "closed"
	} else {
		ctx.Data["State"] = "open"
	}

	ctx.HTML(200, ISSUES)
}

func renderAttachmentSettings(ctx *middleware.Context) {
	ctx.Data["IsAttachmentEnabled"] = setting.AttachmentEnabled
	ctx.Data["AttachmentAllowedTypes"] = setting.AttachmentAllowedTypes
	ctx.Data["AttachmentMaxFiles"] = setting.AttachmentMaxFiles
}

func NewIssue(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.issues.new")
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	if ctx.User.IsAdmin {
		var (
			repo = ctx.Repo.Repository
			err  error
		)
		ctx.Data["Labels"], err = models.GetLabelsByRepoID(repo.ID)
		if err != nil {
			ctx.Handle(500, "GetLabelsByRepoID: %v", err)
			return
		}

		ctx.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}
		ctx.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
		if err != nil {
			ctx.Handle(500, "GetMilestones: %v", err)
			return
		}

		ctx.Data["Assignees"], err = repo.GetAssignees()
		if err != nil {
			ctx.Handle(500, "GetAssignees: %v", err)
			return
		}
	}

	ctx.HTML(200, ISSUE_NEW)
}

func NewIssuePost(ctx *middleware.Context, form auth.CreateIssueForm) {
	ctx.Data["Title"] = ctx.Tr("repo.issues.new")
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	var (
		repo        = ctx.Repo.Repository
		labelIDs    []int64
		milestoneID int64
		assigneeID  int64
		attachments []string
	)
	if ctx.User.IsAdmin {
		// Check labels.
		labelIDs = base.StringsToInt64s(strings.Split(form.LabelIDs, ","))
		labelIDMark := base.Int64sToMap(labelIDs)
		labels, err := models.GetLabelsByRepoID(repo.ID)
		if err != nil {
			ctx.Handle(500, "GetLabelsByRepoID: %v", err)
			return
		}
		hasSelected := false
		for i := range labels {
			if labelIDMark[labels[i].ID] {
				labels[i].IsChecked = true
				hasSelected = true
			}
		}
		ctx.Data["HasSelectedLabel"] = hasSelected
		ctx.Data["label_ids"] = form.LabelIDs
		ctx.Data["Labels"] = labels

		// Check milestone.
		milestoneID = form.MilestoneID
		if milestoneID > 0 {
			ctx.Data["OpenMilestones"], err = models.GetMilestones(repo.ID, -1, false)
			if err != nil {
				ctx.Handle(500, "GetMilestones: %v", err)
				return
			}
			ctx.Data["ClosedMilestones"], err = models.GetMilestones(repo.ID, -1, true)
			if err != nil {
				ctx.Handle(500, "GetMilestones: %v", err)
				return
			}
			ctx.Data["Milestone"], err = repo.GetMilestoneByID(milestoneID)
			if err != nil {
				ctx.Handle(500, "GetMilestoneByID: %v", err)
				return
			}
			ctx.Data["milestone_id"] = milestoneID
		}

		// Check assignee.
		assigneeID = form.AssigneeID
		if assigneeID > 0 {
			ctx.Data["Assignees"], err = repo.GetAssignees()
			if err != nil {
				ctx.Handle(500, "GetAssignees: %v", err)
				return
			}
			ctx.Data["Assignee"], err = repo.GetAssigneeByID(assigneeID)
			if err != nil {
				ctx.Handle(500, "GetAssigneeByID: %v", err)
				return
			}
			ctx.Data["assignee_id"] = assigneeID
		}
	}

	if setting.AttachmentEnabled {
		attachments = ctx.QueryStrings("attachments")
	}

	if ctx.HasError() {
		ctx.HTML(200, ISSUE_NEW)
		return
	}

	issue := &models.Issue{
		RepoID:      ctx.Repo.Repository.ID,
		Index:       int64(repo.NumIssues) + 1,
		Name:        form.Title,
		PosterID:    ctx.User.Id,
		Poster:      ctx.User,
		MilestoneID: milestoneID,
		AssigneeID:  assigneeID,
		Content:     form.Content,
	}
	if err := models.NewIssue(repo, issue, labelIDs, attachments); err != nil {
		ctx.Handle(500, "NewIssue", err)
		return
	}

	// Update mentions.
	mentions := base.MentionPattern.FindAllString(issue.Content, -1)
	if len(mentions) > 0 {
		for i := range mentions {
			mentions[i] = mentions[i][1:]
		}

		if err := models.UpdateMentions(mentions, issue.ID); err != nil {
			ctx.Handle(500, "UpdateMentions", err)
			return
		}
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			ctx.Handle(500, "SendIssueNotifyMail", err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(mentions))
		for _, m := range mentions {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			ctx.Handle(500, "SendIssueMentionMail", err)
			return
		}
	}

	log.Trace("Issue created: %d/%d", ctx.Repo.Repository.ID, issue.ID)
	ctx.Redirect(ctx.Repo.RepoLink + "/issues/" + com.ToStr(issue.Index))
}

func UploadIssueAttachment(ctx *middleware.Context) {
	if !setting.AttachmentEnabled {
		ctx.Error(404, "attachment is not enabled")
		return
	}

	allowedTypes := strings.Split(setting.AttachmentAllowedTypes, ",")
	file, header, err := ctx.Req.FormFile("file")
	if err != nil {
		ctx.Error(500, fmt.Sprintf("FormFile: %v", err))
		return
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, _ := file.Read(buf)
	if n > 0 {
		buf = buf[:n]
	}
	fileType := http.DetectContentType(buf)

	allowed := false
	for _, t := range allowedTypes {
		t := strings.Trim(t, " ")
		if t == "*/*" || t == fileType {
			allowed = true
			break
		}
	}

	if !allowed {
		ctx.Error(400, ErrFileTypeForbidden.Error())
		return
	}

	attach, err := models.NewAttachment(header.Filename, buf, file)
	if err != nil {
		ctx.Error(500, fmt.Sprintf("NewAttachment: %v", err))
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	ctx.JSON(200, map[string]string{
		"uuid": attach.UUID,
	})
}

func checkLabels(labels, allLabels []*models.Label) {
	for _, l := range labels {
		for _, l2 := range allLabels {
			if l.ID == l2.ID {
				l2.IsChecked = true
				break
			}
		}
	}
}

func ViewIssue(ctx *middleware.Context) {
	ctx.Data["PageIsIssueList"] = true
	ctx.Data["RequireDropzone"] = true
	renderAttachmentSettings(ctx)

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, ctx.ParamsInt64(":index"))
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "GetIssueByIndex", err)
		} else {
			ctx.Handle(500, "GetIssueByIndex", err)
		}
		return
	}
	ctx.Data["Title"] = issue.Name

	if err = issue.GetPoster(); err != nil {
		ctx.Handle(500, "GetPoster: %v", err)
		return
	}
	issue.RenderedContent = string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink))

	// Attchments.

	// Metas.
	if err = issue.GetLabels(); err != nil {
		ctx.Handle(500, "GetLabels", err)
		return
	}

	if ctx.IsSigned {
		// 	// Update issue-user.
		// 	if err = models.UpdateIssueUserPairByRead(ctx.User.Id, issue.ID); err != nil {
		// 		ctx.Handle(500, "UpdateIssueUserPairByRead: %v", err)
		// 		return
		// 	}

		if ctx.User.IsAdmin {
			// labels, err := models.GetLabelsByRepoID(ctx.Repo.Repository.ID)
			// if err != nil {
			// 	ctx.Handle(500, "GetLabels.2", err)
			// 	return
			// }
			// checkLabels(issue.Labels, labels)
			// ctx.Data["Labels"] = labels

			// // Get all milestones.
			// ctx.Data["OpenMilestones"], err = models.GetMilestones(ctx.Repo.Repository.ID, -1, false)
			// if err != nil {
			// 	ctx.Handle(500, "GetMilestones.1: %v", err)
			// 	return
			// }
			// ctx.Data["ClosedMilestones"], err = models.GetMilestones(ctx.Repo.Repository.ID, -1, true)
			// if err != nil {
			// 	ctx.Handle(500, "GetMilestones.2: %v", err)
			// 	return
			// }

			// // Get all collaborators.
			// ctx.Data["Collaborators"], err = ctx.Repo.Repository.GetCollaborators()
			// if err != nil {
			// 	ctx.Handle(500, "GetCollaborators", err)
			// 	return
			// }
		}
	}

	// // Get comments.
	// comments, err := models.GetIssueComments(issue.ID)
	// if err != nil {
	// 	ctx.Handle(500, "GetIssueComments: %v", err)
	// 	return
	// }

	// // Get posters.
	// for i := range comments {
	// 	u, err := models.GetUserByID(comments[i].PosterId)
	// 	if err != nil {
	// 		ctx.Handle(500, "GetUserById.2: %v", err)
	// 		return
	// 	}
	// 	comments[i].Poster = u

	// 	if comments[i].Type == models.COMMENT_TYPE_COMMENT {
	// 		comments[i].Content = string(base.RenderMarkdown([]byte(comments[i].Content), ctx.Repo.RepoLink))
	// 	}
	// }

	ctx.Data["Issue"] = issue
	// ctx.Data["Comments"] = comments
	// ctx.Data["IsIssueOwner"] = ctx.Repo.IsOwner() || (ctx.IsSigned && issue.PosterID == ctx.User.Id)
	ctx.HTML(200, ISSUE_VIEW)
}

func UpdateIssue(ctx *middleware.Context, form auth.CreateIssueForm) {
	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx <= 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, idx)
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "issue.UpdateIssue", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssue(GetIssueByIndex)", err)
		}
		return
	}

	if ctx.User.Id != issue.PosterID && !ctx.Repo.IsOwner() {
		ctx.Error(403)
		return
	}

	issue.Name = form.Title
	//issue.MilestoneId = form.MilestoneId
	//issue.AssigneeId = form.AssigneeId
	//issue.LabelIds = form.Labels
	issue.Content = form.Content
	// try get content from text, ignore conflict with preview ajax
	if form.Content == "" {
		issue.Content = ctx.Query("text")
	}
	if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssue(UpdateIssue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok":      true,
		"title":   issue.Name,
		"content": string(base.RenderMarkdown([]byte(issue.Content), ctx.Repo.RepoLink)),
	})
}

func UpdateIssueLabel(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner() {
		ctx.Error(403)
		return
	}

	idx := com.StrTo(ctx.Params(":index")).MustInt64()
	if idx <= 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, idx)
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "issue.UpdateIssueLabel(GetIssueByIndex)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueLabel(GetIssueByIndex)", err)
		}
		return
	}

	isAttach := ctx.Query("action") == "attach"
	labelStrId := ctx.Query("id")
	labelID := com.StrTo(labelStrId).MustInt64()
	label, err := models.GetLabelByID(labelID)
	if err != nil {
		if models.IsErrLabelNotExist(err) {
			ctx.Handle(404, "issue.UpdateIssueLabel(GetLabelById)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueLabel(GetLabelById)", err)
		}
		return
	}

	isNeedUpdate := false
	if isAttach {
		if !issue.HasLabel(labelID) {
			if err = issue.AddLabel(labelID); err != nil {
				ctx.Handle(500, "AddLabel", err)
				return
			}
			isNeedUpdate = true
		}
	} else {
		if issue.HasLabel(labelID) {
			if err = issue.RemoveLabel(labelID); err != nil {
				ctx.Handle(500, "RemoveLabel", err)
				return
			}
			isNeedUpdate = true
		}
	}

	if isNeedUpdate {
		if err = models.UpdateIssue(issue); err != nil {
			ctx.Handle(500, "issue.UpdateIssueLabel(UpdateIssue)", err)
			return
		}

		if isAttach {
			label.NumIssues++
			if issue.IsClosed {
				label.NumClosedIssues++
			}
		} else {
			label.NumIssues--
			if issue.IsClosed {
				label.NumClosedIssues--
			}
		}

		if err = models.UpdateLabel(label); err != nil {
			ctx.Handle(500, "issue.UpdateIssueLabel(UpdateLabel)", err)
			return
		}
	}
	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateIssueMilestone(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner() {
		ctx.Error(403)
		return
	}

	issueId := com.StrTo(ctx.Query("issue")).MustInt64()
	if issueId == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByID(issueId)
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "issue.UpdateIssueMilestone(GetIssueByID)", err)
		} else {
			ctx.Handle(500, "issue.UpdateIssueMilestone(GetIssueByID)", err)
		}
		return
	}

	oldMid := issue.MilestoneID
	mid := com.StrTo(ctx.Query("milestoneid")).MustInt64()
	if oldMid == mid {
		ctx.JSON(200, map[string]interface{}{
			"ok": true,
		})
		return
	}

	// Not check for invalid milestone id and give responsibility to owners.
	issue.MilestoneID = mid
	if err = models.ChangeMilestoneAssign(oldMid, issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssueMilestone(ChangeMilestoneAssign)", err)
		return
	} else if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "issue.UpdateIssueMilestone(UpdateIssue)", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func UpdateAssignee(ctx *middleware.Context) {
	if !ctx.Repo.IsOwner() {
		ctx.Error(403)
		return
	}

	issueId := com.StrTo(ctx.Query("issue")).MustInt64()
	if issueId == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByID(issueId)
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			ctx.Handle(404, "GetIssueByID", err)
		} else {
			ctx.Handle(500, "GetIssueByID", err)
		}
		return
	}

	aid := com.StrTo(ctx.Query("assigneeid")).MustInt64()
	// Not check for invalid assignee id and give responsibility to owners.
	issue.AssigneeID = aid
	if err = models.UpdateIssueUserByAssignee(issue.ID, aid); err != nil {
		ctx.Handle(500, "UpdateIssueUserPairByAssignee: %v", err)
		return
	} else if err = models.UpdateIssue(issue); err != nil {
		ctx.Handle(500, "UpdateIssue", err)
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}

func Comment(ctx *middleware.Context) {
	send := func(status int, data interface{}, err error) {
		if err != nil {
			log.Error(4, "issue.Comment(?): %s", err)

			ctx.JSON(status, map[string]interface{}{
				"ok":     false,
				"status": status,
				"error":  err.Error(),
			})
		} else {
			ctx.JSON(status, map[string]interface{}{
				"ok":     true,
				"status": status,
				"data":   data,
			})
		}
	}

	index := com.StrTo(ctx.Query("issueIndex")).MustInt64()
	if index == 0 {
		ctx.Error(404)
		return
	}

	issue, err := models.GetIssueByIndex(ctx.Repo.Repository.ID, index)
	if err != nil {
		if models.IsErrIssueNotExist(err) {
			send(404, nil, err)
		} else {
			send(200, nil, err)
		}

		return
	}

	// Check if issue owner changes the status of issue.
	var newStatus string
	if ctx.Repo.IsOwner() || issue.PosterID == ctx.User.Id {
		newStatus = ctx.Query("change_status")
	}
	if len(newStatus) > 0 {
		if (strings.Contains(newStatus, "Reopen") && issue.IsClosed) ||
			(strings.Contains(newStatus, "Close") && !issue.IsClosed) {
			issue.IsClosed = !issue.IsClosed
			if err = models.UpdateIssue(issue); err != nil {
				send(500, nil, err)
				return
			} else if err = models.UpdateIssueUserPairsByStatus(issue.ID, issue.IsClosed); err != nil {
				send(500, nil, err)
				return
			}

			if err = issue.GetLabels(); err != nil {
				send(500, nil, err)
				return
			}

			for _, label := range issue.Labels {
				if issue.IsClosed {
					label.NumClosedIssues++
				} else {
					label.NumClosedIssues--
				}

				if err = models.UpdateLabel(label); err != nil {
					send(500, nil, err)
					return
				}
			}

			// Change open/closed issue counter for the associated milestone
			if issue.MilestoneID > 0 {
				if err = models.ChangeMilestoneIssueStats(issue); err != nil {
					send(500, nil, err)
				}
			}

			cmtType := models.COMMENT_TYPE_CLOSE
			if !issue.IsClosed {
				cmtType = models.COMMENT_TYPE_REOPEN
			}

			if _, err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.ID, issue.ID, 0, 0, cmtType, "", nil); err != nil {
				send(200, nil, err)
				return
			}
			log.Trace("%s Issue(%d) status changed: %v", ctx.Req.RequestURI, issue.ID, !issue.IsClosed)
		}
	}

	var comment *models.Comment

	var ms []string
	content := ctx.Query("content")
	// Fix #321. Allow empty comments, as long as we have attachments.
	if len(content) > 0 || len(ctx.Req.MultipartForm.File["attachments"]) > 0 {
		switch ctx.Params(":action") {
		case "new":
			if comment, err = models.CreateComment(ctx.User.Id, ctx.Repo.Repository.ID, issue.ID, 0, 0, models.COMMENT_TYPE_COMMENT, content, nil); err != nil {
				send(500, nil, err)
				return
			}

			// Update mentions.
			ms = base.MentionPattern.FindAllString(issue.Content, -1)
			if len(ms) > 0 {
				for i := range ms {
					ms[i] = ms[i][1:]
				}

				if err := models.UpdateMentions(ms, issue.ID); err != nil {
					send(500, nil, err)
					return
				}
			}

			log.Trace("%s Comment created: %d", ctx.Req.RequestURI, issue.ID)
		default:
			ctx.Handle(404, "issue.Comment", err)
			return
		}
	}

	if comment != nil {
		// uploadFiles(ctx, issue.ID, comment.Id)
	}

	// Notify watchers.
	act := &models.Action{
		ActUserID:    ctx.User.Id,
		ActUserName:  ctx.User.LowerName,
		ActEmail:     ctx.User.Email,
		OpType:       models.COMMENT_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, strings.Split(content, "\n")[0]),
		RepoID:       ctx.Repo.Repository.ID,
		RepoUserName: ctx.Repo.Owner.LowerName,
		RepoName:     ctx.Repo.Repository.LowerName,
		IsPrivate:    ctx.Repo.Repository.IsPrivate,
	}
	if err = models.NotifyWatchers(act); err != nil {
		send(500, nil, err)
		return
	}

	// Mail watchers and mentions.
	if setting.Service.EnableNotifyMail {
		issue.Content = content
		tos, err := mailer.SendIssueNotifyMail(ctx.User, ctx.Repo.Owner, ctx.Repo.Repository, issue)
		if err != nil {
			send(500, nil, err)
			return
		}

		tos = append(tos, ctx.User.LowerName)
		newTos := make([]string, 0, len(ms))
		for _, m := range ms {
			if com.IsSliceContainsStr(tos, m) {
				continue
			}

			newTos = append(newTos, m)
		}
		if err = mailer.SendIssueMentionMail(ctx.Render, ctx.User, ctx.Repo.Owner,
			ctx.Repo.Repository, issue, models.GetUserEmailsByNames(newTos)); err != nil {
			send(500, nil, err)
			return
		}
	}

	send(200, fmt.Sprintf("%s/issues/%d", ctx.Repo.RepoLink, index), nil)
}

func Labels(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.labels")
	ctx.Data["PageIsLabels"] = true
	ctx.Data["RequireMinicolors"] = true
	ctx.HTML(200, LABELS)
}

func NewLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	ctx.Data["Title"] = ctx.Tr("repo.labels")
	ctx.Data["PageIsLabels"] = true

	if ctx.HasError() {
		ctx.Flash.Error(ctx.Data["ErrorMsg"].(string))
		ctx.Redirect(ctx.Repo.RepoLink + "/labels")
		return
	}

	l := &models.Label{
		RepoID: ctx.Repo.Repository.ID,
		Name:   form.Title,
		Color:  form.Color,
	}
	if err := models.NewLabel(l); err != nil {
		ctx.Handle(500, "NewLabel", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/labels")
}

func UpdateLabel(ctx *middleware.Context, form auth.CreateLabelForm) {
	l, err := models.GetLabelByID(form.ID)
	if err != nil {
		switch {
		case models.IsErrLabelNotExist(err):
			ctx.Error(404)
		default:
			ctx.Handle(500, "UpdateLabel", err)
		}
		return
	}

	l.Name = form.Title
	l.Color = form.Color
	if err := models.UpdateLabel(l); err != nil {
		ctx.Handle(500, "UpdateLabel", err)
		return
	}
	ctx.Redirect(ctx.Repo.RepoLink + "/labels")
}

func DeleteLabel(ctx *middleware.Context) {
	if err := models.DeleteLabel(ctx.Repo.Repository.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteLabel: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.issues.label_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/labels",
	})
	return
}

func Milestones(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones")
	ctx.Data["PageIsMilestones"] = true

	isShowClosed := ctx.Query("state") == "closed"
	openCount, closedCount := models.MilestoneStats(ctx.Repo.Repository.ID)
	ctx.Data["OpenCount"] = openCount
	ctx.Data["ClosedCount"] = closedCount

	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var total int
	if !isShowClosed {
		total = int(openCount)
	} else {
		total = int(closedCount)
	}
	ctx.Data["Page"] = paginater.New(total, setting.IssuePagingNum, page, 5)

	miles, err := models.GetMilestones(ctx.Repo.Repository.ID, page, isShowClosed)
	if err != nil {
		ctx.Handle(500, "GetMilestones", err)
		return
	}
	for _, m := range miles {
		m.RenderedContent = string(base.RenderMarkdown([]byte(m.Content), ctx.Repo.RepoLink))
		m.CalOpenIssues()
	}
	ctx.Data["Milestones"] = miles

	if isShowClosed {
		ctx.Data["State"] = "closed"
	} else {
		ctx.Data["State"] = "open"
	}

	ctx.Data["IsShowClosed"] = isShowClosed
	ctx.HTML(200, MILESTONE)
}

func NewMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.new")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())
	ctx.HTML(200, MILESTONE_NEW)
}

func NewMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.new")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_NEW)
		return
	}

	if len(form.Deadline) == 0 {
		form.Deadline = "9999-12-31"
	}
	deadline, err := time.Parse("2006-01-02", form.Deadline)
	if err != nil {
		ctx.Data["Err_Deadline"] = true
		ctx.RenderWithErr(ctx.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &form)
		return
	}

	if err = models.NewMilestone(&models.Milestone{
		RepoID:   ctx.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Content,
		Deadline: deadline,
	}); err != nil {
		ctx.Handle(500, "NewMilestone", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.milestones.create_success", form.Title))
	ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
}

func EditMilestone(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.edit")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["PageIsEditMilestone"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", nil)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}
	ctx.Data["title"] = m.Name
	ctx.Data["content"] = m.Content
	if len(m.DeadlineString) > 0 {
		ctx.Data["deadline"] = m.DeadlineString
	}
	ctx.HTML(200, MILESTONE_NEW)
}

func EditMilestonePost(ctx *middleware.Context, form auth.CreateMilestoneForm) {
	ctx.Data["Title"] = ctx.Tr("repo.milestones.edit")
	ctx.Data["PageIsMilestones"] = true
	ctx.Data["PageIsEditMilestone"] = true
	ctx.Data["RequireDatetimepicker"] = true
	ctx.Data["DateLang"] = setting.DateLang(ctx.Locale.Language())

	if ctx.HasError() {
		ctx.HTML(200, MILESTONE_NEW)
		return
	}

	if len(form.Deadline) == 0 {
		form.Deadline = "9999-12-31"
	}
	deadline, err := time.Parse("2006-01-02", form.Deadline)
	if err != nil {
		ctx.Data["Err_Deadline"] = true
		ctx.RenderWithErr(ctx.Tr("repo.milestones.invalid_due_date_format"), MILESTONE_NEW, &form)
		return
	}

	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", nil)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}
	m.Name = form.Title
	m.Content = form.Content
	m.Deadline = deadline
	if err = models.UpdateMilestone(m); err != nil {
		ctx.Handle(500, "UpdateMilestone", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.milestones.edit_success", m.Name))
	ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
}

func ChangeMilestonStatus(ctx *middleware.Context) {
	m, err := models.GetMilestoneByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrMilestoneNotExist(err) {
			ctx.Handle(404, "GetMilestoneByID", err)
		} else {
			ctx.Handle(500, "GetMilestoneByID", err)
		}
		return
	}

	switch ctx.Params(":action") {
	case "open":
		if m.IsClosed {
			if err = models.ChangeMilestoneStatus(m, false); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones?state=open")
	case "close":
		if !m.IsClosed {
			m.ClosedDate = time.Now()
			if err = models.ChangeMilestoneStatus(m, true); err != nil {
				ctx.Handle(500, "ChangeMilestoneStatus", err)
				return
			}
		}
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones?state=closed")
	default:
		ctx.Redirect(ctx.Repo.RepoLink + "/milestones")
	}
}

func DeleteMilestone(ctx *middleware.Context) {
	if err := models.DeleteMilestoneByID(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteMilestone: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.milestones.deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/milestones",
	})
}

func PullRequest2(ctx *middleware.Context) {
	ctx.HTML(200, "repo/pr2/list")
}
