-- +goose Up

-- An environment goes by one term, and it is the map's (Guidelines.md: one
-- term per thing).
--
-- 00020 gave each population a name of its own — `the-landing` for the people
-- around East Side, `the-lifts` for the people around Riverfront — and that
-- second vocabulary was a mistake: nothing on the map says "the landing", so
-- every surface that named a rule set was speaking a language the city screen
-- never taught. An environment is still defined by its people (§3.15) — what
-- it is *called* is the district they live in.
--
-- The mapping is 00020's, run the other way, written out longhand for the same
-- reason that one was: a migration is a historical record.
--
--     the-landing  →  east-side
--     the-lifts    →  riverfront
--
-- Nothing collides. No row keyed `east-side` or `riverfront` can exist today:
-- 00020 moved every environment key off the district vocabulary, and every
-- writer since has taken its keys from content/environments.
--
-- History rows move too, exactly as they did in 00020: a fire or a day's
-- conflict is a record of something that happened, and it stays readable by
-- staying filed under the name the world now uses for the same place.

UPDATE factor_tenure      SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE factor_tenure      SET environment = 'riverfront' WHERE environment = 'the-lifts';
UPDATE playbook_rule      SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE playbook_rule      SET environment = 'riverfront' WHERE environment = 'the-lifts';
UPDATE playbook_fire      SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE playbook_fire      SET environment = 'riverfront' WHERE environment = 'the-lifts';
UPDATE job_situation      SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE job_situation      SET environment = 'riverfront' WHERE environment = 'the-lifts';
UPDATE conflict_standing  SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE conflict_standing  SET environment = 'riverfront' WHERE environment = 'the-lifts';
UPDATE conflict           SET environment = 'east-side'  WHERE environment = 'the-landing';
UPDATE conflict           SET environment = 'riverfront' WHERE environment = 'the-lifts';
