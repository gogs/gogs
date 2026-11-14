package v1

import (
	"net/http"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listCollaborators(c *apiContext) {
	collaborators, err := c.Repo.Repository.GetCollaborators()
	if err != nil {
		c.Error(err, "get collaborators")
		return
	}

	apiCollaborators := make([]*types.RepositoryCollaborator, len(collaborators))
	for i := range collaborators {
		apiCollaborators[i] = toRepositoryCollaborator(collaborators[i])
	}
	c.JSONSuccess(&apiCollaborators)
}

type addCollaboratorRequest struct {
	Permission *string `json:"permission"`
}

func addCollaborator(c *apiContext, form addCollaboratorRequest) {
	collaborator, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("collaborator"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Status(http.StatusUnprocessableEntity)
		} else {
			c.Error(err, "get user by name")
		}
		return
	}

	if err := c.Repo.Repository.AddCollaborator(collaborator); err != nil {
		c.Error(err, "add collaborator")
		return
	}

	if form.Permission != nil {
		if err := c.Repo.Repository.ChangeCollaborationAccessMode(c.Repo.AccessMode, collaborator.ID, database.ParseAccessMode(*form.Permission)); err != nil {
			c.Error(err, "change collaboration access mode")
			return
		}
	}

	c.NoContent()
}

func isCollaborator(c *apiContext) {
	collaborator, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("collaborator"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Status(http.StatusUnprocessableEntity)
		} else {
			c.Error(err, "get user by name")
		}
		return
	}

	if !c.Repo.Repository.IsCollaborator(collaborator.ID) {
		c.NotFound()
	} else {
		c.NoContent()
	}
}

func deleteCollaborator(c *apiContext) {
	collaborator, err := database.Handle.Users().GetByUsername(c.Request().Context(), c.Param("collaborator"))
	if err != nil {
		if database.IsErrUserNotExist(err) {
			c.Status(http.StatusUnprocessableEntity)
		} else {
			c.Error(err, "get user by name")
		}
		return
	}

	if err := c.Repo.Repository.DeleteCollaboration(collaborator.ID); err != nil {
		c.Error(err, "delete collaboration")
		return
	}

	c.NoContent()
}
