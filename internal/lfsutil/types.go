// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

const ContentType = "application/vnd.git-lfs+json"
const TransferBasic = "basic"
const (
	BasicOperationUpload   = "upload"
	BasicOperationDownload = "download"
)

// Storage is the storage type of an LFS object.
type Storage string

const (
	StorageLocal Storage = "local"
)

// BatchRequest defines the request payload for the batch endpoint.
type BatchRequest struct {
	Operation string `json:"operation"`
	Objects   []struct {
		Oid  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"objects"`
}

type BatchError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type BatchAction struct {
	Href string `json:"href"`
}

type BatchActions struct {
	Download *BatchAction `json:"download,omitempty"`
	Upload   *BatchAction `json:"upload,omitempty"`
	Verify   *BatchAction `json:"verify,omitempty"`
	Error    *BatchError  `json:"error,omitempty"`
}

type BatchObject struct {
	Oid     string       `json:"oid"`
	Size    int64        `json:"size"`
	Actions BatchActions `json:"actions"`
}

// BatchResponse defines the response payload for the batch endpoint.
type BatchResponse struct {
	Transfer string        `json:"transfer"`
	Objects  []BatchObject `json:"objects"`
}
