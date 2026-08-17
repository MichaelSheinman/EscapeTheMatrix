package api

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/character"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
Where you live, on the wire (§3.15's relocation). What makes somewhere else
different is never sent: a player about to move is told the address, the district
and what somebody said about it, and nothing about how the people there behave.
Arriving is the moment your old answers fail rather than the moment they are
handed over, and a payload carrying the destination's holds would let a careful
player write the second rule set before going.

So holds, people and affords cross this boundary for one environment only — the
one the player already lives in, through internal/api/conflict.go. Somewhere else
is a name, a note off the map, and a door.
*/

// homeView is one place a player may live. The note is internal/city's — what
// the map says about that part of town, which is public knowledge. Nothing here
// is the population's.
type homeView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Note string `json:"note"`
	// District is the key of the part of town it is in, and Where is what that
	// district is called. The first is the environment key a rule set is filed
	// under (§3.15), which is why it is sent rather than derived.
	District string `json:"district"`
	Where    string `json:"where"`
	// Yours is whether this is the one they live in now.
	Yours bool `json:"yours"`
}

// saidView is §3.15's recommendation: somebody here, on somewhere else. Sent
// once the decay has said it first ([simulation.Elsewhere]), and carrying no
// reason: the line suggests the trip and never explains the lesson.
type saidView struct {
	// Who said it, as a player reads their name.
	Who  string `json:"who"`
	Says string `json:"says"`
	// Where they were talking about, as a home key, so the offer to move can be
	// the same object as the sentence that caused it.
	Home string `json:"home"`
}

// livingView is where-you-live for one player. movedToday is sent rather than
// worked out by the client, because the day it is measured against is the
// player's own and the client does not hold their timezone. Computed when the
// response is written and stored nowhere (§3.3).
type livingView struct {
	Homes      []homeView `json:"homes"`
	Said       *saidView  `json:"said"`
	MovedToday bool       `json:"movedToday"`
}

// living composes where the player lives and everywhere else they could.
//
// The standing is passed in rather than read again because the recommendation's
// timing is a fact about decay, and the snapshot has already composed it (§3.3:
// a reading is composed once per read, not once per field).
func living(c character.Character, s factor.Standing, clk simulation.Clock) livingView {
	out := livingView{Homes: []homeView{}, MovedToday: c.Settled(clk)}

	for _, h := range city.Homes() {
		out.Homes = append(out.Homes, homeView{
			Key: h.Key, Name: h.Name, Note: h.Note,
			District: h.In, Where: districtName(h.In),
			Yours: h.Key == c.Home.Key,
		})
	}

	// Only the population they live among has anything to say, and only once it
	// has stopped paying. A player among nobody hears nothing, because there is
	// no here for a line to come from.
	here, ok := environment.Lookup(s.Where)
	if !ok || !simulation.Elsewhere(s.Familiarity) {
		return out
	}
	who, ok := here.Person(here.Elsewhere.Who)
	if !ok {
		return out
	}
	there, ok := city.HomeIn(here.Elsewhere.Where)
	if !ok {
		return out
	}
	out.Said = &saidView{Who: who.Name, Says: here.Elsewhere.Says, Home: there.Key}
	return out
}

// districtName is what the map calls a part of town, or its key if the city no
// longer has one by that name.
func districtName(key string) string {
	if p, ok := city.Lookup(key); ok {
		return p.Name
	}
	return key
}

// homeMoved is what the player's other tabs are told when they move house. home
// is a place rather than a home, so a receiving tab stores it where
// character.moved stores an arrival. living travels with it because which
// address is theirs has changed, and a tab holding the old answer would offer
// them rooms they are already in.
type characterRelocated struct {
	Home      placeView     `json:"home"`
	Living    livingView    `json:"living"`
	Conflicts conflictsView `json:"conflicts"`
	// From is where they lived, so a tab that did not send the command can say
	// what happened rather than only what is true now.
	From string `json:"from"`
}

var characterRelocatedMessage = realtime.Define[characterRelocated]("character.relocated")

type relocateInput struct {
	// To is a home key. Not a district: a district is where a population is,
	// and what a player takes is an address inside it.
	To string `json:"to"`
}

type relocatedView struct {
	Character characterView `json:"character"`
	Living    livingView    `json:"living"`
	// Conflicts is who the player lives among now.
	//
	// It rides on this response because moving is the one command whose whole
	// effect is on a screen other than the one that sent it: the button is at
	// the bottom of the people you live among, and pressing it makes everything
	// above it somebody else. A response that carried only the address would
	// leave the player reading their old neighbours under their new landing
	// until they thought to reload (§10.2, rule 2).
	Conflicts conflictsView `json:"conflicts"`
	// Moved is false when they already lived there, which is a real answer and
	// not a refusal (§8: a retry has to be repeatable).
	Moved bool `json:"moved"`
}

/*
relocate makes somewhere else home (§3.15).

Deliberately not a journey. Travelling is `character.move` and changes where the
player is standing; this changes who they live among, and the two are separate
commands because they are separate decisions — the whole pressure of §3.15
depends on the population not following the player's feet.

It costs no money and asks for nothing. What it costs is that the rule set built
for the last place is not in force here, and no charge could be a better teacher
than that.
*/
func (s *Server) relocate(ctx context.Context, tx pgx.Tx, actor command.Actor, in relocateInput) (relocatedView, error) {
	move, err := s.characters.Relocate(ctx, tx, actor.ID, in.To)
	switch {
	case errors.Is(err, character.ErrNoSuchPlace):
		return relocatedView{}, command.Invalid("there is nowhere called %q", in.To)
	case errors.Is(err, character.ErrNobodyLivesThere):
		// Wrong forever: the place is real and is not somewhere to live.
		return relocatedView{}, command.Invalid("nobody lives at %q", in.To)
	case errors.Is(err, character.ErrMovedToday):
		// The world is in the way rather than the request — the same command
		// works tomorrow, which is what a 409 means (§8).
		return relocatedView{}, command.Conflict("you have already moved once today")
	case err != nil:
		return relocatedView{}, err
	}

	standing, err := s.factors.Of(ctx, tx, actor.ID)
	if err != nil {
		return relocatedView{}, err
	}
	// After the move, so it is the new population and not the old one — the
	// whole reason this is on the response.
	among, err := s.conflictSurface(ctx, tx, actor.ID, simulation.Today(s.clk, move.Character.Zone))
	if err != nil {
		return relocatedView{}, err
	}
	out := relocatedView{
		Character: s.view(move.Character),
		Living:    living(move.Character, standing, s.clk),
		Conflicts: among,
		Moved:     move.Moved(),
	}
	if !out.Moved {
		return out, nil
	}

	if err := characterRelocatedMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		characterRelocated{Home: place(move.To), Living: out.Living, Conflicts: among, From: move.From.Name},
	); err != nil {
		return relocatedView{}, err
	}
	// A different population is a different set of ceilings on the same reading
	// (§3.15), and a different amount of wear under it. Nothing about the player
	// changed; what they are read against did.
	if err := s.stood(ctx, tx, actor.ID); err != nil {
		return relocatedView{}, err
	}
	return out, nil
}
