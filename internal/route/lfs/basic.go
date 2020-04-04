// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/strutil"
)

// GET /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func serveBasicDownload(c *context.Context, repo *db.Repository, oid lfsutil.OID) {
	object, err := db.LFS.GetObjectByOID(repo.ID, oid)
	if err != nil {
		if db.IsErrLFSObjectNotExist(err) {
			c.PlainText(http.StatusNotFound, "Object does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		}
		return
	}

	fpath := lfsutil.StorageLocalPath(conf.LFS.ObjectsPath, object.OID)
	r, err := os.Open(fpath)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		log.Error("Failed to open object file [path: %s]: %v", err)
		return
	}
	defer r.Close()

	c.Header().Set("Content-Type", "application/octet-stream")
	c.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))

	_, err = io.Copy(c.Resp, r)
	if err != nil {
		log.Error("Failed to copy object file: %v", err)
	}
}

// PUT /{owner}/{repo}.git/info/lfs/object/basic/{oid}
func serveBasicUpload(c *context.Context, repo *db.Repository, oid lfsutil.OID) {
	err := db.LFS.CreateObject(repo.ID, oid, c.Req.Request.Body, lfsutil.StorageLocal)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		log.Error("Failed to create object [repo_id: %d, oid: %s]: %v", repo.ID, oid, err)
		return
	}
	c.Status(http.StatusOK)

	log.Trace("[LFS] Object created %q", oid)
}

// POST /{owner}/{repo}.git/info/lfs/object/basic/verify
func serveBasicVerify(c *context.Context, repo *db.Repository) {
	var request basicVerifyRequest
	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": strutil.ToUpperFirst(err.Error()),
		})
		return
	}

	if !lfsutil.ValidOID(request.Oid) {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid oid",
		})
		return
	}

	object, err := db.LFS.GetObjectByOID(repo.ID, lfsutil.OID(request.Oid))
	if err != nil {
		if db.IsErrLFSObjectNotExist(err) {
			c.PlainText(http.StatusNotFound, "Object does not exist")
		} else {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get object [repo_id: %d, oid: %s]: %v", repo.ID, request.Oid, err)
		}
		return
	}

	if object.Size != request.Size {
		c.PlainText(http.StatusNotFound, "Object size mismatch")
		return
	}
	c.Status(http.StatusOK)
}

type basicVerifyRequest struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}
