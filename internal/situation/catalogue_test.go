package situation

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/playbook"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The authored situations, as content rather than as machinery.

What is worth proving here is not that the loader loads — the module's tests do
that against a database. It is the set of claims §3.14 makes about what a job
situation *is*, checked against the ones that exist: a delegated rung produces
them, they speak the matcher's own vocabulary, every posture the player's rules
could reach has a consequence written for it, and at least one of them makes an
over-general rule cost something.

Every one of those is a thing an author can quietly break while editing prose,
and none of them is visible in a diff.
*/

func TestEverySituationIsPlayable(t *testing.T) {
	all := All()
	if len(all) < 2 {
		t.Fatalf("%d situations are authored; a pool of one is the same morning forever", len(all))
	}

	for _, a := range all {
		if a.Name == "" || a.Line == "" {
			t.Errorf("%s has nothing said about it", a.Key)
		}
		if !a.Tier.Held() || a.Tier.Passive() {
			t.Errorf("%s is tier %d, which is not a delegated role", a.Key, a.Tier)
		}
		if _, on := simulation.Rungs().At(a.Tier); !on {
			t.Errorf("%s is tier %d, which is not on the ladder", a.Key, a.Tier)
		}
		if len(a.Puts()) == 0 {
			t.Errorf("%s puts no question, so no rule could ever answer it", a.Key)
		}
		// The moment has to be one the matcher will accept, or [Playbooks.Weigh]
		// refuses it and the role produces work nobody's rules can touch.
		if faults := a.Moment().Faults(); len(faults) > 0 {
			t.Errorf("%s states %v, which the vocabulary cannot read", a.Key, faults)
		}
		if a.Moment().Key() != a.Key {
			t.Errorf("%s arises as %q", a.Key, a.Moment().Key())
		}
	}
}

/*
§3.14's demand that failure be legible, from the world's side: whatever the
player's rules amount to, the world has a response written for it.

A moment whose rules could reach a posture nobody authored a consequence for
would surface as a rule that silently did nothing — which is the exact failure
the whole structured-rule argument exists to prevent (technical.md §6).
*/
func TestEveryPostureAMomentCanReachHasAConsequence(t *testing.T) {
	vocab := playbook.Vocab()

	for _, a := range All() {
		reachable := map[playbook.Stance]bool{}
		for _, key := range a.Puts() {
			q, ok := vocab.Question(key)
			if !ok {
				t.Errorf("%s puts %q, which the world does not ask", a.Key, key)
				continue
			}
			for _, term := range q.Actions() {
				reachable[term.Stance] = true
			}
		}

		written := map[playbook.Stance]bool{}
		for _, answer := range a.Answers() {
			if written[answer.Stance] {
				t.Errorf("%s answers %q twice", a.Key, answer.Stance)
			}
			written[answer.Stance] = true

			if !reachable[answer.Stance] {
				t.Errorf("%s answers %q, which none of its questions can reach", a.Key, answer.Stance)
			}
			if answer.Did == "" || answer.Came == "" {
				t.Errorf("%s's %q answer does not say what was done or what came of it", a.Key, answer.Stance)
			}
			switch answer.Outcome {
			case playbook.Good, playbook.Mixed, playbook.Poor:
			default:
				t.Errorf("%s's %q answer came to %q", a.Key, answer.Stance, answer.Outcome)
			}
			// §3.5: only the three factors that are *granted* from committed
			// choices. A job that moved a body or a balance would be writing
			// behind the two modules that own them.
			for _, g := range answer.Moves {
				if !g.Valid() {
					t.Errorf("%s's %q answer moves %s, which is read rather than granted",
						a.Key, answer.Stance, g.Factor.Key())
				}
			}
		}

		for stance := range reachable {
			if !written[stance] {
				t.Errorf("%s can be met with %q and has no answer for it", a.Key, stance)
			}
		}
	}
}

/*
The ticket's sixth line, and §3.14's own: **over-general rules must backfire,
not win.**

	Players will compose "always maximise X." The correct response is a
	situation where exactly that rule costs them something — which is now
	something to author deliberately rather than hope for.

The over-general rule is the one with the same posture on both sides of it, and
the cheapest one to write is "go along with it" — say yes, accept it, give them
something. So there has to be at least one authored moment in which yielding is
the poor outcome, and it has to be at a rung a player can actually reach.

Checked as a property of the catalogue rather than of one file, so that
rewriting `the-roster-gap` into something gentler fails here rather than
quietly removing the lesson.
*/
func TestAnOverGeneralRuleIsMadeToCostSomething(t *testing.T) {
	backfires := map[simulation.Tier][]string{}
	for _, a := range All() {
		answer, written := a.Answer(playbook.Yield)
		if written && answer.Outcome == playbook.Poor {
			backfires[a.Tier] = append(backfires[a.Tier], a.Key)
		}
	}

	// The first delegated rung, because that is the one a player meets first
	// and the one the lesson has to land on.
	first := backfires[simulation.Delegated]
	if len(first) == 0 {
		t.Fatalf("nothing at tier %d makes going along with it cost anything; "+
			"an over-general rule wins, which §3.14 says it must not", simulation.Delegated)
	}
	t.Logf("yielding backfires at tier %d in %v", simulation.Delegated, first)

	// And it must actually cost. A poor outcome that moved nothing would be a
	// sentence about a consequence rather than a consequence.
	for _, key := range first {
		a, _ := Known(key)
		answer, _ := a.Answer(playbook.Yield)
		cost := 0.0
		for _, g := range answer.Moves {
			if g.Credit < 0 {
				cost += g.Credit
			}
		}
		if cost == 0 {
			t.Errorf("%s: going along with it goes badly and takes nothing", key)
		}
	}
}

/*
§3.5's other half, and the ticket's last line: **good rules demonstrate
judgement, feeding Intelligence and Emotional stability.**

Every situation has to have somewhere to go that pays into those two, or a
well-composed playbook is worth exactly as much as a badly composed one and the
whole loop is decoration.
*/
func TestJudgementIsWorthSomething(t *testing.T) {
	for _, a := range All() {
		best := 0.0
		for _, answer := range a.Answers() {
			if answer.Outcome != playbook.Good {
				continue
			}
			for _, g := range answer.Moves {
				if g.Factor != simulation.Intelligence && g.Factor != simulation.Stability {
					continue
				}
				if g.Credit > best {
					best = g.Credit
				}
			}
		}
		if best <= 0 {
			t.Errorf("%s has no answer that pays into judgement, so composing a good rule for it is free", a.Key)
		}
	}
}

/*
Two moments that put more than one question, because §4's second escalation
condition needs one.

"Two rules conflict" cannot arise from a moment that raises a single question —
there is exactly one rule per question per environment — so a catalogue of
single-question moments would leave a third of the escalation rule unreachable
in a delegated role, and a catch-all rule would only be checked by two of the
three conditions.
*/
func TestSomethingCanPutTwoQuestionsAtOnce(t *testing.T) {
	both := []string{}
	for _, a := range All() {
		if len(a.Puts()) > 1 {
			both = append(both, a.Key)
		}
	}
	if len(both) == 0 {
		t.Fatal("no situation puts two questions, so two of a player's answers can never disagree")
	}
	t.Logf("%v put more than one question", both)
}

/*
Every rung §3.14 calls active has work, and no rung it calls passive does.

The first half is what makes a promotion into an active tier mean something; the
second is what keeps "low tiers are genuinely passive — you hold the job, it
pays" true. Both are enforced by the loader; this is the same statement said
where a reader looking for it will find it.
*/
func TestEveryActiveRungHasWorkAndNoPassiveOneDoes(t *testing.T) {
	for _, rung := range simulation.Rungs() {
		pool := For(rung.Tier)
		switch {
		case rung.Tier.Passive() && len(pool) > 0:
			t.Errorf("tier %d pays without being played and has %d situations", rung.Tier, len(pool))
		case !rung.Tier.Passive() && len(pool) == 0:
			t.Errorf("tier %d is active and produces no work", rung.Tier)
		}
	}
	if len(For(simulation.Unemployed)) != 0 {
		t.Error("somebody with no job is being handed work")
	}
}

/* ── what the loader refuses ──────────────────────────────────────────── */

// one is a complete, valid situation file, for the refusals below to break one
// field of at a time.
const one = `
key: a
tier: 3
name: A thing
line: Something happened.
puts: [overtime]
facts:
  - { term: overtime/condition/already, so: true }
answers:
  - { stance: yield, did: You did, came: It went, outcome: mixed }
  - { stance: resist, did: You did not, came: It went, outcome: mixed }
  - { stance: bargain, did: You asked, came: It went, outcome: good, moves: { intelligence: 4 } }
`

func refuses(t *testing.T, files map[string]string, want string) {
	t.Helper()
	tree := fstest.MapFS{}
	for name, body := range files {
		tree[situationsPath+"/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	_, err := parse(tree)
	if err == nil {
		t.Fatalf("the loader accepted %v", files)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("refused with %q, want something about %q", err, want)
	}
}

/*
The loader is strict at boot rather than tolerant at run time (§5.2:
"production validates at startup so a malformed challenge fails in CI, not at
2am").

Each of these is a mistake an author can make in one line, and every one of them
would otherwise show up as a role that quietly produced nothing, a rule that
quietly never matched, or a posture that quietly did nothing.
*/
func TestTheLoaderRefusesWhatCannotBePlayed(t *testing.T) {
	t.Parallel()

	// The one that has to be accepted, or none of the refusals below mean
	// anything. It is written for tier three, so the tier-four and tier-five
	// checks are what this one trips on.
	if _, err := parse(fstest.MapFS{
		situationsPath + "/a.yaml": &fstest.MapFile{Data: []byte(one)},
	}); err == nil || !strings.Contains(err.Error(), "tier 4") {
		t.Fatalf("a lone tier-three situation: %v, want a complaint about the empty rungs above it", err)
	}

	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "tier: 3", "tier: 2", 1)},
		"passive")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "tier: 3", "tier: 9", 1)},
		"not a rung")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "puts: [overtime]", "puts: [weather]", 1)},
		"does not ask")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one,
		"overtime/condition/already", "overtime/condition/nowhere", 1)}, "not a term")
	// A fact about a question this moment is not putting: readable, and
	// untestable, which is worse than unreadable.
	refuses(t, map[string]string{"a.yaml": strings.Replace(one,
		"overtime/condition/already", "help/condition/legal", 1)}, "does not put")
	// A term that takes a number, stated as a truth.
	refuses(t, map[string]string{"a.yaml": strings.Replace(one,
		"overtime/condition/already", "overtime/condition/rate", 1)}, "cannot read")
	// A posture the questions can reach with nothing written for it.
	refuses(t, map[string]string{"a.yaml": strings.Replace(one,
		"  - { stance: bargain, did: You asked, came: It went, outcome: good, moves: { intelligence: 4 } }\n", "", 1)},
		"no answer for it")
	// And a posture they cannot reach, written anyway: content nobody will read.
	refuses(t, map[string]string{"a.yaml": one +
		"  - { stance: withdraw, did: You left, came: It went, outcome: mixed }\n"}, "none of its questions")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "outcome: good", "outcome: pending", 1)},
		"not one of")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "intelligence: 4", "health: 4", 1)},
		"read from the world")
	refuses(t, map[string]string{"a.yaml": strings.Replace(one, "intelligence: 4", "charisma: 4", 1)},
		"not a factor")
	// The key and the filename are one name.
	refuses(t, map[string]string{"b.yaml": one}, "is called")
	// And an unknown field is a failed build rather than a line with no effect.
	refuses(t, map[string]string{"a.yaml": one + "wieght: 3\n"}, "field wieght")
	// A directory with nothing in it that is a situation. Not the same as a
	// missing one, and the message has to say which.
	refuses(t, map[string]string{"notes.md": "nothing here is content"}, "holds no situations")
}
