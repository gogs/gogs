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
// readable.
func parseSessionOptions(logger *logx.Logger) (session.Options, error) {
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
			logger.Error("Failed to collect expired sessions", "error", err)
		},
	}

	switch cmp.Or(strings.ToLower(conf.Session.Provider), "memory") {
	case "memory":
		opts.Initer = session.MemoryIniter()
		opts.Config = session.MemoryConfig{Lifetime: lifetime}
	case "file":
		opts.Initer = session.FileIniter()
		opts.Config = session.FileConfig{RootDir: conf.Session.ProviderConfig, Lifetime: lifetime}
	case "redis":
		redisOpts, err := parseRedisOptions(conf.Session.ProviderConfig)
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
		return session.Options{}, errors.Errorf("unsupported provider %q", conf.Session.Provider)
	}
	return opts, nil
}
