package challenge

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/event"
)

/*
The catalogue: every authored challenge, loaded from the repo and validated
before anybody can be put in one. Repo files are the source of truth (§5.2):

	content/challenges/interview/a/
	  challenge.yaml     # mechanic type, placement rules, factor rewards
	  dialogue.yaml      # nodes, options, authored reactions per bucket

On boot the server loads, validates against the mechanic's config schema, and
upserts into the challenge catalog table so foreign keys work. The database
stores runs rather than definitions. Development hot-reloads on file change;
production validates at startup, so a malformed challenge fails in CI.

Identity comes from the path: §5.3's family and variant are the two directory
names rather than fields, so a file cannot disagree with where it lives and
renaming a challenge is a git mv.

The mechanic validates its own config and the framework validates the plan.
Neither can pass what the other would refuse: a mechanic cannot ship a scene
graph with a cycle in it, and the framework has never seen a mechanic's schema.

A challenge that can never begin is a failed build. §4 separates availability
from offer, so a definition names one, the other, or both.
*/

// Code is a challenge's derived identity, family/variant (§5.3). Never authored:
// it is what a run's foreign key holds, and the database derives it a second
// time from the same two columns, so the two cannot drift.
type Code string

// Of composes the code for one family and variant.
func Of(family, variant string) Code { return Code(family + "/" + variant) }

// Family and Variant take the code apart again, for a caller that has one.
func (c Code) Family() string  { family, _, _ := strings.Cut(string(c), "/"); return family }
func (c Code) Variant() string { _, variant, _ := strings.Cut(string(c), "/"); return variant }

// Definition is one authored challenge, carrying no player and no state. A row
// in challenge_run is one player's relationship with one of these.
type Definition struct {
	Family  string
	Variant string
	Code    Code
	// Mechanic is the type that runs it (§5.3's third identity column).
	Mechanic Type
	// Name is what it is called. Content rather than schema, and out of the
	// catalog table because a name in a row goes stale.
	Name string

	// Opens is the authored event whose acceptance opens this challenge, or
	// empty — §4's offer half. The loader checks the event exists and awaits
	// something, because an event that finishes on the spot has no doorway.
	Opens string
	// Where is the availability half: the spots this challenge stands at of its
	// own accord (§4).
	Where Availability
	// Gate is what has to be true before either half reaches a player at all
	// (gate.go). [Open] unless an author said otherwise.
	Gate Gate

	// Door is the invitation a player reads standing at this challenge's entry
	// point, and Enter is the label on the button that begins it. Both empty
	// for a challenge with no `where` — one that is only ever opened by an
	// event has no doorway to word.
	Door  string
	Enter string

	// Canvas is the painted location a run of this stands in, or empty for a
	// challenge that happens where the player already is. A location key rather
	// than a city place; the client holds the paintings and this names one of
	// them. Unvalidated here. A client with no such painting shows the world
	// the player is standing in.
	Canvas string

	// Plan is what the mechanic made of its own content, already checked.
	Plan Plan
	// Made is everything the mechanic made, of which [Definition.Plan] is the
	// part this package can read. Carried so the type can be asked about its own
	// challenge later ([Say] is the only caller), and an interface because what
	// is behind it is none of this package's business (§5.2).
	Made Configured
}

// Standing reports whether this challenge is available anywhere without an
// event to open it.
func (d Definition) Standing() bool { return len(d.Where.places) > 0 }

/*
Availability is every spot a challenge's own entry point stands at. Placement
conditions and world trigger zones are the same objects (§10.3): an author writes
zones out of internal/city's closed vocabulary, the (place, zone) pairs are
resolved once at boot, and an entry point that resolves to nowhere is a failed
build.

This package's own type rather than internal/event's, because parsing a kind of
content belongs to the module that owns it (§3.1) and because the two are
different questions: an event is placed by the world when it is released, and a
challenge stands somewhere until somebody walks up to it.
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

// catalogue is every authored challenge, in a stable order. An atomic pointer
// rather than a plain slice because §5.2 asks for hot reload in development: a
// running process replaces the whole catalogue at once, so a run that read it
// mid-swap sees one version or the other. Production writes it once, at init.
var catalogue atomic.Pointer[[]Definition]

func init() {
	// The mechanic types that ship inside this package, on the register before
	// anything is loaded. They are registered here rather than from their own
	// files' init functions because within a package init functions run in
	// file-name order and this one reads the content.
	Register(branching{})
	Register(salvaging{})

	loaded, err := Load(content.FS)
	if err != nil {
		// Content that cannot be loaded stops a deploy rather than producing a
		// world with a hole in it. In a test binary it stops the package
		// instead, which is what failing in CI means (§5.2).
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

// Known finds a challenge by code. The false is a real answer: a challenge
// withdrawn from content leaves runs behind it and rows in the catalog table,
// and the run path refuses rather than failing.
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

// Opened finds the challenge an authored event opens, if one does — the other
// direction of the opens link, resolved at load rather than searched at run
// time. Single-valued, because the loader refuses two challenges behind one
// event.
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

// definitionFile is challenge.yaml, exactly. Strict decoding (see [decode])
// makes a field that is not here a failed build rather than a line of content
// silently ignored, so this struct is the authoring contract. Family and variant
// come from the directory names, and the mechanic reads its own documents.
type definitionFile struct {
	Name     string `yaml:"name"`
	Mechanic string `yaml:"mechanic"`
	Opens    string `yaml:"opens"`
	Gate     string `yaml:"gate"`
	Canvas   string `yaml:"canvas"`
	// Door and Enter are what a challenge standing in the world says to somebody
	// who has walked up to it: the invitation, and the words on the button that
	// starts it. Authored per challenge, and required exactly when where is
	// named.
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
// the registered mechanic types. The fs.FS is the content root, the one with
// challenges/ in it, so a test can drive this over an in-memory tree and the
// reloader over the working copy on disk.
func Load(fsys fs.FS) ([]Definition, error) { return parse(fsys, mechanics) }

/*
parse is Load with the registry passed in, so a test can supply its own mechanic
types without registering them into the process.

Validation happens at three widths, and the last is why this is one function
rather than a loop over files:

  - the mechanic's, over its own config documents (§5.2);
  - the framework's, over the plan it produced ([Plan.check]);
  - the catalogue's, over everything together — two challenges behind one event
    is invisible in either file on its own.
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
			// is a directory per family.
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
			// through authoring one.
			return nil, fmt.Errorf("%s: the family %q has no variants", root, family.Name())
		}
	}

	// One event opens at most one challenge. Two would leave no rule for which
	// doorway an accepted event puts the player through.
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
	if err := content.DecodeBytes(raw, &file); err != nil {
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
		// Naming every registered type, because the failure is usually a typo
		// and the answer is on the list.
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
	// The mechanic validates its own config schema (§5.2). What it wants is its
	// business; that it refused is this loader's.
	made, err := mechanic.Configure(dir)
	if err != nil {
		return Definition{}, fmt.Errorf("%s: %w", out.Code, err)
	}
	if made == nil {
		// A mechanic that returned neither a challenge nor a complaint: its own
		// bug rather than an author's, and reported here because the
		// alternative is a nil dereference in the run path.
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
			// An event that does not await finishes the moment it is accepted,
			// so this challenge would never begin.
			return Definition{}, fmt.Errorf("%s is opened by %s, which finishes on the spot", out.Code, out.Opens)
		}
	}

	if out.Where, err = availability(file); err != nil {
		return Definition{}, fmt.Errorf("%s: %w", out.Code, err)
	}
	if out.Opens == "" && len(out.Where.places) == 0 {
		// §4 separates availability from offer, and a challenge with neither is
		// content nobody can reach.
		return Definition{}, fmt.Errorf("%s stands nowhere and is opened by nothing, so it can never begin", out.Code)
	}

	// A challenge that stands somewhere has to say what it says to the person
	// standing at it. Every word a player reads inside a challenge is authored
	// (content/challenges/SPEC.md), so a doorway with no words is one the client
	// would have to invent them for.
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

// slug refuses a directory name that would not survive being half of a code:
// `a/b` + `c` and `a` + `b/c` are the same code. Content keys are lower case.
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
