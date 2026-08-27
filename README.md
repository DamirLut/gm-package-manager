# GameMaker Package Manager

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

## Development

- **Server** — `cd packages/server && make run` (see `Makefile` for test, migrate, and integration targets; `make integration` runs the full npm-protocol flow in Docker)
- **Client** — `cd packages/client && bun install && bun run dev`

CI runs server unit tests, a Docker-based npm-protocol integration suite, and client checks (typecheck, lint, build). Pushing a tag like `v0.0.2` builds and publishes images to GHCR.
