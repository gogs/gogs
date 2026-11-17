package v1

import (
	"context"
	"net/http"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func listCollaborators(c *apiContext) {
	collaborators, err := database.Handle.Collaborations().List(c.Request().Context(), c.Repo.Repository.ID)
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

	// Keep going if the client disconnects, so the collaborator does not end
	// up added without the requested mode.
	ctx := context.WithoutCancel(c.Request().Context())
	if err := database.Handle.Collaborations().Add(ctx, c.Repo.Repository, collaborator.ID); err != nil {
		c.Error(err, "add collaborator")
		return
	}

	if form.Permission != nil {
		if err := database.Handle.Collaborations().ChangeMode(ctx, c.Repo.Repository, c.Repo.AccessMode, collaborator.ID, database.ParseAccessMode(*form.Permission)); err != nil {
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

	if !database.Handle.Collaborations().IsCollaborator(c.Request().Context(), c.Logger, c.Repo.Repository.ID, collaborator.ID) {
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

	if err := database.Handle.Collaborations().Delete(c.Request().Context(), c.Repo.Repository, collaborator.ID); err != nil {
		c.Error(err, "delete collaboration")
		return
	}

	c.NoContent()
}
