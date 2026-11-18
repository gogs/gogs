# Set up your development environment

Gogs is written in [Go](https://golang.org/), please take [A Tour of Go](https://tour.golang.org/) if you haven't done so!

## Outline

- [Environment](#environment)
- [Step 1: Install dependencies](#step-1-install-dependencies)
- [Step 2: Initialize your database](#step-2-initialize-your-database)
- [Step 3: Get the code](#step-3-get-the-code)
- [Step 4: Configure database settings](#step-4-configure-database-settings)
- [Step 5: Start the server](#step-5-start-the-server)
- [Build a production binary](#build-a-production-binary)
- [Run integration tests](#run-integration-tests)
- [Other nice things](#other-nice-things)

## Environment

Gogs is built and runs as a single binary and meant to be cross platform. Therefore, you should be able to develop Gogs in any major platforms you prefer.

## Step 1: Install dependencies

Gogs has the following dependencies:

- [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) (v2.43.1 or higher)
- [Go](https://golang.org/doc/install) (v1.27 or higher)
- [Node.js](https://nodejs.org/en/download) (v24 or higher) with [pnpm](https://pnpm.io/installation)
- [Moon](https://moonrepo.dev/docs/install)
- [portless](https://github.com/vercel-labs/portless)
- [golangci-lint](https://golangci-lint.run/welcome/install/), to run `moon run gogs:lint`
- Database upon your choice (pick one, we choose PostgreSQL in this document):
    - [PostgreSQL](https://wiki.postgresql.org/wiki/Detailed_installation_guides) (v12 or higher)
    - [MySQL](https://dev.mysql.com/downloads/mysql/) with `ENGINE=InnoDB` (v5.7 or higher)
    - [MariaDB](https://mariadb.org/) with `TYPE = mysql` (v10.6 or higher)
    - [SQLite3](https://www.sqlite.org/index.html)

### macOS

1. Install [Homebrew](https://brew.sh/).
1. Install dependencies:

    ```bash
    brew install go postgresql git node pnpm moon portless golangci-lint
    portless trust
    ```

    `portless trust` adds the local CA to your system trust store so `https://gogs.localhost` works without browser warnings. The `moon run gogs:dev` task will start the proxy and register the route automatically.

1. Configure PostgreSQL to start automatically:

    ```bash
    brew services start postgresql
    ```

1.  Ensure `psql`, the PostgreSQL command line client, is on your `$PATH`.
    Homebrew does not put it there by default. Homebrew gives you the command to run to insert `psql` in your path in the "Caveats" section of `brew info postgresql`. Alternatively, you can use the command below. It might need to be adjusted depending on your Homebrew prefix (`/usr/local` below) and shell (bash below).

    ```bash
    hash psql || { echo 'export PATH="/usr/local/opt/postgresql/bin:$PATH"' >> ~/.bash_profile }
    source ~/.bash_profile
    ```

### Ubuntu

1. Add the Node.js package repository:

    ```bash
    curl -fsSL https://deb.nodesource.com/setup_24.x | sudo -E bash -
    ```

1. Install dependencies:

    ```bash
    sudo apt install -y git postgresql postgresql-contrib nodejs
    sudo corepack enable
    sudo npm install -g portless
    portless trust
    ```

    `corepack enable` provides the pnpm version the repository pins in `package.json`.

1. Install [Go](https://go.dev/doc/install), since the `golang-go` package of most Ubuntu releases is older than the version Gogs needs.

1. Install [Moon](https://moonrepo.dev/docs/install) and [golangci-lint](https://golangci-lint.run/welcome/install/).

1. Configure startup services:

    ```bash
    sudo systemctl enable postgresql
    ```

## Step 2: Initialize your database

You need a fresh Postgres database and a database user that has full ownership of that database.

1. Create a database for the current Unix user:

    ```bash
    # For Linux users, first access the postgres user shell
    sudo su - postgres
    ```

    ```bash
    createdb
    ```

2. Create the Gogs user and password:

    ```bash
    createuser --superuser gogs
    psql -c "ALTER USER gogs WITH PASSWORD '<YOUR PASSWORD HERE>';"
    ```

3. Create the Gogs database

    ```bash
    createdb --owner=gogs --encoding=UTF8 --template=template0 gogs
    ```

## Step 3: Get the code

Generally, you don't need a full clone, so set `--depth` to `10`:

```bash
git clone --depth 10 https://github.com/gogs/gogs.git
```

> [!NOTE]
> The repository has Go modules enabled, please clone to somewhere outside of your `$GOPATH`.

## Step 4: Configure database settings

Create a `custom/conf/app.ini` file inside the repository and put the following configuration (everything under `custom/` directory is used to override default files and is excluded by `.gitignore`):

```ini
[database]
TYPE = postgres
HOST = 127.0.0.1:5432
NAME = gogs
USER = gogs
PASSWORD = <YOUR PASSWORD HERE>
SSL_MODE = disable
```

## Step 5: Start the server

The following command will start the web server and automatically recompile and restart the server if any Go files changed:

```bash
moon run gogs:dev
```

> [!NOTE]
> If you changed any file under `conf/`, `templates/` or `public/` directory, be sure to rerun `moon run gogs:dev` afterwards!

## Build a production binary

The following command builds the web frontend and a production binary with it embedded at `.bin/gogs`:

```bash
moon run gogs:build-prod
```

Copy `.bin/gogs` to your server and follow the [installation guide](https://gogs.io/getting-started/installation#configuration) from there. Use `moon run gogs:prod` to run it locally.

> [!NOTE]
> A plain `go build` produces a development binary, which serves the web frontend from the Vite dev server that `moon run gogs:dev` starts, so it shows no pages on its own.

## Run integration tests

The integration tests in `integration-tests` build Gogs, start it with a configuration of their own, and call it over HTTP. They connect to the database of that server and read the sessions it stores, so they seed data and read secrets, e.g., captcha text and two-factor authentication secrets, without going through the server. They do not need the web frontend:

```bash
moon run gogs:test-integration
```

A full run fails when a route of the server gets no request from any test, except the routes that render pages of the web frontend. The tests find every route in the source and compare them with the requests the server records, so a new route needs a test in the same change. A run of some tests, e.g., with `-run`, lists the routes left without failing.

The tests need PostgreSQL. Each run creates a new database on the server that the `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD` and `PGSSLMODE` environment variables point to, and drops it when the run passes.

The LDAP tests also need an LDAP server and are skipped unless `LDAP_HOST` is set. They sign in as an admin, the `LDAP_ADMIN_DN` with the `LDAP_ADMIN_PASSWORD`, on the server that `LDAP_HOST` and `LDAP_PORT` point to, and add the entries they need under `LDAP_BASE_DN`, which the server must have the `inetOrgPerson` and `posixGroup` object classes for. For example, with the server that CI uses:

```bash
docker run -d --name gogs-openldap -p 127.0.0.1:389:389 --ulimit nofile=1024:1024 \
  -e LDAP_DOMAIN=example.org -e LDAP_ADMIN_PASSWORD=admin osixia/openldap:1.5.0
export LDAP_HOST=localhost LDAP_PORT=389 LDAP_BASE_DN=dc=example,dc=org \
  LDAP_ADMIN_DN=cn=admin,dc=example,dc=org LDAP_ADMIN_PASSWORD=admin
```

The SMTP tests serve implicit TLS on port 465 and skip those cases when they cannot listen on it. On Linux, let unprivileged processes listen on it:

```bash
sudo sysctl -w net.ipv4.ip_unprivileged_port_start=465
```

## Other nice things

### Load email templates and static files from disk

When you are actively working on email templates and static files during development, you may want to enable the following configuration to avoid recompiling and restarting Gogs every time you make a change to files under `templates/` and `public/` directories:

```ini
RUN_MODE = dev

[server]
LOAD_ASSETS_FROM_DISK = true
```
