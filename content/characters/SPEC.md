# Character spec

One file per person, at `content/characters/<population>/<key>.yaml`. The
population file next door states what somewhere holds and affords; this states
who lives there.

Everything a player reads here is written as **behaviour and fact**. Never as a
verdict, never as an adjective about a person, and never with a word that names
the model underneath — the loader's `Labelled` check makes a stage name, a
framework word or a developmental colour a failed build rather than a review
comment.

The game is a life simulator, so the real-world counterpart of each field is
named below and the prose imitates it, trimmed to game length. There is no
literary voice anywhere in a character file.

## The fields

| field | real-world counterpart | what it is |
| ----- | ---------------------- | ---------- |
| `key` | — | the filename, and how conflicts name them in `with:`. |
| `name` | a name | what they are called. |
| `among` | — | the population's key, matching the directory. |
| `found` | — | an internal/city key inside that district. Mechanical. |
| `note` | how you would place somebody to a third party | one line, where they sit in the world. |
| `lives` | the address line of a contacts card | the building by name and the unit, plus at most one household fact. |
| `known` | how you would answer "who's that?" | how the character comes to know them at all: the connection, and nothing about its quality. |
| `texts` | how they use a phone | `bursts`, `spare` or `rambles`. See below. |
| `intro` | the first few texts with somebody whose number you have just got | how you met. Two to four turns, them first, alternating. |

## `lives` and `known`, specifically

They exist because a phone has a contacts app, and a contacts card holds facts
about somebody rather than an assessment of them.

- **`known` states a connection, never its temperature.** "Your neighbour on the
  landing" is a fact. "Your closest neighbour" is a conclusion the player is
  supposed to reach by living there, and "she has been good to you" is that
  conclusion said out loud.
- **`lives` states an address, never a household's condition.** "12 Rowan St,
  flat 1C, with her son" is a fact. "12 Rowan St, flat 1C, struggling since her
  hours were cut" is the writing doing the player's reading for them.
- **The address comes from the map.** The street or building is one the world
  draws — a numbered door on a drawn street, or a building the map labels by
  name — never a name invented for the card. A new name is added to the map
  first, then to the population's `nouns:` list, then to a card.
- Neither field ever previews behaviour. Anything that begins "will", "tends
  to", "always" or "never" belongs in a conflict or nowhere. What somebody can
  be relied on to do is the conflicts that name them in `with:`.
- Both are one line. Two sentences is a paragraph pretending to be a caption.

The rest of a card is derived, so the prose never states it: where they turn up
comes from `found`, and what is outstanding between the two of you comes from
the standing the simulation keeps.

## The introduction

```yaml
intro:
  - them: "Hey! It's Sharon, second floor, same stairs as you."
  - them: "Boxes on the landing are my sister's, by the way. She's in with me now."
  - you:  "Wondered whose they were."
  - them: "My brother had the lot over in one run in his van."
```

**It is a text thread, so it comes in bubbles, not paragraphs.** Somebody says
who they are, then says why they are texting, and those are two messages. One
message carrying the whole of it is an email. This is the failure to watch for:
everything a person has to say arriving in a single dense block.

It sits at the top of that person's thread from the first time the phone is
opened, and reading it changes nothing — no question is put, no answer is
chosen, and the standing is untouched.

**What it is for.** Every conflict is a question put to the player. The intro is
the only place a person establishes anything about themselves without wanting
something, and it is what their later asks are allowed to lean on. A refusal
softened with "your bin went out with mine this morning" reads as invented
unless the bins are already between you.

**So it must put something concrete on the table.** One or two specifics that
this person's conflicts already refer to — the till, the bins, the second floor,
the car at Denny's, the rota. A first exchange that establishes nothing is worse
than no intro at all, because it costs the player a read and gives the writing
nothing.

**Rules the loader holds it to.** The neighbour sends the first message; the
player says something somewhere; and the length, count and run are whatever
`texts:` allows — see below.

## `texts:` — how they use a phone

How somebody texts is a fact about them, the same as what they do, and by the
fourth thread a player should know who is typing before they read the name. So
it is authored, and the loader enforces different bounds per habit — which is
what stops every character drifting to the same comfortable middle length.

| `texts:` | what it is | messages | per message | in a row |
| -------- | ---------- | -------- | ----------- | -------- |
| `bursts` | the name, then the reason, then the afterthought, each its own bubble | 4–8 | 110 | 3 |
| `spare` | as few words as it can be sent in; leaves out what they assume you know | 3–5 | 80 | 2 |
| `rambles` | arrives whole, goes round the houses, buries the point in the middle | 3–5 | 420 | 2 |

`spare` is brief to the point of being slightly hard to follow — not rude.
`rambles` is too much in the wrong order — still a text, not an essay:
run-on and self-interrupting, the way somebody types when they are talking.

**The player is held to the same bounds.** Somebody texting a person who sends
four words does not send four hundred back, so the character's replies are
clipped in a `spare` thread and looser in a `rambles` one.

A person's `texts:` governs their conflict lines too. A `spare` neighbour who
sends eighty terse characters here and a fluent paragraph in `content/conflicts`
is two people.

**Register.** Real texts between two people who have just got each other's
numbers, and the reason for the text is ordinary — a number passed on, a parcel,
a name at a door. Trimmed to game length.

- **Your turn is short.** You are texting a near-stranger back. One line.
- **Nobody front-loads.** A first bubble that identifies the sender and states
  their business and adds a detail is three messages written as one.
- **Nothing is explained for the player's benefit.** Neither of them narrates
  who they are; what you learn, you learn from what they mention in passing.
- **It does not foreshadow.** An intro that gestures at what this person will
  later want is the ask arriving early with worse manners.
- **Nothing is settled in it.** No favour is asked, granted or refused, because
  the standing is not moved by it and prose that implied otherwise would lie
  about the ledger.

## Who a line may name

**Only people the player has in their contacts** — the population, and nobody
else. A name that goes nowhere is a thread the phone implies and does not have;
a name that goes somewhere connects two threads.

Everybody else is referred to by relation or role: *my son*, *our lass*, *the
landlord*, *whoever's covering*. This costs the writing nothing, and it is
usually how somebody talks about their family to a near-stranger anyway.

The build enforces it. Any capitalised word that is neither one of the
population's people nor on its `nouns:` list is refused, so a new street or shop
is declared once in `content/environments/<population>.yaml` and then usable
everywhere. Days, months and words like *Mum* are allowed globally.

## Banned

- **Any framework word.** Stages, registers, tiers, archetypes. Boot refuses
  them.
- **Adjectives about a person.** Warm, generous, ruthless, shrewd. A population
  is legible through behaviour or it is not legible.
- **Verdicts about the place.** Nothing says a population is good, bad, hard, or
  better than another one. Both directions of §3.15's transfer have to read as
  invitations.
- **Second-person instruction.** A person's fields describe them, not what the
  player should do about them.
- **The literary register.** No cadence, no image, no sentence that would only
  survive in a novel. `lives` reads like a contacts card and `intro` reads like
  a phone.
- **Restating another field.** `note`, `lives` and `known` are three different
  facts. A `lives` that repeats the `note` is the same fact twice.

## Retired populations

An environment marked `deprecated: true` is being removed. Its characters carry
no `intro`, and writing one for them is a failed build: their threads begin the
first time somebody asks the player for something.
