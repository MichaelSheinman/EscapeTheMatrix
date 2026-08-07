package api

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/routine"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Routines on the wire: the rules the player has set for their own body (§3.4).

Two things cross this boundary, and they are deliberately different kinds of
thing. The routines themselves are *questions and answers* — a standing question,
a list of sentences, and which one is in force — which is the same shape the
playbook (§3.14) will send, because they are the same system. The forecast is a
statement about what those answers do, computed by running the pure core forward
and never stored (§3.3).

The forecast is instrumented exactly like the panel is. A player with no wearable
is not told what their HRV will be in a fortnight, for the same reason they are
not told what it is now: §3.16's rule is about resolution on your own body, and a
projection is a reading of a body too. Biological age is the exception at both
ends — visible at every tier, now and ahead.
*/

// optionView is one answer a routine will take.
type optionView struct {
	Key  string `json:"key"`
	Says string `json:"says"`
	// Costs is the honest half: what the option asks of the player. Nothing is
	// labelled good or bad (§3.2), so the price is stated and the verdict is
	// left to them.
	Costs  string `json:"costs"`
	Chosen bool   `json:"chosen"`
}

// routineView is one routine: what it asks, what may be answered, and what is.
type routineView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Asks string `json:"asks"`
	// Learn is what the player is still missing before they may configure this
	// one (§3.6). Sent whether or not it is locked, because it is the sentence
	// that says *why* — and a client that has to invent it would say it wrong.
	Learn    string       `json:"learn"`
	Unlocked bool         `json:"unlocked"`
	Chosen   string       `json:"chosen"`
	Options  []optionView `json:"options"`
}

type routinesView struct {
	// Set is whether the player has ever configured these. Different from
	// having chosen the answers that happen to be the defaults.
	Set      bool          `json:"set"`
	Routines []routineView `json:"routines"`
}

func routines(c routine.Configured) routinesView {
	out := routinesView{Set: c.Set, Routines: make([]routineView, 0, len(routine.Kinds))}
	for _, k := range routine.Kinds {
		chosen := c.Choices[k.Key]
		view := routineView{
			Key: k.Key, Name: k.Name, Asks: k.Asks, Learn: k.Learn,
			Unlocked: c.Unlocked[k.Key],
			Chosen:   chosen,
			Options:  make([]optionView, 0, len(k.Options)),
		}
		for _, o := range k.Options {
			view.Options = append(view.Options, optionView{
				Key: o.Key, Says: o.Says, Costs: o.Costs, Chosen: o.Key == chosen,
			})
		}
		out.Routines = append(out.Routines, view)
	}
	return out
}

// forecastDays is how far ahead a routine is shown.
//
// A fortnight, because that is the shortest window in which the slow markers
// move at all — LDL takes weeks by design — and because it is short enough that
// a player can decide to actually live it.
const forecastDays = 14

// forecastMetricView is one metric's arc under the current routines.
type forecastMetricView struct {
	Key string `json:"key"`
	// Empty for a metric this player has no instrument for. The same rule as
	// the panel's: what is withheld is the reading, not the idea.
	Values []float64 `json:"values"`
}

/*
forecastView is what the routines currently in force would do over the next
fortnight, if they were kept.

§3.4 asks the player to *watch* their routines move their biomarkers, and the
honest version of that takes real days — one game day is one real calendar day
(§3.1) and nothing here changes that. So the game shows both: the days as they
are lived, and the arc they are on if nothing changes. This is the second one.

The post is a parameter because a job is part of the arc (§3.14). A player on
nights is not living the fortnight their routines describe, and a forecast that
projected the routines alone would be the screen promising a body the rollover
was never going to produce — the two run the same [simulation.Working] over the
same days for exactly that reason.

Composed on read and never stored (§3.3). It has to be — a projection is a
statement about days that have not happened, and a persisted one is a second
answer waiting to disagree with the first.
*/
type forecastView struct {
	Days int `json:"days"`
	// BiologicalAge is one reading per projected day, visible at every tier
	// (§3.5) — the headline the whole screen is arguing about.
	BiologicalAge []float64            `json:"biologicalAge"`
	Metrics       []forecastMetricView `json:"metrics"`
}

func forecast(p health.Panel, c routine.Configured, post simulation.Post, today simulation.Date) forecastView {
	// The first day ahead is today, unless today has already been lived — in
	// which case it is on the panel already and restating it would look like a
	// night the routine has not had yet. Reading it off the panel rather than
	// taking it as a parameter is what keeps a forecast and the days that
	// follow it the same fortnight.
	from := today
	if p.LastDay == today {
		from = today.Next()
	}

	ahead := simulation.Project(p.Body, c.Routine(), post, from, forecastDays)

	out := forecastView{
		Days:          len(ahead),
		BiologicalAge: make([]float64, 0, len(ahead)),
		Metrics:       make([]forecastMetricView, 0, len(health.Metrics)),
	}
	for _, day := range ahead {
		out.BiologicalAge = append(out.BiologicalAge, simulation.BiologicalAge(day.Body))
	}
	for _, m := range health.Metrics {
		view := forecastMetricView{Key: m.Key, Values: []float64{}}
		if p.Sees(m) {
			for _, day := range ahead {
				view.Values = append(view.Values, m.Read(day.Body))
			}
		}
		out.Metrics = append(out.Metrics, view)
	}
	return out
}

// What the player's other tabs are told when the rules change.
type routinesSet struct {
	Routines routinesView `json:"routines"`
	Forecast forecastView `json:"forecast"`
}

var routinesSetMessage = realtime.Define[routinesSet]("routine.set")

/*
Setting the routines.

The whole configuration arrives at once and the whole of it is written (§3.4's
"configured as a unit"). There is no command to change one routine, because a set
of rules for the body is one decision — deciding to sleep more is deciding what
that costs the evening — and a per-routine command would let a client compose a
configuration the player never looked at.

The answers are keys from a closed vocabulary. Nothing here takes a free field,
which is the same rule the playbook is built on: the set of terms is the set of
thinkable strategies (§3.14), so a term that is not in the list is not a thing to
be permissive about.
*/
type setRoutinesInput struct {
	Choices map[string]string `json:"choices"`
}

type setRoutinesResult struct {
	Routines routinesView `json:"routines"`
	Forecast forecastView `json:"forecast"`
}

func (s *Server) setRoutines(ctx context.Context, tx pgx.Tx, actor command.Actor, in setRoutinesInput) (setRoutinesResult, error) {
	c, err := s.routines.Set(ctx, tx, actor.ID, routine.Choices(in.Choices))
	switch {
	case errors.Is(err, routine.ErrNoSuchRoutine):
		// Wrong forever, however often it is sent.
		return setRoutinesResult{}, command.Invalid("there is no routine by that name")
	case errors.Is(err, routine.ErrNoSuchOption):
		return setRoutinesResult{}, command.Invalid("that is not one of the answers to that routine")
	case errors.Is(err, routine.ErrLocked):
		// The world is in the way rather than the request being wrong: the same
		// command works once the player has been through what unlocks it (§3.6).
		return setRoutinesResult{}, command.Conflict(
			"you have not been through what that routine is for yet")
	case err != nil:
		return setRoutinesResult{}, err
	}

	out, err := s.routineScreen(ctx, tx, actor.ID, c)
	if err != nil {
		return setRoutinesResult{}, err
	}

	if err := routinesSetMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		routinesSet(out),
	); err != nil {
		return setRoutinesResult{}, err
	}
	return out, nil
}

/*
Living today by the routines.

This is what `health.live` was standing in for. §3.4's arc is toward "set it once
and never worry about it again", so the player states the rules and the day
follows from them — the three choices are produced rather than answered, which is
the whole difference a routine makes.

It is one command per day, triggered by hand, and it is now the *second* way a
day gets lived. The daily rollover (§3.3, internal/rollover) calls the same seam
for every day a player was away for, and it stops strictly short of today — so
this command lives the day that is still happening and the rollover lives the
ones that have ended. Neither can take the other's, and because the pure core is
deterministic in the date rather than in the order things were asked, the days
they produce are the same days.
*/
func (s *Server) liveByRoutine(ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{}) (liveResult, error) {
	// The day boundary is the player's own (§3.3), and the character is what
	// knows where they are. Composed at the transport, which is what the
	// transport is for — neither module reads the other's tables.
	who, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return liveResult{}, err
	}
	today := simulation.Today(s.clk, who.Zone)

	day, err := s.routines.Day(ctx, tx, actor.ID, today)
	if err != nil {
		return liveResult{}, err
	}
	// The job takes its hours out of the day before the body has it, exactly as
	// it does in the rollover (§3.14). Living today by hand must not be a way to
	// live a day the job was not in — and it is the same call, so the two cannot
	// produce different days for the same date.
	if day, err = s.careers.Shift(ctx, tx, actor.ID, today, day); err != nil {
		return liveResult{}, err
	}

	p, fx, err := s.health.Lived(ctx, tx, actor.ID, today, day)
	switch {
	case errors.Is(err, health.ErrAlreadyLived):
		return liveResult{}, command.Conflict("today has already been lived")
	case err != nil:
		return liveResult{}, err
	}

	out := liveResult{Panel: panel(p, today), Crossed: crossings(fx)}

	// The same message living a day by hand publishes. What the other tabs need
	// to know is that the body moved, and it moved the same way whichever
	// command asked for it.
	if err := healthLivedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		healthLived{Panel: out.Panel, Crossed: out.Crossed},
	); err != nil {
		return liveResult{}, err
	}
	// A body that moved is a composite that moved (§3.5).
	if err := s.stood(ctx, tx, actor.ID); err != nil {
		return liveResult{}, err
	}
	return out, nil
}

// routineScreen is the routines and what they would do, read together.
//
// One place rather than three, because the forecast is a reading of the body
// under the rules and the two are never worth sending apart.
func (s *Server) routineScreen(ctx context.Context, tx pgx.Tx, accountID string, c routine.Configured) (setRoutinesResult, error) {
	p, err := s.health.Ensure(ctx, tx, accountID)
	if err != nil {
		return setRoutinesResult{}, err
	}
	who, err := s.characters.Of(ctx, tx, accountID)
	if err != nil {
		return setRoutinesResult{}, err
	}
	held, err := s.careers.Of(ctx, tx, accountID)
	if err != nil {
		return setRoutinesResult{}, err
	}
	today := simulation.Today(s.clk, who.Zone)
	return setRoutinesResult{
		Routines: routines(c),
		Forecast: forecast(p, c, held.Post(), today),
	}, nil
}
