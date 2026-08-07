package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/social"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/store"
)

/*
Peer endorsement, at the edge it is allowed to reach (§3.5).

The mechanic is proved in internal/social, against a clock a test can move. What
is worth proving here is what only the transport can show: that the offer arrives
in the world snapshot at all, that answering it is a command with no player field
in it, that the *other* player is told over the socket without having asked for
anything, and that nothing on any of those paths names a hidden factor.
*/

// twoPlayers is a server with a pair of accounts, the older of which is the one
// the returned cookie belongs to.
//
// Two is the smallest world in which an offer is possible: the candidate is
// never yourself, so a single-player server correctly has nobody to like and
// would prove nothing about the surface.
func twoPlayers(t *testing.T) (*Server, *store.Store, *http.Cookie, *http.Cookie) {
	t.Helper()
	s, st := newServer(t)
	me := signIn(t, s, "abc")
	them := signIn(t, s, "xyz")
	return s, st, me, them
}

// endorsementOf reads the endorsement surface out of a world snapshot.
func endorsementOf(t *testing.T, s *Server, as *http.Cookie) endorsementView {
	t.Helper()
	rec := read(t, s, as, worldPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out struct {
		Endorsement endorsementView `json:"endorsement"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not a world snapshot: %v", err)
	}
	return out.Endorsement
}

// answered posts one of the two commands and decodes what came back.
func answered(t *testing.T, s *Server, as *http.Cookie, name, key, body string) (*httptest.ResponseRecorder, answerResult) {
	t.Helper()
	rec := post(t, s, as, name, key, body)
	var out answerResult
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("response is not an endorsement: %v", err)
		}
	}
	return rec, out
}

// published lists the message types in one player's outbox, in order.
func published(t *testing.T, st *store.Store, accountID string) []string {
	t.Helper()
	rows, err := st.Pool().Query(t.Context(),
		`SELECT type FROM realtime_outbox WHERE account_id = $1 ORDER BY seq`, accountID)
	if err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatalf("read the outbox: %v", err)
		}
		out = append(out, kind)
	}
	return out
}

// playerNamed is the id behind a display name, which is the only handle a test
// has — deliberately, since none of these surfaces sends one.
func playerNamed(t *testing.T, st *store.Store, subject string) string {
	t.Helper()
	var id string
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT id FROM account WHERE subject = $1`, subject).Scan(&id); err != nil {
		t.Fatalf("find account %s: %v", subject, err)
	}
	return id
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

/*
The offer arrives by looking at the world, and looking again does not make
another.

This is "occasional, not on demand" as the client actually meets it: there is no
route that asks for somebody to like, and refreshing the page is not a way to
get one.
*/
func TestTheWorldSnapshotCarriesAnOfferAndASecondLookDoesNotMakeAnother(t *testing.T) {
	t.Parallel()
	s, st, me, _ := twoPlayers(t)

	first := endorsementOf(t, s, me)
	if first.Offer == nil {
		t.Fatalf("the snapshot offered nobody: %+v", first)
	}
	if first.Offer.Who.Name != "Player xyz" || first.Offer.Until == "" {
		t.Fatalf("offer = %+v, want the other player and a window", first.Offer)
	}
	if first.Cap != social.Cap || first.Given != 0 {
		t.Errorf("standing = %+v, want none of %d given", first, social.Cap)
	}

	again := endorsementOf(t, s, me)
	if again.Offer == nil || again.Offer.ID != first.Offer.ID {
		t.Fatalf("a second page load changed the offer: %+v", again.Offer)
	}

	// The draw is announced once, not once per page load — a message per
	// hydration would be the polling §8 exists instead of.
	mine := published(t, st, playerNamed(t, st, "abc"))
	offers := 0
	for _, kind := range mine {
		if kind == "social.offered" {
			offers++
		}
	}
	if offers != 1 {
		t.Errorf("%d social.offered messages for two page loads, want 1", offers)
	}
}

/*
There is no way to name the player you are endorsing.

The command takes an offer id and nothing else, which is the reason the
leaderboards were able to keep withholding account ids (#16): endorsement does
not need one. A body naming a player is not "wrong about a player" — it is a body
with no offer in it, and it is refused as such.
*/
func TestEndorsementNamesAnOfferAndNeverAPlayer(t *testing.T) {
	t.Parallel()
	s, st, me, _ := twoPlayers(t)

	them := playerNamed(t, st, "xyz")
	for i, body := range []string{
		`{}`,
		`{"offer":0}`,
		`{"player":"` + them + `"}`,
		`{"to":"` + them + `"}`,
	} {
		rec := post(t, s, me, "social.endorse", "no-offer-"+string(rune('a'+i)), body)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("POST social.endorse %s = %d, want 422", body, rec.Code)
		}
	}

	var n int
	if err := st.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM endorsement`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("%d endorsements written by requests that named no offer", n)
	}
}

/*
Endorsing moves the other player's composite, tells them so, and tells the
endorser nothing about them.

Three assertions in one test because they are one property: the credit belongs to
the player who was liked, so everything about it is addressed to them. The
endorser's own response carries their surface and no more — and their own Karma
is untouched, because §7 rules out a factor a player can award themselves.
*/
func TestEndorsingPaysTheOtherPlayerAndTellsThemWithoutTellingYou(t *testing.T) {
	t.Parallel()
	s, st, me, them := twoPlayers(t)

	before := snapshotKarma(t, s, them)
	mineBefore := snapshotKarma(t, s, me)

	offer := endorsementOf(t, s, me).Offer
	if offer == nil {
		t.Fatal("nobody to like")
	}

	rec, out := answered(t, s, me, "social.endorse", "like-1", `{"offer":`+itoa(offer.ID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if out.Endorsement.Offer != nil {
		t.Errorf("the answered offer is still standing: %+v", out.Endorsement.Offer)
	}
	if out.Endorsement.Given != 1 {
		t.Errorf("given = %d after one endorsement", out.Endorsement.Given)
	}
	// Nothing about the other player comes back to the endorser beyond the fact
	// that it worked.
	if out.Endorsement.Received != 0 || len(out.Endorsement.Liked) != 0 {
		t.Errorf("the endorser's own surface = %+v, want somebody who has received nothing", out.Endorsement)
	}

	if after := snapshotKarma(t, s, them); after.Karma <= before.Karma {
		t.Errorf("being liked read %v, no more than %v", after.Karma, before.Karma)
	}
	if mine := snapshotKarma(t, s, me); mine.Karma != mineBefore.Karma {
		t.Errorf("the endorser's composite moved from %v to %v — an endorsement pays the player who was liked (§7)",
			mineBefore.Karma, mine.Karma)
	}

	// The player who was liked did nothing at all, which is why they hear about
	// it over the socket. Their composite moved for the same reason.
	theirs := published(t, st, playerNamed(t, st, "xyz"))
	if !has(theirs, "social.liked") {
		t.Errorf("the endorsed player was told nothing: %v", theirs)
	}
	if !has(theirs, "factor.stood") {
		t.Errorf("the endorsed player's composite moved unannounced: %v", theirs)
	}

	// And their own surface now names who liked them.
	got := endorsementOf(t, s, them)
	if got.Received != 1 || len(got.Liked) != 1 || got.Liked[0].Name != "Player abc" {
		t.Fatalf("the endorsed player's surface = %+v, want one endorsement from Player abc", got)
	}
}

/*
An offer is answered once, and only by the player it was made to.

Every refusal here is a 409: the request is well formed and it is the world that
is in the way — the offer lapsed, or another tab got there first, or it was never
this player's. §8's status codes carry that difference without a message being
read.
*/
func TestAnsweringAnOfferTwiceOrSomebodyElsesIsAConflict(t *testing.T) {
	t.Parallel()
	s, _, me, them := twoPlayers(t)

	offer := endorsementOf(t, s, me).Offer
	if offer == nil {
		t.Fatal("nobody to like")
	}
	id := itoa(offer.ID)

	// Not yours — and the refusal is the same one an invented id gets, so a
	// client cannot probe for offers that exist.
	if rec := post(t, s, them, "social.endorse", "theirs-1", `{"offer":`+id+`}`); rec.Code != http.StatusConflict {
		t.Errorf("another player answering it = %d, want 409", rec.Code)
	}
	if rec := post(t, s, me, "social.endorse", "made-up-1", `{"offer":999999}`); rec.Code != http.StatusConflict {
		t.Errorf("an invented offer = %d, want 409", rec.Code)
	}

	if rec, _ := answered(t, s, me, "social.endorse", "like-1", `{"offer":`+id+`}`); rec.Code != http.StatusOK {
		t.Fatalf("endorse = %d, body = %s", rec.Code, rec.Body)
	}
	// A different intent, so a different key: a retry of the same key would be
	// replayed by the command layer rather than reaching the module (§8).
	if rec := post(t, s, me, "social.endorse", "like-2", `{"offer":`+id+`}`); rec.Code != http.StatusConflict {
		t.Errorf("answering the same offer again = %d, want 409", rec.Code)
	}
	if rec := post(t, s, me, "social.pass", "pass-1", `{"offer":`+id+`}`); rec.Code != http.StatusConflict {
		t.Errorf("passing an answered offer = %d, want 409", rec.Code)
	}
}

/*
Passing closes the offer and tells the other player nothing.

Being passed over is not a judgement anybody is notified of — which is what makes
the button worth pressing when it is pressed, because the alternative to liking
somebody is not "insult them", it is nothing at all.
*/
func TestPassingClosesTheOfferAndReachesNobodyElse(t *testing.T) {
	t.Parallel()
	s, st, me, _ := twoPlayers(t)

	offer := endorsementOf(t, s, me).Offer
	if offer == nil {
		t.Fatal("nobody to like")
	}

	rec, out := answered(t, s, me, "social.pass", "pass-1", `{"offer":`+itoa(offer.ID)+`}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if out.Endorsement.Offer != nil || out.Endorsement.Given != 0 {
		t.Fatalf("standing after passing = %+v, want no offer and nothing given", out.Endorsement)
	}
	if out.Endorsement.Opens == "" {
		t.Error("passing left the screen with no offer and no time it comes back")
	}

	// Passing does not buy a replacement.
	if got := endorsementOf(t, s, me); got.Offer != nil {
		t.Fatalf("passing produced another offer: %+v", got.Offer)
	}

	theirs := published(t, st, playerNamed(t, st, "xyz"))
	if has(theirs, "social.liked") {
		t.Errorf("the player who was passed over was told about it: %v", theirs)
	}
}

/*
The endorser's other tabs are told, and the tab that acted is not.

§8's rules 3 and 4: the acting tab already has the command's response, and every
other tab of the same player learns from the socket. `social.liked`, addressed to
somebody else entirely, carries no origin at all — they did not act.
*/
func TestAnsweringIsEchoedToTheOtherTabsAndNotToTheOneThatActed(t *testing.T) {
	t.Parallel()
	s, st, me, _ := twoPlayers(t)

	offer := endorsementOf(t, s, me).Offer
	if offer == nil {
		t.Fatal("nobody to like")
	}

	sent := httptest.NewRequest(http.MethodPost, "/api/commands/social.endorse",
		strings.NewReader(`{"offer":`+itoa(offer.ID)+`}`))
	sent.Header.Set("Content-Type", "application/json")
	sent.Header.Set("Idempotency-Key", "like-1")
	sent.Header.Set(sessionHeader, "a-tab")
	sent.AddCookie(me)

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, sent)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	origins := map[string]*string{}
	rows, err := st.Pool().Query(t.Context(),
		`SELECT type, origin FROM realtime_outbox ORDER BY seq`)
	if err != nil {
		t.Fatalf("read the outbox: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			kind   string
			origin *string
		)
		if err := rows.Scan(&kind, &origin); err != nil {
			t.Fatalf("read the outbox: %v", err)
		}
		origins[kind] = origin
	}

	if got, ok := origins["social.answered"]; !ok || got == nil || *got != "a-tab" {
		t.Errorf("social.answered origin = %v, want the tab that sent it so its own echo is skipped", got)
	}
	if got, ok := origins["social.liked"]; !ok || got != nil {
		t.Errorf("social.liked origin = %v, want none — the player who was liked did not act", got)
	}
}

/*
Nothing on the endorsement surface names one of §3.5's hidden factors.

The same assertion internal/api makes about the composite and about the boards,
extended to the path that actually *moves* a hidden factor. Read as text rather
than as a struct: a field added carelessly, a note that explains too much, or a
grant leaking into a response would be caught here rather than in a JSON tab.
*/
func TestNoHiddenFactorIsNamedAnywhereOnTheEndorsementPath(t *testing.T) {
	t.Parallel()
	s, _, me, _ := twoPlayers(t)

	offer := endorsementOf(t, s, me).Offer
	if offer == nil {
		t.Fatal("nobody to like")
	}

	bodies := []string{
		read(t, s, me, worldPath).Body.String(),
		post(t, s, me, "social.endorse", "like-1", `{"offer":`+itoa(offer.ID)+`}`).Body.String(),
	}
	for _, body := range bodies {
		for _, f := range simulation.Factors {
			if f.Shown() {
				continue
			}
			if contains(body, f.Key()) {
				t.Errorf("%q appears on the endorsement path — the hidden factors are the hook (§3.5)", f.Key())
			}
		}
	}
}

/*
There is no command that asks for somebody to like.

The absence of a route is the mechanism, in exactly the way it is for factors
(§7): an offer that could be requested would be a button that is always
available, which is the one thing §3.5 says not to build.
*/
func TestThereIsNoCommandThatAsksForAnOffer(t *testing.T) {
	t.Parallel()
	s, _, me, _ := twoPlayers(t)

	for _, name := range []string{"social.offer", "social.draw", "social.next", "endorsement.new"} {
		rec := post(t, s, me, name, "key-"+name, `{}`)
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404 — an offer is made, not asked for", name, rec.Code)
		}
	}
}

// itoa is strconv.Itoa for an offer id, spelled here so the bodies above read as
// JSON rather than as string building.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
