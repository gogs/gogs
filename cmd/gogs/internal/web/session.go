package web

import (
	"cmp"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/session"
	"github.com/flamego/session/redis"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
)

// parseSessionOptions returns the options of the session middleware from the
// "[session]" section, keeping the session IDs and data stored by Macaron
// readable.
func parseSessionOptions() (session.Options, error) {
	lifetime := time.Duration(conf.Session.MaxLifeTime) * time.Second
	opts := session.Options{
		Cookie: session.CookieOptions{
			Name:     conf.Session.CookieName,
			Path:     cmp.Or(conf.Server.Subpath, "/"),
			MaxAge:   86400 * conf.Security.LoginRememberDays,
			Secure:   conf.Session.CookieSecure,
			HTTPOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		GCInterval: time.Duration(conf.Session.GCInterval) * time.Second,
		ErrorFunc: func(err error) {
			log.Error("Failed to collect expired sessions: %v", err)
		},
	}

	var touch func(sid string) error
	switch cmp.Or(strings.ToLower(conf.Session.Provider), "memory") {
	case "memory":
		opts.Initer = session.MemoryIniter()
		opts.Config = session.MemoryConfig{Lifetime: lifetime}
	case "file":
		rootDir := conf.Session.ProviderConfig
		opts.Initer = session.FileIniter()
		opts.Config = session.FileConfig{RootDir: rootDir, Lifetime: lifetime}
		// The file store expires a session by the modification time of its file,
		// which a request that only reads the session does not change.
		touch = func(sid string) error {
			now := time.Now()
			err := os.Chtimes(filepath.Join(rootDir, sid[:1], sid[1:2], sid), now, now)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
	case "redis":
		redisOpts, err := parseRedisOptions(conf.Session.ProviderConfig)
		if err != nil {
			return session.Options{}, errors.Wrap(err, "parse redis options")
		}
		if redisOpts.prefix != "" {
			log.Warn("The key prefix of the Redis session store is not supported and is ignored: %q", redisOpts.prefix)
		}
		opts.Initer = redis.Initer()
		opts.Config = redis.Config{
			Options: &redis.Options{
				Network:     redisOpts.network,
				Addr:        redisOpts.addr,
				Password:    redisOpts.password,
				DB:          redisOpts.db,
				PoolSize:    redisOpts.poolSize,
				IdleTimeout: redisOpts.idleTimeout,
				TLSConfig:   redisOpts.tlsConfig,
			},
			Lifetime: lifetime,
		}
	default:
		return session.Options{}, errors.Errorf("unsupported provider %q", conf.Session.Provider)
	}
	opts.Initer = saveChangesOnly(opts.Initer, touch)
	return opts, nil
}

// saveChangesOnly wraps the store of the initer to save a session only when it
// is new or the request changed it. Otherwise, a request that reads the session
// while another request changes it would write back the data it read. For
// sessions that are not saved, touch keeps them from expiring, unless it is nil.
func saveChangesOnly(initer session.Initer, touch func(sid string) error) session.Initer {
	return func(ctx context.Context, args ...any) (session.Store, error) {
		store, err := initer(ctx, args...)
		if err != nil {
			return nil, err
		}
		return &changeSavingStore{Store: store, touch: touch}, nil
	}
}

type changeSavingStore struct {
	session.Store
	touch func(sid string) error
}

func (s *changeSavingStore) Read(ctx context.Context, sid string) (session.Session, error) {
	isNew := !s.Exist(ctx, sid)
	sess, err := s.Store.Read(ctx, sid)
	if err != nil {
		return nil, err
	}
	return &changeTrackingSession{Session: sess, changed: isNew}, nil
}

func (s *changeSavingStore) Save(ctx context.Context, sess session.Session) error {
	tracked, ok := sess.(*changeTrackingSession)
	if !ok {
		return s.Store.Save(ctx, sess)
	}
	if tracked.changed {
		return s.Store.Save(ctx, tracked.Session)
	}
	if s.touch != nil {
		return s.touch(sess.ID())
	}
	return nil
}

// changeTrackingSession records whether the session was changed.
type changeTrackingSession struct {
	session.Session
	changed bool
}

func (s *changeTrackingSession) Set(key, val any) {
	s.Session.Set(key, val)
	s.changed = true
}

func (s *changeTrackingSession) SetFlash(val any) {
	s.Session.SetFlash(val)
	s.changed = true
}

func (s *changeTrackingSession) Delete(key any) {
	// The session middleware deletes the flash of every request.
	if s.Get(key) == nil {
		return
	}
	s.Session.Delete(key)
	s.changed = true
}

func (s *changeTrackingSession) Flush() {
	s.Session.Flush()
	s.changed = true
}
