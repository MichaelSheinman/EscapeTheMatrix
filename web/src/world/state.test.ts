import { describe, expect, it } from 'vitest'

import {
  applyMessage,
  framesMeasured,
  hydrated,
  initialWorld,
  listed,
  probed,
  qualityChosen,
  qualityDemoted,
  REMEMBERED,
  stood,
  travelled,
  type World,
} from './state'

/*
 * The reducers, which are the part of the client worth testing on their own.
 *
 * They are pure by construction (§10.2) — no store, no fetch, no DOM — so what
 * is asserted below is the whole behaviour: what a statement from the server
 * does to the model, and what it deliberately leaves alone.
 *
 * The exhaustiveness of the reducer table is not tested here because it cannot
 * fail at runtime: a message type the table has no entry for is a compile
 * error, which is the point of writing it as a mapped type.
 */

const somewhere = {
  key: 'east-side',
  name: 'East Side',
  note: 'where you live',
  layer: 'district' as const,
  in: '',
  stop: true,
  ways: [
    { to: 'east-side-apartment', mode: 'foot' as const, says: 'in through the door', fare: 0 },
    { to: 'riverfront', mode: 'transit' as const, says: 'the Crosstown to Riverfront', fare: 275 },
  ],
}

const panel = {
  biologicalAge: 32,
  livedToday: false,
  tiers: [{ key: 'wearable', name: 'A good watch', buys: 'HRV', price: 40000, owned: false, forSale: true }],
  metrics: [
    {
      key: 'hrv',
      name: 'Heart-rate variability',
      unit: 'ms',
      means: 'the autonomic balance sheet',
      tier: 'wearable',
      tierName: 'A good watch',
      lower: false,
      decimals: 0,
      visible: false,
      value: null,
      history: [],
    },
  ],
}

/**
 * The rules for the body, as the server states them (§3.4).
 *
 * A routine is a standing question and a sentence answering it, which is the
 * same shape the playbook composes in (§3.14) — the payload is the interface
 * language as much as the screen is.
 */
const rules = {
  set: false,
  routines: [
    {
      key: 'sleep',
      name: 'Sleep',
      asks: 'What happens when the night gets late?',
      learn: 'A stretch of short nights.',
      unlocked: true,
      chosen: 'whenever',
      options: [
        { key: 'whenever', says: 'I sleep when I run out of day.', costs: 'Nothing, until it does.', chosen: true },
        { key: 'protected', says: 'Eight and a half hours.', costs: 'Evenings.', chosen: false },
      ],
    },
  ],
}

const ahead = {
  days: 2,
  biologicalAge: [31.4, 31.1],
  metrics: [
    { key: 'hrv', values: [] },
    { key: 'sleep', values: [8.5, 8.5] },
  ],
}

/**
 * The progression board (§3.5), exactly as the server sends it.
 *
 * Two slots carry a name and a reading; three carry nothing at all. That is the
 * fixture's whole point — it is not a redacted payload with the hidden numbers
 * stripped in the test, it is what arrives.
 */
const board = {
  karma: 214.8,
  slots: [
    {
      key: 'health',
      name: 'Biological age',
      reads: 'age' as const,
      note: 'composed from every biomarker, including the ones you cannot read',
      value: 32,
      shown: true,
    },
    {
      key: 'money',
      name: 'Money',
      reads: 'money' as const,
      note: 'what you are holding, not what you have earned',
      value: 120000,
      shown: true,
    },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
    { key: '', name: '', reads: '' as const, note: 'something is being measured here', value: null, shown: false },
  ],
  where: 'east-side',
  days: 3,
  familiarity: 1,
}

/**
 * Peer endorsement (§3.5), as the snapshot carries it.
 *
 * Nobody to like right now, which is the state this surface is in most of the
 * time and is therefore the right default for a fixture: "occasional" has to
 * have a resting state, and it has to be the ordinary one.
 */
const nobodyToLike = {
  offer: null,
  given: 1,
  cap: 5,
  opens: '2026-08-06T20:00:00Z',
  received: 2,
  liked: [{ name: 'Ana', avatar: '' }],
}

// One environment, one question, and a rule with a hole in it — the state
// §3.14's loop begins in. The pools carry only what a new player has.
const composed = {
  questions: [
    {
      key: 'help',
      n: 1,
      ask: 'Somebody asks you for help with a task. When do you say yes?',
      note: 'It happens two or three times a week.',
      about: 'someone asks me for help',
      conditions: [
        { key: 'fee', says: 'they are paying me {value}', values: [{ key: 'fair', says: 'above $60' }] },
        { key: 'legal', says: 'the task is legal', values: [] },
      ],
      actions: [
        { key: 'yes', says: 'say yes', values: [] },
        { key: 'no', says: 'say no', values: [] },
      ],
    },
  ],
  sets: [
    {
      environment: 'east-side',
      name: 'East Side',
      rules: [
        {
          question: 'help',
          when: [null],
          join: 'and',
          then: { term: 'yes', value: '' },
          otherwise: null,
          inForce: false,
        },
      ],
    },
  ],
}

const snapshot = {
  character: { money: 120000, at: somewhere, home: somewhere, earningsTaken: 0, earningsLeft: 3 },
  places: [somewhere],
  health: panel,
  routines: rules,
  forecast: ahead,
  playbook: composed,
  karma: board,
  endorsement: nobodyToLike,
  probes: 7,
  database: '17.2',
  schema: 4,
}

function recorded(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return {
    type: 'probe.recorded' as const,
    seq: 1,
    payload: { probes, label, notedAt: at },
  }
}

function result(probes: number, label = '', at = '2026-08-06T12:00:00Z') {
  return { probes, label, notedAt: at, database: '17.2', schema: 4 }
}

describe('hydrating over REST', () => {
  it('takes the count and the server it is talking to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.server).toEqual({ database: '17.2', schema: 4 })
  })

  it('leaves marks alone — they are what this client was told, not what is', () => {
    const told = probed(initialWorld, result(1, 'first'))

    const world = hydrated(told, snapshot)

    expect(world.probes.count).toBe(7)
    expect(world.probes.marks).toBe(told.probes.marks)
  })

  // The renderer reads the store every frame and skips work by comparing
  // references. A reducer that cloned unconditionally would rebuild the scene
  // sixty times a second for nothing.
  it('keeps the slices it did not change', () => {
    const world = hydrated(initialWorld, snapshot)

    const again = hydrated(world, { ...snapshot })
    expect(again.probes).toBe(world.probes)
    expect(again.server).toBe(world.server)
  })
})

describe('a probe this tab sent', () => {
  it('takes the count from the response and remembers who did it', () => {
    const world = probed(initialWorld, result(3, 'mine'))

    expect(world.probes.count).toBe(3)
    expect(world.probes.marks).toHaveLength(1)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'mine', by: 'you' })
  })
})

describe('a probe the socket brought', () => {
  it('is recorded as having happened somewhere else', () => {
    const world = applyMessage(initialWorld, recorded(2, 'theirs'))

    expect(world.probes.count).toBe(2)
    expect(world.probes.marks[0]).toMatchObject({ n: 1, label: 'theirs', by: 'elsewhere' })
  })

  // §8, rule 3 in the model: a command's response and a socket message are two
  // ways of being told, and both land in one place.
  it('numbers marks in the order this client learned of them', () => {
    let world: World = probed(initialWorld, result(1, 'mine'))
    world = applyMessage(world, recorded(2, 'theirs'))
    world = probed(world, result(3, 'mine again'))

    expect(world.probes.marks.map((m) => [m.n, m.by])).toEqual([
      [1, 'you'],
      [2, 'elsewhere'],
      [3, 'you'],
    ])
  })

  // The count is the whole table, so it can move by more than the marks do —
  // other players probe too, and this client is not told about theirs.
  it('takes whatever count the server stated, jump or not', () => {
    const world = applyMessage(probed(initialWorld, result(1)), recorded(40))

    expect(world.probes.count).toBe(40)
    expect(world.probes.marks).toHaveLength(2)
  })
})

describe('the bounded memory of marks', () => {
  it('keeps the most recent and drops the oldest', () => {
    let world = initialWorld
    for (let i = 1; i <= REMEMBERED + 5; i++) world = probed(world, result(i, `probe-${i}`))

    const marks = world.probes.marks
    expect(marks).toHaveLength(REMEMBERED)
    // Numbering counts every probe this client was told about, so the ones that
    // survived keep the numbers they were given — the renderer places marks by
    // that number, and a renumbering would move every mark on screen.
    expect(marks[0].n).toBe(6)
    expect(marks[marks.length - 1].n).toBe(REMEMBERED + 5)
    expect(world.probes.seen).toBe(REMEMBERED + 5)
  })
})

describe('the character', () => {
  const riverfront = {
    key: 'riverfront',
    name: 'Riverfront',
    note: 'where the money lives',
    layer: 'district',
    in: '',
    stop: true,
    ways: [{ to: 'downtown', mode: 'foot', says: 'on foot to Downtown', fare: 0 }],
  }
  const hydratedWorld = hydrated(initialWorld, snapshot)

  it('is hydrated with somewhere to stand, somewhere to live, and money', () => {
    expect(hydratedWorld.character).toEqual(snapshot.character)
    expect(hydratedWorld.places).toEqual(snapshot.places)
  })

  it('walks when another tab says it walked', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 2,
      payload: { at: riverfront, by: 'foot', money: snapshot.character.money },
    })

    expect(world.character?.at).toEqual(riverfront)
    // Where they are, and what the fare left them with — both stated by the
    // server, because a ride moves money and a walk does not (§3.12).
    expect(world.character?.money).toBe(snapshot.character.money)
  })

  it('takes the fare from the message when another tab took the train', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 3,
      payload: { at: riverfront, by: 'transit', money: 119725 },
    })

    expect(world.character?.money).toBe(119725)
    // The journey belongs to the tab that made it. This one only learns where
    // the player ended up.
    expect(world.trip).toBeNull()
  })

  // Go's nil slice is null on the wire. It becomes a list here and nowhere else.
  it('reads a place with no ways out as a place with no ways out', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.moved',
      seq: 4,
      payload: { at: { ...riverfront, ways: null }, by: 'foot', money: 1 },
    })

    expect(world.character?.at.ways).toEqual([])
  })

  // Every number the message carries was stated by the server, including how
  // many paid actions are left — the client counts nothing (§10.2, rule 2).
  it('takes both halves of the cap from the message rather than counting', () => {
    const world = applyMessage(hydratedWorld, {
      type: 'character.earned',
      seq: 3,
      payload: { money: 124500, earned: 4500, earningsTaken: 1, earningsLeft: 2 },
    })

    expect(world.character?.money).toBe(124500)
    expect(world.character?.earningsTaken).toBe(1)
    expect(world.character?.earningsLeft).toBe(2)
  })

  // A journey is stored with the character it moved (§3.12): a fare paid and a
  // place arrived at are one event.
  it('stores a journey beside the character it moved', () => {
    const world = travelled(hydratedWorld, { ...snapshot.character, money: 119725 }, {
      by: 'transit',
      steps: [{ to: 'riverfront', mode: 'transit', says: 'the Crosstown to Riverfront' }],
      fare: 275,
      chances: 0,
    })

    expect(world.character?.money).toBe(119725)
    expect(world.trip?.fare).toBe(275)
    // The whole of what the fare bought: nothing happened on the way.
    expect(world.trip?.chances).toBe(0)
  })

  it('records a walk as free, and as somewhere things could have happened', () => {
    const world = travelled(hydratedWorld, snapshot.character, {
      by: 'foot',
      steps: [
        { to: 'downtown', mode: 'foot', says: 'on foot to Downtown' },
        { to: 'riverfront', mode: 'foot', says: 'on foot to Riverfront' },
      ],
      fare: 0,
      chances: 2,
    })

    expect(world.trip?.fare).toBe(0)
    expect(world.trip?.chances).toBe(2)
  })

  // A message about a character this client has not been told about yet can
  // only be applied to nothing. It is dropped rather than inventing one.
  it('ignores a change that arrives before there is anybody to change', () => {
    const world = applyMessage(initialWorld, {
      type: 'character.moved',
      seq: 1,
      payload: { at: riverfront, by: 'foot', money: 0 },
    })

    expect(world).toBe(initialWorld)
  })
})

/*
 * Where in the painting the player is standing (§10.3).
 *
 * The one slice no server statement touches. It is written by the renderer,
 * from authored geometry it loaded itself, and it carries no game rule — which
 * rectangle a walk ended inside is a fact about a Tiled file. What *happens*
 * in a zone is the event system's (#21, #22), and these keys are the vocabulary
 * it will be asked in.
 */
describe('standing somewhere', () => {
  const stoop = {
    level: 'east-side-street',
    zones: [
      { key: 'home-door', name: 'Your stoop' },
      { key: 'on-foot', name: 'On foot, the East Side' },
    ],
    portals: [{ key: 'east-side-apartment', name: 'Your apartment', to: 'east-side-apartment' }],
  }

  it('is nowhere until the world has drawn a location', () => {
    expect(initialWorld.standing).toBeNull()
  })

  it('records the zones and the ways out, most specific zone first', () => {
    const world = stood(hydrated(initialWorld, snapshot), stoop)

    expect(world.standing?.zones[0].key).toBe('home-door')
    expect(world.standing?.portals[0].to).toBe('east-side-apartment')
  })

  it('says nothing about the character, who has not moved district', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = stood(before, stoop)

    expect(world.character).toBe(before.character)
    expect(world.health).toBe(before.health)
  })

  // Walking into a district nobody has painted, or signing out: both leave the
  // world with no location on it, and nowhere is the honest answer.
  it('goes back to nowhere when there is no location on screen', () => {
    const world = stood(hydrated(initialWorld, snapshot), stoop)

    expect(stood(world, null).standing).toBeNull()
  })
})

/*
 * The rules for the body (§3.4), which arrive by both routes: hydrated with the
 * world at boot, and pushed when another tab states them.
 */
/*
 * The effects tier (§10.4). The one slice of the model that is a fact about the
 * device rather than about the world, and the reducers are where the two rules
 * that keep it honest live: asking clears a demotion, and a demotion only ever
 * goes downwards.
 */
describe('how the world is drawn', () => {
  it('starts on the floor, unchosen, and unmeasured', () => {
    expect(initialWorld.quality).toEqual({
      tier: 'low',
      chosen: false,
      demoted: false,
      fps: 0,
    })
  })

  it('takes the tier that was asked for, and keeps what was measured', () => {
    const measured = framesMeasured(initialWorld, 58)
    const world = qualityChosen(measured, 'high', true)

    expect(world.quality.tier).toBe('high')
    expect(world.quality.chosen).toBe(true)
    expect(world.quality.fps).toBe(58)
  })

  it('drops back to the floor when the machine could not hold it', () => {
    const world = qualityDemoted(qualityChosen(initialWorld, 'high', true))

    expect(world.quality.tier).toBe('low')
    expect(world.quality.demoted).toBe(true)
    // Not chosen: the machine decided this, not the player.
    expect(world.quality.chosen).toBe(false)
  })

  it('lets a machine be told twice', () => {
    const dropped = qualityDemoted(qualityChosen(initialWorld, 'high', true))
    const again = qualityChosen(dropped, 'high', true)

    // A button that stopped working after one refusal would be worse than a
    // slow frame.
    expect(again.quality).toEqual({ tier: 'high', chosen: true, demoted: false, fps: 0 })
  })

  it('leaves the world alone when the frame rate has not moved', () => {
    const world = framesMeasured(initialWorld, 60)
    // The renderer reports this from its own loop, and a new object every frame
    // would be a React render every frame.
    expect(framesMeasured(world, 60)).toBe(world)
    expect(framesMeasured(world, 59)).not.toBe(world)
  })

  it('says nothing about the world it is drawing', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = qualityChosen(before, 'high', true)

    expect(world.character).toBe(before.character)
    expect(world.places).toBe(before.places)
    expect(world.health).toBe(before.health)
  })
})

describe('the routines', () => {
  it('are hydrated with the fortnight they lead to', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.routines?.set).toBe(false)
    expect(world.routines?.routines[0].asks).toBe('What happens when the night gets late?')
    expect(world.forecast?.days).toBe(2)
    expect(world.forecast?.biologicalAge).toEqual([31.4, 31.1])
  })

  // A projection is a reading of a body, so it obeys the panel's rule: what is
  // withheld is the reading, not the idea that the measurement exists (§3.16).
  it('carry no projected values for a metric with no instrument behind it', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.forecast?.metrics.find((m) => m.key === 'hrv')?.values).toEqual([])
    expect(world.forecast?.metrics.find((m) => m.key === 'sleep')?.values).toEqual([8.5, 8.5])
  })

  it('are restated whole when another tab sets them', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'routine.set',
      seq: 4,
      payload: {
        routines: {
          set: true,
          routines: [{ ...rules.routines[0], chosen: 'protected', options: null }],
        },
        forecast: { days: 2, biologicalAge: [30.9, 30.2], metrics: null },
      },
    })

    expect(world.routines?.set).toBe(true)
    expect(world.routines?.routines[0].chosen).toBe('protected')
    // An empty Go slice is null on the wire, and this is the one place that is
    // turned back into a list — nothing downstream should have to think about it.
    expect(world.routines?.routines[0].options).toEqual([])
    expect(world.forecast?.metrics).toEqual([])
    expect(world.forecast?.biologicalAge).toEqual([30.9, 30.2])

    // The rules changing says nothing about the body. The panel moves when a
    // day is lived, which is a different message.
    expect(world.health).toBe(before.health)
  })
})

/*
 * The playbook (§3.14): rules for judgement, set per environment (§3.15).
 *
 * Two things are worth asserting in the model rather than on the server. A blank
 * is a real state of a rule and survives the trip as one — nothing here fills it
 * in — and the vocabulary arrives already holding only what this player has,
 * so there is no branch in this client that could decide otherwise.
 */
describe('the playbook', () => {
  it('is hydrated as a rule set per environment, none of it in force', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.playbook?.sets).toHaveLength(1)
    expect(world.playbook?.sets[0].environment).toBe('east-side')
    expect(world.playbook?.sets[0].rules[0].inForce).toBe(false)
    // The hole the player has yet to fill, kept as a hole.
    expect(world.playbook?.sets[0].rules[0].when).toEqual([null])
    expect(world.playbook?.sets[0].rules[0].otherwise).toBeNull()
  })

  it('carries only the words this player has, and no count of the ones they have not', () => {
    const world = hydrated(initialWorld, snapshot)
    const help = world.playbook?.questions[0]

    expect(help?.conditions.map((t) => t.key)).toEqual(['fee', 'legal'])
    expect(help?.conditions[1].values).toEqual([])
    expect(JSON.stringify(world.playbook)).not.toContain('locked')
  })

  it('is restated whole when a rule is composed in another tab', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, {
      type: 'playbook.composed',
      seq: 9,
      payload: {
        playbook: {
          questions: [{ ...composed.questions[0], conditions: null, actions: null }],
          sets: [
            {
              environment: 'east-side',
              name: 'East Side',
              rules: [
                {
                  question: 'help',
                  when: [{ term: 'fee', value: 'fair' }],
                  join: 'and',
                  then: { term: 'yes', value: '' },
                  otherwise: { term: 'no', value: '' },
                  inForce: true,
                },
              ],
            },
          ],
        },
      },
    })

    expect(world.playbook?.sets[0].rules[0].inForce).toBe(true)
    expect(world.playbook?.sets[0].rules[0].when[0]?.value).toBe('fair')
    // An empty Go slice is null on the wire, and this is the one place that is
    // turned back into a list.
    expect(world.playbook?.questions[0].conditions).toEqual([])

    // Composing a rule says nothing about the body or the balance: it is a
    // statement about what the character will do, not about what has happened.
    expect(world.health).toBe(before.health)
    expect(world.character).toBe(before.character)
  })
})

/*
 * The daily rollover (§3.3): the world moving with nobody asking.
 *
 * It is the only message no command of this client's caused, which makes it the
 * clearest case for the socket existing at all — a tab left open overnight
 * would otherwise still be showing yesterday's body.
 */
describe('the world rolling over', () => {
  const rolled = (crossed: { metric: string; from: string; to: string; worse: boolean }[] | null) => ({
    type: 'rollover.ran' as const,
    seq: 9,
    payload: {
      panel: { ...panel, biologicalAge: 33.6, livedToday: false },
      forecast: { days: 2, biologicalAge: [33.4, 33.2], metrics: null },
      crossed,
      days: 4,
      skipped: 0,
    },
  })

  it('restates the panel and the fortnight it now leads to', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(before, rolled(null))

    expect(world.health?.biologicalAge).toBe(33.6)
    expect(world.forecast?.biologicalAge).toEqual([33.4, 33.2])
    // A rollover applies the rules; it does not change them.
    expect(world.routines).toBe(before.routines)
  })

  it('says what the last day applied crossed', () => {
    const before = hydrated(initialWorld, snapshot)
    const world = applyMessage(
      before,
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )

    expect(world.crossings).toEqual([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }])
  })

  it('leaves the last crossings alone when a quiet day crossed nothing', () => {
    const before = applyMessage(
      hydrated(initialWorld, snapshot),
      rolled([{ metric: 'hrv', from: 'fair', to: 'poor', worse: true }]),
    )
    const world = applyMessage(before, rolled(null))

    expect(world.crossings).toBe(before.crossings)
  })
})

/*
 * Progression (§3.5): the composite, and the factors nobody is shown.
 *
 * There is no level in this game, and there is no level bar in this model. What
 * the reducer stores is a number the server composed on read, and a board whose
 * hidden slots are empty on arrival rather than emptied here.
 */
describe('the progression board', () => {
  const moved = (karma: number) => ({
    type: 'factor.stood' as const,
    seq: 11,
    payload: { karma: { ...board, karma, slots: board.slots } },
  })

  it('arrives with the world and is replaced whenever the server recomposes it', () => {
    const before = hydrated(initialWorld, snapshot)
    expect(before.karma?.karma).toBe(214.8)

    const world = applyMessage(before, moved(198.2))
    expect(world.karma?.karma).toBe(198.2)
  })

  // §3.5: the composite can fall, which is what keeps it from being a level bar
  // wearing a different name. Nothing in this client clamps or ratchets it.
  it('lets the composite fall', () => {
    const before = applyMessage(hydrated(initialWorld, snapshot), moved(300))
    const world = applyMessage(before, moved(12))

    expect(world.karma?.karma).toBe(12)
  })

  // The strongest assertion in the file: there is nothing on the wire to reveal.
  it('carries three slots with no name, no key and no reading', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), moved(214.8))
    const hidden = world.karma?.slots.filter((s) => !s.shown) ?? []

    expect(hidden).toHaveLength(3)
    for (const slot of hidden) {
      expect(slot.key).toBe('')
      expect(slot.name).toBe('')
      expect(slot.value).toBeNull()
    }
  })

  it('keeps the two that are shown, as the readings the design surfaces them as', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), moved(214.8))
    const shown = world.karma?.slots.filter((s) => s.shown) ?? []

    expect(shown.map((s) => [s.key, s.reads])).toEqual([
      ['health', 'age'],
      ['money', 'money'],
    ])
  })

  // §3.15: the environment wears out, and the player has to be able to feel it.
  it('says how worn the place they are standing in is', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), {
      type: 'factor.stood' as const,
      seq: 12,
      payload: { karma: { ...board, days: 40, familiarity: 0.21 } },
    })

    expect(world.karma?.where).toBe('east-side')
    expect(world.karma?.familiarity).toBeCloseTo(0.21)
  })

  // Go sends an empty slice as null. Nothing downstream should have to know.
  it('reads a null board back as a list', () => {
    const world = applyMessage(hydrated(initialWorld, snapshot), {
      type: 'factor.stood' as const,
      seq: 13,
      payload: { karma: { ...board, slots: null } },
    })

    expect(world.karma?.slots).toEqual([])
  })
})

/*
 * The leaderboards (§3.5).
 *
 * The distributed board is the one slice of this model the boot snapshot does
 * not carry — it is a read-only paginated list of other people (§10.2), fetched
 * when somebody looks at it. What the socket does is move the player's own place
 * on it, because a rank changes when *somebody else* plays.
 */
describe('the leaderboards', () => {
  const sealed = (n: number) => ({
    key: `slot-${n}`,
    name: '',
    note: 'a real ranking of something nobody has told you the name of',
    reads: '' as const,
    shown: false,
  })

  const karmaBoard = {
    key: 'karma',
    name: 'Karma',
    note: 'everything at once',
    reads: 'karma' as const,
    shown: true,
  }

  const index = {
    refreshedAt: '2026-08-07T09:00:00Z',
    players: 3,
    boards: [
      { board: karmaBoard, rank: 2, of: 3, value: 214.8 },
      { board: sealed(3), rank: 3, of: 3, value: null },
    ],
  }

  const rebuild = (moved: unknown[], refreshedAt = '2026-08-07T09:05:00Z') => ({
    type: 'board.moved' as const,
    seq: 21,
    payload: { refreshedAt, players: 4, moved } as never,
  })

  it('is not part of the boot snapshot', () => {
    expect(hydrated(initialWorld, snapshot).boards).toBeNull()
  })

  it('stores the index the boards screen fetched', () => {
    const world = listed(initialWorld, index)
    expect(world.boards?.players).toBe(3)
    expect(world.boards?.boards.map((p) => p.board.key)).toEqual(['karma', 'slot-3'])
  })

  it('moves the ranks a rebuild says moved, and leaves the others alone', () => {
    const world = applyMessage(
      listed(initialWorld, index),
      rebuild([{ board: karmaBoard, from: 2, to: 1, of: 4 }]),
    )

    expect(world.boards?.boards[0].rank).toBe(1)
    expect(world.boards?.boards[0].of).toBe(4)
    // Untouched, because the message said nothing about it.
    expect(world.boards?.boards[1].rank).toBe(3)
    expect(world.boards?.boards[1].of).toBe(3)
    // And the whole index now dates from the rebuild that moved them.
    expect(world.boards?.refreshedAt).toBe('2026-08-07T09:05:00Z')
    expect(world.boards?.players).toBe(4)
  })

  /*
   * §3.5's claim, stated as a test: a ranking discloses nothing.
   *
   * A sealed board can move a player up the table and still carry no name, no
   * `reads` and no value — there is nothing in the message for this client to
   * decide to withhold.
   */
  it('moves a player on a board that has no name and no number', () => {
    const world = applyMessage(
      listed(initialWorld, index),
      rebuild([{ board: sealed(3), from: 3, to: 1, of: 4 }]),
    )

    const moved = world.boards?.boards[1]
    expect(moved?.rank).toBe(1)
    expect(moved?.board.name).toBe('')
    expect(moved?.board.reads).toBe('')
    expect(moved?.value).toBeNull()
  })

  // The socket keeps this slice current; it does not create it. A rank that
  // moved before anybody opened the screen is not worth reconstructing from a
  // partial message, because opening it fetches the whole thing.
  it('ignores a rebuild when the boards have never been fetched', () => {
    const world = applyMessage(initialWorld, rebuild([{ board: karmaBoard, from: 0, to: 1, of: 4 }]))
    expect(world.boards).toBeNull()
    expect(world).toBe(initialWorld)
  })

  // Go sends an empty slice as null.
  it('reads a null list of movements back as none', () => {
    const world = applyMessage(listed(initialWorld, index), rebuild(null as never))
    expect(world.boards?.boards[0].rank).toBe(2)
    expect(world.boards?.refreshedAt).toBe('2026-08-07T09:05:00Z')
  })
})

/*
 * Peer endorsement (§3.5).
 *
 * The one slice of this model that another human being moves. Three messages
 * carry it — the game named somebody, this player answered somewhere else, and
 * somebody said they like them — and all three restate the whole surface, which
 * is the property these tests are really about: there is no counter incremented
 * here, so a client cannot end up with a number the server never said.
 */
describe('peer endorsement', () => {
  const anOffer = {
    offer: { id: 41, who: { name: 'Ilya', avatar: '' }, until: '2026-08-06T20:00:00Z' },
    given: 1,
    cap: 5,
    opens: '',
    received: 2,
    liked: [{ name: 'Ana', avatar: '' }],
  }

  const message = (type: 'social.offered' | 'social.answered', endorsement: unknown) =>
    ({ type, seq: 20, payload: { endorsement } }) as never

  it('arrives with the world, resting, and says when the next one comes', () => {
    const world = hydrated(initialWorld, snapshot)

    expect(world.endorsement?.offer).toBeNull()
    expect(world.endorsement?.opens).toBe('2026-08-06T20:00:00Z')
    expect(world.endorsement?.given).toBe(1)
    expect(world.endorsement?.cap).toBe(5)
  })

  it('takes an offer the game made and drops it again when it is answered', () => {
    const offered = applyMessage(hydrated(initialWorld, snapshot), message('social.offered', anOffer))
    expect(offered.endorsement?.offer?.who.name).toBe('Ilya')
    // Nothing but the offer's own id addresses the other player.
    expect(offered.endorsement?.offer?.id).toBe(41)

    const answered = applyMessage(
      offered,
      message('social.answered', { ...anOffer, offer: null, given: 2, opens: '2026-08-07T09:00:00Z' }),
    )
    expect(answered.endorsement?.offer).toBeNull()
    expect(answered.endorsement?.given).toBe(2)
  })

  /*
   * The message the whole feature exists for.
   *
   * Nobody asked for it, nobody was present, and no command's response could
   * have carried it — §8's "another player's action affecting you", arriving for
   * the first time in this game. The count comes off the wire whole: this client
   * does not add one to what it had.
   */
  it('stores what the server says when somebody else says they like you', () => {
    const before = applyMessage(hydrated(initialWorld, snapshot), message('social.offered', anOffer))

    const world = applyMessage(before, {
      type: 'social.liked' as const,
      seq: 21,
      payload: {
        by: { name: 'Ilya', avatar: '' },
        endorsement: {
          ...anOffer,
          received: 3,
          liked: [
            { name: 'Ilya', avatar: '' },
            { name: 'Ana', avatar: '' },
          ],
        },
      },
    })

    expect(world.endorsement?.received).toBe(3)
    expect(world.endorsement?.liked.map((p) => p.name)).toEqual(['Ilya', 'Ana'])
    // The offer this player is holding is untouched by somebody else's action.
    expect(world.endorsement?.offer?.id).toBe(41)
  })

  // Go sends an empty slice as null. Nothing downstream should have to know.
  it('reads a null list of admirers back as a list', () => {
    const world = applyMessage(
      hydrated(initialWorld, snapshot),
      message('social.offered', { ...anOffer, liked: null, received: 0 }),
    )
    expect(world.endorsement?.liked).toEqual([])
  })
})
