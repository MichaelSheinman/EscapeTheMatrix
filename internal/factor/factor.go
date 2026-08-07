/*
Package factor owns progression: the factors a player is made of, and the one
composite they are shown.

HighLevelOverview.md §3.5 is the specification and it starts by taking something
away — **there is no level.** Progression is a set of factors, most of them
hidden from the player, mixed into a composite called Karma. Health and Money are
revealed. Intelligence, emotional stability and kindness are real, tracked, and
never named. The formula is never stated, because the questions *what are the
hidden factors and what grows them* are the hook the game is built on.

# What this package is, and what internal/simulation is

The arithmetic is not here. Weighting, the money curve, the saturating credit
curve and the environment's decay all live in the pure core (§3.1), where they
can be tuned by a unit test that runs in milliseconds rather than by a
playthrough. What is here is state, its ownership, and one boundary.

# The boundary

**The hidden factors do not leave this package.** [Factors.Of] returns a
[Standing] whose slots carry a reading for the two revealed factors and *nothing
at all* for the other three — no name, no value, no key. It is the same move
internal/health makes for instrumentation: the veil is applied on the way out, at
the last possible moment, and there is no code path in which it is applied
earlier or skipped. Nothing upstream of this file behaves differently because a
factor is hidden; everything downstream is structurally unable to show it.

# Where credit comes from

From committed choices, and from nothing else (§7). An authored option taken in
a challenge (§5.1), a daily conflict answered (§4), an endorsement received
(§3.5) — each carries authored deltas, and [Factors.Record] is how they arrive.

There is deliberately **no command in this package.** A factor a player could
award themselves is not a measurement of anything, and a free-text field that
moved a hidden number would be the exact thing §7 rules out. The tickets that
bring the choices — #17, #24, #30 — call Record inside their own transaction.

It owns `factor_credit` and `factor_tenure` (§3.1), takes the caller's
transaction and never opens one (§3.2), and reads no other module's tables: the
body arrives through [Panel], the balance through [Purse], and where the player
is through [Environment].
*/
package factor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// ErrNotGranted is a grant naming a factor nobody awards.
//
// Health is read off the body and Money off the balance (§3.5); a caller trying
// to grant either has made a programming error, and it is reported here rather
// than quietly landing in a total that no longer means what it says.
var ErrNotGranted = errors.New("factor: that factor is not awarded")

/*
Panel is what this package is allowed to know about a player's body.

§3.1's rule about cross-module access, kept as narrow as it goes: the health
factor is the biological age and the biological age is a pure function of the
body, so one struct is the whole of what crosses. This package cannot read an
instrument, a sample or a ledger date even by accident.
*/
type Panel interface {
	Body(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Body, error)
}

/*
Purse is what this package is allowed to know about a player's money.

Deliberately read-only, and deliberately not character.Wallet: progression does
not spend. §3.5's money factor is the amount *held*, which is why the balance is
the whole of the question.
*/
type Purse interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
}

/*
Environment is which environment the player is in, for the decay in §3.15.

An interface because the thing that decides it is changing. §3.15 says an
environment is defined by its people rather than its geography, and #29 is what
brings populations; until then the district a player is standing in is the
closest honest answer the city can give, and it is already the thing that changes
when they relocate. Replacing that is a binding in internal/api rather than an
edit here.
*/
type Environment interface {
	Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error)
}

// Nowhere is an Environment for a world that has none: everywhere is the same
// place, so nothing ever wears out. Never bound in the server.
type Nowhere struct{}

func (Nowhere) Where(context.Context, pgx.Tx, string) (string, error) { return "nowhere", nil }

/*
Slot is one place on the board, as the player is allowed to see it.

The two revealed factors carry a name and a reading. The three hidden ones carry
neither — not a masked value, not a placeholder name, nothing but the fact that
something is there. §3.5: "Health and Money are shown; the rest appear as unnamed
or locked slots."

That is a stronger statement than it looks. A slot with a name and a hidden
number would tell the player *what* is being measured, which is half the puzzle;
a slot with a number and no name would let them work out the other half from
which way it moved. Both halves are withheld, and the slot exists at all so the
player knows there is something to find.
*/
type Slot struct {
	// Key and Name are empty for a hidden factor. The client keys these by
	// position, because position is all a locked slot has.
	Key  string
	Name string
	// Reads says how the value should be rendered — "age" or "money" — so the
	// client formats without knowing a rule. Empty for a hidden factor.
	Reads string
	// Value is nil for a hidden factor. Not zero: a zero is a reading, and
	// there is no reading.
	Value *float64
	// Note is the sentence beside a revealed slot, and the sentence beside a
	// locked one is deliberately not an explanation of what it measures.
	Note string
	// Shown is the fact the whole struct is about.
	Shown bool
}

/*
Standing is a player's progression, as far out as it is allowed to travel.

Karma is here because Karma is the thing the player understands as growth (§3.5).
The factors underneath it are here only as [Slot]s, which is to say the hidden
ones are not here at all.

Familiarity and Days describe the environment rather than the player, and they
are safe to show for the same reason §3.15 wants them felt: a plateau the
player cannot notice is a plateau they will grind against instead of travelling.
It is a statement about a *place*, and it names no factor.
*/
type Standing struct {
	// Karma, on the pure core's scale. Composed on read, stored nowhere.
	Karma float64
	// Slots is the board in §3.5's order: the revealed ones, then the rest.
	Slots []Slot

	// Where the player is, as the environment key the decay is kept against.
	Where string
	// Days lived there, and how much of an offered reward it still pays.
	Days        int
	Familiarity float64
}

/*
Recorded is what one set of grants did.

Offered and Paid are the honest pair, and they are the visible half of §3.15's
decay: the same committed choice in a worn-out environment is worth a fraction of
what it was worth on the first day there, and reporting only what was paid would
make an exhausted place look like a change in the player rather than in the
place.
*/
type Recorded struct {
	Standing Standing
	// Offered is the authored total before familiarity, Paid the total after.
	Offered float64
	Paid    float64
}

// Factors is the module. It holds no per-player state: the transaction comes
// from the caller and everything below is fixed at boot (§3.1, §3.2).
type Factors struct {
	panel Panel
	purse Purse
	where Environment
}

// New binds the module to the three things it reads and cannot reach otherwise.
func New(panel Panel, purse Purse, where Environment) *Factors {
	return &Factors{panel: panel, purse: purse, where: where}
}

/*
Of composes the player's standing.

Everything in the answer is derived. Nothing is written, nothing is cached, and
there is no number in the database this could disagree with — which is §3.3's
"everything derived is computed on read, never stored as a stale number" made
structural rather than promised.
*/
func (f *Factors) Of(ctx context.Context, tx pgx.Tx, accountID string) (Standing, error) {
	body, err := f.panel.Body(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}
	held, err := f.purse.Balance(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}
	credit, err := f.credit(ctx, tx, accountID)
	if err != nil {
		return Standing{}, err
	}
	where, tenure, err := f.tenure(ctx, tx, accountID, false)
	if err != nil {
		return Standing{}, err
	}

	// Ceilings are §3.15's, and #29 is what authors them. An unauthored ceiling
	// is no ceiling (the pure core's rule), so the zero value is correct here
	// rather than merely harmless.
	reading := simulation.Karma(simulation.Standing{
		Body:   body,
		Purse:  held,
		Credit: credit,
	})

	return Standing{
		Karma:       reading.Karma,
		Slots:       slots(body, held),
		Where:       where,
		Days:        tenure.days,
		Familiarity: simulation.Familiarity(tenure.days, tenure.worked),
	}, nil
}

/*
Record applies authored deltas from a committed choice.

The environment's familiarity is applied here rather than by the caller, and that
is the point of the method existing: §3.15's decay is a property of *where the
player is*, and a caller that scaled its own rewards would be a caller that could
forget to. Costs are not scaled — see simulation.Pays.

What the environment absorbs is the *offered* total, before decay, because that
is the closest honest proxy the mechanics have for how thoroughly the place has
been worked (§3.15's note that decay must track mastery rather than elapsed
time).

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
Day ages the environment the player is standing in by one of their days.

This is the Decay half of §3.3's rollover work, and it is called once per missed
day rather than once per run — a player caught up from a week away has a week of
it to lose, and a curve against elapsed days that was applied once would give
them a night's worth.

Strictly forward against the environment's own last day, in the same way the
body's ledger is: a retried job, a second tab hydrating, and the scheduled
rollover arriving mid-catch-up all reach the same state, and none of them can age
a place twice.
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
	return nil
}

/*
slots is the veil, and it is the last thing that happens on the way out.

One slot per factor, in §3.5's order, whether or not the player may see it — the
locked slots are the shape of the question the game is asking. A revealed factor
carries the reading the design says it surfaces as: health as a biological age
(§3.5: "your biological age is 24" carries meaning that "health: 72" does not),
money as the amount held.

Note what a revealed slot does *not* carry either: its 0–100 score, and its
weight. The composition is right there in [Factors.Of] and none of it comes this
way — telling the player that their health factor is 71 out of 100 would state
half the formula, and the formula is never stated.
*/
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
			slot.Note = "what you are holding, not what you have earned"
		}
		out = append(out, slot)
	}
	return out
}

// name is what a revealed factor is called on screen.
//
// Only the revealed two have one. §3.5's other three have names in the design
// document and in the pure core's keys, and neither of those is a place a player
// can read.
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
		// A key the vocabulary no longer has is ignored rather than fatal, for
		// the same reason a retired routine option falls back to a default: the
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

func (f *Factors) add(ctx context.Context, tx pgx.Tx, accountID string, which simulation.Factor, credit float64) error {
	// greatest(...) is the floor the pure core states — a factor driven at is
	// not a hole — kept here as well so it holds against the stored value
	// rather than only against the one that happened to be read.
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
// player is now.
//
// The row is created on read because a player who has never been counted
// anywhere is in an environment worth all of what it offers, which is the right
// answer rather than a special case, and because Record and Day both need
// somewhere to write a moment later.
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
