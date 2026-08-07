package api

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/health"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The health panel on the wire.
//
// §3.16's rule survives the trip: the simulation is complete for everyone, and
// instrumentation decides what crosses this boundary. A metric the player
// cannot see is sent as itself — name, unit, what it means, and which
// instrument would show it — with no value and no history. Which is the
// difference that matters: the player knows the measurement exists and knows
// what would let them take it. What is withheld is the reading, not the idea.

type tierView struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Buys  string `json:"buys"`
	Price int64  `json:"price"`
	Owned bool   `json:"owned"`
	// ForSale is false for the baseline, which everyone has and nobody buys.
	ForSale bool `json:"forSale"`
}

type metricView struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Unit     string `json:"unit"`
	Means    string `json:"means"`
	Tier     string `json:"tier"`
	TierName string `json:"tierName"`
	// Lower says which way is the healthy one, so a client can draw a trend
	// without knowing a rule.
	Lower    bool `json:"lower"`
	Decimals int  `json:"decimals"`

	Visible bool `json:"visible"`
	// Null when this player has no instrument for it. Not zero — a zero is a
	// reading, and there is no reading.
	Value *float64 `json:"value"`
	// Oldest first. Empty for a metric that cannot be seen, and full the moment
	// the instrument that sees it is bought, because the samples were being
	// written all along.
	History []float64 `json:"history"`
}

type panelView struct {
	// Visible at every tier, always, and composed on read from the panel
	// (§3.5, §3.3). Never stored.
	BiologicalAge float64 `json:"biologicalAge"`
	// Whether today has already been lived. One game day is one real day (§3.1).
	LivedToday bool         `json:"livedToday"`
	Tiers      []tierView   `json:"tiers"`
	Metrics    []metricView `json:"metrics"`
}

func panel(p health.Panel, today simulation.Date) panelView {
	out := panelView{
		BiologicalAge: simulation.BiologicalAge(p.Body),
		LivedToday:    p.LastDay == today,
	}

	owned := map[string]bool{}
	for _, t := range p.Owned {
		owned[t.Key] = true
	}
	for _, t := range append([]health.Tier{health.Baseline}, health.ForSale...) {
		out.Tiers = append(out.Tiers, tierView{
			Key:     t.Key,
			Name:    t.Name,
			Buys:    t.Buys,
			Price:   t.Price.Cents(),
			Owned:   owned[t.Key] || t.Key == health.Baseline.Key,
			ForSale: t.Key != health.Baseline.Key,
		})
	}

	for _, m := range health.Metrics {
		view := metricView{
			Key: m.Key, Name: m.Name, Unit: m.Unit, Means: m.Means,
			Tier: m.Tier.Key, TierName: m.Tier.Name,
			Lower: m.Lower, Decimals: m.Decimals,
			Visible: p.Sees(m),
			History: []float64{},
		}
		if view.Visible {
			v := m.Read(p.Body)
			view.Value = &v
			for _, s := range p.History {
				view.History = append(view.History, m.Read(s.Body))
			}
		}
		out.Metrics = append(out.Metrics, view)
	}
	return out
}

// crossedView is a band a metric moved across today — the pure core's effect,
// carried out to the player unchanged.
type crossedView struct {
	Metric string `json:"metric"`
	From   string `json:"from"`
	To     string `json:"to"`
	// Worse says which direction it went, so the client says it the right way
	// round without knowing what any band means.
	Worse bool `json:"worse"`
}

func crossings(fx simulation.Effects) []crossedView {
	out := []crossedView{}
	for _, e := range fx {
		c, ok := e.(simulation.Crossed)
		if !ok {
			continue
		}
		out = append(out, crossedView{
			Metric: c.Metric,
			From:   c.From.String(),
			To:     c.To.String(),
			Worse:  c.To < c.From,
		})
	}
	return out
}

// What the player's other tabs are told.
type healthLived struct {
	Panel   panelView     `json:"panel"`
	Crossed []crossedView `json:"crossed"`
}

type healthBought struct {
	Panel panelView `json:"panel"`
	Tier  tierView  `json:"tier"`
	Spent int64     `json:"spent"`
	Money int64     `json:"money"`
}

var (
	healthLivedMessage  = realtime.Define[healthLived]("health.lived")
	healthBoughtMessage = realtime.Define[healthBought]("health.bought")
)

/*
Living a day.

The inputs are a closed vocabulary rather than numbers, because that is what a
routine (§3.4) will produce once routines exist — this command is the interim
driver, and the shape it takes now is the shape the rollover (§3.3) will apply
automatically later. Nothing here accepts a free field.
*/
type liveInput struct {
	Sleep    string `json:"sleep"`
	Training string `json:"training"`
	Diet     string `json:"diet"`
}

var (
	sleeps = map[string]time.Duration{
		"short":    5 * time.Hour,
		"ordinary": 7 * time.Hour,
		"full":     8*time.Hour + 30*time.Minute,
	}
	trainings = map[string]simulation.Training{
		"rest":  simulation.Rest,
		"light": simulation.Light,
		"hard":  simulation.Hard,
	}
	diets = map[string]simulation.Diet{
		"poor":  simulation.Poor,
		"mixed": simulation.Mixed,
		"clean": simulation.Clean,
	}
)

type liveResult struct {
	Panel   panelView     `json:"panel"`
	Crossed []crossedView `json:"crossed"`
}

func (s *Server) live(ctx context.Context, tx pgx.Tx, actor command.Actor, in liveInput) (liveResult, error) {
	sleep, ok := sleeps[in.Sleep]
	if !ok {
		return liveResult{}, command.Invalid("%q is not a way to sleep", in.Sleep)
	}
	training, ok := trainings[in.Training]
	if !ok {
		return liveResult{}, command.Invalid("%q is not a way to train", in.Training)
	}
	diet, ok := diets[in.Diet]
	if !ok {
		return liveResult{}, command.Invalid("%q is not a way to eat", in.Diet)
	}

	// The day boundary is the player's own (§3.3), and the character is what
	// knows where they are. Two modules composed at the transport, which is
	// what the transport is for — neither reads the other's tables.
	who, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return liveResult{}, err
	}

	today := simulation.Today(s.clk, who.Zone)

	p, fx, err := s.health.Lived(ctx, tx, actor.ID, today,
		simulation.Day{Sleep: sleep, Training: training, Diet: diet})
	switch {
	case errors.Is(err, health.ErrAlreadyLived):
		// A day cannot be lived twice. The request is fine and it is the
		// calendar that is in the way, so it is a 409 (§8).
		return liveResult{}, command.Conflict("today has already been lived")
	case err != nil:
		return liveResult{}, err
	}

	out := liveResult{Panel: panel(p, today), Crossed: crossings(fx)}

	if err := healthLivedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		healthLived{Panel: out.Panel, Crossed: out.Crossed},
	); err != nil {
		return liveResult{}, err
	}
	return out, nil
}

type buyInput struct {
	Tier string `json:"tier"`
}

type buyResult struct {
	Panel panelView `json:"panel"`
	Tier  tierView  `json:"tier"`
	Spent int64     `json:"spent"`
	Money int64     `json:"money"`
}

/*
Buying instrumentation.

The money leaves through character.Wallet inside this command's transaction, so
there is no state in which the instrument exists unpaid for or the money is gone
without it. What the purchase changes is what the player can *see* — the same
body, the same numbers, the same history, now legible. §3.16 calls this a money
sink that buys insight rather than power, and it is the loop the economy exists
to serve.
*/
func (s *Server) buy(ctx context.Context, tx pgx.Tx, actor command.Actor, in buyInput) (buyResult, error) {
	p, spent, err := s.health.Buy(ctx, tx, actor.ID, in.Tier)
	switch {
	case errors.Is(err, health.ErrNoSuchTier):
		return buyResult{}, command.Invalid("there is no instrument called %q", in.Tier)
	case errors.Is(err, health.ErrAlreadyOwned):
		return buyResult{}, command.Conflict("you already have that")
	case errors.Is(err, character.ErrCannotAfford):
		// The world is in the way rather than the request being wrong: the same
		// command works once there is money for it.
		return buyResult{}, command.Conflict("that costs more than you have")
	case err != nil:
		return buyResult{}, err
	}

	who, err := s.characters.Of(ctx, tx, actor.ID)
	if err != nil {
		return buyResult{}, err
	}

	out := buyResult{
		Panel: panel(p, simulation.Today(s.clk, who.Zone)),
		Spent: spent.Cents(),
		Money: who.Money.Cents(),
	}
	for _, t := range out.Panel.Tiers {
		if t.Key == in.Tier {
			out.Tier = t
		}
	}

	if err := healthBoughtMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		healthBought{Panel: out.Panel, Tier: out.Tier, Spent: out.Spent, Money: out.Money},
	); err != nil {
		return buyResult{}, err
	}
	return out, nil
}
