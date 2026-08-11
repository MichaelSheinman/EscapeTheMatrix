package career

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The roles, as authored content rather than as Go (§5.2, and the decision of
2026-08-09).

They lived in a `var` block in career.go until now, which made every posting a
recompile and made "add a job" a code review. technical.md §5.2 already decided
where content goes — repo files, versioned and diffable, loaded and validated at
boot — and a posting is content in exactly the way a situation or a conflict is:
prose somebody writes, in a voice, that the program never reasons about.

# One directory per environment

`content/jobs/<environment>/<key>.yaml`, and the directory is the whole of
§3.15's scoping. A role belongs to the population whose *working life* it is
rather than to the district the building stands in — an east-sider takes the
night shift on the Strip, and nobody on that landing is getting hired into the
tower across the river. So the folder says whose ladder this is, and `hired`
says where in the city you walk in.

That is also why the job surface can be local at all. Before this, `Roles` was
one global list and the board showed all of it wherever you stood, which is why
moving house changed the people, the rules, the ceilings and the decay — and
left the postings identical.

# What is here and what is still the pure core's

No wage. The pay, the hours and the requirement are read off [simulation.Rungs]
by tier, so a file cannot state a salary and drift from the arithmetic the
rollover does. What a file may say about money is [Role.Premium]: what *this*
posting hands over on top of its rung, which is a fact about a job rather than
about a tier.
*/

// Where the authored roles live, one directory per environment.
const jobsPath = "jobs"

/*
Ladder is the fewest roles an environment must offer.

A population with one job is a population with no decision in it: §3.14's ladder
only teaches the trade if there is more than one way onto it, and §3.15's
transfer only bites if arriving somewhere means meeting different work. Three is
the floor under "there is a choice here".
*/
const Ladder = 3

/*
catalogue is the one parsed copy — the ladders keyed by environment, each held
in the order it is climbed — and order is every environment that has work, in
key order (§7).

A var initializer rather than an `init()`, which is load-bearing rather than
style: package-level variables are initialized before `init()` runs, and by
dependency. Anything else in this package that reads a role while *it* is being
initialized therefore sees a full catalogue, instead of an empty map and a panic
that only reproduces in whichever file the linker happened to order first.
*/
var catalogue, order = loadJobs(content.FS)

// loadJobs parses the shipped ladders and refuses to boot without them.
//
// technical.md §5.2: "production validates at startup so a malformed challenge
// fails in CI, not at 2am." A panic here is that, and it is why [parseJobs]
// takes an fs.FS — every failure path below is worth a test, and a test cannot
// catch an initializer that has already run.
func loadJobs(files fs.FS) (map[string][]Role, []string) {
	all, keys, err := parseJobs(files)
	if err != nil {
		panic("career: " + err.Error())
	}
	return all, keys
}

/*
The YAML shape.

Separate from [Role] on purpose, like every other loader in this repository:
what an author writes and what the program reads are two decisions, and a role
carrying its file's field names would make renaming one a migration.

There is deliberately no `pay`, no `hours` and no `environment`. The first two
are the pure core's by tier; the third is the directory the file is in, and a
role that could name a different one would be a role filed under two answers.
*/
type jobFile struct {
	Tier int    `yaml:"tier"`
	Name string `yaml:"name"`
	// Posting is the advert in the sections a real one has. The contract —
	// budgets, register, what may never be written — is content/jobs/SPEC.md,
	// and the budgets are enforced in [posting].
	Posting struct {
		Intro          string   `yaml:"intro"`
		Duties         []string `yaml:"duties"`
		Qualifications []string `yaml:"qualifications"`
	} `yaml:"posting"`
	// Shift is when the working day starts, "HH:MM". The end is the rung's
	// hours later, and the start has to agree with the rung's Nights — the
	// file authors when the day begins, the pure core owns how long it is.
	Shift string `yaml:"shift"`
	// Premium is cents a day over the rung. Absent is zero, which is most of
	// them.
	Premium int64 `yaml:"premium"`
	// Toll is what a day of this job does to the hidden factors, in the same
	// `factor: credit` vocabulary a situation's `moves` block uses. Never sent
	// to a client — see [Role.Toll].
	Toll map[string]float64 `yaml:"toll"`
	// Hired is where somebody is taken on: a place in internal/city, and one of
	// the kinds of spot that place actually has.
	Hired struct {
		At   string `yaml:"at"`
		Zone string `yaml:"zone"`
	} `yaml:"hired"`
	// Interview is the authored challenge that hires into this role, and
	// Offered is which of its outcomes are an offer. Both absent for a role
	// that hires off the street.
	Interview string   `yaml:"interview"`
	Offered   []string `yaml:"offered"`
}

/*
parseJobs reads every environment's ladder and checks what the rest of the
package then gets to assume.

Strict twice over, like internal/environment's loader: an unknown YAML field is
a failed build rather than an authored line nobody notices has no effect, and
every cross-reference — a place, a zone, a tier — is resolved here rather than
at the moment a player happens to walk into it.
*/
func parseJobs(files fs.FS) (map[string][]Role, []string, error) {
	dirs, err := fs.ReadDir(files, jobsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", jobsPath, err)
	}

	out := map[string][]Role{}
	var keys []string
	// Keys are unique across the whole world rather than within a ladder: the
	// `career` table stores one, and two roles by one name would be one row
	// meaning different jobs depending on where its owner lives.
	seen := map[string]string{}
	// One place hires one role, everywhere. Standing somewhere that took on two
	// would make "take what is being taken on here" an ambiguous command.
	hires := map[string]string{}

	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		env := dir.Name()
		entries, err := fs.ReadDir(files, path.Join(jobsPath, env))
		if err != nil {
			return nil, nil, fmt.Errorf("read %s/%s: %w", jobsPath, env, err)
		}

		var ladder []Role
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			name := path.Join(jobsPath, env, entry.Name())

			var file jobFile
			if err := decodeJob(files, name, &file); err != nil {
				return nil, nil, err
			}
			key := strings.TrimSuffix(entry.Name(), ".yaml")
			role, err := job(env, key, file)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", name, err)
			}

			if other, taken := seen[key]; taken {
				return nil, nil, fmt.Errorf("%s: %q is already a job in %q", name, key, other)
			}
			seen[key] = env

			if at := file.Hired.At; at != "" {
				if other, taken := hires[at]; taken {
					return nil, nil, fmt.Errorf("%s: %q already hires %q", name, at, other)
				}
				hires[at] = key
			}
			ladder = append(ladder, role)
		}

		if len(ladder) < Ladder {
			return nil, nil, fmt.Errorf("%s/%s offers %d job(s); a ladder is at least %d",
				jobsPath, env, len(ladder), Ladder)
		}
		// Lowest rung first, ties in key order, so the board reads as a ladder
		// and two processes agree on it (§7).
		sort.SliceStable(ladder, func(i, j int) bool {
			if ladder[i].Tier != ladder[j].Tier {
				return ladder[i].Tier < ladder[j].Tier
			}
			return ladder[i].Key < ladder[j].Key
		})
		out[env], keys = ladder, append(keys, env)
	}

	if len(out) == 0 {
		return nil, nil, fmt.Errorf("%s holds no ladders", jobsPath)
	}
	sort.Strings(keys)
	return out, keys, nil
}

// job reads and checks one file.
func job(env, key string, file jobFile) (Role, error) {
	if key == "" {
		return Role{}, fmt.Errorf("a job has no key")
	}
	if strings.TrimSpace(file.Name) == "" {
		return Role{}, fmt.Errorf("%q has no name", key)
	}
	tier := simulation.Tier(file.Tier)
	rung, known := simulation.Rungs().At(tier)
	if !known {
		return Role{}, fmt.Errorf("%q is tier %d, which is not a rung of the ladder", key, file.Tier)
	}
	advert, err := posting(key, file)
	if err != nil {
		return Role{}, err
	}
	start, err := shiftStart(key, file.Shift, rung.Post.Nights)
	if err != nil {
		return Role{}, err
	}
	if file.Premium < 0 {
		return Role{}, fmt.Errorf("%q pays a premium of %d", key, file.Premium)
	}

	out := Role{
		Environment: env,
		Tier:        tier,
		Key:         key,
		Name:        strings.TrimSpace(file.Name),
		Posting:     advert,
		ShiftStart:  start,
		Premium:     simulation.Money(file.Premium),
		Interview:   file.Interview,
		Offered:     file.Offered,
	}

	for _, name := range sortedTolls(file.Toll) {
		which, err := granted(name)
		if err != nil {
			return Role{}, fmt.Errorf("%q %w", key, err)
		}
		out.Toll = append(out.Toll, simulation.Grant{Factor: which, Credit: file.Toll[name]})
	}

	// Where it is taken on. A role with nowhere is one of §3.14's upper rungs,
	// standing on the ladder and out of reach until #28 — so an absent `hired`
	// is allowed, and half of one is not.
	switch where, zone := file.Hired.At, file.Hired.Zone; {
	case where == "" && zone == "":
		if file.Interview != "" {
			return Role{}, fmt.Errorf("%q interviews for a job nobody hires for", key)
		}
	case where == "" || zone == "":
		return Role{}, fmt.Errorf("%q is hired at %q in the %q, which is half an address", key, where, zone)
	default:
		place, ok := city.Lookup(where)
		if !ok {
			return Role{}, fmt.Errorf("%q is hired at %q, which is not a place", key, where)
		}
		if place.Layer == city.Home {
			return Role{}, fmt.Errorf("%q is hired at %q, which is somewhere somebody lives", key, where)
		}
		if !city.Spot(where, zone) {
			return Role{}, fmt.Errorf("%q is hired at the %q in %q, which has no such spot", key, zone, where)
		}
		out.Hired = at([]string{where}, []string{zone})
	}

	if file.Interview == "" && len(file.Offered) > 0 {
		return Role{}, fmt.Errorf("%q names outcomes that hire for it and no interview to reach them", key)
	}
	if file.Interview != "" && len(file.Offered) == 0 {
		return Role{}, fmt.Errorf("%q interviews and no outcome of it is an offer", key)
	}
	return out, nil
}

// granted resolves a `toll` line's factor name, refusing one that is read from
// the world rather than granted (§3.5).
func granted(name string) (simulation.Factor, error) {
	for _, f := range simulation.Factors {
		if strings.EqualFold(f.String(), name) {
			if !f.Granted() {
				return 0, fmt.Errorf("tolls %s, which is read from the world rather than granted", name)
			}
			return f, nil
		}
	}
	return 0, fmt.Errorf("tolls %q, which is not a factor", name)
}

// sortedTolls is a `toll` block's factor names in the simulation's own order,
// so two processes build the same slice.
func sortedTolls(toll map[string]float64) []string {
	out := make([]string, 0, len(toll))
	for name := range toll {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// trimJob flattens an authored block into one paragraph.
func trimJob(s string) string { return strings.Join(strings.Fields(s), " ") }

/*
The posting budgets (content/jobs/SPEC.md). A posting over budget is where the
old register creeps back in — a fourth sentence of intro is an atmosphere
sentence — so the budgets are boot errors rather than style advice.
*/
const (
	introBudget  = 220
	bulletBudget = 70
)

// posting checks one advert against the SPEC's structural budgets.
func posting(key string, file jobFile) (Posting, error) {
	out := Posting{Intro: trimJob(file.Posting.Intro)}
	if out.Intro == "" {
		return Posting{}, fmt.Errorf("%q has no posting intro, so there is nothing for a player to read", key)
	}
	if len(out.Intro) > introBudget {
		return Posting{}, fmt.Errorf("%q has a posting intro of %d characters; the budget is %d",
			key, len(out.Intro), introBudget)
	}
	var err error
	if out.Duties, err = bullets(key, "duties", file.Posting.Duties, 3, 5); err != nil {
		return Posting{}, err
	}
	if out.Qualifications, err = bullets(key, "qualifications", file.Posting.Qualifications, 2, 4); err != nil {
		return Posting{}, err
	}
	return out, nil
}

// bullets checks one section: how many lines a real posting's version of it
// carries, and no line long enough to become a paragraph.
func bullets(key, section string, lines []string, least, most int) ([]string, error) {
	if len(lines) < least || len(lines) > most {
		return nil, fmt.Errorf("%q lists %d %s; a posting has %d to %d",
			key, len(lines), section, least, most)
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = trimJob(line)
		if line == "" {
			return nil, fmt.Errorf("%q has an empty line in its %s", key, section)
		}
		if len(line) > bulletBudget {
			return nil, fmt.Errorf("%q has a %s line of %d characters; the budget is %d",
				key, section, len(line), bulletBudget)
		}
		out = append(out, line)
	}
	return out, nil
}

/*
shiftStart reads "HH:MM" into minutes from midnight and holds it to the rung.

A nights rung starts in the evening and a day rung starts in the morning —
this is the check that would have caught "Afternoons at the laundromat" on a
tier whose Post is ten hours of nights. The end needs no field and no check:
it is the start plus the rung's hours, wherever that lands.
*/
func shiftStart(key, shift string, nights bool) (int, error) {
	var h, m int
	if n, err := fmt.Sscanf(shift, "%2d:%2d", &h, &m); err != nil || n != 2 ||
		h < 0 || h > 23 || m < 0 || m > 59 || len(shift) != 5 {
		return 0, fmt.Errorf("%q has a shift of %q; a shift is \"HH:MM\"", key, shift)
	}
	start := h*60 + m
	const morning, afternoon, evening = 5 * 60, 14 * 60, 18 * 60
	if nights && start >= morning && start < evening {
		return 0, fmt.Errorf("%q starts at %s on a nights rung; a nights shift starts at or after 18:00", key, shift)
	}
	if !nights && (start < morning || start > afternoon) {
		return 0, fmt.Errorf("%q starts at %s on a day rung; a day shift starts between 05:00 and 14:00", key, shift)
	}
	return start, nil
}

// decodeJob reads one YAML file strictly.
func decodeJob(files fs.FS, name string, into any) error {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return nil
}

/* ── reading the ladders ────────────────────────────────────────────────── */

/*
RolesIn is the ladder for one population, lowest rung first.

The empty slice for an environment nobody authored jobs for, which is a real
state of the world rather than an error: a player among nobody
([environment.Nowhere]) has no local work, and a board with nothing on it is the
honest answer.
*/
func RolesIn(env string) []Role {
	ladder := catalogue[env]
	out := make([]Role, len(ladder))
	copy(out, ladder)
	return out
}

/*
All is every authored role in the world, ladder by ladder.

For the callers that mean "the whole catalogue" rather than "the work here":
the boot checks that run over shipped content, and the tests that hold the
ladder to §3.14. The *board* is [RolesIn] and is local — see [every].
*/
func All() []Role {
	var out []Role
	every(func(r Role) bool { out = append(out, r); return true })
	return out
}

// Ladders is every environment that has work, in key order.
func Ladders() []string {
	out := make([]string, len(order))
	copy(out, order)
	return out
}

/*
every is every authored role in the world, ladder by ladder in key order.

The three lookups that use it — by key, by place, by interview code — are
deliberately *not* scoped to an environment. A player who has moved still holds
the job they were in until they leave it, and a challenge that was begun as an
interview is still that interview after the move; scoping those would make a
role vanish out from under a row that names it.

What is scoped is the *board* ([RolesIn]), which is the one surface that answers
"what work is there", and that question is local (§3.15).
*/
func every(yield func(Role) bool) {
	for _, env := range order {
		for _, r := range catalogue[env] {
			if !yield(r) {
				return
			}
		}
	}
}

// Lookup finds a role by key, anywhere in the world.
//
// Not scoped to an environment, because the `career` table stores a key and a
// player who has moved still holds the job they were in until they leave it.
func Lookup(key string) (Role, bool) { return ByKey(key) }
