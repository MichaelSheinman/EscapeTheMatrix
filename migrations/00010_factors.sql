-- +goose Up

-- What the hidden factors have accumulated (§3.5).
--
-- Owned by internal/factor, which is the only thing that reads or writes it
-- (§3.1). Two things about its shape are the design rather than convenience.
--
-- First, **there is no Karma column here, and there is nowhere else one could
-- be.** §3.3: everything derived is computed on read and only the inputs are
-- persisted. Karma is a composition of a body, a balance and the totals below,
-- so a stored copy could only ever be a number that disagreed with the three
-- things it was a reading of.
--
-- Second, **there is no award history.** §3.2 enumerates the history surfaces
-- this game keeps and a factor ledger is not one of them: a factor delta is
-- recorded by whatever awarded it — a challenge outcome record (§5), an event
-- record (§4) — and what is current state is the total. Two timelines of the
-- same thing is how you end up unable to render either.
--
-- One row per factor rather than a column per factor, for the same reason
-- health_instrument holds a row per tier: the vocabulary is authored content
-- (§17 leaves the factor list open past V1) and a sixth factor should not be a
-- migration. Only the hidden three are ever written here — health is read off
-- the body and money off the balance, and neither is anybody's to award.
CREATE TABLE factor_credit (
    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- A simulation.Factor key: 'intelligence', 'stability', 'kindness'. Text
    -- rather than an enum, so that renaming the vocabulary is a code change and
    -- a data migration rather than a lock on the whole table.
    factor     text        NOT NULL,

    -- Credit, not a score. Unbounded above; what it reads as on the board
    -- saturates, and that is a pure function in internal/simulation.
    credit     double precision NOT NULL DEFAULT 0,

    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, factor)
);

-- How worn each environment is, for this player (§3.15).
--
-- Karma gains from an environment decay the longer the player stays and the
-- more thoroughly they have mastered it, and relocating restores the rate. So
-- the decay state is per *environment* and is kept rather than reset: a player
-- who leaves the East Side and comes back finds it exactly as worn out as they
-- left it, which is what stops a lap of the city being a way to refresh a place
-- one has already exhausted.
CREATE TABLE factor_tenure (
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Which environment. Today it is the district the player is standing in,
    -- which is the closest thing the city has to §3.15's "defined by its
    -- people". #29 replaces the source of this key with real populations; the
    -- column does not change when it does.
    environment text        NOT NULL,

    -- Days lived here — the weak half of the decay curve.
    days        integer     NOT NULL DEFAULT 0,

    -- Raw credit this environment has been *offered*, before decay. The
    -- mastery half, and deliberately the pre-decay figure: §3.15 asks decay to
    -- track mastery rather than elapsed time, and counting the decayed payout
    -- would let the curve flatten itself into a stall.
    worked      double precision NOT NULL DEFAULT 0,

    -- The last local day counted against `days`, in the player's own calendar.
    -- A DATE carries no timezone and none is wanted: it is already their day.
    -- Null until one has been. Strictly forward, like the body's own ledger, so
    -- a retried rollover cannot age a place twice.
    last_day    date,

    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, environment)
);

-- No backfill and no rows created here.
--
-- A player with no factor_credit row has no credit, which is exactly what a
-- player who has made no committed choices has. A player with no factor_tenure
-- row has never been counted anywhere, and the first environment they are in is
-- worth all of what it offers — which is the right answer and not a special
-- case.
