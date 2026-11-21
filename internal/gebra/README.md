# Gebra

A fine-grained authorization engine for Go, modeled on Google's Zanzibar and compatible with the OpenFGA API and DSL.

Gebra runs embedded in Gogs as a Go package, or standalone as an HTTP server (`cmd/gebra`) that speaks OpenFGA's HTTP API. It stores everything in the application's own database, with Postgres as the database it is tuned for, and SQLite and MySQL supported. It keeps a materialized view of group membership, so checks and lists read precomputed rows instead of walking the graph, and lists are sorted and paged without limits.

## Features

- **OpenFGA model and API**: schema 1.1 DSL, direct and computed relations, tuple to userset, union, intersection, exclusion, type restrictions, wildcards, conditions (CEL), contextual tuples, Check, BatchCheck, Expand, ListObjects, StreamedListObjects, ListUsers, Read, ReadChanges and assertions.
- **Materialized view**: union-only relations are flattened into member, edge and reach rows, so a check is one statement at any nesting depth.
- **Sorting and no query limits**: lists page with keyset cursors over sortable object attributes (`string`, `int` and `time`), with no cap on results or depth.
- **Short write transactions**: a write commits its tuples, one inbox row and its pending changes, and returns. A single applier per store updates the view in batches.
- **Consistency chosen by the caller**: a write returns a revision token. Queries read the view by default, or pass `MinRevision` or `HIGHER_CONSISTENCY` to read their writes. A write can also `Wait` until it is applied.
- **Bounded staleness**: when the view falls more than `MaxStaleness` (1s by default) behind, default queries evaluate the graph instead.
- **Model changes without downtime**: a new model's view is built next to the live one, and queries switch to it at once when it is complete.
- **Observability**: OpenTelemetry traces and metrics for every operation, a client span for every SQL statement, applier lag and inbox depth.

## Architecture

```mermaid
flowchart LR
  app[Gogs or HTTP client] -->|Write| w[Write]
  app -->|Check, List| q[Query]
  w --> tuple[(gebra_tuple)]
  w --> inbox[(gebra_inbox, gebra_pending)]
  inbox --> applier[Applier, one per store]
  applier --> view[(View: member, edge, reach)]
  applier --> change[(gebra_change)]
  applier --> state[(gebra_view_state)]
  q -->|view current enough| view
  q -->|otherwise| tuple
```

- **Tuples** (`gebra_tuple`) are the source of truth. Read always sees every committed write.
- **Inbox** (`gebra_inbox`) holds the committed writes that the view does not include yet. Its ID is the revision token. `gebra_pending` holds their tuple changes, one row each, so that queries can tell whether a pending write affects their answer.
- **Applier**: one per store, elected with a Postgres advisory lock or MySQL `GET_LOCK`, and an in-process goroutine on SQLite. It is the only writer of the view, the change log and the view state, so the view needs no locks.
- **View**: `gebra_view_member` (direct members of a set), `gebra_view_edge` (set to set) and `gebra_view_reach` (the transitive closure of edges). Every row belongs to a generation, built for one model.
- **Database layer**: plain `database/sql` with explicit SQL, migrated by goose from `migrations/<dialect>`, with its own version table `gebra_db_version`.

A store is the unit of isolation: its tuples share the tables with every other store, but it has its own applier, inbox and view generations. Stores apply writes in parallel, and a store that falls behind does not slow the queries of others. Relations cannot refer to another store.

## Data flow

### Writes

```mermaid
sequenceDiagram
  participant C as Caller
  participant W as Write
  participant DB as Database
  participant A as Applier
  C->>W: Write(tuples)
  W->>DB: tx: change gebra_tuple, insert gebra_inbox and gebra_pending
  W-->>C: revision (inbox ID)
  A->>DB: tx: read pending inbox rows in ID order
  A->>DB: update the view, append gebra_change, delete inbox and pending rows
  Note over C,A: With Wait, Write returns after the applier commits
```

Writes of the same tuple serialize on its row, so the later write gets the higher revision. Writes of different tuples may commit out of order, so a revision counts as applied when no inbox row at or below it is left.

### Queries

```mermaid
flowchart TD
  start[Check or list] --> tok{Token or HIGHER_CONSISTENCY?}
  tok -->|no| stale{Oldest pending write older than MaxStaleness?}
  tok -->|yes| met{Can a pending write change the answer?}
  met -->|no| view[Answer from the view]
  met -->|yes, list| partial[Re-check only the affected objects]
  met -->|yes, check| graph[Evaluate the graph]
  stale -->|no| view
  stale -->|yes| graph
```

- A default check is one statement: the compiled condition on the view, which also verifies the serving generation and that the store exists.
- A token check is also one statement, which adds whether any pending change can affect the answer, from the pending rows and the model's precomputed effects (`gebra_model_effect`).
- Relations with exclusion, intersection or conditions are evaluated from tuples as of the view's point in time, and every statement of a query reads one snapshot.

### Model changes

```mermaid
sequenceDiagram
  participant C as Caller
  participant DB as Database
  participant A as Applier
  C->>DB: WriteAuthorizationModel
  A->>DB: start a build generation for the new model
  loop chunks of 1,000 tuples, then sets
    A->>DB: build members and edges, then reach rows
    A->>DB: apply new writes to the part already built
  end
  A->>DB: flip: the build becomes the serving generation
  Note over DB: The old generation is deleted 1 minute later
```

Until the flip, queries that name no model use the old model from its view, and queries that name the new model evaluate the graph. A newer model cancels the build of an older one.

## Performance

Gebra is benchmarked against OpenFGA through each server's HTTP API, with the same dataset, the same request stream at the same arrival rates, and the same correctness check. On a store of 100,000 repos, Gebra holds 5,000 requests/s with 0 violations, at least twice OpenFGA's rate, and at 2,500 requests/s its check p99 is 2.4 ms against 3.6 s for OpenFGA.

### Dataset and traffic

- **Dataset**: one store with 100,000 repos, 3,000 users, 1,000 teams and 500 orgs, which is 389,897 tuples. The model is GitHub-like: org owners and members, nested teams, repo admins, writers and readers, public repos (`user:*`) and blocked users (`can_read: reader but not blocked`). Gebra's view holds 309,508 member, 280,901 edge and 801,002 reach rows.
- **Traffic**: requests arrive at random (Poisson) times at a fixed rate, and latency counts from when a request was due, so time spent queued counts too. Users, repos and orgs are picked with a Zipf distribution.
  - 92% are checks.
  - 7% list a page of 50 repos a user reads. Lists may be stale, since opening a repo from a list issues a check.
  - 1% start a change session: a permission change (a collaborator, a team member, a team grant, a block, 100 collaborators at once, or a team added to an org), then, after a think time, a check with the write's token that must see the change, two more checks with the token, and sometimes a list. A check that misses its own write counts as a violation.
- **Rates**: 2,500, 5,000, 7,500, 10,000, 15,000, 20,000 and 30,000 requests/s, for 20 s each after a 5 s warmup, stopping after the first rate at which a p99 exceeds 1 s or the server falls behind the offered rate.

### Results

p50 / p99 in milliseconds, and requests completed per second.

| Server | Rate/s | Completed/s | Check | Token check | List | Write | Violations | Errors |
|---|---|---|---|---|---|---|---|---|
| Gebra | 2,500 | 2,503 | **1.1 / 2.4** | **1.1 / 16.3** | 21.2 / 32.1 | **2.1 / 5.3** | 0 | 0 |
| OpenFGA | 2,500 | 2,502 | 2,082 / 3,638 | 275 / 973 | 1,982 / 3,463 | 363 / 3,419 | 0 | 0 |
| Gebra | 5,000 | 5,010 | 1.1 / 29.1 | 1.2 / 38.2 | 24.5 / 108.6 | 2.7 / 49.4 | 0 | 0 |
| Gebra | 7,500 | 4,162 | saturated | saturated | saturated | saturated | 0 | 1,387 timeouts |

| Server | Highest rate held | Checks and token checks within 20 ms p99 | Bottleneck |
|---|---|---|---|
| Gebra | **5,000/s** | **2,500/s** | Postgres CPU: 3.9 cores at 2,500/s, 7.7 at 5,000/s |
| OpenFGA | below 2,500/s | below 2,500/s | OpenFGA's own CPU: Postgres used 1.6 cores while the machine used 13.1 of 16 |

Loading took 2 min 22 s for Gebra, with `COPY` and its view build, and 1 min 3 s for OpenFGA through its Write API.

### Configuration

The benchmark is `TestApplierBenchHTTP` in `applierbench_http_test.go`, whose doc comment has the commands. Every setting below is chosen so that neither server gets an advantage the other lacks.

- **Machine**: Intel Xeon @ 2.60 GHz (Ice Lake), 16 vCPUs (8 cores with hyperthreading), 31 GiB of RAM, under KVM, with Postgres, both servers and the load generator on it and nothing else running. Go 1.27.4.
- **Postgres 15.20**, shared by both servers. Both databases are vacuumed and analyzed after loading.
  - Memory: `shared_buffers` 4 GB, `work_mem` 16 MB, `effective_cache_size` 16 GB, `maintenance_work_mem` 1 GB.
  - Connections and durability: `max_connections` 300, `synchronous_commit` on, `fsync` on.
  - WAL: `max_wal_size` 8 GB, `checkpoint_timeout` 15 min.
  - Planner: `random_page_cost` 1.1, `effective_io_concurrency` 200, `max_parallel_workers_per_gather` 0.
- **Gebra**, `cmd/gebra` built from this tree, as its own process:
  - `--db-type postgres`, `--db-max-open-conns 96`, which also sets 96 idle connections.
  - `--max-staleness 1s`, the default, which matches OpenFGA's 1s cache TTL below.
  - Tracing off (`OTEL_TRACES_SAMPLER=always_off`), Prometheus metrics on, as for OpenFGA.
  - Sessions set `max_parallel_workers_per_gather=0`, which `cmd/gebra` does for its short index lookups.
- **OpenFGA v1.23.0**, started by `testdata/openfga-bench.sh`, which downloads the release and checks its sha256:
  - `--datastore-engine postgres`, `--datastore-max-open-conns 96`, `--datastore-max-idle-conns 96`, to match Gebra's pool.
  - `--check-query-cache-enabled`, `--check-iterator-cache-enabled` and `--list-objects-iterator-cache-enabled`, each with a TTL of 1s, so default reads may be as stale as Gebra's. `HIGHER_CONSISTENCY` bypasses them.
  - `--check-cache-limit 1000000`, since the default of 10,000 entries would cap hit rates; `--cache-controller-enabled=false`, since the TTL already bounds staleness.
  - `--shared-iterator-enabled` and `--experimentals enable-check-optimizations,enable-list-objects-optimizations`, its performance options.
  - `--listObjects-max-results 50`, the page size, since OpenFGA has no paging.
  - `--listObjects-deadline 10s`, `--request-timeout 10s` and `--http-upstream-timeout 10s`, the benchmark's request timeout.
  - `--authn-method none`, `--trace-enabled=false`, `--log-level warn`, playground and profiler off.
- **Client**: one Go HTTP client for both servers, with keep-alives and 4,096 connections per host, so that it is never the bottleneck, and a 10s timeout per request.

What differs between the two:

- **Token checks**: Gebra passes the write's revision as `min_revision`. OpenFGA has no revision tokens, so it gets `HIGHER_CONSISTENCY`.
- **Lists**: Gebra returns a page of 50 sorted results. OpenFGA returns the first 50 it finds, unsorted, which favors OpenFGA.
- **Loading**: Gebra loads with `COPY` and builds its view. OpenFGA loads through its Write API. Neither is measured.
- **Rounds**: one round of each, with 20 s steps, so the tails at the edge rates are noisy.

## Correctness

- Conformance tests cover every Zanzibar and OpenFGA feature listed above, and run against OpenFGA when `GEBRA_OPENFGA_URL` is set.
- Differential fuzzing compared Gebra with OpenFGA on 1,440 models and 35,610 queries. The remaining differences are intentional or OpenFGA bugs, and the minimized cases are replayed by `TestFuzzRegressions`.
- `GEBRA_WRITE_FUZZ` checks after every write that the view gives the same answers as graph evaluation, including across model changes and view builds.
- `GEBRA_PENDING_FUZZ` checks that queries with a token which the view answers despite pending writes give the answers of the latest state.
- Concurrency tests cover out-of-order commits, revision tokens, applier failover, view builds under concurrent writes and cancelled builds, and a check racing an applier batch or a flip.
- The suites run on SQLite, Postgres and MySQL 8.

## Known limits

- **View builds are slow on large stores**: a build covers one set at a time on one core, so a store of 1 million repos and 1,000 teams takes about 1 h 40 min to make a new model live.
- **Lists are the slowest query**: about 20 ms at p50 on the dataset above, since they sort their candidates before paging.
