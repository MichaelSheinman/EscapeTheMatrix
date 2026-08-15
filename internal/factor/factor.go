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

# The leaderboards

§3.5's other half is in board.go: one overall board for the composite, plus a
separate board for each individual factor — **including the hidden ones**, which
is only safe because a ranking is a position rather than a number. That is the
reason the boards are in this package rather than in one of their own: sorting
players by a hidden factor means reading it, so the sort happens inside the veil
and a *rank* is what comes out.

It owns `factor_credit`, `factor_tenure` and the two board tables (§3.1), takes
the caller's transaction and never opens one (§3.2), and reads no other module's
tables: the body arrives through [Panel], the balance through [Purse], where the
player is through [Environment], and who else is playing through [Roster].
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
Environment is which population the player is among, for §3.15's decay and
ceilings.

It answers with a key from internal/environment, and it is an interface because
*which* population that is is a fact about a character rather than about
progression: it follows from where they live, which is internal/character's, and
composing the two is internal/api's job (§3.1). What the key then affords is
authored content this package looks up directly, because a ceiling is not a
per-player question and an interface that carried one would let a caller quietly
lift it.

[environment.Nowhere] — or any key no population answers to — is no ceiling and a
place that never wears out.
*/
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
	// Among is what that population is called, as a player reads it. Empty
	// where there is no population — [environment.Nowhere], a key nobody
	// authors — which is a real state rather than a missing name.
	Among string
	// Days lived there, and how much of an offered reward it still pays.
	Days        int
	Familiarity float64

	// Curve is every day this player has behind them, oldest first, and what
	// the composite read at the end of each. Empty on their first day.
	//
	// It is the only quantity in this struct that is *not* composed on read,
	// and it is allowed to be for the reason [Point] gives: a finished day
	// cannot be recomposed, because the body and the balance it was a reading
	// of have both moved on since. Today is deliberately not in it — the live
	// Karma above is today.
	Curve []Point
}

/*
Point is one day, and what Karma read at the end of it.

A composite and nothing else. §3.5's formula is never stated, and a curve made
of composites states no part of it: a rise says something happened, not which
of five things moved, and the player working out *which* is the whole hook.
*/
type Point struct {
	Day   simulation.Date
	Karma float64
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
	panel  Panel
	purse  Purse
	where  Environment
	roster Roster
}

// New binds the module to the four things it reads and cannot reach otherwise.
//
// The fourth is the population, and only the leaderboards (board.go) use it: a
// composite is a reading of one player, a rank is a statement about all of them.
func New(panel Panel, purse Purse, where Environment, roster Roster) *Factors {
	return &Factors{panel: panel, purse: purse, where: where, roster: roster}
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

technical.md §4 decides release from, among other things, "what would serve the
player next", and this is the only reading of that sentence this codebase is
willing to compute: the factors a player is furthest behind *themselves* on.
Against their own average rather than against a target, because §3.5's factors
have no target — a player at eighty on everything is not lagging on anything,
and one at forty on everything is not lagging on all five.

**No number leaves.** The answer is a list of [simulation.Factor]s, in the order
they are behind, and nothing that could be turned back into a score. That is the
same boundary [Factors.Of] keeps with [Slot], and it is why this method is here
rather than in the module that wanted it: §3.2's "nothing is labelled" is only
true if the hidden three stay unquantified everywhere outside this package, and
a caller handed five floats would eventually render four of them.

Ties break on [simulation.Factors]' own order, so the answer is stable and the
same twice — §7's determinism, applied to the thing that decides what the world
puts in front of somebody.
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
	// order. The same shape, and for the same reason, as internal/event's
	// stableBy: a sort that broke ties by chance would make "what would serve
	// this player" a different answer every time it was asked.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && reading.Of(out[j]) < reading.Of(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
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
Ready reports whether this player's factors reach a requirement.

§3.14's "each step up requires capability", answered where it is the only place
it can be answered: inside the veil. A [simulation.Requirement] carries a floor
per factor including the three the player is never shown (§3.5), so the
comparison has to happen here — and what comes back is a yes or a no and nothing
that could be turned back into a score.

It is the same one-way shape [Factors.Lagging] keeps and for the same reason. A
caller handed the shortfall would eventually render it, and a rendered shortfall
on a hidden factor is the factor named, valued, and the puzzle over.

The reading is taken under §3.15's ceilings, exactly as [Factors.Of]'s is, and
that is a design decision rather than a shared helper: a rung is hired against
what the player can *currently demonstrate*, which is what a ceiling caps. A
population that has run out of ways to show intelligence has run out of ways to
be promoted for it, and the answer is to go somewhere else.
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
floor. The three hidden ones carry **neither** — not a name, not a value, and
not whether they are met — which is a stronger statement than it looks and the
same one [Slot] makes. A locked entry that said "met" would say what is being
measured the moment it flipped, and one that said "not met" would say it by
staying put. What the player is told is that something else is being asked, and
that they are or are not being taken on; working out which of the two is the
whole of §3.5's hook.

Nothing here decides whether they may have the job. That is [Factors.Ready],
which reads every floor including the ones this method refuses to describe.
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

/*
Want is one thing a rung asks for, veiled.

Zero-valued for a hidden factor: no key, no name, not shown, and not met. Read
that last field carefully — it is `false` because there is nothing to say, not
because the floor was missed, and no caller may treat it as a reading.
*/
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

/*
compose is the one place a Karma is composed, and therefore the one place
§3.15's ceilings are applied.

There is exactly one of these on purpose. A reading taken without the ceilings
would be a *different* reading of the same player — higher, and higher in
precisely the way the design says a mastered environment must stop being — and
the moment two code paths compose a standing, one of them eventually forgets.
So [Factors.Of], [Factors.reading] and the leaderboards all arrive here, and the
only thing a caller supplies is the two things it has already read.
*/
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

Three of the four come from the population, which is authored content and not a
per-player fact — the economic ceiling, and the two caps that follow from how
much situational variety the place has. An environment key nobody authors is no
ceiling at all, which is the pure core's convention and is what makes a player
standing outside every population a player whose reading of themselves still
works.

The fourth is intelligence, and it is the one §3.15 puts on the player rather
than on the place: adaptive capacity plateaus once a rule set works and rises by
building another one somewhere else. [Factors.rebuilt] counts how many the player
has, [simulation.Reach] turns that into the cap, and the consequence is the whole
point — grinding one population raises the credit and moves the reading not at
all, and moving raises the ceiling before a single new choice has been made.
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

/*
rebuilt is how many populations this player has built a rule set for.

Counted from the offered credit each environment has absorbed rather than from
whether a playbook has sentences in it, and the difference matters: composing
rules for somewhere you have never been is free (§3.15 hands every player a set
for every environment from the first day), and a ceiling that rose from typing
would be a hidden factor scored from something other than a committed choice,
which is exactly what §7 forbids.

[simulation.RuleSet] is the threshold, and the count is over every environment
including the one the player is standing in — where they are now is a place they
may well have worked out.
*/
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
in, and it writes down what the composite read as that day ended.

This is the Decay half of §3.3's rollover work, and it is called once per missed
day rather than once per run — a player caught up from a week away has a week of
it to lose, and a curve against elapsed days that was applied once would give
them a night's worth.

Strictly forward against the environment's own last day, in the same way the
body's ledger is: a retried job, a second tab hydrating, and the scheduled
rollover arriving mid-catch-up all reach the same state, and none of them can age
a place twice — or record the same day twice with a different number.

The reading is taken *here* rather than reconstructed later because it cannot be
reconstructed later. The rollover walks a week of days oldest first and the body
moves under each of them; by the time anybody asks, the body that Tuesday's Karma
was a reading of no longer exists. This is the one moment Tuesday is composable.
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

	// After the ageing, and deliberately: the decay this day applied is part of
	// what the day came to, and a reading taken before it would be a reading of
	// a day that had not finished happening.
	return f.Mark(ctx, tx, accountID, on)
}

/*
Mark records what this player's Karma reads, against a day of theirs.

Two callers, and they are the two halves of the same rule — *whenever the game
states a player's standing, the day it stated it is written down*. The rollover
calls it for each day it closes (see [Factors.Day]); internal/api calls it for
today, every time it composes a standing to send. Without the second, the day a
player joined would never appear on their own curve: their first day ends before
any rollover has a day to walk, so nothing else would ever compose it.

It overwrites rather than declining, because today's reading moves all day and
the last one taken is the one that stood. A finished day is only ever marked
once — [Factors.Day]'s ledger guard sees to that — so the overwrite cannot reach
backwards and rewrite a day that is over.
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
			slot.Note = "what you are holding"
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

// curve reads every day this player has behind them, oldest first.
//
// Unpaged and unwindowed. A day is one row and a year is three hundred and
// sixty-five of them, and the whole point of the curve is that it reaches back
// to the day somebody joined — a window would quietly turn "how far you have
// come" into "the last fortnight", which is the question the health panel's
// trend already answers.
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
		// A DATE comes back as midnight UTC, which is the calendar day it is —
		// the same read internal/factor's tenure does of its own last_day.
		out = append(out, Point{Day: simulation.On(day, time.UTC), Karma: karma})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("factor: read karma history: %w", err)
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
