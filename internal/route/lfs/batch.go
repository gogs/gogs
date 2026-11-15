package lfs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/flamego/flamego"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsx"
	"gogs.io/gogs/internal/strx"
)

// POST /{owner}/{repo}.git/info/lfs/objects/batch
func serveBatch(logger *logx.Logger, store Store) flamego.Handler {
	return func(c flamego.Context, owner *database.User, repo *database.Repository, token *lfsx.Token) {
		var request batchRequest
		defer func() { _ = c.Request().Request.Body.Close() }()
		err := json.NewDecoder(c.Request().Request.Body).Decode(&request)
		if err != nil {
			responseJSON(logger, c.ResponseWriter(), http.StatusBadRequest, responseError{
				Message: strx.ToUpperFirst(err.Error()),
			})
			return
		}

		// NOTE: Only the basic transfer is supported.
		transfer := transferBasic
		// Example: https://try.gogs.io/gogs/gogs.git/info/lfs/objects/basic
		baseHref := fmt.Sprintf("%s%s/%s.git/info/lfs/objects/basic", conf.Server.ExternalURL, owner.Name, repo.Name)

		// Git LFS sends the token it got over SSH only to the batch API, so the
		// actions carry it to the transfers.
		withToken := func(header map[string]string) map[string]string {
			if token == nil {
				return header
			}
			if header == nil {
				header = make(map[string]string, 1)
			}
			header["Authorization"] = c.Request().Header.Get("Authorization")
			return header
		}

		objects := make([]batchObject, 0, len(request.Objects))
		switch request.Operation {
		case basicOperationUpload:
			for _, obj := range request.Objects {
				var actions batchActions
				if lfsx.ValidOID(obj.Oid) {
					actions = batchActions{
						Upload: &batchAction{
							Href: fmt.Sprintf("%s/%s", baseHref, obj.Oid),
							Header: withToken(map[string]string{
								// NOTE: git-lfs v2.5.0 sets the Content-Type based on the uploaded file.
								// This ensures that the client always uses the designated value for the header.
								"Content-Type": "application/octet-stream",
							}),
						},
						Verify: &batchAction{
							Href:   fmt.Sprintf("%s/verify", baseHref),
							Header: withToken(nil),
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

		case basicOperationDownload:
			oids := make([]lfsx.OID, 0, len(request.Objects))
			for _, obj := range request.Objects {
				oids = append(oids, obj.Oid)
			}
			stored, err := store.GetLFSObjectsByOIDs(c.Request().Context(), repo.ID, oids...)
			if err != nil {
				internalServerError(logger, c.ResponseWriter())
				logger.Error("Failed to get objects", "repoID", repo.ID, "oids", oids, "error", err)
				return
			}
			storedSet := make(map[lfsx.OID]*database.LFSObject, len(stored))
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
							Href:   fmt.Sprintf("%s/%s", baseHref, obj.Oid),
							Header: withToken(nil),
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
			responseJSON(logger, c.ResponseWriter(), http.StatusBadRequest, responseError{
				Message: "Operation not recognized",
			})
			return
		}

		responseJSON(logger, c.ResponseWriter(), http.StatusOK, batchResponse{
			Transfer: transfer,
			Objects:  objects,
		})
	}
}

// batchRequest defines the request payload for the batch endpoint.
type batchRequest struct {
	Operation string `json:"operation"`
	Objects   []struct {
		Oid  lfsx.OID `json:"oid"`
		Size int64    `json:"size"`
	} `json:"objects"`
}

type batchError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type batchAction struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header,omitempty"`
}

type batchActions struct {
	Download *batchAction `json:"download,omitempty"`
	Upload   *batchAction `json:"upload,omitempty"`
	Verify   *batchAction `json:"verify,omitempty"`
	Error    *batchError  `json:"error,omitempty"`
}

type batchObject struct {
	Oid     lfsx.OID     `json:"oid"`
	Size    int64        `json:"size"`
	Actions batchActions `json:"actions"`
}

// batchResponse defines the response payload for the batch endpoint.
type batchResponse struct {
	Transfer string        `json:"transfer"`
	Objects  []batchObject `json:"objects"`
}

type responseError struct {
	Message string `json:"message"`
}

const contentType = "application/vnd.git-lfs+json"

func responseJSON(logger *logx.Logger, w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		logger.Error("Failed to encode JSON", "error", err)
		return
	}
}
