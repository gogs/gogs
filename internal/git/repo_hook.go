package git

import (
	"os"
	"path/filepath"
	"slices"
)

// Hook returns a Git hook by given name in the directory of the repository. It
// returns an os.ErrNotExist if the name is not one of ServerSideHooks.
func (r *Repository) Hook(dir string, name HookName) (*Hook, error) {
	// 🚨 SECURITY: Prevent reading and writing files other than the hooks, e.g.,
	// with a name like "../config".
	if !slices.Contains(ServerSideHooks, name) {
		return nil, os.ErrNotExist
	}

	// 1. Check if there is an active hook.
	fpath := filepath.Join(r.path, dir, string(name))
	if isFile(fpath) {
		p, err := os.ReadFile(fpath)
		if err != nil {
			return nil, err
		}
		return &Hook{
			name:    name,
			path:    fpath,
			content: string(p),
		}, nil
	}

	// 2. Fall back to the sample content.
	return &Hook{
		name:     name,
		path:     fpath,
		isSample: true,
		content:  serverSideHookSamples[name],
	}, nil
}

// Hooks returns the server-side Git hooks in the directory of the repository.
func (r *Repository) Hooks(dir string) ([]*Hook, error) {
	hooks := make([]*Hook, 0, len(ServerSideHooks))
	for _, name := range ServerSideHooks {
		h, err := r.Hook(dir, name)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}
	return hooks, nil
}
