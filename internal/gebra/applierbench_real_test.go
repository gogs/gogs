package gebra

// The realistic workload ("real") models the traffic of a Gogs-like site on
// the first store, at fixed arrival rates instead of flat out. Requests arrive
// at random (Poisson) times, and each one's latency counts from when it was
// due, so time spent queued behind slow requests counts too. Each rate of
// GEBRA_BENCH_RATES runs for GEBRA_BENCH_WARMUP, which is not measured, plus
// GEBRA_BENCH_DURATION, and the ladder stops after the first rate at which
// checks without a token have a p99 above GEBRA_BENCH_STOP_MS or the engine
// completes less than 90% of the requests due.
//
// Of the arrivals, GEBRA_BENCH_LIST_SHARE (default 0.07) list a page of 50
// repos a user reads, half of GEBRA_BENCH_WRITE_SHARE (default 0.01) start a
// change session, and the rest check a relation. Users, repos and orgs are
// picked with a Zipf distribution, so that a few get most of the traffic.
//
// A change session is a user changing permissions and then loading the next
// page with the revision token of their write. The change is one of these,
// each to tuples that the dataset does not have, so that removing them
// afterwards restores the dataset:
//
//   - 40% add a user as a reader or writer of a repo;
//   - 24% add a user to a team;
//   - 15% give an extra team (one the dataset does not use) read access to
//     a repo;
//   - 15% block a member of a repo's org, i.e., revoke their access;
//   - 5% add 100 collaborators at once;
//   - 1% add an extra team to an org, which reaches all of the org's repos.
//
// After a think time, which is 200ms to 2s for a share of 1 minus
// GEBRA_BENCH_FAST_SHARE (default 0.1) of the sessions, as for a person
// loading the next page, and up to 50ms for the others, as for API scripts,
// the session checks with its token that its change is visible, makes two
// more checks of the page, and lists a page of repos 30% of the time. The
// list goes without the token, since a user opening a repo from it gets a
// check, which has the token. Then it waits up to 2s and removes the change. Both writes count as writes. A
// change that is not visible to a check with its token is a violation.
//
// GEBRA_BENCH_WRITES=single or bulk makes every arrival a write instead, to
// measure write throughput: a new change of 1 collaborator, or of 100 at once,
// or the removal of an earlier change, each half of the time once earlier
// changes have committed, so that the dataset keeps its size. The ladder
// stops on the writes' p99 instead of the checks'.

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stretchr/testify/require"
)

// benchOpStats is the outcome of one kind of operation of the realistic
// workload. Ops is completions per second of the measured duration.
type benchOpStats struct {
	N      int     `json:"n"`
	Ops    float64 `json:"ops"`
	P50    float64 `json:"p50_ms"`
	P99    float64 `json:"p99_ms"`
	P999   float64 `json:"p999_ms"`
	Max    float64 `json:"max_ms"`
	Errors int     `json:"errors,omitempty"`
}

// The kinds of operations of the realistic workload. Writes of large changes
// count apart from the others, since they take much longer.
const (
	realCheck      = "check"
	realList       = "list"
	realCheckToken = "check_token"
	realWrite      = "write"
	realWriteBulk  = "write_bulk"
	realWriteFan   = "write_fanout"
)

// realExtraTeams is the number of extra teams, each with realExtraMembers
// members, that change sessions give access to repos and orgs.
const (
	realExtraTeams   = 64
	realExtraMembers = 8
)

func realExtraTeam(i int) string { return fmt.Sprintf("team:rx%03d", i) }

// realExtraMember returns the k-th member of the i-th extra team.
func realExtraMember(users, i, k int) string {
	return fmt.Sprintf("user:u%07d", (i*7919+k*131)%users)
}

// realChange is the change of a session, and what a check with the write's
// revision must answer once the change is visible.
type realChange struct {
	op   string
	keys []TupleKey
	// The session's checks are about user and object; verify is the relation
	// whose answer must be want.
	user, object, verify string
	want                 bool
	think                time.Duration
}

// realGen picks the requests of the realistic workload. Only the dispatcher
// goroutine uses it.
type realGen struct {
	rng                       *rand.Rand
	users, teams, orgs, repos int
	user, repo, org           *rand.Zipf
	fastShare                 float64
	// reserved holds the tuples of sessions in progress, so that no two
	// sessions change the same tuple.
	reserved sync.Map
}

func newRealGen(scale int, fastShare float64) *realGen {
	users, teams, orgs, repos, _, _ := benchLadderDataset(scale)
	rng := rand.New(rand.NewPCG(uint64(scale), 7))
	// v flattens the head, so that the hottest user or repo gets a few percent
	// of the traffic rather than a sixth of it.
	zipf := func(n int) *rand.Zipf { return rand.NewZipf(rng, 1.1, 10, uint64(n-1)) }
	return &realGen{
		rng: rng, users: users, teams: teams, orgs: orgs, repos: repos,
		user: zipf(users), repo: zipf(repos), org: zipf(orgs), fastShare: fastShare,
	}
}

// spread maps a Zipf rank to an ID, so that the hot IDs are spread over the
// dataset rather than being its lowest ones.
func spread(rank uint64, n int) int { return int(rank * 1_000_003 % uint64(n)) }

func (g *realGen) userIdx() int { return spread(g.user.Uint64(), g.users) }
func (g *realGen) repoIdx() int { return spread(g.repo.Uint64(), g.repos) }

func benchUser(i int) string { return fmt.Sprintf("user:u%07d", i) }
func benchRepo(i int) string { return fmt.Sprintf("repo:r%08d", i) }

// check returns a check without a token: half of them of a member of an org
// on one of the org's repos, the others of any user on any repo.
func (g *realGen) check() [3]string {
	rels := [...]string{"reader", "reader", "can_read", "can_read", "writer", "admin"}
	rel := rels[g.rng.IntN(len(rels))]
	if g.rng.IntN(2) == 0 {
		o := spread(g.org.Uint64(), g.orgs)
		user := benchUser((o*101 + g.rng.IntN(50)*97) % g.users)
		return [3]string{user, rel, benchRepo(o + g.orgs*g.rng.IntN(g.repos/g.orgs))}
	}
	return [3]string{benchUser(g.userIdx()), rel, benchRepo(g.repoIdx())}
}

// collaborator returns a direct reader or writer tuple that the dataset does
// not have.
func (g *realGen) collaborator() (u, r int, rel string) {
	for {
		u, r = g.userIdx(), g.repoIdx()
		rel = "reader"
		if g.rng.IntN(3) == 0 {
			rel = "writer"
		}
		if u != (r*13+2)%g.users && u != (r*17+1)%g.users && u != (r*31)%g.users {
			return u, r, rel
		}
	}
}

// reserve reserves the keys for a session, or returns false when another
// session holds one of them.
func (g *realGen) reserve(keys []TupleKey) bool {
	for i, k := range keys {
		if _, taken := g.reserved.LoadOrStore(k, true); taken {
			for _, held := range keys[:i] {
				g.reserved.Delete(held)
			}
			return false
		}
	}
	return true
}

func (g *realGen) release(keys []TupleKey) {
	for _, k := range keys {
		g.reserved.Delete(k)
	}
}

// change returns a change session's change, with its keys reserved.
func (g *realGen) change() realChange {
	for {
		c := g.pickChange()
		if g.reserve(c.keys) {
			if g.rng.Float64() < g.fastShare {
				c.think = time.Duration(g.rng.Int64N(int64(50 * time.Millisecond)))
			} else {
				// Log-uniform between 200ms and 2s.
				c.think = time.Duration(float64(200*time.Millisecond) * math.Pow(10, g.rng.Float64()))
			}
			return c
		}
	}
}

func (g *realGen) pickChange() realChange {
	p := g.rng.IntN(100)
	switch {
	case p < 40:
		u, r, rel := g.collaborator()
		return realChange{
			op: realWrite, keys: []TupleKey{benchKey(benchUser(u), rel, benchRepo(r))},
			user: benchUser(u), object: benchRepo(r), verify: rel, want: true,
		}
	case p < 64:
		t, u := g.rng.IntN(g.teams), g.userIdx()
		for k := 0; k < 8; k++ {
			if u == (t*37+k*997)%g.users {
				u, k = (u+1)%g.users, -1
			}
		}
		team := fmt.Sprintf("team:t%06d", t)
		return realChange{
			op: realWrite, keys: []TupleKey{benchKey(benchUser(u), "member", team)},
			user: benchUser(u), object: team, verify: "member", want: true,
		}
	case p < 79:
		i, r := g.rng.IntN(realExtraTeams), g.repoIdx()
		return realChange{
			op: realWrite, keys: []TupleKey{benchKey(realExtraTeam(i)+"#member", "reader", benchRepo(r))},
			user: realExtraMember(g.users, i, g.rng.IntN(realExtraMembers)), object: benchRepo(r), verify: "reader", want: true,
		}
	case p < 94:
		// Repos with r%10 == 0 have blocked tuples in the dataset.
		r := g.repoIdx()
		if r%10 == 0 {
			r = (r + 1) % g.repos
		}
		u := benchUser(((r%g.orgs)*101 + g.rng.IntN(50)*97) % g.users)
		return realChange{
			op: realWrite, keys: []TupleKey{benchKey(u, "blocked", benchRepo(r))},
			user: u, object: benchRepo(r), verify: "can_read", want: false,
		}
	case p < 99:
		c := realChange{op: realWriteBulk, verify: "reader", want: true}
		seen := map[TupleKey]bool{}
		for len(c.keys) < 100 {
			u, r, _ := g.collaborator()
			k := benchKey(benchUser(u), "reader", benchRepo(r))
			if !seen[k] {
				seen[k] = true
				c.keys = append(c.keys, k)
			}
		}
		c.user, c.object = c.keys[0].User, c.keys[0].Object
		return c
	}
	i, o := g.rng.IntN(realExtraTeams), spread(g.org.Uint64(), g.orgs)
	return realChange{
		op: realWriteFan, keys: []TupleKey{benchKey(realExtraTeam(i)+"#member", "member", fmt.Sprintf("org:o%05d", o))},
		user:   realExtraMember(g.users, i, g.rng.IntN(realExtraMembers)),
		object: benchRepo(o + g.orgs*g.rng.IntN(g.repos/g.orgs)), verify: "reader", want: true,
	}
}

// collaborators returns a change that adds n collaborators that the dataset
// does not have, with its keys reserved.
func (g *realGen) collaborators(n int) realChange {
	for {
		c := realChange{op: realWrite}
		if n > 1 {
			c.op = realWriteBulk
		}
		seen := map[TupleKey]bool{}
		for len(c.keys) < n {
			u, r, rel := g.collaborator()
			k := benchKey(benchUser(u), rel, benchRepo(r))
			if !seen[k] {
				seen[k] = true
				c.keys = append(c.keys, k)
			}
		}
		if g.reserve(c.keys) {
			return c
		}
	}
}

// realStats collects the samples of one rate.
type realStats struct {
	start      time.Time
	mu         sync.Mutex
	samples    map[string][]time.Duration
	errs       map[string]int
	lastErr    error
	violations int
	violation  string
}

// add records an operation that was due at due, unless it was due before
// the measurement started.
func (s *realStats) add(op string, due time.Time, took time.Duration, err error) {
	if due.Before(s.start) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples[op] = append(s.samples[op], took)
	if err != nil {
		s.errs[op]++
		s.lastErr = err
	}
}

func (s *realStats) violate(desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.violations++
	if s.violation == "" {
		s.violation = desc
	}
}

// realTarget is what the realistic workload sends its requests to.
type realTarget interface {
	// check checks q. A token is one that write returned, and the check must
	// see that write.
	check(ctx context.Context, q [3]string, token string) (bool, error)
	// list lists a page of 50 repos that user reads.
	list(ctx context.Context, user string) error
	write(ctx context.Context, req *WriteRequest) (token string, err error)
}

// realEngine is the engine in process as the target of the realistic
// workload. Its tokens are revisions.
type realEngine struct {
	e       *Engine
	storeID string
}

func (r realEngine) check(ctx context.Context, q [3]string, token string) (bool, error) {
	return r.e.Check(ctx, r.storeID, &CheckRequest{
		TupleKey: CheckTupleKey{User: q[0], Relation: q[1], Object: q[2]}, Consistency: Consistency{MinRevision: token},
	})
}

func (r realEngine) list(ctx context.Context, user string) error {
	_, _, err := r.e.ListObjects(ctx, r.storeID, &ListObjectsRequest{Type: "repo", Relation: "reader", User: user, PageSize: 50})
	return err
}

func (r realEngine) write(ctx context.Context, req *WriteRequest) (string, error) {
	return r.e.Write(ctx, r.storeID, req)
}

// realLoad runs the realistic workload on the engine in process at each rate
// and returns one result per rate.
func (b *applierBench) realLoad(d time.Duration) []benchResult {
	return b.realRun(d, realEngine{e: b.e, storeID: b.stores[0]}, b.awaitViews)
}

// realRun runs the realistic workload on the target at each rate and returns
// one result per rate. await waits until the target's view includes every
// write and returns how long that took, or is nil for a target without a
// view. Without an engine in process, the results lack the engine's metrics.
func (b *applierBench) realRun(d time.Duration, target realTarget, await func() time.Duration) []benchResult {
	t, ctx, e := b.t, b.ctx, b.e
	envFloat := func(name, def string) float64 {
		v, err := strconv.ParseFloat(benchEnv(name, def), 64)
		require.NoError(t, err, name)
		return v
	}
	writeShare := envFloat("GEBRA_BENCH_WRITE_SHARE", "0.01")
	listShare := envFloat("GEBRA_BENCH_LIST_SHARE", "0.07")
	fastShare := envFloat("GEBRA_BENCH_FAST_SHARE", "0.1")
	stopMs := envFloat("GEBRA_BENCH_STOP_MS", "1000")
	warmup, err := time.ParseDuration(benchEnv("GEBRA_BENCH_WARMUP", "2s"))
	require.NoError(t, err)
	var rates []float64
	for _, s := range strings.Split(benchEnv("GEBRA_BENCH_RATES", "2500,5000,10000,20000,40000"), ",") {
		rate, err := strconv.ParseFloat(s, 64)
		require.NoError(t, err, "GEBRA_BENCH_RATES")
		rates = append(rates, rate)
	}

	g := newRealGen(b.scale, fastShare)
	var extra []TupleKey
	for i := range realExtraTeams {
		for k := range realExtraMembers {
			extra = append(extra, benchKey(realExtraMember(g.users, i, k), "member", realExtraTeam(i)))
		}
	}
	// OpenFGA takes up to 100 tuples per write.
	for chunk := range slices.Chunk(extra, 100) {
		_, err = target.write(ctx, &WriteRequest{Writes: chunk, OnDuplicate: "ignore"})
		require.NoError(t, err)
	}
	if await != nil {
		await()
	}

	opCtx := func() (context.Context, context.CancelFunc) { return context.WithTimeout(ctx, 10*time.Second) }
	check := func(s *realStats, op string, due time.Time, q [3]string, token string) (bool, error) {
		cctx, cancel := opCtx()
		defer cancel()
		ok, err := target.check(cctx, q, token)
		s.add(op, due, time.Since(due), err)
		return ok, err
	}
	list := func(s *realStats, op string, due time.Time, user string) {
		cctx, cancel := opCtx()
		defer cancel()
		err := target.list(cctx, user)
		s.add(op, due, time.Since(due), err)
	}
	write := func(s *realStats, op string, due time.Time, req *WriteRequest) (string, error) {
		cctx, cancel := opCtx()
		defer cancel()
		req.OnDuplicate, req.OnMissing = "ignore", "ignore"
		token, err := target.write(cctx, req)
		s.add(op, due, time.Since(due), err)
		return token, err
	}
	session := func(s *realStats, due time.Time, c realChange, rng *rand.Rand) {
		defer g.release(c.keys)
		token, err := write(s, c.op, due, &WriteRequest{Writes: c.keys})
		if err != nil {
			return
		}
		time.Sleep(c.think)
		at := time.Now()
		ok, err := check(s, realCheckToken, at, [3]string{c.user, c.verify, c.object}, token)
		if err == nil && ok != c.want {
			s.violate(fmt.Sprintf("%s %s %s is %t %s after writing %v", c.user, c.verify, c.object, ok, time.Since(at)+c.think, c.keys[0]))
		}
		for _, rel := range []string{"writer", "admin"} {
			if strings.HasPrefix(c.object, "repo:") {
				_, _ = check(s, realCheckToken, time.Now(), [3]string{c.user, rel, c.object}, token)
			}
		}
		if rng.IntN(10) < 3 {
			list(s, realList, time.Now(), c.user)
		}
		time.Sleep(time.Duration(rng.Int64N(int64(2 * time.Second))))
		_, _ = write(s, c.op, time.Now(), &WriteRequest{Deletes: c.keys})
	}

	writesOnly := benchEnv("GEBRA_BENCH_WRITES", "")
	bulkSize := 1
	switch writesOnly {
	case "", "single":
	case "bulk":
		bulkSize = 100
	default:
		t.Fatalf("GEBRA_BENCH_WRITES is %q, want single or bulk", writesOnly)
	}
	mainOp := realCheck
	if writesOnly != "" {
		mainOp = realWrite
		if bulkSize > 1 {
			mainOp = realWriteBulk
		}
	}
	// added holds the committed changes of the write-only workload that are
	// not removed yet.
	var addedMu sync.Mutex
	var added []realChange

	var out []benchResult
	for _, rate := range rates {
		r := b.base
		r.Workload = "real"
		r.Load = benchLoadAvg()
		r.Rate = rate
		start := time.Now()
		s := &realStats{start: start.Add(warmup), samples: map[string][]time.Duration{}, errs: map[string]int{}}
		end := s.start.Add(d)
		var before benchMetrics
		if e != nil {
			before = b.metrics()
		}
		var reads, sessions sync.WaitGroup
		inflight := make(chan struct{}, 1024)
		due, scheduled := start, 0
		for {
			now := time.Now()
			if !now.Before(end) {
				break
			}
			for !due.After(now) {
				if !due.Before(s.start) {
					scheduled++
				}
				at := due
				switch p := g.rng.Float64(); {
				case writesOnly != "":
					addedMu.Lock()
					var c realChange
					remove := len(added) > 0 && p < 0.5
					if remove {
						i := g.rng.IntN(len(added))
						c = added[i]
						added[i] = added[len(added)-1]
						added = added[:len(added)-1]
					}
					addedMu.Unlock()
					if !remove {
						c = g.collaborators(bulkSize)
					}
					inflight <- struct{}{}
					reads.Add(1)
					go func() {
						defer func() { <-inflight; reads.Done() }()
						if remove {
							_, _ = write(s, c.op, at, &WriteRequest{Deletes: c.keys})
							g.release(c.keys)
							return
						}
						if _, err := write(s, c.op, at, &WriteRequest{Writes: c.keys}); err != nil {
							g.release(c.keys)
							return
						}
						addedMu.Lock()
						added = append(added, c)
						addedMu.Unlock()
					}()
				case p < writeShare/2:
					c := g.change()
					rng := rand.New(rand.NewPCG(g.rng.Uint64(), 0))
					sessions.Add(1)
					go func() {
						defer sessions.Done()
						session(s, at, c, rng)
					}()
				case p < writeShare/2+listShare:
					user := benchUser(g.userIdx())
					inflight <- struct{}{}
					reads.Add(1)
					go func() {
						defer func() { <-inflight; reads.Done() }()
						list(s, realList, at, user)
					}()
				default:
					q := g.check()
					inflight <- struct{}{}
					reads.Add(1)
					go func() {
						defer func() { <-inflight; reads.Done() }()
						_, _ = check(s, realCheck, at, q, "")
					}()
				}
				due = due.Add(time.Duration(g.rng.ExpFloat64() / rate * float64(time.Second)))
			}
			time.Sleep(time.Until(due))
		}
		reads.Wait()
		sessions.Wait()
		if await != nil {
			r.CatchUp = await().Seconds()
		}
		if e != nil {
			b.metrics().since(before, &r)
		}

		r.PerOp = map[string]benchOpStats{}
		for op, ds := range s.samples {
			slices.Sort(ds)
			q := func(p float64) float64 {
				return float64(ds[min(len(ds)-1, int(math.Ceil(p*float64(len(ds))))-1)]) / float64(time.Millisecond)
			}
			r.PerOp[op] = benchOpStats{
				N: len(ds), Ops: float64(len(ds)) / d.Seconds(), Errors: s.errs[op],
				P50: q(0.5), P99: q(0.99), P999: q(0.999), Max: q(1),
			}
			r.N += len(ds)
			r.Errors += s.errs[op]
		}
		// The dispatcher falls behind when every request in flight waits, so
		// the arrivals it dispatched give the rate the engine sustained.
		r.Achieved = float64(scheduled) / d.Seconds()
		r.Ops = r.Achieved
		r.P50, r.P99, r.Max = r.PerOp[mainOp].P50, r.PerOp[mainOp].P99, r.PerOp[mainOp].Max
		r.Violations = s.violations
		if s.violation != "" {
			r.Err = "violation: " + s.violation
		} else if s.lastErr != nil {
			r.Err = s.lastErr.Error()
		}
		t.Logf("%s real %.0f/s: achieved %.0f/s of %d due, %s p99 %.1f ms, %d violations", r.Variant, rate, r.Achieved, scheduled, mainOp, r.P99, r.Violations)
		out = append(out, r)
		if r.P99 > stopMs || r.Achieved < 0.9*rate {
			break
		}
	}
	return out
}
