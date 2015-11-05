// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"
	"io"
)

// Blob represents a blob type object.
type Blob struct {
	*TreeEntry
}

// Data returns a io.ReadCloser which can be read for blob data.
func (b *Blob) Data() (io.ReadCloser, error) {
	_, _, rc, err := b.ptree.repo.getRawObject(b.ID, false)
	if err != nil {
		return nil, fmt.Errorf("getRawObject: %v", err)
	}
	return rc, nil
}
