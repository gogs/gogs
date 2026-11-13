package web

import (
	"cmp"
	"crypto/tls"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/cache/redis"
	"unknwon.dev/ini/v2"

	"gogs.io/gogs/internal/conf"
)

func parseCacheOptions(confOpts conf.CacheOptions) (cache.Options, error) {
	opts := cache.Options{
		GCInterval: time.Duration(confOpts.Interval) * time.Second,
	}

	switch cmp.Or(strings.ToLower(confOpts.Adapter), "memory") {
	case "memory":
		opts.Initer = cache.MemoryIniter()
	case "file":
		opts.Initer = cache.FileIniter()
		opts.Config = cache.FileConfig{RootDir: confOpts.Host}
	case "redis":
		redisOpts, err := parseRedisOptions(confOpts.Host)
		if err != nil {
			return cache.Options{}, errors.Wrap(err, "parse redis options")
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
			KeyPrefix: redisOpts.prefix,
		}
	default:
		return cache.Options{}, errors.Errorf("unsupported adapter %q", confOpts.Adapter)
	}
	return opts, nil
}

// redisOptions are the Redis connection options of the cache and session
// stores.
type redisOptions struct {
	network     string
	addr        string
	password    string
	db          int
	poolSize    int
	idleTimeout time.Duration
	prefix      string
	tlsConfig   *tls.Config
}

// parseRedisOptions parses the comma-separated options in the format of
// go-macaron's Redis adapters, e.g., "network=tcp,addr=:6379,db=0".
func parseRedisOptions(s string) (redisOptions, error) {
	cfg, err := ini.Load([]byte(strings.ReplaceAll(s, ",", "\n")))
	if err != nil {
		return redisOptions{}, errors.Wrap(err, "load options")
	}

	var opts redisOptions
	for k, v := range cfg.Section("").KeyValues() {
		switch k {
		case "network":
			opts.network = v
		case "addr":
			opts.addr = v
		case "password":
			opts.password = v
		case "db":
			opts.db, err = strconv.Atoi(v)
			if err != nil {
				return redisOptions{}, errors.Wrapf(err, "parse db %q", v)
			}
		case "pool_size":
			opts.poolSize, err = strconv.Atoi(v)
			if err != nil {
				return redisOptions{}, errors.Wrapf(err, "parse pool_size %q", v)
			}
		case "idle_timeout":
			opts.idleTimeout, err = time.ParseDuration(v + "s")
			if err != nil {
				return redisOptions{}, errors.Wrapf(err, "parse idle_timeout %q", v)
			}
		case "prefix":
			opts.prefix = v
		case "tls":
			// Matches go-macaron's Redis adapters: any non-empty `tls=` value enables
			// TLS with InsecureSkipVerify.
			opts.tlsConfig = &tls.Config{InsecureSkipVerify: true}
		case "hset_name":
			// Macaron stored values in a single Redis hash named by this key,
			// whereas Flamego stores per-key with KeyPrefix, so this knob has no equivalent.
		default:
			return redisOptions{}, errors.Errorf("unsupported option %q", k)
		}
	}
	return opts, nil
}
