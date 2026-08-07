package character

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Getting about: §3.12's walk-versus-transit trade, as the one thing that moves a
character.

The geography is `internal/city` and has no state in it. What is here is the
part that has to be true about a database: read the row under a lock, ask the
city for a route, charge what it costs, write where they ended up — and give the
walk to whatever is going to make something happen on it.

Three properties this file exists to hold, all of them from §3.12 and §3.1:

  - **Movement costs no game time.** Nothing below reads the clock, touches
    `earned_on` / `earned_today`, or advances a day. Travelling is not an
    action, it is how you get to one. TestTravellingCostsNoGameTime asserts it
    against Postgres rather than against this paragraph.

  - **Walking is free.** Every walk in the city has a zero fare, so the money
    path below is only ever reached by a ride. That is enforced by the city, not
    by a condition here — see TestEveryPlaceCanBeWalkedToFromEveryOther.

  - **Walking is the encounter surface; transit skips it.** A walk is offered,
    step by step, to [Encounters]. A ride is not offered at all. That is the
    whole of the trade: the fare buys you out of your own life for one journey.
*/

var (
	// ErrNoWay is a journey the city does not offer in the mode that was asked
	// for. Not a bad request — the places are real and the player may well be
	// able to make the journey another way, or after walking out of a building
	// — so it is a 409 at the edge rather than a 422.
	ErrNoWay = errors.New("character: there is no way there like that")
)

/*
Encounters is what a walk offers the rest of the game.

§3.12: "walking is the encounter surface, not a wait… movement costs no game
time, but it is where things happen to you." §3.9's placement conditions — on
foot downtown, at home late — are the same objects as the trigger zones an author
draws in a location (technical.md §10.3), and the thing that decides *which* of
them fires is the event system: #21 for the backlog and its lifecycle, #22 for
release policy and escalation.

Neither exists yet, so this is an interface with a stub bound to it, in the same
way internal/routine names its Gate and internal/rollover names its World. The
ticket that builds the backlog replaces the binding in internal/api rather than
editing this package, and until then what is missing is visible in the code
instead of only in a backlog.
*/
type Encounters interface {
	// Walked is one step of a journey taken on foot, offered as it happens.
	// Inside the caller's transaction, like everything else (§3.2): an
	// encounter placed by a walk that then rolled back never happened.
	Walked(ctx context.Context, tx pgx.Tx, accountID string, step Step) error
}

// Step is one leg of a journey: where it went, and how.
type Step struct {
	From city.Place
	To   city.Place
	Mode city.Mode
	// Says is the step as a sentence fragment — "in through the door".
	Says string
}

// Quiet is an Encounters with nothing in the city yet.
//
// Not a silent default: it is passed in explicitly at the one place the server
// is wired, so the line that will one day name the event system is already
// there and already read at boot.
type Quiet struct{}

func (Quiet) Walked(context.Context, pgx.Tx, string, Step) error { return nil }

/*
Trip is one journey, and what it was.

`Chances` is the honest half of the trade before the event system exists: how
many walked steps this journey passed through, each of which is somewhere an
encounter could have been placed. It is zero for a ride, always, and that
difference is the thing a player can see today. #21 turns a chance into a thing
that happened; the number does not change shape when it does.
*/
type Trip struct {
	Character Character
	By        city.Mode
	// Steps is the journey, in order. Empty when the player was already there.
	Steps []Step
	Fare  simulation.Money
	// Chances is how many steps were walked. Zero for a ride.
	Chances int
}

/*
Travel moves the character, on foot or by transit.

Going to where they already are is not an error. It changes nothing, costs
nothing, and refusing it would mean a client that lost the response to its first
attempt could not safely repeat it (§8).
*/
func (m *Characters) Travel(ctx context.Context, tx pgx.Tx, accountID, to string, by city.Mode) (Trip, error) {
	if !city.Known(to) {
		return Trip{}, ErrNoSuchPlace
	}

	// Locked, because the fare below is a read-modify-write against money and
	// two rides arriving together must not both see the same balance.
	c, err := m.read(ctx, tx, accountID, true)
	if err != nil {
		return Trip{}, err
	}

	route, ok := city.Route(c.At.Key, to, by)
	if !ok {
		return Trip{}, ErrNoWay
	}

	trip := Trip{By: by, Fare: city.Cost(route)}
	for _, w := range route {
		trip.Steps = append(trip.Steps, Step{
			From: placeOrKey(w.From),
			To:   placeOrKey(w.To),
			Mode: w.Mode,
			Says: w.Name,
		})
		if w.Mode == city.OnFoot {
			trip.Chances++
		}
	}

	// The fare first, so a journey that cannot be paid for does not also happen.
	// Spend refuses rather than going negative, and the refusal is the caller's
	// to turn into a status code.
	if trip.Fare > 0 {
		if err := m.Spend(ctx, tx, accountID, trip.Fare); err != nil {
			return Trip{}, err
		}
	}

	if len(route) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE character SET place = $2, updated_at = now() WHERE account_id = $1`,
			accountID, to,
		); err != nil {
			return Trip{}, fmt.Errorf("character: travel: %w", err)
		}
	}

	// What happened on the way. Offered step by step rather than as a journey,
	// because §3.9 places events at a spot rather than at an itinerary — and
	// because a ride, which is offered nothing, is the same code path with an
	// empty list rather than a special case.
	for _, step := range trip.Steps {
		if step.Mode != city.OnFoot {
			continue
		}
		if err := m.encounters.Walked(ctx, tx, accountID, step); err != nil {
			return Trip{}, fmt.Errorf("character: travel: %w", err)
		}
	}

	after, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return Trip{}, err
	}
	trip.Character = after
	return trip, nil
}
