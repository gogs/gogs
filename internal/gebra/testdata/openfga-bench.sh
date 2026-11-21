#!/usr/bin/env bash
# Runs OpenFGA for the HTTP benchmark in applierbench_http_test.go: downloads
# a pinned release, recreates its database on Postgres, migrates it, and runs
# the server in the foreground on 127.0.0.1:18080. Stop it with Ctrl-C.
#
# PGHOST, PGPORT, PGUSER and PGPASSWORD select the Postgres server, which
# should be the one that Gebra's benchmark uses. OPENFGA_DB (default
# openfga_bench) is the database, which is dropped and created anew, and
# OPENFGA_CONNS (default 96, as GEBRA_BENCH_CONNS) the connection pool size.
# The caches' TTL of 1s is the max staleness that the benchmark gives
# cmd/gebra.
set -euo pipefail

version=1.23.0
sha256=06a02dc0034188eb35004aed0a010f41d332e31a6b4db4c75e1d89de05fc1441
dir="${OPENFGA_DIR:-/tmp/openfga-$version}"
db="${OPENFGA_DB:-openfga_bench}"
conns="${OPENFGA_CONNS:-96}"
export PGHOST="${PGHOST:-127.0.0.1}" PGPORT="${PGPORT:-5432}" PGUSER="${PGUSER:-postgres}" PGPASSWORD="${PGPASSWORD:-postgres}"

if [[ ! -x "$dir/openfga" ]]; then
	mkdir -p "$dir"
	tarball="$dir/openfga_${version}_linux_amd64.tar.gz"
	curl -fsSL -o "$tarball" "https://github.com/openfga/openfga/releases/download/v$version/openfga_${version}_linux_amd64.tar.gz"
	echo "$sha256  $tarball" | sha256sum -c -
	tar -xzf "$tarball" -C "$dir" openfga
fi

psql -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS \"$db\" WITH (FORCE)" -c "CREATE DATABASE \"$db\""

datastore=(
	--datastore-engine postgres
	--datastore-uri "postgres://$PGHOST:$PGPORT/$db?sslmode=disable"
	--datastore-username "$PGUSER"
	--datastore-password "$PGPASSWORD"
)
"$dir/openfga" migrate "${datastore[@]}"

exec "$dir/openfga" run "${datastore[@]}" \
	--datastore-max-open-conns "$conns" \
	--datastore-max-idle-conns "$conns" \
	--check-query-cache-enabled \
	--check-query-cache-ttl 1s \
	--check-iterator-cache-enabled \
	--check-iterator-cache-ttl 1s \
	--list-objects-iterator-cache-enabled \
	--list-objects-iterator-cache-ttl 1s \
	--check-cache-limit 1000000 \
	--cache-controller-enabled=false \
	--shared-iterator-enabled \
	--experimentals enable-check-optimizations,enable-list-objects-optimizations \
	--listObjects-max-results 50 \
	--listObjects-deadline 10s \
	--request-timeout 10s \
	--http-upstream-timeout 10s \
	--max-tuples-per-write 100 \
	--http-addr 127.0.0.1:18080 \
	--grpc-addr 127.0.0.1:18081 \
	--metrics-addr 127.0.0.1:18082 \
	--authn-method none \
	--playground-enabled=false \
	--profiler-enabled=false \
	--trace-enabled=false \
	--log-level warn \
	--log-format json
