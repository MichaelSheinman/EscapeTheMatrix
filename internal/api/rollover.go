package api

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
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
	if err := rolledOverMessage.Publish(ctx, tx,
		realtime.To{Account: accountID},
		rolledOver{
			Panel:    panel(p, today),
			Forecast: forecast(p, set, today),
			Crossed:  crossings(r.Effects),
			Days:     r.Days,
			Skipped:  r.Skipped,
		},
	); err != nil {
		return err
	}

	// A body that moved is a composite that moved (§3.5), and the rollover is
	// the one thing that can move it while nobody is looking. Sent as its own
	// message rather than folded into the one above, because Karma is a reading
	// of a balance and three hidden totals as much as of a body — the rollover
	// happens to be the occasion, not the subject.
	return s.stood(ctx, tx, accountID)
}

/*
work is §3.3's rollover, composed out of the modules that exist.

internal/rollover names the three pieces of its day as one interface and binds a
stub that does nothing, so that a rollover doing three quarters of its job is
visible in the code rather than only in a backlog. This is where those pieces are
filled in, one ticket at a time, without that package being edited.

Karma decay is the piece #15 brings. Ageing the environment the player is
standing in is per *day* rather than per run — §3.15's decay is a curve against
how thoroughly a place has been worked, and a player caught up from a week away
has a week of it to lose.
*/
type work struct {
	factors *factor.Factors
	// events is §4's backlog, topped up once per run. Its Replenish is in
	// event.go beside the rest of that module's transport, which is what makes
	// "one ticket fills in one method here" true rather than aspirational.
	events *event.Events
}

// Decay ages the environment the player is in, by one of their days (§3.15).
func (w work) Decay(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) error {
	return w.factors.Day(ctx, tx, accountID, on)
}

// Recompute recomputes environment ceilings and factor caps (§3.5, §3.15). #29
// is what authors them; an unauthored ceiling is no ceiling, which is what the
// pure core already does with a zero.
func (work) Recompute(context.Context, pgx.Tx, string, simulation.Date) error { return nil }

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
