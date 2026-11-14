package lfs

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"gopkg.in/macaron.v1"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsx"
	"gogs.io/gogs/internal/strx"
)

const transferBasic = "basic"
const (
	basicOperationUpload   = "upload"
	basicOperationDownload = "download"
)

type basicHandler struct {
	logger *logx.Logger
	store  Store
	// The default storage backend for uploading new objects.
	defaultStorage lfsx.Storage
	// The list of available storage backends to access objects.
	storagers map[lfsx.Storage]lfsx.Storager
}

// DefaultStorager returns the default storage backend.
func (h *basicHandler) DefaultStorager() lfsx.Storager {
	return h.storagers[h.defaultStorage]
}

// Storager returns the given storage backend.
func (h *basicHandler) Storager(storage lfsx.Storage) lfsx.Storager {
	return h.storagers[storage]
}

// GET /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func (h *basicHandler) serveDownload(c *macaron.Context, repo *database.Repository, oid lfsx.OID) {
	object, err := h.store.GetLFSObjectByOID(c.Req.Context(), repo.ID, oid)
	if err != nil {
		if database.IsErrLFSObjectNotExist(err) {
			responseJSON(h.logger, c.Resp, http.StatusNotFound, responseError{
				Message: "Object does not exist",
			})
		} else {
			internalServerError(h.logger, c.Resp)
			h.logger.Error("Failed to get object", "repoID", repo.ID, "oid", oid, "error", err)
		}
		return
	}

	s := h.Storager(object.Storage)
	if s == nil {
		internalServerError(h.logger, c.Resp)
		h.logger.Error("Object storage not found", "repoID", object.RepoID, "oid", object.OID, "storage", object.Storage)
		return
	}

	c.Header().Set("Content-Type", "application/octet-stream")
	c.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	c.Status(http.StatusOK)

	err = s.Download(object.OID, c.Resp)
	if err != nil {
		h.logger.Error("Failed to download object", "oid", object.OID, "error", err)
		return
	}
}

// PUT /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func (h *basicHandler) serveUpload(c *macaron.Context, repo *database.Repository, oid lfsx.OID) {
	// NOTE: LFS client will retry upload the same object if there was a partial failure,
	// therefore we would like to skip ones that already exist.
	_, err := h.store.GetLFSObjectByOID(c.Req.Context(), repo.ID, oid)
	if err == nil {
		// Object exists, drain the request body and we're good.
		_, _ = io.Copy(io.Discard, c.Req.Request.Body)
		c.Req.Request.Body.Close()
		c.Status(http.StatusOK)
		return
	} else if !database.IsErrLFSObjectNotExist(err) {
		internalServerError(h.logger, c.Resp)
		h.logger.Error("Failed to get object", "repoID", repo.ID, "oid", oid, "error", err)
		return
	}

	s := h.DefaultStorager()
	written, err := s.Upload(oid, c.Req.Request.Body)
	if err != nil {
		if err == lfsx.ErrInvalidOID || err == lfsx.ErrOIDMismatch {
			responseJSON(h.logger, c.Resp, http.StatusBadRequest, responseError{
				Message: err.Error(),
			})
		} else {
			internalServerError(h.logger, c.Resp)
			h.logger.Error("Failed to upload object", "storage", s.Storage(), "oid", oid, "error", err)
		}
		return
	}

	err = h.store.CreateLFSObject(c.Req.Context(), repo.ID, oid, written, s.Storage())
	if err != nil {
		// NOTE: It is OK to leave the file when the whole operation failed
		// with a DB error, a retry on client side will skip the upload as
		// the file already exists on disk.
		internalServerError(h.logger, c.Resp)
		h.logger.Error("Failed to create object", "repoID", repo.ID, "oid", oid, "error", err)
		return
	}
	c.Status(http.StatusOK)

	h.logger.Debug("Object created", "oid", oid)
}

// POST /{owner}/{repo}.git/info/lfs/object/basic/verify
func (h *basicHandler) serveVerify(c *macaron.Context, repo *database.Repository) {
	var request basicVerifyRequest
	defer func() { _ = c.Req.Request.Body.Close() }()

	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		responseJSON(h.logger, c.Resp, http.StatusBadRequest, responseError{
			Message: strx.ToUpperFirst(err.Error()),
		})
		return
	}

	if !lfsx.ValidOID(request.Oid) {
		responseJSON(h.logger, c.Resp, http.StatusBadRequest, responseError{
			Message: "Invalid oid",
		})
		return
	}

	object, err := h.store.GetLFSObjectByOID(c.Req.Context(), repo.ID, request.Oid)
	if err != nil {
		if database.IsErrLFSObjectNotExist(err) {
			responseJSON(h.logger, c.Resp, http.StatusNotFound, responseError{
				Message: "Object does not exist",
			})
		} else {
			internalServerError(h.logger, c.Resp)
			h.logger.Error("Failed to get object", "repoID", repo.ID, "oid", request.Oid, "error", err)
		}
		return
	}

	if object.Size != request.Size {
		responseJSON(h.logger, c.Resp, http.StatusBadRequest, responseError{
			Message: "Object size mismatch",
		})
		return
	}
	c.Status(http.StatusOK)
}

type basicVerifyRequest struct {
	Oid  lfsx.OID `json:"oid"`
	Size int64    `json:"size"`
}
