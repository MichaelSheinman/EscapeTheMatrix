package api

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The character's commands: walking somewhere, and taking a paid action.
//
// Both are thin on purpose. The transport decodes and maps errors to status
// codes, internal/character owns the table, and internal/simulation owns the
// rules — this file is the seam, and the fact that it has almost nothing in it
// is the point of the seam being here.

// placeView is a district as the client draws it.
//
// The city's vocabulary crosses the wire rather than living in the client,
// because the district list is authored content (§17 leaves it open) and a
// client with its own copy would show yesterday's city after a deploy.
type placeView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
}

func place(p character.Place) placeView {
	return placeView{Key: p.Key, Name: p.Name, Note: p.Note}
}

// characterView is the character as the client holds it.
//
// Money is cents, and stays cents all the way to the screen: formatting it is
// presentation, and a float on the wire would arrive rounded.
type characterView struct {
	Money int64     `json:"money"`
	At    placeView `json:"at"`
	Home  placeView `json:"home"`
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
		EarningsTaken: earnings.Taken,
		EarningsLeft:  earnings.Left,
	}
}

// What the player's other tabs are told. Two messages rather than one
// "character changed", because §10.2 asks every socket message to be a
// deliberate decision about a piece of the world rather than a diff.
type characterMoved struct {
	At placeView `json:"at"`
}

type characterEarned struct {
	Money  int64 `json:"money"`
	Earned int64 `json:"earned"`
	// Both halves of the cap, so the tab hearing about this states what the
	// server said rather than incrementing a number of its own (§10.2).
	EarningsTaken int `json:"earningsTaken"`
	EarningsLeft  int `json:"earningsLeft"`
}

var (
	characterMovedMessage  = realtime.Define[characterMoved]("character.moved")
	characterEarnedMessage = realtime.Define[characterEarned]("character.earned")
)

type moveInput struct {
	District string `json:"district"`
}

// move walks the character to a district.
//
// §3.12: the player walks between locations, and walking is the encounter
// surface rather than a wait — which is why this is a command with a place in
// it rather than a camera movement the client could do on its own.
func (s *Server) move(ctx context.Context, tx pgx.Tx, actor command.Actor, in moveInput) (characterView, error) {
	c, err := s.characters.MoveTo(ctx, tx, actor.ID, in.District)
	switch {
	case errors.Is(err, character.ErrNoSuchPlace):
		// Wrong forever, however often it is sent: there is no such place.
		return characterView{}, command.Invalid("there is no district called %q", in.District)
	case err != nil:
		return characterView{}, err
	}

	if err := characterMovedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		characterMoved{At: place(c.At)},
	); err != nil {
		return characterView{}, err
	}
	return s.view(c), nil
}

// dayRate is what one paid action pays until jobs exist.
//
// §3.4 makes income structural — job tier, performance and assets — and the
// tiers that decide this number are #27 and #28. One rate in one place until
// then, rather than a number scattered through handlers.
const dayRate = simulation.Money(4500) // $45

type workResult struct {
	Character characterView `json:"character"`
	Earned    int64         `json:"earned"`
}

// work takes a paid action, if the player has one left today.
//
// The refusal is a 409 rather than a 422: the request is perfectly well formed,
// and it is the world — specifically the player's own day — that is in the way.
// Tomorrow the identical request works, which is exactly what 409 means (§8).
func (s *Server) work(ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{}) (workResult, error) {
	done, err := s.characters.Work(ctx, tx, actor.ID, dayRate)
	switch {
	case errors.Is(err, character.ErrDayIsSpent):
		return workResult{}, command.Conflict(
			"that is %d paid actions today — the rest of the day is yours, but it does not pay",
			simulation.PaidActionsPerDay)
	case err != nil:
		return workResult{}, err
	}

	view := s.view(done.Character)
	if err := characterEarnedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		characterEarned{
			Money:         view.Money,
			Earned:        done.Earned.Cents(),
			EarningsTaken: view.EarningsTaken,
			EarningsLeft:  view.EarningsLeft,
		},
	); err != nil {
		return workResult{}, err
	}
	return workResult{Character: view, Earned: done.Earned.Cents()}, nil
}
