package social

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
The module, against a real Postgres (§14).

Two of the rules this ticket is about are enforced by the schema rather than by
Go — no self-endorsement, and no repeat endorsement of the same player — which
is exactly why a mock would prove nothing here. The rest is time: the cadence,
the window and the rolling cap are all statements about a clock, and every one of
them is driven below by moving one rather than by sleeping.
*/

// world is a fresh database, a clock a test moves by hand, and n players.
//
// The transaction is rolled back by a cleanup rather than by a closure around
// f: a t.Fatalf inside f ends the goroutine outright, and a closure would never
// return — the connection would be held until the pool gave up.
func world(t *testing.T, n int, f func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	players := make([]string, 0, n)
	for i := range n {
		players = append(players, newAccount(t, ctx, tx, string(rune('a'+i))))
	}

	clk := &clock.Stepping{At: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)}
	f(ctx, tx, New(clk, everybody{}, &ledger{}), clk, players)
}

// newAccount makes somebody for the offers to be about. Both tables reference
// account, so there have to be real rows.
func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx, who string) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', $1, $1 || '@example.test', 'Player ' || $1)
		RETURNING id`, who).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

// everybody is a Crowd over the account table — the same thing internal/api
// binds, spelled here so the module's own tests do not need the transport.
type everybody struct{}

func (everybody) Everyone(ctx context.Context, tx pgx.Tx) ([]string, error) {
	return ids(ctx, tx, `SELECT id FROM account ORDER BY created_at, id`)
}

func (everybody) Identify(ctx context.Context, tx pgx.Tx, want []string) ([]Player, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, display_name, coalesce(avatar_url, '') FROM account WHERE id = ANY($1)`, want)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		if err := rows.Scan(&p.ID, &p.Name, &p.Avatar); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ledger is a Credits that remembers what it was handed, so a test can assert
// where an endorsement landed without reaching into internal/factor.
type ledger struct {
	to     []string
	grants []simulation.Grant
}

func (l *ledger) Record(_ context.Context, _ pgx.Tx, accountID string, gs ...simulation.Grant) error {
	for _, g := range gs {
		l.to = append(l.to, accountID)
		l.grants = append(l.grants, g)
	}
	return nil
}

func kept(e *Endorsements) *ledger { return e.credits.(*ledger) }

/*
refuses asserts that the schema rejects a statement, inside a savepoint.

The savepoint is the point. A constraint violation aborts the whole transaction
in Postgres, so a test that made two of these in a row would see the second fail
for the wrong reason — "current transaction is aborted" is not the constraint
holding, it is the previous one still being on the floor.
*/
func refuses(t *testing.T, ctx context.Context, tx pgx.Tx, why, sql string, args ...any) {
	t.Helper()
	nested, err := tx.Begin(ctx)
	if err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, err = nested.Exec(ctx, sql, args...)
	_ = nested.Rollback(ctx)
	if err == nil {
		t.Errorf("the schema allowed %s", why)
	}
}

// look is Look, failing the test rather than returning an error.
func look(t *testing.T, ctx context.Context, tx pgx.Tx, e *Endorsements, who string) Standing {
	t.Helper()
	out, _, err := e.Look(ctx, tx, who)
	if err != nil {
		t.Fatalf("look: %v", err)
	}
	return out
}

/*
The first look hands the player somebody, and the second does not.

This is the whole of "occasional, not on demand" in one assertion: opening the
game is what draws an offer, and opening it again five minutes later draws
nothing, because the cadence is measured from when the offer was made.
*/
func TestAnOfferIsDrawnOnceAndNotAgainUntilTheCadenceHasPassed(t *testing.T) {
	t.Parallel()
	world(t, 2, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me, them := players[0], players[1]

		first, drawn, err := e.Look(ctx, tx, me)
		if err != nil {
			t.Fatalf("look: %v", err)
		}
		if !drawn || first.Offer == nil {
			t.Fatalf("the first look drew nobody: %+v", first)
		}
		if first.Offer.Who.ID != them {
			t.Fatalf("offered %q, want the only other player", first.Offer.Who.ID)
		}
		if first.Offer.ExpiresAt.Sub(first.Offer.OfferedAt) != Window {
			t.Errorf("the offer stands for %v, want %v", first.Offer.ExpiresAt.Sub(first.Offer.OfferedAt), Window)
		}

		// Looking again is not another offer. The whole feature turns on this.
		clk.Advance(5 * time.Minute)
		again, drawnAgain, err := e.Look(ctx, tx, me)
		if err != nil {
			t.Fatalf("look again: %v", err)
		}
		if drawnAgain {
			t.Error("a second look drew a second offer")
		}
		if again.Offer == nil || again.Offer.ID != first.Offer.ID {
			t.Fatalf("the standing offer changed under a second look: %+v", again.Offer)
		}

		// Answered, then straight back: still nothing, because the cadence runs
		// from when the offer was made rather than from when it was answered.
		if _, _, err := e.Endorse(ctx, tx, me, first.Offer.ID); err != nil {
			t.Fatalf("endorse: %v", err)
		}
		if got := look(t, ctx, tx, e, me); got.Offer != nil {
			t.Fatalf("answering one offer produced another: %+v", got.Offer)
		}
	})
}

/*
The cadence is a real wait, and it is stated to the player.

`opens` is sent for the same reason the cap is: a button that vanishes without
saying when it comes back is indistinguishable from a broken screen.
*/
func TestTheNextOfferComesOnTheCadenceAndIsAnnouncedBeforeItDoes(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me := players[0]

		first := look(t, ctx, tx, e, me)
		if first.Offer == nil {
			t.Fatal("no first offer")
		}
		if _, err := e.Pass(ctx, tx, me, first.Offer.ID); err != nil {
			t.Fatalf("pass: %v", err)
		}

		waiting := look(t, ctx, tx, e, me)
		if waiting.Offer != nil {
			t.Fatalf("passing bought a replacement: %+v", waiting.Offer)
		}
		if want := first.Offer.OfferedAt.Add(Cadence); !waiting.Opens.Equal(want) {
			t.Errorf("the next offer opens at %v, want %v", waiting.Opens, want)
		}

		// A minute short of the cadence is still short of it.
		clk.Advance(Cadence - time.Minute)
		if got := look(t, ctx, tx, e, me); got.Offer != nil {
			t.Fatal("an offer arrived a minute early")
		}

		clk.Advance(2 * time.Minute)
		got := look(t, ctx, tx, e, me)
		if got.Offer == nil {
			t.Fatal("the cadence passed and nobody was offered")
		}
		if !got.Opens.IsZero() {
			t.Errorf("an offer is standing and the screen is also told it opens at %v", got.Opens)
		}
		// Not the same person: somebody passed over is not offered again the
		// same week.
		if got.Offer.Who.ID == first.Offer.Who.ID {
			t.Error("the player passed over was offered again straight away")
		}
	})
}

/*
An offer that nobody answered lapses.

Without this the partial unique index would hold the slot forever and the player
would be handed one stranger and then never another — which is the failure mode
"one open offer at a time" quietly has if nothing ever closes them.
*/
func TestAnOfferLeftAloneLapsesAndTheNextOneStillComes(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me := players[0]

		first := look(t, ctx, tx, e, me)
		if first.Offer == nil {
			t.Fatal("no first offer")
		}

		clk.Advance(Window + time.Minute)
		lapsed := look(t, ctx, tx, e, me)
		if lapsed.Offer != nil {
			t.Fatalf("an offer outlived its window: %+v", lapsed.Offer)
		}

		// Answering it now is refused, and refused as a conflict rather than as
		// something the client got wrong.
		if _, _, err := e.Endorse(ctx, tx, me, first.Offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Fatalf("endorsing a lapsed offer = %v, want ErrNoOffer", err)
		}

		clk.Advance(Cadence)
		if got := look(t, ctx, tx, e, me); got.Offer == nil {
			t.Fatal("a lapsed offer blocked the next one")
		}
	})
}

/*
The endorsement lands on the player who was liked, and on nobody else.

§3.5's factor is "how helpful the player is", so being liked is a statement about
the person who was liked — and §7 is why the endorser gains nothing: a factor a
player can award themselves measures the button rather than them.
*/
func TestTheCreditGoesToThePlayerWhoWasLiked(t *testing.T) {
	t.Parallel()
	world(t, 2, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me, them := players[0], players[1]

		offer := look(t, ctx, tx, e, me).Offer
		if offer == nil {
			t.Fatal("no offer")
		}

		mine, liked, err := e.Endorse(ctx, tx, me, offer.ID)
		if err != nil {
			t.Fatalf("endorse: %v", err)
		}
		if liked.To != them || liked.From.ID != me || liked.Credit != Credit {
			t.Fatalf("liked = %+v, want %s endorsed by %s", liked, them, me)
		}

		l := kept(e)
		if len(l.to) != 1 || l.to[0] != them {
			t.Fatalf("credit went to %v, want the player who was liked", l.to)
		}
		if l.grants[0].Factor != simulation.Kindness || l.grants[0].Credit != Credit {
			t.Fatalf("grant = %+v, want the factor §3.5 pairs endorsement with", l.grants[0])
		}

		// The endorser's own surface says nothing about having been liked.
		if mine.Received != 0 || len(mine.Liked) != 0 {
			t.Errorf("the endorser's standing = %+v, want somebody who has received nothing", mine)
		}

		theirs, err := e.Of(ctx, tx, them)
		if err != nil {
			t.Fatalf("read the other player: %v", err)
		}
		if theirs.Received != 1 || len(theirs.Liked) != 1 || theirs.Liked[0].ID != me {
			t.Fatalf("the endorsed player's standing = %+v, want one endorsement from %s", theirs, me)
		}
	})
}

/*
The same player cannot be endorsed twice, ever.

The strongest of the anti-farming rules and the one held by a primary key: two
players who agree to like each other every morning get exactly one morning out of
it. Asserted from both ends — the candidate selection stops offering them, and
the schema refuses if anything ever gets past it.
*/
func TestAPlayerIsNeverEndorsedTwiceByTheSamePerson(t *testing.T) {
	t.Parallel()
	world(t, 2, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me, them := players[0], players[1]

		offer := look(t, ctx, tx, e, me).Offer
		if offer == nil {
			t.Fatal("no offer")
		}
		if _, _, err := e.Endorse(ctx, tx, me, offer.ID); err != nil {
			t.Fatalf("endorse: %v", err)
		}

		// A week later there is nobody left to offer: the only other player has
		// been endorsed, and that is once-ever rather than once-a-week.
		clk.Advance(2 * Span)
		if got := look(t, ctx, tx, e, me); got.Offer != nil {
			t.Fatalf("offered %q again after endorsing them", got.Offer.Who.ID)
		}

		// And the row itself cannot be written a second time.
		var n int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM endorsement WHERE from_account = $1 AND to_account = $2`,
			me, them).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Fatalf("%d endorsements between the same pair, want 1", n)
		}
		refuses(t, ctx, tx, "a second endorsement of the same player", `
			INSERT INTO endorsement (from_account, to_account, at, credit)
			VALUES ($1, $2, now(), 1)`, me, them)
	})
}

/*
Nobody is ever offered themselves, and the schema would refuse it anyway.

The only-other-player case above cannot catch this: a self-offer in a two-player
world looks the same as a correct one until you check who. So this runs with one
player, where the only candidate there is, is the wrong one.
*/
func TestNobodyIsOfferedThemselves(t *testing.T) {
	t.Parallel()
	world(t, 1, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me := players[0]

		if got := look(t, ctx, tx, e, me); got.Offer != nil {
			t.Fatalf("the only player in the world was offered %+v", got.Offer)
		}

		refuses(t, ctx, tx, "an offer of a player to themselves", `
			INSERT INTO endorsement_offer (account_id, candidate_id, offered_at, expires_at)
			VALUES ($1, $1, now(), now())`, me)
		refuses(t, ctx, tx, "a player to endorse themselves", `
			INSERT INTO endorsement (from_account, to_account, at, credit)
			VALUES ($1, $1, now(), 1)`, me)
	})
}

/*
The rolling cap is a second, independent bound.

The cadence already limits a player to about eight a week; this is what holds if
the cadence is ever wrong, and it is counted over real endorsements rather than
over offers — so no amount of offering can spend more than [Cap] in a [Span].
*/
func TestTheWeeklyCapStopsOffersAltogether(t *testing.T) {
	t.Parallel()
	world(t, Cap+3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me := players[0]

		for i := range Cap {
			got := look(t, ctx, tx, e, me)
			if got.Offer == nil {
				t.Fatalf("no offer on round %d of %d", i+1, Cap)
			}
			if got.Given != i {
				t.Fatalf("given = %d on round %d", got.Given, i+1)
			}
			if _, _, err := e.Endorse(ctx, tx, me, got.Offer.ID); err != nil {
				t.Fatalf("endorse on round %d: %v", i+1, err)
			}
			clk.Advance(Cadence + time.Minute)
		}

		capped := look(t, ctx, tx, e, me)
		if capped.Offer != nil {
			t.Fatalf("offered somebody past the cap: %+v", capped.Offer)
		}
		if capped.Given != Cap || capped.Cap != Cap {
			t.Fatalf("standing = %+v, want %d of %d given", capped, Cap, Cap)
		}

		// The window rolls: once the first endorsement is a week old there is
		// room again, so the cap is a rate rather than a lifetime allowance.
		clk.Advance(Span)
		if got := look(t, ctx, tx, e, me); got.Offer == nil {
			t.Fatalf("the cap never rolled off: %+v", got)
		}
	})
}

/*
An offer can be answered once, by the player it was made to, and by nobody else.

All four refusals are one error on purpose: telling a client probing ids which of
them it hit would tell it something about offers that were never theirs.
*/
func TestAnOfferIsAnsweredOnceAndOnlyByWhoItWasMadeTo(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me, somebodyElse := players[0], players[1]

		offer := look(t, ctx, tx, e, me).Offer
		if offer == nil {
			t.Fatal("no offer")
		}

		// Not yours.
		if _, _, err := e.Endorse(ctx, tx, somebodyElse, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("another player answering it = %v, want ErrNoOffer", err)
		}
		// Not an offer at all.
		if _, _, err := e.Endorse(ctx, tx, me, offer.ID+9999); !errors.Is(err, ErrNoOffer) {
			t.Errorf("an invented id = %v, want ErrNoOffer", err)
		}

		if _, _, err := e.Endorse(ctx, tx, me, offer.ID); err != nil {
			t.Fatalf("endorse: %v", err)
		}
		// Twice.
		if _, _, err := e.Endorse(ctx, tx, me, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("answering the same offer twice = %v, want ErrNoOffer", err)
		}
		// And passing an answered one is the same refusal.
		if _, err := e.Pass(ctx, tx, me, offer.ID); !errors.Is(err, ErrNoOffer) {
			t.Errorf("passing an answered offer = %v, want ErrNoOffer", err)
		}
	})
}

/*
Candidates are drawn fewest-endorsements-first.

The ordering is a design decision, not query convenience: endorsement exists to
give a hidden factor a human signal, and a signal that concentrated on whoever
was already visible would measure visibility instead. So the player nobody has
liked is the one the game names next.
*/
func TestTheLeastEndorsedPlayerIsOfferedFirst(t *testing.T) {
	t.Parallel()
	world(t, 4, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me := players[0]
		popular, quiet := players[1], players[3]

		// Somebody else has already been liked twice. The offer should skip
		// them in favour of the player at the back of the queue.
		for _, from := range []string{players[2], players[3]} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO endorsement (from_account, to_account, at, credit)
				VALUES ($1, $2, now(), 1)`, from, popular); err != nil {
				t.Fatalf("seed endorsement: %v", err)
			}
		}
		// And players[2] has one, so the only untouched candidate is the last.
		if _, err := tx.Exec(ctx, `
			INSERT INTO endorsement (from_account, to_account, at, credit)
			VALUES ($1, $2, now(), 1)`, players[1], players[2]); err != nil {
			t.Fatalf("seed endorsement: %v", err)
		}

		got := look(t, ctx, tx, e, me)
		if got.Offer == nil {
			t.Fatal("no offer")
		}
		if got.Offer.Who.ID != quiet {
			t.Fatalf("offered %q, want the player nobody has endorsed", got.Offer.Who.ID)
		}
	})
}

/*
Nobody to like is an ordinary state, not a failure.

A world where everybody eligible has already been endorsed hands back a standing
with no offer on it, and the player sees the same screen they see between
cadences. Which is the point: "occasional" has to have a resting state that reads
as normal.
*/
func TestRunningOutOfPeopleIsJustNoOffer(t *testing.T) {
	t.Parallel()
	world(t, 2, func(ctx context.Context, tx pgx.Tx, e *Endorsements, clk *clock.Stepping, players []string) {
		me := players[0]

		offer := look(t, ctx, tx, e, me).Offer
		if offer == nil {
			t.Fatal("no offer")
		}
		if _, _, err := e.Endorse(ctx, tx, me, offer.ID); err != nil {
			t.Fatalf("endorse: %v", err)
		}

		clk.Advance(2 * Cadence)
		got, drawn, err := e.Look(ctx, tx, me)
		if err != nil {
			t.Fatalf("look: %v", err)
		}
		if drawn || got.Offer != nil {
			t.Fatalf("somebody was found in an empty pool: %+v", got.Offer)
		}
		if got.Given != 1 || got.Cap != Cap {
			t.Errorf("standing = %+v, want one given of %d", got, Cap)
		}
	})
}

/*
Passing tells nobody and costs the candidate nothing.

Being passed over is not a judgement anybody is notified of, and nothing is
recorded against the person — the only trace is that this player will not be
shown them again for a week.
*/
func TestPassingRecordsNothingAgainstTheCandidate(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me := players[0]

		offer := look(t, ctx, tx, e, me).Offer
		if offer == nil {
			t.Fatal("no offer")
		}
		passed := offer.Who.ID

		if _, err := e.Pass(ctx, tx, me, offer.ID); err != nil {
			t.Fatalf("pass: %v", err)
		}

		if len(kept(e).to) != 0 {
			t.Errorf("passing moved a factor: %v", kept(e).to)
		}
		var n int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM endorsement WHERE to_account = $1`, passed).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("%d endorsements written for a player who was passed over", n)
		}

		theirs, err := e.Of(ctx, tx, passed)
		if err != nil {
			t.Fatalf("read the candidate: %v", err)
		}
		if theirs.Received != 0 {
			t.Errorf("the passed-over player was told something: %+v", theirs)
		}
	})
}

/*
Of is a plain read: it never draws, and it never lapses.

Worth holding, because it is called from the tail of every command that answers
an offer. A read with a side effect there would mean endorsing somebody could
hand you the next stranger immediately, which is the whole rule undone by a
convenience.
*/
func TestReadingTheStandingDrawsNobody(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me := players[0]

		for range 3 {
			got, err := e.Of(ctx, tx, me)
			if err != nil {
				t.Fatalf("of: %v", err)
			}
			if got.Offer != nil {
				t.Fatalf("a plain read drew an offer: %+v", got.Offer)
			}
		}

		var n int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM endorsement_offer WHERE account_id = $1`, me).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("%d offers written by reads", n)
		}
	})
}

/*
Only one offer can stand at a time, and the database is what says so.

Two tabs opening the world in the same instant is the ordinary case rather than
the exotic one, and the guard for it is a partial unique index — asserted here by
writing the second row directly, which is what a lost race would amount to.
*/
func TestOnlyOneOfferMayStandAtOnce(t *testing.T) {
	t.Parallel()
	world(t, 3, func(ctx context.Context, tx pgx.Tx, e *Endorsements, _ *clock.Stepping, players []string) {
		me := players[0]

		if got := look(t, ctx, tx, e, me); got.Offer == nil {
			t.Fatal("no offer")
		}
		refuses(t, ctx, tx, "a second open offer", `
			INSERT INTO endorsement_offer (account_id, candidate_id, offered_at, expires_at)
			VALUES ($1, $2, now(), now() + interval '1 hour')`, me, players[2])
	})
}

/*
An unbound world offers nobody.

social.Alone is the Crowd for a server with no account table to ask, and the
point of asserting it is that "there is nobody to like" must be a quiet, ordinary
answer rather than an error path — the same way factor.Alone ranks nobody.
*/
func TestACrowdOfNobodyOffersNobody(t *testing.T) {
	t.Parallel()
	world(t, 2, func(ctx context.Context, tx pgx.Tx, _ *Endorsements, clk *clock.Stepping, players []string) {
		e := New(clk, Alone{}, Unrecorded{})
		got, drawn, err := e.Look(ctx, tx, players[0])
		if err != nil {
			t.Fatalf("look: %v", err)
		}
		if drawn || got.Offer != nil {
			t.Fatalf("a world with nobody in it offered %+v", got.Offer)
		}
	})
}
