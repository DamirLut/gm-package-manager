<p align="center">
  <img src=".github/assets/gmpm.png" alt="gmpm logo" width="200" />
</p>

# GameMaker Package Manager
![Tests](https://github.com/damirlut/gm-package-manager/actions/workflows/ci.yml/badge.svg)
![GitHub Tag](https://img.shields.io/github/v/tag/damirlut/gm-package-manager)

A self-hosted package registry for [GameMaker](https://gamemaker.io) assets and plugins. It speaks the standard npm protocol, so it works with the regular `npm` CLI — no special client required.

## Features

- **npm-compatible registry** — `npm publish`, `npm install`, `npm unpublish`, dist-tags, and full semver versioning
- **GameMaker-aware metadata** — packages carry `gm.destination` and `gm.displayName` so the GameMaker IDE installs them to the right project folder
- **Website** — package search with autocomplete, package pages with readme rendering, version history, and dependency lists
- **Authentication** — token-based auth via `npm adduser`; first login auto-creates an account (can be disabled)
- **Access control** — per-package read/publish rules with glob patterns (`$all`, `$anonymous`, `$authenticated`, or specific users)
- **Audit log** — every publish/unpublish/login is recorded to a JSONL audit trail
- **Simple storage** — metadata in SQLite, tarballs on the local filesystem (S3 backend planned)

## Quick start

```sh
docker compose up -d
```

The stack is exposed on `http://localhost:3000` (override with `PORT`). Data persists in the `server-data` volume; images are published to `ghcr.io/damirlut/gm-package-manager/{server,client}`.

## Usage

Point npm at the registry and log in (first login creates the account):

```sh
npm config set registry http://localhost:3000
npm adduser --registry http://localhost:3000
```

Publish a GameMaker package (a `package.json` with `gm.destination` / `gm.displayName` plus a tarball):

```sh
npm publish
```

## Configuration (server env vars)

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_PATH` | `./storage/metadata.db` | SQLite database location |
| `DATABASE_AUTO_MIGRATE` | `true` | Run schema migrations on startup |
| `STORAGE_BACKEND` | `local` | Storage backend (`s3` not implemented yet) |
| `STORAGE_PATH` | `./storage` | Directory for tarballs and the audit log |
| `DISABLE_SIGNUP` | `false` | When `true`, only existing users can log in |
| `GITHUB_CLIENT_ID` | *(unset)* | GitHub OAuth app client id; enables the "Sign in with GitHub" button on the website |
| `GITHUB_CLIENT_SECRET` | *(unset)* | GitHub OAuth app client secret |

### GitHub sign-in setup

1. Create an OAuth App at <https://github.com/settings/developers> with the callback URL `https://<your-host>/-/auth/github/callback`.
2. Set `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` (see `docker-compose.yaml`).
3. First GitHub login creates a profile (unless `DISABLE_SIGNUP=true`); more provider accounts can be linked later from the `/account` page, which also shows login history, recent activity, and account deletion.

Website sessions live in the browser as a `gmpm_session` cookie and are independent from npm CLI tokens.

## Development

- **Server** — `cd packages/server && make run` (see `Makefile` for test, migrate, and integration targets; `make integration` runs the full npm-protocol flow in Docker)
- **Client** — `cd packages/client && bun install && bun run dev`

CI runs server unit tests, a Docker-based npm-protocol integration suite, and client checks (typecheck, lint, build). Pushing a tag like `v0.0.2` builds and publishes images to GHCR.
