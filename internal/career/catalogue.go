package career

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/MichaelSheinman/EscapeTheMatrix/content"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

/*
The roles, as authored content rather than as Go (§5.2). A posting is content in
the way a situation or a conflict is: prose somebody writes that the program
never reasons about.

One directory per environment, content/jobs/<environment>/<key>.yaml, which is
the whole of §3.15's scoping. A role belongs to the population whose working life
it is rather than to the district the building stands in — a Wilmingtonian takes
the night shift on the Strip. The folder says whose working life this is, and
hired says where in the city you walk in. That is what makes the job surface
local: one global list would show the same board wherever the player stood.

A file states its own wage, its own hours, its own capability floors
and its own [Role.Experience] — the routes of previous work that qualify somebody
for it. Until this decision a `tier:` line indexed a table in the pure core and
those four facts came back from it, which meant two jobs sharing a number were
the same job in every way a player could measure. §3.15 says progression is
lateral; a global rung ordering every job in the world was the one mechanic
saying otherwise.

The one thing a file still may not state is [Role.Toll], in the sense that it may
state it and nothing ever shows it (§3.5).
*/

// Where the authored roles live, one directory per environment.
const jobsPath = "jobs"

/*
Least is the fewest roles an environment must offer.

A population with one job is a population with no decision in it: work only
teaches a trade if there is more than one way into it, and §3.15's transfer only
bites if arriving somewhere means meeting different work. Three is the floor
under "there is a choice here".
*/
const Least = 3

/*
catalogue is the one parsed copy — the jobs keyed by environment, cheapest
first — and order is every environment that has work, in key order (§7).

A var initializer rather than an `init()`, which is load-bearing rather than
style: package-level variables are initialized before `init()` runs, and by
dependency. Anything else in this package that reads a role while *it* is being
initialized therefore sees a full catalogue, instead of an empty map and a panic
that only reproduces in whichever file the linker happened to order first.
*/
var catalogue, order = loadJobs(content.FS)

// loadJobs parses the shipped jobs and refuses to boot without them.
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

There is deliberately no `environment`: it is the directory the file is in, and a
role that could name a different one would be a role filed under two answers.
There is also no `nights` — a shift starting at 22:00 is a night shift and
saying so twice is two facts that can disagree.
*/
type jobFile struct {
	Name string `yaml:"name"`
	// Pay is cents a day. Every job states its own.
	Pay int64 `yaml:"pay"`
	// Hours is the length of the shift. A whole number, because no posting in
	// this world advertises eight and a half.
	Hours int `yaml:"hours"`
	// Fed is whether the shift leaves the player's eating alone. Absent is
	// false, which is the honest default for work that does not sit down.
	Fed bool `yaml:"fed"`
	// Requires is the capability floor, by factor name, on the 0–100 scale a
	// reading is. Absent is a job that takes anybody who turns up.
	Requires map[string]float64 `yaml:"requires"`
	/*
		Experience is the previous work that qualifies somebody, and exactly one
		of its two keys is written.

		`all_of` is one route: a map of role key to cumulative days. `any_of` is
		a list of routes, any one of which qualifies. The words are in the file
		rather than implied by the shape (the decision of 2026-08-15) — a list
		that silently meant OR and a map that silently meant AND is a boolean
		expression nobody wrote down, in a document an author is meant to be
		able to read.

		Depth stops at two. `any_of` holds `all_of` blocks and nothing else, and
		`all_of` holds roles and nothing else.
	*/
	Experience *struct {
		AllOf map[string]int              `yaml:"all_of"`
		AnyOf []map[string]map[string]int `yaml:"any_of"`
	} `yaml:"experience"`
	// Delegated is whether the job runs while the player is elsewhere and hands
	// them situations (§3.14's loop). Authored per job rather than implied by
	// seniority.
	Delegated bool `yaml:"delegated"`
	// Posting is the advert in the sections a real one has. The contract —
	// budgets, register, what may never be written — is content/jobs/SPEC.md,
	// and the budgets are enforced in [posting].
	Posting struct {
		Intro          string   `yaml:"intro"`
		Duties         []string `yaml:"duties"`
		Qualifications []string `yaml:"qualifications"`
	} `yaml:"posting"`
	// Shift is when the working day starts, "HH:MM". The end is `hours` later,
	// and whether it is night work is read off the clock rather than authored
	// twice — see [shiftStart].
	Shift string `yaml:"shift"`
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
parseJobs reads every environment's jobs and checks what the rest of the
package then gets to assume.

Strict twice over, like internal/environment's loader: an unknown YAML field is
a failed build rather than an authored line nobody notices has no effect, and
every cross-reference — a place, a zone, a job — is resolved here rather than
at the moment a player happens to walk into it.
*/
func parseJobs(files fs.FS) (map[string][]Role, []string, error) {
	dirs, err := fs.ReadDir(files, jobsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", jobsPath, err)
	}

	out := map[string][]Role{}
	var keys []string
	// Keys are unique across the whole world rather than within an environment: the
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

		var jobs []Role
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			name := path.Join(jobsPath, env, entry.Name())

			var file jobFile
			if err := content.Decode(files, name, &file); err != nil {
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
			jobs = append(jobs, role)
		}

		if len(jobs) < Least {
			return nil, nil, fmt.Errorf("%s/%s offers %d job(s); a population is offered at least %d",
				jobsPath, env, len(jobs), Least)
		}
		// Cheapest first, ties in key order, so the board has an order two
		// processes agree on (§7).
		sort.SliceStable(jobs, func(i, j int) bool {
			if jobs[i].Pay != jobs[j].Pay {
				return jobs[i].Pay < jobs[j].Pay
			}
			return jobs[i].Key < jobs[j].Key
		})
		out[env], keys = jobs, append(keys, env)
	}

	if len(out) == 0 {
		return nil, nil, fmt.Errorf("%s holds no work", jobsPath)
	}
	sort.Strings(keys)
	if err := check(out, keys); err != nil {
		return nil, nil, err
	}
	return out, keys, nil
}

/*
check holds the experience graph to the three things the rest of the game
assumes about it, once every environment is parsed.

None of them could be checked a file at a time, which is why they are here:
each one is about a job's relationship to the other jobs around it.

  - **Every route names work that exists, where this player lives.** A
    prerequisite pointing at another environment's job would be a posting nobody
    living here could ever answer — §3.15's transfer is about meeting *different*
    work, and an unreachable vacancy is a locked door drawn on the board.
  - **The graph is acyclic.** Two jobs each requiring days of the other is a
    pair nobody can ever enter, and it would fail silently: the board would show
    both, forever, with nothing to say about why neither opens.
  - **Every environment has a way in.** At least one job asking for no previous
    work. Without it a population is one nobody can start working in, which is
    the check that actually matters — the other two are typos.
*/
func check(all map[string][]Role, envs []string) error {
	for _, env := range envs {
		jobs := all[env]
		here := map[string]Role{}
		for _, r := range jobs {
			here[r.Key] = r
		}

		var ways int
		for _, r := range jobs {
			if !r.Experience.Asks() {
				ways++
				continue
			}
			for _, want := range r.Experience.Roles() {
				if _, known := here[want]; !known {
					return fmt.Errorf("%s/%s: %q asks for experience of %q, which is not work in %s",
						jobsPath, env, r.Key, want, env)
				}
			}
		}
		if ways == 0 {
			return fmt.Errorf("%s/%s: every job asks for experience, so nobody living there can start working",
				jobsPath, env)
		}
		if cycle := cycles(here); cycle != "" {
			return fmt.Errorf("%s/%s: %s ask for experience of each other, so none of them can be reached",
				jobsPath, env, cycle)
		}
	}
	return nil
}

/*
cycles finds a job that requires its own experience, however far around.

A depth-first walk with the two-colour marking, over one environment's graph.
The names of a cycle rather than a boolean, because "there is a cycle in your
job graph" is a sentence that sends somebody reading thirteen files.
*/
func cycles(here map[string]Role) string {
	const (
		walking = 1
		done    = 2
	)
	mark := map[string]int{}

	keys := make([]string, 0, len(here))
	for key := range here {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var walk func(string, []string) string
	walk = func(key string, path []string) string {
		switch mark[key] {
		case done:
			return ""
		case walking:
			// Back at somewhere still on the stack. The cycle is the tail of
			// the path from where it was first entered.
			for i, seen := range path {
				if seen == key {
					return strings.Join(append(path[i:], key), " → ")
				}
			}
			return key
		}
		mark[key] = walking
		for _, want := range here[key].Experience.Roles() {
			if _, known := here[want]; !known {
				continue
			}
			if found := walk(want, append(path, key)); found != "" {
				return found
			}
		}
		mark[key] = done
		return ""
	}

	for _, key := range keys {
		if found := walk(key, nil); found != "" {
			return found
		}
	}
	return ""
}

// The bounds a shift and a wage have to sit inside. Not tuning — the outer edge
// of what a job in this world can claim to be, so that a typo of a factor of ten
// is a failed build rather than a posting paying eight thousand a day.
const (
	shortestShift = 1
	longestShift  = 16
	mostPay       = 100_000 // a thousand dollars a day, in cents
)

// job reads and checks one file.
func job(env, key string, file jobFile) (Role, error) {
	if key == "" {
		return Role{}, fmt.Errorf("a job has no key")
	}
	if strings.TrimSpace(file.Name) == "" {
		return Role{}, fmt.Errorf("%q has no name", key)
	}
	if file.Pay <= 0 || file.Pay > mostPay {
		return Role{}, fmt.Errorf("%q pays %d cents a day", key, file.Pay)
	}
	if file.Hours < shortestShift || file.Hours > longestShift {
		return Role{}, fmt.Errorf("%q works a shift of %d hours; a shift is %d to %d",
			key, file.Hours, shortestShift, longestShift)
	}
	advert, err := posting(key, file)
	if err != nil {
		return Role{}, err
	}
	// The shift decides whether this is night work. One authored fact, read two
	// ways, rather than two facts that can disagree.
	start, nights, err := shiftStart(key, file.Shift)
	if err != nil {
		return Role{}, err
	}
	needs, err := requires(key, file.Requires)
	if err != nil {
		return Role{}, err
	}
	route, err := experience(key, file)
	if err != nil {
		return Role{}, err
	}

	out := Role{
		Environment: env,
		Key:         key,
		Name:        strings.TrimSpace(file.Name),
		Posting:     advert,
		ShiftStart:  start,
		Pay:         simulation.Money(file.Pay),
		Post: simulation.Post{
			Hours:  time.Duration(file.Hours) * time.Hour,
			Nights: nights,
			Fed:    file.Fed,
		},
		Needs:      needs,
		Experience: route,
		Delegated:  file.Delegated,
		Interview:  file.Interview,
		Offered:    file.Offered,
	}

	for _, name := range sortedTolls(file.Toll) {
		which, err := granted(name)
		if err != nil {
			return Role{}, fmt.Errorf("%q %w", key, err)
		}
		out.Toll = append(out.Toll, simulation.Grant{Factor: which, Credit: file.Toll[name]})
	}

	// Where it is taken on. A role with nowhere is one nothing hires into yet —
	// on the board and out of reach — so an absent `hired` is allowed, and half
	// of one is not.
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
shiftStart reads "HH:MM" into minutes from midnight, and says whether that is
night work.

One authored fact rather than two. A file used to state a start and inherit
`nights` from its rung, which made "Afternoons at the laundromat" on a nights
tier a thing the loader had to catch; now the clock decides. A shift beginning
at or after 18:00, or before 05:00, is worked against the body clock, and there
is no way for a posting to claim otherwise.

The end needs no field: it is the start plus the job's hours, wherever that
lands.
*/
func shiftStart(key, shift string) (int, bool, error) {
	var h, m int
	if n, err := fmt.Sscanf(shift, "%2d:%2d", &h, &m); err != nil || n != 2 ||
		h < 0 || h > 23 || m < 0 || m > 59 || len(shift) != 5 {
		return 0, false, fmt.Errorf("%q has a shift of %q; a shift is \"HH:MM\"", key, shift)
	}
	start := h*60 + m
	const morning, afternoon, evening = 5 * 60, 14 * 60, 18 * 60
	// The hours nobody starts a shift at. A job beginning at three in the
	// afternoon is neither a day nor a night as this simulation understands
	// them, and letting one through would put a body somewhere Working has no
	// arithmetic for.
	if start > afternoon && start < evening {
		return 0, false, fmt.Errorf("%q starts at %s; a shift starts between 05:00 and 14:00 or at or after 18:00",
			key, shift)
	}
	return start, start >= evening || start < morning, nil
}

// requires reads the capability floor, refusing a factor the world does not
// have and a reading off the 0–100 scale.
func requires(key string, want map[string]float64) (simulation.Requirement, error) {
	var out simulation.Requirement
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		at := want[name]
		if at <= 0 || at > 100 {
			return out, fmt.Errorf("%q requires %s of %g; a reading is 0 to 100", key, name, at)
		}
		var known bool
		for _, f := range simulation.Factors {
			if !strings.EqualFold(f.String(), name) {
				continue
			}
			known = true
			switch f {
			case simulation.Health:
				out.Health = at
			case simulation.Wealth:
				out.Money = at
			case simulation.Intelligence:
				out.Intelligence = at
			case simulation.Stability:
				out.Stability = at
			default:
				out.Kindness = at
			}
		}
		if !known {
			return out, fmt.Errorf("%q requires %q, which is not a factor", key, name)
		}
	}
	return out, nil
}

/*
experience reads the routes of previous work that qualify somebody for a job.

Exactly one of `all_of` and `any_of`, and saying so is the point: the author
writes the operator rather than having it inferred from whether they typed a map
or a list (2026-08-15). Both absent is an entry-level posting.

What it cannot check here is whether the roles named exist — that needs the whole
catalogue, and it happens in [check] once every environment is parsed.
*/
func experience(key string, file jobFile) (Experience, error) {
	block := file.Experience
	if block == nil {
		return nil, nil
	}
	switch {
	case len(block.AllOf) > 0 && len(block.AnyOf) > 0:
		return nil, fmt.Errorf("%q writes both all_of and any_of; a posting asks for one or the other", key)
	case len(block.AllOf) == 0 && len(block.AnyOf) == 0:
		return nil, fmt.Errorf("%q has an empty experience block; a job that asks for none writes none", key)
	case len(block.AllOf) > 0:
		route, err := allOf(key, block.AllOf)
		if err != nil {
			return nil, err
		}
		return Experience{route}, nil
	}

	out := make(Experience, 0, len(block.AnyOf))
	for i, one := range block.AnyOf {
		inner, ok := one["all_of"]
		if !ok || len(one) != 1 {
			return nil, fmt.Errorf("%q writes an any_of route that is not an all_of block; "+
				"every route names its operator", key)
		}
		route, err := allOf(key, inner)
		if err != nil {
			return nil, err
		}
		for _, before := range out[:i] {
			if same(before, route) {
				return nil, fmt.Errorf("%q offers the same route twice", key)
			}
		}
		out = append(out, route)
	}
	return out, nil
}

// allOf reads one route: every job in it, for a real number of days.
func allOf(key string, block map[string]int) (Route, error) {
	if len(block) == 0 {
		return nil, fmt.Errorf("%q writes an empty all_of; a route names at least one job", key)
	}
	out := Route{}
	for role, days := range block {
		if role == key {
			return nil, fmt.Errorf("%q asks for experience of itself", key)
		}
		if days <= 0 {
			return nil, fmt.Errorf("%q asks for %d days of %q", key, days, role)
		}
		out[role] = days
	}
	return out, nil
}

// same reports whether two routes ask for exactly the same work.
func same(a, b Route) bool {
	if len(a) != len(b) {
		return false
	}
	for role, days := range a {
		if b[role] != days {
			return false
		}
	}
	return true
}

/* ── reading the catalogue ──────────────────────────────────────────────── */

/*
RolesIn is the work one population is offered, cheapest first.

The empty slice for an environment nobody authored jobs for, which is a real
state of the world rather than an error: a player among nobody
([environment.Nowhere]) has no local work, and a board with nothing on it is the
honest answer.
*/
func RolesIn(env string) []Role {
	jobs := catalogue[env]
	out := make([]Role, len(jobs))
	copy(out, jobs)
	return out
}

/*
All is every authored role in the world, environment by environment.

For the callers that mean "the whole catalogue" rather than "the work here":
the boot checks that run over shipped content. The *board* is [RolesIn] and is
local — see [every].
*/
func All() []Role {
	var out []Role
	every(func(r Role) bool { out = append(out, r); return true })
	return out
}

// Environments is every environment that has work, in key order.
func Environments() []string {
	out := make([]string, len(order))
	copy(out, order)
	return out
}

/*
every is every authored role in the world, environment by environment in key
order.

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
