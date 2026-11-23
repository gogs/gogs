package web

import (
	"cmp"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/session"
	"github.com/flamego/session/redis"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

// parseSessionOptions returns the options of the session middleware from the
// "[session]" section, keeping the session IDs and data stored by Macaron
// readable. The cookie is scoped to subpath and kept for loginRememberDays.
func parseSessionOptions(logger *logx.Logger, sessionOpts conf.SessionOpts, subpath string, loginRememberDays int) (session.Options, error) {
	lifetime := time.Duration(sessionOpts.MaxLifeTime) * time.Second
	opts := session.Options{
		Cookie: session.CookieOptions{
			Name:     sessionOpts.CookieName,
			Path:     cmp.Or(subpath, "/"),
			MaxAge:   86400 * loginRememberDays,
			Secure:   sessionOpts.CookieSecure,
			HTTPOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
		GCInterval: time.Duration(sessionOpts.GCInterval) * time.Second,
		ErrorFunc: func(err error) {
			logger.Error("Failed to collect expired sessions", "error", err)
		},
	}

	switch cmp.Or(strings.ToLower(sessionOpts.Provider), "memory") {
	case "memory":
		opts.Initer = session.MemoryIniter()
		opts.Config = session.MemoryConfig{Lifetime: lifetime}
	case "file":
		opts.Initer = session.FileIniter()
		opts.Config = session.FileConfig{RootDir: sessionOpts.ProviderConfig, Lifetime: lifetime}
	case "redis":
		redisOpts, err := parseRedisOptions(sessionOpts.ProviderConfig)
		if err != nil {
			return session.Options{}, errors.Wrap(err, "parse redis options")
		}
		if redisOpts.prefix != "" {
			logger.Warn("The key prefix of the Redis session store is not supported and is ignored", "prefix", redisOpts.prefix)
		}
		opts.Initer = redis.Initer()
		opts.Config = redis.Config{
			Options: &redis.Options{
				Network:         redisOpts.network,
				Addr:            redisOpts.addr,
				Password:        redisOpts.password,
				DB:              redisOpts.db,
				PoolSize:        redisOpts.poolSize,
				ConnMaxIdleTime: redisOpts.idleTimeout,
				TLSConfig:       redisOpts.tlsConfig,
			},
			Lifetime: lifetime,
		}
	default:
		return session.Options{}, errors.Errorf("unsupported provider %q", sessionOpts.Provider)
	}
	return opts, nil
}
