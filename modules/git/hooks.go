// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

// hookNames is a list of Git hooks' name that are supported.
var hookNames = []string{
	"pre-applypatch",
	"applypatch-msg",
	"prepare-commit-msg",
	"commit-msg",
	"pre-commit",
	"pre-rebase",
	"post-commit",
	"post-receive",
	"post-update",
}

var (
	ErrNotValidHook = errors.New("not a valid Git hook")
)

// IsValidHookName returns true if given name is a valid Git hook.
func IsValidHookName(name string) bool {
	for _, hn := range hookNames {
		if hn == name {
			return true
		}
	}
	return false
}

// Hook represents a Git hook.
type Hook struct {
	name     string
	IsActive bool   // Indicates whether repository has this hook.
	Content  string // Content of hook if it's active.
	Sample   string // Sample content from Git.
	path     string // Hook file path.
}

// GetHook returns a Git hook by given name and repository.
func GetHook(repoPath, name string) (*Hook, error) {
	if !IsValidHookName(name) {
		return nil, ErrNotValidHook
	}
	h := &Hook{
		name: name,
		path: path.Join(repoPath, "hooks", name),
	}
	if isFile(h.path) {
		data, err := ioutil.ReadFile(h.path)
		if err != nil {
			return nil, err
		}
		h.IsActive = true
		h.Content = string(data)
	} else if isFile(h.path + ".sample") {
		data, err := ioutil.ReadFile(h.path + ".sample")
		if err != nil {
			return nil, err
		}
		h.Sample = string(data)
	}
	return h, nil
}

func (h *Hook) Name() string {
	return h.name
}

// Update updates hook settings.
func (h *Hook) Update() error {
	if len(strings.TrimSpace(h.Content)) == 0 {
		return os.Remove(h.path)
	}
	return ioutil.WriteFile(h.path, []byte(strings.Replace(h.Content, "\r", "", -1)), os.ModePerm)
}

// ListHooks returns a list of Git hooks of given repository.
func ListHooks(repoPath string) (_ []*Hook, err error) {
	if !isDir(path.Join(repoPath, "hooks")) {
		return nil, errors.New("hooks path does not exist")
	}

	hooks := make([]*Hook, len(hookNames))
	for i, name := range hookNames {
		hooks[i], err = GetHook(repoPath, name)
		if err != nil {
			return nil, err
		}
	}
	return hooks, nil
}

func (repo *Repository) GetHook(name string) (*Hook, error) {
	return GetHook(repo.Path, name)
}

func (repo *Repository) Hooks() ([]*Hook, error) {
	return ListHooks(repo.Path)
}
