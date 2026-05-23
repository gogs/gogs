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
	goredis "github.com/redis/go-redis/v9"
	"gopkg.in/ini.v1"

	"gogs.io/gogs/internal/conf"
)

// webAPICacherOptions translates the [cache] section in app.ini into the
// equivalent flamego cache initer + config, mirroring the adapter set and DSN
// conventions that the macaron cache middleware accepts. Adapter values map
// 1:1 to flamego's adapters; HOST is interpreted per-adapter the same way
// macaron interpreted AdapterConfig.
func webAPICacherOptions() (cache.Options, error) {
	opts := cache.Options{GCInterval: time.Duration(conf.Cache.Interval) * time.Second}

	switch strings.ToLower(conf.Cache.Adapter) {
	case "", "memory":
		opts.Initer = cache.MemoryIniter()
	case "file":
		opts.Initer = cache.FileIniter()
		opts.Config = cache.FileConfig{RootDir: conf.Cache.Host}
	case "redis":
		cfg, err := redisConfigFromHost(conf.Cache.Host)
		if err != nil {
			return cache.Options{}, errors.Wrap(err, "parse redis HOST")
		}
		opts.Initer = redis.Initer()
		opts.Config = cfg
	case "postgres":
		opts.Initer = postgres.Initer()
		opts.Config = postgres.Config{DSN: conf.Cache.Host, InitTable: true}
	default:
		return cache.Options{}, errors.Errorf("unrecognized cache adapter %q", conf.Cache.Adapter)
	}
	return opts, nil
}

// redisConfigFromHost parses the macaron-style comma-separated key=value
// connection string into a flamego redis cache config. Recognized keys:
// network, addr, password, db, pool_size, idle_timeout, prefix, tls. The
// `hset_name` key from macaron has no flamego equivalent and is ignored.
// `tls=true` mirrors macaron's session/redis behavior (InsecureSkipVerify).
func redisConfigFromHost(host string) (redis.Config, error) {
	cfg, err := ini.Load([]byte(strings.ReplaceAll(host, ",", "\n")))
	if err != nil {
		return redis.Config{}, errors.Wrap(err, "load HOST")
	}

	out := redis.Config{Options: &goredis.Options{Network: "tcp"}}
	for k, v := range cfg.Section("").KeysHash() {
		switch k {
		case "network":
			out.Options.Network = v
		case "addr":
			out.Options.Addr = v
		case "password":
			out.Options.Password = v
		case "db":
			n, err := strconv.Atoi(v)
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse db %q", v)
			}
			out.Options.DB = n
		case "pool_size":
			n, err := strconv.Atoi(v)
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse pool_size %q", v)
			}
			out.Options.PoolSize = n
		case "idle_timeout":
			d, err := time.ParseDuration(v + "s")
			if err != nil {
				return redis.Config{}, errors.Wrapf(err, "parse idle_timeout %q", v)
			}
			out.Options.ConnMaxIdleTime = d
		case "prefix":
			out.KeyPrefix = v
		case "tls":
			// Matches go-macaron/session/redis: any non-empty `tls=` value enables
			// TLS with InsecureSkipVerify, used by operators connecting to managed
			// Redis endpoints whose certs aren't in the local trust store.
			out.Options.TLSConfig = &tls.Config{InsecureSkipVerify: true}
		case "hset_name":
			// macaron stored values in a single Redis hash named by this key;
			// flamego stores per-key with KeyPrefix, so this knob has no equivalent.
		default:
			return redis.Config{}, errors.Errorf("unsupported redis HOST key %q", k)
		}
	}
	return out, nil
}
