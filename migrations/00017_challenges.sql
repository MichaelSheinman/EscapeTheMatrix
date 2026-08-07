-- +goose Up

-- The challenge catalog (§5): which authored challenges exist, and nothing
-- about what any of them is.
--
-- technical.md §5.2 is unusually specific about what this table is for. Repo
-- files are the source of truth — "content is versioned, diffable, reviewable,
-- and it travels with the code that interprets it" — and on boot the server
-- "loads, validates against the mechanic's config schema, and **upserts into a
-- `challenge` catalog table so foreign keys work. The database stores *runs*,
-- never definitions.**"
--
-- So this table holds identity and provenance and stops. There is no name here,
-- no scene, no option, no factor delta and no config document: every one of
-- those is content, and a copy of content in a table is a copy that disagrees
-- with the repo the first time somebody edits a line and forgets to redeploy.
-- What a run needs from a definition it reads out of the catalogue in memory.
--
-- The one thing the table is *needed* for is the foreign key underneath
-- `challenge_run.challenge`. A run has to name something that exists, and
-- "exists" is a property of content — so content is published here, once, at
-- boot, and the database can then enforce it like any other reference.
CREATE TABLE challenge (
    -- §5.3's identity, as three columns rather than one flat string:
    --
    --   "Flat codes like `challenge_resolve_conflict_variant_a` bury structure
    --    in a string — 'has this player done a resolve-conflict-shaped thing?'
    --    becomes an unindexable LIKE prefix match. With columns it's a real
    --    query, and the release policy can avoid repeating a *shape* rather
    --    than just an instance."
    --
    -- family is the shape (`resolve_conflict`), variant is the instance (`a`),
    -- and mechanic is the code that runs it (`dialogue_branch`). All three are
    -- text: §5.3 rejects a Postgres enum outright, because "adding a value
    -- requires ALTER TYPE, can't share a transaction with other DDL, and values
    -- can never be removed — and challenge codes will be added constantly".
    family      text        NOT NULL,
    variant     text        NOT NULL,
    mechanic    text        NOT NULL,

    -- "The display code is derived" (§5.3), and it is derived *here* so that it
    -- cannot be derived twice and differ. A stored generated column gives the
    -- foreign key below something to point at while leaving the three columns
    -- above as the only thing anybody writes.
    code        text        NOT NULL
                GENERATED ALWAYS AS (family || '/' || variant) STORED
                PRIMARY KEY,

    -- When this definition was last seen in the repo, and when it stopped
    -- being. Retiring rather than deleting, for the reason the event catalogue
    -- keeps its rows: a challenge withdrawn from content has runs behind it,
    -- and a player's history must not be deleted by an author tidying up.
    --
    -- Stated by the injected clock rather than by now(), like every other
    -- timestamp this schema holds (§3.1).
    published_at timestamptz NOT NULL,
    retired_at   timestamptz,

    -- The separator in `code` has to mean something, so neither half may
    -- contain one. Without this, `a/b` + `c` and `a` + `b/c` are the same
    -- challenge as far as the primary key is concerned.
    CONSTRAINT challenge_family_flat  CHECK (family  !~ '/' AND family  <> ''),
    CONSTRAINT challenge_variant_flat CHECK (variant !~ '/' AND variant <> ''),
    CONSTRAINT challenge_mechanic_named CHECK (mechanic <> '')
);

-- §5.3's real query: "has this player done a resolve-conflict-shaped thing?"
-- The family leads because that is what the question is about.
CREATE INDEX challenge_family_idx ON challenge (family, variant);


-- One player's relationship with one challenge: §5's *run*, and the only thing
-- about challenges this database holds.
--
-- Two sentences of technical.md §5.1 are the whole shape of this table.
--
--   * **"~6 minutes. Once ever, per player. `challenge_run` carries a unique
--     constraint on (player, challenge)."** That constraint is below, and it is
--     the mechanism rather than a safety net: there is no code path that starts
--     a second run, and the index is what makes that true under two tabs.
--   * **"Save and resume is mandatory — even at 6 minutes, players abandon
--     partway. In-flight run state is checkpointed at scene boundaries."** So
--     the row carries where the run *is* and what it has committed, and it is
--     written exactly when a scene is left. Not on a timer: a timer would land
--     a resumed player half way through a conversation, which is the incoherent
--     resume the rule exists to rule out.
CREATE TABLE challenge_run (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The catalog reference (§5.3): text with a foreign key, never an enum.
    -- ON DELETE RESTRICT by omission — nothing deletes from the catalog, and a
    -- migration that started to would find the runs in its way, which is the
    -- correct outcome.
    challenge   text        NOT NULL REFERENCES challenge (code),

    -- running | finished. There is no 'abandoned': abandoning is what a running
    -- row *is* between two sittings, and a state for it would be a state the
    -- player could be put into by closing a tab.
    state       text        NOT NULL,

    -- The checkpoint. Which scene the run is standing at — the authored key, so
    -- that resuming is "show this scene" rather than "replay these choices" —
    -- and, when the run has finished, the scene it ended on.
    scene       text        NOT NULL,

    -- Everything committed so far, in order: [{"scene": "...", "option": "..."}].
    --
    -- jsonb rather than a child table, following the same argument routine
    -- configuration is stored under: it is written whole, read whole, and never
    -- queried a field at a time. It is also the reason a completed run can be
    -- read back as a *story* — which is what §3.6's diagnosis argues from.
    trail       jsonb       NOT NULL DEFAULT '[]'::jsonb,

    -- Which outcome bucket the trail fell in. §5.1: "the rubric-equivalent is a
    -- scored bucket (dismissed you / heard you / moved by you) with written
    -- reaction text per branch."
    --
    -- The *bucket* is here and the score is not, and the asymmetry is §3.3's
    -- rule: "everything derived is computed on read, never stored as a stale
    -- number. Only the inputs are persisted." A score is a sum over the trail
    -- and the trail is right here, so a column for it would be a second copy
    -- that can disagree with the first. The bucket is not derived in the same
    -- sense — it is what the player was actually told, and an author retuning a
    -- threshold next month must not retroactively change that.
    --
    -- Null until the run finishes, because until then there is no outcome.
    bucket      text,

    -- The event that opened this, if one did (§4: "an event *can* offer a
    -- challenge… they are distinct systems, not one abstraction"). Kept so the
    -- run can settle it when it finishes, through internal/event's Resolve.
    --
    -- Not a foreign key, deliberately: the two systems are distinct, a
    -- challenge may be entered with no event at all, and a run that outlived
    -- its backlog row is still a run.
    opened_by   bigint,

    began_at    timestamptz NOT NULL,
    -- When the checkpoint last moved. The one number that says how stale a
    -- resumed run is, which is what a screen would use to decide whether to
    -- re-establish the scene or drop the player straight back into it.
    moved_at    timestamptz NOT NULL,
    finished_at timestamptz,

    CONSTRAINT challenge_run_state CHECK (state IN ('running', 'finished')),

    -- Finishing is three facts that are one fact: it ended, it ended then, and
    -- it ended in a bucket. A row with two of the three is a run that was
    -- settled by something that did not finish it.
    CONSTRAINT challenge_run_finished CHECK ((state = 'finished') = (finished_at IS NOT NULL)),
    CONSTRAINT challenge_run_bucket   CHECK ((state = 'finished') = (bucket IS NOT NULL)),

    CONSTRAINT challenge_run_trail CHECK (jsonb_typeof(trail) = 'array')
);

-- §5.1's "once ever, per player", held by the database.
--
-- A unique constraint rather than an index, because it is a rule about the
-- domain rather than a read that happens to be fast: a challenge is a
-- six-minute authored endeavour with a committed judgement in it, and a second
-- run would be the player finding out that none of it was load-bearing.
ALTER TABLE challenge_run
    ADD CONSTRAINT challenge_run_once UNIQUE (account_id, challenge);

-- Resuming: this player's run that has not finished. Partial, because the
-- answer is almost always one row or none.
CREATE INDEX challenge_run_open_idx
    ON challenge_run (account_id, moved_at) WHERE state = 'running';

-- §5.3's shape question, asked of a player rather than of the catalogue: which
-- families they have already been through.
CREATE INDEX challenge_run_done_idx
    ON challenge_run (account_id, challenge) WHERE state = 'finished';

-- No rows created here, and none in `challenge` either. The catalog is filled
-- by the process at boot from what is in the repo, which is the only place a
-- definition ever comes from (§5.2).
