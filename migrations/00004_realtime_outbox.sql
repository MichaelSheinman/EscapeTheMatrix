-- +goose Up

-- The per-player outbox that makes a dropped socket survivable (technical.md
-- §8, protocol rule 2).
--
-- The socket *will* drop — sleeping laptops, flaky wifi, deploys — and whatever
-- the server decided while it was down has to still be there when it comes
-- back. Without this table a reconnecting player silently misses an event
-- release and the world appears to skip, which §8 names as the most commonly
-- forgotten piece of this design.
--
-- It is the only path a message takes. `NOTIFY` carries no payload; it is a
-- doorbell telling a connection to read from here. Live delivery and resume are
-- therefore the same query against the same rows, so there is one thing to get
-- right rather than two that can disagree.
CREATE TABLE realtime_outbox (
    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Per player, gapless, assigned from realtime_stream below. Gapless is what
    -- lets a client say "I processed 12" and the server answer with exactly
    -- 13.. — a global sequence would make every resume a scan for relevance.
    seq        bigint      NOT NULL,

    -- The envelope's {type, payload}. json rather than jsonb for the reason the
    -- idempotency record gives: jsonb normalises key order, and nothing queries
    -- inside this column.
    type       text        NOT NULL,
    payload    json        NOT NULL,

    -- The session — the tab — whose command caused this, or NULL when the world
    -- did it on its own. A socket skips rows carrying its own session id: the
    -- acting client already applied the command's HTTP response (§8, rule 3),
    -- so an echo would apply it twice. Another tab of the same player has a
    -- different session id and receives it (rule 4).
    origin     text,

    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, seq)
);

-- Read by the sweep. The outbox is deliberately short-lived: it covers a drop,
-- not a history. A client whose resume point has already been swept is told so
-- and rehydrates over REST, which is what a page load does anyway.
CREATE INDEX realtime_outbox_created_at_idx ON realtime_outbox (created_at);

-- The sequence counter, one row per player.
--
-- Separate from the outbox because the outbox is swept: deriving the next
-- sequence from max(seq) there would hand out a number twice the moment the
-- rows behind it were deleted. The upsert that increments this takes a row
-- lock, so two commands for one player cannot be assigned the same number, and
-- the number is drawn inside the command's own transaction — a message exists
-- exactly when the effect it describes committed.
CREATE TABLE realtime_stream (
    account_id uuid   PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,
    last_seq   bigint NOT NULL
);
