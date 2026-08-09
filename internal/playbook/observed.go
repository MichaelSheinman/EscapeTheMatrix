package playbook

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
What the log says about itself — the other half of §3.6's evidence, and the
moments a diagnosis is *about*.

internal/health can say what a body has been doing. Only this package can say
what kept happening to the player and what their own answers made of it, and
§3.1 puts the reading here for the same reason it puts the body's reading in
internal/health: the module that owns the rows owns what they mean.

# A failure is an escalation that already reached the player

[Failed] does not invent a category. §4's three conditions are the game's own
definition of something having gone badly — no rule covered it, two rules
disagreed, or a rule fired and it went worse than doing nothing — and the world
already puts each of them in front of the player as an authored event. So the
moment of consequence §3.6 asks a diagnosis to be staged at is not a new concept
this file has to define: it is the escalation the player has already been handed,
read back.

That has a consequence worth stating plainly, because it is the mechanic's
pacing. A player who never writes a rule escalates constantly (nothing covers
anything), a player who writes one catch-all escalates on outcomes, and a player
whose playbook is genuinely good escalates rarely — and diagnoses rarely, and has
correspondingly little to work out. All three are the design working.
*/

// Observation is one thing the log says, ready to be pointed at. The same shape
// internal/health produces and the same reason it is not the consumer's type.
type Observation struct {
	Key  string
	Says string
	From string
}

/*
Since is how far back the log looks when it is being argued from.

A fortnight, matching health.Window and §3.15's pattern — "one bad outcome is
noise, the same rule failing every morning for a week is a pattern". The two
windows are the same length on purpose: evidence from the body and evidence from
the log have to be about the same stretch of somebody's life, or a player would
be comparing a fortnight of readings against a quarter of decisions.
*/
const Since = 14 * 24 * time.Hour

/*
Failed is the escalations this player has already been handed, newest first.

Newest first, and uncapped, because the caller wants the most recent one and the
rest are only there so a conclusion drawn about an older moment can still be
found. That is the mirror image of [Playbooks.Unraised], which is oldest first
and capped because it is feeding the world rather than reading it back.
*/
func (m *Playbooks) Failed(
	ctx context.Context, tx pgx.Tx, accountID string, now time.Time,
) ([]Escalation, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, at, environment, situation, verdict, outcome
		  FROM playbook_fire
		 WHERE account_id = $1
		   AND raised_at IS NOT NULL
		   AND at >= $2
		 ORDER BY at DESC, id DESC`, accountID, now.Add(-Since))
	if err != nil {
		return nil, fmt.Errorf("playbook: read failures: %w", err)
	}
	defer rows.Close()

	out := []Escalation{}
	for rows.Next() {
		var (
			e       Escalation
			verdict string
			outcome string
		)
		if err := rows.Scan(&e.Fire, &e.At, &e.Environment, &e.Situation, &verdict, &outcome); err != nil {
			return nil, fmt.Errorf("playbook: read failures: %w", err)
		}
		cause, ok := Because(Verdict(verdict), Outcome(outcome))
		if !ok {
			// A row that met a condition when it was raised and no longer
			// does — a poor outcome later re-settled. It is not a failure any
			// more, and offering it to be explained would be asking somebody
			// to account for something that came good.
			continue
		}
		e.Cause = cause
		e.Note = Decision{Verdict: Verdict(verdict)}.Note()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read failures: %w", err)
	}
	return out, nil
}

// Observable is every key this package can produce — the same list health
// exports, for the same check and the same reason it is written out by hand.
func Observable() []string {
	return []string{
		"playbook/nothing-covered-it",
		"playbook/answers-disagreed",
		"playbook/answer-keeps-failing",
	}
}

/*
Observed turns a window of the log into things the player can point at.

Three readings, and each is a count over [Trend] rather than a verdict about it —
the same discipline [Trend]'s own doc comment states: "answer 4 fired eleven
times this week and seven went badly" is arguable; "answer 4 is performing
poorly" is the game telling them what to think.

The `[]Trend` is the caller's because [Playbooks.Evidence] already computes it in
one query and the window is the caller's business. Passing it in rather than
re-querying is also what makes this a pure function, which is what makes its
thresholds testable without a database.
*/
func Observed(trends []Trend) []Observation {
	var (
		put        int
		unanswered int
		undecided  int
		worst      Trend
	)
	for _, t := range trends {
		put += t.Put
		unanswered += t.Unanswered
		undecided += t.Undecided
		if t.Poor > worst.Poor {
			worst = t
		}
	}

	out := []Observation{}
	if put == 0 {
		// Nothing has happened. A player with an empty fortnight has nothing to
		// argue from here, which is honest — and is why the body is the other
		// ledger.
		return out
	}

	if unanswered >= 3 {
		out = append(out, Observation{
			Key: "playbook/nothing-covered-it",
			Says: fmt.Sprintf("%d of the %d moments in the last fortnight put a question you had no complete rule for.",
				unanswered, put),
			From: "Your playbook",
		})
	}
	if undecided >= 2 {
		out = append(out, Observation{
			Key: "playbook/answers-disagreed",
			Says: fmt.Sprintf("%d times in the last fortnight, two of your own answers reached the moment and disagreed.",
				undecided),
			From: "Your playbook",
		})
	}
	// Two is where a bad outcome stops being a bad day. §3.15's own line is
	// that one is noise and the same answer failing repeatedly is a pattern,
	// and a pattern is the only thing here worth calling evidence.
	if worst.Poor >= 2 {
		out = append(out, Observation{
			Key: "playbook/answer-keeps-failing",
			Says: fmt.Sprintf("The answer you wrote for %q decided %d moments in the last fortnight and %d of them went badly.",
				worst.Question, worst.Fired, worst.Poor),
			From: "Your playbook",
		})
	}
	return out
}
