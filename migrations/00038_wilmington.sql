-- +goose Up

-- The district goes by its own name. There is one district, the map calls it
-- Wilmington, and `east-side` was a key describing where it sat in a city that
-- never grew a west side. The key becomes the name the player reads.
--
--     east-side  →  wilmington
--
-- Environment rows move longhand, the way 00020 and 00023 moved them: an
-- environment key is a district key, and every table that files something
-- under one keeps filing it under the same place's current name.

UPDATE factor_tenure     SET environment = 'wilmington' WHERE environment = 'east-side';
UPDATE playbook_rule     SET environment = 'wilmington' WHERE environment = 'east-side';
UPDATE playbook_fire     SET environment = 'wilmington' WHERE environment = 'east-side';
UPDATE job_situation     SET environment = 'wilmington' WHERE environment = 'east-side';
UPDATE conflict_standing SET environment = 'wilmington' WHERE environment = 'east-side';
UPDATE conflict          SET environment = 'wilmington' WHERE environment = 'east-side';

-- Place keys carry the district as a prefix — `east-side-apartment`,
-- `east-side-clinic` — so every place inside the district moves by the same
-- substitution.

UPDATE character SET place = 'wilmington' || substr(place, 10)
 WHERE place LIKE 'east-side%';
UPDATE character SET home = 'wilmington' || substr(home, 10)
 WHERE home LIKE 'east-side%';
UPDATE event_backlog SET place = 'wilmington' || substr(place, 10)
 WHERE place LIKE 'east-side%';
UPDATE event_record SET place = 'wilmington' || substr(place, 10)
 WHERE place LIKE 'east-side%';
