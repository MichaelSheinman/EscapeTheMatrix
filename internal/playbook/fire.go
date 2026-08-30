package playbook

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

/*
Rule-fire records: what the character did while the player was elsewhere, and on
whose authority. §3.3 keeps a short enumerated list of history surfaces rather
than a write-on-every-change audit table, and this is one of them.

Diagnosis is the player pointing at their own accumulated evidence (§3.6), and
transfer failure is legible only as a pattern: one bad outcome is noise, the same
answer failing daily for a week is something to act on. A pattern is a query over
rows, so there are rows.

Three decisions:

The rule is stored as it stood rather than referenced. Editing a sentence is the
change (§3.14), so a record pointing at the current rule would rewrite its own
history exactly when the player was trying to read it.

Every question the moment put gets a row, including the ones the player has no
answer to. Gaps in the playbook are what demand attention, and a log that only
recorded the rules that ran would be silent about them.
*/

// More refusals, in the same style as the rest of the module: Go errors, mapped
// to status codes at the edge (§8).
var (
	// ErrNoSuchFact is a situation stating something this vocabulary cannot
	// read — a term that is not in it, or a term stated as the wrong sort of
	// thing. Refused at the seam rather than read past, because a moment the
	// matcher cannot understand is an authoring mistake and would otherwise
	// look exactly like a player with no rules.
	ErrNoSuchFact = errors.New("playbook: no such fact")
	// ErrNoSuchFire is a record that is not this player's, or not there.
	ErrNoSuchFire = errors.New("playbook: no such record")
	// ErrNoSuchOutcome is an outcome outside the closed set.
	ErrNoSuchOutcome = errors.New("playbook: no such outcome")
)

// Outcome is what came of what the character did: four values and no number. A
// record saying "this rule scored 63" would grade the player's judgement in the
// game's own units (§3.2). What the record has to support is §4's escalation
// rule and §3.6's pattern, and both are counting.
type Outcome string

const (
	// Pending is an outcome the world has not answered yet.
	Pending Outcome = "pending"
	Good    Outcome = "good"
	Mixed   Outcome = "mixed"
	Poor    Outcome = "poor"
)

// outcomes is the closed set.
var outcomes = map[Outcome]bool{Pending: true, Good: true, Mixed: true, Poor: true}

// TODO: Why the name fire? Why not moment?
// Fire is one recorded moment: what was put, what the playbook made of it, and
// what came of it.
type Fire struct {
	ID          int64
	At          time.Time
	Environment string
	Situation   string
	Verdict     Verdict
	Outcome     Outcome
	SettledAt   time.Time
	// Considered is every question the moment put, in question order: the shape
	// [Decide] produced, read back.
	Considered []Consideration
	// Note is why it escalated, in the words the log uses. Empty when it did not.
	Note string
	// Cause is which of §4's three escalation conditions this met, or empty.
	// Derived from the verdict and the outcome rather than stored — see
	// [Because].
	Cause Cause
	// RaisedAt is when the world put this to the player, and zero until it did.
	RaisedAt time.Time
}

// Log is a page of the record.
type Log struct {
	Fires []Fire
	Page  int
	Pages int
	Total int
}

// LogPage is how many moments a page of the log holds. A constant rather than a
// parameter: a page size the client could choose is a way of asking for the whole
// history at once, and this table grows daily.
const LogPage = 20

// Trend is how one answer has been going over a window — §3.6's pattern, as
// counts rather than a rate or a verdict. "Your answer on being asked for help
// fired eleven times this week and seven went badly" is something a player can
// argue with; "that answer is performing poorly" tells them what to think
// (§3.2).
type Trend struct {
	Question string
	// Put is the moments that raised this question at all.
	Put int
	// Fired is the moments this answer decided.
	Fired int
	// Weighed is the moments it reached an action but another answer decided,
	// or nothing did because they disagreed.
	Weighed int
	// Unanswered is the moments the player had no complete rule for.
	Unanswered int
	// Undecided is the moments the rule turned on something nobody stated.
	Undecided int
	// Poor and Good are the settled outcomes of the moments this answer decided.
	Poor int
	Good int
}

/*
Weigh reads the rules in force where the player is standing and decides, writing
nothing.

The environment comes first: [Playbooks.InForce] refuses a key that is not an
environment and considers only that one set. §3.15 turns on the same rules giving
different answers in different places, so a matcher that fell back to another set
would be running a different game.

Nothing is recorded here, because nothing has happened. The caller weighs, acts
if it may, and then records what it did.
*/
func (m *Playbooks) Weigh(
	ctx context.Context, tx pgx.Tx, accountID, environment string, s Situation,
) (Decision, error) {
	if faults := s.Faults(); len(faults) > 0 {
		return Decision{}, fmt.Errorf("%w: %s", ErrNoSuchFact, strings.Join(faults, ", "))
	}
	rules, err := m.InForce(ctx, tx, accountID, environment)
	if err != nil {
		return Decision{}, err
	}
	return Decide(rules, s), nil
}

// Foresaw audits a player's rule set for one environment against the questions a
// moment would put, with no moment in hand. Rule conflicts are statically
// detectable (§6): a catalogue of authored situations declares which questions
// each moment raises, so a whole environment's contradictions can be found
// before any of them happens.
func (m *Playbooks) Foresaw(
	ctx context.Context, tx pgx.Tx, accountID, environment string, puts ...string,
) (Audit, error) {
	rules, err := m.InForce(ctx, tx, accountID, environment)
	if err != nil {
		return Audit{}, err
	}
	return Foresee(rules, puts...), nil
}

/*
Record writes what the playbook decided and what came of it.

at comes from the caller rather than now(): the rollover replays missed days
(§3.3), and a record stamped with the moment it was written would file a
Tuesday's conflicts under Friday.

A decision with nothing considered is not written. It is a moment put to a player
standing somewhere the question is never asked, and a row for it would be
evidence of nothing.
*/
func (m *Playbooks) Record(
	ctx context.Context, tx pgx.Tx, accountID string,
	at time.Time, environment string, d Decision, outcome Outcome,
) (Fire, error) {
	if !outcomes[outcome] {
		return Fire{}, fmt.Errorf("%w: %q", ErrNoSuchOutcome, outcome)
	}
	if _, ok := m.environment(environment); !ok {
		return Fire{}, fmt.Errorf("%w: %q", ErrNoSuchEnvironment, environment)
	}
	if len(d.Considered) == 0 {
		return Fire{}, nil
	}

	var settled *time.Time
	if outcome != Pending {
		settled = &at
	}

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO playbook_fire (account_id, at, environment, situation, verdict, outcome, settled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		accountID, at, environment, d.Situation, string(d.Verdict), string(outcome), settled,
	).Scan(&id); err != nil {
		return Fire{}, fmt.Errorf("playbook: write record: %w", err)
	}

	for _, c := range d.Considered {
		var when [MaxConditions]Ref
		var met [MaxConditions]*bool
		for i := 0; i < MaxConditions; i++ {
			if i < len(c.Rule.When) {
				when[i] = c.Rule.When[i]
			}
			if i < len(c.Met) {
				met[i] = flag(c.Met[i])
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO playbook_fire_rule (
				fire_id, question, state, join_word,
				when_1_term, when_1_value, when_1_met,
				when_2_term, when_2_value, when_2_met,
				matched, then_term, then_value, else_term, else_value, stance)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
			id, c.Question, string(c.State), nullJoin(c.Rule.Join),
			null(when[0].Term), null(when[0].Value), met[0],
			null(when[1].Term), null(when[1].Value), met[1],
			flag(c.Held),
			null(c.Rule.Then.Term), null(c.Rule.Then.Value),
			null(c.Rule.Else.Term), null(c.Rule.Else.Value),
			null(string(c.Stance)),
		); err != nil {
			return Fire{}, fmt.Errorf("playbook: write record: %w", err)
		}
	}

	out := Fire{
		ID: id, At: at, Environment: environment, Situation: d.Situation,
		Verdict: d.Verdict, Outcome: outcome, Considered: d.Considered, Note: d.Note(),
	}
	out.Cause, _ = Because(d.Verdict, outcome)
	if settled != nil {
		out.SettledAt = at
	}
	return out, nil
}

/*
Settle answers a record whose consequences were not known when it was written,
once. A record that could be re-graded would let the same moment be counted twice
in a pattern, and §3.6's evidence is only worth anything if a week of poor
outcomes is a week rather than a re-reading.

It reports the escalation the answer caused, if any — §4's third condition, and
the only one that can fire here. The first two were known when the record was
written and are on [Decision.Escalation]. Returning it here makes the third
visible at the moment it becomes true, and it is the same value
[Playbooks.Unraised] hands back later.
*/
func (m *Playbooks) Settle(
	ctx context.Context, tx pgx.Tx, accountID string, fire int64, at time.Time, outcome Outcome,
) (Escalation, bool, error) {
	if !outcomes[outcome] || outcome == Pending {
		return Escalation{}, false, fmt.Errorf("%w: %q", ErrNoSuchOutcome, outcome)
	}
	var (
		out     = Escalation{Fire: fire}
		verdict string
		raised  *time.Time
	)
	err := tx.QueryRow(ctx, `
		UPDATE playbook_fire SET outcome = $1, settled_at = $2
		 WHERE id = $3 AND account_id = $4 AND outcome = 'pending'
		RETURNING at, environment, situation, verdict, raised_at`,
		string(outcome), at, fire, accountID,
	).Scan(&out.At, &out.Environment, &out.Situation, &verdict, &raised)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Escalation{}, false, fmt.Errorf("%w: %d", ErrNoSuchFire, fire)
	case err != nil:
		return Escalation{}, false, fmt.Errorf("playbook: settle record: %w", err)
	}

	cause, escalates := Because(Verdict(verdict), outcome)
	if !escalates || cause != Backfired || raised != nil {
		// Only the third condition is news here. A record that was already
		// silent or clashing escalated when it was written, and one already
		// raised has been to the player once.
		return Escalation{}, false, nil
	}
	out.Cause = cause
	return out, true, nil
}

// Fires is a page of the log, newest first — §3.6's evidence surface. Paginated
// because a player who has been going a year has a year of moments, and reading
// back what happened is done at the scale of the last few days.
//
// Every part is re-read against the vocabulary as it comes back. A term withdrawn
// from content reads as its bare key, because a log line that dropped a word
// would be wrong rather than merely shorter.
func (m *Playbooks) Fires(ctx context.Context, tx pgx.Tx, accountID string, page int) (Log, error) {
	var total int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM playbook_fire WHERE account_id = $1`, accountID,
	).Scan(&total); err != nil {
		return Log{}, fmt.Errorf("playbook: count records: %w", err)
	}

	out := Log{Fires: []Fire{}, Page: 1, Total: total, Pages: (total + LogPage - 1) / LogPage}
	if out.Pages == 0 {
		return out, nil
	}
	// Clamped rather than refused, and the answer says which page it got.
	out.Page = page
	if out.Page < 1 {
		out.Page = 1
	}
	if out.Page > out.Pages {
		out.Page = out.Pages
	}

	rows, err := tx.Query(ctx, `
		SELECT id, at, environment, situation, verdict, outcome, settled_at, raised_at
		  FROM playbook_fire
		 WHERE account_id = $1
		 ORDER BY at DESC, id DESC
		 LIMIT $2 OFFSET $3`, accountID, LogPage, (out.Page-1)*LogPage)
	if err != nil {
		return Log{}, fmt.Errorf("playbook: read records: %w", err)
	}
	defer rows.Close()

	ids := []int64{}
	at := map[int64]int{}
	for rows.Next() {
		var (
			f       Fire
			verdict string
			outcome string
			settled *time.Time
			raised  *time.Time
		)
		if err := rows.Scan(&f.ID, &f.At, &f.Environment, &f.Situation,
			&verdict, &outcome, &settled, &raised); err != nil {
			return Log{}, fmt.Errorf("playbook: read records: %w", err)
		}
		f.Verdict, f.Outcome = Verdict(verdict), Outcome(outcome)
		f.Cause, _ = Because(f.Verdict, f.Outcome)
		if settled != nil {
			f.SettledAt = *settled
		}
		if raised != nil {
			f.RaisedAt = *raised
		}
		f.Considered = []Consideration{}
		at[f.ID] = len(out.Fires)
		ids = append(ids, f.ID)
		out.Fires = append(out.Fires, f)
	}
	if err := rows.Err(); err != nil {
		return Log{}, fmt.Errorf("playbook: read records: %w", err)
	}
	if len(ids) == 0 {
		return out, nil
	}

	weighed, err := m.considered(ctx, tx, ids)
	if err != nil {
		return Log{}, err
	}
	for fire, considered := range weighed {
		out.Fires[at[fire]].Considered = considered
	}

	for i := range out.Fires {
		out.Fires[i].Note = Decision{
			Verdict: out.Fires[i].Verdict, Considered: out.Fires[i].Considered,
		}.Note()
	}
	return out, nil
}

/*
Moments reads named records rather than a page of them, keyed by id.
[Playbooks.Fires] answers "what have my rules been doing lately"; this answers
"what happened in that one", which is what an escalation event asks when the
player opens it days later. Scoped by account, so a row that is not this
player's does not exist to them.

It returns a [Fire] with its considerations on it. This module holds which rule
fired and where; who it was with and what was said belong to whoever authored the
moment (§3.1).
*/
func (m *Playbooks) Moments(
	ctx context.Context, tx pgx.Tx, accountID string, fires []int64,
) (map[int64]Fire, error) {
	out := map[int64]Fire{}
	if len(fires) == 0 {
		return out, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT id, at, environment, situation, verdict, outcome, settled_at, raised_at
		  FROM playbook_fire
		 WHERE account_id = $1 AND id = ANY($2)`, accountID, fires)
	if err != nil {
		return nil, fmt.Errorf("playbook: read records: %w", err)
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var (
			f       Fire
			verdict string
			outcome string
			settled *time.Time
			raised  *time.Time
		)
		if err := rows.Scan(&f.ID, &f.At, &f.Environment, &f.Situation,
			&verdict, &outcome, &settled, &raised); err != nil {
			return nil, fmt.Errorf("playbook: read records: %w", err)
		}
		f.Verdict, f.Outcome = Verdict(verdict), Outcome(outcome)
		f.Cause, _ = Because(f.Verdict, f.Outcome)
		if settled != nil {
			f.SettledAt = *settled
		}
		if raised != nil {
			f.RaisedAt = *raised
		}
		f.Considered = []Consideration{}
		out[f.ID] = f
		ids = append(ids, f.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read records: %w", err)
	}

	// After the cursor is done, never inside it: the connection is busy until
	// this one closes, and a second query mid-scan deadlocks.
	weighed, err := m.considered(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for fire, considered := range weighed {
		f := out[fire]
		f.Considered = considered
		f.Note = Decision{Verdict: f.Verdict, Considered: considered}.Note()
		out[fire] = f
	}
	return out, nil
}

// considered reads the rules weighed in a set of fires, keyed by fire. Shared by
// [Playbooks.Fires] and [Playbooks.Unraised], so the rendering of a stored rule
// back into its sentence cannot drift between the log and the escalation.
func (m *Playbooks) considered(
	ctx context.Context, tx pgx.Tx, ids []int64,
) (map[int64][]Consideration, error) {
	out := map[int64][]Consideration{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT fire_id, question, state, join_word,
		       when_1_term, when_1_value, when_1_met,
		       when_2_term, when_2_value, when_2_met,
		       matched, then_term, then_value, else_term, else_value, stance
		  FROM playbook_fire_rule
		 WHERE fire_id = ANY($1)
		 ORDER BY fire_id, question`, ids)
	if err != nil {
		return nil, fmt.Errorf("playbook: read records: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fire   int64
			c      Consideration
			state  string
			joined *string
			raw    [8]*string
			met    [MaxConditions]*bool
			held   *bool
			stance *string
		)
		if err := rows.Scan(&fire, &c.Question, &state, &joined,
			&raw[0], &raw[1], &met[0], &raw[2], &raw[3], &met[1],
			&held, &raw[4], &raw[5], &raw[6], &raw[7], &stance); err != nil {
			return nil, fmt.Errorf("playbook: read records: %w", err)
		}
		c.State = State(state)
		c.Stance = Stance(text(stance))
		c.Held = truthOf(held)
		c.Met = []Truth{}
		c.Rule = Sentence{
			Join: Join(text(joined)),
			Then: Ref{Term: text(raw[4]), Value: text(raw[5])},
			Else: Ref{Term: text(raw[6]), Value: text(raw[7])},
		}
		// Only a complete rule is ever recorded, so a second condition slot is
		// filled exactly when the sentence had two — the mid-edit ambiguity
		// playbook_rule needs a stored count for cannot arise here.
		if c.State != Unanswered {
			when := []Ref{{Term: text(raw[0]), Value: text(raw[1])}}
			if second := (Ref{Term: text(raw[2]), Value: text(raw[3])}); !second.Blank() {
				when = append(when, second)
			}
			for i, w := range when {
				c.Rule.When = append(c.Rule.When, w)
				c.Met = append(c.Met, truthOf(met[i]))
			}
			c.Action = c.Rule.Then
			if c.Held == Unmet {
				c.Action = c.Rule.Else
			}
			if c.Held == Unsaid {
				c.Action = Ref{}
			}
			if q, ok := vocabulary.Question(c.Question); ok {
				c.Says = q.Read(c.Rule)
				if !c.Action.Blank() {
					c.Did = q.Reads(Action, c.Action)
				}
			}
		}
		out[fire] = append(out[fire], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read records: %w", err)
	}
	return out, nil
}

/*
Evidence is how each answer has been going since a moment — §3.6's pattern.

One query rather than a scan of the log, because the pattern is the point and a
client that counted rows itself would be computing a game rule (§10.2). The
window is the caller's: a week is what §3.15 calls a pattern, and a diagnosis
(#32) may want longer.
*/
func (m *Playbooks) Evidence(
	ctx context.Context, tx pgx.Tx, accountID string, since time.Time,
) ([]Trend, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.question,
		       count(*),
		       count(*) FILTER (WHERE r.state = 'fired'),
		       count(*) FILTER (WHERE r.state = 'weighed'),
		       count(*) FILTER (WHERE r.state = 'unanswered'),
		       count(*) FILTER (WHERE r.state = 'undecided'),
		       count(*) FILTER (WHERE r.state = 'fired' AND f.outcome = 'poor'),
		       count(*) FILTER (WHERE r.state = 'fired' AND f.outcome = 'good')
		  FROM playbook_fire_rule r
		  JOIN playbook_fire f ON f.id = r.fire_id
		 WHERE f.account_id = $1 AND f.at >= $2
		 GROUP BY r.question
		 ORDER BY min(r.n)`, accountID, since)
	if err != nil {
		return nil, fmt.Errorf("playbook: read evidence: %w", err)
	}
	defer rows.Close()

	out := []Trend{}
	for rows.Next() {
		var t Trend
		if err := rows.Scan(&t.Question, &t.Put, &t.Fired, &t.Weighed,
			&t.Unanswered, &t.Undecided, &t.Poor, &t.Good); err != nil {
			return nil, fmt.Errorf("playbook: read evidence: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("playbook: read evidence: %w", err)
	}
	return out, nil
}

/* ── columns ───────────────────────────────────────────────────────────── */

// flag writes a three-valued reading as a nullable boolean. NULL is "the moment
// did not say", which is a third answer rather than a missing one — see Truth.
func flag(t Truth) *bool {
	yes, no := true, false
	switch t {
	case Met:
		return &yes
	case Unmet:
		return &no
	default:
		return nil
	}
}

func truthOf(b *bool) Truth {
	if b == nil {
		return Unsaid
	}
	if *b {
		return Met
	}
	return Unmet
}

func nullJoin(j Join) *string {
	if j == "" {
		return nil
	}
	s := string(j)
	return &s
}
