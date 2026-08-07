-- +goose Up

-- Peer endorsement (§3.5): the one place a hidden factor is moved by a person
-- rather than by the simulation.
--
-- HighLevelOverview.md §3.5 asks for "an occasional 'I like this user' button
-- players can press. Rate it, don't make it always-available." §5's V1/V2 table
-- keeps it in V1 for a specific reason: it is *asynchronous*. Nobody has to be
-- online, nobody has to answer, and the player being endorsed does nothing at
-- all. That is the whole of the player-to-player interaction V1 ships, and it
-- is what gives Kindness a human signal rather than an AI-only proxy.
--
-- Two of the ticket's rules are enforced by this schema rather than by Go, and
-- that is deliberate: a rule a bug can skip is a rule the anti-farming argument
-- does not actually have.
--
--   * **No self-endorsement** — the CHECK below. There is no code path, no
--     retry and no race that can write a row where the two accounts are equal.
--   * **No repeat endorsement of the same player** — the primary key. Once,
--     ever, in that direction. It is the strongest of the anti-farming rules:
--     two players who agree to like each other every morning get exactly one
--     morning out of it.
--
-- What is *not* here is a score. An endorsement is an event, and the credit it
-- was worth lands in factor_credit, which internal/factor owns. This table is
-- the record that the event happened, which is the thing that must not be able
-- to happen twice.
CREATE TABLE endorsement (
    -- Who pressed the button.
    from_account uuid             NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Who they said they liked. This is the account the Kindness credit went
    -- to: §3.5's factor is "how helpful the player is", and being liked by
    -- somebody is a statement about the person who was liked. The endorser
    -- gains nothing, which is §7 applied to a button — a factor a player can
    -- award themselves is not a measurement of anything.
    to_account   uuid             NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    at           timestamptz      NOT NULL,

    -- What the grant was worth before the environment's familiarity was applied
    -- (§3.15). Kept because it is the authored figure the offer was made under:
    -- retuning the constant later must not rewrite what an old endorsement was.
    credit       double precision NOT NULL,

    PRIMARY KEY (from_account, to_account),
    CONSTRAINT endorsement_not_self CHECK (from_account <> to_account)
);

-- The rate limit's read: how many this player has given inside the window.
CREATE INDEX endorsement_from_at_idx ON endorsement (from_account, at DESC);

-- The candidate ordering's read: how many each player has received. Fewest
-- first, so the human signal spreads instead of piling onto whoever is already
-- visible on a leaderboard.
CREATE INDEX endorsement_to_idx ON endorsement (to_account);

-- Who the game has offered this player, and what became of it.
--
-- **This table is what makes the button occasional.** The design's instruction
-- is "rate it, don't make it always-available", and the shape that satisfies it
-- is an *offer* rather than a search: the player cannot look somebody up and
-- like them, because there is nothing to type. The server names one person, for
-- a while, now and then — and the only two things the player may do about it
-- are press the button or let it go.
--
-- That also answers a question a free-text endorsement could not: how do you
-- address another player at all, when the leaderboards deliberately hand out
-- names and never account ids? You do not. You answer an offer by its id, and
-- the server already knows who it named.
CREATE TABLE endorsement_offer (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id   uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    candidate_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Both stated by the injected clock rather than by now(), because the
    -- cadence and the window are game rules and a test has to be able to move
    -- them (§3.1). Every other timestamp in this schema comes off the database
    -- clock; these two are the exception, and the reason is that they are the
    -- mechanic rather than bookkeeping.
    offered_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,

    -- Null while the offer stands.
    answered_at  timestamptz,
    answer       text,

    CONSTRAINT endorsement_offer_not_self CHECK (account_id <> candidate_id),
    CONSTRAINT endorsement_offer_answered CHECK ((answered_at IS NULL) = (answer IS NULL)),
    CONSTRAINT endorsement_offer_answer   CHECK (answer IS NULL OR answer IN ('liked', 'passed', 'lapsed'))
);

-- At most one open offer per player, held by the database rather than by a
-- guard in Go. Two tabs opening the world at the same instant is the ordinary
-- case this exists for: both transactions would otherwise draw a candidate and
-- the player would be handed two people to like at once, which is the opposite
-- of occasional.
--
-- An offer whose window has passed is closed by writing 'lapsed' rather than by
-- being left open, which is what keeps this index meaning "there is somebody on
-- screen right now" instead of "there was, once".
CREATE UNIQUE INDEX endorsement_offer_open_idx
    ON endorsement_offer (account_id) WHERE answered_at IS NULL;

-- The cadence's read: when this player was last offered anybody.
CREATE INDEX endorsement_offer_recent_idx ON endorsement_offer (account_id, offered_at DESC);

-- The candidate filter's read: who this player has been shown lately. Somebody
-- passed over is not offered again immediately — but they are not gone forever
-- either, because in a small population that would empty the pool.
CREATE INDEX endorsement_offer_candidate_idx ON endorsement_offer (account_id, candidate_id, offered_at DESC);

-- No rows created here. A player who has never opened the game has never been
-- offered anybody, which is exactly true.
