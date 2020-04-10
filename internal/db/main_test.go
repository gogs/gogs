// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/jinzhu/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/testutil"
)

func TestMain(m *testing.M) {
	flag.Parse()
	if !testing.Verbose() {
		// Remove the primary logger and register a noop logger.
		log.Remove(log.DefaultConsoleName)
		err := log.New("noop", testutil.InitNoopLogger)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func deleteTables(db *gorm.DB, tables ...interface{}) error {
	for _, t := range tables {
		err := db.Delete(t).Error
		if err != nil {
			return err
		}
	}
	return nil
}
