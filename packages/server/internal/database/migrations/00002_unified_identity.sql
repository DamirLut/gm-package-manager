-- +goose Up
ALTER TABLE users RENAME TO users_old;
ALTER TABLE tokens RENAME TO tokens_old;

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMP NOT NULL,
    updated_at    TIMESTAMP NOT NULL
);

CREATE TABLE tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         TEXT NOT NULL DEFAULT 'user',
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    scopes       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    expires_at   TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    revoked_at   TIMESTAMP,
    created_ip   TEXT NOT NULL
);

INSERT INTO users SELECT id, username, password_hash, status, created_at, updated_at FROM users_old;
INSERT INTO tokens SELECT id, user_id, type, token_hash, prefix, scopes, created_at, expires_at, last_used_at, revoked_at, created_ip FROM tokens_old;

DROP TABLE tokens_old;
DROP TABLE users_old;

-- External provider accounts linked to one profile.
CREATE TABLE identities (
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    provider_account_id TEXT NOT NULL,
    username            TEXT NOT NULL,
    email               TEXT,
    avatar_url          TEXT,
    created_at          TIMESTAMP NOT NULL,
    updated_at          TIMESTAMP NOT NULL,
    UNIQUE (provider, provider_account_id),
    UNIQUE (user_id, provider)
);

-- Browser sessions for the website.
CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_ip TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

-- One-time CSRF secrets for the OAuth round trip.
CREATE TABLE oauth_states (
    id         INTEGER PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,
    provider   TEXT NOT NULL,
    user_id    INTEGER NOT NULL DEFAULT 0,
    redirect   TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

-- Per-user feed behind the profile's login history and recent activity.
CREATE TABLE events (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    ip         TEXT,
    user_agent TEXT,
    provider   TEXT,
    detail     TEXT,
    created_at TIMESTAMP NOT NULL
);
CREATE INDEX idx_events_user ON events (user_id, created_at DESC);

-- +goose Down
DROP INDEX idx_events_user;
DROP TABLE events;
DROP TABLE oauth_states;
DROP TABLE sessions;
DROP TABLE identities;

ALTER TABLE users RENAME TO users_old;
ALTER TABLE tokens RENAME TO tokens_old;

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMP NOT NULL,
    updated_at    TIMESTAMP NOT NULL
);

CREATE TABLE tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id),
    type         TEXT NOT NULL DEFAULT 'user',
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    scopes       TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL,
    expires_at   TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    revoked_at   TIMESTAMP,
    created_ip   TEXT NOT NULL
);

-- password-only accounts survive; tokens of dropped users go with them
INSERT INTO users SELECT id, username, password_hash, status, created_at, updated_at FROM users_old WHERE password_hash IS NOT NULL;
INSERT INTO tokens SELECT id, user_id, type, token_hash, prefix, scopes, created_at, expires_at, last_used_at, revoked_at, created_ip FROM tokens_old WHERE user_id IN (SELECT id FROM users);

DROP TABLE tokens_old;
DROP TABLE users_old;
