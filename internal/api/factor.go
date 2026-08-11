package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Progression on the wire (§3.5).

The rule that shapes this file is the same one internal/factor enforces in Go:
**the hidden factors do not cross this boundary.** A locked slot arrives with no
key, no name and no value — the client is structurally unable to render one,
because there is nothing in the payload to render. Which is why there is no
`hidden` list here and no per-factor score anywhere: withholding a number the
client was sent is a decision somebody can undo in a hurry, and not sending it is
not.

There is also no factor *command*. §7: hidden factors are scored from committed
choices, not from anything the player types, and a route that let a player add to
their own kindness would make the number a statement about the route rather than
about them. Credit arrives through internal/factor's Go surface, from the modules
that own the choices — #17's endorsement, #24's challenges, #30's conflicts.
*/

// slotView is one place on the board. The three hidden factors send this struct
// with nothing in it but `note` and `shown: false`.
type slotView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Reads says how to render the value — "age" or "money" — so the client
	// formats without knowing a rule. Empty for a locked slot.
	Reads string `json:"reads"`
	Note  string `json:"note"`
	// Null for a locked slot. Not zero: a zero is a reading, and there is no
	// reading — the same distinction the health panel draws for a metric this
	// player has no instrument for (§3.16).
	Value *float64 `json:"value"`
	Shown bool     `json:"shown"`
}

/*
karmaView is the composite and the board it was made of.

No scale accompanies it, and that is deliberate. §3.2 says Karma replaces the
level bar rather than renaming it, and a number sent alongside the maximum it
could reach is a progress bar however it is drawn. It is a reading, like the
biological age beside it.
*/
type karmaView struct {
	// Composed on read by the server, from a body, a balance and three totals
	// the player is never shown (§3.3, §3.5). Stored nowhere.
	Karma float64    `json:"karma"`
	Slots []slotView `json:"slots"`

	// Every day behind this player and what the composite read at the end of
	// each, oldest first — with today, still moving, on the end of it.
	//
	// Composites only. §3.5's formula stays unstated because a curve of the
	// mix says a thing happened without saying which of five things moved, and
	// working that out is the game. Five curves would be the answer key.
	Curve []pointView `json:"curve"`

	// Which population the player is among, what it is called, and how worn it
	// is (§3.15). A statement about *people* — it names no factor and gives
	// nothing about the formula away.
	//
	// `where` is the environment key, and the client needs it for more than a
	// label: it is the tag on the rule set actually in force (§3.15's
	// context-tagged playbook), and the server is the only thing that knows
	// which one that is. `among` is the same population as a person reads it,
	// sent rather than looked up because the client has a list of places and
	// populations are not places.
	Where       string  `json:"where"`
	Among       string  `json:"among"`
	Days        int     `json:"days"`
	Familiarity float64 `json:"familiarity"`
}

// pointView is one day on the curve. The date is the player's own local day, as
// a plain calendar date — no clock and no zone, because it is already theirs.
type pointView struct {
	Day   string  `json:"day"`
	Karma float64 `json:"karma"`
}

func karma(s factor.Standing) karmaView {
	out := karmaView{
		Karma:       s.Karma,
		Slots:       make([]slotView, 0, len(s.Slots)),
		Curve:       make([]pointView, 0, len(s.Curve)),
		Where:       s.Where,
		Among:       s.Among,
		Days:        s.Days,
		Familiarity: s.Familiarity,
	}
	for _, p := range s.Curve {
		out.Curve = append(out.Curve, pointView{
			Day:   p.Day.At().Format(time.DateOnly),
			Karma: p.Karma,
		})
	}
	for _, slot := range s.Slots {
		out.Slots = append(out.Slots, slotView{
			Key:   slot.Key,
			Name:  slot.Name,
			Reads: slot.Reads,
			Note:  slot.Note,
			Value: slot.Value,
			Shown: slot.Shown,
		})
	}
	return out
}

// What the player's tabs are told when the composite has moved.
type factorStood struct {
	Karma karmaView `json:"karma"`
}

var factorStoodMessage = realtime.Define[factorStood]("factor.stood")

/*
stood restates the composite to every tab this player has open.

Karma is a reading of everything at once — a body, a balance, three hidden totals
— so almost any command moves it, and no single command's response is the right
place to carry it. A `liveResult` that also stated a Karma would be the health
module answering a question about money.

So it goes over the socket instead, addressed to the account with **no Origin**:
the tab that acted gets it too, because the response it is about to apply
deliberately does not contain it. §8's rule 4 exists to stop a client hearing its
own action twice, and this is not the client's action — it is a consequence of it
that nothing else was going to say.

Called at the end of every command that can move the composite, inside that
command's transaction, so a command that rolled back never announced a standing
nobody has.
*/
func (s *Server) stood(ctx context.Context, tx pgx.Tx, accountID string) error {
	who, err := s.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return err
	}
	standing, err := s.standing(ctx, tx, accountID, simulation.Today(s.clk, who.Zone))
	if err != nil {
		return err
	}
	return factorStoodMessage.Publish(ctx, tx,
		realtime.To{Account: accountID}, factorStood{Karma: karma(standing)})
}

/*
standing composes a player's progression and writes today onto their curve.

The mark comes first so that what is composed a line later already has today on
the end of it, which is what makes the curve one list rather than a list and a
loose number the client has to append. It is also the only way the *first* day
of a player's curve exists: their first day ends before any rollover has a day
to walk, so a curve fed only by the rollover would begin on day two and never
show them where they started.

Every caller that states a standing to a player goes through here, and that is
the whole of the rule — the game does not tell somebody where they stand without
recording that it did.
*/
func (s *Server) standing(
	ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date,
) (factor.Standing, error) {
	if err := s.factors.Mark(ctx, tx, accountID, on); err != nil {
		return factor.Standing{}, err
	}
	return s.factors.Of(ctx, tx, accountID)
}

type purse struct{ characters *character.Characters }

/*
purse answers factor.Purse: what a player is holding.

It ensures the character exists rather than requiring one, which is the same
"a read that may write" internal/health does for the body — and here it is load-
bearing rather than tidy. The leaderboards (§3.5) compose progression for *every*
account at once, so a single account whose first wake failed and left no
character behind would otherwise take every board down with it. Creating one is
idempotent and is what the next page load would do anyway.
*/
func (p purse) Balance(ctx context.Context, tx pgx.Tx, accountID string) (simulation.Money, error) {
	who, err := p.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return 0, err
	}
	return who.Money, nil
}

/*
population answers factor.Environment and situation.Where: which population the
player is among.

§3.15's inversion, wired. An environment is defined by its people, so the
question "where is this player" is answered by looking up who lives around them
— and "around them" is decided by **where they live**, not by where they are
standing this second.

That distinction is the mechanic rather than a detail. A trip is a relocation
(§3.15: "a second home, not an excursion"), so the population you are among
changes when you move house and not when you walk downtown for an afternoon. If
it followed the player's feet, an hour at the office would reset the decay on a
population they have exhausted and a walk home would restore it — the ceiling
would be avoidable by pacing, and §3.15's whole pressure would be a rounding
error. It also means the rules in force are the rules for the people you live
among, which is the only reading under which "your old answers fail when you
arrive" is a thing that can happen.

It lives here rather than in internal/factor because it is a composition of three
modules — a character, a city and a catalogue of populations — which is what the
transport layer is for. Nobody reads anybody else's tables.
*/
type population struct{ characters *character.Characters }

// among is which population this player lives among, for the callers that need
// the key rather than the interface — the job surface, which is local (§3.15).
func (s *Server) among(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	return population{characters: s.characters}.Where(ctx, tx, accountID)
}

func (p population) Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	who, err := p.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return "", err
	}
	// A home is a place inside a district (§3.12's layers). Falling back to its
	// own key covers a home that *is* a district, which the city does not
	// currently author and which would otherwise read as the empty string.
	around := who.Home.Key
	if who.Home.Layer != city.District && who.Home.In != "" {
		around = who.Home.In
	}
	// Nobody, rather than an error. A district with no population authored for
	// it is a real state of the world — the city has four and #31 is what makes
	// the second population reachable — and being among nobody means no
	// ceilings and no wear, which is what environment.Nowhere says.
	if among, ok := environment.Lookup(around); ok {
		return among.Key, nil
	}
	return environment.Nowhere, nil
}
