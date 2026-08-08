package conflict

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/clock"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/environment"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store/storetest"
)

/*
§3.15's loop, against a real Postgres (§14).

The catalogue is proved in catalogue_test.go and the selection in
standing_test.go, both with nothing running. What is left is the half that only
exists because there is a player and a ledger: friction follows from what
actually happened, the rules in force settle what they cover, what they do not
cover waits, an answer changes what tomorrow is about, and none of it is random.
*/

// among is the population every case below lives in. A shipped one rather than
// an invented one, because the pathways are authored *for* a population and a
// test that made one up would be testing a catalogue nobody ships.
const among = "east-side"

// world is everything one player's daily friction needs around it.
//
// The playbook is the real module rather than a stand-in, deliberately: the
// whole ticket is that a conflict is answered by the *player's own rules*, and a
// fake matcher would prove the plumbing and none of the design.
type world struct {
	ctx     context.Context
	tx      pgx.Tx
	clk     *clock.Stepping
	m       *Conflicts
	rules   *playbook.Playbooks
	paid    *ledger
	account string
}

// here is a Where for a player who lives among one population and does not move.
// §3.15's transfer is internal/playbook's test; what this package needs is a
// population that exists.
type here struct{}

func (here) Where(context.Context, pgx.Tx, string) (string, error) { return among, nil }

// ledger is an Awards that records what it was handed, so a test can assert that
// an answer's authored deltas were actually paid.
type ledger struct{ grants []simulation.Grant }

func (l *ledger) Award(_ context.Context, _ pgx.Tx, _ string, grants ...simulation.Grant) error {
	l.grants = append(l.grants, grants...)
	return nil
}

func (l *ledger) credit(f simulation.Factor) float64 {
	out := 0.0
	for _, g := range l.grants {
		if g.Factor == f {
			out += g.Credit
		}
	}
	return out
}

// populations is the environment set the playbook holds rule sets for. The
// world's own, so that a rule composed here is a rule in force there.
type populations struct{}

func (populations) Environments() []playbook.Environment {
	all := environment.All()
	out := make([]playbook.Environment, 0, len(all))
	for _, e := range all {
		out = append(out, playbook.Environment{Key: e.Key, Name: e.Name})
	}
	return out
}

// with runs f inside one transaction on a fresh database, which is the shape
// every command has (§3.2) and therefore the shape every test should have.
//
// The transaction is rolled back by a cleanup rather than by a closure around f,
// because a t.Fatalf inside f ends the goroutine outright — a closure would
// never return and the connection would be held until the pool gave up.
func with(t *testing.T, f func(*world)) {
	t.Helper()
	st := storetest.New(t)
	ctx := t.Context()

	tx, err := st.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	clk := &clock.Stepping{At: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)}
	rules := playbook.New(populations{})
	paid := &ledger{}

	f(&world{
		ctx: ctx, tx: tx, clk: clk, rules: rules, paid: paid,
		m:       New(clk, rules, here{}, paid),
		account: newAccount(t, ctx, tx),
	})
}

func newAccount(t *testing.T, ctx context.Context, tx pgx.Tx) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO account (provider, subject, email, display_name)
		VALUES ('google', 'conflict-test', 'conflict@example.com', 'Conflict')
		RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("create account: %v", err)
	}
	return id
}

func (w *world) live(t *testing.T, day int) Arisen {
	t.Helper()
	out, err := w.m.Day(w.ctx, w.tx, w.account, on(day))
	if err != nil {
		t.Fatalf("day %d: %v", day, err)
	}
	return out
}

func (w *world) waiting(t *testing.T) (Waiting, bool) {
	t.Helper()
	out, standing, err := w.m.Waiting(w.ctx, w.tx, w.account)
	if err != nil {
		t.Fatalf("waiting: %v", err)
	}
	return out, standing
}

func (w *world) answer(t *testing.T, id int64, stance playbook.Stance) Answer {
	t.Helper()
	out, err := w.m.Answer(w.ctx, w.tx, w.account, id, stance)
	if err != nil {
		t.Fatalf("answer %d with %q: %v", id, stance, err)
	}
	return out
}

func (w *world) living(t *testing.T, day int) Living {
	t.Helper()
	out, err := w.m.Living(w.ctx, w.tx, w.account, on(day))
	if err != nil {
		t.Fatalf("living: %v", err)
	}
	return out
}

func (w *world) standing(t *testing.T, person string) Standing {
	t.Helper()
	all, err := w.m.standings(w.ctx, w.tx, w.account, among)
	if err != nil {
		t.Fatalf("standings: %v", err)
	}
	return all[person]
}

// compose writes a rule for one question that answers it the same way whatever
// the world says, so that the conflict resolves without the player. The whole
// point of the over-general rule is §3.14's own worked example: it covers
// everything, and what it costs is charged somewhere else.
func (w *world) compose(t *testing.T, question string, then, otherwise playbook.Ref) {
	t.Helper()
	q, asks := playbook.Vocab().Question(question)
	if !asks {
		t.Fatalf("%q is not a question", question)
	}
	var when playbook.Ref
	for _, term := range q.Conditions() {
		if term.Start && !term.Takes() {
			when = playbook.Ref{Term: term.Key}
			break
		}
	}
	if when.Blank() {
		t.Fatalf("%q has no plain opening condition", question)
	}
	if _, err := w.rules.Compose(w.ctx, w.tx, w.account, among, question, playbook.Sentence{
		When: []playbook.Ref{when},
		Join: playbook.And,
		Then: then,
		Else: otherwise,
	}); err != nil {
		t.Fatalf("compose %q: %v", question, err)
	}
}

// covering answers every question the shipped pool can put, all one way, so that
// whatever arises is settled without the player.
func (w *world) covering(t *testing.T) {
	t.Helper()
	for question, both := range map[string][2]string{
		"help":  {"yes", "no"},
		"money": {"none", "think"},
		"voice": {"quiet", "back"},
	} {
		w.compose(t, question,
			playbook.Ref{Term: both[0]}, playbook.Ref{Term: both[1]})
	}
}

/* ── it happens because you live somewhere ────────────────────────────── */

func TestADayAmongPeopleProducesFrictionWithOneOfThem(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, 5)
		if !arose.Arose() {
			t.Fatal("a day among six people produced nothing")
		}
		people, _ := environment.Lookup(among)
		if _, one := people.Person(arose.Person); !one {
			t.Errorf("it was with %q, who is not one of these people", arose.Person)
		}
		if _, written := Pathway(among, arose.Conflict); !written {
			t.Errorf("%q is not a pathway this population has", arose.Conflict)
		}
		if !Known(arose.Strain) {
			t.Errorf("it followed from %q", arose.Strain)
		}
		// Nothing covers it yet, so it is in front of the player (§3.15's
		// third line: anything the rules do not cover escalates).
		if !arose.Waiting {
			t.Error("a player with no rules had one settled for them")
		}
	})
}

func TestNothingHappensToSomebodyAmongNobody(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		w.m = New(w.clk, w.rules, nobody{}, w.paid)
		if arose := w.live(t, 5); arose.Arose() {
			t.Errorf("%+v happened to somebody who lives among no one", arose)
		}
	})
}

// nobody is a Where for a player whose home has no population around it, which
// is a real state of the world: the city has four districts and two of them
// nobody lives in (§3.12).
type nobody struct{}

func (nobody) Where(context.Context, pgx.Tx, string) (string, error) {
	return environment.Nowhere, nil
}

func TestTheSameDayWalkedTwiceIsOneConflict(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		first := w.live(t, 5)
		second := w.live(t, 5)
		if !first.Arose() || second.Arose() {
			t.Errorf("the same day produced %+v and then %+v", first, second)
		}
	})
}

func TestNothingNewArrivesWhileSomethingIsWaiting(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		if !w.live(t, 5).Waiting {
			t.Fatal("nothing is waiting after the first day")
		}
		// §3.15's "good rules buy time, and the holes in your playbook are
		// exactly what demands your attention", as a mechanic: a week away with
		// nothing covered is one decision waiting rather than seven.
		for day := 6; day <= 12; day++ {
			if arose := w.live(t, day); arose.Arose() {
				t.Fatalf("day %d piled %+v on top of an unresolved one", day, arose)
			}
		}
		standing, is := w.waiting(t)
		if !is {
			t.Fatal("the one thing waiting went missing")
		}
		w.answer(t, standing.ID, standing.Pathway.Answers()[0].Stance)
		if arose := w.live(t, 13); !arose.Arose() {
			t.Error("the day after it was settled produced nothing")
		}
	})
}

/* ── the rules settle what they cover ─────────────────────────────────── */

func TestRulesSettleWhatTheyCoverAndTheLogSaysSo(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		w.covering(t)

		arose := w.live(t, 5)
		if !arose.Arose() {
			t.Fatal("nothing arose")
		}
		if arose.Waiting {
			t.Fatalf("%+v reached the player, whose rules answer every question", arose)
		}
		if arose.Handled == "" || arose.Fire == 0 {
			t.Fatalf("%+v was settled by nothing and recorded nowhere", arose)
		}
		if _, is := w.waiting(t); is {
			t.Error("something is in front of a player whose rules handled it")
		}

		// §3.14's step 2: the review surface has the moment, who it was with,
		// and what came of it — joined from here, because internal/playbook
		// holds which rule fired and this holds what was going on.
		told, err := w.m.Told(w.ctx, w.tx, w.account, []int64{arose.Fire})
		if err != nil {
			t.Fatalf("told: %v", err)
		}
		line, held := told[arose.Fire]
		if !held || line.Name == "" || line.Line == "" || line.Came == "" || line.Who == "" {
			t.Fatalf("the log line is %+v", line)
		}
		if strings.Contains(line.Line, Who) {
			t.Error("the log line still has a placeholder where the name goes")
		}

		// And the authored deltas were paid (§3.5).
		moved := false
		for _, f := range simulation.Factors {
			if f.Granted() && w.paid.credit(f) != 0 {
				moved = true
			}
		}
		if !moved {
			t.Error("a settled conflict moved nothing")
		}
	})
}

func TestAnUncoveredConflictIsAnsweredByHandAndMovesTheLedger(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		arose := w.live(t, 5)
		standing, is := w.waiting(t)
		if !is {
			t.Fatal("nothing is waiting")
		}
		if standing.Line == "" || standing.Who.Name == "" || standing.Who.Does == "" {
			t.Fatalf("the live decision is %+v", standing)
		}
		if standing.Note == "" {
			t.Error("nothing says why it reached the player")
		}
		if len(standing.Answers()) < 2 {
			t.Errorf("%d things the player could do about it", len(standing.Answers()))
		}

		// A live decision that cannot be found again after closing the tab was
		// never live (§5.1's property, applied here).
		if again, _ := w.waiting(t); again.ID != standing.ID {
			t.Fatalf("it was a different conflict on the second look: %d then %d", standing.ID, again.ID)
		}

		before := w.standing(t, arose.Person)
		var moving Answer
		for _, a := range standing.Pathway.Answers() {
			if a.Shift.Moves() {
				moving = a
			}
		}
		if moving.Stance == "" {
			t.Skip("nothing this pathway can be met with changes the ledger")
		}
		answered := w.answer(t, standing.ID, moving.Stance)
		// The name is filled in on the way out, which is what the player reads
		// and what the log will read back.
		if answered.Came == "" {
			t.Fatal("nothing came of it")
		}
		if strings.Contains(answered.Came+answered.Did, Who) {
			t.Errorf("the answer still has a placeholder in it: %q", answered.Did)
		}
		after := w.standing(t, arose.Person)
		if after == before {
			t.Errorf("answering changed nothing between them: %+v", after)
		}
		if _, is := w.waiting(t); is {
			t.Error("it is still waiting after it was answered")
		}

		// Twice is refused, and so is a posture nobody wrote an answer for.
		if _, err := w.m.Answer(w.ctx, w.tx, w.account, standing.ID, moving.Stance); err == nil {
			t.Error("it was answered twice")
		}
	})
}

/* ── it follows from what actually happened ───────────────────────────── */

func TestFrictionFollowsFromTheLedgerRatherThanFromARoll(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		people, _ := environment.Lookup(among)
		// Somebody the player owes a great deal, and has left things hanging
		// with. §3.15's first line: friction follows from actual state.
		owed := people.People()[4]
		if err := w.m.save(w.ctx, w.tx, w.account, among, Standing{
			Person: owed.Key, Owed: -4, Unsettled: 2, Met: 3, Last: on(4),
		}); err != nil {
			t.Fatalf("save: %v", err)
		}

		arose := w.live(t, 5)
		if arose.Person != owed.Key {
			t.Fatalf("the day was with %q; the person with everything outstanding is %q",
				arose.Person, owed.Key)
		}
		// And it is about what is outstanding, not about something else.
		if arose.Strain != Owing {
			t.Errorf("it followed from %q rather than from the debt", arose.Strain)
		}
	})
}

func TestTheSamePlayerOnTheSameDayGetsTheSameConflict(t *testing.T) {
	t.Parallel()
	// §7: nothing in this path is random, so a rollover that is retried or
	// replayed produces one world rather than two. Two databases, one account
	// id, one day.
	first, second := "", ""
	for _, into := range []*string{&first, &second} {
		with(t, func(w *world) {
			// The account id is what the hash is over, so both runs use one.
			if _, err := w.tx.Exec(w.ctx,
				`UPDATE account SET id = '00000000-0000-0000-0000-0000000000aa' WHERE id = $1`,
				w.account); err != nil {
				t.Fatalf("rename account: %v", err)
			}
			w.account = "00000000-0000-0000-0000-0000000000aa"
			arose := w.live(t, 9)
			*into = arose.Person + "/" + arose.Conflict
		})
	}
	if first != second || first == "" {
		t.Errorf("the same day produced %q and then %q", first, second)
	}
}

/* ── and a fortnight of it is a pattern ───────────────────────────────── */

func TestAFortnightOfLivingSomewhereReadsAsAPattern(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		w.covering(t)
		for day := 1; day <= 14; day++ {
			w.live(t, day)
		}

		living := w.living(t, 14)
		if !living.Among() {
			t.Fatal("the player is among nobody")
		}
		people, _ := environment.Lookup(among)
		if len(living.People) != len(people.People()) {
			t.Errorf("%d of %d people are on the surface", len(living.People), len(people.People()))
		}
		if len(living.Holds) == 0 {
			t.Error("nothing says what these people have in common")
		}

		met := 0
		for _, n := range living.People {
			met += n.Met
			if n.Person.Does == "" {
				t.Errorf("%q is legible from nothing they do", n.Person.Key)
			}
		}
		if met != 14 {
			t.Errorf("%d conflicts over fourteen days", met)
		}
		// More than one person, which is the property the staleness term in the
		// pressure exists for: a population rather than one neighbour.
		seen := 0
		for _, n := range living.People {
			if n.Met > 0 {
				seen++
			}
		}
		if seen < 3 {
			t.Errorf("a fortnight was spent on %d of the six", seen)
		}

		// §3.6's pattern: which question keeps arriving, and how many of them
		// the player's own rules covered. Every one of these was covered,
		// because the rules answer everything.
		if len(living.Questions) == 0 {
			t.Fatal("a fortnight of conflicts adds up to no pattern at all")
		}
		total, covered := 0, 0
		for _, q := range living.Questions {
			total += q.Met
			covered += q.Covered
		}
		if total != 14 || covered != 14 {
			t.Errorf("%d conflicts, %d of them covered; the rules answer every question", total, covered)
		}
	})
}

func TestWhatTheRulesDoNotCoverIsCountedAsUncovered(t *testing.T) {
	t.Parallel()
	with(t, func(w *world) {
		// No rules at all, so the first day escalates and everything stops
		// behind it — which is itself the shape of a playbook full of holes.
		w.live(t, 5)
		living := w.living(t, 5)
		total, covered := 0, 0
		for _, q := range living.Questions {
			total += q.Met
			covered += q.Covered
		}
		if total != 1 || covered != 0 {
			t.Errorf("%d conflicts, %d covered; a player with no rules covers nothing", total, covered)
		}
	})
}
