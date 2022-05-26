# Set up your development environment

Gogs is written in [Go](https://golang.org/), please take [A Tour of Go](https://tour.golang.org/) if you haven't done so!

## Outline

- [Environment](#environment)
- [Step 1: Install dependencies](#step-1-install-dependencies)
- [Step 2: Initialize your database](#step-2-initialize-your-database)
- [Step 3: Get the code](#step-3-get-the-code)
- [Step 4: Configure database settings](#step-4-configure-database-settings)
- [Step 5: Start the server](#step-5-start-the-server)
- [Other nice things](#other-nice-things)

## Environment

Gogs is built and runs as a single binary and meant to be cross platform. Therefore, you should be able to develop Gogs in any major platforms you prefer.

## Step 1: Install dependencies

Gogs has the following dependencies:

- [Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) (v1.8.3 or higher)
- [Go](https://golang.org/doc/install) (v1.16 or higher)
- [Less.js](http://lesscss.org/usage/#command-line-usage-installing)
- [Task](https://github.com/go-task/task) (v3)
- Database upon your choice (pick one, we choose PostgreSQL in this document):
    - [PostgreSQL](https://wiki.postgresql.org/wiki/Detailed_installation_guides) (v9.6 or higher)
    - [MySQL](https://dev.mysql.com/downloads/mysql/) with `ENGINE=InnoDB` (v5.7 or higher)
    - [SQLite3](https://www.sqlite.org/index.html)
    - [TiDB](https://github.com/pingcap/tidb)

### macOS

1. Install [Homebrew](https://brew.sh/).
1. Install dependencies:

    ```bash
    brew install go postgresql git npm go-task/tap/go-task
    npm install -g less
    npm install -g less-plugin-clean-css
    ```

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

1. Add package repositories:

    ```bash
    curl -sL https://deb.nodesource.com/setup_10.x | sudo -E bash -
    ```

1. Update repositories:

    ```bash
    sudo apt-get update
    ```

1. Install dependencies:

    ```bash
    sudo apt install -y make git-all postgresql postgresql-contrib golang-go nodejs
    npm install -g less
    go install github.com/go-task/task/v3/cmd/task@latest
    ```

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

**NOTE** The repository has Go modules enabled, please clone to somewhere outside of your `$GOPATH`.

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
task web --watch
```

**NOTE** If you changed any file under `conf/`, `template/` or `public/` directory, be sure to run `task generate` afterwards!

## Other nice things

### Load HTML templates and static files from disk

When you are actively working on HTML templates and static files during development, you may want to enable the following configuration to avoid recompiling and restarting Gogs every time you make a change to files under `template/` and `public/` directories:

```ini
RUN_MODE = dev

[server]
LOAD_ASSETS_FROM_DISK = true
```

### Offline development

Sometimes you will want to develop Gogs but it just so happens you will be on a plane or a train or perhaps a beach, and you will have no WiFi. And you may raise your fist toward heaven and say something like, "Why, we can put a man on the moon, so why can't we develop high-quality Git hosting without an Internet connection?" But lower your hand back to your keyboard and fret no further, you *can* develop Gogs with no connectivity by setting the following configuration in your `custom/conf/app.ini`:

```ini
[server]
OFFLINE_MODE = true
```
