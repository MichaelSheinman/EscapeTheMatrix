-- +goose Up

-- A rule-fire row cites its question by key alone. Playbook prose names a
-- question by what it is about, never by a number, so the authored number the
-- rows carried had no reader.
ALTER TABLE playbook_fire_rule DROP COLUMN n;
