# Environment spec

An environment is a population, not a place (§3.15): a set of people with shared
values, living somewhere internal/city already has. Everything a player reads
about them is written as **behaviour and fact**. Never as a verdict, never as an
adjective, and never with a word that names the model underneath — the loader's
`Labelled` check makes a stage name, a framework word or a developmental colour
a failed build rather than a review comment.

The game is a life simulator, so the real-world counterpart of each field is
named below and the prose imitates it, trimmed to game length. There is no
literary voice anywhere in a population file.

## What the player must be able to learn (the payload)

1. **Who these people are, by what they do.** The values of the place are
   inferable from the behaviour of its people and from nothing else.
2. **Where they stand in the player's life** — a neighbour, the man behind the
   till, the woman who decides whose card works on which floor. Position, not
   personality.
3. **Nothing about what any of it teaches.** A population never explains why
   the player's rules will fail here. Finding that out is the game.

## The fields

| field | what it is |
| ----- | ---------- |
| `key` | the internal/city district. One term for an environment — the map's. |
| `note` | who these people are, in a sentence or two. |
| `holds` | what they have in common, written as behaviour. At least three. |
| `affords` | §3.15's factor table. Mechanical; never written in prose. |
| `people` | at least four, each as below. |
| `elsewhere` | one person's gossip about another population. |

### A person

| field | real-world counterpart | what it is |
| ----- | ---------------------- | ---------- |
| `name` | a name | what they are called. |
| `note` | how you would place somebody to a third party | one line, where they sit in the world. |
| `lives` | the line in a contacts card under an address | where they live, as a neighbour would say it — a floor, a landing, a street. Never a postal address. |
| `known` | how you would answer "who's that?" | how the character comes to know them at all: the connection, and nothing about its quality. |
| `found` | — | an internal/city key inside the district. Mechanical. |
| `does` | — | one sentence of what they can be relied on to do. The load-bearing field, and the reason no person has a disposition. |

## `lives` and `known`, specifically

They exist because a phone has a contacts app, and a contacts card holds facts
about somebody rather than an assessment of them. The line between the two is
the whole reason the roster panel that used to list `does` was deleted:

- **`known` states a connection, never its temperature.** "Your neighbour on the
  landing" is a fact. "Your closest neighbour" is a conclusion the player is
  supposed to reach by living there, and "she has been good to you" is that
  conclusion said out loud.
- **`lives` states an address, never a household's condition.** "Ground floor,
  with her son" is a fact. "Ground floor, struggling since her hours were cut"
  is the writing doing the player's reading for them.
- Neither field ever previews behaviour. Anything that begins "will", "tends
  to", "always" or "never" belongs in `does` or nowhere.
- Both are one line. Two sentences is a paragraph pretending to be a caption.

The rest of a card is derived, so the prose never states it: where they turn up
comes from `found`, and what is outstanding between the two of you comes from
the standing the simulation keeps.

## Banned tells

- **Any framework word.** Colours, stages, registers, tiers, archetypes. Boot
  refuses them.
- **Adjectives about a person.** Warm, generous, ruthless, shrewd. A population
  is legible through behaviour or it is not legible.
- **Verdicts about the place.** Nothing says a population is good, bad, hard, or
  better than another one. Both directions of §3.15's transfer have to read as
  invitations.
- **Second-person instruction.** A person's fields describe them, not what the
  player should do about them.
- **The literary register.** No cadence, no image, no sentence that would only
  survive in a novel. `lives` reads like a contacts card; `does` reads like
  somebody describing a person they know.
- **Restating another field.** `note`, `lives` and `known` are three different
  facts. A `lives` that repeats the `note` is the same fact twice.

## Elsewhere

Gossip, and nothing more. Somebody who lives here mentioning somewhere they have
heard of, for their own reasons, with no idea what going there would teach
anybody. *A recommendation suggests the trip; it never explains the lesson.*
It is attributed to one of this population's own people, because a line
attributed to nobody is a prompt.
