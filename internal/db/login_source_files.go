// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"gopkg.in/ini.v1"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/auth/github"
	"gogs.io/gogs/internal/auth/ldap"
	"gogs.io/gogs/internal/auth/pam"
	"gogs.io/gogs/internal/auth/smtp"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/osutil"
)

// loginSourceFilesStore is the in-memory interface for login source files stored on file system.
//
// NOTE: All methods are sorted in alphabetical order.
type loginSourceFilesStore interface {
	// GetByID returns a clone of login source by given ID.
	GetByID(id int64) (*LoginSource, error)
	// Len returns number of login sources.
	Len() int
	// List returns a list of login sources filtered by options.
	List(opts ListLoginSourceOptions) []*LoginSource
	// Update updates in-memory copy of the authentication source.
	Update(source *LoginSource)
}

var _ loginSourceFilesStore = (*loginSourceFiles)(nil)

// loginSourceFiles contains authentication sources configured and loaded from local files.
type loginSourceFiles struct {
	sync.RWMutex
	sources []*LoginSource
	clock   func() time.Time
}

var _ errutil.NotFound = (*ErrLoginSourceNotExist)(nil)

type ErrLoginSourceNotExist struct {
	args errutil.Args
}

func IsErrLoginSourceNotExist(err error) bool {
	_, ok := err.(ErrLoginSourceNotExist)
	return ok
}

func (err ErrLoginSourceNotExist) Error() string {
	return fmt.Sprintf("login source does not exist: %v", err.args)
}

func (ErrLoginSourceNotExist) NotFound() bool {
	return true
}

func (s *loginSourceFiles) GetByID(id int64) (*LoginSource, error) {
	s.RLock()
	defer s.RUnlock()

	for _, source := range s.sources {
		if source.ID == id {
			return source, nil
		}
	}

	return nil, ErrLoginSourceNotExist{args: errutil.Args{"id": id}}
}

func (s *loginSourceFiles) Len() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.sources)
}

func (s *loginSourceFiles) List(opts ListLoginSourceOptions) []*LoginSource {
	s.RLock()
	defer s.RUnlock()

	list := make([]*LoginSource, 0, s.Len())
	for _, source := range s.sources {
		if opts.OnlyActivated && !source.IsActived {
			continue
		}

		list = append(list, source)
	}
	return list
}

func (s *loginSourceFiles) Update(source *LoginSource) {
	s.Lock()
	defer s.Unlock()

	source.Updated = s.clock()
	for _, old := range s.sources {
		if old.ID == source.ID {
			*old = *source
		} else if source.IsDefault {
			old.IsDefault = false
		}
	}
}

// loadLoginSourceFiles loads login sources from file system.
func loadLoginSourceFiles(authdPath string, clock func() time.Time) (loginSourceFilesStore, error) {
	if !osutil.IsDir(authdPath) {
		return &loginSourceFiles{clock: clock}, nil
	}

	store := &loginSourceFiles{clock: clock}
	return store, filepath.Walk(authdPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == authdPath || !strings.HasSuffix(path, ".conf") {
			return nil
		} else if info.IsDir() {
			return filepath.SkipDir
		}

		authSource, err := ini.Load(path)
		if err != nil {
			return errors.Wrap(err, "load file")
		}
		authSource.NameMapper = ini.TitleUnderscore

		// Set general attributes
		s := authSource.Section("")
		loginSource := &LoginSource{
			ID:        s.Key("id").MustInt64(),
			Name:      s.Key("name").String(),
			IsActived: s.Key("is_activated").MustBool(),
			IsDefault: s.Key("is_default").MustBool(),
			File: &loginSourceFile{
				path: path,
				file: authSource,
			},
		}

		fi, err := os.Stat(path)
		if err != nil {
			return errors.Wrap(err, "stat file")
		}
		loginSource.Updated = fi.ModTime()

		// Parse authentication source file
		authType := s.Key("type").String()
		cfgSection := authSource.Section("config")
		switch authType {
		case "ldap_bind_dn":
			var cfg ldap.Config
			err = cfgSection.MapTo(&cfg)
			if err != nil {
				return errors.Wrap(err, `map "config" section`)
			}
			loginSource.Type = auth.LDAP
			loginSource.Provider = ldap.NewProvider(false, &cfg)

		case "ldap_simple_auth":
			var cfg ldap.Config
			err = cfgSection.MapTo(&cfg)
			if err != nil {
				return errors.Wrap(err, `map "config" section`)
			}
			loginSource.Type = auth.DLDAP
			loginSource.Provider = ldap.NewProvider(true, &cfg)

		case "smtp":
			var cfg smtp.Config
			err = cfgSection.MapTo(&cfg)
			if err != nil {
				return errors.Wrap(err, `map "config" section`)
			}
			loginSource.Type = auth.SMTP
			loginSource.Provider = smtp.NewProvider(&cfg)

		case "pam":
			var cfg pam.Config
			err = cfgSection.MapTo(&cfg)
			if err != nil {
				return errors.Wrap(err, `map "config" section`)
			}
			loginSource.Type = auth.PAM
			loginSource.Provider = pam.NewProvider(&cfg)

		case "github":
			var cfg github.Config
			err = cfgSection.MapTo(&cfg)
			if err != nil {
				return errors.Wrap(err, `map "config" section`)
			}
			loginSource.Type = auth.GitHub
			loginSource.Provider = github.NewProvider(&cfg)

		default:
			return fmt.Errorf("unknown type %q", authType)
		}

		store.sources = append(store.sources, loginSource)
		return nil
	})
}

// loginSourceFileStore is the persistent interface for a login source file.
type loginSourceFileStore interface {
	// SetGeneral sets new value to the given key in the general (default) section.
	SetGeneral(name, value string)
	// SetConfig sets new values to the "config" section.
	SetConfig(cfg any) error
	// Save persists values to file system.
	Save() error
}

var _ loginSourceFileStore = (*loginSourceFile)(nil)

type loginSourceFile struct {
	path string
	file *ini.File
}

func (f *loginSourceFile) SetGeneral(name, value string) {
	f.file.Section("").Key(name).SetValue(value)
}

func (f *loginSourceFile) SetConfig(cfg any) error {
	return f.file.Section("config").ReflectFrom(cfg)
}

func (f *loginSourceFile) Save() error {
	return f.file.SaveTo(f.path)
}
