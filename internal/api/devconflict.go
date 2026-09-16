package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/conflict"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The developer's toolbar, conversation half.

Four more commands under the same rule as dev.go's: **not registered at all**
outside a production binary, rather than guarded inside. Everything that file
says about why applies here unchanged, so this only says what is different.

# Why conversations need their own

A challenge is a thing you enter; a conversation is a thing that accumulates.
What is being proof-read in `content/conflicts` is rarely one moment — it is how
four of them read in a row against the same person, whether the second one still
makes sense once the first has moved the ledger, whether the standing the third
is selected out of is the standing the second left behind. None of that is
visible one day at a time, and the rollover produces at most one turn per person
per day (00032) precisely so that a lived history stays paced.

So these assemble one. `dev.conflict.raise` puts a named pathway to a named
person on the first day they are free, which means four presses build a
four-message thread you can read end to end in a minute rather than in four
days.

# What they cannot do

They cannot invent a moment, edit a line, or decide an answer. Every one of them
goes through [conflict.Conflicts.Force], which goes through the same `raise` the
rollover does — so a forced conversation is weighed against the real playbook,
writes a real fire record, moves the real standing and pays whatever its answer
pays. A pathway proof-read through this toolbar has been proof-read against the
game, which is the only reason a toolbar is worth having.

The dates are the one visible seam. A history assembled in a minute has its
messages stamped on consecutive days marching into the future, because the
one-per-person-per-day rule is a real rule and this does not break it. That
reads as slightly odd in the thread and it is the honest signal: this was
assembled, not lived.
*/

// devPathwayView is one authored pathway as somebody writing it needs to see
// it — the whole catalogue including populations the player is nowhere near,
// which is the opposite of what any player-facing surface may show.
type devPathwayView struct {
	Conflict string `json:"conflict"`
	// Among is the population it is authored for, and Name what it is called.
	Among string `json:"among"`
	Name  string `json:"name"`
	// Follows is the pressure it arises from, and Puts the one playbook question
	// it raises — the two things that decide when the game would ever select it.
	Follows string `json:"follows"`
	Puts    string `json:"puts"`
	// Line is the moment itself, with {who} still in it. Unfilled deliberately:
	// this is the authored string, and seeing the placeholder is how somebody
	// spots the pathway that forgot to name anybody.
	Line string `json:"line"`
	// Answers is the postures it is written for, by stance.
	Answers []string `json:"answers"`
	// With is who may send it, by person key. The toolbar offers a raise per
	// person, and before this field it offered all six for every pathway —
	// which is how a rota and a boiler man came to be sendable by the man
	// behind the till. Sent so the buttons that cannot work are not drawn,
	// rather than drawn and then refused.
	With []string `json:"with"`
	// After is what must already have passed with the same person, written the
	// way an author would read it back ("the-quick-hand", or "the-quick-hand
	// (yield)"). A pathway with anything here is a sequel, and the toolbar says
	// so rather than letting somebody wonder why the button did nothing.
	After []string `json:"after"`
	// Every is how many days before it comes round again, or nought for one
	// that is spent once it has been answered.
	Every int `json:"every"`
	// Here is whether this pathway belongs to the population the player is
	// living among — the ones `dev.conflict.raise` can actually put to them.
	Here bool `json:"here"`
}

// devPersonView is one neighbour and what stands between them and the player,
// which is what decides who the world would pick next.
type devPersonView struct {
	Person string `json:"person"`
	Name   string `json:"name"`
	// Pressure is [conflict.Standing.Pressure] — the number Turn sorts on, shown
	// because "why did it pick her again" is the question this toolbar exists to
	// answer.
	Pressure int `json:"pressure"`
	// Raise is every pathway that could be put to this person right now, by key
	// — theirs to send, and with whatever it waits on already behind them. The
	// toolbar draws buttons from this rather than from the catalogue, so a
	// moment that is somebody else's or a sequel to a conversation that has not
	// happened is simply not offered. The cadence is not consulted: something
	// said last week and not due back is still there to be re-read.
	Raise []string `json:"raise"`
	// Under is the pressures they are under, in the order the pathway pool is
	// searched. The first one with writing behind it is the one that fires.
	Under []string `json:"under"`
	// Met is how many conversations there have been, and Today whether they have
	// already had their turn.
	Met   int  `json:"met"`
	Today bool `json:"today"`
}

// devConversationResult is what all four answer with: the catalogue, who is
// around, and the two player surfaces the command just moved.
//
// The whole of it every time, for the reason devResult states — a toolbar
// holding a list that disagrees with the server about what is there is a
// toolbar that lies about the one thing it is for.
type devConversationResult struct {
	Pathways []devPathwayView `json:"pathways"`
	People   []devPersonView  `json:"people"`
	// Where is the population the player is living among, empty if none.
	Where string `json:"where"`
	// Conflicts is the player's own surface, so the client can drop it straight
	// into the store and what is waiting updates without a reload. The thread
	// itself is not sent: the toolbar listed the messages under its buttons for
	// a while, and it was a worse copy of what the phone screen already draws —
	// the state being proof-read is the one the player reads, so read it there.
	Conflicts conflictsView `json:"conflicts"`
	// Notice is one plain sentence for the developer reading it. Not
	// player-facing copy and not in the world's voice.
	Notice string `json:"notice"`
}

// devRaiseInput names one pathway and who to put it to. The person may be left
// out, in which case whoever is under the most pressure takes it — which is the
// world's own answer and usually the one wanted.
type devRaiseInput struct {
	Conflict string `json:"conflict"`
	Person   string `json:"person"`
}

// devPersonInput names one neighbour.
type devPersonInput struct {
	Person string `json:"person"`
}

/*
devConversationSurface composes the answer.

Read back after every change rather than predicted from it, exactly as
devSurface does: the toolbar shows what the server thinks happened and never
what the command believed it was about to do.
*/
func (s *Server) devConversationSurface(
	ctx context.Context, tx pgx.Tx, accountID, notice string,
) (devConversationResult, error) {
	c, err := s.characters.Ensure(ctx, tx, accountID)
	if err != nil {
		return devConversationResult{}, err
	}
	today := simulation.Today(s.clk, c.Zone)
	// The population, not the district. `among` is the same reading the module
	// itself takes through its Where seam, so the toolbar and the selection
	// never disagree about who "around here" means.
	home, err := s.among(ctx, tx, accountID)
	if err != nil {
		return devConversationResult{}, err
	}

	out := devConversationResult{Notice: notice, Where: home}
	for _, a := range conflict.All() {
		view := devPathwayView{
			Conflict: a.Key,
			Among:    a.Among,
			Name:     a.Name,
			Follows:  string(a.Follows),
			Puts:     a.Puts,
			Line:     conflict.Said(a.Line),
			Here:     a.Among == home,
			Answers:  []string{},
			With:     a.With,
			After:    after(a),
			Every:    a.Every,
		}
		for _, ans := range a.Answers() {
			view.Answers = append(view.Answers, string(ans.Stance))
		}
		out.Pathways = append(out.Pathways, view)
	}

	// Who is around, and how overdue each of them is. Straight off the same
	// standing the selection reads, so the numbers on screen are the numbers
	// Turn sorted on rather than a second opinion about them.
	if among, live := environment.Lookup(home); live {
		standings, err := s.conflicts.Standings(ctx, tx, accountID, home)
		if err != nil {
			return devConversationResult{}, err
		}
		// And what each of them could actually be put right now, so the strip
		// draws the buttons that work rather than all of them.
		raisable, err := s.conflicts.Raisable(ctx, tx, accountID, home, today)
		if err != nil {
			return devConversationResult{}, err
		}
		for _, p := range among.People() {
			// Read as of today, because a month-old ask count is not a pressure
			// anybody is under any more — the same reading the raise takes, so
			// the strain on screen is the strain the button was drawn from.
			st := standings[p.Key].Fresh(today)
			st.Person = p.Key
			view := devPersonView{
				Person:   p.Key,
				Name:     p.Name,
				Pressure: st.Pressure(today),
				Met:      st.Met,
				Today:    st.Last == today,
				Under:    []string{},
				Raise:    raisable[p.Key],
			}
			for _, strain := range st.Under() {
				view.Under = append(view.Under, string(strain))
			}
			out.People = append(out.People, view)
		}
	}

	if out.Conflicts, err = s.conflictSurface(ctx, tx, accountID, today); err != nil {
		return devConversationResult{}, err
	}
	return out, nil
}

// Listing every pathway there is to put, and who there is to put it to.
func (s *Server) devConversations(
	ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{},
) (devConversationResult, error) {
	return s.devConversationSurface(ctx, tx, actor.ID, "")
}

// senders is everybody in a population whose moment this pathway is.
//
// Never empty for a shipped build: `with:` is required, non-empty, and checked
// against this very population at boot — so a pathway authored for the
// environment the player lives among always has somebody who could send it.
func senders(among environment.Environment, a conflict.Authored) []environment.Person {
	var out []environment.Person
	for _, who := range among.People() {
		if a.Sends(who.Key) {
			out = append(out, who)
		}
	}
	return out
}

// after is what a pathway waits on, as an author would read it back.
func after(a conflict.Authored) []string {
	out := make([]string, 0, len(a.After))
	for _, need := range a.After {
		out = append(out, need.String())
	}
	return out
}

// reached narrows a list of people to the ones this pathway could actually be
// put to right now, off [conflict.Conflicts.Raisable] — theirs to send, and
// with whatever it waits on already behind them.
//
// The second half is the one that is easy to leave out and the one that made
// the button look broken: a sequel belongs to everybody in its `with:`, but it
// has only been unlocked by whoever answered the conversation it follows. Sort
// the whole `with:` and the most overdue of them takes it, which is a refusal
// even though somebody standing right there was ready for it.
func reached(all []environment.Person, raisable map[string][]string, key string) []environment.Person {
	var out []environment.Person
	for _, who := range all {
		for _, k := range raisable[who.Key] {
			if k == key {
				out = append(out, who)
				break
			}
		}
	}
	return out
}

/*
Putting one named conversation to one named person, now.

Two steps, and the first is the one that would otherwise make the button look
broken: whatever is already waiting is withdrawn, because [conflict.Conflicts.hold]
would take this back the moment it landed and the toolbar would report success
and show nothing. The notice says so — the same bargain devEnable makes about a
challenge that had already been played, and for the same reason. Nobody presses
"raise" wanting to be told to press something else first.

With no person named, whoever is under the most pressure takes it. That is the
world's own answer, which is what somebody checking "would this ever reach the
right person" actually wants.
*/
func (s *Server) devConflictRaise(
	ctx context.Context, tx pgx.Tx, actor command.Actor, in devRaiseInput,
) (devConversationResult, error) {
	key := strings.TrimSpace(in.Conflict)
	if key == "" {
		return devConversationResult{}, command.Invalid("a conflict has to be named")
	}
	c, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return devConversationResult{}, err
	}
	home, err := s.among(ctx, tx, actor.ID)
	if err != nil {
		return devConversationResult{}, err
	}
	among, live := environment.Lookup(home)
	if !live {
		return devConversationResult{}, command.Conflict(
			"nobody lives around here — move somewhere with a population first")
	}
	authored, written := conflict.Pathway(home, key)
	if !written {
		return devConversationResult{}, command.Conflict(
			"%q is not authored for %s", key, home)
	}

	today := simulation.Today(s.clk, c.Zone)
	person := strings.TrimSpace(in.Person)
	if person == "" {
		standings, err := s.conflicts.Standings(ctx, tx, actor.ID, home)
		if err != nil {
			return devConversationResult{}, err
		}
		// Whoever is most overdue *among the people this could be put to now*.
		// The filter is the whole fix: a pathway belongs to people, and to
		// people whose thread has got to it, so putting the question to the
		// population at large and taking whoever came out on top would pick
		// somebody who cannot send it — and every press with nobody selected
		// would come back a refusal even where a ready sender was standing
		// right there. Sorting the ones who can is still the world's own
		// answer, which is what "no person named" is asking for.
		raisable, err := s.conflicts.Raisable(ctx, tx, actor.ID, home, today)
		if err != nil {
			return devConversationResult{}, err
		}
		pool := reached(senders(among, authored), raisable, key)
		who, _, anybody := conflict.Turn(pool, standings, actor.ID, today)
		if !anybody {
			// A refusal per mistake, for the reason [conflict.Conflicts.Force]
			// gives: "she would never say this", "not yet" and "no reason to"
			// are three different problems and want three different fixes.
			who := strings.Join(authored.With, ", ")
			switch {
			case len(senders(among, authored)) == 0:
				return devConversationResult{}, command.Conflict(
					"nobody around here sends %q — it is %s's", key, who)
			case len(authored.After) > 0:
				return devConversationResult{}, command.Conflict(
					"nobody around here has got to %q yet — it comes after %s, and none "+
						"of %s has been there", key, strings.Join(after(authored), ", "), who)
			}
			return devConversationResult{}, command.Conflict(
				"%q follows %s, and nothing between you and %s is %s yet",
				key, authored.Follows, who, authored.Follows)
		}
		person = who.Key
	}

	withdrew, err := s.conflicts.Withdraw(ctx, tx, actor.ID)
	if err != nil {
		return devConversationResult{}, err
	}

	arisen, err := s.conflicts.Force(ctx, tx, actor.ID, person, key, today)
	switch {
	// A moment this person could not send, or could not send yet. A refusal the
	// author can act on — it names who it belongs to, or which conversation has
	// to happen first — and it must reach the strip as words. Unmapped it is a
	// 500, and the toolbar says the command could not be executed, which reads
	// as the button being broken rather than as the ordering rule holding.
	case errors.Is(err, conflict.ErrOutOfTurn):
		return devConversationResult{}, command.Conflict("%s", strings.TrimPrefix(
			err.Error(), "conflict: out of turn: "))
	case errors.Is(err, conflict.ErrNoSuchPerson):
		return devConversationResult{}, command.Conflict("%q is not among %s", person, home)
	case err != nil:
		return devConversationResult{}, err
	case !arisen.Arose():
		return devConversationResult{}, command.Conflict(
			"%q did not land — %s may already have one on every day from today", key, person)
	}

	notice := fmt.Sprintf("%s put %q to %s", authored.Name, key, person)
	switch {
	case arisen.Deferred && arisen.Fallback != "":
		// The ordinary case for the toolbar: you pressed the button, so you are
		// here, so it is yours to answer. Naming what the rules would have done
		// is the useful half — it is what you are checking the writing against,
		// and what would take it if you walked away.
		notice += fmt.Sprintf(" — it is on your phone (your rules would have said %s)", arisen.Fallback)
	case arisen.Deferred:
		notice += " — it is on your phone, and your rules had no answer for it"
	case arisen.Waiting:
		notice += fmt.Sprintf(" — it is waiting on you (%s)", arisen.Cause)
	case arisen.Handled != "":
		notice += fmt.Sprintf(" — your rules answered it: %s", arisen.Handled)
	}
	if withdrew != "" {
		notice += fmt.Sprintf(". %s was waiting and has been withdrawn", withdrew)
	}
	return s.devConversationSurface(ctx, tx, actor.ID, notice)
}

/*
Running the day the world would have run, now.

The real thing: [conflict.Conflicts.Day] against today, which walks the
population, stops at the first conflict the player's rules could not settle, and
produces however many that comes to. It is the one command here that tests the
*selection* rather than the writing — who turns up, in what order, out of which
pressure — and the only way to see it without waiting for a rollover.

A day already walked produces nothing, which is the one-per-person-per-day rule
doing its job rather than a failure; the notice says so instead of leaving the
button looking dead.
*/
func (s *Server) devConflictDay(
	ctx context.Context, tx pgx.Tx, actor command.Actor, _ struct{},
) (devConversationResult, error) {
	c, err := s.characters.Ensure(ctx, tx, actor.ID)
	if err != nil {
		return devConversationResult{}, err
	}
	arisen, err := s.conflicts.Day(ctx, tx, actor.ID, simulation.Today(s.clk, c.Zone))
	if err != nil {
		return devConversationResult{}, err
	}

	notice := "the day produced nothing — everybody has had their turn today, or something is already waiting"
	if len(arisen) > 0 {
		said := make([]string, 0, len(arisen))
		for _, a := range arisen {
			line := fmt.Sprintf("%s with %s", a.Conflict, a.Person)
			switch {
			case a.Deferred:
				line += " (yours to answer)"
			case a.Waiting:
				line += " (waiting)"
			case a.Handled != "":
				line += fmt.Sprintf(" (%s)", a.Handled)
			}
			said = append(said, line)
		}
		notice = fmt.Sprintf("%d conflict(s): %s", len(arisen), strings.Join(said, "; "))
	}
	return s.devConversationSurface(ctx, tx, actor.ID, notice)
}

/*
Putting a whole conversation back to never having happened.

Both halves — the messages and the standing they moved — see
[conflict.Conflicts.Forget] for why the second is not optional. This is the
command that makes the toolbar a loop rather than a ratchet: proof-read four
messages, forget them, fix the second one's wording, read the four again from a
clean ledger.

What it does not undo is anything the answers paid, exactly as dev.reset does
not. The world moved, and this is not a time machine.
*/
func (s *Server) devConflictForget(
	ctx context.Context, tx pgx.Tx, actor command.Actor, in devPersonInput,
) (devConversationResult, error) {
	person := strings.TrimSpace(in.Person)
	if person == "" {
		return devConversationResult{}, command.Invalid("a person has to be named")
	}
	home, err := s.among(ctx, tx, actor.ID)
	if err != nil {
		return devConversationResult{}, err
	}
	gone, err := s.conflicts.Forget(ctx, tx, actor.ID, home, person)
	if err != nil {
		return devConversationResult{}, err
	}

	notice := fmt.Sprintf("%s is back to never having happened (%d message(s) gone)", person, gone)
	if gone == 0 {
		notice = fmt.Sprintf("%s had nothing to forget; their ledger is clear", person)
	}
	return s.devConversationSurface(ctx, tx, actor.ID, notice)
}
