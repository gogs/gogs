---
name: gogs-install
description: >-
  Install and configure a self-hosted Gogs Git service: database setup
  (PostgreSQL, MySQL/MariaDB, SQLite), binary or Docker install, a minimal
  custom/conf/app.ini with environment-expanded secrets, GOGS_CUSTOM, and
  creating the first admin. Use when setting up or reconfiguring a Gogs
  server.
---

# Installing Gogs

References: `docs/getting-started/installation.mdx`,
`docs/fine-tuning/configuration-primer.mdx`,
`docs/fine-tuning/reverse-proxy.mdx`, `docs/fine-tuning/run-as-service.mdx`.

## Prerequisites

- Database: PostgreSQL >= 12, MySQL >= 5.7, MariaDB >= 10.6, or SQLite 3
- Git >= 2.43.1 on the server
- An SSH server only for Git over SSH (a builtin one is available)

## Database

```bash
# PostgreSQL
psql -c "CREATE USER gogs WITH PASSWORD '...';"
psql -c "CREATE DATABASE gogs OWNER gogs ENCODING 'UTF8';"

# MySQL / MariaDB (use TYPE = mysql for both)
mysql -u root -p < scripts/mysql.sql
```

## Install

- Binary: download from https://dl.gogs.io or GitHub releases, then
  `gogs web` (`gogs web --help` for flags, `-p` for the port).
- Docker: `docker-next/` is the modern non-root image without container
  options; `docker/` is the traditional root image with many options.
- Source: follow "Build a production binary" in
  `docs/dev/local_development.md`.

## Minimal app.ini

`custom/conf/app.ini` overlays the shipped defaults, so set only what you
change:

```ini
RUN_MODE = prod

[server]
EXTERNAL_URL = https://gogs.example.com/
DOMAIN       = gogs.example.com

[database]
TYPE     = postgres
HOST     = 127.0.0.1:5432
NAME     = gogs
USER     = gogs
PASSWORD = ${GOGS_DATABASE_PASSWORD}

[security]
SECRET_KEY = ${GOGS_SECURITY_SECRET_KEY}
```

- `${...}` expands from the environment at startup, keeping secrets out of
  the file (systemd: `Environment=`).
- Gogs refuses to start with the default `SECRET_KEY`; use a long random
  value.
- `custom/` sits next to the binary unless `GOGS_CUSTOM` points elsewhere.

## First admin

The first user to sign up becomes admin. Or, on the server:

```bash
gogs admin create-user --name admin --password "$PASSWORD" \
  --email admin@example.com --admin --config custom/conf/app.ini
```

For HTTPS and running as a service, see the reverse proxy and
run-as-service guides.
