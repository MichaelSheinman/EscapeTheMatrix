// Package routine owns the player's routines: the rules they have set for their
// own body.
//
// HighLevelOverview.md §3.4 makes routines the early game's main loop — sleep,
// workout, diet and organization, configured once and then watched — and §3.14
// says the thing that decides this package's shape: **a routine is already a
// rule, and the playbook is the same concept pointed at judgement.** One system,
// one interface language. So a routine here is a standing *question* about the
// body and a closed list of *answers*, each written as a sentence, which is
// exactly the form the playbook composes in. Nothing on this surface takes
// typing, and nothing on it is a number the player sets.
//
// The split against internal/simulation is the same one internal/health already
// keeps: the pure core holds the mechanics (what a routine does to a body), and
// this package holds the content (which routines exist, what they ask, and what
// a player may answer). A tuning pass changes constants in the core; an authored
// pass changes sentences here; neither is a migration.
//
// It owns the `routine` table (§3.1), takes the caller's transaction and never
// opens one (§3.2), and reads no other module's tables.
package routine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/simulation"
)

// The refusals this module has. Go errors rather than status codes:
// internal/api maps them at the edge (§8), so a module never learns HTTP.
var (
	// ErrNoSuchRoutine is a routine key that is not one of the four.
	ErrNoSuchRoutine = errors.New("routine: no such routine")
	// ErrNoSuchOption is an answer that is not in that routine's vocabulary.
	ErrNoSuchOption = errors.New("routine: no such option")
)

/*
Kind is one routine: a standing question about the body, and the answers it will
take.

The vocabulary is the design (§3.14). What a player can put in a routine defines
the space of lives they can think of arranging, which is why the options below
are sentences a person would actually say rather than levels of a dial — and why
adding one is an authored decision rather than a widening of a range.
*/
type Kind struct {
	Key  string
	Name string
	// Asks is the standing question, in the player's own words. The playbook
	// poses its rules the same way ("when someone asks you for help, say yes
	// if…"), and routines read as answers to questions for the same reason:
	// a rule you can say out loud is a rule you can notice was wrong.
	Asks string
	// Options are the answers, ordered from the life that happens on its own to
	// the most deliberate one. The first is therefore also the default: an
	// unconfigured player is not a player with no routine, they are a player
	// whose routine is whatever happens.
	Options []Option
}

// Default is the answer a player who has not answered has.
func (k Kind) Default() string { return k.Options[0].Key }

// Option finds an answer by key.
func (k Kind) Option(key string) (Option, bool) {
	for _, o := range k.Options {
		if o.Key == key {
			return o, true
		}
	}
	return Option{}, false
}

/*
Option is one answer, and what choosing it does.

`Says` is the whole of what the player reads. `Costs` is the honest half — what
the option asks of them, or what it trades away — because §3.2 says nothing is
labelled good or bad, and the way to keep that true while still being fair is to
state the price rather than the verdict.
*/
type Option struct {
	Key   string
	Says  string
	Costs string
	// into writes this option's mechanics into a routine. Unexported: the
	// numbers are the pure core's business and the wire's business is the
	// sentence, so there is no way for a client to be sent one of these.
	into func(*simulation.Routine)
}

// The four routines §3.4 names, in the order they are offered.
//
// Sleep first because everything else answers to it — the panel's own drivers
// say so — and organization last because it is the one that decides how much of
// the other three actually happens.
var (
	Sleep = Kind{
		Key:  "sleep",
		Name: "Sleep",
		Asks: "What happens when the night gets late?",
		Options: []Option{
			{
				Key:   "whenever",
				Says:  "I sleep when I run out of day.",
				Costs: "Nothing to keep up. The debt is the price, and it is paid later.",
				into:  func(r *simulation.Routine) { r.Sleep = 5*time.Hour + 30*time.Minute },
			},
			{
				Key:   "midnight",
				Says:  "I am in bed by midnight, every night.",
				Costs: "The end of the evening. About seven hours, which is nearly enough.",
				into:  func(r *simulation.Routine) { r.Sleep = 7 * time.Hour },
			},
			{
				Key:   "protected",
				Says:  "Eight and a half hours, and nothing is allowed to take them.",
				Costs: "Evenings, and the plans other people make in them.",
				into:  func(r *simulation.Routine) { r.Sleep = 8*time.Hour + 30*time.Minute },
			},
		},
	}

	Workout = Kind{
		Key:  "workout",
		Name: "Training",
		Asks: "When does training happen?",
		Options: []Option{
			{
				Key:   "whenever",
				Says:  "I train when I feel like it.",
				Costs: "Nothing. Also nothing happens.",
				into:  func(r *simulation.Routine) { r.Training = week(everyDay(simulation.Rest)) },
			},
			{
				Key:   "daily",
				Says:  "Something every day, nothing heroic.",
				Costs: "An hour a day, and no session you can be proud of.",
				into:  func(r *simulation.Routine) { r.Training = week(everyDay(simulation.Light)) },
			},
			{
				Key:   "split",
				Says:  "Three hard sessions a week, easy in between, Sunday off.",
				Costs: "Six days that are spoken for. The rest days are part of it.",
				into: func(r *simulation.Routine) {
					r.Training = week(map[time.Weekday]simulation.Training{
						time.Monday: simulation.Hard, time.Tuesday: simulation.Light,
						time.Wednesday: simulation.Hard, time.Thursday: simulation.Light,
						time.Friday: simulation.Hard, time.Saturday: simulation.Light,
						time.Sunday: simulation.Rest,
					})
				},
			},
			{
				Key:   "relentless",
				Says:  "Hard, six days out of seven.",
				Costs: "Everything the recovery costs, which is not visible from here.",
				into: func(r *simulation.Routine) {
					w := everyDay(simulation.Hard)
					w[time.Sunday] = simulation.Rest
					r.Training = week(w)
				},
			},
		},
	}

	Diet = Kind{
		Key:  "diet",
		Name: "Food",
		Asks: "What do you eat when nobody has planned it?",
		Options: []Option{
			{
				Key:   "whatever",
				Says:  "Whatever is in front of me.",
				Costs: "No time at all. It is the cheapest thing here and the dearest.",
				into:  func(r *simulation.Routine) { r.Diet = simulation.Poor },
			},
			{
				Key:   "sometimes",
				Says:  "I cook when I have the time.",
				Costs: "A few evenings, and a week that is better on the quiet ones.",
				into:  func(r *simulation.Routine) { r.Diet = simulation.Mixed },
			},
			{
				Key:   "cooked",
				Says:  "I cook what I eat, and the week's food is bought on Sunday.",
				Costs: "A Sunday afternoon, every Sunday.",
				into:  func(r *simulation.Routine) { r.Diet = simulation.Clean },
			},
		},
	}

	Organization = Kind{
		Key:  "organization",
		Name: "Organization",
		Asks: "How much of the week is decided in advance?",
		Options: []Option{
			{
				Key:   "loose",
				Says:  "I keep it in my head.",
				Costs: "Nothing to maintain. Half of what you intend does not happen, and carrying it is its own low-grade load.",
				into:  func(r *simulation.Routine) { r.Organization = simulation.Loose },
			},
			{
				Key:   "listed",
				Says:  "Tomorrow is written down before I sleep.",
				Costs: "Ten minutes a night. Most of what you intend happens.",
				into:  func(r *simulation.Routine) { r.Organization = simulation.Listed },
			},
			{
				Key:   "regimented",
				Says:  "Every hour of the week is assigned.",
				Costs: "Nearly all of it happens — and the arrangement becomes a load you are carrying too.",
				into:  func(r *simulation.Routine) { r.Organization = simulation.Regimented },
			},
		},
	}
)

// Kinds is every routine, in the order it is offered.
var Kinds = []Kind{Sleep, Workout, Diet, Organization}

func kind(key string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Key == key {
			return k, true
		}
	}
	return Kind{}, false
}

// week fills a training week, so that an option states only the days it has an
// opinion about and the rest are rest.
func week(days map[time.Weekday]simulation.Training) [7]simulation.Training {
	var out [7]simulation.Training
	for day, t := range days {
		out[day] = t
	}
	return out
}

func everyDay(t simulation.Training) map[time.Weekday]simulation.Training {
	out := make(map[time.Weekday]simulation.Training, 7)
	for day := time.Sunday; day <= time.Saturday; day++ {
		out[day] = t
	}
	return out
}

/*
Choices is the player's answer to every routine: routine key to option key.

It is the whole configuration and it is the only thing that gets written. There
is no command to change one routine, because a set of rules for the body is one
decision — you do not decide to sleep more without deciding what that costs the
evening — and holding it as a document rather than as four columns is what keeps
that true at rest as well as in the API.
*/
type Choices map[string]string

// Configured is a player's routines: what they answered, and whether they have
// ever answered at all.
type Configured struct {
	// Choices is complete: every routine has an entry, defaulted for the ones
	// the player has not answered, so nothing downstream has to decide what an
	// absent answer means.
	Choices Choices
	// Set is whether the player has ever configured routines. Different from
	// having chosen the defaults, which is a decision.
	Set bool
}

// Chosen is the option in force for a routine.
func (c Configured) Chosen(k Kind) Option {
	if o, ok := k.Option(c.Choices[k.Key]); ok {
		return o
	}
	// An option renamed out of the vocabulary since it was chosen. The default
	// rather than an error: content moves, and a player standing on the old
	// version has to be able to walk out of it.
	o, _ := k.Option(k.Default())
	return o
}

// Routine is the rules these answers add up to, for the pure core to live by.
func (c Configured) Routine() simulation.Routine {
	var r simulation.Routine
	for _, k := range Kinds {
		c.Chosen(k).into(&r)
	}
	return r
}

/*
Routines is the module. It holds no per-player state: the transaction comes from
the caller (§3.1).

Every routine is configurable by everybody, always. #9 built this package behind
a `Gate` interface, and #32 bound it to a diagnosis so that a routine arrived
only once the player had named the problem it solves (§3.6). That gate is
withdrawn: the four routines are the early game's main loop (§3.4) and a player
who cannot touch them has no loop to play. Diagnosis survives as a mechanic in
internal/diagnosis; what it does not do any more is hand a tool over, so nothing
in this package asks anything about a player's history.
*/
type Routines struct{}

func New() *Routines { return &Routines{} }

// Of reads the player's routines.
//
// A player with no row is not an error: they are somebody who has not set any
// routines, which is a state the game has plenty to say about. What comes back
// is complete either way, so nothing downstream has to know which it was.
func (m *Routines) Of(ctx context.Context, tx pgx.Tx, accountID string) (Configured, error) {
	var (
		raw   []byte
		setAt *time.Time
	)
	err := tx.QueryRow(ctx,
		`SELECT choices, set_at FROM routine WHERE account_id = $1`, accountID,
	).Scan(&raw, &setAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		raw, setAt = nil, nil
	case err != nil:
		return Configured{}, fmt.Errorf("routine: read: %w", err)
	}

	stored := Choices{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &stored); err != nil {
			return Configured{}, fmt.Errorf("routine: read choices: %w", err)
		}
	}

	out := Configured{
		Choices: make(Choices, len(Kinds)),
		Set:     setAt != nil,
	}
	for _, k := range Kinds {
		if _, ok := k.Option(stored[k.Key]); ok {
			out.Choices[k.Key] = stored[k.Key]
			continue
		}
		out.Choices[k.Key] = k.Default()
	}
	return out, nil
}

/*
Set writes the whole configuration.

Every routine is stated, every answer is checked against the vocabulary, and one
row is written. A routine left out of `in` is set to its default rather than left
as it was: this is the player saying what their life is, not patching one field
of it, and a partial write would leave the stored document meaning something
nobody said.

Nothing here refuses an answer for being unearned. Every routine takes every one
of its options from the first day, and the only refusals are a routine or an
option that does not exist.
*/
func (m *Routines) Set(ctx context.Context, tx pgx.Tx, accountID string, in Choices) (Configured, error) {
	for key := range in {
		if _, ok := kind(key); !ok {
			return Configured{}, fmt.Errorf("%w: %q", ErrNoSuchRoutine, key)
		}
	}

	chosen := make(Choices, len(Kinds))
	for _, k := range Kinds {
		answer, said := in[k.Key]
		if !said || answer == "" {
			answer = k.Default()
		}
		if _, ok := k.Option(answer); !ok {
			return Configured{}, fmt.Errorf("%w: %q is not an answer to %q", ErrNoSuchOption, answer, k.Key)
		}
		chosen[k.Key] = answer
	}

	document, err := json.Marshal(chosen)
	if err != nil {
		return Configured{}, fmt.Errorf("routine: write choices: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO routine (account_id, choices, set_at)
		VALUES ($1, $2, now())
		ON CONFLICT (account_id) DO UPDATE
		   SET choices = EXCLUDED.choices, set_at = now(), updated_at = now()`,
		accountID, document,
	); err != nil {
		return Configured{}, fmt.Errorf("routine: write: %w", err)
	}

	return m.Of(ctx, tx, accountID)
}

/*
Day is the day this player's routines produce for a date.

The seam the rest of the game uses. The interim command in internal/api calls it
for today; the daily rollover (#10) will call it for every day a player was away
for, in order — and because the pure core is deterministic in the date, those two
produce the same days for the same dates without having to agree about anything
else.

Nothing here decides whether the day may be applied. That is internal/health's
question, and it already has an answer for it.
*/
func (m *Routines) Day(ctx context.Context, tx pgx.Tx, accountID string, on simulation.Date) (simulation.Day, error) {
	c, err := m.Of(ctx, tx, accountID)
	if err != nil {
		return simulation.Day{}, err
	}
	return c.Routine().On(on), nil
}
