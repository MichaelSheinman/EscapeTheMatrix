// Package health owns the player's body and what they can see of it.
//
// HighLevelOverview.md §3.16 splits those two things apart, and the split is
// the point of the package: **the simulation always runs the full model, and
// instrumentation changes the player's resolution on their own body rather than
// their health.** Buying a watch does not make anyone healthier. A player
// without one still suffers the real HRV collapse and simply cannot see the
// cause — which *is* the diagnosis problem (§3.6), and the reason the game has
// anything to teach.
//
// So: internal/simulation computes the whole panel for everyone (it does not
// know instrumentation exists), this package stores it, and visibility is
// applied at the last possible moment — on read, on the way out. There is no
// code path in which a tier changes a number.
//
// It owns `health_panel`, `health_sample` and `health_instrument` (§3.1), and
// spends money through character.Wallet rather than by touching another
// module's table.
package health

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

var (
	// ErrAlreadyLived is one day per day (§3.1). The request is fine and it is
	// the calendar that is in the way.
	ErrAlreadyLived = errors.New("health: today has already been lived")
	// ErrNoSuchTier is an instrument that does not exist.
	ErrNoSuchTier = errors.New("health: no such instrument")
	// ErrAlreadyOwned is buying the same instrument twice.
	ErrAlreadyOwned = errors.New("health: already owned")
)

/*
Tier is a level of instrumentation: what the player has bought the ability to
see (§3.16).

Baseline is free and needs nothing. The other two are money sinks that buy
*insight* rather than power, which is what ties the economy to the learning loop
— and is the mechanical justification for money's rising contribution to Karma
(§3.5), since early money genuinely does buy the ability to see and act.
*/
type Tier struct {
	Key   string
	Name  string
	Price simulation.Money
	// Buys says what becomes visible, in the player's language. It never names
	// a value — the whole point of a locked metric is that its number is not
	// the thing being withheld from you, its *existence* is what you are being
	// sold.
	Buys string
}

// Baseline is what every player has from the start, and is not for sale.
var Baseline = Tier{
	Key:  "baseline",
	Name: "Baseline",
	Buys: "what you can notice without any instrument: how long you slept, your pulse, your weight",
}

// Wearable and Clinical are the two that are.
var (
	Wearable = Tier{
		Key:   "wearable",
		Name:  "A good watch",
		Price: simulation.Dollars(400),
		Buys:  "heart-rate variability, recovery, training load, aerobic capacity",
	}
	Clinical = Tier{
		Key:   "clinical",
		Name:  "Bloodwork",
		Price: simulation.Dollars(950),
		Buys:  "fasting glucose, inflammation, cholesterol, the stress axis",
	}
)

// ForSale is the instrumentation a player can buy, in the order it is offered.
var ForSale = []Tier{Wearable, Clinical}

func tier(key string) (Tier, bool) {
	for _, t := range append([]Tier{Baseline}, ForSale...) {
		if t.Key == key {
			return t, true
		}
	}
	return Tier{}, false
}

/*
Metric is one readout, and the tier that can see it.

Every one is a real-world concept (§3.16's rule), and `Means` is the sentence
that makes it teachable rather than a number with a label. The value is read out
of the body by a function rather than stored beside it, which is what keeps this
list a description of the panel rather than a second copy of it.
*/
type Metric struct {
	Key   string
	Name  string
	Unit  string
	Means string
	Tier  Tier
	// Lower says which direction is the healthy one, so a client can colour a
	// trend without being told a rule.
	Lower bool
	// Decimals it is worth reading to.
	Decimals int
	Read     func(simulation.Body) float64
}

// Metrics is the whole panel, in the order it reads.
//
// The tier on each one is the *only* place visibility is decided. Adding a
// metric is a line here; moving one between tiers is a word.
var Metrics = []Metric{
	{
		Key: "sleep", Name: "Slept last night", Unit: "h", Tier: Baseline, Decimals: 1,
		Means: "the input everything else answers to",
		Read:  func(b simulation.Body) float64 { return b.LastSleep },
	},
	{
		Key: "resting-hr", Name: "Resting heart rate", Unit: "bpm", Tier: Baseline, Lower: true,
		Means: "lower with fitness, higher with debt and unrecovered load",
		Read:  func(b simulation.Body) float64 { return b.RestingHR },
	},
	{
		Key: "weight", Name: "Weight", Unit: "kg", Tier: Baseline, Decimals: 1,
		Means: "an energy balance, moving in grams rather than kilos",
		Read:  func(b simulation.Body) float64 { return b.WeightKg },
	},
	{
		Key: "hrv", Name: "Heart-rate variability", Unit: "ms", Tier: Wearable,
		Means: "the autonomic balance sheet — it falls under sleep loss and load, and returns slowly",
		Read:  func(b simulation.Body) float64 { return b.HRV },
	},
	{
		Key: "recovery", Name: "Recovery", Unit: "", Tier: Wearable,
		Means: "this morning's HRV against what your own body is capable of",
		Read:  func(b simulation.Body) float64 { return b.Recovery },
	},
	{
		Key: "vo2max", Name: "Aerobic capacity", Unit: "ml/kg/min", Tier: Wearable, Decimals: 1,
		Means: "earned slowly, lost slowly, and barely earned at all on a wrecked body",
		Read:  func(b simulation.Body) float64 { return b.Fitness },
	},
	{
		Key: "load", Name: "Training load", Unit: "", Tier: Wearable, Lower: false,
		Means: "not bad in itself; bad without recovery underneath it",
		Read:  func(b simulation.Body) float64 { return b.Load },
	},
	{
		Key: "glucose", Name: "Fasting glucose", Unit: "mmol/L", Tier: Clinical, Lower: true, Decimals: 2,
		Means: "one short night measurably reduces insulin sensitivity the next morning",
		Read:  func(b simulation.Body) float64 { return b.Glucose },
	},
	{
		Key: "crp", Name: "Inflammation", Unit: "mg/L", Tier: Clinical, Lower: true, Decimals: 2,
		Means: "the marker that catches doing everything right except sleeping",
		Read:  func(b simulation.Body) float64 { return b.CRP },
	},
	{
		Key: "ldl", Name: "LDL cholesterol", Unit: "mmol/L", Tier: Clinical, Lower: true, Decimals: 2,
		Means: "moves over weeks, so a clean fortnight shows nothing at first",
		Read:  func(b simulation.Body) float64 { return b.LDL },
	},
	{
		Key: "cortisol", Name: "Morning cortisol", Unit: "nmol/L", Tier: Clinical, Lower: true,
		Means: "the stress axis: sleep debt first, then hard days taken without recovery",
		Read:  func(b simulation.Body) float64 { return b.Cortisol },
	},
}

// Panel is everything known about a player's body right now.
type Panel struct {
	Body simulation.Body
	// Owned is the instrumentation bought, always including Baseline.
	Owned []Tier
	// LastDay is the last local day applied. Zero when none has been.
	LastDay simulation.Date
	// History is the biomarker time series, oldest first, whole panels.
	History []Sample
}

// Sample is one day in the time series.
type Sample struct {
	On   simulation.Date
	Body simulation.Body
}

// Sees reports whether this panel's instrumentation can see a metric.
//
// The one place visibility is decided, and it is applied on the way *out* of
// the module — nothing upstream of here has behaved differently.
func (p Panel) Sees(m Metric) bool {
	if m.Tier.Key == Baseline.Key {
		return true
	}
	for _, t := range p.Owned {
		if t.Key == m.Tier.Key {
			return true
		}
	}
	return false
}

// Health is the module.
type Health struct {
	clk    simulation.Clock
	wallet character.Wallet
}

// New binds the module to the clock and to the wallet it spends through.
//
// The wallet is an interface rather than *character.Characters, which is §3.1's
// rule about cross-module access made structural: this package cannot read a
// district, a home or a timezone even by accident, and a test can hand it
// something else entirely.
func New(clk simulation.Clock, wallet character.Wallet) *Health {
	return &Health{clk: clk, wallet: wallet}
}

// historyDays is how much of the time series is read for display. Enough to see
// a trend rather than a number, which is the whole reason it is kept.
const historyDays = 21

// Ensure creates the panel if the account has none, and returns everything.
func (h *Health) Ensure(ctx context.Context, tx pgx.Tx, accountID string) (Panel, error) {
	if err := h.create(ctx, tx, accountID); err != nil {
		return Panel{}, err
	}
	return h.Of(ctx, tx, accountID)
}

// create writes the starting body, if there is not one already.
//
// Called by everything that touches the panel rather than only at sign-in, so
// that an account made before this module existed has a body the first time it
// is asked about rather than a 500. Doing nothing is the overwhelmingly common
// case and costs one insert that conflicts.
func (h *Health) create(ctx context.Context, tx pgx.Tx, accountID string) error {
	// The reference body lives in the pure core and nowhere else, so the
	// starting values cannot drift between a migration and a tuning pass.
	// §3.13's onboarding (#33) is what makes height and weight a real answer.
	b := simulation.Starting(1.78, 78)

	if _, err := tx.Exec(ctx, `
		INSERT INTO health_panel (
			account_id, sleep_debt, fitness, height_m,
			last_sleep, resting_hr, weight_kg,
			hrv, recovery, load,
			glucose, crp, ldl, cortisol)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (account_id) DO NOTHING`,
		accountID, b.SleepDebt, b.Fitness, b.HeightM,
		b.LastSleep, b.RestingHR, b.WeightKg,
		b.HRV, b.Recovery, b.Load,
		b.Glucose, b.CRP, b.LDL, b.Cortisol,
	); err != nil {
		return fmt.Errorf("health: ensure: %w", err)
	}
	return nil
}

// Of reads the panel, the instrumentation, and the recent time series.
func (h *Health) Of(ctx context.Context, tx pgx.Tx, accountID string) (Panel, error) {
	p, err := h.body(ctx, tx, accountID, false)
	if err != nil {
		return Panel{}, err
	}

	rows, err := tx.Query(ctx, `
		SELECT tier FROM health_instrument WHERE account_id = $1 ORDER BY bought_at`, accountID)
	if err != nil {
		return Panel{}, fmt.Errorf("health: read instruments: %w", err)
	}
	defer rows.Close()
	p.Owned = []Tier{Baseline}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return Panel{}, fmt.Errorf("health: read instruments: %w", err)
		}
		if t, ok := tier(key); ok {
			p.Owned = append(p.Owned, t)
		}
	}
	if err := rows.Err(); err != nil {
		return Panel{}, fmt.Errorf("health: read instruments: %w", err)
	}

	p.History, err = h.history(ctx, tx, accountID)
	if err != nil {
		return Panel{}, err
	}
	return p, nil
}

/*
Body is the panel with nothing else attached, for another module to read.

The narrow half of Of, and it exists because §3.1 requires cross-module access to
go through an interface: internal/factor composes Karma from the biological age,
which is a pure function of this struct, so one value is the whole of what has to
cross. A module handed this cannot reach an instrument, a sample, or the ledger
date — and nothing about instrumentation applies here, because a tier decides
what a *player* may see and never what the simulation is (§3.16).
*/
func (h *Health) Body(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Body, error) {
	if err := h.create(ctx, tx, accountID); err != nil {
		return simulation.Body{}, err
	}
	p, err := h.body(ctx, tx, accountID, false)
	if err != nil {
		return simulation.Body{}, err
	}
	return p.Body, nil
}

// Lived applies one day of living, on a date.
//
// One game day is one real calendar day (§3.1), so a day already lived is
// refused rather than applied twice — and the day is the *player's* own (§3.3),
// which is why it arrives as a parameter rather than being read off a clock
// here: this module has no idea what timezone anybody is in, and the caller
// that does is the one that can say.
//
// The date also has to be a parameter for the rollover (#10) to exist at all. A
// player who was away for three days has three days to live, in order, and each
// of them is a different weekday to the routine that produced it. A function
// that could only ever apply "today" could catch nobody up.
//
// The rules are the pure core's; what happens here is the part that has to be
// true about a database, and the effects the core returns are handed back for
// the caller to decide what they mean.
func (h *Health) Lived(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, day simulation.Day) (Panel, simulation.Effects, error) {
	if err := h.create(ctx, tx, accountID); err != nil {
		return Panel{}, nil, err
	}

	before, err := h.body(ctx, tx, accountID, true)
	if err != nil {
		return Panel{}, nil, err
	}

	// Strictly forward. Refusing a day that is not after the last one applied
	// covers both the day lived twice and the rollover that arrives out of
	// order, and it is the guard that makes a retried job harmless: the second
	// attempt finds the ledger already past its date and declines.
	if !before.LastDay.Before(on) {
		return Panel{}, nil, ErrAlreadyLived
	}

	after, fx := simulation.Advance(instant(h.clk, on), before.Body, day)

	if err := h.write(ctx, tx, accountID, after, on); err != nil {
		return Panel{}, nil, err
	}

	// The sample is written for every player, at every tier, including the
	// metrics this one cannot see. Revealing a *history* is what makes buying
	// an instrument worth anything — "your recovery has been collapsing for
	// three weeks" cannot be said about data nobody recorded (§3.2, §3.6).
	for _, e := range fx {
		s, ok := e.(simulation.Sampled)
		if !ok {
			continue
		}
		if err := h.sample(ctx, tx, accountID, on, s); err != nil {
			return Panel{}, nil, err
		}
	}

	panel, err := h.Of(ctx, tx, accountID)
	if err != nil {
		return Panel{}, nil, err
	}
	return panel, fx, nil
}

// Buy takes an instrument's price out of the player's money and records it.
//
// The spend goes through the Wallet interface, in the caller's transaction, so
// a purchase that fails to record cannot have taken the money and a payment
// that fails cannot have handed over the instrument.
func (h *Health) Buy(ctx context.Context, tx pgx.Tx, accountID, key string) (Panel, simulation.Money, error) {
	t, ok := tier(key)
	if !ok || t.Key == Baseline.Key {
		return Panel{}, 0, ErrNoSuchTier
	}
	if err := h.create(ctx, tx, accountID); err != nil {
		return Panel{}, 0, err
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO health_instrument (account_id, tier) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, accountID, t.Key)
	if err != nil {
		return Panel{}, 0, fmt.Errorf("health: buy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Panel{}, 0, ErrAlreadyOwned
	}

	if err := h.wallet.Spend(ctx, tx, accountID, t.Price); err != nil {
		// Including ErrCannotAfford, which the caller turns into a 409. The
		// insert above is undone by the transaction rolling back, so there is
		// no state in which the instrument exists unpaid for.
		return Panel{}, 0, err
	}

	panel, err := h.Of(ctx, tx, accountID)
	if err != nil {
		return Panel{}, 0, err
	}
	return panel, t.Price, nil
}

// instant is the clock a day is lived against.
//
// Advance stamps the effects it returns with whatever the clock says, and says
// so: "a caller replaying a missed day needs to be able to set [the instant] to
// that day rather than to now." This is that caller. A day being lived as it
// happens gets the real clock; a day being caught up gets its own midday, so a
// week of backfilled samples is dated across the week it describes rather than
// arriving as seven readings taken in the same second.
//
// The comparison is what keeps the two cases one function. Midday on the
// player's local today is still in the future for anybody east of UTC, and the
// game does not record a measurement that has not been taken yet.
func instant(clk simulation.Clock, on simulation.Date) simulation.Clock {
	now := clk.Now()
	if at := on.At(); at.Before(now) {
		return clock.Fixed{At: at}
	}
	return clk
}

/* ── the database ─────────────────────────────────────────────────────── */

func (h *Health) body(ctx context.Context, tx pgx.Tx, accountID string, lock bool) (Panel, error) {
	q := `SELECT sleep_debt, fitness, height_m, last_sleep, resting_hr, weight_kg,
	             hrv, recovery, load, glucose, crp, ldl, cortisol, last_day
	        FROM health_panel WHERE account_id = $1`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		b       simulation.Body
		lastDay time.Time
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(
		&b.SleepDebt, &b.Fitness, &b.HeightM, &b.LastSleep, &b.RestingHR, &b.WeightKg,
		&b.HRV, &b.Recovery, &b.Load, &b.Glucose, &b.CRP, &b.LDL, &b.Cortisol, &lastDay,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Every account has a panel; the one that does not is one this module
		// has not been asked about yet, and Ensure is how that is fixed.
		return Panel{}, pgx.ErrNoRows
	}
	if err != nil {
		return Panel{}, fmt.Errorf("health: read panel: %w", err)
	}

	return Panel{Body: b, LastDay: simulation.On(lastDay, time.UTC)}, nil
}

func (h *Health) write(ctx context.Context, tx pgx.Tx, accountID string, b simulation.Body, on simulation.Date) error {
	_, err := tx.Exec(ctx, `
		UPDATE health_panel SET
			sleep_debt = $2, fitness = $3, last_sleep = $4, resting_hr = $5, weight_kg = $6,
			hrv = $7, recovery = $8, load = $9,
			glucose = $10, crp = $11, ldl = $12, cortisol = $13,
			last_day = make_date($14, $15, $16), updated_at = now()
		WHERE account_id = $1`,
		accountID, b.SleepDebt, b.Fitness, b.LastSleep, b.RestingHR, b.WeightKg,
		b.HRV, b.Recovery, b.Load,
		b.Glucose, b.CRP, b.LDL, b.Cortisol,
		on.Year, int(on.Month), on.Day,
	)
	if err != nil {
		return fmt.Errorf("health: write panel: %w", err)
	}
	return nil
}

func (h *Health) sample(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date, s simulation.Sampled) error {
	b := s.Body
	_, err := tx.Exec(ctx, `
		INSERT INTO health_sample (
			account_id, on_day, taken_at,
			sleep_debt, fitness, last_sleep, resting_hr, weight_kg,
			hrv, recovery, load, glucose, crp, ldl, cortisol)
		VALUES ($1, make_date($2,$3,$4), $5, $6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (account_id, on_day) DO NOTHING`,
		accountID, on.Year, int(on.Month), on.Day, s.At,
		b.SleepDebt, b.Fitness, b.LastSleep, b.RestingHR, b.WeightKg,
		b.HRV, b.Recovery, b.Load, b.Glucose, b.CRP, b.LDL, b.Cortisol,
	)
	if err != nil {
		return fmt.Errorf("health: write sample: %w", err)
	}
	return nil
}

func (h *Health) history(ctx context.Context, tx pgx.Tx, accountID string) ([]Sample, error) {
	rows, err := tx.Query(ctx, `
		SELECT on_day, sleep_debt, fitness, last_sleep, resting_hr, weight_kg,
		       hrv, recovery, load, glucose, crp, ldl, cortisol
		  FROM health_sample
		 WHERE account_id = $1
		 ORDER BY on_day DESC
		 LIMIT $2`, accountID, historyDays)
	if err != nil {
		return nil, fmt.Errorf("health: read history: %w", err)
	}
	defer rows.Close()

	var out []Sample
	for rows.Next() {
		var (
			on time.Time
			b  simulation.Body
		)
		if err := rows.Scan(&on, &b.SleepDebt, &b.Fitness, &b.LastSleep, &b.RestingHR, &b.WeightKg,
			&b.HRV, &b.Recovery, &b.Load, &b.Glucose, &b.CRP, &b.LDL, &b.Cortisol); err != nil {
			return nil, fmt.Errorf("health: read history: %w", err)
		}
		out = append(out, Sample{On: simulation.On(on, time.UTC), Body: b})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("health: read history: %w", err)
	}

	// Newest-first out of the database because that is what LIMIT wants;
	// oldest-first out of here because that is what a trend is drawn in.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
