package character

// The city, as a closed vocabulary.
//
// HighLevelOverview.md §3.12: one city, built deep and freely walkable, with
// districts that have real character — downtown for offices and transit, the
// east side where the player's apartment is, the riverfront where money lives,
// the strip for warehouses and night work. The player walks between them, and
// walking is the encounter surface rather than a wait.
//
// **§17 leaves the city and its district list open.** These four are the ones
// §3.12 names as examples, written down in one place so that settling that
// question is an edit here rather than a search. They are keys and labels only:
// what a district *contains* is the painted world (#11, #12), and what happens
// in one is the event system's placement rules (§4).

// Place is somewhere in the city — a district today, an interior later.
type Place struct {
	// Key is what the database and the wire carry. Stable; the label is not.
	Key string
	// Name is what a player reads.
	Name string
	// Note is the district's character in a few words. It is copy, and it will
	// move to content/ (§5.2) with the rest of the authored text.
	Note string
}

// Districts is the whole city at V1.
var Districts = []Place{
	{Key: "east-side", Name: "East Side", Note: "walk-ups, corner shops, and where you live"},
	{Key: "downtown", Name: "Downtown", Note: "offices, transit, and everybody in a hurry"},
	{Key: "riverfront", Name: "Riverfront", Note: "where the money lives"},
	{Key: "the-strip", Name: "The Strip", Note: "warehouses, loading bays, night work"},
}

// district finds a district by key.
func district(key string) (Place, bool) {
	for _, p := range Districts {
		if p.Key == key {
			return p, true
		}
	}
	return Place{}, false
}

// startingDistrict is where a new character lives and stands.
//
// §3.12 names the east side as the player's own neighbourhood. Onboarding
// (#33) will make this a choice with visible consequences; until then it is the
// same for everyone and stated in one place.
const startingDistrict = "east-side"
