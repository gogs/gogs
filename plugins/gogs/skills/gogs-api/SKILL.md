---
name: gogs-api
description: >-
  Use the Gogs REST API (/api/v1, GitHub v3-style) with access tokens:
  create tokens, manage repositories, migrations and mirrors, file
  contents, issues, labels, milestones, webhooks, collaborators, deploy
  keys, organizations and admin user management, with Link-header
  pagination. Use when automating a Gogs instance.
---

# Gogs REST API

References: `docs/api-reference/introduction.mdx`,
`docs/api-reference/openapi.json`, `docs/advancing/webhooks.mdx`.

## Auth

```bash
# Basic auth only to mint a token (not possible with 2FA; use user settings instead)
curl -u alice -X POST https://gogs.example.com/api/v1/users/alice/tokens \
  -H 'Content-Type: application/json' -d '{"name":"automation"}'

export GOGS=https://gogs.example.com/api/v1 TOKEN=...
curl -H "Authorization: token $TOKEN" $GOGS/user
```

Missing permissions can surface as `404 Not Found` rather than 403.

## Common calls

```bash
H=(-H "Authorization: token $TOKEN" -H 'Content-Type: application/json')

curl "${H[@]}" -X POST $GOGS/user/repos -d '{"name":"hello","private":true,"auto_init":true,"readme":"Default"}'
curl "${H[@]}" -X POST $GOGS/repos/migrate -d '{"clone_addr":"https://github.com/o/r.git","uid":1,"repo_name":"r","mirror":true}'
curl "${H[@]}" -X POST $GOGS/repos/alice/hello/mirror-sync
curl "${H[@]}" $GOGS/repos/alice/hello/raw/main/README.md
curl "${H[@]}" -X PUT $GOGS/repos/alice/hello/contents/docs/a.md \
  -d "{\"message\":\"add a.md\",\"content\":\"$(base64 -w0 a.md)\",\"branch\":\"main\"}"
curl "${H[@]}" -X POST $GOGS/repos/alice/hello/issues -d '{"title":"Bug","body":"Steps..."}'
curl "${H[@]}" -X POST $GOGS/repos/alice/hello/hooks \
  -d '{"type":"gogs","config":{"url":"https://ci.example.com/hook","content_type":"json","secret":"..."},"events":["push"],"active":true}'
```

`uid` in migrate is the owner's user or org ID (`GET /users/{name}`).

## Admin endpoints

Site admins can `POST /admin/users`, `PATCH/DELETE /admin/users/{name}`,
create repos/orgs for users, and manage team members and repos under
`/admin/teams/...`. Confirm before deleting users or repositories.

## Pagination

Lists are 1-based via `?page=`. Follow the `Link` header (`next`, `last`,
`first`, `prev`) rather than building URLs.

## Go client

`github.com/gogs/go-gogs-client` wraps the same API.
