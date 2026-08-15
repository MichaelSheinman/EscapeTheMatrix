# Situation spec

A situation is what landed on your desk while you were elsewhere: something at
the job you hold that ran without you and now needs an answer. It renders in
the Work overlay as one panel — a title, the account, and a row of buttons —
so it reads like a real escalation reaching a manager, not like a story.

The game is a life simulator: imitate the real thing (a report from the person
involved, a summary of what happened, what is being asked of you now), trimmed
to game length. No literary voice, no atmosphere, no oblique significance.

## What the player must be able to learn (the payload)

1. **What happened** — concretely: who, what, when, and what it has already
   cost or is about to.
2. **What is being asked of you now** — a situation with no live decision in
   it is a report, not a situation.
3. **Enough to weigh every button.** Each answer is a stance the playbook can
   also take automatically, so the account must contain the facts those
   stances turn on. The `facts` block states them mechanically; `line` must
   state the same things in words.

## Format

```yaml
roles:                          # which jobs produce this; each must be `delegated`
  - operations
name: Someone quits             # plain factual label, ≤ 50 chars
line: >-                        # the account, 2–4 sentences, ≤ 320 chars
  ...
answers:
  - stance: open
    did: ...                    # the button label: an action, ≤ 60 chars
    came: >-                    # what observably followed, 1–3 sentences, ≤ 260 chars
      ...
```

The lengths above are targets for new writing, not loader checks — unlike
`content/jobs` and `content/conflicts`, internal/situation enforces no budget
today. Treat a field that runs well past its target as a signal to look for the
sentence that carries no fact, rather than as a build failure.

## People

Situations have no authored roster (unlike conflicts, where `{who}` is drawn
from `content/environments`). The people in a situation are your own staff,
peers and superiors, so **naming them in the prose is correct** — a report
that says "a supervisor" where a real one says "Priya" is worse writing, not
safer. Keep a name consistent within a file; do not reuse names across files
for different people.

## Never written

- The literary register: atmosphere, mood, significance, aphorism endings.
- A verdict on the player's answer. `came` states what happened next, and the
  outcome band (good/mixed/poor) is mechanical and never spoken.
- Anything the deletion test kills — if a sentence carries no fact about what
  happened, what it cost, or what is being asked, cut it.
- Second-guessing the player in advance ("what a person does with ten minutes
  outside a room is a thing the room finds out about later").

## Worked example — good

```yaml
name: Someone quits
line: >-
  Dez handed in his notice on the floor at four, loud enough that the picking
  line stopped. He has been here eleven years. He says nobody has fixed the
  Saturday rota since March and he is done asking.
answers:
  - stance: open
    did: Ask him what has actually been going on
    came: >-
      He talks for twenty minutes and most of it is not the rota. Two of the
      night-shift problems he lists are ones you had not heard of. He works
      his notice and trains his replacement.
```

Concrete, states the cost, names the person, and every button is weighable
from it.

## Worked example — bad (the old register; never write this)

> The fourth floor is carpet. That is the first thing about it and for a while
> it is the only thing.

Atmosphere with no facts in it: nothing happened, nobody is asking anything,
and no button is easier to choose for having read it.
