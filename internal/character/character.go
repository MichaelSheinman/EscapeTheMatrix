// Package character owns the player: their money, where they are standing, and
// where they live. Another module that needs to know whether a player can
// afford something asks through the [Wallet] interface below. Every method
// takes the caller's transaction and never opens one. The rules live in
// internal/simulation and the geography lives in internal/city.
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
	// ErrNoCharacter means the account has no character row. Every account gets
	// one, so this is a bug rather than a state and is reported as such.
	ErrNoCharacter = errors.New("character: no character for this account")
	// ErrNoSuchPlace is a key that names nowhere in internal/city.
	ErrNoSuchPlace = errors.New("character: no such place")
	// ErrDayIsSpent is §3.4's cap, reached. The request is fine and it is the
	// world that is in the way, which is a 409 at the edge.
	ErrDayIsSpent = errors.New("character: no paid actions left today")
	// ErrCannotAfford is a purchase larger than the balance.
	ErrCannotAfford = errors.New("character: not enough money")
	// ErrNobodyLivesThere is a place that is real and is not somewhere to live.
	ErrNobodyLivesThere = errors.New("character: nobody lives there")
	// ErrMovedToday is §3.15's one move a day, already made. Like ErrDayIsSpent
	// it is the world in the way rather than the request, which is a 409.
	ErrMovedToday = errors.New("character: already moved today")
)

// Character is the player, as this module holds them.
type Character struct {
	AccountID string
	Money     simulation.Money
	// At is where they are standing and Home is where they live. Two places
	// rather than one, because walking home is a thing you do (§3.12). Either
	// may be a street, an interior or the home itself.
	At   city.Place
	Home city.Place
	// Zone is the player's own timezone — the thing their day boundary is
	// measured in (§3.3).
	Zone *time.Location

	// EarnedOn and EarnedToday are the inputs to §3.4's cap and the only part of
	// it that is stored. How many actions are left is read from them.
	EarnedOn    simulation.Date
	EarnedToday int

	// MovedOn is the player's own day Home last changed (§3.15). Whether they
	// may move again is read from it against today, never stored.
	MovedOn simulation.Date
}

// Settled reports whether the player has already moved house today — §3.15's one
// move a day, as a question about right now.
func (c Character) Settled(clk simulation.Clock) bool {
	return c.MovedOn == simulation.Today(clk, c.Zone)
}

// Earnings is the derived half of the cap: what is true right now.
type Earnings struct {
	Taken int
	Left  int
}

// Earnings reads the cap against the player's own day. Derived on read and never
// stored (§3.3): the count in the row is only meaningful together with the date
// beside it, so at local midnight the same row starts meaning "none taken today"
// without anything being written.
func (c Character) Earnings(clk simulation.Clock) Earnings {
	taken := c.EarnedToday
	if c.EarnedOn != simulation.Today(clk, c.Zone) {
		taken = 0
	}
	return Earnings{Taken: taken, Left: simulation.EarningsLeft(taken)}
}

// Wallet is what another module is allowed to know about a character's money
// (§3.1). internal/health spends through it to buy instrumentation, and it
// reaches no district, timezone or anything else this module holds.
//
// A spend names what it was for. The pair goes on the ledger line, and a caller
// that could move money without saying why would be a hole in the statement.
type Wallet interface {
	Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error)
	Spend(ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money, kind, counterparty string) error
}

/*
Recorder is where a movement of money is written down — internal/ledger, as the
one thing this module is allowed to know about it.

Every method below that changes money_cents calls it, inside the same statement's
transaction, with the balance the update returned. That is what makes the
statement complete: the ledger is not a second place money is tracked, it is the
only account of the one place it is.
*/
type Recorder interface {
	Record(
		ctx context.Context, tx pgx.Tx, accountID string,
		kind string, counterparty string, amount, after simulation.Money,
	) error
}

// Characters is the module. It holds no per-player state: the transaction comes
// from the caller and the clock is injected (§3.1).
type Characters struct {
	clk simulation.Clock
	// encounters is what a walk is offered to (§3.12) — see travel.go.
	encounters Encounters
	record     Recorder
}

func New(clk simulation.Clock, encounters Encounters, record Recorder) *Characters {
	return &Characters{clk: clk, encounters: encounters, record: record}
}

// Ensure creates the character if the account has none, and returns it. Called
// when a player signs in. Every new character starts the same way, and that fact
// lives here.
func (m *Characters) Ensure(ctx context.Context, tx pgx.Tx, accountID string) (Character, error) {
	// RETURNING gives a row only when the insert happened, which is what tells a
	// first sign-in from every one after it — so the opening line is written
	// once and a returning player does not open their statement again.
	var opened int64
	err := tx.QueryRow(ctx, `
		INSERT INTO character (account_id, money_cents, place, home)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id) DO NOTHING
		RETURNING money_cents`,
		accountID, int64(startingMoney), city.Start, city.Lives,
	).Scan(&opened)
	switch {
	case err == nil:
		if err := m.record.Record(ctx, tx, accountID, "opening", "",
			simulation.Money(opened), simulation.Money(opened)); err != nil {
			return Character{}, err
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return Character{}, fmt.Errorf("character: ensure: %w", err)
	}
	return m.Of(ctx, tx, accountID)
}

// startingMoney is what a new character has.
const startingMoney = simulation.Money(120000) // $1,200

// Of reads the character.
func (m *Characters) Of(ctx context.Context, tx pgx.Tx, accountID string) (Character, error) {
	return m.read(ctx, tx, accountID, false)
}

// read loads the character, optionally taking the row lock a read-modify-write
// needs. FOR UPDATE is what serialises two commands arriving together: the
// second waits for the first to commit and reads what it wrote, so a player
// pressing a button twice cannot spend the same money twice.
func (m *Characters) read(ctx context.Context, tx pgx.Tx, accountID string, lock bool) (Character, error) {
	q := `SELECT money_cents, place, home, timezone, earned_on, earned_today, moved_on
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
		movedOn  time.Time
	)
	err := tx.QueryRow(ctx, q, accountID).Scan(&money, &at, &home, &zone, &earnedOn, &earned, &movedOn)
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
		MovedOn:  simulation.On(movedOn, time.UTC),
	}
	// A place that is no longer in the vocabulary keeps its key rather than
	// erroring: the city's list is still open (§17), and a player standing
	// somewhere that was renamed should be able to walk out of it.
	c.At = placeOrKey(at)
	c.Home = placeOrKey(home)
	return c, nil
}

// SetZone records the timezone the player's device reports.
//
// The day boundary is measured in the player's own timezone (§3.3), and the only
// party that knows it is their browser: sign-in is a redirect, so the name
// arrives on the world snapshot's header instead (internal/api/world.go). The
// caller validates it against the timezone database, and the write here only
// happens on a change, so an ordinary page load writes nothing.
func (m *Characters) SetZone(ctx context.Context, tx pgx.Tx, accountID, zone string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE character
		   SET timezone = $2, updated_at = now()
		 WHERE account_id = $1 AND timezone <> $2`,
		accountID, zone,
	); err != nil {
		return fmt.Errorf("character: set zone: %w", err)
	}
	return nil
}

// Worked is what a paid action produced.
type Worked struct {
	Character Character
	Earned    simulation.Money
}

// Work takes a paid action, if the player has one left today. The cap is §3.4's
// and the arithmetic is internal/simulation's; this is the part that has to be
// true about a database: read the row under a lock, ask the rules what it pays,
// write the result and the day it belongs to.
func (m *Characters) Work(
	ctx context.Context, tx pgx.Tx, accountID string, offer simulation.Money, counterparty string,
) (Worked, error) {
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

	var balance int64
	if err := tx.QueryRow(ctx, `
		UPDATE character
		   SET money_cents  = money_cents + $2,
		       earned_on    = make_date($3, $4, $5),
		       earned_today = $6,
		       updated_at   = now()
		 WHERE account_id = $1
		 RETURNING money_cents`,
		accountID, pay.Cents(), today.Year, int(today.Month), today.Day, taken+1,
	).Scan(&balance); err != nil {
		return Worked{}, fmt.Errorf("character: work: %w", err)
	}
	if err := m.record.Record(ctx, tx, accountID, "paid", counterparty,
		pay, simulation.Money(balance)); err != nil {
		return Worked{}, err
	}

	after, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return Worked{}, err
	}
	return Worked{Character: after, Earned: pay}, nil
}

/*
Wage pays structural income in — the half of §3.4's earning the cap has nothing
to say about. A day's wage arrives because a job is held and a day happened, so
there is no action to count, and counting one would make holding a job spend the
same allowance as taking a paid action.

That is why this is a method rather than a call into [Work] with a different
argument. earned_today is untouched here, and TestAWageIsNotAPaidAction keeps it
that way.

There is no command behind it: the only caller is internal/career, inside the
rollover's transaction.
*/
func (m *Characters) Wage(
	ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money, kind, counterparty string,
) error {
	if amount < 0 {
		return fmt.Errorf("character: wage: negative amount %d", amount)
	}
	if amount == 0 {
		return nil
	}
	var balance int64
	err := tx.QueryRow(ctx, `
		UPDATE character
		   SET money_cents = money_cents + $2, updated_at = now()
		 WHERE account_id = $1
		 RETURNING money_cents`,
		accountID, amount.Cents(),
	).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoCharacter
	}
	if err != nil {
		return fmt.Errorf("character: wage: %w", err)
	}
	return m.record.Record(ctx, tx, accountID, kind, counterparty, amount, simulation.Money(balance))
}

// Balance answers the Wallet interface.
func (m *Characters) Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error) {
	c, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return 0, err
	}
	return c.Money, nil
}

// Spend takes money, refusing rather than going negative. The check and the
// write are one statement against a locked row, so two purchases arriving
// together cannot both see the same balance and both succeed. Affordability is
// enforced here because this is the only package that can enforce it.
func (m *Characters) Spend(
	ctx context.Context, tx pgx.Tx, accountID string, amount simulation.Money, kind, counterparty string,
) error {
	if amount < 0 {
		return fmt.Errorf("character: spend: negative amount %d", amount)
	}

	var balance int64
	err := tx.QueryRow(ctx, `
		UPDATE character
		   SET money_cents = money_cents - $2, updated_at = now()
		 WHERE account_id = $1 AND money_cents >= $2
		 RETURNING money_cents`,
		accountID, amount.Cents(),
	).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either there is no character or there is not enough money — a bug and
		// a refusal, told apart rather than reported as one.
		if _, err := m.Of(ctx, tx, accountID); err != nil {
			return err
		}
		return ErrCannotAfford
	}
	if err != nil {
		return fmt.Errorf("character: spend: %w", err)
	}
	return m.record.Record(ctx, tx, accountID, kind, counterparty, -amount, simulation.Money(balance))
}

// placeOrKey is the place internal/city knows, or a stand-in wearing its key.
func placeOrKey(key string) city.Place {
	if p, ok := city.Lookup(key); ok {
		return p
	}
	return city.Place{Key: key, Name: key}
}

// location resolves a timezone name, falling back to UTC. A name the machine's
// timezone database does not have is possible on a stripped container image, and
// UTC is a real timezone rather than whatever the process is running in.
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
