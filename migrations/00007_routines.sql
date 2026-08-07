-- +goose Up

-- The player's routines: the rules they have set for their own body (§3.4).
--
-- Owned by internal/routine, which is the only thing that reads or writes it
-- (§3.1). The health module never selects from here — it is handed a day and
-- does not learn where the day came from, which is what keeps "a routine
-- produces a day" a seam the rollover (#10) can stand in as well.
CREATE TABLE routine (
    -- One configuration per account. There is no set of routines to choose
    -- between and no way to end up with two.
    account_id uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,

    -- Every answer, in one document, read and written whole.
    --
    -- A column per routine would make adding a fifth one a migration, and the
    -- vocabulary is authored content that §17 still leaves open — the same
    -- argument health_instrument already makes for holding one row per tier
    -- owned instead of one column per tier. It is also what "configured as a
    -- unit" means at rest: there is no command that changes one routine, so
    -- there is no reason for the storage to be able to.
    --
    -- The keys are routine keys and the values are option keys, both from the
    -- vocabulary in internal/routine. An option that has since been renamed
    -- away falls back to that routine's default on read rather than erroring,
    -- for the same reason a district that left the city keeps its key: the
    -- content moves and a player standing on the old version has to be able to
    -- walk out of it.
    choices    jsonb       NOT NULL DEFAULT '{}'::jsonb,

    -- Whether the player has ever answered. Distinct from an empty document,
    -- because "you have not set your routines" and "you set them all back to
    -- the way an unarranged life runs" are different things to say to somebody
    -- — and only one of them is worth a prompt.
    set_at     timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- No backfill, and no row created here.
--
-- The defaults live in the vocabulary in internal/routine and nowhere else;
-- writing them in as literals would be a second copy that drifts the first time
-- an option is retuned. A player with no row reads as an unconfigured life,
-- which is exactly what they have.
