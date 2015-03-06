// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"strconv"
	"time"
)

// Author and Committer information
type Signature struct {
	Email string
	Name  string
	When  time.Time
}

// Helper to get a signature from the commit line, which looks like these:
//     author Patrick Gundlach <gundlach@speedata.de> 1378823654 +0200
//     author Patrick Gundlach <gundlach@speedata.de> Thu, 07 Apr 2005 22:13:13 +0200
// but without the "author " at the beginning (this method should)
// be used for author and committer.
//
// FIXME: include timezone for timestamp!
func newSignatureFromCommitline(line []byte) (_ *Signature, err error) {
	sig := new(Signature)
	emailstart := bytes.IndexByte(line, '<')
	sig.Name = string(line[:emailstart-1])
	emailstop := bytes.IndexByte(line, '>')
	sig.Email = string(line[emailstart+1 : emailstop])

	// Check date format.
	firstChar := line[emailstop+2]
	if firstChar >= 48 && firstChar <= 57 {
		timestop := bytes.IndexByte(line[emailstop+2:], ' ')
		timestring := string(line[emailstop+2 : emailstop+2+timestop])
		seconds, err := strconv.ParseInt(timestring, 10, 64)
		if err != nil {
			return nil, err
		}
		sig.When = time.Unix(seconds, 0)
	} else {
		sig.When, err = time.Parse("Mon Jan _2 15:04:05 2006 -0700", string(line[emailstop+2:]))
		if err != nil {
			return nil, err
		}
	}
	return sig, nil
}
