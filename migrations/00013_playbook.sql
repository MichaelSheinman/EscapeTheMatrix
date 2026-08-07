-- +goose Up

-- The playbook (§3.14): the rules a player composes for their own judgement.
--
-- Three of the ticket's rules are held by this schema rather than by Go, which
-- is deliberate — a rule a bug can skip is a rule the design does not actually
-- have.
--
--   * **At most two conditions, and no nesting.** There are exactly two
--     condition columns. A third term, or a parenthesised sub-expression, is
--     not something a future caller can get wrong: there is nowhere to put it.
--     §3.14 makes that ceiling a design choice — two terms is as much as a rule
--     can carry and still read as a sentence — and this is where it is kept.
--   * **A number cannot exist without a term to hold it.** Each part of the
--     sentence is a (term, value) pair, and the CHECKs below refuse a value
--     whose term is NULL. Which of the closed lists that value has to come from
--     is content and is checked in Go; that it belongs to *something* is
--     structural.
--   * **The second condition cannot exist unless the sentence has two.**
--     `conditions` is stored rather than inferred from whether the second slot
--     is filled, because those are different states: one condition is a
--     complete sentence, and two conditions with the second still blank is a
--     player mid-edit whose rule must not be in force.
--
-- What is *not* here is a draft. There is one row per question per environment
-- and no committed copy beside it, because editing the sentence is the change
-- (§3.14's loop runs while the world runs). What replaces a save button is the
-- blank: every column below is nullable except the bookkeeping, and a sentence
-- with a hole in it is simply not covering its question.
--
-- Nor is there a rule *set* table. §3.15's rule sets are context-tagged, and the
-- context is the environment — so the tag is a column and a set is every row
-- carrying it. A table would add a name the world already has and an id nobody
-- would address anything by.
CREATE TABLE playbook_rule (
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- Which rule set. An environment key — a district today, a population once
    -- #29 lands. Not a foreign key: the environments are authored content in
    -- Go (internal/city), not rows, and §17 leaves the city's district list
    -- open. A key that stops being an environment reads back as a set nobody
    -- is offered rather than as a broken reference.
    environment text        NOT NULL,

    -- Which of the standing questions this answers. Content again, and a
    -- question renamed out of the vocabulary reads back as no answer at all,
    -- which is the safe direction for that mistake to fall in.
    question    text        NOT NULL,

    -- One or two. The ceiling is also the number of columns below, so this
    -- cannot describe a sentence the table could not hold.
    conditions  smallint    NOT NULL,

    -- The only word in a rule that changes the logic, which is why it is the
    -- only piece of grammar that is also stored.
    join_word   text        NOT NULL,

    when_1_term  text,
    when_1_value text,
    when_2_term  text,
    when_2_value text,

    then_term    text,
    then_value   text,
    else_term    text,
    else_value   text,

    updated_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, environment, question),

    CONSTRAINT playbook_rule_conditions CHECK (conditions IN (1, 2)),
    CONSTRAINT playbook_rule_join       CHECK (join_word IN ('and', 'or')),
    CONSTRAINT playbook_rule_second     CHECK (
        conditions = 2 OR (when_2_term IS NULL AND when_2_value IS NULL)),
    CONSTRAINT playbook_rule_when_1     CHECK (when_1_term IS NOT NULL OR when_1_value IS NULL),
    CONSTRAINT playbook_rule_when_2     CHECK (when_2_term IS NOT NULL OR when_2_value IS NULL),
    CONSTRAINT playbook_rule_then       CHECK (then_term   IS NOT NULL OR then_value   IS NULL),
    CONSTRAINT playbook_rule_else       CHECK (else_term   IS NOT NULL OR else_value   IS NULL)
);

-- The matcher's read (#19): every rule in force where the player is standing.
-- The primary key already leads with the account, so this is the same prefix
-- one column shorter and exists for the environment-scoped scan rather than for
-- the per-question lookup.
CREATE INDEX playbook_rule_set_idx ON playbook_rule (account_id, environment);


-- Which words of the vocabulary a player has earned (§3.6).
--
-- Only the earned ones. The vocabulary everybody starts with is authored in
-- content/playbook and is read from there, so adding a starting term reaches
-- players who already exist — and this table means exactly "what struggle has
-- unlocked", which is the only thing worth keeping per player.
--
-- A term is `question/kind/key`, because keys are unique inside one pool of one
-- question and nowhere wider: a question can have both a condition and an action
-- called "leave", and one of them does.
CREATE TABLE playbook_unlock (
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    term        text        NOT NULL,
    unlocked_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, term)
);

-- No rows created here, and none created on a player's first read either. A
-- player who has answered nothing has no rules, which is exactly true, and the
-- loop in §3.14 begins from there.
