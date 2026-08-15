-- +goose Up

-- Diagnosis is out of V1 (§3.6, 2026-08-09).
--
-- The mechanic was built and shipped, then withdrawn: the gate it fed — a
-- routine arriving only once the player had named the problem it solves — was
-- taken off (#32), and with nothing reading the conclusions the table was the
-- last thing left standing. `internal/diagnosis` and its authored causes are
-- already gone; this drops the storage behind them.
--
-- Nothing is migrated out. The rows are conclusions, not history: what they
-- were argued from lives in `playbook_fire` and `health_sample` and is
-- untouched, so if diagnosis returns it can be re-argued from the same series
-- rather than restored from a dead table. Forward-only (see migrations/embed.go
-- — a merged migration is corrected by a new one, never rewritten).
DROP TABLE IF EXISTS diagnosis;
