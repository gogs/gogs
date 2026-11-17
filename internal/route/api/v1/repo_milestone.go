package v1

import (
	"net/http"
	"time"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listMilestones(c *apiContext) {
	milestones, err := database.Handle.Milestones().ListByRepoID(c.Request().Context(), c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get milestones by repository ID")
		return
	}

	apiMilestones := make([]*types.IssueMilestone, len(milestones))
	for i := range milestones {
		apiMilestones[i] = toIssueMilestone(milestones[i])
	}
	c.JSONSuccess(&apiMilestones)
}

func getMilestone(c *apiContext) {
	milestone, err := database.Handle.Milestones().GetByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}
	c.JSONSuccess(toIssueMilestone(milestone))
}

type createMilestoneRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Deadline    *time.Time `json:"due_on"`
}

func createMilestone(c *apiContext, form createMilestoneRequest) {
	if form.Deadline == nil {
		defaultDeadline, _ := time.ParseInLocation("2006-01-02", "9999-12-31", time.Local)
		form.Deadline = &defaultDeadline
	}

	milestone := &database.Milestone{
		RepoID:   c.Repo.Repository.ID,
		Name:     form.Title,
		Content:  form.Description,
		Deadline: *form.Deadline,
	}

	if err := database.Handle.Milestones().Create(c.Request().Context(), milestone); err != nil {
		c.Error(err, "new milestone")
		return
	}
	c.JSON(http.StatusCreated, toIssueMilestone(milestone))
}

type editMilestoneRequest struct {
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	State       *string    `json:"state"`
	Deadline    *time.Time `json:"due_on"`
}

func editMilestone(c *apiContext, form editMilestoneRequest) {
	milestone, err := database.Handle.Milestones().GetByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id"))
	if err != nil {
		c.NotFoundOrError(err, "get milestone by repository ID")
		return
	}

	if len(form.Title) > 0 {
		milestone.Name = form.Title
	}
	if form.Description != nil {
		milestone.Content = *form.Description
	}
	if form.Deadline != nil && !form.Deadline.IsZero() {
		milestone.Deadline = *form.Deadline
	}

	if form.State != nil {
		if err = database.Handle.Milestones().ChangeStatus(c.Request().Context(), milestone, types.IssueStateClosed == types.IssueStateType(*form.State)); err != nil {
			c.Error(err, "change status")
			return
		}
	} else if err = database.Handle.Milestones().Update(c.Request().Context(), milestone); err != nil {
		c.Error(err, "update milestone")
		return
	}

	c.JSONSuccess(toIssueMilestone(milestone))
}

func deleteMilestone(c *apiContext) {
	if err := database.Handle.Milestones().DeleteByID(c.Request().Context(), c.Repo.Repository.ID, c.ParamInt64("id")); err != nil {
		c.Error(err, "delete milestone of repository by ID")
		return
	}
	c.NoContent()
}
