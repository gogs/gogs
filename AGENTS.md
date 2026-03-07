## Core principles

- Stop telling me "You're right", it just shows how incompetent you are. Do it right on your first try, fact-check and review after changes. If you are not sure, ask for help.
- When you see changes made outside your knowledge, use the current version as your new starting point. Do not blindly overwrite those changes or you suck. Even if you have to update the code, always respect the pattern in the surrounding context!

## Style and mechanics

This applies to all texts, including but not limited to UI, documentation, code comments.

- Use sentence case. Preserve original casing for brand names.
- End with a period for a full sentence.
- Do not add comments that repeat what the code is doing, always prefer more descriptive names. Do add comments for intentions that aren't obvious via reading the code alone. This rule takes precedence over matching existing patterns.

## Coding guidelines

- Use `github.com/cockroachdb/errors` for error handling.
- Use `github.com/stretchr/testify` for assertions in tests. Be mindful about the choice of `require` and `assert`, the former should be used when the test cannot proceed meaningfully after a failed assertion.

## Build instructions

- Prefer `task` command over vanilla `go` command when available. Use `--force` flag when necessary.
- Run `task lint` after every time you finish changing code, and fix all linter errors.
- Run `go mod tidy` after every time you change `go.mod`, do not manually edit `go.sum` file.

## Tool-use guidance

- Use `gh` CLI to access information on github.com that is not publicly available.

## Source code control

- When pushing changes to a pull request from a fork, use SSH address and do not add remote.
- Never commit on the `main` branch directly unless being explicitly asked to do so. A single ask only grants a single commit action on the `main` branch.
- Never amend commits unless being explicitly asked to do so.
