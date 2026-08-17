-- +goose Up

-- Who answers a conflict: you, or your rules (§3.14, §3.15).
--
-- The playbook is the **fallback**, not the first resort. 00021 and everything
-- built on it had this the wrong way round: every conflict was weighed by the
-- rules and only what they could not settle ever reached the player. That makes
-- the phone a complaints desk for rule failures rather than a conversation.
--
-- The rule is presence:
--
--   * **Offline** — the character handles it. The rules resolve it, and the
--     player reads back what happened. This is §3.14's "it happens without you",
--     and it is the whole reason a playbook is worth composing.
--   * **Online** — it goes to the phone and the player answers it themselves.
--     Somebody who is sitting there when a neighbour asks them something is a
--     person having a conversation, not a person watching a rule engine have one
--     on their behalf.
--   * **Online and left unanswered** — it goes back to the rules once they are
--     gone. Not answering is not a third outcome; it is being away, arrived at
--     slowly.
--
-- Two columns carry it, and they are two facts rather than one.

-- Whether this conflict is waiting because the player was *there*, as opposed to
-- waiting because their rules could not settle it.
--
-- The distinction is the whole of the third case above. A conflict deferred to a
-- present player has an answer its rules would have given and may be handed back
-- to them when they leave. A conflict waiting because the playbook was silent or
-- because two rules contradicted each other has no such answer — handing it to
-- the rules would be handing it to the thing that already failed — so it stays
-- in front of the player until they deal with it. One column, because
-- collapsing them would make an unanswerable moment auto-resolve into whichever
-- of two contradicting rules happened to be checked first.
ALTER TABLE conflict ADD COLUMN deferred boolean NOT NULL DEFAULT false;

-- What the rules would have done, as the posture it amounts to.
--
-- Written when the conflict is deferred and read only if the player leaves
-- without answering. Stored rather than recomputed for the reason `strain` is:
-- the playbook may have been edited in between, and "your rules answered this
-- while you were out" has to mean the rules as they stood when it arrived — not
-- the ones the player rewrote at midnight after seeing it go badly.
--
-- Empty where the rules had no answer, which is exactly the case that must never
-- lapse.
ALTER TABLE conflict ADD COLUMN fallback text NOT NULL DEFAULT '';

-- The sweep's own index: what is deferred, still waiting, and now overdue for
-- its fallback. Partial, because the answered ones are the overwhelming majority
-- of the table and none of them is ever a candidate.
CREATE INDEX conflict_deferred
    ON conflict (account_id)
    WHERE waiting AND deferred;


-- +goose Down

DROP INDEX conflict_deferred;
ALTER TABLE conflict DROP COLUMN fallback;
ALTER TABLE conflict DROP COLUMN deferred;
