// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"encoding/json"
	"fmt"
	"net/http"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/lfsutil"
	"gogs.io/gogs/internal/strutil"
)

// POST /{owner}/{repo}.git/info/lfs/object/batch
func serveBatch(c *context.Context, owner *db.User, repo *db.Repository) {
	var request batchRequest
	defer c.Req.Request.Body.Close()
	err := json.NewDecoder(c.Req.Request.Body).Decode(&request)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": strutil.ToUpperFirst(err.Error()),
		})
		return
	}

	// NOTE: We only support basic transfer as of now.
	transfer := lfsutil.TransferBasic
	// Example: https://try.gogs.io/gogs/gogs.git/info/lfs/object/basic
	baseHref := fmt.Sprintf("%s%s/%s.git/info/lfs/objects/basic", conf.Server.ExternalURL, owner.Name, repo.Name)

	objects := make([]batchObject, 0, len(request.Objects))
	switch request.Operation {
	case lfsutil.BasicOperationUpload:
		for _, obj := range request.Objects {
			var actions batchActions
			if lfsutil.ValidOID(obj.Oid) {
				actions = batchActions{
					Upload: &batchAction{
						Href: fmt.Sprintf("%s/%s", baseHref, obj.Oid),
					},
					Verify: &batchAction{
						Href: fmt.Sprintf("%s/verify", baseHref),
					},
				}
			} else {
				actions = batchActions{
					Error: &batchError{
						Code:    http.StatusUnprocessableEntity,
						Message: "Object has invalid oid",
					},
				}
			}

			objects = append(objects, batchObject{
				Oid:     obj.Oid,
				Size:    obj.Size,
				Actions: actions,
			})
		}

	case lfsutil.BasicOperationDownload:
		oids := make([]string, 0, len(request.Objects))
		for _, obj := range request.Objects {
			oids = append(oids, obj.Oid)
		}
		stored, err := db.LFS.GetObjectsByOIDs(repo.ID, oids...)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			log.Error("Failed to get objects [repo_id: %d, oids: %v]: %v", repo.ID, oids, err)
			return
		}
		storedSet := make(map[string]*db.LFSObject, len(stored))
		for _, obj := range stored {
			storedSet[obj.OID] = obj
		}

		for _, obj := range request.Objects {
			var actions batchActions
			if stored := storedSet[obj.Oid]; stored != nil {
				if stored.Size != obj.Size {
					actions.Error = &batchError{
						Code:    http.StatusUnprocessableEntity,
						Message: "Object size mismatch",
					}
				} else {
					actions.Download = &batchAction{
						Href: fmt.Sprintf("%s/%s", baseHref, obj.Oid),
					}
				}
			} else {
				actions.Error = &batchError{
					Code:    http.StatusNotFound,
					Message: "Object does not exist",
				}
			}

			objects = append(objects, batchObject{
				Oid:     obj.Oid,
				Size:    obj.Size,
				Actions: actions,
			})
		}

	default:
		c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Operation not recognized",
		})
		return
	}

	c.JSONSuccess(batchResponse{
		Transfer: transfer,
		Objects:  objects,
	})
}

// batchRequest defines the request payload for the batch endpoint.
type batchRequest struct {
	Operation string `json:"operation"`
	Objects   []struct {
		Oid  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"objects"`
}

type batchError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type batchAction struct {
	Href string `json:"href"`
}

type batchActions struct {
	Download *batchAction `json:"download,omitempty"`
	Upload   *batchAction `json:"upload,omitempty"`
	Verify   *batchAction `json:"verify,omitempty"`
	Error    *batchError  `json:"error,omitempty"`
}

type batchObject struct {
	Oid     string       `json:"oid"`
	Size    int64        `json:"size"`
	Actions batchActions `json:"actions"`
}

// batchResponse defines the response payload for the batch endpoint.
type batchResponse struct {
	Transfer string        `json:"transfer"`
	Objects  []batchObject `json:"objects"`
}
