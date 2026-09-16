package api

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/account"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/command"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/factor"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/realtime"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/social"
)

/*
Peer endorsement on the wire (§3.5).

Nothing here names another player as an argument. social.endorse takes an offer
id and nothing else, so "you may only like who you were offered" is a shape
rather than a check, and the leaderboards can go on withholding account ids.

The endorser learns nothing about the person they endorsed. Their response
carries their own standing; the recipient's moved Karma is published to the
recipient over the socket. internal/social cannot do otherwise, because the
interface it hands a grant through returns an error rather than a standing.

And the player being liked does nothing: no request, no presence, no reply. That
is why endorsement can ship in V1, and the socket message below is one of the
four things §8 says the socket exists for.
*/

// personView is another player, as far as this surface is allowed to know them.
// A name and a picture, the same pair a leaderboard row carries, and no account
// id: an offer is answered by the offer's id, so nothing here has to address
// anybody.
type personView struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

func person(p social.Player) personView {
	return personView{Name: p.Name, Avatar: p.Avatar}
}

func people(ps []social.Player) []personView {
	out := make([]personView, 0, len(ps))
	for _, p := range ps {
		out = append(out, person(p))
	}
	return out
}

// offerView is somebody to like, and until when.
type offerView struct {
	// ID is what an answer names. Meaningless to anybody else: every read of an
	// offer is keyed by (id, account), so guessing one belonging to another
	// player is refused exactly as guessing one that never existed is.
	ID  int64      `json:"id"`
	Who personView `json:"who"`
	// Until is when the offer lapses, so the screen can say so rather than
	// having the button quietly stop working.
	Until string `json:"until"`
}

// endorsementView is the whole endorsement surface for one player. offer is
// null most of the time, and opens is sent beside it to say when it comes back.
type endorsementView struct {
	Offer *offerView `json:"offer"`
	Given int        `json:"given"`
	Cap   int        `json:"cap"`
	// Opens is when the next offer may be drawn, RFC3339 — empty while one is
	// standing, and empty when one may be drawn the moment they look.
	Opens string `json:"opens"`
	// Received is how many people have said they like this player, ever, and
	// Liked is the most recent few by name. A count of people rather than a
	// factor: nothing here says what being liked is worth.
	Received int          `json:"received"`
	Liked    []personView `json:"liked"`
}

func endorsement(s social.Standing) endorsementView {
	out := endorsementView{
		Given:    s.Given,
		Cap:      s.Cap,
		Opens:    stamp(s.Opens),
		Received: s.Received,
		Liked:    people(s.Liked),
	}
	if s.Offer != nil {
		out.Offer = &offerView{
			ID:    s.Offer.ID,
			Who:   person(s.Offer.Who),
			Until: stamp(s.Offer.ExpiresAt),
		}
	}
	return out
}

// endorsementOffered is what a player's tabs are told when the game names
// somebody for them. Published when an offer is drawn — at most once every
// [social.Cadence] — rather than on every world read. No Origin, and receiving
// it twice restates the same offer.
type socialOffered struct {
	Endorsement endorsementView `json:"endorsement"`
}

var socialOfferedMessage = realtime.Define[socialOffered]("social.offered")

// What a player's *other* tabs are told when this one answered an offer.
type socialAnswered struct {
	Endorsement endorsementView `json:"endorsement"`
}

var socialAnsweredMessage = realtime.Define[socialAnswered]("social.answered")

// endorsementLiked is what the player who was liked is told. They are told who,
// by name, and they do not have to do anything about it.
type socialLiked struct {
	By personView `json:"by"`
	// The recipient's whole surface, restated, so their client stores what the
	// server says rather than incrementing a count of its own (§10.2).
	Endorsement endorsementView `json:"endorsement"`
}

var socialLikedMessage = realtime.Define[socialLiked]("social.liked")

// answerInput is the whole of what either command takes: which offer. No player
// field, because a command that named somebody would let a client choose who to
// endorse, and "occasional, not on demand" would become a rate limiter in front
// of a search (§3.5).
type answerInput struct {
	Offer int64 `json:"offer"`
}

type answerResult struct {
	Endorsement endorsementView `json:"endorsement"`
}

// endorse says you like the player you were offered. Every refusal is 409 rather
// than 422: the request is well formed and the world is in the way. An offer that
// lapsed while the screen was open, one another tab already answered, and a
// weekly cap already spent were all true a moment ago and may be true again.
func (s *Server) endorse(ctx context.Context, tx pgx.Tx, actor command.Actor, in answerInput) (answerResult, error) {
	if in.Offer <= 0 {
		// The one refusal that *is* permanent: there is no such thing as offer
		// zero, however long the client waits.
		return answerResult{}, command.Invalid("an endorsement names the offer it answers")
	}

	standing, liked, err := s.social.Endorse(ctx, tx, actor.ID, in.Offer)
	switch {
	case errors.Is(err, social.ErrNoOffer):
		return answerResult{}, command.Conflict("that offer is no longer open")
	case errors.Is(err, social.ErrCapped):
		return answerResult{}, command.Conflict("that is as many as you may give for now")
	case errors.Is(err, social.ErrAlready):
		return answerResult{}, command.Conflict("you have already said you like that player")
	case err != nil:
		return answerResult{}, err
	}

	out := answerResult{Endorsement: endorsement(standing)}

	if err := socialAnsweredMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		socialAnswered(out),
	); err != nil {
		return answerResult{}, err
	}

	// The other player. Composed here rather than inside internal/social,
	// because it is a second account's view and the module that made the grant
	// is deliberately unable to read anything back about them.
	theirs, err := s.social.Of(ctx, tx, liked.To)
	if err != nil {
		return answerResult{}, err
	}
	// No Origin: they did not act. Every tab they have open hears it.
	if err := socialLikedMessage.Publish(ctx, tx,
		realtime.To{Account: liked.To},
		socialLiked{By: person(liked.From), Endorsement: endorsement(theirs)},
	); err != nil {
		return answerResult{}, err
	}

	// Their composite moved, and nothing in this command's response says so —
	// which is exactly the case factor.stood exists for. Note whose: the
	// endorser's own standing is untouched, because an endorsement pays the
	// player who was liked and nobody else (§7).
	if err := s.stood(ctx, tx, liked.To); err != nil {
		return answerResult{}, err
	}
	return out, nil
}

/*
Letting the offer go.

It writes nothing against the candidate and tells them nothing — passing is not
a judgement anybody is notified of — and it does not buy a replacement, because
the next offer comes on the cadence either way. What it buys is not having to
press a button you mean nothing by, which is the difference between a signal and
a habit.
*/
func (s *Server) passOffer(ctx context.Context, tx pgx.Tx, actor command.Actor, in answerInput) (answerResult, error) {
	if in.Offer <= 0 {
		return answerResult{}, command.Invalid("passing names the offer it declines")
	}

	standing, err := s.social.Pass(ctx, tx, actor.ID, in.Offer)
	switch {
	case errors.Is(err, social.ErrNoOffer):
		return answerResult{}, command.Conflict("that offer is no longer open")
	case err != nil:
		return answerResult{}, err
	}

	out := answerResult{Endorsement: endorsement(standing)}
	if err := socialAnsweredMessage.Publish(ctx, tx,
		realtime.To{Account: actor.ID, Origin: actor.Session},
		socialAnswered(out),
	); err != nil {
		return answerResult{}, err
	}
	return out, nil
}

/*
looked is the world snapshot's half: read the surface, and offer somebody if the
game owes them one.

A read that may write, like the character row the same snapshot creates. It is
the only trigger there is — no job hands offers to people who are not playing —
which is what keeps the cadence honest: a fortnight away does not accrue a
fortnight of strangers, because an offer is something to answer now rather than a
resource to bank.
*/
func (s *Server) looked(ctx context.Context, tx pgx.Tx, accountID string) (endorsementView, error) {
	standing, drawn, err := s.social.Look(ctx, tx, accountID)
	if err != nil {
		return endorsementView{}, err
	}

	out := endorsement(standing)
	if drawn {
		if err := socialOfferedMessage.Publish(ctx, tx,
			realtime.To{Account: accountID}, socialOffered{Endorsement: out}); err != nil {
			return endorsementView{}, err
		}
	}
	return out, nil
}

/*
crowd answers social.Crowd: who may be offered, and what they are called.

The adapter §3.1 asks for, in the layer that is allowed to compose two modules.
It is the same pair of questions internal/factor's roster asks, deliberately
answered through a second interface rather than a shared one: a leaderboard and
an endorsement want different things from the same list — one ranks everybody,
the other has to be able to stop naming somebody — and a shared type is how one
of those quietly acquires the other's rules.
*/
type crowd struct{ accounts *account.Accounts }

func (c crowd) Everyone(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return c.accounts.Everyone(ctx, tx)
}

func (c crowd) Identify(ctx context.Context, tx pgx.Tx, ids []string) ([]social.Player, error) {
	profiles, err := c.accounts.Profiles(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]social.Player, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, social.Player{ID: p.ID, Name: p.Name, Avatar: p.AvatarURL})
	}
	return out, nil
}

/*
grants answers social.Credits: where an endorsement lands.

Three lines, and the third one is the design. internal/factor's Record returns
the recipient's composed standing; this drops it on the floor, and the interface
on the other side has nowhere to put it — so the package that decides who gets
endorsed cannot learn anything about the person who was.
*/
type grants struct{ factors *factor.Factors }

func (g grants) Record(ctx context.Context, tx pgx.Tx, accountID string, gs ...simulation.Grant) error {
	_, err := g.factors.Record(ctx, tx, accountID, gs...)
	return err
}
