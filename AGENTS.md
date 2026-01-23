## Core principles

- When you see changes made outside your knowledge, use the current version as your new starting point. Do not blindly overwrite those changes or you suck. Even if you have to update the code, always respect the god damn pattern in the surrounding context!

## Style and mechanics

This applies to all texts, including but not limited to UI, documentation, code comments.

- Use sentence case. Preserve original casing for brand names.
- End with a period for a full sentence.
- Do not add comments that repeat what the code is doing, always prefer more descriptive names. Do add comments for intentions that aren't obvious via reading the code alone. This rule takes precedence over matching existing patterns.

## Coding guidelines

- Use `github.com/cockroachdb/errors` for error handling.
- Use `github.com/stretchr/testify` for assertions in tests. Be mindful about the choice of `require` and `assert`, the former should be used when the test cannot proceed meaningfully after a failed assertion.

## Tool-use guidance

- Use `gh` CLI to access information on github.com that is not publicly available.
