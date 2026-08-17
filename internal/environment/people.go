package environment

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/MichaelSheinman/EscapeTheMatrix/internal/city"
)

/*
The people of every population, one file per character under
`content/characters/<population>/<key>.yaml`.

Each carries the contacts card — where they live, how you know them, what they
do — and an introduction: a short exchange sitting at the top of their thread
from the first time the phone is opened.

The introduction is what a conflict line is allowed to lean on. Every conflict
is a question put to the player, so without one the only place to establish
anything about somebody is inside an ask, and a refusal ends up softened with
"your bin went out with mine this morning" about bins never mentioned.

An introduction is inert. [conflict.Conflicts.Threads] reads it from here and
renders it; the standing, the playbook and the record are all untouched, so
being introduced to somebody leaves the ledger exactly as it was.

The text is identical for every player, so it is read from content on each
request rather than written to a row when a life begins.
*/

const charactersPath = "characters"

/*
Texting is how somebody uses a phone, which is as much a fact about them as
what they do.

The population is legible through behaviour (§3.15), and by the time a player
has read four threads they know who sends six words at midnight and who sends
four hundred. So the habit is authored rather than left to whoever wrote the
line that day, and the bounds below are enforced per person — which is what
stops every character drifting to the same comfortable middle length.
*/
type Texting string

const (
	// Bursts is several short messages in a row: the name, then the reason,
	// then the afterthought, each its own bubble.
	Bursts Texting = "bursts"
	// Spare is as few words as the message can be sent in. Somebody who texts
	// this way withholds by habit rather than by intent, and what they leave
	// out is most of it.
	Spare Texting = "spare"
	// Rambles is one long message that arrives whole, goes around the houses
	// and buries the point in the middle of it.
	Rambles Texting = "rambles"
)

// Habits is the closed set, in no particular order — nothing ranks these.
var Habits = []Texting{Bursts, Spare, Rambles}

// bounds is what each habit is held to: how many messages, how long any one of
// them may run, and how many may arrive before the other person replies.
var bounds = map[Texting]struct{ fewest, most, said, run int }{
	// Many, short, and up to three before you get a word in.
	Bursts: {fewest: 4, most: 8, said: 110, run: 3},
	// Few and clipped. Two in a row is already unlike them.
	Spare: {fewest: 3, most: 5, said: 80, run: 2},
	// Long, and the length is the point — the ceiling is high enough to be a
	// wall of text, because that is what this person sends.
	Rambles: {fewest: 3, most: 5, said: 420, run: 2},
}

/*
Turn is one message in an introduction.

A bool rather than a speaker key: only two people are ever in an introduction,
and a turn attributed by key could put words in a third person's mouth.
Alternation is checked at load, so callers can rely on the turns running them,
you, them.
*/
type Turn struct {
	// Them is whether this is the neighbour speaking; false is the player.
	Them bool
	// Says is the message, in their words.
	Says string
}

// The YAML shape, kept apart from [Person] for the reason every loader here
// keeps them apart: what an author writes and what the program reads are two
// decisions, and a type that carried its file's field names would make renaming
// one a migration.
type characterFile struct {
	Key   string `yaml:"key"`
	Name  string `yaml:"name"`
	Among string `yaml:"among"`
	Note  string `yaml:"note"`
	Found string `yaml:"found"`
	Does  string `yaml:"does"`
	// Lives and Known are the contacts card: an address and a connection, both
	// facts. content/characters/SPEC.md is what keeps them from drifting into
	// the job `does` already has.
	Lives string `yaml:"lives"`
	Known string `yaml:"known"`
	// Texts is their habit on a phone, one of [Habits].
	Texts string `yaml:"texts"`
	// Elsewhere is §3.15's recommendation, as gossip, and it lives on a person
	// rather than on a population because a person is who says it. Exactly one
	// character in each population carries one — checked in [attach], which is
	// the only place that can see all of them at once.
	Elsewhere struct {
		Where string `yaml:"where"`
		Says  string `yaml:"says"`
	} `yaml:"elsewhere"`
	// Intro is how you met, them first and alternating. Written as a list of
	// one-key mappings so the file reads as a transcript:
	//
	//	intro:
	//	  - them: …
	//	  - you: …
	//	  - them: …
	Intro []struct {
		Them string `yaml:"them"`
		You  string `yaml:"you"`
	} `yaml:"intro"`
}

/*
people reads every character out of a content filesystem, by population.

One directory per population and one file per character, each named for its key,
so a population's membership is read off the directory rather than from a roster
that could disagree with it.
*/
func people(files fs.FS, deprecated map[string]bool) (map[string][]Person, error) {
	dirs, err := fs.ReadDir(files, charactersPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", charactersPath, err)
	}

	out := map[string][]Person{}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		among := dir.Name()
		here := path.Join(charactersPath, among)
		entries, err := fs.ReadDir(files, here)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", here, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			name := path.Join(here, entry.Name())

			var file characterFile
			if err := decode(files, name, &file); err != nil {
				return nil, err
			}
			p, err := character(file, deprecated[among])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			// The path is checked against the file's own fields, so a rename
			// cannot leave the two disagreeing about who this is.
			if stem := strings.TrimSuffix(entry.Name(), ".yaml"); p.Key != stem {
				return nil, fmt.Errorf("%s: is called %q", name, p.Key)
			}
			if file.Among != among {
				return nil, fmt.Errorf("%s: says they are among %q", name, file.Among)
			}
			out[among] = append(out[among], p)
		}
		// By key, so a population's people are in the same order in every
		// process (§7). The order is load-bearing beyond display: it breaks
		// ties in the day's selection and orders [Environment.Places].
		sort.Slice(out[among], func(i, j int) bool { return out[among][i].Key < out[among][j].Key })
	}
	return out, nil
}

// character reads and checks one file. `spent` marks a population on its way
// out, whose people need no introduction.
func character(file characterFile, spent bool) (Person, error) {
	if file.Key == "" {
		return Person{}, fmt.Errorf("somebody has no key")
	}
	if strings.TrimSpace(file.Name) == "" || strings.TrimSpace(file.Note) == "" {
		return Person{}, fmt.Errorf("%q needs a name and a note", file.Key)
	}
	// The load-bearing one. A person with no behaviour is a person whose values
	// are only knowable by being told them, which is the one way §3.15 says a
	// population may never be legible.
	if trim(file.Does) == "" {
		return Person{}, fmt.Errorf("%q does nothing, so nothing about them is legible from behaviour", file.Key)
	}
	// Somebody a player can look up is somebody with an address and a reason
	// they know them. Both required rather than optional, because a card that
	// fell back to the behaviour when a field was missing would quietly become
	// the roster panel §3.2 had removed.
	if trim(file.Lives) == "" || trim(file.Known) == "" {
		return Person{}, fmt.Errorf("%q needs somewhere they live and how you know them", file.Key)
	}

	// Reusing existing locations is §3.15's instruction, and somewhere in the
	// population's own district is what makes it a population rather than a
	// mailing list.
	found, ok := city.Lookup(file.Found)
	if !ok {
		return Person{}, fmt.Errorf("%q is found at %q, which is not a place", file.Key, file.Found)
	}
	if found.Key != file.Among && found.In != file.Among {
		return Person{}, fmt.Errorf("%q is found at %q, which is not in %q", file.Key, file.Found, file.Among)
	}

	out := Person{
		Key:   file.Key,
		Name:  strings.TrimSpace(file.Name),
		Note:  trim(file.Note),
		Found: file.Found,
		Does:  trim(file.Does),
		Lives: trim(file.Lives),
		Known: trim(file.Known),
	}
	habit, err := texting(file, spent)
	if err != nil {
		return Person{}, err
	}
	out.Texts = habit

	intro, err := introduction(file, habit, spent)
	if err != nil {
		return Person{}, err
	}
	out.Intro = intro

	// Carried on the person rather than assembled here: which population this
	// is the word for, and whether it is the only one, are questions [attach]
	// answers with the whole world in hand.
	if said := trim(file.Elsewhere.Says); said != "" || file.Elsewhere.Where != "" {
		if said == "" || file.Elsewhere.Where == "" {
			return Person{}, fmt.Errorf("%q points somewhere else without a word about it, or the other way about", file.Key)
		}
		out.elsewhere = Elsewhere{Who: file.Key, Where: file.Elsewhere.Where, Says: said}
	}

	for what, text := range map[string]string{
		"name": out.Name, "note": out.Note, "behaviour": out.Does,
		"address": out.Lives, "connection": out.Known,
	} {
		if word := Labelled(text); word != "" {
			return Person{}, fmt.Errorf("%q says %q in their %s, which labels the framework (§3.2)",
				file.Key, word, what)
		}
	}
	for i, turn := range out.Intro {
		if word := Labelled(turn.Says); word != "" {
			return Person{}, fmt.Errorf("%q says %q in how you met (%d), which labels the framework (§3.2)",
				file.Key, word, i+1)
		}
	}
	if word := Labelled(out.elsewhere.Says); word != "" {
		return Person{}, fmt.Errorf("%q says %q about %q, which labels the framework (§3.2)",
			file.Key, word, out.elsewhere.Where)
	}
	return out, nil
}

// texting reads the phone habit, which every character has and a retired one
// does not need.
func texting(file characterFile, spent bool) (Texting, error) {
	habit := Texting(strings.TrimSpace(file.Texts))
	if habit == "" {
		if spent {
			return "", nil
		}
		return "", fmt.Errorf("%q has no way of texting; every character has one of %v", file.Key, Habits)
	}
	if _, known := bounds[habit]; !known {
		return "", fmt.Errorf("%q texts %q, which is not one of %v", file.Key, habit, Habits)
	}
	if spent {
		return "", fmt.Errorf("%q says how they text, and %q is on its way out", file.Key, file.Among)
	}
	return habit, nil
}

/*
introduction reads the exchange: them first, and both of them in it.

Consecutive bubbles from one person are how texting works — you say who you are,
then you say what you want, and they are two messages. What is bounded instead
is the length of any one of them and how many can arrive before the other
person gets a word in ([Said] and [Run]), which is what keeps a burst from
becoming a monologue.

`spent` excuses the intro entirely, and refuses one that is written anyway. A
character without an intro simply has no thread until somebody asks them
something, which is the right cost for a population being retired.
*/
func introduction(file characterFile, habit Texting, spent bool) ([]Turn, error) {
	if len(file.Intro) == 0 {
		if spent {
			return nil, nil
		}
		return nil, fmt.Errorf("%q has no introduction, so their first word to you is an ask", file.Key)
	}
	if spent {
		return nil, fmt.Errorf("%q has an introduction, and %q is on its way out", file.Key, file.Among)
	}
	held := bounds[habit]
	if len(file.Intro) < held.fewest || len(file.Intro) > held.most {
		return nil, fmt.Errorf("%q %s and is introduced in %d message(s); that is %d to %d",
			file.Key, habit, len(file.Intro), held.fewest, held.most)
	}

	out := make([]Turn, 0, len(file.Intro))
	run, spoke := 0, false
	for i, turn := range file.Intro {
		them, you := trim(turn.Them), trim(turn.You)
		switch {
		case them != "" && you != "":
			return nil, fmt.Errorf("%q's introduction has both of you speaking at message %d", file.Key, i+1)
		case them == "" && you == "":
			return nil, fmt.Errorf("%q's introduction says nothing at message %d", file.Key, i+1)
		}
		// Them first: an introduction opening on the player is the player
		// texting a stranger.
		if i == 0 && them == "" {
			return nil, fmt.Errorf("%q's introduction opens on the player, who has no reason to text a stranger", file.Key)
		}

		said, mine := them, them != ""
		if !mine {
			said, spoke = you, true
		}
		// The player is held to the neighbour's bounds too. Somebody texting a
		// person who sends four words does not send four hundred back, and a
		// thread where the two of them write nothing like each other reads as
		// two authors rather than two people.
		if len(said) > held.said {
			return nil, fmt.Errorf("%q %s and runs to %d characters at message %d; the budget is %d",
				file.Key, habit, len(said), i+1, held.said)
		}
		if i > 0 && out[i-1].Them == mine {
			run++
		} else {
			run = 1
		}
		if run > held.run {
			return nil, fmt.Errorf("%q %s and sends %d messages in a row without a reply; the most is %d",
				file.Key, habit, run, held.run)
		}
		out = append(out, Turn{Them: mine, Says: said})
	}
	// Both of them in it, or it is somebody talking at the player rather than
	// the two of them getting each other's numbers.
	if !spoke {
		return nil, fmt.Errorf("%q's introduction never has the player say anything", file.Key)
	}
	return out, nil
}

/*
attach puts the people into their populations and checks the three rules no
single file can settle: that a population has enough people to be one, that
person keys do not collide across the world, and that exactly one person in each
carries the word about somewhere else.
*/
func attach(all []Environment, by map[string][]Person, nouns map[string][]string) ([]Environment, error) {
	// Person keys are unique across the whole world rather than within a
	// population. #30 selects a conflict from relationship state, and a
	// relationship is with a person: two Bezes in two populations would be one
	// row that means different people depending on where its owner lives.
	who := map[string]string{}
	known := map[string]bool{}
	for _, e := range all {
		known[e.Key] = true
	}
	for among := range by {
		if !known[among] {
			return nil, fmt.Errorf("%s/%s is nobody's population", charactersPath, among)
		}
	}

	for i, e := range all {
		here := by[e.Key]
		if len(here) < Company {
			return nil, fmt.Errorf("%q has %d person/people; a population is at least %d",
				e.Key, len(here), Company)
		}

		var said []Person
		for _, p := range here {
			if other, taken := who[p.Key]; taken {
				return nil, fmt.Errorf("%q is already somebody in %q", p.Key, other)
			}
			who[p.Key] = e.Key
			if p.elsewhere.Where != "" {
				said = append(said, p)
			}
		}
		// §3.15's recommendation. None leaves the loop without a next
		// population; more than one reads as a leaflet rather than as somebody
		// mentioning a place for their own reasons.
		switch {
		case len(said) == 0:
			return nil, fmt.Errorf("nobody in %q has a word about anywhere else; §3.15's loop has no last population", e.Key)
		case len(said) > 1:
			return nil, fmt.Errorf("%q has %d people talking about somewhere else; it wants one", e.Key, len(said))
		case said[0].elsewhere.Where == e.Key:
			return nil, fmt.Errorf("%q points at itself as somewhere else", e.Key)
		case !known[said[0].elsewhere.Where]:
			return nil, fmt.Errorf("%q points at %q, which is not a population", e.Key, said[0].elsewhere.Where)
		}

		all[i].people = here
		all[i].Elsewhere = said[0].elsewhere

		// Last for this population, and after the people are attached: who may
		// be named is who lives here, which is not knowable from one file.
		if err := naming(all[i], nouns[e.Key]); err != nil {
			return nil, err
		}
	}
	return all, nil
}
