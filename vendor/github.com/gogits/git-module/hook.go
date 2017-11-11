// Copyright 2015 The Gogs Authors. All rights reserved.
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

var (
	// Direcotry of hook and sample files. Can be changed to "custom_hooks" for very purpose.
	HookDir       = "hooks"
	HookSampleDir = HookDir
	// HookNames is a list of Git server hooks' name that are supported.
	HookNames = []string{
		"pre-receive",
		"update",
		"post-receive",
	}
)

var (
	ErrNotValidHook = errors.New("not a valid Git hook")
)

// IsValidHookName returns true if given name is a valid Git hook.
func IsValidHookName(name string) bool {
	for _, hn := range HookNames {
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
		path: path.Join(repoPath, HookDir, name),
	}
	if isFile(h.path) {
		data, err := ioutil.ReadFile(h.path)
		if err != nil {
			return nil, err
		}
		h.IsActive = true
		h.Content = string(data)
		return h, nil
	}

	// Check sample file
	samplePath := path.Join(repoPath, HookSampleDir, h.name) + ".sample"
	if isFile(samplePath) {
		data, err := ioutil.ReadFile(samplePath)
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

// Update updates content hook file.
func (h *Hook) Update() error {
	if len(strings.TrimSpace(h.Content)) == 0 {
		if isExist(h.path) {
			return os.Remove(h.path)
		}
		return nil
	}
	os.MkdirAll(path.Dir(h.path), os.ModePerm)
	return ioutil.WriteFile(h.path, []byte(strings.Replace(h.Content, "\r", "", -1)), os.ModePerm)
}

// ListHooks returns a list of Git hooks of given repository.
func ListHooks(repoPath string) (_ []*Hook, err error) {
	if !isDir(path.Join(repoPath, "hooks")) {
		return nil, errors.New("hooks path does not exist")
	}

	hooks := make([]*Hook, len(HookNames))
	for i, name := range HookNames {
		hooks[i], err = GetHook(repoPath, name)
		if err != nil {
			return nil, err
		}
	}
	return hooks, nil
}
