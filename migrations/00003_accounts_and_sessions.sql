-- +goose Up

-- Accounts, and the sessions that speak for them (technical.md §9).
--
-- Account required up front, no anonymous play. Identity arrives from an OAuth
-- provider, so there is no password to store here and no anonymous→registered
-- merge path to build — the two things §9 rules out are ruled out by there
-- being nowhere to put them.
CREATE TABLE account (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity is (provider, subject) and never the email address. Google's
    -- `sub` is stable for the life of the account; an email can be changed by
    -- its owner and, on a workspace domain, reassigned to a different person —
    -- keying on it would eventually hand one player another's account.
    provider     text        NOT NULL,
    subject      text        NOT NULL,

    -- Refreshed from the provider at every sign-in. These are labels the player
    -- sees, not identity: nothing joins on them.
    email        text        NOT NULL,
    display_name text        NOT NULL,
    avatar_url   text,

    created_at   timestamptz NOT NULL DEFAULT now(),
    -- Touched at sign-in. §3.3 gates scheduled rollover work on recent
    -- activity, and this is the column that question will be asked of.
    last_seen_at timestamptz NOT NULL DEFAULT now(),

    UNIQUE (provider, subject)
);

-- Sessions are server-side, in Postgres (§9). The cookie carries a bearer token
-- and nothing else — no claims, no signature, nothing the client could be
-- trusted about — so signing out is one UPDATE here rather than a hope that the
-- browser discarded something.
CREATE TABLE session (
    -- sha256 of the token, never the token. The value in the cookie does not
    -- touch disk, so a leaked dump of this table cannot be replayed as a
    -- sign-in the way a table of live tokens could.
    token_hash bytea       PRIMARY KEY,

    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    created_at timestamptz NOT NULL DEFAULT now(),
    -- Absolute, and set to match the cookie's Max-Age exactly. Server and
    -- browser therefore agree on when a session ends; a sliding expiry would
    -- make one outlive the other.
    expires_at timestamptz NOT NULL,

    -- Set by signing out. The row is kept rather than deleted so that a cookie
    -- presented afterwards is a revoked session — a thing worth seeing in a log
    -- — rather than an unrecognised token indistinguishable from a typo.
    revoked_at timestamptz
);

-- Signing out everywhere, and "which devices am I signed in on", both read this.
CREATE INDEX session_account_idx ON session (account_id);
-- Read by the sweep that deletes dead sessions, which arrives with the job queue.
CREATE INDEX session_expires_at_idx ON session (expires_at);

-- command_idempotency.actor now holds an account id. The rows written before
-- accounts existed are all scoped to the single 'anonymous' actor, which no
-- request can ever present again, so they can only take up space.
DELETE FROM command_idempotency WHERE actor = 'anonymous';
