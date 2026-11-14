package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/tool"
)

var (
	ErrFileTypeForbidden = errors.New("file type is not allowed")
	ErrTooManyFiles      = errors.New("maximum number of files to upload exceeded")
)

func MustEnableIssues(c *context.Context) {
	if !c.Repo.Repository.EnableIssues {
		c.NotFound()
		return
	}

	if c.Repo.Repository.EnableExternalTracker {
		c.Redirect(c.Repo.Repository.ExternalTrackerURL)
		return
	}
}

func MustAllowPulls(c *context.Context) {
	if !c.Repo.Repository.AllowsPulls() {
		c.NotFound()
		return
	}

	// User can send pull request if owns a forked repository.
	if c.IsLogged && database.Handle.Repositories().HasForkedBy(c.Req.Context(), c.Repo.Repository.ID, c.User.ID) {
		c.Repo.PullRequest.Allowed = true
		c.Repo.PullRequest.HeadInfo = c.User.Name + ":" + c.Repo.BranchName
	}
}

// ServeIssueList serves the issue or pull request list of a repository. Like
// the server-rendered page did, it sends signed-out visitors to sign in for the
// lists about the viewer.
func ServeIssueList(c *context.Context) {
	if slices.Contains([]string{"assigned", "created_by", "mentioned"}, c.Query("type")) && !c.IsLogged {
		c.RawRedirect(conf.Server.Subpath + "/user/sign-in?redirect_to=" + url.QueryEscape(conf.Server.Subpath+c.Req.URL.RequestURI()))
		return
	}
	c.ServeWeb()
}

func renderAttachmentSettings(c *context.Context) {
	c.Data["RequireDropzone"] = true
	c.Data["IsAttachmentEnabled"] = conf.Attachment.Enabled
	c.Data["AttachmentAllowedTypes"] = conf.Attachment.AllowedTypes
	c.Data["AttachmentMaxSize"] = conf.Attachment.MaxSize
	c.Data["AttachmentMaxFiles"] = conf.Attachment.MaxFiles
}

func RetrieveRepoMilestonesAndAssignees(c *context.Context, repo *database.Repository) {
	var err error
	c.Data["OpenMilestones"], err = database.GetMilestones(repo.ID, -1, false)
	if err != nil {
		c.Error(err, "get open milestones")
		return
	}
	c.Data["ClosedMilestones"], err = database.GetMilestones(repo.ID, -1, true)
	if err != nil {
		c.Error(err, "get closed milestones")
		return
	}

	c.Data["Assignees"], err = repo.GetAssignees()
	if err != nil {
		c.Error(err, "get assignees")
		return
	}
}

func RetrieveRepoMetas(c *context.Context, repo *database.Repository) []*database.Label {
	if !c.Repo.IsWriter() {
		return nil
	}

	labels, err := database.GetLabelsByRepoID(repo.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return nil
	}
	c.Data["Labels"] = labels

	RetrieveRepoMilestonesAndAssignees(c, repo)
	if c.Written() {
		return nil
	}

	return labels
}

func getFileContentFromDefaultBranch(c *context.Context, filename string) (string, bool) {
	if c.Repo.Commit == nil {
		var err error
		c.Repo.Commit, err = c.Repo.GitRepo.BranchCommit(c.Req.Context(), c.Repo.Repository.DefaultBranch)
		if err != nil {
			return "", false
		}
	}

	entry, err := c.Repo.Commit.TreeEntry(c.Req.Context(), filename)
	if err != nil {
		return "", false
	}
	p, err := entry.Blob().Bytes(c.Req.Context())
	if err != nil {
		return "", false
	}
	return string(p), true
}

func setTemplateIfExists(c *context.Context, ctxDataKey string, possibleFiles []string) {
	for _, filename := range possibleFiles {
		content, found := getFileContentFromDefaultBranch(c, filename)
		if found {
			c.Data[ctxDataKey] = content
			return
		}
	}
}

func ValidateRepoMetas(c *context.Context, f form.NewIssue) ([]int64, int64, int64) {
	var (
		repo = c.Repo.Repository
		err  error
	)

	labels := RetrieveRepoMetas(c, c.Repo.Repository)
	if c.Written() {
		return nil, 0, 0
	}

	if !c.Repo.IsWriter() {
		return nil, 0, 0
	}

	// Check labels.
	labelIDs := tool.StringsToInt64s(strings.Split(f.LabelIDs, ","))
	labelIDMark := tool.Int64sToMap(labelIDs)
	hasSelected := false
	for i := range labels {
		if labelIDMark[labels[i].ID] {
			labels[i].IsChecked = true
			hasSelected = true
		}
	}
	c.Data["HasSelectedLabel"] = hasSelected
	c.Data["label_ids"] = f.LabelIDs
	c.Data["Labels"] = labels

	// Check milestone.
	milestoneID := f.MilestoneID
	if milestoneID > 0 {
		c.Data["Milestone"], err = repo.GetMilestoneByID(milestoneID)
		if err != nil {
			c.Error(err, "get milestone by ID")
			return nil, 0, 0
		}
		c.Data["milestone_id"] = milestoneID
	}

	// Check assignee.
	assigneeID := f.AssigneeID
	if assigneeID > 0 {
		c.Data["Assignee"], err = repo.GetAssigneeByID(assigneeID)
		if err != nil {
			c.Error(err, "get assignee by ID")
			return nil, 0, 0
		}
		c.Data["assignee_id"] = assigneeID
	}

	return labelIDs, milestoneID, assigneeID
}

func uploadAttachment(c *context.Context, allowedTypes []string) {
	file, header, err := c.Req.FormFile("file")
	if err != nil {
		c.Error(err, "get file")
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
		c.PlainText(http.StatusBadRequest, ErrFileTypeForbidden.Error())
		return
	}

	attach, err := database.NewAttachment(header.Filename, buf, file)
	if err != nil {
		c.Error(err, "new attachment")
		return
	}

	log.Trace("New attachment uploaded: %s", attach.UUID)
	c.JSONSuccess(map[string]string{
		"uuid": attach.UUID,
	})
}

func UploadIssueAttachment(c *context.Context) {
	if !conf.Attachment.Enabled {
		c.NotFound()
		return
	}

	uploadAttachment(c, conf.Attachment.AllowedTypes)
}

// ServeIssue serves the page of an issue, and sends pull requests to their
// own page.
func ServeIssue(c *context.Context) {
	index := c.ParamsInt64(":index")
	if index <= 0 {
		c.NotFound()
		return
	}
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	if issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("pulls/%d", issue.Index)))
		return
	}
	MustEnableIssues(c)
	if c.Written() {
		return
	}
	c.ServeWeb()
}

// ServePull serves the page of a pull request, and sends issues to their own
// page.
func ServePull(c *context.Context) {
	index := c.ParamsInt64(":index")
	if index <= 0 {
		c.NotFound()
		return
	}
	issue, err := database.GetIssueByIndex(c.Repo.Repository.ID, index)
	if err != nil {
		c.NotFoundOrError(err, "get issue by index")
		return
	}
	if !issue.IsPull {
		c.RawRedirect(c.Repo.MakeURL(fmt.Sprintf("issues/%d", issue.Index)))
		return
	}
	MustAllowPulls(c)
	if c.Written() {
		return
	}
	c.ServeWeb()
}
