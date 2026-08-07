package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/rollover"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The daily rollover on the wire (§3.3).

It is the one thing in the game that happens with nobody asking, which makes it
the clearest case for §8's push half: the player did not send a command, so
there is no response to carry the answer, and a tab left open overnight would
otherwise show yesterday's body until it was reloaded.

There is deliberately no rollover *command*. A player cannot ask for their day
to end early, and a route that let them would be a route that let them end it
twice — the catch-up is a consequence of arriving, not a thing to request.
*/

// rolledOver is what a player's open tabs are told when their world moved on
// its own.
//
// The whole panel and the whole forecast, restated, for the same reason living
// a day by hand restates them: the client computes no game rules (§10.2), and a
// body that has moved is a body whose every reading and whose whole arc have
// moved with it.
type rolledOver struct {
	Panel    panelView    `json:"panel"`
	Forecast forecastView `json:"forecast"`
	// Crossed is what the *last* day applied moved across. A threshold crossed
	// three weeks ago is not news, and a client handed thirty days of crossings
	// at once would have to decide which of them to say out loud.
	Crossed []crossedView `json:"crossed"`
	// Days and Skipped are the honest pair (§3.3's catch-up window): a player
	// back after a season has a large Skipped, and reporting only the days
	// lived would make a catch-up look like a full replay.
	Days    int `json:"days"`
	Skipped int `json:"skipped"`
}

var rolledOverMessage = realtime.Define[rolledOver]("rollover.ran")

/*
Rolled answers rollover.Announce.

The rollover module says *that* the world moved and this says what it looks
like, which is the split §3.1 asks for: message shapes live next to the views
they carry, and internal/rollover has no idea a socket exists.

It runs inside the rollover's own transaction, so the message and the days it
describes commit together — a player cannot be told about a body that was rolled
back, and a body cannot move without the tabs watching it being told.
*/
func (s *Server) Rolled(ctx context.Context, tx pgx.Tx, accountID string, r rollover.Report) error {
	who, err := s.characters.Of(ctx, tx, accountID)
	if err != nil {
		return err
	}
	p, err := s.health.Of(ctx, tx, accountID)
	if err != nil {
		return err
	}
	set, err := s.routines.Of(ctx, tx, accountID)
	if err != nil {
		return err
	}

	today := simulation.Today(s.clk, who.Zone)
	// No Origin: nobody's command caused this, so every tab this player has
	// open receives it — including the one that was on screen when it happened
	// (§8, rule 4 is about echoes of your own action, and this is not one).
	return rolledOverMessage.Publish(ctx, tx,
		realtime.To{Account: accountID},
		rolledOver{
			Panel:    panel(p, today),
			Forecast: forecast(p, set, today),
			Crossed:  crossings(r.Effects),
			Days:     r.Days,
			Skipped:  r.Skipped,
		},
	)
}

/*
Rollover is the module the queue's workers run.

Exposed so that cmd/server can hand it to internal/jobs and then tie the queue
back on with Through. That knot — the worker needs the rollover, the rollover
needs the queue — is the reason this accessor exists at all; nothing else in the
process asks for it.
*/
func (s *Server) Rollover() *rollover.Rollovers { return s.rollovers }

/*
wake catches a player up and re-arms their schedule.

Called from the two places a player is known to be here: the sign-in that opens
their session, and the world snapshot every page load fetches. §3.3 says the
next login triggers a catch-up rollover, and for a game whose sessions last a
month "login" in the useful sense is "opened the app" rather than "went through
Google again".

It runs in its own transaction, before the one that reads the world, and its
failure is logged and swallowed. A rollover that could not run is a player who
is a day behind and will be caught up by the next one; refusing to render their
world over it would turn a recoverable lag into a blank screen.
*/
func (s *Server) wake(ctx context.Context, accountID string) {
	r, err := s.rollovers.Waking(ctx, accountID)
	if err != nil {
		s.log.Error("catch-up rollover", "account", accountID, "error", err)
		return
	}
	if r.Ran() {
		s.log.Info("caught up",
			"account", accountID, "days", r.Days, "skipped", r.Skipped,
			"next", r.Next.Format(time.RFC3339))
	}
}
