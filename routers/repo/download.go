// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Unknwon/com"
	"github.com/go-martini/martini"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func SingleDownload(ctx *middleware.Context, params martini.Params) {
	treename := params["_1"]

	blob, err := ctx.Repo.Commit.GetBlobByPath(treename)
	if err != nil {
		ctx.Handle(500, "repo.SingleDownload(GetBlobByPath)", err)
		return
	}

	dataRc, err := blob.Data()
	if err != nil {
		ctx.Handle(500, "repo.SingleDownload(Data)", err)
		return
	}

	buf := make([]byte, 1024)
	n, _ := dataRc.Read(buf)
	if n > 0 {
		buf = buf[:n]
	}

	defer func() {
		dataRc.Close()
	}()

	contentType, isTextFile := base.IsTextFile(buf)
	_, isImageFile := base.IsImageFile(buf)
	ctx.Res.Header().Set("Content-Type", contentType)
	if !isTextFile && !isImageFile {
		ctx.Res.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(treename))
		ctx.Res.Header().Set("Content-Transfer-Encoding", "binary")
	}
	ctx.Res.Write(buf)
	io.Copy(ctx.Res, dataRc)
}

func ZipDownload(ctx *middleware.Context, params martini.Params) {
	commitId := ctx.Repo.CommitId
	archivesPath := filepath.Join(ctx.Repo.GitRepo.Path, "archives/zip")
	if !com.IsDir(archivesPath) {
		if err := os.MkdirAll(archivesPath, 0755); err != nil {
			ctx.Handle(500, "ZipDownload -> os.Mkdir(archivesPath)", err)
			return
		}
	}

	archivePath := filepath.Join(archivesPath, commitId+".zip")

	if com.IsFile(archivePath) {
		ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+".zip")
		return
	}

	if err := ctx.Repo.Commit.CreateArchive(archivePath, git.AT_ZIP); err != nil {
		ctx.Handle(500, "ZipDownload -> CreateArchive "+archivePath, err)
		return
	}

	ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+".zip")
}

func TarGzDownload(ctx *middleware.Context, params martini.Params) {
	commitId := ctx.Repo.CommitId
	archivesPath := filepath.Join(ctx.Repo.GitRepo.Path, "archives/targz")
	if !com.IsDir(archivesPath) {
		if err := os.MkdirAll(archivesPath, 0755); err != nil {
			ctx.Handle(500, "TarGzDownload -> os.Mkdir(archivesPath)", err)
			return
		}
	}

	archivePath := filepath.Join(archivesPath, commitId+".tar.gz")

	if com.IsFile(archivePath) {
		ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+".tar.gz")
		return
	}

	if err := ctx.Repo.Commit.CreateArchive(archivePath, git.AT_TARGZ); err != nil {
		ctx.Handle(500, "TarGzDownload -> CreateArchive "+archivePath, err)
		return
	}

	ctx.ServeFile(archivePath, ctx.Repo.Repository.Name+".tar.gz")
}
