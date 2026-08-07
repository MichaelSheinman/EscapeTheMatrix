-- +goose Up

-- The situations an active role produced while the player was elsewhere (§3.14).
--
-- Owned by internal/situation, which is the only thing that reads or writes it
-- (§3.1). HighLevelOverview.md §3.14's loop is *it happens without you → you
-- review → you compose rules → a rule fails → you revise*, and this table is
-- the first arrow: one row per day a delegated role produced something, and
-- what became of it.
--
-- What it deliberately does not hold is any of the writing. The moment, the
-- answers to it and what each of them came to are authored content in
-- `content/situations` (technical.md §5.2), and a copy of them here would show
-- yesterday's prose after a deploy. What is persisted is the key, the answer,
-- and the two facts nothing can recompute: whether the player has dealt with it
-- and when.
--
-- It is not a second log either. `playbook_fire` is the record — which rule
-- fired, on which condition, and how it went — and every row here that reached
-- the matcher points at one. This table exists for the half a fire cannot
-- carry: a moment waiting on a live decision, which is a piece of world rather
-- than a piece of history.
CREATE TABLE job_situation (
    id          bigserial   PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The player's own local day the role produced it on. A DATE for the same
    -- reason `career.paid_through` is one: it is already their calendar day, in
    -- their timezone, written by whichever rollover walked it.
    on_day      date        NOT NULL,

    -- The authored moment, by key. Not its words — see above.
    situation   text        NOT NULL,
    -- The rung held when it arose. Kept because a role is what generates
    -- situations (§3.14) and a player who has since moved up or walked out
    -- still has to be able to read back what the job they had was doing.
    tier        smallint    NOT NULL CHECK (tier > 0),
    -- Where the player was, by environment key (§3.15). The same column the
    -- fire record keeps, and for the same reason: the rules that were weighed
    -- are the rules in force *there*, and a log that lost the place would have
    -- lost what makes a rule set transfer or fail.
    environment text        NOT NULL,

    -- The rule-fire record this moment produced, in internal/playbook. Null
    -- only for a moment nothing weighed at all.
    fire_id     bigint,

    -- What the character did, as the posture it amounted to — empty while the
    -- moment is still waiting on the player. A posture rather than an action
    -- term because that is what the situation authored an answer for: the
    -- vocabulary has many words for going along with something and the world
    -- only has one consequence for it.
    stance      text        NOT NULL DEFAULT '',
    -- One of pending | good | mixed | poor, the same closed set the fire record
    -- keeps. Pending is a moment nobody has answered yet.
    outcome     text        NOT NULL DEFAULT 'pending',

    -- Whether this is still in front of the player. The one piece of world in
    -- the table, and what makes "good rules buy time" true rather than a
    -- slogan: a role produces nothing new while something it produced is still
    -- waiting, so a playbook that covers its work keeps the work moving and a
    -- playbook full of gaps stops it.
    waiting     boolean     NOT NULL,

    created_at  timestamptz NOT NULL DEFAULT now(),
    decided_at  timestamptz,

    -- One situation per player per day. This is what makes the rollover's walk
    -- idempotent: a retried job, a second tab hydrating, or a scheduled run
    -- arriving mid-catch-up all try to produce the same day's moment and only
    -- the first of them writes one.
    CONSTRAINT job_situation_day UNIQUE (account_id, on_day),

    -- Waiting and decided are one fact at two ages, exactly as `career.tier`
    -- and `career.since` are.
    CONSTRAINT job_situation_settled CHECK (waiting = (decided_at IS NULL)),
    CONSTRAINT job_situation_answered CHECK ((stance <> '') = (decided_at IS NOT NULL))
);

-- The one question this table is asked in the request path: is anything still
-- in front of this player. Partial, because a settled moment is history and is
-- read by the log rather than by the world snapshot.
CREATE INDEX job_situation_waiting
    ON job_situation (account_id, on_day)
    WHERE waiting;

-- And the join the log makes: a page of fire records, given their situations.
CREATE INDEX job_situation_fire ON job_situation (account_id, fire_id);
