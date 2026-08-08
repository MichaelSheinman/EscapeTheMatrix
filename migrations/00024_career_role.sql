-- +goose Up

-- Which role the rung is held in (§3.14, and the job-board decision of
-- 2026-08-08): a tier used to name a role uniquely, and now a rung can carry
-- more than one employer — warehouse nights and the corner shop till both sit
-- on tier 1. The column holds the role's key from internal/career's catalogue.
-- NULL is a row written before roles could share a rung; reading resolves it
-- to the first role of the tier, which is exactly what the old rows meant.
ALTER TABLE career ADD COLUMN role text;
