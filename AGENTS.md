## Core principles

- Stop telling me "You're right", it just shows how incompetent you are. Do it right on your first try, fact-check and review after changes. If you are not sure, ask for help.
- When you see changes made outside your knowledge, use the current version as your new starting point. Do not blindly overwrite those changes or you suck. Even if you have to update the code, always respect the pattern in the surrounding context!

## Style and mechanics

This applies to all texts, including but not limited to UI, documentation, code comments.

- Use sentence case. Preserve original casing for brand names.
- End with a period for a full sentence.
- Never use em dashes (`—`) or en dashes (`–`) in prose. Rewrite the sentence with a comma, period, colon, or parentheses instead. Exception: em/en dashes are allowed as visual separators in UI design (e.g., between a title and description, in a terminal prompt label) where they function as a graphic element rather than punctuation.
- Do not overuse semicolons. Two short sentences are almost always clearer than one sentence joined by a semicolon. Reserve the semicolon for the rare case where the two clauses are so tightly coupled that splitting them loses meaning, never as a default em-dash replacement or a way to chain related thoughts.
- Do not add comments that repeat what the code is doing, always prefer more descriptive names. Do add comments for intentions that aren't obvious via reading the code alone. This rule takes precedence over matching existing patterns.
- Do not include implementation details in CHANGELOG entries, describe the visible impact only from user's POV.
- Always use `e.g.,` and `i.e.,` with the trailing comma.

## Coding guidelines

- Use `github.com/cockroachdb/errors` for error handling.
- Use `github.com/stretchr/testify` for assertions in tests. Be mindful about the choice of `require` and `assert`, the former should be used when the test cannot proceed meaningfully after a failed assertion.
- Every 5xx response must log the error directly inside the handler, do not log errors in a shared helper.

## Localization

- Only edit `conf/locale/locale_en-US.ini`. The other `locale_*.ini` files are community-maintained translations. Do not add, remove, or rewrite keys in them, even when removing keys that are dead on the Go/template side.

## UI guidelines

- Design mobile-friendly. Every UI must look and work well on narrow viewports before adding desktop refinements via responsive breakpoints. Test at ~375px width before considering a UI done.
- Meet WCAG 2.2 AA at minimum. Specifically: every interactive control has a discernible accessible name (visible label or `aria-label`); color is never the sole carrier of information (pair with text, icon, or shape); text and meaningful icons meet 4.5:1 contrast against their background (3:1 for large text and UI components); focus is always visible and never trapped; touch targets are at least 24×24 CSS px (40×40 preferred). When unsure, lean toward more contrast, larger targets, and explicit labels.
- For work under `web/`, follow the patterns in [`web/DESIGN.md`](web/DESIGN.md) (typography, color hierarchy, surface chrome, file naming, accessibility specifics). Update that doc when a pattern is used in two places.
- When a page needs server data to render, fetch it in the TanStack Router route's `loader` so the page only mounts after the response arrives. Do not fire that fetch from a `useEffect` inside the page component, which causes a flash of empty UI before the data lands.

## Build instructions

- Prefer `moon run <project>:<task>` over vanilla `go` or `pnpm` commands when available (e.g., `moon run gogs:build`, `moon run web:dev`). Pass `--force` to bypass cache when necessary.
- Run `moon run gogs:lint` after every time you finish changing Go code, and `moon run web:lint` after changing frontend code, then fix all linter errors.

## Tool-use guidance

- Use `gh` CLI to access information on github.com that is not publicly available.
- Run the Chrome DevTools MCP in headless mode so it does not steal focus from the user's foreground browser session. After finishing any task that used the Chrome DevTools MCP, kill all `chrome-devtools-mcp` processes with `pkill -f chrome-devtools-mcp`.

## Source code control

- When pushing changes to a pull request from a fork, use SSH address and do not add remote.
- Never commit on the `main` branch directly unless being explicitly asked to do so. A single ask only grants a single commit action on the `main` branch.
- Never amend commits unless being explicitly asked to do so.
- When creating a git worktree, the worktree directory name must match its branch name. Do not use random or generated suffixes.
