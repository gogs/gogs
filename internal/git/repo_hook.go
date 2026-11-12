package git

import (
	"os"
	"path/filepath"
)

// DefaultHooksDir is the default directory for Git hooks.
const DefaultHooksDir = "hooks"

// NewHook creates and returns a new hook with given name. Update method must be
// called to actually save the hook to disk.
func (r *Repository) NewHook(dir string, name HookName) *Hook {
	return &Hook{
		name: name,
		path: filepath.Join(r.path, dir, string(name)),
	}
}

// Hook returns a Git hook by given name in the repository. Giving empty
// directory will use the default directory. It returns an os.ErrNotExist if
// both active and sample hook do not exist.
func (r *Repository) Hook(dir string, name HookName) (*Hook, error) {
	if dir == "" {
		dir = DefaultHooksDir
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

	// 2. Check if sample content exists.
	sample := ServerSideHookSamples[name]
	if sample != "" {
		return &Hook{
			name:     name,
			path:     fpath,
			isSample: true,
			content:  sample,
		}, nil
	}

	return nil, os.ErrNotExist
}

// Hooks returns a list of Git hooks found in the repository. Giving empty
// directory will use the default directory. It may return an empty slice when
// no hooks found.
func (r *Repository) Hooks(dir string) ([]*Hook, error) {
	hooks := make([]*Hook, 0, len(ServerSideHooks))
	for _, name := range ServerSideHooks {
		h, err := r.Hook(dir, name)
		if err != nil {
			if err == os.ErrNotExist {
				continue
			}
			return nil, err
		}
		hooks = append(hooks, h)
	}
	return hooks, nil
}
