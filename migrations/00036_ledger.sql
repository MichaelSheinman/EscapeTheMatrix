-- +goose Up
-- Every movement of a player's money, in the order it happened.
--
-- A balance is one number and says nothing about how it got there. Rent leaving
-- on the first of the month, a wage arriving, a fare paid on the way somewhere
-- — all of it was a column going up and down with no record, so the player
-- could see what they have and never what they are doing.
--
-- Append-only. A line is a statement about a moment that has passed, and the
-- only correction the game can make to one is another line.
CREATE TABLE ledger (
    id           bigserial   PRIMARY KEY,
    account_id   uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- When it happened, off the injected clock rather than the database's, so a
    -- caught-up day is dated to the day it describes.
    at           timestamptz NOT NULL,

    -- What kind of movement, from internal/ledger's list, and who it was with.
    -- Text rather than an enum for the reason 00005 gives about places: the list
    -- is still open, and a new kind should not need a migration.
    --
    -- The counterparty is the authored name of the other side — the job, the
    -- instrument, the landlord — and is empty where there is no other side. The
    -- sentence a player reads is composed from the pair; no prose is stored.
    kind         text        NOT NULL,
    counterparty text        NOT NULL DEFAULT '',

    -- Signed: money in is positive, money out is negative.
    amount_cents bigint      NOT NULL,

    -- The balance this movement left behind, stamped in the same transaction
    -- that wrote it. Recorded rather than summed, so the running balance a
    -- statement shows is the one the character actually had at that moment.
    balance_cents bigint     NOT NULL
);

-- Every read is one player's lines, newest first.
CREATE INDEX ledger_account_at_idx ON ledger (account_id, id DESC);

-- Characters that already exist have a balance with nothing behind it. They get
-- one opening line rather than invented history: the money is real, and the
-- transactions that would explain it never happened.
INSERT INTO ledger (account_id, at, kind, counterparty, amount_cents, balance_cents)
SELECT account_id, created_at, 'opening', '', money_cents, money_cents FROM character;
