// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"errors"
	"io"

	"github.com/Unknwon/com"
)

type Blob struct {
	repo *Repository
	*TreeEntry
}

func (b *Blob) Data() (io.Reader, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(b.repo.Path, "git", "show", b.Id.String())
	if err != nil {
		return nil, errors.New(string(stderr))
	}
	return bytes.NewBuffer(stdout), nil
}
