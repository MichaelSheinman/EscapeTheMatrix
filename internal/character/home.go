package character

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Moving house: §3.15's relocation, as the one thing that changes who a player is
among.

The design calls this the core progression, and the reason it is four screens of
consequence and about forty lines of code is that everything it moves is
composed from `home` on read. The rule set in force, the daily conflicts, the
ceilings on the reading, the decay the reward is scaled by — all of them ask
which population the player lives among, and that question is answered by this
column. So relocating writes one word, and the world is different.

Two properties this file exists to hold:

  - **Moving puts you in the rooms.** Both `home` and `place` change, because
    taking somewhere to live and then being left across the river from it is not
    a thing that happens to anybody, and a world that said "you live on the
    riverfront" on one screen and "you are on the east side" on the next would
    be describing two different people.

    This is not in tension with the rule that the population follows where a
    player *lives* rather than where they are standing (internal/api's
    `population`). That rule exists so an afternoon downtown cannot reset the
    decay on a place somebody has exhausted, and it still holds exactly: walk
    back an hour later and `home` has not moved, so neither has the population.
    What it never required was that arriving leave you somewhere else.

  - **Not twice in a day.** §3.15's stay has to last across real days for a
    failing rule set to read as a pattern rather than as one bad morning, and a
    player who could bounce between populations inside an afternoon would never
    be anywhere long enough to be failed by. The day is the player's own, like
    every other day in this package.

What relocating deliberately does *not* cost is money. The price of a new
population is that your answers do not work there — that is the whole of §3.15 —
and a deposit on top would let a player with a full balance buy their way past
the only thing the mechanic is made of.
*/

// Move is a change of address: where they lived, and where they live now.
//
// From is sent back rather than left to the caller's memory because the only
// interesting thing about a move is that it was from somewhere.
type Move struct {
	Character Character
	From      city.Place
	To        city.Place
}

// Moved reports whether anything actually changed. A move to where they already
// live is not an error and not a move.
func (m Move) Moved() bool { return m.From.Key != m.To.Key }

/*
Relocate makes somewhere else home.

Moving to where they already live is allowed and changes nothing, for the same
reason [Characters.Travel] permits going where you already are (§8): a client
that lost the response to its first attempt has to be able to repeat it, and a
retry that came back as a refusal would be indistinguishable from a rule.

That case is also why the day is spent *only* by a move that happened. A repeat
of a successful relocation must not consume tomorrow's.
*/
func (m *Characters) Relocate(ctx context.Context, tx pgx.Tx, accountID, to string) (Move, error) {
	there, ok := city.Lookup(to)
	if !ok {
		return Move{}, ErrNoSuchPlace
	}
	// A street is not somewhere to live, and neither is a laundromat. The layer
	// is the city's answer and this is the only place it is asked, so there is
	// no second list of what may be moved into to disagree with it.
	if there.Layer != city.Home {
		return Move{}, ErrNobodyLivesThere
	}

	// Locked, because what follows is a read-modify-write against the day: two
	// moves arriving together must not both see a character who has not moved.
	c, err := m.read(ctx, tx, accountID, true)
	if err != nil {
		return Move{}, err
	}

	move := Move{Character: c, From: c.Home, To: there}
	if !move.Moved() {
		return move, nil
	}
	if c.Settled(m.clk) {
		return Move{}, ErrMovedToday
	}

	// `place` as well as `home`: you take the rooms and you are in them. There
	// is no journey to make of it — no route, no fare, and nothing offered to
	// [Encounters] — because what happened is that somebody moved house, not
	// that they walked across town. Movement still costs no game time (§3.1).
	today := simulation.Today(m.clk, c.Zone)
	if _, err := tx.Exec(ctx, `
		UPDATE character
		   SET home       = $2,
		       place      = $2,
		       moved_on   = make_date($3, $4, $5),
		       updated_at = now()
		 WHERE account_id = $1`,
		accountID, to, today.Year, int(today.Month), today.Day,
	); err != nil {
		return Move{}, fmt.Errorf("character: relocate: %w", err)
	}

	after, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return Move{}, err
	}
	move.Character = after
	return move, nil
}
