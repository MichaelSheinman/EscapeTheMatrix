-- +goose Up

-- Which challenges have been turned on for which player (§5, the gate).
--
-- §4 gives a challenge two ways to reach a player: it stands somewhere in the
-- world, or an event offers it. Neither of those answers *when* — a challenge
-- that stands in a doorway stands there from the first minute of the game, for
-- everybody, forever. That is right for the small ones and wrong for the big
-- ones: a house on fire two doors down is not scenery, it is something the
-- world decides has happened to you today.
--
-- The thing that decides is not built yet, and this table is the honest
-- placeholder for it. A challenge declares `gate: flag` in its own definition,
-- and then it reaches nobody until a row in here says otherwise. What writes
-- the row today is a developer's toolbar; what writes it later is whatever §4's
-- release policy grows into. Both write the same row, which is the point of
-- having one — the surface, the begin command and the tests all ask the same
-- question now as they will then, and only the answerer changes.
--
-- What this deliberately is **not** is a copy of the catalogue with a boolean
-- on it. There is no row per (player, challenge) pair waiting to be flipped:
-- absence is the closed state, so a challenge nobody has been given has no
-- rows at all, and turning one off is a delete rather than a false. A table
-- that held both states would be a table somebody eventually reads as "these
-- are the challenges", which is the quest list §4 refuses to keep.
CREATE TABLE challenge_flag (
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The catalog reference, exactly as a run holds it: text with a foreign
    -- key. A challenge withdrawn from content keeps its flags for the same
    -- reason it keeps its runs — the row is a fact about what was turned on,
    -- and an author tidying up does not get to rewrite it.
    challenge   text        NOT NULL REFERENCES challenge (code),

    -- When it was turned on, from the injected clock (§3.1). Kept rather than
    -- implied because the thing that will eventually write these rows is a
    -- release policy, and a policy that cannot see when it last released
    -- something is a policy that releases it again.
    enabled_at  timestamptz NOT NULL,

    -- Who turned it on: `dev` today, and whatever names itself later. Not an
    -- enum, and not validated here — it is provenance for a human reading the
    -- table, and the moment it constrains anything it stops being that.
    by          text        NOT NULL,

    PRIMARY KEY (account_id, challenge)
);

-- No index below this line, and that is deliberate. The read the challenge
-- surface does on every walk is "everything turned on for this player", and the
-- primary key already leads with the account — so the constraint above is the
-- index for it, and a second one would be the same tree written twice.

