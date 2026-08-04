-- +goose Up

-- The record that makes a command apply once (technical.md §8).
--
-- A command carries an Idempotency-Key. The executor claims the key inside the
-- same transaction as the handler, so the claim and the effect commit together
-- or not at all: a rolled-back command leaves no record and a retry re-runs it,
-- while a committed one is answered from here forever after.
--
-- Concurrency rests on the primary key. A second transaction inserting the same
-- key blocks on the first's uncommitted row rather than racing it, then finds
-- the committed result and replays it. That is the whole of "the player paid
-- twice" being impossible.
CREATE TABLE command_idempotency (
    -- Who issued it. Idempotency records hold a response body, so the scope has
    -- to include the caller — replaying one player's answer to another player's
    -- key would be a data leak by construction. Until accounts land (#3) there
    -- is exactly one actor and this reads 'anonymous'.
    actor        text        NOT NULL,
    command      text        NOT NULL,
    key          text        NOT NULL,

    -- sha256 of the request body. The same key sent with a different payload is
    -- a client bug, not a retry, and is answered 422 rather than with a result
    -- that does not belong to the request.
    request_hash bytea       NOT NULL,

    -- The answer, exactly as it was first written.
    --
    -- json, not jsonb: jsonb normalises key order, so a replay would return a
    -- body that differs byte-for-byte from the original. Replay returning the
    -- identical bytes is the promise being kept; nothing queries inside this
    -- column.
    status       smallint,
    response     json,

    recorded_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (actor, command, key)
);

-- Records are only useful for as long as a client might retry. Sweeping them is
-- a job, and the job queue arrives with the rollover ticket; this index is what
-- that sweep will read.
CREATE INDEX command_idempotency_recorded_at_idx ON command_idempotency (recorded_at);

-- The walking skeleton's probe gains an input, so that the command layer has a
-- payload to validate and a result whose contents visibly come from the first
-- send rather than the retry.
ALTER TABLE skeleton_probe ADD COLUMN label text;

-- And a state rule, so that "the world does not allow this" (409) is a thing
-- the probe can actually demonstrate, distinct from "the request is wrong"
-- (422). An unlabelled probe is always allowed, hence the partial index.
CREATE UNIQUE INDEX skeleton_probe_label_idx ON skeleton_probe (label) WHERE label IS NOT NULL;
