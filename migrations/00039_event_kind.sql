-- +goose Up

-- An event no longer belongs to a family. Release is the cadence caps, the
-- authored weight and how little the player has seen a candidate, so nothing
-- reads the column or the index grouping by it.
DROP INDEX event_record_signal_idx;
ALTER TABLE event_record DROP COLUMN kind;
