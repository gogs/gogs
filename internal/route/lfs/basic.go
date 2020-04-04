// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/strutil"
)

const transferBasic = "basic"
const (
	basicOperationUpload   = "upload"
	basicOperationDownload = "download"
)

// GET /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func serveBasicDownload(c *macaron.Context, repo *db.Repository, oid lfsutil.OID) {
	object, err := db.LFS.GetObjectByOID(repo.ID, oid)
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

	fpath := lfsutil.StorageLocalPath(conf.LFS.ObjectsPath, object.OID)
	r, err := os.Open(fpath)
	if err != nil {
		internalServerError(c.Resp)
		log.Error("Failed to open object file [path: %s]: %v", fpath, err)
		return
	}
	defer r.Close()

	c.Header().Set("Content-Type", "application/octet-stream")
	c.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	c.Status(http.StatusOK)

	_, err = io.Copy(c.Resp, r)
	if err != nil {
		log.Error("Failed to copy object file: %v", err)
		return
	}
}

// PUT /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func serveBasicUpload(c *macaron.Context, repo *db.Repository, oid lfsutil.OID) {
	// NOTE: LFS client will retry upload the same object if there was a partial failure,
	// therefore we would like to skip ones that already exist.
	_, err := db.LFS.GetObjectByOID(repo.ID, oid)
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

	err = db.LFS.CreateObject(repo.ID, oid, c.Req.Request.Body, lfsutil.StorageLocal)
	if err != nil {
		internalServerError(c.Resp)
		log.Error("Failed to create object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		return
	}
	c.Status(http.StatusOK)

	log.Trace("[LFS] Object created %q", oid)
}

// POST /{owner}/{repo}.git/info/lfs/object/basic/verify
func serveBasicVerify(c *macaron.Context, repo *db.Repository) {
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

	object, err := db.LFS.GetObjectByOID(repo.ID, lfsutil.OID(request.Oid))
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
		responseJSON(c.Resp, http.StatusNotFound, responseError{
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
