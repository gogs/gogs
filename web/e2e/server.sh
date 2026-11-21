#!/usr/bin/env bash
# Starts a throwaway Gogs for the browser tests, with one admin and nothing
# else, and removes it when the tests stop it. It takes the binary of
# `moon run gogs:build-prod`, which serves the built web app by itself.
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
binary=${GOGS_E2E_BINARY:-$root/.bin/gogs}
port=${GOGS_E2E_PORT:-3900}
dir=$(mktemp -d -t gogs-e2e-XXXXXX)
server=

cleanup() {
  if [ -n "$server" ]; then
    kill "$server" 2>/dev/null || true
  fi
  rm -rf "$dir"
}
trap cleanup EXIT
trap 'exit 0' INT TERM

cat >"$dir/app.ini" <<INI
RUN_USER = $(id -un)
RUN_MODE = prod

[server]
EXTERNAL_URL = http://localhost:$port/
DOMAIN = localhost
HTTP_ADDR = 127.0.0.1
HTTP_PORT = $port
; Nothing of the tests may reach the SSH keys of the user who runs them.
DISABLE_SSH = true
SSH_ROOT_PATH = $dir/ssh

[repository]
ROOT = $dir/repositories

[database]
TYPE = sqlite3
PATH = $dir/gogs.db

[security]
SECRET_KEY = $(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')

[session]
PROVIDER = file

[log]
MODE = console
LEVEL = Info
INI

export GOGS_WORK_DIR=$dir GOGS_CUSTOM=$dir/custom
"$binary" web --config "$dir/app.ini" &
server=$!

# The command that creates a user needs the tables, which the server creates
# when it starts. The tests wait for the account (see playwright.config.ts).
until curl --silent --output /dev/null "http://127.0.0.1:$port/"; do
  kill -0 "$server"
  sleep 0.2
done
# The account only exists in this throwaway instance (see `ADMIN` in admin.ts).
"$binary" admin create-user --config "$dir/app.ini" --admin \
  --name e2e-admin --password e2e-password-1234 --email e2e-admin@example.com

wait "$server"
