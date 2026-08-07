package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
Release and escalation at the edge (§4).

The policy is proved in internal/event with nothing running and the three
conditions in internal/playbook against a database. What only this layer can
show is the join — a moment the player's rules could not handle becomes
something they are standing in front of — and the promise the ticket makes about
the surface: there is no recommendation UI, so none of this may cross the wire
as advice.
*/

// aMomentNothingCovered records a moment in an environment the player has no
// rules for, which is §4's first escalation condition.
//
// Directly through the module, because there is no command that fires a rule
// and there is not supposed to be — situations arise while the player is
// elsewhere (§3.14), and what generates them is #30.
func aMomentNothingCovered(t *testing.T, s *Server, st *store.Store) {
	t.Helper()
	account := onlyAccount(t, st)
	ctx := t.Context()

	if err := s.store.InTx(ctx, func(tx pgx.Tx) error {
		moment := playbook.Arises("a-favour", "help").Reading("help/condition/fee", 9000)
		d, err := s.playbooks.Weigh(ctx, tx, account, "downtown", moment)
		if err != nil {
			return err
		}
		if d.Verdict != playbook.Silent {
			t.Errorf("the moment was %q, want %q", d.Verdict, playbook.Silent)
		}
		_, err = s.playbooks.Record(ctx, tx, account, s.clk.Now(), "downtown", d, playbook.Pending)
		return err
	}); err != nil {
		t.Fatalf("record a moment: %v", err)
	}
}

/*
A moment the playbook could not handle reaches the player as something in the
world.

The whole of §4's "escalation is the other half". It is delivered through the
same surface as everything else — placed somewhere, offered once, refusable —
because an escalation that arrived through its own channel would be a
notifications list, and the ticket's last line rules one out.
*/
func TestWhatThePlaybookCouldNotHandleComesToThePlayer(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// The first snapshot fills the backlog and runs the catch-up, so anything
	// that arrives after this is not the world's ordinary generation.
	eventsOf(t, s, as)
	aMomentNothingCovered(t, s, st)

	// The next time the player is here at all, the escalation is raised. It is
	// not gated on a day boundary: a conflict this morning waiting until
	// tomorrow's rollover would be the game quiet about the one thing it must
	// not be quiet about.
	eventsOf(t, s, as)

	account := onlyAccount(t, st)
	var (
		queued int
		raised int
	)
	if err := s.store.InTx(t.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `
			SELECT count(*) FROM event_backlog
			 WHERE account_id = $1 AND event = $2 AND state <> 'done'`,
			account, escalations[playbook.Unhandled],
		).Scan(&queued); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `
			SELECT count(*) FROM playbook_fire
			 WHERE account_id = $1 AND raised_at IS NOT NULL`, account).Scan(&raised)
	}); err != nil {
		t.Fatalf("read the backlog: %v", err)
	}
	if queued != 1 {
		t.Fatalf("%d escalations are waiting in the world, want 1", queued)
	}
	if raised != 1 {
		t.Errorf("%d records are marked raised, want 1", raised)
	}

	// And it is the thing the player is handed next, ahead of whatever the
	// world had generated for them.
	offer := eventsOf(t, s, as).Offer
	if offer == nil {
		t.Fatal("nothing was offered with an escalation waiting")
	}
	if offer.Name != "Nothing covered it" {
		t.Errorf("the world offered %q ahead of the escalation", offer.Name)
	}

	// Once. Coming back does not raise the same conflict again.
	if rec := post(t, s, as, "event.decline", "no-1", `{"event":`+itoa(offer.ID)+`}`); rec.Code != http.StatusOK {
		t.Fatalf("declining: status = %d, body = %s", rec.Code, rec.Body)
	}
	eventsOf(t, s, as)
	if err := s.store.InTx(t.Context(), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `
			SELECT count(*) FROM playbook_fire
			 WHERE account_id = $1 AND raised_at IS NOT NULL`, account).Scan(&raised)
	}); err != nil {
		t.Fatalf("read the records: %v", err)
	}
	if raised != 1 {
		t.Errorf("%d records raised after coming back, want the same 1", raised)
	}
}

/*
The ticket's last acceptance criterion: no recommendation list, queue, or
suggestion feed anywhere in the UI.

The client cannot render what it is never sent, so this is asserted at the wire
rather than in a component. Everything the release policy decides from — how long
the player was away, which factors are lagging, what they have been declining,
what is queued behind the one thing in front of them — is server-side and stays
there. §4 says the notification box is where the server makes recommendations,
and the offer *is* the notification box: the recommendation is the moment
happening, not a sentence about it.
*/
func TestNoRecommendationCrossesTheWire(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)

	// A world with something to say: a backlog, an escalation, and a moment the
	// player answered badly.
	eventsOf(t, s, as)
	aMomentNothingCovered(t, s, st)
	eventsOf(t, s, as)

	body := read(t, s, as, worldPath).Body.String()
	for _, word := range []string{
		"lagging", "recommend", "suggest", "backlog", "escalat",
		"away", "unraised", "cooling", "attention",
	} {
		if strings.Contains(strings.ToLower(body), word) {
			t.Errorf("the world snapshot says %q; selection is invisible (§4)", word)
		}
	}

	// The playbook's log is the one place an escalation is legible, and it is
	// history rather than advice: it says what happened and never what to do.
	log := read(t, s, as, logPath).Body.String()
	for _, word := range []string{"recommend", "suggest", "you should", "try "} {
		if strings.Contains(strings.ToLower(log), word) {
			t.Errorf("the playbook log says %q; the log reports, it does not advise", word)
		}
	}
}
