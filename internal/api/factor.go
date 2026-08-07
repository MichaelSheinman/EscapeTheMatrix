package api

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
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

	// Where the player is, as a place key the client already has a name for,
	// and how worn it is (§3.15). A statement about a place — it names no
	// factor and gives nothing about the formula away.
	Where       string  `json:"where"`
	Days        int     `json:"days"`
	Familiarity float64 `json:"familiarity"`
}

func karma(s factor.Standing) karmaView {
	out := karmaView{
		Karma:       s.Karma,
		Slots:       make([]slotView, 0, len(s.Slots)),
		Where:       s.Where,
		Days:        s.Days,
		Familiarity: s.Familiarity,
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
	standing, err := s.factors.Of(ctx, tx, accountID)
	if err != nil {
		return err
	}
	return factorStoodMessage.Publish(ctx, tx,
		realtime.To{Account: accountID}, factorStood{Karma: karma(standing)})
}

/*
district answers factor.Environment: which environment a player is in.

§3.15 says an environment is defined by its people rather than its geography, and
#29 is the ticket that brings populations. Until it does, the district a player
is standing in is the closest honest answer the world can give — it is already
the thing that changes when they relocate, which is the property the decay curve
actually leans on, and internal/city already knows that a shop on the East Side
is on the East Side.

It lives here rather than in internal/factor because it is a composition of two
modules, which is what the transport layer is for: neither reads the other's
tables, and the day #29 lands this binding is replaced rather than edited.
*/
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

type district struct{ characters *character.Characters }

func (d district) Where(ctx context.Context, tx pgx.Tx, accountID string) (string, error) {
	who, err := d.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return "", err
	}
	// A district is its own environment; anywhere else is the district it sits
	// inside (§3.12's layers). A place with neither is not reachable in the
	// city as authored, and falling back to its own key keeps a wear record
	// attached to somewhere rather than to the empty string.
	if who.At.Layer != city.District && who.At.In != "" {
		return who.At.In, nil
	}
	return who.At.Key, nil
}
