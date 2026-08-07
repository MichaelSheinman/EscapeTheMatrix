-- +goose Up

-- The job a player holds, and how far up the ladder they have ever been (§3.14).
--
-- Owned by internal/career, which is the only thing that reads or writes it
-- (§3.1). What it deliberately does not hold is anything about what a tier *is*:
-- the pay, the hours, and the capability a rung is hired against are all in
-- internal/simulation, where they are numbers a unit test can tune (§17's "job
-- tier thresholds"). A copy of them in a table would be a second answer waiting
-- to disagree with the first, and a migration every time somebody retuned one.
--
-- Four columns and one of them is a ledger. That is the whole of it, because
-- §3.4 makes income *structural*: what has to be persisted is which rung is
-- held and which days have been paid for, and everything else — what today
-- earns, whether the step up is open, what the shift does to a body — is
-- composed on read from those two facts and the pure core (§3.3).
CREATE TABLE career (
    -- One career per account, so the account id is the key.
    account_id  uuid        PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,

    -- The rung currently held. Zero is holding no job, which is where everybody
    -- starts and where leaving one returns them to — a real state rather than a
    -- missing row, so that "unemployed" and "never played" are the same shape
    -- and nothing downstream has to tell them apart.
    tier        smallint    NOT NULL DEFAULT 0 CHECK (tier >= 0),

    -- The highest rung ever held. Kept because leaving a job must not take back
    -- the interview that was passed to get into it: an interview is a challenge,
    -- and a challenge is once ever (§5.1), so a player who walked away from the
    -- dispatch desk and wants it back has nothing left to prove and must not be
    -- asked to prove it again.
    reached     smallint    NOT NULL DEFAULT 0 CHECK (reached >= 0),

    -- When the current rung was taken. Null while holding nothing.
    since       timestamptz,

    -- The last local day this player has been paid for, and the reason a
    -- structural wage is idempotent. The rollover walks a calendar (§3.3) and
    -- may walk the same day twice — a retried job, a second tab hydrating, a
    -- scheduled run arriving during a catch-up — and this is what makes the
    -- second walk pay nothing.
    --
    -- A DATE because it is the player's own calendar day, in their own
    -- timezone, exactly like `character.earned_on` beside it. Null means
    -- nothing has ever been paid, which is only the state of somebody who has
    -- never held a job: taking one stamps it to the day before the current one,
    -- so a fortnight of catch-up cannot pay for a fortnight of employment that
    -- never happened.
    paid_through date,

    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    -- You cannot be holding a rung higher than the highest you ever reached.
    -- The two columns are the same fact at two ages, and this is what keeps
    -- them from drifting into disagreeing about it.
    CONSTRAINT career_reached CHECK (reached >= tier),
    -- Holding a job and having started it are one fact.
    CONSTRAINT career_since CHECK ((tier > 0) = (since IS NOT NULL))
);

-- Every account that already exists gets a career, so that signing in on an
-- account made before this migration lands in a world rather than in an error.
-- Unemployed, which is where §3.14's ladder starts.
INSERT INTO career (account_id) SELECT id FROM account;
