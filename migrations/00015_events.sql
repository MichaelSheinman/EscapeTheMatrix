-- +goose Up

-- The event backlog (§4): the things that could happen to one player, and where
-- each of them has got to.
--
-- technical.md §4 opens with "**the world maintains a backlog** — events are
-- generated ahead of demand and sit in a queue, so there is always something to
-- do", and the rest of the section is mostly about what must *not* happen to
-- one. Two of those rules are the reason this is a table with a state column
-- rather than a queue of messages:
--
--   * **Declining does not consume** (§3.9). Walking past an argument returns
--     it to the backlog. A row that was offered and refused goes back to
--     'waiting' with a count incremented — it is never deleted, and there is no
--     code path here that deletes one.
--   * **Revalidation at release.** A backlogged event is re-checked against
--     current state before being surfaced and silently dropped or requeued if
--     its preconditions no longer hold — which means an event has to be able to
--     sit here for a fortnight and still be a candidate, so what is stored is
--     the authored key and nothing about the world as it was when it was
--     generated.
--
-- What is *not* here is the event itself. Its words, its placement rules and
-- its successor are authored content in `content/events` (§5.2), loaded at
-- boot; this table holds one player's relationship with it. An event withdrawn
-- from content leaves rows behind, and the release path drops them.
CREATE TABLE event_backlog (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id   uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The authored key. Text rather than a foreign key for the same reason
    -- playbook_fire.situation is: the catalogue is content in the repo, not
    -- rows, and content that has been withdrawn must not take a player's
    -- backlog down with it.
    event        text        NOT NULL,

    -- Where in §4's lifecycle this instance has got to:
    --
    --   generated → eligible → offered → (declined | accepted) → resolved
    --
    -- Three of those five are stored and two are not, deliberately.
    -- 'waiting' is §4's *generated*; **eligible is computed at release and
    -- never stored**, because a stored eligibility is a stale one and
    -- revalidation against current state is the whole rule. 'declined' is not
    -- stored either — declining returns the row to 'waiting', which is what
    -- "declining does not consume" means as a state machine rather than as a
    -- sentence. What is left is: waiting, offered, accepted (opened something
    -- that finishes later), done.
    state        text        NOT NULL,

    -- Stated by the injected clock rather than by now(), like every other
    -- timestamp that is a game rule (§3.1): patience, the offer window and the
    -- rest after a decline are all mechanics a test has to be able to move.
    generated_at timestamptz NOT NULL,

    -- The earliest this may be surfaced. Moved forward when a release
    -- re-validates it and the preconditions do not hold, and when the player
    -- walks away from it — so an event that cannot happen yet stops being
    -- reconsidered on every step of every journey.
    eligible_at  timestamptz NOT NULL,

    -- How many times it has been put to the player, and how many of those they
    -- walked away from. Kept on the row rather than counted from the record,
    -- because the release path reads them on every candidate and the record is
    -- history rather than an index.
    offers       smallint    NOT NULL DEFAULT 0,
    declines     smallint    NOT NULL DEFAULT 0,

    -- Where it was last put, out of internal/city's places and zones. Kept
    -- after the offer ends because it is what a screen says about the thing
    -- that just happened, and because an event pinned to a spot the player has
    -- since left has to be able to say which spot that was.
    place        text,
    zone         text,

    offered_at   timestamptz,
    expires_at   timestamptz,
    settled_at   timestamptz,

    CONSTRAINT event_backlog_state CHECK (state IN ('waiting', 'offered', 'accepted', 'done')),

    -- An offer is exactly the state that has a window on it. Leaving 'offered'
    -- clears both, which is what keeps the partial index below meaning "there
    -- is something on screen now" rather than "there was, once".
    CONSTRAINT event_backlog_window CHECK ((state = 'offered') = (expires_at IS NOT NULL)),
    CONSTRAINT event_backlog_offered CHECK ((state = 'offered') = (offered_at IS NOT NULL)),
    CONSTRAINT event_backlog_placed CHECK (state <> 'offered' OR (place IS NOT NULL AND zone IS NOT NULL)),
    CONSTRAINT event_backlog_settled CHECK ((state = 'done') = (settled_at IS NOT NULL))
);

-- One thing on screen at a time, held by the database rather than by a check in
-- Go. Two tabs walking into the same doorway in the same instant is the
-- ordinary case this exists for: both transactions would otherwise find the
-- backlog quiet and release something, and the player would be handed two
-- moments at once — which §4's "there is a cap on what gets surfaced" rules out
-- for a long absence and which nothing else would rule out for a race.
CREATE UNIQUE INDEX event_backlog_open_idx
    ON event_backlog (account_id) WHERE state = 'offered';

-- The same event is never in one player's backlog twice while it is unfinished.
--
-- The rule that makes chaining safe: responding to an event queues its
-- successor (§4), and a chain answered twice would otherwise queue a second
-- copy of a successor already waiting. It also means "is this already in the
-- backlog" is a question the generator asks the schema rather than the other
-- way round.
CREATE UNIQUE INDEX event_backlog_pending_idx
    ON event_backlog (account_id, event) WHERE state <> 'done';

-- The release path's read: this player's waiting events, oldest first.
CREATE INDEX event_backlog_waiting_idx
    ON event_backlog (account_id, eligible_at) WHERE state = 'waiting';


-- What became of the events this player was actually shown.
--
-- technical.md §3.2 enumerates the history surfaces this game keeps and calls
-- that list the contract. "Event records — fired, outcome — §4; diagnosis
-- evidence" is the line this table answers, and it answers two questions the
-- backlog row cannot:
--
--   * **What the player did with what they were offered.** A backlog row holds
--     the current state of one event; a player who has walked past the same
--     encounter four times has one row and four rows of history, and §4's
--     "declining is signal" is a query over the second of those.
--   * **What happened, in an order a person can read.** The backlog is a set of
--     things that have not happened yet, which is not a story. This is.
--
-- The answers are kept apart on purpose. 'declined' is a button pressed and
-- 'left' is a player who walked away without answering, and both return the
-- event to the backlog — but only one of them is somebody making a decision,
-- and a release policy (#22) that could not tell them apart would read
-- inattention as refusal.
CREATE TABLE event_record (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Which instance in the backlog this is a record of. An accepted event that
    -- opens something else is settled later by whichever module ran it (#23,
    -- #28, #30), and settling it has to find the record it is settling rather
    -- than the most plausible one.
    backlog_id  bigint      NOT NULL REFERENCES event_backlog (id) ON DELETE CASCADE,

    event       text        NOT NULL,

    -- The family, stored rather than looked up, because it is what the decline
    -- signal is grouped by and content may be re-classified later — a signal
    -- that changed shape retroactively would be a signal about the content.
    kind        text        NOT NULL,

    -- What the event was called at the time. History that reads back as
    -- `a-knock-at-the-door` is not history, and an event withdrawn from
    -- content must not take its own past with it.
    name        text        NOT NULL,

    at          timestamptz NOT NULL,

    -- Where it was put. Not nullable: everything recorded here was placed
    -- somewhere, including the ones that were dropped on the way to being
    -- offered — those were dropped while the player stood in a particular
    -- place, and where the world gave up on something is worth as much as
    -- where it happened.
    place       text        NOT NULL,
    zone        text        NOT NULL,

    -- accepted | declined | left | lapsed | dropped.
    answer      text        NOT NULL,

    -- When an accepted event finished. Null while it is still open, which is
    -- every accepted event that opened something else — a challenge (#23), a
    -- job situation (#28), a conflict (#30). Same shape, and the same reason,
    -- as playbook_fire.settled_at.
    resolved_at timestamptz,

    CONSTRAINT event_record_answer CHECK (
        answer IN ('accepted', 'declined', 'left', 'lapsed', 'dropped')),
    CONSTRAINT event_record_resolved CHECK (
        resolved_at IS NULL OR answer = 'accepted')
);

-- The log's read: this player's events, newest first.
CREATE INDEX event_record_log_idx ON event_record (account_id, at DESC, id DESC);

-- §4's signal read: how this player has been answering one family of things,
-- lately. The kind leads because that is what the question is about.
CREATE INDEX event_record_signal_idx ON event_record (account_id, kind, at DESC);

-- Settling an accepted event, later, from the module that ran what it opened.
CREATE INDEX event_record_open_idx
    ON event_record (backlog_id) WHERE answer = 'accepted' AND resolved_at IS NULL;

-- No rows created here. A player who has never played has never been offered
-- anything, and their backlog is filled by their first rollover (§3.3).
