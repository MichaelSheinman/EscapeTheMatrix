-- +goose Up

-- What the player concluded, and what they had to go on when they concluded it
-- (§3.6).
--
-- Owned by internal/diagnosis. One row per conclusion, and the table holds
-- **only conclusions** — not the failures they are about and not the evidence
-- they were argued from. Both of those already exist elsewhere: a failure is a
-- row in `playbook_fire` or a day in `health_sample`, and the evidence is read
-- off the same series. Storing either again would be a second answer that goes
-- stale (§3.3), and the one this table would get wrong is the one that matters:
-- a player's history keeps moving after they have drawn a conclusion from it,
-- and that is the whole of what makes a conclusion revisitable.
--
-- There is no column for whether the conclusion was right. Not omitted for
-- later — refused. §3.6: "never say correct. No right/wrong feedback, ever.
-- The player acts on their diagnosis and the world's response reveals whether
-- they were right." A boolean here is how that rule gets broken eighteen months
-- from now by somebody who only wanted it for an analytics query, so the shape
-- of the table is the enforcement.
CREATE TABLE diagnosis (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The failure this is a conclusion about, by the stable key the module
    -- composes for it. Text rather than a foreign key, and for a stronger
    -- reason than the usual one: an occasion is not a row anywhere. It is
    -- composed on read out of whichever history recorded the thing going badly,
    -- so the only durable part of it is its name.
    occasion    text        NOT NULL,

    -- The cause the player named, by authored key. A cause withdrawn from
    -- content later leaves its conclusions behind rather than taking them with
    -- it — the same rule `playbook_unlock` and `playbook_fire` keep.
    cause       text        NOT NULL,

    -- When they named it, from the caller's clock (§3.1).
    at          timestamptz NOT NULL,

    -- The evidence they pointed at: observation keys, in the order they
    -- committed them. This is the conclusion's reasoning, kept because §3.6
    -- lets a player revisit one and a revisit is only meaningful against what
    -- the last one was argued from.
    pointed     text[]      NOT NULL,

    -- Every observation available to them at the time, pointed at or not.
    --
    -- The one thing here that could not be recomputed later, and the reason it
    -- is worth a column: §3.6's fifth rule is "the player can revisit a past
    -- conclusion once they have more evidence", and *more* is a comparison
    -- against what they had. Without this row the game would either offer a
    -- revisit every day (a retry loop, which rule 3 forbids) or never offer one
    -- at all.
    had         text[]      NOT NULL,

    CONSTRAINT diagnosis_pointed_at_something CHECK (cardinality(pointed) > 0)
);

-- The reads: this player's conclusions newest first, and the set of causes they
-- have named — which is what gates a routine (§3.6) and is asked on every read
-- of the routines screen.
CREATE INDEX diagnosis_account_idx ON diagnosis (account_id, at DESC, id DESC);
CREATE INDEX diagnosis_cause_idx ON diagnosis (account_id, cause);

-- Nothing is back-filled.
--
-- Every routine has been configurable from the start because the thing meant to
-- unlock them did not exist yet (see routine.Open), so every account that
-- already plays has answers stored for routines it has not earned. Those
-- answers stay, and keep running: the rollover lives by whatever is in
-- `routine.choices` whether or not the routine is unlocked, so nobody wakes up
-- to a life they had arranged being quietly cancelled. What they lose is the
-- ability to *change* it until they have sat down with somebody and said what
-- they think happened, which is the thing §3.6 says is earned rather than
-- issued.
