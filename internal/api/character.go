package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The character's commands: going somewhere, and taking a paid action.

// placeView is one place, and every way out of it, as the client draws them.
// The city crosses the wire rather than living in the client, and the ways go
// with it: a portal drawn in a location is only the placement of a way.
type placeView struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Note  string `json:"note"`
	Layer string `json:"layer"`
	// In is the district this place sits inside; empty for a district.
	In string `json:"in"`
	// Stop is whether the transit network calls here.
	Stop bool      `json:"stop"`
	Ways []wayView `json:"ways"`
}

// wayView is one step out of a place: where it goes, how, and what it costs.
type wayView struct {
	To   string `json:"to"`
	Mode string `json:"mode"`
	Says string `json:"says"`
	Fare int64  `json:"fare"`
}

func place(p city.Place) placeView {
	out := placeView{
		Key: p.Key, Name: p.Name, Note: p.Note,
		Layer: string(p.Layer), In: p.In, Stop: p.Stop,
		Ways: []wayView{},
	}
	for _, w := range city.WaysFrom(p.Key) {
		out.Ways = append(out.Ways, wayView{
			To: w.To, Mode: string(w.Mode), Says: w.Name, Fare: w.Fare.Cents(),
		})
	}
	return out
}

// characterView is the character as the client holds it.
//
// Money is cents, and stays cents all the way to the screen: formatting it is
// presentation, and a float on the wire would arrive rounded.
type characterView struct {
	Money int64     `json:"money"`
	At    placeView `json:"at"`
	Home  placeView `json:"home"`
	// The timezone the day boundary is measured in (§3.3). The client needs it
	// back because the character's clock is what decides night and day on the
	// canvas — and it is the character's, not the device's: the stored zone is
	// the one the rollover runs against.
	Timezone string `json:"timezone"`
	// The derived half of §3.4's cap, computed on read at the moment the
	// response is written and never stored anywhere (§3.3).
	EarningsTaken int `json:"earningsTaken"`
	EarningsLeft  int `json:"earningsLeft"`
}

func (s *Server) view(c character.Character) characterView {
	earnings := c.Earnings(s.clk)
	return characterView{
		Money:         c.Money.Cents(),
		At:            place(c.At),
		Home:          place(c.Home),
		Timezone:      c.Zone.String(),
		EarningsTaken: earnings.Taken,
		EarningsLeft:  earnings.Left,
	}
}

// What the player's other tabs are told. Two messages rather than one
// "character changed", because §10.2 asks every socket message to be a
// deliberate decision about a piece of the world rather than a diff.
type characterMoved struct {
	At placeView `json:"at"`
	// How they got there, and what it cost them — so a tab that did not send
	// the command can say "you took the Crosstown" rather than only redrawing
	// a place name.
	By    string `json:"by"`
	Money int64  `json:"money"`
}

var characterMovedMessage = realtime.Define[characterMoved]("character.moved")

type moveInput struct {
	// To is a place key from anywhere in internal/city — a street, a shop, or
	// the player's own kitchen. There is one command for all of §3.12's layers
	// because there is one question being asked: take me there.
	To string `json:"to"`
	// By is "foot" or "transit". Required rather than defaulted: one of them
	// costs money, and a client that forgot to say which it meant must not have
	// the server pick the one with a fare on it.
	By string `json:"by"`
}

// stepView is one leg of a journey, as the client reads it back.
type stepView struct {
	To   string `json:"to"`
	Mode string `json:"mode"`
	Says string `json:"says"`
}

// tripView is the journey that was made: what it cost, and how many places it
// passed through where something could have happened. `chances` is zero for a
// ride, always.
type tripView struct {
	Character characterView `json:"character"`
	By        string        `json:"by"`
	Steps     []stepView    `json:"steps"`
	Fare      int64         `json:"fare"`
	Chances   int           `json:"chances"`
}

// move takes the character somewhere, on foot or by transit. Neither mode
// costs game time; what separates them is a fare and what happens on the way.
func (s *Server) move(ctx context.Context, tx pgx.Tx, actor command.Actor, in moveInput) (tripView, error) {
	by, ok := city.ModeOf(in.By)
	if !ok {
		return tripView{}, command.Invalid(
			"%q is not a way of getting anywhere — it is %q or %q", in.By, city.OnFoot, city.ByTransit)
	}

	trip, err := s.characters.Travel(ctx, tx, actor.ID, in.To, by)
	switch {
	case errors.Is(err, character.ErrNoSuchPlace):
		// Wrong forever, however often it is sent: there is no such place.
		return tripView{}, command.Invalid("there is nowhere called %q", in.To)
	case errors.Is(err, character.ErrNoWay) && by == city.ByTransit:
		// The world is in the way rather than the request: walk out to a stop
		// and the identical command works, which is what a 409 means (§8).
		return tripView{}, command.Conflict(
			"the network does not run between here and there — walk out to a stop first")
	case errors.Is(err, character.ErrNoWay):
		return tripView{}, command.Conflict("there is no way through to there from here")
	case errors.Is(err, character.ErrCannotAfford):
		return tripView{}, command.Conflict(
			"the fare is %s and you do not have it — walking is free", spent(city.Fare))
	case err != nil:
		return tripView{}, err
	}

	out := tripView{
		Character: s.view(trip.Character),
		By:        string(trip.By),
		Steps:     []stepView{},
		Fare:      trip.Fare.Cents(),
		Chances:   trip.Chances,
	}
	for _, step := range trip.Steps {
		out.Steps = append(out.Steps, stepView{
			To: step.To.Key, Mode: string(step.Mode), Says: step.Says,
		})
	}

	if err := characterMovedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		characterMoved{At: place(trip.Character.At), By: out.By, Money: out.Character.Money},
	); err != nil {
		return tripView{}, err
	}
	// A fare is money no longer held, and where the player now stands is a
	// different environment to wear out (§3.5, §3.15).
	if err := s.stood(ctx, tx, actor.ID); err != nil {
		return tripView{}, err
	}
	return out, nil
}

// spent writes an amount the way a refusal has to read it.
//
// Money is cents everywhere else and formatting is the client's job (§10.2) —
// but a refusal is a sentence, and a sentence with 275 in it is not one.
func spent(m simulation.Money) string {
	return fmt.Sprintf("$%d.%02d", m.Cents()/100, m.Cents()%100)
}
