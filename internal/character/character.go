// Package character owns the player: their money, where they are standing, and
// where they live.
//
// technical.md §3.1: package by feature, and **a module owns its own tables and
// is the only thing that touches them.** This package owns `character`. Another
// module that needs to know whether a player can afford something asks through
// the Wallet interface below; nothing anywhere selects from this table but the
// file you are reading. That rule is why this is the first module: it is easier
// to keep than to introduce later, and the moment two packages both write one
// table there is no longer a place to put the rule that governs it.
//
// Every method takes the caller's transaction and never opens one. §3.2 puts a
// player command inside exactly one Postgres transaction, and a module that
// reaches into the pool on its own has escaped it — a money change that
// committed while the command that caused it rolled back is the exact bug the
// single-transaction rule exists to make unreachable.
//
// The rules live in internal/simulation and the geography lives in
// internal/city. What is here is state, its shape, and the ownership of it.
package character

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. They are Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoCharacter means the account has no character row. It is a bug
	// rather than a state — every account gets one — so it is reported rather
	// than papered over.
	ErrNoCharacter = errors.New("character: no character for this account")
	// ErrNoSuchPlace is a key that names nowhere in internal/city.
	ErrNoSuchPlace = errors.New("character: no such place")
	// ErrDayIsSpent is §3.4's cap, reached. The request is fine and it is the
	// world that is in the way, which is a 409 at the edge.
	ErrDayIsSpent = errors.New("character: no paid actions left today")
	// ErrCannotAfford is a purchase larger than the balance.
	ErrCannotAfford = errors.New("character: not enough money")
)

// Character is the player, as this module holds them.
type Character struct {
	AccountID string
	Money     simulation.Money
	// At is where they are standing; Home is where they live. Two places
	// rather than one, because walking home is a thing you do (§3.12). Either
	// may be a street, an interior, or the home itself — internal/city's
	// layers are all places, and the player stands in exactly one of them.
	At   city.Place
	Home city.Place
	// Zone is the player's own timezone — the thing their day boundary is
	// measured in (§3.3).
	Zone *time.Location

	// EarnedOn and EarnedToday are the *inputs* to §3.4's cap, and the only
	// part of it that is stored. How many actions are left is read from them.
	EarnedOn    simulation.Date
	EarnedToday int
}

// Earnings is the derived half of the cap: what is true right now.
type Earnings struct {
	Taken int
	Left  int
}

// Earnings reads the cap against the player's own day.
//
// Derived on read, never stored (§3.3). The count in the row is only meaningful
// together with the date beside it: at the player's local midnight the same row
// starts meaning "none taken today" without anything having been written, which
// is precisely what a stored "remaining" would get wrong.
func (c Character) Earnings(clk simulation.Clock) Earnings {
	taken := c.EarnedToday
	if c.EarnedOn != simulation.Today(clk, c.Zone) {
		taken = 0
	}
	return Earnings{Taken: taken, Left: simulation.EarningsLeft(taken)}
}

// Wallet is what another module is allowed to know about a character's money.
//
// §3.1 requires cross-module access to go through an interface rather than a
// SELECT into someone else's tables. This is that interface: internal/health
// spends through it to buy instrumentation, and gets no way to read a district,
// a timezone, or anything else that is none of its business.
type Wallet interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
	Spend(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error
}

// Characters is the module. It holds no per-player state: the transaction comes
// from the caller and the clock is injected (§3.1).
type Characters struct {
	clk simulation.Clock
	// encounters is what a walk is offered to (§3.12). An interface with a
	// stub bound to it until the event system exists — see travel.go.
	encounters Encounters
}

func New(clk simulation.Clock, encounters Encounters) *Characters {
	return &Characters{clk: clk, encounters: encounters}
}

// Ensure creates the character if the account has none, and returns it.
//
// Called when a player signs in. Onboarding (#33) will replace the defaults
// here with answers that visibly change the starting state; until then every
// new character starts the same way, and that fact lives in one place.
func (m *Characters) Ensure(ctx context.Context, tx pgx.Tx, accountID string) (Character, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO character (account_id, money_cents, place, home)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id) DO NOTHING`,
		accountID, int64(startingMoney), city.Start, city.Lives,
	); err != nil {
		return Character{}, fmt.Errorf("character: ensure: %w", err)
	}
	return m.Of(ctx, tx, accountID)
}

// startingMoney is what a new character has. A placeholder, like the starting
// place: §3.13's onboarding (#33) decides the real starting situation, and
// where it starts is internal/city's to say.
const startingMoney = simulation.Money(120000) // $1,200

// Of reads the character.
func (m *Characters) Of(ctx context.Context, tx pgx.Tx, accountID string) (Character, error) {
	return m.read(ctx, tx, accountID, false)
}

// read loads the character, optionally taking the row lock that a
// read-modify-write needs.
//
// FOR UPDATE is what makes two commands arriving together correct rather than
// lucky: the second waits for the first to commit and then reads what it wrote,
// so a player pressing a button twice cannot spend the same money twice.
func (m *Characters) read(ctx context.Context, tx pgx.Tx, accountID string, lock bool) (Character, error) {
	q := `SELECT money_cents, place, home, timezone, earned_on, earned_today
	        FROM character WHERE account_id = $1`
	if lock {
		q += ` FOR UPDATE`
	}

	var (
		money    int64
		at, home string
		zone     string
		earnedOn time.Time
		earned   int
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(&money, &at, &home, &zone, &earnedOn, &earned)
	if errors.Is(err, pgx.ErrNoRows) {
		return Character{}, ErrNoCharacter
	}
	if err != nil {
		return Character{}, fmt.Errorf("character: read: %w", err)
	}

	c := Character{
		AccountID:   accountID,
		Money:       simulation.Money(money),
		Zone:        location(zone),
		EarnedToday: earned,
		// A DATE carries no timezone, and none is wanted: it is already the
		// player's own calendar day, written by whoever last earned.
		EarnedOn: simulation.On(earnedOn, time.UTC),
	}
	// A place that is no longer in the vocabulary keeps its key rather than
	// erroring: the city's list is still open (§17), and a player standing
	// somewhere that was renamed should be able to walk out of it.
	c.At = placeOrKey(at)
	c.Home = placeOrKey(home)
	return c, nil
}

// Worked is what a paid action produced.
type Worked struct {
	Character Character
	Earned    simulation.Money
}

// Work takes a paid action, if the player has one left today.
//
// The cap is §3.4's, the arithmetic is internal/simulation's, and this function
// is the part that has to be true about a database: read the row under a lock,
// ask the rules what it pays, write the result and the day it belongs to.
func (m *Characters) Work(ctx context.Context, tx pgx.Tx, accountID string, offer simulation.Money) (Worked, error) {
	c, err := m.read(ctx, tx, accountID, true)
	if err != nil {
		return Worked{}, err
	}

	today := simulation.Today(m.clk, c.Zone)
	taken := c.EarnedToday
	if c.EarnedOn != today {
		// Their midnight has passed since the count was written, so it is
		// somebody else's day and this is the first action of theirs.
		taken = 0
	}

	pay := simulation.Earn(offer, taken)
	if pay == 0 {
		return Worked{}, ErrDayIsSpent
	}

	if _, err := tx.Exec(ctx, `
		UPDATE character
		   SET money_cents  = money_cents + $2,
		       earned_on    = make_date($3, $4, $5),
		       earned_today = $6,
		       updated_at   = now()
		 WHERE account_id = $1`,
		accountID, pay.Cents(), today.Year, int(today.Month), today.Day, taken+1,
	); err != nil {
		return Worked{}, fmt.Errorf("character: work: %w", err)
	}

	after, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return Worked{}, err
	}
	return Worked{Character: after, Earned: pay}, nil
}

/*
Wage pays structural income in.

§3.4 splits earning in two, and this is the half the cap has nothing to say
about: "income is primarily **structural** — it comes from job tier, performance
and assets, not from repeating actions", and the cap applies "only to earning,
never to play". A day's wage arrives because a job is held and a day happened,
so there is no action to count and counting one would make holding a job spend
the same allowance as taking a paid action.

That is the whole reason this is a method rather than a call into [Work] with a
different argument. `earned_today` is deliberately untouched here, and
`TestAWageIsNotAPaidAction` is what keeps it that way.

There is no command behind it. Money a player could add to their own balance is
not a measurement of anything, in exactly the way a factor they could award
themselves is not (§3.5) — the only caller is internal/career, inside the
rollover's transaction.
*/
func (m *Characters) Wage(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error {
	if amount < 0 {
		return fmt.Errorf("character: wage: negative amount %d", amount)
	}
	if amount == 0 {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE character
		   SET money_cents = money_cents + $2, updated_at = now()
		 WHERE account_id = $1`,
		accountID, amount.Cents(),
	)
	if err != nil {
		return fmt.Errorf("character: wage: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoCharacter
	}
	return nil
}

// Balance answers the Wallet interface.
func (m *Characters) Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error) {
	c, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return 0, err
	}
	return c.Money, nil
}

// Spend takes money, refusing rather than going negative.
//
// The check and the write are one statement against a locked row, so two
// purchases arriving together cannot both see the same balance and both
// succeed. The affordability rule is enforced here rather than by the caller,
// because this is the only package that can enforce it.
func (m *Characters) Spend(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money) error {
	if amount < 0 {
		return fmt.Errorf("character: spend: negative amount %d", amount)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE character
		   SET money_cents = money_cents - $2, updated_at = now()
		 WHERE account_id = $1 AND money_cents >= $2`,
		accountID, amount.Cents(),
	)
	if err != nil {
		return fmt.Errorf("character: spend: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either there is no character or there is not enough money. The first
		// is a bug and the second is a refusal, so they are told apart rather
		// than reported as one.
		if _, err := m.Of(ctx, tx, accountID); err != nil {
			return err
		}
		return ErrCannotAfford
	}
	return nil
}

// placeOrKey is the place internal/city knows, or a stand-in wearing its key.
func placeOrKey(key string) city.Place {
	if p, ok := city.Lookup(key); ok {
		return p
	}
	return city.Place{Key: key, Name: key}
}

// location resolves a timezone name, falling back to UTC.
//
// A name the machine's timezone database does not have is a real possibility on
// a stripped container image, and the answer is a real timezone rather than
// whatever the process happens to be running in.
func location(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return zone
}
