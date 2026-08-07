-- +goose Up

-- The daily rollover's own ledger (§3.3).
--
-- Owned by internal/rollover, which is the only thing that reads or writes it
-- (§3.1). One row per player, holding the last local day the world was advanced
-- through for them and a little bookkeeping about the run that did it.
--
-- It is deliberately not health_panel.last_day. That column is the *body's*
-- ledger and answers "has this body lived that day"; this one answers "has the
-- world been advanced for this player through that day", which is a bigger
-- question the moment the backlog (#21), Karma decay (#15) and environment
-- ceilings (#29) land. Reusing the body's date would make every one of those
-- modules depend on health having been the thing that moved, and would put the
-- rollover's own idempotency at the mercy of a player pressing a button.
CREATE TABLE rollover (
    account_id uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,

    -- The last local day rolled over, in the player's own calendar. A DATE
    -- carries no timezone and none is wanted: it is already their day.
    --
    -- Created at the day before the player's current one, so a brand-new
    -- account has nothing to catch up on. Starting it at zero would hand every
    -- first sign-in a backfill of days that happened to nobody.
    last_day   date        NOT NULL,

    -- When the last run happened, how many days it applied, and how many it
    -- skipped because they were older than the catch-up window. The
    -- operational questions — "is anybody falling behind", "who came back after
    -- a season away" — are this table plus one WHERE clause.
    ran_at     timestamptz NOT NULL DEFAULT now(),
    days       integer     NOT NULL DEFAULT 0,
    skipped    integer     NOT NULL DEFAULT 0,
    -- Total rollovers ever run for this player, which is what makes a schedule
    -- that has silently stopped firing visible.
    runs       bigint      NOT NULL DEFAULT 0,

    created_at timestamptz NOT NULL DEFAULT now()
);

-- The dormancy sweep and "who is behind" both read this way round.
CREATE INDEX rollover_ran_at_idx ON rollover (ran_at);

-- No backfill and no row created here, for the same reason internal/routine's
-- migration creates none: the value a row starts at depends on the player's
-- timezone, and the only thing that knows a timezone is Go. internal/rollover
-- creates the row the first time it is asked about a player.

-- River's tables are NOT here.
--
-- The queue owns its own schema and its own migration ledger, and it moves on
-- its own release schedule rather than the game's (§3.4). Copying its SQL into
-- this directory would pin the game's migration history to a library version
-- and break the moment the two disagreed about what version the database is at.
-- internal/jobs runs River's migrator at boot, next to this one.
