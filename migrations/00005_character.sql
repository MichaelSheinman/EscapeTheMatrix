-- +goose Up

-- The character: money, where they are, and where they live (§3.3's table).
--
-- Owned by internal/character, which is the only thing that reads or writes it
-- (§3.1). Another module wanting to know what a player can afford asks that
-- package through an interface rather than selecting from here — the rule this
-- table exists to establish as much as to hold state.
CREATE TABLE character (
    -- One character per account, so the account id is the key. There is no
    -- separate character id to carry around, and no way to end up with two.
    account_id  uuid        PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,

    -- Cents, never a float: money is counted rather than measured, and a
    -- rounding error in a balance is the bug a player finds first.
    money_cents bigint      NOT NULL,

    -- Where they are standing, and where home is. District keys from the
    -- authored vocabulary in internal/character; §17 leaves the city and its
    -- district list open, so these are text rather than an enum the schema
    -- would have to be migrated to change.
    district    text        NOT NULL,
    home        text        NOT NULL,

    -- The player's own day boundary (§3.3). One game day is one real calendar
    -- day, and midnight happens somewhere before it happens here.
    timezone    text        NOT NULL DEFAULT 'UTC',

    -- §3.4 caps money-producing actions per day. What is stored is the count
    -- and the local day it belongs to; how many are *left* is computed on read
    -- (§3.3), because a stored remainder is wrong every midnight.
    earned_on    date       NOT NULL DEFAULT '0001-01-01',
    earned_today integer    NOT NULL DEFAULT 0 CHECK (earned_today >= 0),

    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Every account that already exists gets a character, so that signing in on an
-- account made before this migration lands in a world rather than in an error.
-- New accounts get theirs when they sign in.
INSERT INTO character (account_id, money_cents, district, home)
SELECT id, 120000, 'east-side', 'east-side' FROM account;
