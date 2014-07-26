// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	// "io"
	// "os"
	// "path/filepath"

	// "github.com/Unknwon/com"

	// "github.com/gogits/git"

	// "github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func SingleDownload(ctx *middleware.Context) {
	// treename := params["_1"]

	// blob, err := ctx.Repo.Commit.GetBlobByPath(treename)
	// if err != nil {
	// 	ctx.Handle(500, "repo.SingleDownload(GetBlobByPath)", err)
	// 	return
	// }

	// dataRc, err := blob.Data()
	// if err != nil {
	// 	ctx.Handle(500, "repo.SingleDownload(Data)", err)
	// 	return
	// }

	// buf := make([]byte, 1024)
	// n, _ := dataRc.Read(buf)
	// if n > 0 {
	// 	buf = buf[:n]
	// }

	// defer func() {
	// 	dataRc.Close()
	// }()

	// contentType, isTextFile := base.IsTextFile(buf)
	// _, isImageFile := base.IsImageFile(buf)
	// ctx.Res.Header().Set("Content-Type", contentType)
	// if !isTextFile && !isImageFile {
	// 	ctx.Res.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(treename))
	// 	ctx.Res.Header().Set("Content-Transfer-Encoding", "binary")
	// }
	// ctx.Res.Write(buf)
	// io.Copy(ctx.Res, dataRc)
}
