package web

import (
	"cmp"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/markup"
)

// withIssueTracker lets through viewers who can read the repository, and
// everyone else when its issues are public, like the server-rendered labels
// and milestones pages.
func withIssueTracker(c flamego.Context, repoCtx *repoContext) {
	if !repoCtx.Viewer.CanRead() && !repoCtx.Repo.CanGuestViewIssues() {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// withRepoWriter lets through viewers who can write to the repository. Like
// the server-rendered pages, the repository is not found for others.
func withRepoWriter(c flamego.Context, repoCtx *repoContext) {
	if !repoCtx.Viewer.CanWrite() {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("repository does not exist"))
	}
}

// idParam returns the positive ID in the route, or 0.
func idParam(c flamego.Context) int64 {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	return max(id, 0)
}

// textFieldError returns the response for a required text field that is
// empty or longer than maxLen characters, or nil.
func textFieldError(l i18n.Locale, field, label, value string, maxLen int) *bindingErrorResponse {
	var msg string
	switch {
	case strings.TrimSpace(value) == "":
		msg = l.Tr(label) + l.Tr("form.require_error")
	case utf8.RuneCountInString(value) > maxLen:
		msg = l.Tr(label) + l.Tr("form.max_size_error", strconv.Itoa(maxLen))
	default:
		return nil
	}
	return &bindingErrorResponse{Fields: fieldErrors{field: &msg}}
}

type repoLabel struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Color          string `json:"color"`
	OpenIssueCount int    `json:"openIssueCount"`
}

type repoLabels struct {
	Items []repoLabel `json:"items"`
	// Templates are the predefined sets of labels, only sent to viewers who
	// can add them.
	Templates      []string `json:"templates,omitempty"`
	ViewerCanWrite bool     `json:"viewerCanWrite"`
}

func getRepoLabels(repoCtx *repoContext) (statusCode int, resp any, err error) {
	labels, err := database.GetLabelsByRepoID(repoCtx.Repo.ID)
	if err != nil {
		log.Error("getRepoLabels: list labels of repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list labels")
	}
	items := make([]repoLabel, 0, len(labels))
	for _, label := range labels {
		label.CalOpenIssues()
		items = append(items, repoLabel{ID: label.ID, Name: label.Name, Color: label.Color, OpenIssueCount: label.NumOpenIssues})
	}
	labelsResp := &repoLabels{Items: items, ViewerCanWrite: repoCtx.Viewer.CanWrite()}
	if labelsResp.ViewerCanWrite {
		labelsResp.Templates = database.LabelTemplates
	}
	return http.StatusOK, labelsResp, nil
}

type repoLabelRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

var labelColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (req repoLabelRequest) validate(l i18n.Locale) *bindingErrorResponse {
	if resp := textFieldError(l, "name", "repo.issues.label_title", req.Name, 50); resp != nil {
		return resp
	}
	if !labelColor.MatchString(req.Color) {
		msg := l.Tr("repo.issues.label_color") + l.Tr("form.hex_color_error")
		return &bindingErrorResponse{Fields: fieldErrors{"color": &msg}}
	}
	return nil
}

func postRepoLabel(l i18n.Locale, repoCtx *repoContext, req repoLabelRequest) (statusCode int, resp any, err error) {
	if resp := req.validate(l); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	label := &database.Label{RepoID: repoCtx.Repo.ID, Name: req.Name, Color: req.Color}
	if err = database.NewLabels(label); err != nil {
		log.Error("postRepoLabel: create label in repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create label")
	}
	return http.StatusCreated, nil, nil
}

// withRepoLabel injects the label of the repository named in the route.
func withRepoLabel(c flamego.Context, repoCtx *repoContext) {
	id := idParam(c)
	label, err := database.GetLabelOfRepoByID(repoCtx.Repo.ID, id)
	if err != nil {
		if database.IsErrLabelNotExist(err) {
			writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("label does not exist"))
			return
		}
		log.Error("withRepoLabel: get label %d: %v", id, err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get label"))
		return
	}
	c.Map(label)
}

func patchRepoLabel(l i18n.Locale, label *database.Label, req repoLabelRequest) (statusCode int, resp any, err error) {
	if resp := req.validate(l); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	label.Name = req.Name
	label.Color = req.Color
	if err = database.UpdateLabel(label); err != nil {
		log.Error("patchRepoLabel: update label %d: %v", label.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update label")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoLabel(repoCtx *repoContext, label *database.Label) (statusCode int, resp any, err error) {
	if err = database.DeleteLabel(repoCtx.Repo.ID, label.ID); err != nil {
		log.Error("deleteRepoLabel: delete label %d: %v", label.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete label")
	}
	return http.StatusNoContent, nil, nil
}

type repoLabelsInitializeRequest struct {
	Template string `json:"template"`
}

// postRepoLabelsInitialize adds a predefined set of labels.
func postRepoLabelsInitialize(l i18n.Locale, repoCtx *repoContext, req repoLabelsInitializeRequest) (statusCode int, resp any, err error) {
	if !slices.Contains(database.LabelTemplates, req.Template) {
		msg := l.Tr("repo.issues.label_templates.helper")
		return http.StatusBadRequest, &bindingErrorResponse{Fields: fieldErrors{"template": &msg}}, nil
	}
	list, err := database.GetLabelTemplateFile(req.Template)
	if err != nil {
		log.Error("postRepoLabelsInitialize: load label template %q: %v", req.Template, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "load label template")
	}
	labels := make([]*database.Label, len(list))
	for i := range list {
		labels[i] = &database.Label{RepoID: repoCtx.Repo.ID, Name: list[i][0], Color: list[i][1]}
	}
	if err = database.NewLabels(labels...); err != nil {
		log.Error("postRepoLabelsInitialize: create labels in repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create labels")
	}
	return http.StatusCreated, nil, nil
}

type repoMilestone struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	DescriptionHTML string `json:"descriptionHTML,omitempty"`
	// Deadline is a date, e.g., "2026-12-31", and is not sent when there is
	// none.
	Deadline         string     `json:"deadline,omitempty"`
	IsOverdue        bool       `json:"isOverdue"`
	IsClosed         bool       `json:"isClosed"`
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
	OpenIssueCount   int        `json:"openIssueCount"`
	ClosedIssueCount int        `json:"closedIssueCount"`
	Completeness     int        `json:"completeness"`
}

type repoMilestones struct {
	Items          []repoMilestone `json:"items"`
	Page           int             `json:"page"`
	TotalPages     int             `json:"totalPages"`
	OpenCount      int64           `json:"openCount"`
	ClosedCount    int64           `json:"closedCount"`
	ViewerCanWrite bool            `json:"viewerCanWrite"`
}

// getRepoMilestones lists the open milestones of the repository, or the
// closed ones with `state=closed`.
func getRepoMilestones(c flamego.Context, repoCtx *repoContext) (statusCode int, resp any, err error) {
	repo := repoCtx.Repo
	isClosed := c.Query("state") == "closed"
	openCount, closedCount := database.MilestoneStats(repo.ID)
	total := openCount
	if isClosed {
		total = closedCount
	}
	page := pageParam(c)
	miles, err := database.GetMilestones(repo.ID, page, isClosed)
	if err != nil {
		log.Error("getRepoMilestones: list milestones of repository %d: %v", repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list milestones")
	}

	items := make([]repoMilestone, 0, len(miles))
	for _, m := range miles {
		item := repoMilestone{
			ID:               m.ID,
			Name:             m.Name,
			Deadline:         m.DeadlineString,
			IsOverdue:        m.IsOverDue,
			IsClosed:         m.IsClosed,
			OpenIssueCount:   int(m.CountIssues(false, false)),
			ClosedIssueCount: int(m.CountIssues(true, false)),
		}
		if all := item.OpenIssueCount + item.ClosedIssueCount; all > 0 {
			item.Completeness = item.ClosedIssueCount * 100 / all
		}
		if m.Content != "" {
			item.DescriptionHTML = string(markup.Markdown(m.Content, repo.Link(), repo.ComposeMetas()))
		}
		if m.IsClosed {
			item.ClosedAt = &m.ClosedDate
		}
		items = append(items, item)
	}
	pageSize := int64(conf.UI.IssuePagingNum)
	return http.StatusOK, &repoMilestones{
		Items:          items,
		Page:           page,
		TotalPages:     int((total + pageSize - 1) / pageSize),
		OpenCount:      openCount,
		ClosedCount:    closedCount,
		ViewerCanWrite: repoCtx.Viewer.CanWrite(),
	}, nil
}

type repoMilestoneRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	// Deadline is a date, e.g., "2026-12-31", or empty for none.
	Deadline string `json:"deadline"`
}

// apply copies the request to the milestone, or returns the response for
// fields that cannot be used.
func (req repoMilestoneRequest) apply(l i18n.Locale, m *database.Milestone) *bindingErrorResponse {
	if resp := textFieldError(l, "title", "repo.milestones.title", req.Title, 50); resp != nil {
		return resp
	}
	// Milestones without a due date are stored as due on the last day of 9999.
	deadline := cmp.Or(req.Deadline, "9999-12-31")
	t, err := time.ParseInLocation("2006-01-02", deadline, time.Local)
	if err != nil {
		msg := l.Tr("repo.milestones.invalid_due_date_format")
		return &bindingErrorResponse{Fields: fieldErrors{"deadline": &msg}}
	}
	m.Name = req.Title
	m.Content = req.Description
	m.Deadline = t
	return nil
}

func postRepoMilestone(l i18n.Locale, repoCtx *repoContext, req repoMilestoneRequest) (statusCode int, resp any, err error) {
	m := &database.Milestone{RepoID: repoCtx.Repo.ID}
	if resp := req.apply(l, m); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if err = database.NewMilestone(m); err != nil {
		log.Error("postRepoMilestone: create milestone in repository %d: %v", repoCtx.Repo.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create milestone")
	}
	return http.StatusCreated, nil, nil
}

// withRepoMilestone injects the milestone of the repository named in the
// route.
func withRepoMilestone(c flamego.Context, repoCtx *repoContext) {
	id := idParam(c)
	// An ID of 0 would match any milestone of the repository.
	if id == 0 {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("milestone does not exist"))
		return
	}
	m, err := database.GetMilestoneByRepoID(repoCtx.Repo.ID, id)
	if err != nil {
		if database.IsErrMilestoneNotExist(err) {
			writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("milestone does not exist"))
			return
		}
		log.Error("withRepoMilestone: get milestone %d: %v", id, err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get milestone"))
		return
	}
	c.Map(m)
}

func getRepoMilestone(m *database.Milestone) (statusCode int, resp any, err error) {
	return http.StatusOK, &repoMilestoneRequest{Title: m.Name, Description: m.Content, Deadline: m.DeadlineString}, nil
}

func patchRepoMilestone(l i18n.Locale, m *database.Milestone, req repoMilestoneRequest) (statusCode int, resp any, err error) {
	if resp := req.apply(l, m); resp != nil {
		return http.StatusBadRequest, resp, nil
	}
	if err = database.UpdateMilestone(m); err != nil {
		log.Error("patchRepoMilestone: update milestone %d: %v", m.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update milestone")
	}
	return http.StatusNoContent, nil, nil
}

// putRepoMilestoneClosed closes the milestone.
func putRepoMilestoneClosed(m *database.Milestone) (statusCode int, resp any, err error) {
	if m.IsClosed {
		return http.StatusNoContent, nil, nil
	}
	m.ClosedDate = time.Now()
	if err = database.ChangeMilestoneStatus(m, true); err != nil {
		log.Error("putRepoMilestoneClosed: close milestone %d: %v", m.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "close milestone")
	}
	return http.StatusNoContent, nil, nil
}

// deleteRepoMilestoneClosed reopens the milestone.
func deleteRepoMilestoneClosed(m *database.Milestone) (statusCode int, resp any, err error) {
	if !m.IsClosed {
		return http.StatusNoContent, nil, nil
	}
	if err = database.ChangeMilestoneStatus(m, false); err != nil {
		log.Error("deleteRepoMilestoneClosed: reopen milestone %d: %v", m.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "reopen milestone")
	}
	return http.StatusNoContent, nil, nil
}

func deleteRepoMilestone(repoCtx *repoContext, m *database.Milestone) (statusCode int, resp any, err error) {
	if err = database.DeleteMilestoneOfRepoByID(repoCtx.Repo.ID, m.ID); err != nil {
		log.Error("deleteRepoMilestone: delete milestone %d: %v", m.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete milestone")
	}
	return http.StatusNoContent, nil, nil
}
