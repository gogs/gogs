# Azulflow

A GitHub Actions workflow planner for Go that reads workflow files as GitHub does and schedules their jobs as GitHub does.

Azulflow was written from scratch, from GitHub's own parsers and conformance suites, for the server side of native Gogs Actions. It takes the unchanged `.github/workflows` files of a repository and an event, and decides which workflows run, which jobs they have and when each job may start, with the same results, defaults and error messages as GitHub. A repository moving from GitHub to Gogs keeps its workflows as they are.

## Features

- **GitHub's workflow syntax**, read with GitHub's schema (`workflow-v1.0.json`) and GitHub's error messages, including YAML anchors, `${{ }}` in every place GitHub allows it and the contexts each place may use.
- **GitHub's expression language**: all operators, functions and type coercions, e.g., `fromJSON`, `toJSON`, `format`, `contains`, `startsWith`, `case` and the status functions `success()`, `failure()`, `always()` and `cancelled()`, with the same number formatting, string comparison and memory limit as GitHub.
- **Triggers**: `branches`, `tags`, `paths` and their `-ignore` forms with GitHub's glob patterns, activity types, `workflow_dispatch` inputs with their types and defaults, and the ref each event runs on, e.g., the default branch for `schedule`.
- **Schedules**: `cron` with an optional IANA `timezone`, firing at the times GitHub fires them, e.g., when both day fields are restricted either may match, a time skipped by a daylight saving change moves to the end of the change, and runs are at least 5 minutes apart.
- **Job graph**: `needs`, job `if` with status functions, skipped jobs skipping their dependents unless their condition says otherwise, `continue-on-error` and `timeout-minutes`.
- **Matrices**: `include`, `exclude`, `fail-fast` and `max-parallel`, up to GitHub's 256 jobs, and matrices built from the outputs of the jobs they need, expanded once those jobs finish.
- **Reusable workflows**: local, same repository and other repository calls, nested up to GitHub's 10 levels, with typed `inputs`, `secrets` and `secrets: inherit`, `outputs`, `permissions` checked against the caller, and matrices of calls.
- **Concurrency**: workflow, job and call `concurrency` groups, compared case-insensitively, with `cancel-in-progress`, a pending run replaced by a newer one and `queue: max` for up to 100 pending runs.
- **Re-runs**: re-running failed jobs keeps the results and outputs of the jobs that succeeded and runs the failed jobs and their dependents again.
- **Run overview**: `Run.Graph()` returns every job of the run and of each called workflow, with the jobs it needs and its status, as JSON that draws the graph GitHub shows for a run, including jobs whose matrix or call is not expanded yet.

## Packages

- `expr` parses and evaluates expressions.
- `template` reads YAML into tokens that keep their source positions, validates them against GitHub's schema and evaluates the expressions in them.
- `model` loads a workflow file and the reusable workflows it calls into GitHub's workflow template, e.g., triggers, jobs and steps, with the values that are only known at run time kept as tokens.
- `planner` matches events to workflows and runs a workflow: `Plan` returns the jobs that can start, and `Run.Complete` returns the jobs that can start once a job finishes.

A `Run` is a deterministic state machine without I/O. The server persists the jobs it reports, and restores a run by calling `Plan` with the same options and replaying its `Complete` and `Cancel` calls in their original order. Steps are evaluated by the runner, not the planner, as on GitHub.

## Correctness

- `TestXlang` in `expr` runs all 1,020 cases of GitHub's cross-language conformance suite for expressions (from [actions/languageservices](https://github.com/actions/languageservices)), including the 5 that GitHub's TypeScript implementation skips. Where the TypeScript and C# implementations disagree, e.g., in the order arguments are evaluated or how numbers are parsed, Azulflow follows C#, which runs on GitHub's servers.
- `TestXLang` in `model` runs the 195 cases of GitHub's conformance suite for workflow parsing, comparing the template and the error messages. One case is skipped, as it expects the result of a GitHub bug that a feature flag fixes.
- Unchanged workflow files of CPython, Rust, uv, Home Assistant and Next.js are planned and run with simulated job results, including failures, and their run graphs are checked job by job.
- Made-up workflows in `planner/testdata/complex` call reusable workflows 4 levels deep, with matrices of calls, outputs feeding matrices and failures deep inside a call.
- `FuzzPlanOracle` generates workflows with needs, status functions, matrices, `fail-fast`, `continue-on-error` and matrices of calls with `max-parallel`, runs them with random job results, and compares every job status with an independent model of GitHub's rules. `FuzzLoadAndPlan` loads arbitrary workflow files, seeded from the conformance suite, plans them and runs them with arbitrary results, and checks that nothing panics and every run ends. `FuzzEval` checks that arbitrary expressions never panic and fail only with GitHub's errors, and `FuzzJSON` that `fromJSON` accepts only JSON and `toJSON` writes JSON that reads back to the same value. Run them with, e.g., `go test -fuzz FuzzPlanOracle -timeout 0 ./internal/azulflow/planner`, and commit the failing inputs they write to `testdata/fuzz` with the fix, so that `go test` replays them.

## Security and robustness

Workflow files come from anyone who can push a branch or open a pull request, so one workflow must never be able to take down the server.

- **Bounded templates.** Reading a workflow stops at GitHub's limits: 1 MB per file, 51 files per run, objects nested 50 levels deep, 1,000,000 parse events and 10 MB of tokens.
- **Bounded expressions.** An expression may use at most 10 MiB of memory while it is evaluated, as on GitHub, so `toJSON(github)` repeated in a `format` fails with GitHub's error instead of exhausting memory.
- **Bounded runs.** A matrix has at most 256 jobs, reusable workflows nest at most 10 levels, and a concurrency group holds at most 100 pending runs. A job that would wait for a concurrency group held by its own run is canceled as a deadlock, with GitHub's message, instead of waiting forever.
- **No side effects.** The planner does not read files, call the network or start processes. Reusable workflows are read through a `FileProvider` that the server implements. Secrets are not part of the plan: `Job.Secrets` resolves them for one job, and jobs of called workflows receive only the secrets their callers pass.
