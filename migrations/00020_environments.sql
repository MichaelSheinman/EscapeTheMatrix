-- +goose Up

-- Environments stop being districts and start being populations (§3.15).
--
-- No table changes shape here. Four columns across three modules already hold an
-- *environment key* — `factor_tenure.environment`, `playbook_rule.environment`,
-- `playbook_fire.environment` and `job_situation.environment` — and every one of
-- them was written with a district key in it, because a district was the closest
-- thing the world had to "who you are among" until `content/environments`
-- existed. This migration is the change of what those keys mean.
--
-- HighLevelOverview.md §3.15: **"an environment is defined by its people, not
-- its geography."** A population lives around a district, so the two are
-- related, and that is exactly why the keys have to move rather than stay: a
-- district with nobody authored for it is not an environment at all, and a
-- district that gains a second population later would make one key mean two
-- things. What is filed under an environment key from here on is a set of people
-- with shared values.
--
-- The mapping is the one the shipped content declares. It is written out longhand
-- rather than derived, because a migration is a historical record: the day a
-- third population arrives, this file must still say what happened in August.
--
--     east-side   →  the-landing
--     riverfront  →  the-lifts
--     downtown    →  (nobody lives there)
--     the-strip   →  (nobody lives there)
--
-- Nothing collides. No player can already have a row keyed by a population,
-- because populations did not exist before this migration ran.

UPDATE factor_tenure   SET environment = 'the-landing' WHERE environment = 'east-side';
UPDATE factor_tenure   SET environment = 'the-lifts'   WHERE environment = 'riverfront';
UPDATE playbook_rule   SET environment = 'the-landing' WHERE environment = 'east-side';
UPDATE playbook_rule   SET environment = 'the-lifts'   WHERE environment = 'riverfront';
UPDATE playbook_fire   SET environment = 'the-landing' WHERE environment = 'east-side';
UPDATE playbook_fire   SET environment = 'the-lifts'   WHERE environment = 'riverfront';
UPDATE job_situation   SET environment = 'the-landing' WHERE environment = 'east-side';
UPDATE job_situation   SET environment = 'the-lifts'   WHERE environment = 'riverfront';

-- Two districts have no population, so rules and wear filed under them are
-- filed under nobody.
--
-- The wear is deleted: `factor_tenure` is a statement about how thoroughly a
-- player has worked a set of people out, and there are no people there to have
-- worked out. Leaving the rows would leave a player counted as having mastered
-- somewhere that does not exist, which is the one thing §3.15's intelligence
-- ceiling reads.
DELETE FROM factor_tenure WHERE environment IN ('downtown', 'the-strip');

-- The rules are deleted too, and this is the loss worth being honest about: a
-- player who composed answers for Downtown loses them. There is no population to
-- move them to — that is the point of the change — and internal/playbook already
-- treats a set for an unknown environment as a set nobody can reach, so keeping
-- the rows would only mean carrying invisible ones forever.
--
-- Acceptable because this game has never been deployed (technical.md's
-- deployment section is deliberately deferred), and stated because the next
-- migration to move an environment key will not have that excuse.
DELETE FROM playbook_rule WHERE environment IN ('downtown', 'the-strip');

-- History is *not* deleted. `playbook_fire` and `job_situation` are records of
-- things that happened, and a log that quietly loses a week because the world
-- was re-described is a log nobody can trust about the weeks it kept. Both
-- columns are documented as "not a foreign key: environments are authored
-- content", and a fire filed under a district reads back as a fire in a place
-- the world no longer names — which is true.
