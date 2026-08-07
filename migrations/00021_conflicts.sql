-- +goose Up

-- Living among a population, one day at a time (§3.15).
--
-- HighLevelOverview.md §3.15 calls daily conflict "the ongoing mechanic" and
-- "the engine of the whole loop": living among people means resolving friction
-- with them every day, the rules auto-resolve what they cover, and everything
-- else arrives as a live decision. Two tables carry it, and the split between
-- them is the design rather than normalisation.
--
--   `conflict_standing` is **what is between you and one person** — the state a
--   conflict is generated *from*. §3.15's own list: who owes whom, who was
--   slighted, what was left unresolved, who has been asked too often.
--
--   `conflict` is **what happened on one day** — which person, which authored
--   pathway, and what became of it.
--
-- Owned by internal/conflict, which is the only thing that reads or writes
-- either (§3.1). Neither holds any of the writing: the moment, the answers and
-- what each of them came to are authored content under `content/conflicts`
-- (technical.md §5.2), and a copy here would show yesterday's prose after a
-- deploy.

-- What stands between this player and one person.
--
-- One row per person they have ever had anything with, created on the first
-- conflict and updated by every one after it. Absent means nothing has ever
-- passed between them, which is a real and common state — a population is six
-- people and a new arrival owes none of them anything.
--
-- Every column is a *count of something that happened*, and that is deliberate:
-- technical.md §7 forbids a model anywhere in this path, so the thing a conflict
-- is selected from has to be arithmetic somebody can read back. There is no
-- affinity score, no relationship level, and nothing here that could be rendered
-- as a bar (§3.2).
CREATE TABLE conflict_standing (
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The population this person belongs to, by environment key (§3.15). Part
    -- of the key rather than derived, because what stands between two people is
    -- a fact about the place the two of them are in: a player who moves away
    -- and comes back finds the same landing, and the people on the other side
    -- of the city are none of its business.
    environment text        NOT NULL,
    -- The person, by the key `content/environments` authors them under. Not a
    -- foreign key: people are authored content, and a population re-authored
    -- between deploys leaves rows for somebody nobody can look up — which is
    -- read as a person who has left rather than as corruption.
    person      text        NOT NULL,

    -- The favour ledger, signed. Positive is what they owe you and negative is
    -- what you owe them, in one column because it is one fact — "who owes whom"
    -- has one answer at a time and two columns would let both be true.
    owed        smallint    NOT NULL DEFAULT 0,
    -- Times you have let them down and it was not put right.
    slighted    smallint    NOT NULL DEFAULT 0,
    -- Things left hanging: a conflict you put off, walked away from, or never
    -- came back to.
    unsettled   smallint    NOT NULL DEFAULT 0,
    -- Times they have asked you for something this month, and the month that
    -- count belongs to. The playbook's own vocabulary asks "they have asked me
    -- {value} this month", so the window is a calendar month because that is
    -- the sentence a player composes a rule out of.
    asked       smallint    NOT NULL DEFAULT 0,
    asked_in    date,

    -- How many conflicts you have had with them at all, and the last day one
    -- did. Both are what keeps a population from becoming one person: somebody
    -- nobody has seen in a week is due, and somebody who was here yesterday is
    -- not.
    met         integer     NOT NULL DEFAULT 0,
    last_day    date,

    updated_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, environment, person),

    -- The bounds the module clamps to, stated where the data is. A count that
    -- ran away would make one person permanently the most pressing thing in the
    -- world, which is the failure mode of every relationship meter ever built.
    CONSTRAINT conflict_standing_bounds CHECK (
        owed BETWEEN -9 AND 9 AND
        slighted BETWEEN 0 AND 9 AND
        unsettled BETWEEN 0 AND 9 AND
        asked BETWEEN 0 AND 9
    ),
    -- A count of asks belongs to a month or to nothing.
    CONSTRAINT conflict_standing_month CHECK ((asked = 0) OR (asked_in IS NOT NULL))
);

-- Reading a whole population's standing at once, which is what generating a
-- day's conflict does and what the surface showing it needs.
CREATE INDEX conflict_standing_among ON conflict_standing (account_id, environment);


-- One day's friction with one of them.
--
-- The same shape `job_situation` has, for the same reason: a moment that the
-- player's own rules either settled or could not. What is different is where it
-- comes from — a job situation is produced by a *role*, and this is produced by
-- *living somewhere*, which is why it is keyed by a person and by which of
-- §3.15's pressures it followed from.
CREATE TABLE conflict (
    id          bigserial   PRIMARY KEY,
    account_id  uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,

    -- The player's own local day, as a DATE, for the same reason every other
    -- day-keyed table in this schema uses one: it is already their calendar
    -- day, in their timezone, written by whichever rollover walked it.
    on_day      date        NOT NULL,

    -- Where they were living, by environment key (§3.15) — the population, not
    -- the district. The rules that were weighed are the rules in force *there*,
    -- so a log that lost the place would have lost what makes a rule set
    -- transfer or fail.
    environment text        NOT NULL,
    -- Who it was with, and which of §3.15's pressures put them at the front of
    -- the queue. The pressure is stored rather than recomputed because the
    -- standing has moved since: reading back "this happened because you owed
    -- her" is only possible if the day said so at the time.
    person      text        NOT NULL,
    strain      text        NOT NULL,

    -- The authored pathway, by key. Not its words — see above.
    conflict    text        NOT NULL,
    -- The one playbook question it put. One rather than a list, because that is
    -- what keeps a daily conflict short (§3.15: "clearly lighter than an
    -- authored challenge") — and because it makes "this question keeps going
    -- badly among these people" a GROUP BY rather than a join.
    question    text        NOT NULL,

    -- The rule-fire record this produced, in internal/playbook. Null only for a
    -- moment nothing weighed at all.
    fire_id     bigint,

    -- What was done, as the posture it amounted to, and how it went. Empty and
    -- pending while the conflict is still in front of the player.
    stance      text        NOT NULL DEFAULT '',
    outcome     text        NOT NULL DEFAULT 'pending',

    -- Whether the player's own rules settled this one, or it had to come to
    -- them. The whole of §3.15's "good rules buy time, and the holes in your
    -- playbook are exactly what demands your attention", as a column that can be
    -- counted: a week in which the rules covered two conflicts out of seven is a
    -- week with a diagnosable shape (§3.6), and neither `stance` nor `fire_id`
    -- can say which of the two it was after the player has answered by hand.
    covered     boolean     NOT NULL DEFAULT false,

    -- Whether it is still in front of the player. The one piece of world in the
    -- table: nothing new comes up while something is unresolved, which is what
    -- makes "good rules buy time, and the holes in your playbook are what demand
    -- your attention" a mechanic rather than a slogan.
    waiting     boolean     NOT NULL,

    created_at  timestamptz NOT NULL DEFAULT now(),
    decided_at  timestamptz,

    -- One conflict per player per day. Daily conflicts are the texture of a
    -- place and not its weather: two in a day would be twice the friction for
    -- the same amount of living, and it is this constraint that makes a retried
    -- rollover, a second tab hydrating and a scheduled run arriving mid-catch-up
    -- produce one between them rather than one each.
    CONSTRAINT conflict_day UNIQUE (account_id, on_day),

    -- Waiting and decided are one fact at two ages.
    CONSTRAINT conflict_settled CHECK (waiting = (decided_at IS NULL)),
    CONSTRAINT conflict_answered CHECK ((stance <> '') = (decided_at IS NOT NULL))
);

-- Is anything still in front of this player: the one question asked in the
-- request path, and the gate on generating anything new.
CREATE INDEX conflict_waiting
    ON conflict (account_id, on_day)
    WHERE waiting;

-- The join the playbook's log makes: a page of fire records, given what was
-- happening at the time.
CREATE INDEX conflict_fire ON conflict (account_id, fire_id);

-- And the pattern (§3.6, §3.15): how a window of days among one population went,
-- by person and by question. One bad outcome is noise; the same answer failing
-- every morning for a week is a thing a player can act on, and this is the index
-- that makes counting it cheap.
CREATE INDEX conflict_among ON conflict (account_id, environment, on_day);
