package web

import (
	"crypto/tls"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/cache"
	"github.com/flamego/cache/postgres"
	"github.com/flamego/cache/redis"
	"gopkg.in/ini.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/strx"
)

func parseCacheOptions(confOpts conf.CacheOptions) (cache.Options, error) {
	opts := cache.Options{
		GCInterval: time.Duration(confOpts.Interval) * time.Second,
	}

	switch strx.Coalesce(strings.ToLower(confOpts.Adapter), "memory") {
	case "memory":
		opts.Initer = cache.MemoryIniter()
	case "file":
		opts.Initer = cache.FileIniter()
		opts.Config = cache.FileConfig{RootDir: confOpts.Host}
	case "redis":
		cfg, err := parseRedisConfig(confOpts.Host)
		if err != nil {
			return cache.Options{}, errors.Wrap(err, "parse redis config")
		}
		opts.Initer = redis.Initer()
		opts.Config = cfg
	case "postgres":
		opts.Initer = postgres.Initer()
		opts.Config = postgres.Config{DSN: confOpts.Host, InitTable: true}
	default:
		return cache.Options{}, errors.Errorf("unsupported adapter %q", confOpts.Adapter)
	}
	return opts, nil
}

func parseRedisConfig(host string) (redis.Config, error) {
	cfg, err := ini.Load([]byte(strings.ReplaceAll(host, ",", "\n")))
	if err != nil {
		return redis.Config{}, errors.Wrap(err, "load HOST")
	}

	var config redis.Config
	for k, v := range cfg.Section("").KeysHash() {
		switch k {
		case "network":
			config.Options.Network = v
		case "addr":
			config.Options.Addr = v
		case "password":
			config.Options.Password = v
		case "db":
			n, err := strconv.Atoi(v)
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse db %q", v)
			}
			config.Options.DB = n
		case "pool_size":
			n, err := strconv.Atoi(v)
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse pool_size %q", v)
			}
			config.Options.PoolSize = n
		case "idle_timeout":
			d, err := time.ParseDuration(v + "s")
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse idle_timeout %q", v)
			}
			config.Options.ConnMaxIdleTime = d
		case "prefix":
			config.KeyPrefix = v
		case "tls":
			// Matches go-macaron/session/redis: any non-empty `tls=` value enables
			// TLS with InsecureSkipVerify.
			config.Options.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		case "hset_name":
			// Macaron stored values in a single Redis hash named by this key,
			// whereas Flamego stores per-key with KeyPrefix, so this knob has no equivalent.
		default:
			return redis.Config{}, errors.Errorf("unsupported redis HOST key %q", k)
		}
	}
	return config, nil
}
