package git

import "context"

// WorktreeAddOptions contains optional arguments for adding a worktree.
//
// Docs: https://git-scm.com/docs/git-worktree#Documentation/git-worktree.txt-add
type WorktreeAddOptions struct {
	// The new branch name to create and checkout in the worktree.
	Branch string
	CommandOptions
}

// WorktreeAdd creates a new worktree at the given path linked to this
// repository. The commitIsh determines the HEAD of the new worktree.
func (r *Repository) WorktreeAdd(ctx context.Context, path, commitIsh string, opts ...WorktreeAddOptions) error {
	var opt WorktreeAddOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"worktree", "add"}
	if opt.Branch != "" {
		args = append(args, "-b", opt.Branch)
	}
	args = append(args, "--end-of-options", path, commitIsh)

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}

// WorktreeRemoveOptions contains optional arguments for removing a worktree.
//
// Docs: https://git-scm.com/docs/git-worktree#Documentation/git-worktree.txt-remove
type WorktreeRemoveOptions struct {
	// Indicates whether to force removal even if the worktree is dirty.
	Force bool
	CommandOptions
}

// WorktreeRemove removes a worktree at the given path.
func (r *Repository) WorktreeRemove(ctx context.Context, path string, opts ...WorktreeRemoveOptions) error {
	var opt WorktreeRemoveOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"worktree", "remove"}
	if opt.Force {
		args = append(args, "--force")
	}
	args = append(args, "--end-of-options", path)

	_, err := exec(ctx, r.path, args, opt.Envs)
	return err
}
