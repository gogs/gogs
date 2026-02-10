package v1

import (
	"net/http"
	"strconv"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

type CreateLabelRequest struct {
	Name  string `json:"name" binding:"Required"`
	Color string `json:"color" binding:"Required;Size(7)"`
}

type EditLabelRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func ListLabels(c *context.APIContext) {
	labels, err := database.GetLabelsByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get labels by repository ID")
		return
	}

	apiLabels := make([]*types.IssueLabel, len(labels))
	for i := range labels {
		apiLabels[i] = ToIssueLabel(labels[i])
	}
	c.JSONSuccess(&apiLabels)
}

func GetLabel(c *context.APIContext) {
	var label *database.Label
	var err error
	idStr := c.Params(":id")
	if id, _ := strconv.ParseInt(idStr, 10, 64); id > 0 {
		label, err = database.GetLabelOfRepoByID(c.Repo.Repository.ID, id)
	} else {
		label, err = database.GetLabelOfRepoByName(c.Repo.Repository.ID, idStr)
	}
	if err != nil {
		c.NotFoundOrError(err, "get label")
		return
	}

	c.JSONSuccess(ToIssueLabel(label))
}

func CreateLabel(c *context.APIContext, form CreateLabelRequest) {
	label := &database.Label{
		Name:   form.Name,
		Color:  form.Color,
		RepoID: c.Repo.Repository.ID,
	}
	if err := database.NewLabels(label); err != nil {
		c.Error(err, "new labels")
		return
	}
	c.JSON(http.StatusCreated, ToIssueLabel(label))
}

func EditLabel(c *context.APIContext, form EditLabelRequest) {
	label, err := database.GetLabelOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get label of repository by ID")
		return
	}

	if form.Name != nil {
		label.Name = *form.Name
	}
	if form.Color != nil {
		label.Color = *form.Color
	}
	if err := database.UpdateLabel(label); err != nil {
		c.Error(err, "update label")
		return
	}
	c.JSONSuccess(ToIssueLabel(label))
}

func DeleteLabel(c *context.APIContext) {
	if err := database.DeleteLabel(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Error(err, "delete label")
		return
	}

	c.NoContent()
}
