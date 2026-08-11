-- +goose Up

-- One day, and what this player's Karma read at the end of it (§3.5).
--
-- Owned by internal/factor, like the two tables 00010 created, and it needs
-- reading against that migration's second note — *there is no award history* —
-- because this is not one. A factor ledger would be a second timeline of what
-- the totals in `factor_credit` already are, and the two would disagree. This
-- is a different thing: the composite as it stood on a day that has ended.
--
-- Karma is still composed on read and still stored nowhere (§3.3). Today's
-- reading is not in this table until today is over, and a row here can never
-- disagree with the live composition, because a row here is about a day that
-- cannot change any more. A reading of the past is a record; a reading of the
-- present would be a stale copy.
--
-- Why it is kept at all: the player is shown one number that moves, and one
-- number with no yesterday beside it is a number nobody can act on. The curve
-- is the only place the game says "this went up when you did that" without
-- saying which factor moved — every point on it is the composite, and the
-- composite names nothing (§3.5's formula is never stated).
CREATE TABLE factor_history (
    account_id uuid             NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The player's own local day, in their own calendar. A DATE carries no
    -- timezone and none is wanted — it is already theirs, the same way
    -- factor_tenure.last_day is.
    day        date             NOT NULL,

    -- What the composite read. Unbounded, like the composition itself: no
    -- maximum is stored because there is no maximum (§3.5 — a number kept
    -- beside the ceiling it could reach is a level bar however it is drawn).
    karma      double precision NOT NULL,

    PRIMARY KEY (account_id, day)
);

-- No backfill. A player with no rows here has no days behind them yet, which is
-- exactly true of somebody who joined this morning — their curve is one point,
-- and it is today's.
