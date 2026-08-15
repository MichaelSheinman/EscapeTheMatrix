-- +goose Up

-- The ladder, removed (§3.15, and the decision of 2026-08-15).
--
-- §3.14 said "jobs sit in tiers" and this schema believed it: `career.tier` was
-- a rung number that a Go table turned into a wage, a shift and a capability
-- floor, and `career.reached` was the highest rung ever stood on. Both are gone
-- here, because §3.15 — "progression is lateral, not vertical; the player does
-- not climb, they widen" — is the spine of the design and the ladder was the one
-- place the game contradicted it.
--
-- What replaces a tier is nothing at all: a wage, a shift and a requirement are
-- facts about *a job*, authored on the job, and there is no rung between the
-- posting and the arithmetic any more. What replaces `reached` is this file's
-- one new table, which is a better answer to a question the scalar was only
-- approximating.

-- Every period of employment a player has had: which job, when it started, and
-- when it ended.
--
-- It exists because a prerequisite is now a real mechanic — a posting may ask
-- for forty-five days on a dispatch desk — and until now this database could
-- not answer that question about anybody. `career` holds the job somebody is in;
-- the day they left it, how long they had been there stopped existing.
--
-- Tenure is **cumulative and never lost**: leaving a job costs the wage and
-- nothing else (the decision of 2026-08-15). Two stints of twenty days are forty
-- days of experience, and there is deliberately no column that would let a
-- second reading disagree — no "longest run", no "continuous since". A player
-- who quits for a fortnight to deal with something keeps every day they worked.
CREATE TABLE career_stint (
    id         bigserial   PRIMARY KEY,
    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The role's key from internal/career's catalogue, the same vocabulary
    -- `career.role` speaks.
    role       text        NOT NULL,

    -- The player's own local days, like `career.paid_through` and
    -- `character.earned_on` above them: a stint is measured in the calendar the
    -- person lives in, not in ours. Null `ended_on` is the job they are in now.
    started_on date        NOT NULL,
    ended_on   date,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT career_stint_span CHECK (ended_on IS NULL OR ended_on >= started_on)
);

-- The one query this table exists for: how many days has this account worked
-- this role.
CREATE INDEX career_stint_tenure ON career_stint (account_id, role);

-- One job at a time, held as an index rather than as care in the code. Two open
-- stints would be two shifts in one day, which the pure core has no way to
-- express and no business expressing.
CREATE UNIQUE INDEX career_stint_open ON career_stint (account_id) WHERE ended_on IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
    -- A row written before roles could share a rung named its job by tier alone
    -- (see 00024). Resolving it now, once, to the first role of that tier in key
    -- order — which is exactly what [career.Of] resolved it to on every read, so
    -- this transcribes what those rows already meant rather than deciding
    -- anything new. After this the role column is the only name a job has.
    UPDATE career SET role = CASE tier
        WHEN 1 THEN 'corner-shop-nights'
        WHEN 2 THEN 'bank-counter'
        WHEN 3 THEN 'billing'
        WHEN 4 THEN 'partner-track'
        WHEN 5 THEN 'principal'
    END
    WHERE tier > 0 AND role IS NULL;

    -- Everybody currently employed gets the stint they are standing in, opened
    -- on the day they were hired. `since` is a timestamp in the server's clock
    -- and a stint wants the player's day; the cast is close enough for rows that
    -- predate the table, and every stint opened after this is written from the
    -- player's own calendar.
    INSERT INTO career_stint (account_id, role, started_on)
    SELECT account_id, role, since::date
      FROM career
     WHERE role IS NOT NULL AND since IS NOT NULL;

    -- A row holding a tier with no role left is one nothing above could name.
    -- It becomes unemployed, which is a real state rather than a broken one.
    UPDATE career SET role = NULL, since = NULL WHERE role IS NULL;
END
$$;
-- +goose StatementEnd

-- The rung is not a thing any more, so neither is the constraint that kept two
-- readings of it agreeing.
ALTER TABLE career DROP CONSTRAINT career_reached;
ALTER TABLE career DROP CONSTRAINT career_since;
ALTER TABLE career DROP COLUMN tier;
ALTER TABLE career DROP COLUMN reached;

-- Holding a job and having started it are still one fact; the job is now named
-- by the only column that names one.
ALTER TABLE career
    ADD CONSTRAINT career_since CHECK ((role IS NOT NULL) = (since IS NOT NULL));

COMMENT ON COLUMN career.role IS
    'The job held, by catalogue key. NULL is unemployed. §3.15: there is no rung.';

-- A situation arose in a *job*, not on a rung. The same transcription as above,
-- for the same reason: a player reading back what their role was doing has to be
-- told which role, and a tier can no longer answer that.
ALTER TABLE job_situation ADD COLUMN role text;

UPDATE job_situation SET role = CASE tier
    WHEN 3 THEN 'billing'
    WHEN 4 THEN 'partner-track'
    WHEN 5 THEN 'principal'
    ELSE 'billing'
END;

ALTER TABLE job_situation ALTER COLUMN role SET NOT NULL;
ALTER TABLE job_situation DROP COLUMN tier;
