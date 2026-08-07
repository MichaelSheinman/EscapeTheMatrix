-- +goose Up

-- What the release policy (§4) needs that no table held: how long the player
-- was away before this sitting, and whether a moment the playbook could not
-- handle has been put to them yet.
--
-- technical.md §4: "**release is a server decision, not a clock event** — based
-- on activity, time away, and what would serve the player next", and "a long
-- absence does not dump the backlog; there is a cap on what gets surfaced".
-- Both halves of that sentence need a number this schema did not have.


-- How long this account had been away when its current sitting began.
--
-- `last_seen_at` cannot answer it. It is touched on every catch-up rollover,
-- which is every page load, so by the time a walk releases anything the gap has
-- already been closed by the act of measuring it — a player back from a
-- fortnight would look, to the release policy, exactly like one who refreshed.
--
-- So the gap is *kept* rather than derived, and it is kept for the sitting
-- rather than for the request: internal/account only rewrites it when the gap
-- it observes is long enough to be a new sitting rather than a second page.
-- That is what makes "somebody who has just come back" a state that lasts as
-- long as coming back does.
--
-- NULL is an account that has never been away, which is every account on its
-- first day. Not zero: zero is a real gap and would read as "here a moment ago"
-- rather than "never measured".
ALTER TABLE account ADD COLUMN last_away interval;


-- When this moment was handed to the player as a live decision, or NULL.
--
-- §4's escalation rule: "escalate on three conditions, not one — no rule
-- matches, two rules conflict, or a rule fired and the outcome was poor.
-- Escalating only on 'no match' lets one catch-all rule silence the game
-- forever." All three are already derivable from the row — `verdict` carries
-- the first two and `outcome` the third — so the *cause* is not stored, because
-- a stored copy of a derived fact is a copy that can disagree.
--
-- What is not derivable is whether the world has already acted on it. An
-- escalation put to the player twice is the same conflict arriving twice, and a
-- record with no such column would have no way to tell the difference between
-- "not yet raised" and "raised last Tuesday".
--
-- Nullable rather than a boolean and a timestamp, for the same reason
-- `settled_at` is: the two facts are one fact.
ALTER TABLE playbook_fire ADD COLUMN raised_at timestamptz;

-- The escalation read: this player's moments that escalated and have not been
-- put to them, oldest first.
--
-- The predicate names the three conditions rather than a flag, which is what
-- keeps the index and the rule in internal/playbook from being two statements
-- of the same thing that drift. A partial index because the answer is almost
-- always "none": most moments are answered, settle well, and never come here.
CREATE INDEX playbook_fire_escalated_idx
    ON playbook_fire (account_id, at, id)
 WHERE raised_at IS NULL
   AND (verdict IN ('silent', 'clash') OR outcome = 'poor');
