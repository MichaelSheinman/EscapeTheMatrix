-- +goose Up

-- The walking skeleton's proof of life.
--
-- This table exists so that a click in the browser can be shown to reach
-- Postgres inside a transaction and come back with a number that was not
-- there before. It carries no game meaning, and no module in §3.1 owns it.
--
-- The first ticket that wants the space should drop it in a new forward
-- migration rather than editing this one.
CREATE TABLE skeleton_probe (
    id       bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    noted_at timestamptz NOT NULL DEFAULT now()
);
