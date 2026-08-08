package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/career"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/challenge"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
Jobs through the transport that ships them: a real router, a real Postgres, and
the same commands the browser sends (§14).

What is worth proving here is what neither internal/career's tests nor
internal/simulation's can: that a wage arrives because a day ended and spends
none of §3.4's paid-action allowance, that the interview is a door the world
hides from anybody it is not for, and that getting through it hires somebody
inside the same transaction as the last thing they said.
*/

// jobView is the job surface as it comes off the wire, which is the only shape
// a client ever sees it in.
type jobView struct {
	Held *struct {
		Tier   int    `json:"tier"`
		Key    string `json:"key"`
		Name   string `json:"name"`
		Pay    int64  `json:"pay"`
		Hours  int    `json:"hours"`
		Nights bool   `json:"nights"`
	} `json:"held"`
	Since  string `json:"since"`
	Paid   int64  `json:"paid"`
	Ladder []struct {
		Tier int    `json:"tier"`
		Key  string `json:"key"`
		Pay  int64  `json:"pay"`
	} `json:"ladder"`
	Next  *openView  `json:"next"`
	Here  *openView  `json:"here"`
	Board []openView `json:"board"`
}

type openView struct {
	Role struct {
		Tier int    `json:"tier"`
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"role"`
	Wants []struct {
		Label string `json:"label"`
		State string `json:"state"`
	} `json:"wants"`
	Ready bool `json:"ready"`
	Walk  *struct {
		Place string `json:"place"`
		Zone  string `json:"zone"`
	} `json:"walk"`
	Take      bool   `json:"take"`
	Interview string `json:"interview"`
}

// job reads the job surface out of the world snapshot.
func job(t *testing.T, s *Server, as *http.Cookie) jobView {
	t.Helper()
	var out struct {
		Career jobView `json:"career"`
	}
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	return out.Career
}

// decodeJob reads what one of the two commands answered with.
func decodeJob(t *testing.T, rec *httptest.ResponseRecorder) (out struct {
	Career    jobView `json:"career"`
	Character struct {
		Money        int64 `json:"money"`
		EarningsLeft int   `json:"earningsLeft"`
	} `json:"character"`
}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("not a job: %v — %s", err, rec.Body)
	}
	return out
}

// onTheBay walks the player onto the loading bay and takes the night shift,
// which is where every career in this game starts.
func onTheBay(t *testing.T, s *Server, as *http.Cookie, key string) {
	t.Helper()
	if rec := post(t, s, as, "character.move", key+"-walk",
		`{"to":"the-strip-loading-bay","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walk to the bay = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "job.take", key+"-take", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("take the shift = %d, body = %s", rec.Code, rec.Body)
	}
}

/*
The ladder is on the world snapshot from the first page load, whole.

§3.14: "the ladder itself teaches the trade: more money costs more of your
attention." A ladder somebody can see one rung of teaches nothing, so every
role arrives — most of them jobs this player cannot have — with its day rate
and its hours. A rung can carry more than one role since the east side got its
own work, so the ladder is longer than the rung table and never out of order.
*/
func TestTheWholeLadderIsOnTheWorldFromTheStart(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	held := job(t, s, as)

	if held.Held != nil {
		t.Errorf("a new player holds %+v", held.Held)
	}
	if held.Paid != 0 {
		t.Errorf("holding nothing pays %d a day", held.Paid)
	}
	if len(held.Ladder) != len(career.Roles) {
		t.Fatalf("%d rungs sent, want every role (%d)", len(held.Ladder), len(career.Roles))
	}
	tiers := map[int]bool{}
	for i, rung := range held.Ladder {
		if rung.Tier <= 0 || rung.Pay <= 0 || rung.Key == "" {
			t.Errorf("rung %d = %+v", i, rung)
		}
		if i > 0 && rung.Tier < held.Ladder[i-1].Tier {
			t.Errorf("the ladder is out of order at %d: %+v", i, rung)
		}
		tiers[rung.Tier] = true
	}
	for tier := 1; tier <= len(simulation.Rungs()); tier++ {
		if !tiers[tier] {
			t.Errorf("no role on the ladder at tier %d", tier)
		}
	}
	// The step up from nothing is the bottom rung, and it asks for nothing —
	// which is what makes it the bottom rung.
	if held.Next == nil || held.Next.Role.Tier != 1 {
		t.Fatalf("the step up from holding nothing = %+v", held.Next)
	}
	if len(held.Next.Wants) != 0 || !held.Next.Ready {
		t.Errorf("the bottom rung asks %+v and is ready = %v", held.Next.Wants, held.Next.Ready)
	}
	// And it is not on offer from the east side, because a job is taken where
	// it is (§3.12).
	if held.Here != nil {
		t.Errorf("somebody is hiring in the east side: %+v", held.Here)
	}
}

/*
A job is walked into where it is, and taking it from anywhere else is refused.

Placement is the server's, exactly as an event's and a challenge's are. The
command carries no argument at all, so there is nothing for a client to name.
*/
func TestAJobIsTakenWhereItIs(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	if rec := post(t, s, as, "job.take", "k1", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("taking a job from the east side = %d, body = %s", rec.Code, rec.Body)
	}

	if rec := post(t, s, as, "character.move", "k2",
		`{"to":"the-strip-loading-bay","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walk to the bay = %d, body = %s", rec.Code, rec.Body)
	}

	// Standing on it, the vacancy is on the surface before anything is done.
	if here := job(t, s, as).Here; here == nil || here.Role.Tier != 1 || here.Walk == nil {
		t.Fatalf("standing on the bay, what is on offer = %+v", here)
	}

	rec := post(t, s, as, "job.take", "k3", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("take the shift = %d, body = %s", rec.Code, rec.Body)
	}
	took := decodeJob(t, rec)
	if took.Career.Held == nil || took.Career.Held.Tier != 1 {
		t.Fatalf("holding %+v after taking the night shift", took.Career.Held)
	}
	if !took.Career.Held.Nights || took.Career.Held.Hours == 0 {
		t.Errorf("the bottom rung is %+v, and §3.14 says it is nights", took.Career.Held)
	}
	if took.Career.Paid <= 0 || took.Career.Since == "" {
		t.Errorf("a job that pays %d a day, started %q", took.Career.Paid, took.Career.Since)
	}

	// Twice is a refusal rather than a second job.
	if rec := post(t, s, as, "job.take", "k4", `{}`); rec.Code != http.StatusConflict {
		t.Errorf("taking a second job = %d, want 409", rec.Code)
	}
}

/*
§3.4, both halves in one test.

**Income is structural**: the wage arrives because a day ended, from a rollover
nobody asked for, at the rung's day rate. **The cap applies only to earning
actions**: being paid four days' wages spends none of the three paid actions a
day has, and the player can still take all three.
*/
func TestAWageArrivesFromTheDayEndingAndIsNotAPaidAction(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	onTheBay(t, s, as, "k")
	// The sign-in happened before the clock was stopped, so the rollover's own
	// ledger is stamped with the real day rather than with this one. Put it
	// where the stopped clock says yesterday was, so that what is measured
	// below is the four days that follow and not a boundary nobody chose.
	upTo(t, st, accountOf(t, st), "2026-08-05")
	before := money(t, st, accountOf(t, st))

	// Four days pass with nobody looking, and the next page load catches them
	// up (§3.3) — which is the only way a wage is ever paid.
	clk.At = clk.At.AddDate(0, 0, 4)
	world := snapshot(t, s, as)

	rung, _ := simulation.Rungs().At(1)
	if want := before + 4*rung.Pay.Cents(); world.Character.Money != want {
		t.Fatalf("money = %d after four nights, want %d", world.Character.Money, want)
	}
	// Nothing was taken off the day's allowance, because a wage is not an
	// action taken.
	if world.Character.EarningsLeft != simulation.PaidActionsPerDay {
		t.Errorf("earnings left = %d after four days of wages, want %d",
			world.Character.EarningsLeft, simulation.PaidActionsPerDay)
	}

	// Reading the world again pays nothing more: the ledger is what makes a
	// wage idempotent in the date, and a page load is not a day.
	held := money(t, st, accountOf(t, st))
	_ = snapshot(t, s, as)
	if again := money(t, st, accountOf(t, st)); again != held {
		t.Errorf("reading the world paid another %d cents", again-held)
	}
}

/*
§3.14: "it costs you health if your routines are bad."

The costing is internal/simulation's and the seam is internal/rollover's; what
this proves is that the whole chain is wired — the same routines, the same days,
with and without the night shift, leave two different bodies. Two players rather
than one, because a body cannot be un-lived.
*/
func TestHoldingTheNightShiftAgesABodyFasterThanHoldingNothing(t *testing.T) {
	t.Parallel()

	aged := func(t *testing.T, working bool) float64 {
		t.Helper()
		s, _, as := signedIn(t)
		clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

		// The same rules for the body in both worlds. Everything protected, so
		// the only difference left is the job.
		set := `{"choices":{"sleep":"protected","workout":"split","diet":"cooked","organization":"listed"}}`
		if rec := post(t, s, as, "routine.set", "r", set); rec.Code != http.StatusOK {
			t.Fatalf("set routines = %d, body = %s", rec.Code, rec.Body)
		}
		if working {
			onTheBay(t, s, as, "k")
		}

		clk.At = clk.At.AddDate(0, 0, 14)
		return healthOf(t, s, as).BiologicalAge
	}

	free, nights := aged(t, false), aged(t, true)
	t.Logf("after a fortnight: no job %.1f, warehouse nights %.1f", free, nights)
	if nights <= free {
		t.Errorf("a fortnight of nights read %.1f and a fortnight of nothing read %.1f", nights, free)
	}
}

/*
§3.14's step up, end to end: a door the world hides until somebody is standing
one rung below it with the capability, and an interview behind the door.

The whole of it is played over the real commands, which is the only way to prove
that internal/challenge does not know it is running an interview and
internal/career does not know an interview is a challenge.
*/
func TestTheInterviewIsHiddenUntilItIsYoursToSitAndHiresYouWhenYouPassIt(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	// Standing at the desk with no job at all: nothing to see, and the command
	// refuses even though the challenge stands there.
	if rec := post(t, s, as, "character.move", "m1",
		`{"to":"the-strip-warehouse","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walk to the warehouse = %d, body = %s", rec.Code, rec.Body)
	}
	if door := doorway(t, s, as); door != "" {
		t.Fatalf("the interview stands in front of somebody with no job: %q", door)
	}
	if rec := post(t, s, as, "challenge.begin", "b1",
		`{"challenge":"interview/a"}`); rec.Code != http.StatusConflict {
		t.Fatalf("beginning an interview nobody may sit = %d, body = %s", rec.Code, rec.Body)
	}

	// On the bottom rung, with a body that has not yet been ruined by it, the
	// door is there — and so is the requirement, veiled.
	onTheBay(t, s, as, "k")
	if rec := post(t, s, as, "character.move", "m2",
		`{"to":"the-strip-warehouse","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walk back to the warehouse = %d, body = %s", rec.Code, rec.Body)
	}

	step := job(t, s, as).Next
	if step == nil || step.Role.Tier != 2 {
		t.Fatalf("the step up from the bay = %+v", step)
	}
	if len(step.Wants) == 0 {
		t.Fatal("the step up asks for nothing")
	}
	blanked := 0
	for _, w := range step.Wants {
		switch w.State {
		case "hidden":
			blanked++
			if w.Label != "" {
				t.Errorf("a blanked requirement is labelled %q", w.Label)
			}
		case "met", "unmet":
			if w.Label == "" {
				t.Errorf("a shown requirement has no label: %+v", w)
			}
		default:
			t.Errorf("a requirement is in the state %q", w.State)
		}
	}
	if blanked == 0 {
		t.Error("nothing about the step up is hidden, so §3.5's factors are all on screen")
	}

	// A fresh player has no credit at all, so the hidden floors are not
	// reached: the door is visible and the challenge is not sittable.
	if step.Ready {
		t.Fatal("somebody who has done nothing reaches the requirement for tier 2")
	}
	if door := doorway(t, s, as); door != "" {
		t.Fatalf("the interview stands in front of somebody it would refuse: %q", door)
	}

	// Give them what the rung asks for the only way the game ever does: an
	// authored delta from a committed choice (§3.5, §5.1).
	capable(t, s, st)
	if step := job(t, s, as).Next; step == nil || !step.Ready || step.Interview != "interview/a" {
		t.Fatalf("with the capability, the step up = %+v", step)
	}
	if door := doorway(t, s, as); door != "interview/a" {
		t.Fatalf("the door in front of a capable player is %q", door)
	}

	// Sit it, and take the best answer at every scene, which is the passage
	// that ends in the job.
	if rec := post(t, s, as, "challenge.begin", "b2",
		`{"challenge":"interview/a"}`); rec.Code != http.StatusOK {
		t.Fatalf("begin the interview = %d, body = %s", rec.Code, rec.Body)
	}
	best(t, s, as, "interview/a")

	held := job(t, s, as)
	if held.Held == nil || held.Held.Tier != 2 {
		t.Fatalf("after passing the interview, holding %+v", held.Held)
	}
	second, _ := simulation.Rungs().At(2)
	if held.Paid != second.Pay.Cents() {
		t.Errorf("the desk pays %d a day, want %d", held.Paid, second.Pay.Cents())
	}

	// And the door is gone, because a challenge is once ever (§5.1).
	if door := doorway(t, s, as); door != "" {
		t.Errorf("the interview is still standing there: %q", door)
	}
}

/*
Leaving a job stops the wages and keeps the rung reached.

The second half is the one that matters: an interview is a challenge and a
challenge is once ever (§5.1), so a player who walked away from the desk can
walk back into it rather than being asked for something the game cannot give
them a second time.
*/
func TestLeavingAJobIsRealAndDoesNotUndoTheInterview(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	clk := stopped(t, s, time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC))

	if rec := post(t, s, as, "job.leave", "l0", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("leaving a job nobody holds = %d, body = %s", rec.Code, rec.Body)
	}

	onTheBay(t, s, as, "k")
	capable(t, s, st)
	if rec := post(t, s, as, "character.move", "m1",
		`{"to":"the-strip-warehouse","by":"foot"}`); rec.Code != http.StatusOK {
		t.Fatalf("walk to the warehouse = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "challenge.begin", "b1",
		`{"challenge":"interview/a"}`); rec.Code != http.StatusOK {
		t.Fatalf("begin the interview = %d, body = %s", rec.Code, rec.Body)
	}
	best(t, s, as, "interview/a")

	rec := post(t, s, as, "job.leave", "l1", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("leave = %d, body = %s", rec.Code, rec.Body)
	}
	if left := decodeJob(t, rec); left.Career.Held != nil || left.Career.Paid != 0 {
		t.Fatalf("after leaving: holding %+v at %d a day", left.Career.Held, left.Career.Paid)
	}

	held := money(t, st, accountOf(t, st))
	clk.At = clk.At.AddDate(0, 0, 3)
	if world := snapshot(t, s, as); world.Character.Money != held {
		t.Errorf("three days of unemployment paid %d cents", world.Character.Money-held)
	}

	// Standing at the desk again, it is on offer — with no interview to sit.
	here := job(t, s, as).Here
	if here == nil || here.Role.Tier != 2 || here.Walk == nil {
		t.Fatalf("standing at the desk after leaving it, what is on offer = %+v", here)
	}
	if rec := post(t, s, as, "job.take", "t2", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("walk back into the desk job = %d, body = %s", rec.Code, rec.Body)
	}
	if again := job(t, s, as); again.Held == nil || again.Held.Tier != 2 {
		t.Errorf("walked back into %+v", again.Held)
	}
}

/* ── helpers ──────────────────────────────────────────────────────────── */

// upTo stamps the rollover ledger, so a test that stopped the clock decides
// which days are owed rather than inheriting the real one's answer.
func upTo(t *testing.T, st *store.Store, accountID, day string) {
	t.Helper()
	if _, err := st.Pool().Exec(t.Context(),
		`UPDATE rollover SET last_day = $2::date WHERE account_id = $1`, accountID, day,
	); err != nil {
		t.Fatalf("move the ledger: %v", err)
	}
}

// doorway is the challenge standing where the player is, by code, and empty for
// nothing.
func doorway(t *testing.T, s *Server, as *http.Cookie) string {
	t.Helper()
	var out struct {
		Challenge struct {
			Here *struct {
				Challenge string `json:"challenge"`
			} `json:"here"`
		} `json:"challenge"`
	}
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("snapshot is not a world: %v", err)
	}
	if out.Challenge.Here == nil {
		return ""
	}
	return out.Challenge.Here.Challenge
}

/*
capable gives the player the hidden credit the dispatch desk asks for.

Written straight into internal/factor's table rather than played for, and that
is deliberate rather than lazy: the only way to earn it in the game is to finish
a challenge, a challenge is once ever, and there are two of them — so a test
that earned it would be a test about the challenge catalogue rather than about
the ladder. What is being proved here is that the gate reads the factors, not
how the factors got there.
*/
func capable(t *testing.T, s *Server, st *store.Store) {
	t.Helper()
	rung, _ := simulation.Rungs().At(2)
	account := accountOf(t, st)

	for _, which := range rung.Needs.Wants() {
		if which.Shown() {
			continue
		}
		// The credit a reading of `floor` corresponds to, inverted off the pure
		// core's own curve so a tuning pass moves both together.
		floor := rung.Needs.Of(which)
		credit := simulation.CreditHalf * floor / (100 - floor)
		if _, err := st.Pool().Exec(t.Context(), `
			INSERT INTO factor_credit (account_id, factor, credit)
			VALUES ($1, $2, $3)
			ON CONFLICT (account_id, factor) DO UPDATE SET credit = EXCLUDED.credit`,
			account, which.Key(), credit*1.2,
		); err != nil {
			t.Fatalf("grant %s: %v", which, err)
		}
	}
}

/*
best plays a whole passage of a challenge, taking the highest-weighted option at
every scene.

It reads the plan out of the catalogue to find them, which nothing outside a test
may do — the weights are hidden scoring (§3.10) and there is no path from the
wire to them. That is the point of it being here: the test needs the best
passage and a player has to find it.
*/
func best(t *testing.T, s *Server, as *http.Cookie, code string) {
	t.Helper()
	authored, known := challenge.Known(challenge.Code(code))
	if !known {
		t.Fatalf("%s is not authored", code)
	}

	scene := authored.Plan.Opening
	for step := range 12 {
		at, ok := authored.Plan.Scene(scene)
		if !ok {
			t.Fatalf("%s is not a scene of %s", scene, code)
		}
		pick := at.Options[0]
		for _, o := range at.Options {
			if o.Weight > pick.Weight {
				pick = o
			}
		}

		body := `{"challenge":"` + code + `","scene":"` + scene + `","option":"` + pick.Key + `"}`
		rec := post(t, s, as, "challenge.choose", newKey(step), body)
		if rec.Code != http.StatusOK {
			t.Fatalf("choosing %s at %s = %d, body = %s", pick.Key, scene, rec.Code, rec.Body)
		}
		if pick.Then == "" {
			return
		}
		scene = pick.Then
	}
	t.Fatalf("%s did not finish in twelve scenes", code)
}

/*
A job is taken off the board, from wherever the player is standing.

2026-08-08: hiring happens on the job surface, the way people actually get
hired — nobody walks across a city to be allowed to say yes. The map spot
stays as the other door onto the same rung (TestAJobIsTakenWhereItIs); this
one moves nobody an inch.
*/
func TestAJobIsTakenOffTheBoardWithoutMoving(t *testing.T) {
	t.Parallel()
	s, _, as := signedIn(t)

	// The board is on the surface from the first read: a posting for every
	// role with a way in, and none for the rungs that have none yet.
	posted := map[string]openView{}
	for _, o := range job(t, s, as).Board {
		posted[o.Role.Key] = o
	}
	for _, key := range []string{
		"warehouse-nights", "corner-shop-nights", "tower-lobby-nights",
		"dispatch", "clinic-front-desk", "operations",
	} {
		if _, on := posted[key]; !on {
			t.Errorf("%s is not posted on the board", key)
		}
	}
	if len(posted) != 6 {
		t.Errorf("the board posts %d roles, want 6", len(posted))
	}
	// A walk-in is takeable from a kitchen. An interview rung is not takeable,
	// and its interview is not sittable by somebody holding nothing.
	if o := posted["corner-shop-nights"]; !o.Take || !o.Ready {
		t.Errorf("the corner shop posting = %+v", o)
	}
	if o := posted["dispatch"]; o.Take || o.Interview != "" {
		t.Errorf("the dispatch posting = %+v", o)
	}

	// Take the corner shop from the apartment.
	rec := post(t, s, as, "job.take", "board-1", `{"role":"corner-shop-nights"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("take off the board = %d, body = %s", rec.Code, rec.Body)
	}
	if out := decodeJob(t, rec); out.Career.Held == nil || out.Career.Held.Key != "corner-shop-nights" {
		t.Fatalf("after the board, holding %+v", out.Career.Held)
	}

	// The job held is not a posting, and a second job is refused whichever
	// door it is asked for through.
	posted = map[string]openView{}
	for _, o := range job(t, s, as).Board {
		posted[o.Role.Key] = o
	}
	if _, on := posted["corner-shop-nights"]; on {
		t.Error("the job held is still posted on the board")
	}
	if o, on := posted["warehouse-nights"]; !on || o.Take {
		t.Errorf("the other tier-1 posting while working = %+v", o)
	}
	if rec := post(t, s, as, "job.take", "board-2",
		`{"role":"warehouse-nights"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a second job off the board = %d, body = %s", rec.Code, rec.Body)
	}

	// And the board refuses what it does not post: an unknown role is wrong
	// forever, a rung with no way in is the world saying no.
	if rec := post(t, s, as, "job.leave", "board-3", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("leave = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "job.take", "board-4",
		`{"role":"astronaut"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown role = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "job.take", "board-5",
		`{"role":"regional"}`); rec.Code != http.StatusConflict {
		t.Fatalf("a rung with no way in = %d, body = %s", rec.Code, rec.Body)
	}
	if rec := post(t, s, as, "job.take", "board-6",
		`{"role":"dispatch"}`); rec.Code != http.StatusConflict {
		t.Fatalf("an interview rung off the board = %d, body = %s", rec.Code, rec.Body)
	}
}

/*
An interview is sat from the kitchen (2026-08-08: interviews happen online).

The door is career-gated exactly as it always was — held one rung below,
requirement reached — and no longer place-gated: the challenge's authored
`where` is its standing-place on the map, and the board opens the same door
from anywhere. Everything that is not an interview still has to be stood in
front of (the place gate lives on in challenge_test.go).
*/
func TestAnInterviewIsSatFromTheKitchen(t *testing.T) {
	t.Parallel()
	s, st, as := signedIn(t)
	stopped(t, s, time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC))

	// An east-side career so far: the till off the board, nobody moving.
	if rec := post(t, s, as, "job.take", "k1", `{"role":"corner-shop-nights"}`); rec.Code != http.StatusOK {
		t.Fatalf("take the till = %d, body = %s", rec.Code, rec.Body)
	}
	capable(t, s, st)

	// Both tier-2 postings now carry their interviews, sittable from here —
	// the rung's requirement is the rung's, whichever counter asks it.
	posted := map[string]openView{}
	for _, o := range job(t, s, as).Board {
		posted[o.Role.Key] = o
	}
	if o := posted["clinic-front-desk"]; o.Interview != "interview/c" || !o.Ready {
		t.Fatalf("the clinic posting = %+v", o)
	}
	if o := posted["dispatch"]; o.Interview != "interview/a" {
		t.Fatalf("the dispatch posting = %+v", o)
	}

	// The interview is not what stands in the apartment — whatever does (the
	// stoop has its own life) — and it begins anyway, and passing it hires:
	// same transaction, same rules.
	if door := doorway(t, s, as); door == "interview/c" {
		t.Fatal("the clinic's interview is standing in the kitchen")
	}
	if rec := post(t, s, as, "challenge.begin", "k2",
		`{"challenge":"interview/c"}`); rec.Code != http.StatusOK {
		t.Fatalf("sit the interview from the kitchen = %d, body = %s", rec.Code, rec.Body)
	}
	best(t, s, as, "interview/c")

	held := job(t, s, as)
	if held.Held == nil || held.Held.Key != "clinic-front-desk" || held.Held.Tier != 2 {
		t.Fatalf("after the interview, holding %+v", held.Held)
	}
	second, _ := simulation.Rungs().At(2)
	if held.Paid != second.Pay.Cents() {
		t.Errorf("the front desk pays %d a day, want %d", held.Paid, second.Pay.Cents())
	}
}

// Compile-time note: career.Roles is what the ladder above is a view of, and a
// role with no rung would be a screen with a gap in it. internal/career asserts
// that; this is only here so the import reads as deliberate.
var _ = career.Roles
