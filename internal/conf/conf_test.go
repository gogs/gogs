// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package conf

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/ini.v1"

	"gogs.io/gogs/internal/testutil"
)

func TestInit(t *testing.T) {
	ini.PrettyFormat = false
	defer func() {
		MustInit("")
		ini.PrettyFormat = true
	}()

	assert.Nil(t, Init(filepath.Join("testdata", "custom.ini")))

	cfg := ini.Empty()
	cfg.NameMapper = ini.SnackCase

	for _, v := range []struct {
		section string
		config  interface{}
	}{
		{"", &App},
		{"server", &Server},
		{"server", &SSH},
		{"repository", &Repository},
		{"database", &Database},
		{"security", &Security},
		{"email", &Email},
		{"auth", &Auth},
		{"user", &User},
		{"session", &Session},
		{"attachment", &Attachment},
		{"time", &Time},
		{"picture", &Picture},
		{"mirror", &Mirror},
		{"i18n", &I18n},
	} {
		err := cfg.Section(v.section).ReflectFrom(v.config)
		if err != nil {
			t.Fatalf("%s: %v", v.section, err)
		}
	}

	buf := new(bytes.Buffer)
	_, err := cfg.WriteTo(buf)
	if err != nil {
		t.Fatal(err)
	}

	testutil.AssertGolden(t, filepath.Join("testdata", "TestInit.golden.ini"), testutil.Update("TestInit"), buf.String())
}
