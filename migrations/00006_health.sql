-- +goose Up

-- The body, and what the player can see of it (§3.16).
--
-- Owned by internal/health. Two things are stored and they are deliberately
-- different kinds of thing: the panel, which is the simulation's current state
-- and is always complete, and the instrumentation, which is only what the
-- player has bought the ability to *see*. Nothing about a tier appears in the
-- panel table, because instrumentation changes resolution rather than health —
-- buying a watch does not make anyone healthier.
CREATE TABLE health_panel (
    account_id uuid PRIMARY KEY REFERENCES account (id) ON DELETE CASCADE,

    -- The hidden drivers. No tier reveals these directly; they are what the
    -- visible metrics are readings of.
    sleep_debt  double precision NOT NULL,
    fitness     double precision NOT NULL,
    height_m    double precision NOT NULL,

    -- Baseline: free, from the start.
    last_sleep  double precision NOT NULL,
    resting_hr  double precision NOT NULL,
    weight_kg   double precision NOT NULL,

    -- Wearable.
    hrv         double precision NOT NULL,
    recovery    double precision NOT NULL,
    load        double precision NOT NULL,

    -- Clinical.
    glucose     double precision NOT NULL,
    crp         double precision NOT NULL,
    ldl         double precision NOT NULL,
    cortisol    double precision NOT NULL,

    -- The last day applied, in the player's own calendar (§3.3). The daily
    -- rollover reads this to know whether today has already been lived.
    last_day    date        NOT NULL DEFAULT '0001-01-01',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Biological age is NOT here. It is composed on read from the columns above
-- (§3.3, §3.5) — storing it would create a second answer that goes stale the
-- moment any of its inputs moves.

-- The biomarker time series: §3.2's first named history surface, and the
-- evidence a diagnosis is later argued from (§3.6).
--
-- One row per day lived, holding the whole panel — including the metrics the
-- player cannot see yet. Buying instrumentation later has to reveal a *history*
-- and not just a live number, because "your recovery has been collapsing for
-- three weeks" is the sentence the whole diagnosis mechanic is built on, and it
-- cannot be said about data that was never recorded.
CREATE TABLE health_sample (
    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    -- The player's local day this sample is for. One per day, so a rollover
    -- that runs twice cannot double-record.
    on_day     date        NOT NULL,
    taken_at   timestamptz NOT NULL,

    sleep_debt double precision NOT NULL,
    fitness    double precision NOT NULL,
    last_sleep double precision NOT NULL,
    resting_hr double precision NOT NULL,
    weight_kg  double precision NOT NULL,
    hrv        double precision NOT NULL,
    recovery   double precision NOT NULL,
    load       double precision NOT NULL,
    glucose    double precision NOT NULL,
    crp        double precision NOT NULL,
    ldl        double precision NOT NULL,
    cortisol   double precision NOT NULL,

    PRIMARY KEY (account_id, on_day)
);

-- Instrumentation: what the player has bought the ability to see.
--
-- One row per tier owned, rather than a column per tier on the character, so
-- that adding a tier is content rather than a migration — and so that when the
-- purchase happened is a fact the game keeps.
CREATE TABLE health_instrument (
    account_id uuid        NOT NULL REFERENCES account (id) ON DELETE CASCADE,
    tier       text        NOT NULL,
    bought_at  timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (account_id, tier)
);

-- There is deliberately no backfill here.
--
-- A body's reference values live in internal/simulation.Starting and nowhere
-- else; writing them into this file as literals would be a second copy that
-- drifts the first time they are tuned. internal/health creates the panel the
-- first time it is asked for one, which covers accounts made before this
-- migration and accounts made after it with the same code path.
