// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package avatar_test

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogits/gogs/modules/avatar"
	"github.com/gogits/gogs/modules/log"
)

const TMPDIR = "test-avatar"

func TestFetch(t *testing.T) {
	os.Mkdir(TMPDIR, 0755)
	defer os.RemoveAll(TMPDIR)

	hash := avatar.HashEmail("ssx205@gmail.com")
	a := avatar.New(hash, TMPDIR)
	a.UpdateTimeout(time.Millisecond * 200)
}

func TestFetchMany(t *testing.T) {
	os.Mkdir(TMPDIR, 0755)
	defer os.RemoveAll(TMPDIR)

	t.Log("start")
	var n = 5
	ch := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			hash := avatar.HashEmail(strconv.Itoa(i) + "ssx205@gmail.com")
			a := avatar.New(hash, TMPDIR)
			a.Update()
			t.Log("finish", hash)
			ch <- true
		}(i)
	}
	for i := 0; i < n; i++ {
		<-ch
	}
	t.Log("end")
}

// cat
// wget http://www.artsjournal.com/artfulmanager/wp/wp-content/uploads/2013/12/200x200xmirror_cat.jpg.pagespeed.ic.GOZSv6v1_H.jpg -O default.jpg
/*
func TestHttp(t *testing.T) {
	http.Handle("/", avatar.CacheServer("./", "default.jpg"))
	http.ListenAndServe(":8001", nil)
}
*/

func TestLogTrace(t *testing.T) {
	log.Trace("%v", errors.New("console log test"))
}
