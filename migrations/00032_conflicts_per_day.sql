-- +goose Up

-- More than one conflict a day (§3.15).
--
-- 00021 shipped `CONSTRAINT conflict_day UNIQUE (account_id, on_day)` — one
-- conflict per player per day — on the argument that "two in a day would be
-- twice the friction for the same amount of living". That argument was wrong,
-- and it capped the mechanic the whole game is built on.
--
-- Daily conflict is not the texture of a place. It is the core loop: the player
-- grows by resolving conflicts, and HighLevelOverview.md §3.15 says living among
-- a population means "resolving conflicts with them **every day**" — plural,
-- with no cap anywhere in the design. A world that hands over one conversation
-- per day and then goes quiet is a world with nothing in it to play.
--
-- What replaces the cap is not a bigger number. It is a rule about *people*:
--
--   **One conflict per person per day.** Somebody you have already dealt with
--   this morning does not come back the same afternoon with a second ask; the
--   next one comes from somebody else. So a day is bounded by how many people
--   the player lives among rather than by a counter, which is the same bound
--   life has and needs no tuning.
--
-- That keeps every property the old constraint was load-bearing for. It is
-- still the unique key `Conflicts.Day` inserts against, so a retried rollover, a
-- second tab hydrating and a scheduled run arriving mid-catch-up still produce
-- one row between them rather than one each — the idempotency was never about
-- the day being singular, only about the insert having a natural key.
--
-- What bounds the *unresolved* end of it is unchanged and lives in the module:
-- nothing new arises while something is waiting (`Conflicts.hold`). So a
-- playbook that covers the population absorbs a morning of conversations
-- quietly, and a hole in it stops the day at the first thing the rules could not
-- settle. That is §3.15's "good rules buy time, and the holes in your playbook
-- are exactly what demands your attention" — and it is a better throttle than a
-- per-day count, because the player can do something about it.

ALTER TABLE conflict DROP CONSTRAINT conflict_day;

ALTER TABLE conflict ADD CONSTRAINT conflict_person_day
    UNIQUE (account_id, on_day, person);


-- +goose Down

-- Down collapses a day back to its first conflict, because the old constraint
-- cannot be restored over rows that have already exercised the new rule. The
-- survivor is the earliest of each day, which is the one that day would have
-- produced under the old rule.
DELETE FROM conflict c
 USING conflict keep
 WHERE c.account_id = keep.account_id
   AND c.on_day = keep.on_day
   AND c.id > keep.id;

ALTER TABLE conflict DROP CONSTRAINT conflict_person_day;

ALTER TABLE conflict ADD CONSTRAINT conflict_day UNIQUE (account_id, on_day);
