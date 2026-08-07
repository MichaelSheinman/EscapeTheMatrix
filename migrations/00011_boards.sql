-- +goose Up

-- The leaderboards (§3.5), as a refreshed snapshot rather than as a query.
--
-- §3.5 asks for a *distributed* board: one overall board for the composite,
-- plus a separate board for each individual factor. Seeing other real players
-- is what makes the composite mean anything, and it is the one thing a
-- single-player simulation cannot state on its own.
--
-- Three things about this table are the design rather than convenience.
--
-- **It holds positions, and — for the three hidden factors — nothing else.**
-- §3.5's hidden boards rank without disclosing how the number is produced, and
-- that is made structural here: a locked board's rows carry a NULL `value`,
-- because there is no column anywhere in this schema that a hidden factor's
-- *reading* is written to. A bug in a view cannot leak a number that was never
-- stored. What is stored for a hidden board is an ordering, which is what a
-- ranking is.
--
-- **The board keys here are the ones that go over the wire**, not the pure
-- core's factor keys — 'karma', 'health', 'money', and 'slot-3'..'slot-5' for
-- the three §3.5 never names. Same argument: internal/factor is the boundary,
-- and a table whose contents could be selected straight into a response is a
-- table that should not contain the word 'kindness'. Which factor a slot is
-- lives in Go, next to the veil that applies it.
--
-- **It is written by a background job, and only by one.** §3.4 lists
-- "leaderboard refresh" as a V1 job type, and the reason is that a rank is a
-- global fact: answering "where do I stand" per request would mean sorting
-- every player on every page load, and the answer would still be a snapshot —
-- just an expensive one taken at a worse moment. So the sort happens on a
-- schedule, the whole table is replaced under an advisory lock, and reads are
-- an index scan over an already-ordered thing.
CREATE TABLE factor_board (
    -- Which board. See above: this is the wire key, deliberately.
    board      text             NOT NULL,

    account_id uuid             NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- 1 is the top. Dense and gapless: ties are broken deterministically when
    -- the board is built (score, then account id), so a rank is a *position*
    -- rather than a score class. Two players who read identically still occupy
    -- two rungs, and which of them is above the other does not flap between
    -- refreshes.
    rank       integer          NOT NULL,

    -- What the player is shown beside the rank — a biological age, an amount
    -- held, a composite. NULL for the three boards §3.5 keeps sealed, and there
    -- is nowhere else their reading could be written.
    value      double precision,

    PRIMARY KEY (board, account_id)
);

-- The read path: one page of one board, in order. Unique because ranks are
-- assigned by position — a duplicate would mean the refresh sorted badly, and
-- it should fail loudly rather than serve two number fours.
CREATE UNIQUE INDEX factor_board_rank_idx ON factor_board (board, rank);

-- When the boards were last built, and over how many players.
--
-- A single row, enforced by the primary key, because there is one snapshot: the
-- whole table is replaced at once, so a per-board timestamp could only ever say
-- the same thing five more times. It is shown to the player for the same reason
-- the environment's wear is (§3.15) — a number that is minutes old should say
-- so rather than pretend to be live.
CREATE TABLE factor_board_refresh (
    only_row     boolean     PRIMARY KEY DEFAULT true CHECK (only_row),
    refreshed_at timestamptz NOT NULL,
    players      integer     NOT NULL
);

-- No rows created here. A database with no refresh yet has no boards, which is
-- exactly true, and the first run of the job fills both tables in one
-- transaction.
