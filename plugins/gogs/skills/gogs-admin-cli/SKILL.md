---
name: gogs-admin-cli
description: >-
  Maintain a Gogs instance from the command line: gogs admin subcommands
  (create users, clean archives and missing repositories, git gc, rewrite
  authorized_keys, resync hooks) and gogs backup / restore for migrations
  and database switches. Use for Gogs server maintenance, backups, or
  moving to another server or database.
---

# Gogs admin CLI

Reference: `docs/advancing/cli-reference.mdx`.

Run these on the Gogs server, as the user that runs Gogs, with the same
`custom/conf/app.ini` (`--config` if it isn't the default).

## Maintenance

| Command | Purpose |
|---------|---------|
| `gogs admin create-user --name N --password P --email E [--admin]` | Create a user |
| `gogs admin delete-inactive-users` | Remove never-activated accounts |
| `gogs admin delete-repository-archives` | Clear generated archive files |
| `gogs admin delete-missing-repositories` | Drop DB records whose Git data is gone |
| `gogs admin reinit-missing-repositories` | Recreate missing bare repos on disk |
| `gogs admin collect-garbage` | `git gc` across all repositories |
| `gogs admin resync-hooks` | Rewrite server-side Git hooks |
| `gogs admin rewrite-authorized-keys` | Regenerate SSH `authorized_keys` |

- `rewrite-authorized-keys` replaces the whole file; non-Gogs keys in it
  are lost. Check with the user first.
- `delete-missing-repositories` and `delete-inactive-users` delete data.
  Take a backup and confirm before running them.
- After moving repositories or the binary path, run `resync-hooks`.

## Backup and restore

```bash
gogs backup                                  # zip of database, repositories and related files
gogs backup --target /backups --exclude-mirror-repos
gogs backup --database-only
gogs restore --from gogs-backup-XXXX.zip
gogs restore --from gogs-backup-XXXX.zip --database-only
```

`--database-only` and `--exclude-repos` work on both commands. Restore is
also the way to migrate servers or switch database engines: back up,
point the new `app.ini` at the new database, then restore. Stop the web
server during restore and confirm with the user, since it overwrites
existing data.

## Internal commands

`gogs serv` and `gogs hook` are used by SSH and Git hooks. Don't run them
by hand.
