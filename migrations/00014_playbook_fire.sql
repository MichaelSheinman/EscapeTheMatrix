-- +goose Up

-- Rule-fire records (§3.14): what the character did while the player was
-- elsewhere, and on whose authority.
--
-- technical.md §3.3 enumerates the history surfaces this game keeps and calls
-- that list "the contract" — there is no write-on-every-change audit table, and
-- history lives per domain. "Playbook rule-fire records — §3.14 'rule 4 fired';
-- diagnosis evidence" is the line these two tables answer, and nothing wider.
--
-- Why it is worth a table at all: §3.6's diagnosis is the player pointing at
-- their own accumulated evidence, and §3.15's transfer failure is only legible
-- as a *pattern* — "one bad outcome is noise, the same rule failing daily for a
-- week is a pattern". A pattern is a query over rows. Current state cannot hold
-- one, and a rule the player has since edited would have taken its own history
-- with it.
--
-- Two tables rather than one, because a moment can put more than one standing
-- question at once and the interesting cases are exactly the ones with two rows
-- underneath: two rules that both reached an action and disagreed. A jsonb blob
-- would hold the same facts and answer none of the questions asked of it —
-- "which of my answers keeps going wrong", "how often does this escalate" — as
-- an index scan.
CREATE TABLE playbook_fire (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- When it happened, from the caller's clock (§3.1) rather than from now():
    -- the rollover replays days, and a record stamped with the moment it was
    -- written would put a Tuesday's conflicts on a Friday.
    at          timestamptz NOT NULL,

    -- Which rule set was in force. §3.15's whole thesis is that the right rules
    -- depend on where you are, so a record that did not say where is evidence
    -- for nothing.
    environment text        NOT NULL,

    -- The authored key of the moment (#28, #30). Text rather than a foreign key
    -- for the same reason `environment` is: the situation catalogue is content
    -- in the repo, not rows, and a moment withdrawn from it must not take the
    -- history of what it did with it.
    situation   text        NOT NULL,

    -- answered | silent | clash. The last two are two of technical.md §4's
    -- three escalation conditions, recorded rather than inferred, because "this
    -- came to me because nothing covered it" and "because two of my answers
    -- disagreed" are different things to tell a player.
    verdict     text        NOT NULL,

    -- pending | good | mixed | poor — §4's third escalation condition. It is
    -- separate from the verdict because it is not known at the same time: an
    -- active job's consequences run for weeks (§3.4), so the record is written
    -- when the rule fires and settled when the world gets round to answering.
    outcome     text        NOT NULL,
    settled_at  timestamptz,

    CONSTRAINT playbook_fire_verdict CHECK (verdict IN ('answered', 'silent', 'clash')),
    CONSTRAINT playbook_fire_outcome CHECK (outcome IN ('pending', 'good', 'mixed', 'poor')),
    CONSTRAINT playbook_fire_settled CHECK ((outcome = 'pending') = (settled_at IS NULL))
);

-- The log's read: this player's moments, newest first.
CREATE INDEX playbook_fire_log_idx ON playbook_fire (account_id, at DESC, id DESC);


-- One row per question the moment put — including the ones the player has no
-- answer to, because "this happened and you had nothing to say about it" is the
-- most useful line in the whole log (§3.14: gaps in the playbook are what demand
-- attention).
--
-- The rule is stored as it stood. Not a reference to playbook_rule: editing a
-- sentence *is* the change (§3.14 has no draft and no save), so a record that
-- pointed at the current row would rewrite its own history the next time the
-- player revised — and revising after a failure is the entire loop.
CREATE TABLE playbook_fire_rule (
    fire_id      bigint   NOT NULL REFERENCES playbook_fire (id) ON DELETE CASCADE,

    question     text     NOT NULL,

    -- The question's authored number at the time, which is how a log line cites
    -- an answer ("answer 4 fired"). Stored rather than looked up, so inserting
    -- a question later does not renumber what the player already read.
    n            smallint NOT NULL,

    -- unanswered | undecided | weighed | fired. `unanswered` is no complete rule
    -- at all; `undecided` is a rule the moment did not say enough to apply.
    -- Keeping those apart is the difference between "compose an answer to this"
    -- and "your answer turns on something nobody told you".
    state        text     NOT NULL,

    join_word    text,

    -- Every part of the sentence, and — the point of the whole table — whether
    -- each condition was met. §3.14: "the player can see exactly which condition
    -- matched and which action fired. That is what makes transfer failure
    -- diagnosable rather than a matter of interpretation." A record that kept
    -- only the verdict would have thrown that away on the way to the table.
    -- NULL is "the moment did not say", which is a third answer and not a false.
    when_1_term  text, when_1_value text, when_1_met boolean,
    when_2_term  text, when_2_value text, when_2_met boolean,

    -- Whether the whole `when` held. NULL for a rule that could not be applied,
    -- and — together with the two action slots below — which of them fired: a
    -- held `when` fires the `then` and an unheld one fires the `otherwise`.
    -- Storing the answer twice would be storing it once and hoping.
    matched      boolean,

    -- Both halves of the answer as the player composed them, not only the one
    -- taken. A log line reads as the sentence it came from, and a record that
    -- kept only the branch that fired would print "then say yes otherwise …"
    -- for a rule that had a perfectly good otherwise.
    then_term    text, then_value  text,
    else_term    text, else_value  text,

    -- What the action that fired amounted to, out of the closed set in
    -- internal/playbook. Stored because it is what a clash was about, and
    -- because the content it came from may be retuned later: a record that
    -- re-derived it would change what an old disagreement had been about.
    stance       text,

    PRIMARY KEY (fire_id, question),

    CONSTRAINT playbook_fire_rule_state CHECK (
        state IN ('unanswered', 'undecided', 'weighed', 'fired')),
    CONSTRAINT playbook_fire_rule_acted CHECK (
        (state IN ('weighed', 'fired')) = (matched IS NOT NULL)),
    CONSTRAINT playbook_fire_rule_answer CHECK (
        (state = 'unanswered') = (then_term IS NULL AND else_term IS NULL)),
    CONSTRAINT playbook_fire_rule_then  CHECK (then_term IS NOT NULL OR then_value IS NULL),
    CONSTRAINT playbook_fire_rule_else  CHECK (else_term IS NOT NULL OR else_value IS NULL),
    CONSTRAINT playbook_fire_rule_when_1 CHECK (when_1_term IS NOT NULL OR when_1_value IS NULL),
    CONSTRAINT playbook_fire_rule_when_2 CHECK (when_2_term IS NOT NULL OR when_2_value IS NULL)
);

-- §3.6's pattern read: how one answer has been going, over a window. The
-- question leads because that is what a player points at — "this rule keeps
-- failing" — and the fire is what carries the date and the outcome.
CREATE INDEX playbook_fire_rule_question_idx ON playbook_fire_rule (question, fire_id);
