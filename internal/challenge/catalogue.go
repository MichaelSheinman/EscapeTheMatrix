package challenge

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
)

/*
The catalogue: every authored challenge, loaded from the repo and validated
before anybody can be put in one.

technical.md §5.2 decides all of this and leaves little to interpret:

	**Repo files are the source of truth. Not a CMS.**

	    content/challenges/resolve_conflict/a/
	      challenge.yaml     # mechanic type, placement rules, factor rewards
	      dialogue.yaml      # nodes, options, authored reactions per bucket

	On boot the server loads, validates against the mechanic's config schema,
	and upserts into a `challenge` catalog table so foreign keys work. The
	database stores *runs*, never definitions. **Dev mode hot-reloads on file
	change**; production validates at startup so a malformed challenge fails in
	CI, not at 2am.

Three things in here are the ticket rather than the plumbing.

**Identity comes from the path.** §5.3's family and variant are the two
directory names, not fields — so a file cannot disagree with where it lives, and
renaming a challenge is a `git mv` rather than an edit in two places that can
half-happen.

**The mechanic validates its own config, and the framework validates the plan.**
Neither one can pass what the other would refuse: a mechanic cannot ship a scene
graph with a cycle in it, and the framework cannot second-guess a mechanic's own
schema because it has never seen it.

**A challenge that can never begin is a failed build.** §4 separates
availability from offer — "an event *can* offer a challenge; the world *can*
expose a challenge entry point" — so a definition names one, the other, or both,
and naming neither is refused here rather than discovered by nobody.
*/

// Code is a challenge's derived identity: `family/variant` (§5.3).
//
// Derived and never authored. It is what a run's foreign key holds, and the
// database derives it a second time from the same two columns — so the two
// cannot drift, whichever of them somebody reads.
type Code string

// Of composes the code for one family and variant.
func Of(family, variant string) Code { return Code(family + "/" + variant) }

// Family and Variant take the code apart again, for a caller that has one.
func (c Code) Family() string  { family, _, _ := strings.Cut(string(c), "/"); return family }
func (c Code) Variant() string { _, variant, _ := strings.Cut(string(c), "/"); return variant }

/*
Definition is one authored challenge.

It carries no player and no state. A row in `challenge_run` is one player's
relationship with one of these; this is the thing itself, shared by everybody
and edited by an author rather than by the game.
*/
type Definition struct {
	Family  string
	Variant string
	Code    Code
	// Mechanic is the type that runs it (§5.3's third identity column).
	Mechanic Type
	// Name is what it is called. Content, not schema: it is deliberately not in
	// the catalog table, because a name in a row is a name that goes stale.
	Name string

	// Opens is the authored event whose acceptance opens this challenge, or
	// empty. §4's offer half — "an event *can* offer a challenge" — and the
	// loader checks the event exists and is one that `awaits` something,
	// because an event that finishes on the spot has no doorway in it.
	Opens string
	// Where is the availability half: the spots this challenge stands at of its
	// own accord (§4: "the world *can* expose a challenge entry point; some
	// challenges are simply always available").
	Where Availability
	// Gate is what has to be true before either half reaches a player at all
	// (gate.go). [Open] unless an author said otherwise, which is what keeps the
	// challenges authored before the gate existed behaving as they did.
	Gate Gate

	// Door is the invitation a player reads standing at this challenge's entry
	// point, and Enter is the label on the button that begins it. Both empty
	// for a challenge with no `where` — one that is only ever opened by an
	// event has no doorway to word.
	Door  string
	Enter string

	// Canvas is the painted location a run of this stands in, or empty for a
	// challenge that happens where the player already is.
	//
	// A location key rather than a city place, and the difference is the whole
	// of it: a burning flat is not somewhere in the city graph, it is somewhere
	// a run puts you for six minutes and never again. internal/city has never
	// heard of it and must not — the client holds the paintings (§10.3), and
	// this names one of them.
	//
	// Unvalidated here, deliberately. Whether a build has painted that location
	// is a fact about the client's assets, which this package cannot see and
	// should not pretend to; a client with no such painting shows the world the
	// player is actually standing in, which is what every challenge did before
	// this field existed.
	Canvas string

	// Plan is what the mechanic made of its own content, already checked.
	Plan Plan
	// Made is the whole of what the mechanic made, of which [Definition.Plan]
	// is the part this package can read. It is carried so that the type can be
	// asked about its own challenge later — [Say] is the only thing that asks —
	// and it is an interface rather than a value because what is behind it is
	// none of this package's business (§5.2).
	Made Configured
}

// Standing reports whether this challenge is available anywhere without an
// event to open it.
func (d Definition) Standing() bool { return len(d.Where.places) > 0 }

/*
Availability is every spot a challenge's own entry point stands at.

The same arrangement §4 requires of an event's placement and for the same
reason: "placement conditions and world trigger zones are the same objects"
(§10.3). An author writes zones out of internal/city's closed vocabulary, the
concrete (place, zone) pairs are resolved once at boot, and an entry point that
resolves to nowhere is a failed build rather than a door nobody ever finds.

It is this package's own type rather than internal/event's, because §3.1 puts
the parsing of a kind of content in the module that owns it, and because the two
are not the same question: an event is placed *by the world* at the moment it is
released, and a challenge simply stands somewhere until somebody walks up to it.
*/
type Availability struct {
	// spots is place key → the zones allowed there, in the city's own priority
	// order (most specific first).
	spots map[string][]string
	// places is the same keys in the city's authored order.
	places []string
}

// At is the zone this challenge stands in at a given place, and whether it
// stands there at all.
func (a Availability) At(place string) (string, bool) {
	zones := a.spots[place]
	if len(zones) == 0 {
		return "", false
	}
	return zones[0], true
}

// Places is every place this challenge stands in, in the city's order.
func (a Availability) Places() []string {
	out := make([]string, len(a.places))
	copy(out, a.places)
	return out
}

/* ── the catalogue, loaded once and swappable ──────────────────────────── */

/*
catalogue is every authored challenge, in a stable order.

An atomic pointer rather than a plain slice because §5.2 asks for hot reload in
development: a running process replaces the whole catalogue at once, and a run
that read it mid-swap has to see one version of the world or the other rather
than half of each. Production writes it exactly once, at init.
*/
var catalogue atomic.Pointer[[]Definition]

func init() {
	// The mechanic types that ship inside this package, on the register before
	// anything is loaded. They are registered here rather than from their own
	// files' init functions because within a package init functions run in
	// file-name order and this one reads the content — a registration in
	// `dialogue.go` would arrive after the loader had already refused the
	// challenges that name it. A type in its own package would be later still.
	//
	// `dialogue_branch` got here the way §5.2 asks for: the first challenge of a
	// shape was written as code (#24) and the type was extracted on the second
	// (#25), so the name is now a name for a kind of challenge rather than for
	// one.
	//
	// `salvage` did not, and the exception is worth stating rather than
	// smuggling. Its shape is a bag, a house with more in it than the bag holds,
	// and a decision about each thing that came out — and there is no version of
	// that which is one challenge's worth of code, because the scene graph is
	// *generated* from the item list. Writing the house fire as bespoke Go would
	// have meant writing the generator anyway and then extracting a type that
	// was already the whole file. §5.2's discipline is there to stop a shape
	// being guessed at from one example; here the shape is the only thing there
	// is, and the words are all content.
	Register(branching{})
	Register(salvaging{})

	loaded, err := Load(content.FS)
	if err != nil {
		// The same decision internal/city, internal/playbook and internal/event
		// take, for the same reason: content that cannot be loaded should stop
		// a deploy, not produce a world with a hole in it. In a test binary it
		// stops the package instead, which is §5.2's "fails in CI, not at 2am".
		panic("challenge: " + err.Error())
	}
	catalogue.Store(&loaded)
}

// All is every authored challenge, in the catalogue's order.
func All() []Definition {
	held := catalogue.Load()
	if held == nil {
		return nil
	}
	out := make([]Definition, len(*held))
	copy(out, *held)
	return out
}

// Known finds a challenge by code.
//
// The false is a real answer: a challenge withdrawn from content leaves runs
// behind it and rows in the catalog table, and the run path refuses politely
// rather than failing.
func Known(code Code) (Definition, bool) {
	held := catalogue.Load()
	if held == nil {
		return Definition{}, false
	}
	for _, d := range *held {
		if d.Code == code {
			return d, true
		}
	}
	return Definition{}, false
}

// Opened finds the challenge an authored event opens, if one does.
//
// The other direction of the `opens` link, resolved at load rather than
// searched at run time — and single-valued, because the loader refuses two
// challenges behind one event.
func Opened(eventKey string) (Definition, bool) {
	held := catalogue.Load()
	if held == nil || eventKey == "" {
		return Definition{}, false
	}
	for _, d := range *held {
		if d.Opens == eventKey {
			return d, true
		}
	}
	return Definition{}, false
}

// swap replaces the whole catalogue. Only the reloader calls it.
func swap(loaded []Definition) { catalogue.Store(&loaded) }

/* ── reading the files ─────────────────────────────────────────────────── */

// root is where challenges live inside the content tree.
const root = "challenges"

// spec is the one file every challenge has, whatever its mechanic.
const spec = "challenge.yaml"

// definitionFile is challenge.yaml, exactly.
//
// Strict decoding (see [decode]) means a field that is not here is a failed
// build rather than a line of content silently ignored, so this struct is the
// authoring contract. Note what is *not* in it: family and variant, which come
// from the directory names, and anything the mechanic owns, which the mechanic
// reads for itself out of its own documents.
type definitionFile struct {
	Name     string `yaml:"name"`
	Mechanic string `yaml:"mechanic"`
	Opens    string `yaml:"opens"`
	Gate     string `yaml:"gate"`
	Canvas   string `yaml:"canvas"`
	// Door and Enter are what a challenge standing in the world says to
	// somebody who has walked up to it: the invitation, and the words on the
	// button that starts it. Authored per challenge because they cannot be
	// otherwise — the component that used to supply them said "you can go up
	// your own stairs" and "Stop and listen" to everybody, which is the
	// argument on the stoop describing itself, and read as nonsense at a bank
	// counter (2026-08-10). Required exactly when `where` is named.
	Door  string `yaml:"door"`
	Enter string `yaml:"enter"`

	Where struct {
		Places    []string `yaml:"places"`
		Layers    []string `yaml:"layers"`
		Districts []string `yaml:"districts"`
		Zones     []string `yaml:"zones"`
	} `yaml:"where"`
}

// Load reads a whole content tree and validates every challenge in it against
// the registered mechanic types.
//
// The fs.FS is the content root — the one that has `challenges/` in it — so a
// test can drive this over an in-memory tree and the reloader can drive it over
// the working copy on disk.
func Load(fsys fs.FS) ([]Definition, error) { return parse(fsys, mechanics) }

/*
parse is Load with the registry passed in, so a test can supply its own
mechanic types without registering them into the process.

Validation happens in three widening circles, and the outermost one is the
reason this is a single function rather than a loop over files:

  - **the mechanic's**, over its own config documents (§5.2's config schema);
  - **the framework's**, over the plan it produced ([Plan.check]);
  - **the catalogue's**, over everything together — two challenges behind one
    event is invisible in either file on its own.
*/
func parse(fsys fs.FS, types map[Type]Mechanic) ([]Definition, error) {
	families, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}

	var out []Definition
	for _, family := range families {
		if !family.IsDir() {
			// A README beside the families, or an editor's stray file. Content
			// is a directory per family and nothing else.
			continue
		}
		if err := slug(family.Name()); err != nil {
			return nil, fmt.Errorf("%s: the family %q %w", root, family.Name(), err)
		}

		variants, err := fs.ReadDir(fsys, path.Join(root, family.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", family.Name(), err)
		}
		found := false
		for _, variant := range variants {
			if !variant.IsDir() {
				continue
			}
			if err := slug(variant.Name()); err != nil {
				return nil, fmt.Errorf("%s: the variant %q %w", family.Name(), variant.Name(), err)
			}
			found = true

			d, err := read(fsys, types, family.Name(), variant.Name())
			if err != nil {
				return nil, err
			}
			out = append(out, d)
		}
		if !found {
			// A family directory with no variants in it is somebody halfway
			// through authoring one, and a build is not the place to be halfway.
			return nil, fmt.Errorf("%s: the family %q has no variants", root, family.Name())
		}
	}

	// One event opens at most one challenge. Two would be an accepted event
	// with two doorways behind it and no rule for which one the player walks
	// through — decided, if it were allowed, by directory order.
	behind := map[string]Code{}
	for _, d := range out {
		if d.Opens == "" {
			continue
		}
		if first, taken := behind[d.Opens]; taken {
			return nil, fmt.Errorf("%s and %s are both opened by %s", first, d.Code, d.Opens)
		}
		behind[d.Opens] = d.Code
	}

	// Stable order: family, then variant. Every list this package makes comes
	// out of this one.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Family != out[j].Family {
			return out[i].Family < out[j].Family
		}
		return out[i].Variant < out[j].Variant
	})
	return out, nil
}

// read loads one variant directory into a definition.
func read(fsys fs.FS, types map[Type]Mechanic, family, variant string) (Definition, error) {
	where := path.Join(root, family, variant)

	raw, err := fs.ReadFile(fsys, path.Join(where, spec))
	if err != nil {
		return Definition{}, fmt.Errorf("%s/%s: %w", family, variant, err)
	}
	var file definitionFile
	if err := decode(raw, &file); err != nil {
		return Definition{}, fmt.Errorf("%s/%s/%s: %w", family, variant, spec, err)
	}

	out := Definition{
		Family:   family,
		Variant:  variant,
		Code:     Of(family, variant),
		Mechanic: Type(strings.TrimSpace(file.Mechanic)),
		Name:     strings.TrimSpace(file.Name),
		Opens:    strings.TrimSpace(file.Opens),
		Canvas:   strings.TrimSpace(file.Canvas),
		Door:     trimLine(file.Door),
		Enter:    trimLine(file.Enter),
	}
	if out.Name == "" {
		return Definition{}, fmt.Errorf("%s has no name", out.Code)
	}
	if out.Gate, err = gate(file.Gate); err != nil {
		return Definition{}, fmt.Errorf("%s %w", out.Code, err)
	}

	mechanic, known := types[out.Mechanic]
	if !known {
		// Naming every registered type, because the failure is almost always a
		// typo and the answer is almost always on the list. §5.2: a new
		// mechanic is code, so this is never fixed by editing content alone.
		names := make([]string, 0, len(types))
		for name := range types {
			names = append(names, string(name))
		}
		sort.Strings(names)
		return Definition{}, fmt.Errorf("%s runs on %q, which is not a mechanic type (there is %s)",
			out.Code, file.Mechanic, list(names))
	}

	dir, err := fs.Sub(fsys, where)
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", out.Code, err)
	}
	// §5.2's "validate against the mechanic's config schema". Whatever the type
	// wants is its own business; that it refused is this loader's.
	made, err := mechanic.Configure(dir)
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", out.Code, err)
	}
	if made == nil {
		// A mechanic that returned neither a challenge nor a complaint. Its own
		// bug rather than an author's, and it is worth saying so here because
		// the alternative is a nil dereference three calls away in the run path.
		return Definition{}, fmt.Errorf("%s: %s made nothing of it and did not say why", out.Code, out.Mechanic)
	}
	plan := made.Plan()
	if err := plan.check(); err != nil {
		return Definition{}, fmt.Errorf("%s %w", out.Code, err)
	}
	out.Plan, out.Made = plan, made

	if out.Opens != "" {
		opener, ok := event.Known(out.Opens)
		switch {
		case !ok:
			return Definition{}, fmt.Errorf("%s is opened by %q, which is not an event", out.Code, out.Opens)
		case !opener.Awaits:
			// An event that does not await finishes the moment it is accepted
			// (internal/event's `after`), so there is no moment at which this
			// challenge would begin.
			return Definition{}, fmt.Errorf("%s is opened by %s, which finishes on the spot", out.Code, out.Opens)
		}
	}

	if out.Where, err = availability(file); err != nil {
		return Definition{}, fmt.Errorf("%s: %w", out.Code, err)
	}
	if out.Opens == "" && len(out.Where.places) == 0 {
		// §4 separates availability from offer, and a challenge with neither is
		// content nobody can ever reach — which is the failure this whole
		// arrangement of resolving-at-boot exists to make impossible.
		return Definition{}, fmt.Errorf("%s stands nowhere and is opened by nothing, so it can never begin", out.Code)
	}

	// A challenge that stands somewhere has to say what it says to the person
	// standing at it. Every word a player reads inside a challenge is authored
	// (content/challenges/SPEC.md, 2026-08-10) — the client renders and writes
	// nothing — so a doorway with no words would be a doorway the component
	// would have to invent them for, which is the arrangement this replaced.
	if len(out.Where.places) > 0 {
		if out.Door == "" {
			return Definition{}, fmt.Errorf("%s stands in the world and has no door, so nothing tells the player what they are standing at", out.Code)
		}
		if out.Enter == "" {
			return Definition{}, fmt.Errorf("%s stands in the world and has no enter, so the button that begins it has nothing to say", out.Code)
		}
	}
	if len(out.Where.places) == 0 && (out.Door != "" || out.Enter != "") {
		return Definition{}, fmt.Errorf("%s words a doorway it does not have", out.Code)
	}
	return out, nil
}

// trimLine flattens an authored line into one paragraph.
func trimLine(s string) string { return strings.Join(strings.Fields(s), " ") }

/*
availability resolves what an author wrote into concrete spots.

The three ways of naming places are additive and each is optional — `places` for
the specific, `layers` for "any interior", `districts` for "anywhere in the east
side". The whole block is optional too, for a challenge that is only ever opened
by an event. What is not optional is the result: if an author named somewhere,
it has to resolve to somewhere, or the build fails.
*/
func availability(file definitionFile) (Availability, error) {
	named := len(file.Where.Places) + len(file.Where.Layers) + len(file.Where.Districts)
	if named == 0 && len(file.Where.Zones) == 0 {
		return Availability{}, nil
	}
	if len(file.Where.Zones) == 0 {
		return Availability{}, fmt.Errorf("names places to stand in and no zones to stand in them")
	}
	if named == 0 {
		return Availability{}, fmt.Errorf("names zones and nowhere to find them")
	}

	zones := map[string]bool{}
	for _, z := range file.Where.Zones {
		if !city.Zone(z) {
			return Availability{}, fmt.Errorf("names the zone %q, which is not a kind of spot", z)
		}
		zones[z] = true
	}

	wanted := map[string]bool{}
	for _, key := range file.Where.Places {
		if !city.Known(key) {
			return Availability{}, fmt.Errorf("names the place %q, which is nowhere", key)
		}
		wanted[key] = true
	}
	for _, layer := range file.Where.Layers {
		found := false
		for _, p := range city.Places() {
			if string(p.Layer) == layer {
				wanted[p.Key], found = true, true
			}
		}
		if !found {
			return Availability{}, fmt.Errorf("names the layer %q, which is not one", layer)
		}
	}
	for _, district := range file.Where.Districts {
		outer, ok := city.Lookup(district)
		if !ok || outer.Layer != city.District {
			return Availability{}, fmt.Errorf("names the district %q, which is not one", district)
		}
		wanted[district] = true
		for _, p := range city.Places() {
			if p.In == district {
				wanted[p.Key] = true
			}
		}
	}

	out := Availability{spots: map[string][]string{}}
	// The city's own order, so priority is the city's opinion about which spot
	// is the more specific rather than the order somebody typed the zones in.
	for _, p := range city.Places() {
		if !wanted[p.Key] {
			continue
		}
		var allowed []string
		for _, z := range city.ZonesIn(p.Key) {
			if zones[z] {
				allowed = append(allowed, z)
			}
		}
		if len(allowed) == 0 {
			continue
		}
		out.spots[p.Key] = allowed
		out.places = append(out.places, p.Key)
	}
	if len(out.places) == 0 {
		return Availability{}, fmt.Errorf("stands nowhere: no place it names has any of its zones")
	}
	return out, nil
}

// slug refuses a directory name that would not survive being half of a code.
//
// The separator is the point: `a/b` + `c` and `a` + `b/c` are the same code,
// and the database says so too (see the CHECK constraints in the migration).
// The rest is house style — content keys in this repo are lower case, and a
// catalogue that was case-sensitive on some filesystems and not on others would
// be a catalogue that changed shape when it moved.
func slug(name string) error {
	if name == "" {
		return fmt.Errorf("is empty")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return fmt.Errorf("is not lower case letters, digits, - or _")
		}
	}
	return nil
}

// list reads a set of names as a sentence.
func list(names []string) string {
	switch len(names) {
	case 0:
		return "no mechanic type at all"
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

/*
decode reads one YAML document strictly.

KnownFields(true) is the whole point: a misspelt field is a failed build rather
than a silently ignored line of content. It is the same helper internal/event
and internal/playbook use, and it is duplicated rather than shared for §3.1's
reason — the two would otherwise grow a shared "content utilities" package that
knows what all of them mean.
*/
func decode(raw []byte, into any) error {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	return dec.Decode(into)
}
