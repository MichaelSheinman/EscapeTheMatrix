-- +goose Up

-- The day the player last moved house (§3.15's relocation, #31).
--
-- A date rather than a timestamp, and the player's own local one, exactly like
-- `character.earned_on` above it: the rule it exists to hold is "not twice in a
-- day", and a day is a thing that starts at somebody's midnight rather than at
-- ours. What is *left* — whether they may move right now — is computed on read
-- against their timezone (§3.3), because a stored "can move" is wrong every
-- midnight.
--
-- '0001-01-01' is a character who has never moved, which is every character
-- there was before this migration: they live where a new one starts, and the
-- first move they make is free of yesterday.
ALTER TABLE character
    ADD COLUMN moved_on date NOT NULL DEFAULT '0001-01-01';

COMMENT ON COLUMN character.moved_on IS
    'The player-local day home last changed. §3.15: one move a day.';
