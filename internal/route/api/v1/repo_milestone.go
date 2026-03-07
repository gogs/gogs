package v1

import (
	"net/http"
	"time"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listMilestones(c *context.APIContext) {
	milestones, err := database.GetMilestonesByRepoID(c.Repo.Repository.ID)
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

func getMilestone(c *context.APIContext) {
	milestone, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
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

func createMilestone(c *context.APIContext, form createMilestoneRequest) {
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

	if err := database.NewMilestone(milestone); err != nil {
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

func editMilestone(c *context.APIContext, form editMilestoneRequest) {
	milestone, err := database.GetMilestoneByRepoID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
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
		if err = milestone.ChangeStatus(types.IssueStateClosed == types.IssueStateType(*form.State)); err != nil {
			c.Error(err, "change status")
			return
		}
	} else if err = database.UpdateMilestone(milestone); err != nil {
		c.Error(err, "update milestone")
		return
	}

	c.JSONSuccess(toIssueMilestone(milestone))
}

func deleteMilestone(c *context.APIContext) {
	if err := database.DeleteMilestoneOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Error(err, "delete milestone of repository by ID")
		return
	}
	c.NoContent()
}
