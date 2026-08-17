/*
Package factor owns progression: the factors a player is made of, and the one
composite they are shown.

There is no level (§3.5). Progression is a set of factors, most hidden, mixed
into a composite called Karma. Health and Money are revealed; intelligence,
emotional stability and kindness are real, tracked and never named. The formula is
never stated, because working out what the hidden factors are and what grows them
is what the player is playing for.

The arithmetic is not here. Weighting, the money curve, the saturating credit
curve and the environment's decay live in the pure core (§3.1). What is here is
state, its ownership, and one boundary.

That boundary: the hidden factors do not leave this package. [Factors.Of] returns
a [Standing] whose slots carry a reading for the two revealed factors and nothing
at all for the other three — no name, no value, no key. The hiding happens on the
way out, at the last possible moment, and there is no code path that skips it.

Credit comes from committed choices and nothing else (§7) — an authored option
taken in a challenge, a conflict answered, an endorsement received — each
carrying authored deltas that arrive through [Factors.Record]. There is
deliberately no command in this package: a factor a player could award themselves
is not a measurement of anything.

The leaderboards are in board.go: one board for the composite, plus one per
factor including the hidden ones, which is safe because a ranking is a position
rather than a number. They live here because sorting by a hidden factor means
reading it, so the sort happens in here.

It owns factor_credit, factor_tenure and the two board tables (§3.1), takes the
caller's transaction and never opens one (§3.2), and reads no other module's
tables: the body through [Panel], the balance through [Purse], where the player
is through [Environment], and who else is playing through [Roster].
*/
package factor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// ErrNotGranted is a grant naming a factor nobody awards. Health is read off
// the body and Money off the balance (§3.5), so granting either is a
// programming error rather than a total that quietly stops meaning what it says.
var ErrNotGranted = errors.New("factor: that factor is not awarded")

// Panel is what this package is allowed to know about a player's body: the
// health factor is biological age, which is a pure function of the body, so one
// struct is the whole of what crosses (§3.1). No instrument, sample or ledger
// date is reachable from here.
type Panel interface {
	Body(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Body, error)
}

// Purse is what this package is allowed to know about a player's money.
// Read-only, and not character.Wallet: progression does not spend, and §3.5's
// money factor is the amount held.
type Purse interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
}

// Environment is which population the player is among, for §3.15's decay and
// ceilings. An interface because which population that is follows from where
// they live, which is internal/character's (§3.1). What the key affords is
// authored content this package looks up directly, since a ceiling is not a
// per-player question. [environment.Nowhere], or any unknown key, is no ceiling
// and a place that never wears out.
type Environment interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

// Nowhere is an Environment for a world with no populations in it: everywhere is
// the same place, so nothing ever wears out and nothing is capped. Never bound
// in the server.
type Nowhere struct{}

func (Nowhere) Where(context.Context, pgx.Tx, string) (string, error) {
	return environment.Nowhere, nil
}

/*
Slot is one place on the board, as the player is allowed to see it. The two
revealed factors carry a name and a reading. A hidden one carries just the fact
that something is being measured there (§3.5).

Both halves are withheld together: a name with a hidden number would say what is
being measured, and a number with no name would let the player work that out from
which way it moved. The slot is there so they know there is something to find.
*/
type Slot struct {
	// Key and Name are empty for a hidden factor, so the client keys these by
	// position.
	Key  string
	Name string
	// Reads says how the value should be rendered — "age" or "money" — so the
	// client formats it without knowing a rule. Empty for a hidden factor.
	Reads string
	// Value is nil for a hidden factor, since a zero would be a reading.
	Value *float64
	// Note is the sentence beside the slot. For a hidden factor it says that
	// something is measured here and stops there.
	Note string
	// Shown is the fact the whole struct is about.
	Shown bool
}

// Standing is a player's progression, as far out as it is allowed to travel.
// Karma is what the player understands as growth (§3.5), and the factors
// underneath it appear only as [Slot]s.
//
// Familiarity and Days describe the environment rather than the player and are
// safe to show: a plateau the player cannot notice is one they grind against
// instead of travelling (§3.15). Both are statements about a place and name no
// factor.
type Standing struct {
	// Karma, on the pure core's scale. Composed on read, stored nowhere.
	Karma float64
	// Slots is the board in §3.5's order: the revealed ones, then the rest.
	Slots []Slot

	// Where the player is, as the environment key the decay is kept against.
	Where string
	// Among is what that population is called, as a player reads it. Empty
	// where there is no population — [environment.Nowhere], a key nobody
	// authors — which is a real state rather than a missing name.
	Among string
	// Days lived there, and how much of an offered reward it still pays.
	Days        int
	Familiarity float64

	// Curve is every day this player has behind them, oldest first, and what the
	// composite read at the end of each. Empty on their first day.
	//
	// The only quantity here not composed on read: a finished day cannot be
	// recomposed, because the body and balance it was a reading of have moved
	// on. Today is not in it — the live Karma above is today.
	Curve []Point
}

// Point is one day, and what Karma read at the end of it. A composite and
// nothing else: a rise says something happened, not which of five things moved,
// and working out which is what the player is playing for (§3.5).
type Point struct {
	Day   simulation.Date
	Karma float64
}

// Recorded is what one set of grants did. Offered and Paid are the visible half
// of §3.15's decay: the same choice in a worn-out environment is worth a
// fraction of what it was on the first day there, and reporting only what was
// paid would make an exhausted place look like a change in the player.
type Recorded struct {
	Standing Standing
	// Offered is the authored total before familiarity, Paid the total after.
	Offered float64
	Paid    float64
}

// Factors is the module. It holds no per-player state: the transaction comes
// from the caller and everything below is fixed at boot (§3.1, §3.2).
type Factors struct {
	panel  Panel
	purse  Purse
	where  Environment
	roster Roster
}

// New binds the module to the four things it reads and cannot reach otherwise.
// Only the leaderboards use the roster: a composite is a reading of one player,
// a rank is a statement about all of them.
func New(panel Panel, purse Purse, where Environment, roster Roster) *Factors {
	return &Factors{panel: panel, purse: purse, where: where, roster: roster}
}

// Of composes the player's standing. Everything in the answer is derived:
// nothing is written, nothing is cached, and there is no stored number this
// could disagree with (§3.3).
func (f *Factors) Of(ctx context.Context, tx pgx.Tx, accountID string) (Standing, error) {
	body, err := f.panel.Body(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}
	held, err := f.purse.Balance(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}
	reading, where, tenure, err := f.compose(ctx, tx, accountID, body, held)
	if err != nil {
		return Standing{}, err
	}

	curve, err := f.curve(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}

	out := Standing{
		Karma:       reading.Karma,
		Slots:       slots(body, held),
		Where:       where,
		Days:        tenure.days,
		Familiarity: simulation.Familiarity(tenure.days, tenure.worked),
		Curve:       curve,
	}
	if among, ok := environment.Lookup(where); ok {
		out.Among = among.Name
	}
	return out, nil
}

/*
Lagging is which of the player's factors sit below their own average, worst
first.

The only reading of §4's "what would serve the player next" this codebase is
willing to compute. Against their own average rather than a target, because
§3.5's factors have no target: a player at eighty on everything is not lagging on
anything.

No number leaves. The answer is a list of [simulation.Factor]s in the order they
are behind and nothing that could be turned back into a score, which is why the
method is here rather than in the module that wanted it. Ties break on
[simulation.Factors]' own order, so the answer is the same twice (§7).
*/
func (f *Factors) Lagging(ctx context.Context, tx pgx.Tx, accountID string) ([]simulation.Factor, error) {
	reading, err := f.reading(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}

	var total float64
	for _, which := range simulation.Factors {
		total += reading.Of(which)
	}
	mean := total / float64(len(simulation.Factors))

	out := []simulation.Factor{}
	for _, which := range simulation.Factors {
		if reading.Of(which) < mean {
			out = append(out, which)
		}
	}
	// Insertion sort on one key, keeping equal readings in the pure core's
	// order: a sort that broke ties by chance would make "what would serve this
	// player" a different answer every time it was asked.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && reading.Of(out[j]) < reading.Of(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

/*
Record applies authored deltas from a committed choice.

The environment's familiarity is applied here rather than by the caller, which is
the point of the method existing: §3.15's decay is a property of where the player
is, and a caller that scaled its own rewards could forget to. Costs are not
scaled — see [simulation.Pays].

What the environment absorbs is the offered total, before decay, because that is
the closest proxy for how thoroughly the place has been worked (§3.15's decay
tracks mastery rather than elapsed time).

Everything happens in the caller's transaction, so a choice whose command rolled
back never moved a factor.
*/
func (f *Factors) Record(ctx context.Context, tx pgx.Tx, accountID string, grants ...simulation.Grant) (Recorded, error) {
	for _, g := range grants {
		if !g.Valid() {
			return Recorded{}, fmt.Errorf("%w: %s", ErrNotGranted, g.Factor)
		}
	}

	where, tenure, err := f.tenure(ctx, tx, accountID, true)
	if err != nil {
		return Recorded{}, err
	}
	familiarity := simulation.Familiarity(tenure.days, tenure.worked)

	out := Recorded{}
	for _, g := range grants {
		paid := simulation.Pays(g.Credit, familiarity)
		out.Offered += g.Credit
		out.Paid += paid

		if err := f.add(ctx, tx, accountID, g.Factor, paid); err != nil {
			return Recorded{}, err
		}
	}

	// Only what was offered as a reward wears the place out. A cost is not
	// effort spent mastering anywhere.
	worked := 0.0
	for _, g := range grants {
		if g.Credit > 0 {
			worked += g.Credit
		}
	}
	if worked > 0 {
		if err := f.worked(ctx, tx, accountID, where, worked); err != nil {
			return Recorded{}, err
		}
	}

	out.Standing, err = f.Of(ctx, tx, accountID)
	if err != nil {
		return Recorded{}, err
	}
	return out, nil
}

/*
Ready reports whether this player's factors reach a requirement — §3.14's
capability check. It is answered here because a [simulation.Requirement] carries
a floor per factor, including the three the player is never shown (§3.5).
A yes or a no comes back, and nothing that could be turned back into a score.

The reading is taken under §3.15's ceilings, as [Factors.Of]'s is: a job is hired
against what the player can currently demonstrate, and a population that has run
out of ways to show intelligence has run out of ways to be hired for it.
*/
func (f *Factors) Ready(
	ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement,
) (bool, error) {
	reading, err := f.reading(ctx, tx, accountID)
	if err != nil {
		return false, err
	}
	return need.Met(reading), nil
}

/*
Wanting is a requirement as the player is allowed to read it: one entry per
factor it asks about, in the pure core's order.

The two revealed factors carry their name and whether this player clears the
floor. The three hidden ones carry none of that — not a name, not a value, not
whether they are met. An entry that said "met" would name what is being measured
the moment it flipped, and one that said "not met" would name it by staying put.

Nothing here decides whether they may have the job. That is [Factors.Ready],
which reads every floor including the ones this refuses to describe.
*/
func (f *Factors) Wanting(
	ctx context.Context, tx pgx.Tx, accountID string, need simulation.Requirement,
) ([]Want, error) {
	asked := need.Wants()
	if len(asked) == 0 {
		return nil, nil
	}
	reading, err := f.reading(ctx, tx, accountID)
	if err != nil {
		return nil, err
	}

	out := make([]Want, 0, len(asked))
	for _, which := range asked {
		if !which.Shown() {
			out = append(out, Want{})
			continue
		}
		out = append(out, Want{
			Key:   which.Key(),
			Name:  name(which),
			Shown: true,
			Met:   need.Reaches(reading, which),
		})
	}
	return out, nil
}

// Want is one thing a job asks for, as the player may read it. Zero-valued for a
// hidden factor: no
// key, no name, not shown, and not met — Met is false because there is nothing
// to say rather than because the floor was missed, and no caller may treat it as
// a reading.
type Want struct {
	// Key and Name are empty for a hidden factor. A client keys these by
	// position, because position is all a blanked one has.
	Key  string
	Name string
	// Shown is the fact the whole struct is about.
	Shown bool
	// Met is whether this player clears the floor, and is only meaningful when
	// Shown. Always false otherwise.
	Met bool
}

// reading composes this player's factor scores. The private half of
// [Factors.Lagging], [Factors.Ready] and [Factors.Wanting], all three of which
// need the full Reading and none of which may let one out.
func (f *Factors) reading(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Reading, error) {
	body, err := f.panel.Body(ctx, tx, accountID)
	if err != nil {
		return simulation.Reading{}, err
	}
	held, err := f.purse.Balance(ctx, tx, accountID)
	if err != nil {
		return simulation.Reading{}, err
	}
	out, _, _, err := f.compose(ctx, tx, accountID, body, held)
	return out, err
}

// compose is the one place a Karma is composed, and therefore the one place
// §3.15's ceilings are applied. A reading taken without them would be a higher
// reading of the same player, and two code paths composing a standing means one
// of them eventually forgets. [Factors.Of], [Factors.reading] and the
// leaderboards all arrive here.
func (f *Factors) compose(
	ctx context.Context, tx pgx.Tx, accountID string, body simulation.Body, held simulation.Money,
) (simulation.Reading, string, tenure, error) {
	credit, err := f.credit(ctx, tx, accountID)
	if err != nil {
		return simulation.Reading{}, "", tenure{}, err
	}
	where, worn, err := f.tenure(ctx, tx, accountID, false)
	if err != nil {
		return simulation.Reading{}, "", tenure{}, err
	}
	ceilings, err := f.ceilings(ctx, tx, accountID, where)
	if err != nil {
		return simulation.Reading{}, "", tenure{}, err
	}
	return simulation.Karma(simulation.Standing{
		Body:     body,
		Purse:    held,
		Credit:   credit,
		Ceilings: ceilings,
	}), where, worn, nil
}

/*
ceilings is how high this player's factors may read where they are (§3.15).

Three of the four come from the population, which is authored content rather than
a per-player fact. An environment key nobody authors is no ceiling at all, so a
player standing outside every population still reads.

The fourth is intelligence, which §3.15 puts on the player rather than the place:
adaptive capacity plateaus once a rule set works and rises by building another
one somewhere else. [Factors.rebuilt] counts how many they have and
[simulation.Reach] turns that into the cap, so grinding one population raises the
credit and moves the reading not at all.
*/
func (f *Factors) ceilings(ctx context.Context, tx pgx.Tx, accountID, where string) (simulation.Ceilings, error) {
	out := environment.Ceilings(where)
	sets, err := f.rebuilt(ctx, tx, accountID)
	if err != nil {
		return simulation.Ceilings{}, err
	}
	out.Intelligence = simulation.Reach(sets)
	return out, nil
}

// rebuilt is how many populations this player has built a rule set for, counted
// from the offered credit each environment has absorbed rather than from whether
// a playbook has sentences in it. Composing rules for somewhere you have never
// been is free, and a ceiling that rose from typing would be a hidden factor
// scored from something other than a committed choice (§7).
// [simulation.RuleSet] is the threshold, over every environment including the
// one the player is standing in.
func (f *Factors) rebuilt(ctx context.Context, tx pgx.Tx, accountID string) (int, error) {
	var sets int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM factor_tenure
		 WHERE account_id = $1 AND worked >= $2`, accountID, simulation.RuleSet).Scan(&sets); err != nil {
		return 0, fmt.Errorf("factor: count rule sets: %w", err)
	}
	return sets, nil
}

/*
Day closes one of the player's days: it ages the environment they are standing
in, and writes down what the composite read as that day ended.

The Decay half of §3.3's rollover work, called once per missed day rather than
once per run, since a player caught up from a week away has a week of it to lose.

Strictly forward against the environment's own last day, like the body's ledger,
so a retried job and a second tab hydrating reach the same state and neither can
age a place twice.

The reading is taken here because it cannot be reconstructed later: the rollover
walks days oldest first and the body moves under each of them, so by the time
anybody asks, the body Tuesday's Karma was a reading of no longer exists.
*/
func (f *Factors) Day(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	where, tenure, err := f.tenure(ctx, tx, accountID, true)
	if err != nil {
		return err
	}
	if tenure.lastDay != nil && !tenure.lastDay.Before(on) {
		return nil
	}

	_, err = tx.Exec(ctx, `
		UPDATE factor_tenure
		   SET days = days + 1, last_day = make_date($3,$4,$5), updated_at = now()
		 WHERE account_id = $1 AND environment = $2`,
		accountID, where, on.Year, int(on.Month), on.Day)
	if err != nil {
		return fmt.Errorf("factor: age environment: %w", err)
	}

	// After the ageing: the decay this day applied is part of what the day came
	// to, and a reading taken before it would be of an unfinished day.
	return f.Mark(ctx, tx, accountID, on)
}

/*
Mark records what this player's Karma reads, against a day of theirs.

Two callers, and one rule: whenever the game states a player's standing, the day
it stated it is written down. The rollover calls it for each day it closes (see
[Factors.Day]), and internal/api calls it for today whenever it composes a
standing to send — without the second, the day a player joined would never appear
on their own curve.

It overwrites rather than declining, because today's reading moves all day and
the last one taken is the one that stood. [Factors.Day]'s ledger guard means a
finished day is only ever marked once, so the overwrite cannot reach backwards.
*/
func (f *Factors) Mark(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	reading, err := f.reading(ctx, tx, accountID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO factor_history (account_id, day, karma)
		VALUES ($1, make_date($2,$3,$4), $5)
		ON CONFLICT (account_id, day) DO UPDATE SET karma = EXCLUDED.karma`,
		accountID, on.Year, int(on.Month), on.Day, reading.Karma)
	if err != nil {
		return fmt.Errorf("factor: record karma: %w", err)
	}
	return nil
}

// slots is where the hidden factors are stripped, and the last thing that
// happens on the way out. One slot per factor in §3.5's order, whether or not the player may see it, so the
// locked slots are the shape of the question the game is asking.
//
// A revealed slot carries the reading the design surfaces it as — health as a
// biological age, money as the amount held. The 0–100 score behind it and its
// weight in the composite stay here: "your health factor is 71 out of 100" would
// state half the formula, and the formula is never stated.
func slots(body simulation.Body, held simulation.Money) []Slot {
	out := make([]Slot, 0, len(simulation.Factors))
	for _, f := range simulation.Factors {
		if !f.Shown() {
			// Nothing. Not a masked number, not a name — a slot, and the fact
			// that it is occupied.
			out = append(out, Slot{
				Note: "something is being measured here",
			})
			continue
		}

		slot := Slot{Key: f.Key(), Name: name(f), Shown: true}
		switch f {
		case simulation.Health:
			// Composed on read from the whole panel, including the metrics this
			// player has no instrument for (§3.16). The one health number
			// visible at every tier.
			v := simulation.BiologicalAge(body)
			slot.Value, slot.Reads = &v, "age"
			slot.Note = "composed from every biomarker, including the ones you cannot read"
		case simulation.Wealth:
			v := float64(held.Cents())
			slot.Value, slot.Reads = &v, "money"
			slot.Note = "what you are holding"
		}
		out = append(out, slot)
	}
	return out
}

// name is what a revealed factor is called on screen. Only the revealed two
// have one: §3.5's other three are named in the design document and in the pure
// core's keys, neither of which a player can read.
func name(f simulation.Factor) string {
	switch f {
	case simulation.Health:
		return "Biological age"
	case simulation.Wealth:
		return "Money"
	default:
		return ""
	}
}

/* ── the database ─────────────────────────────────────────────────────── */

func (f *Factors) credit(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Credit, error) {
	rows, err := tx.Query(ctx,
		`SELECT factor, credit FROM factor_credit WHERE account_id = $1`, accountID)
	if err != nil {
		return simulation.Credit{}, fmt.Errorf("factor: read credit: %w", err)
	}
	defer rows.Close()

	var out simulation.Credit
	for rows.Next() {
		var (
			key    string
			credit float64
		)
		if err := rows.Scan(&key, &credit); err != nil {
			return simulation.Credit{}, fmt.Errorf("factor: read credit: %w", err)
		}
		// A key the vocabulary no longer has is ignored rather than fatal: the
		// content moves, and a player holding the old version has to be able to
		// walk out of it.
		for _, known := range simulation.Factors {
			if known.Key() == key && known.Granted() {
				out = out.Plus(known, credit)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return simulation.Credit{}, fmt.Errorf("factor: read credit: %w", err)
	}
	return out, nil
}

// curve reads every day this player has behind them, oldest first. Unpaged and
// unwindowed: a day is one row, and the point of the curve is that it reaches
// back to the day somebody joined. "The last fortnight" is the question the
// health panel's trend already answers.
func (f *Factors) curve(ctx context.Context, tx pgx.Tx, accountID string) ([]Point, error) {
	rows, err := tx.Query(ctx,
		`SELECT day, karma FROM factor_history WHERE account_id = $1 ORDER BY day`, accountID)
	if err != nil {
		return nil, fmt.Errorf("factor: read karma history: %w", err)
	}
	defer rows.Close()

	out := []Point{}
	for rows.Next() {
		var (
			day   time.Time
			karma float64
		)
		if err := rows.Scan(&day, &karma); err != nil {
			return nil, fmt.Errorf("factor: read karma history: %w", err)
		}
		// A DATE comes back as midnight UTC, which is the calendar day it is.
		out = append(out, Point{Day: simulation.On(day, time.UTC), Karma: karma})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("factor: read karma history: %w", err)
	}
	return out, nil
}

func (f *Factors) add(ctx context.Context, tx pgx.Tx, accountID string, which simulation.Factor, credit float64) error {
	// greatest(...) is the pure core's floor — a factor driven at is not a hole
	// — kept here too so it holds against the stored value.
	_, err := tx.Exec(ctx, `
		INSERT INTO factor_credit (account_id, factor, credit)
		VALUES ($1, $2, greatest($3, 0))
		ON CONFLICT (account_id, factor) DO UPDATE
		   SET credit = greatest(factor_credit.credit + $3, 0), updated_at = now()`,
		accountID, which.Key(), credit)
	if err != nil {
		return fmt.Errorf("factor: record credit: %w", err)
	}
	return nil
}

// tenure is the wear on the environment the player is standing in.
type tenure struct {
	days    int
	worked  float64
	lastDay *simulation.Date
}

// tenure reads — and, when asked, creates and locks — the row for wherever the
// player is now. Created on read because a player never counted anywhere is in
// an environment worth all of what it offers, and because Record and Day both
// need somewhere to write a moment later.
func (f *Factors) tenure(ctx context.Context, tx pgx.Tx, accountID string, lock bool) (string, tenure, error) {
	where, err := f.where.Where(ctx, tx, accountID)
	if err != nil {
		return "", tenure{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO factor_tenure (account_id, environment) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, accountID, where); err != nil {
		return "", tenure{}, fmt.Errorf("factor: ensure environment: %w", err)
	}

	q := `SELECT days, worked, last_day FROM factor_tenure
	       WHERE account_id = $1 AND environment = $2`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		out  tenure
		last *time.Time
	)
	if err := tx.QueryRow(ctx, q, accountID, where).Scan(&out.days, &out.worked, &last); err != nil {
		return "", tenure{}, fmt.Errorf("factor: read environment: %w", err)
	}
	if last != nil {
		on := simulation.On(*last, time.UTC)
		out.lastDay = &on
	}
	return where, out, nil
}

func (f *Factors) worked(ctx context.Context, tx pgx.Tx, accountID, where string, offered float64) error {
	_, err := tx.Exec(ctx, `
		UPDATE factor_tenure SET worked = worked + $3, updated_at = now()
		 WHERE account_id = $1 AND environment = $2`, accountID, where, offered)
	if err != nil {
		return fmt.Errorf("factor: wear environment: %w", err)
	}
	return nil
}
