// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/strutil"
)

const transferBasic = "basic"
const (
	basicOperationUpload   = "upload"
	basicOperationDownload = "download"
)

type basicHandler struct {
	// The default storage backend for uploading new objects.
	defaultStorage lfsutil.Storage
	// The list of available storage backends to access objects.
	storagers map[lfsutil.Storage]lfsutil.Storager
}

// DefaultStorager returns the default storage backend.
func (h *basicHandler) DefaultStorager() lfsutil.Storager {
	return h.storagers[h.defaultStorage]
}

// Storager returns the given storage backend.
func (h *basicHandler) Storager(storage lfsutil.Storage) lfsutil.Storager {
	return h.storagers[storage]
}

// GET /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func (h *basicHandler) serveDownload(c *macaron.Context, repo *db.Repository, oid lfsutil.OID) {
	object, err := db.LFS.GetObjectByOID(c.Req.Context(), repo.ID, oid)
	if err != nil {
		if db.IsErrLFSObjectNotExist(err) {
			responseJSON(c.Resp, http.StatusNotFound, responseError{
				Message: "Object does not exist",
			})
		} else {
			internalServerError(c.Resp)
			log.Error("Failed to get object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		}
		return
	}

	s := h.Storager(object.Storage)
	if s == nil {
		internalServerError(c.Resp)
		log.Error("Failed to locate the object [repo_id: %d, oid: %s]: storage %q not found", object.RepoID, object.OID, object.Storage)
		return
	}

	c.Header().Set("Content-Type", "application/octet-stream")
	c.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	c.Status(http.StatusOK)

	err = s.Download(object.OID, c.Resp)
	if err != nil {
		log.Error("Failed to download object [oid: %s]: %v", object.OID, err)
		return
	}
}

// PUT /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func (h *basicHandler) serveUpload(c *macaron.Context, repo *db.Repository, oid lfsutil.OID) {
	// NOTE: LFS client will retry upload the same object if there was a partial failure,
	// therefore we would like to skip ones that already exist.
	_, err := db.LFS.GetObjectByOID(c.Req.Context(), repo.ID, oid)
	if err == nil {
		// Object exists, drain the request body and we're good.
		_, _ = io.Copy(ioutil.Discard, c.Req.Request.Body)
		c.Req.Request.Body.Close()
		c.Status(http.StatusOK)
		return
	} else if !db.IsErrLFSObjectNotExist(err) {
		internalServerError(c.Resp)
		log.Error("Failed to get object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		return
	}

	s := h.DefaultStorager()
	written, err := s.Upload(oid, c.Req.Request.Body)
	if err != nil {
		if err == lfsutil.ErrInvalidOID {
			responseJSON(c.Resp, http.StatusBadRequest, responseError{
				Message: err.Error(),
			})
		} else {
			internalServerError(c.Resp)
			log.Error("Failed to upload object [storage: %s, oid: %s]: %v", s.Storage(), oid, err)
		}
		return
	}

	err = db.LFS.CreateObject(c.Req.Context(), repo.ID, oid, written, s.Storage())
	if err != nil {
		// NOTE: It is OK to leave the file when the whole operation failed
		// with a DB error, a retry on client side can safely overwrite the
		// same file as OID is seen as unique to every file.
		internalServerError(c.Resp)
		log.Error("Failed to create object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		return
	}
	c.Status(http.StatusOK)

	log.Trace("[LFS] Object created %q", oid)
}

// POST /{owner}/{repo}.git/info/lfs/object/basic/verify
func (*basicHandler) serveVerify(c *macaron.Context, repo *db.Repository) {
	var request basicVerifyRequest
	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		responseJSON(c.Resp, http.StatusBadRequest, responseError{
			Message: strutil.ToUpperFirst(err.Error()),
		})
		return
	}

	if !lfsutil.ValidOID(request.Oid) {
		responseJSON(c.Resp, http.StatusBadRequest, responseError{
			Message: "Invalid oid",
		})
		return
	}

	object, err := db.LFS.GetObjectByOID(c.Req.Context(), repo.ID, request.Oid)
	if err != nil {
		if db.IsErrLFSObjectNotExist(err) {
			responseJSON(c.Resp, http.StatusNotFound, responseError{
				Message: "Object does not exist",
			})
		} else {
			internalServerError(c.Resp)
			log.Error("Failed to get object [repo_id: %d, oid: %s]: %v", repo.ID, request.Oid, err)
		}
		return
	}

	if object.Size != request.Size {
		responseJSON(c.Resp, http.StatusBadRequest, responseError{
			Message: "Object size mismatch",
		})
		return
	}
	c.Status(http.StatusOK)
}

type basicVerifyRequest struct {
	Oid  lfsutil.OID `json:"oid"`
	Size int64       `json:"size"`
}
