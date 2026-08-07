// Code generated from the Go message definitions in internal/realtime.
// DO NOT EDIT — change the Go struct and run `make generate`.
//
// technical.md §8: every server→client message is {type, seq, payload}, and
// these types are generated rather than hand-maintained because two lists of
// the same protocol drift and you find out in production.

/** Payload of the `board.moved` message. */
export type BoardMoved = {
  refreshedAt: string
  players: number
  moved: ({
    board: {
      key: string
      name: string
      note: string
      reads: string
      shown: boolean
    }
    from: number
    to: number
    of: number
  })[] | null
}

/** Payload of the `career.changed` message. */
export type CareerChanged = {
  career: {
    held: {
      tier: number
      key: string
      name: string
      does: string
      costs: string
      pay: number
      hours: number
      nights: boolean
    } | null
    since: string
    paid: number
    next: {
      role: {
        tier: number
        key: string
        name: string
        does: string
        costs: string
        pay: number
        hours: number
        nights: boolean
      }
      wants: ({
        label: string
        state: string
      })[] | null
      ready: boolean
      walk: {
        place: string
        zone: string
      } | null
      interview: string
    } | null
    here: {
      role: {
        tier: number
        key: string
        name: string
        does: string
        costs: string
        pay: number
        hours: number
        nights: boolean
      }
      wants: ({
        label: string
        state: string
      })[] | null
      ready: boolean
      walk: {
        place: string
        zone: string
      } | null
      interview: string
    } | null
    ladder: ({
      tier: number
      key: string
      name: string
      does: string
      costs: string
      pay: number
      hours: number
      nights: boolean
    })[] | null
  }
  character: {
    money: number
    at: {
      key: string
      name: string
      note: string
      layer: string
      in: string
      stop: boolean
      ways: ({
        to: string
        mode: string
        says: string
        fare: number
      })[] | null
    }
    home: {
      key: string
      name: string
      note: string
      layer: string
      in: string
      stop: boolean
      ways: ({
        to: string
        mode: string
        says: string
        fare: number
      })[] | null
    }
    earningsTaken: number
    earningsLeft: number
  }
}

/** Payload of the `challenge.moved` message. */
export type ChallengeMoved = {
  challenge: {
    here: {
      challenge: string
      name: string
      place: string
      zone: string
    } | null
    run: {
      challenge: string
      name: string
      scene: string
      where: string
      taken: string
      beats: ({
        who: string
        line: string
      })[] | null
      ask: string
      choices: ({
        key: string
        text: string
      })[] | null
      stage: number
      stages: number
      done: boolean
      closing: string
      began: string
      moved: string
    } | null
  }
}

/** Payload of the `character.earned` message. */
export type CharacterEarned = {
  money: number
  earned: number
  earningsTaken: number
  earningsLeft: number
}

/** Payload of the `character.moved` message. */
export type CharacterMoved = {
  at: {
    key: string
    name: string
    note: string
    layer: string
    in: string
    stop: boolean
    ways: ({
      to: string
      mode: string
      says: string
      fare: number
    })[] | null
  }
  by: string
  money: number
}

/** Payload of the `event.answered` message. */
export type EventAnswered = {
  events: {
    offer: {
      id: number
      name: string
      kind: string
      line: string
      accept: string
      decline: string
      place: string
      zone: string
      until: string
    } | null
  }
  answer: string
  name: string
  after: string
}

/** Payload of the `event.offered` message. */
export type EventOffered = {
  events: {
    offer: {
      id: number
      name: string
      kind: string
      line: string
      accept: string
      decline: string
      place: string
      zone: string
      until: string
    } | null
  }
}

/** Payload of the `factor.stood` message. */
export type FactorStood = {
  karma: {
    karma: number
    slots: ({
      key: string
      name: string
      reads: string
      note: string
      value: number | null
      shown: boolean
    })[] | null
    where: string
    days: number
    familiarity: number
  }
}

/** Payload of the `health.bought` message. */
export type HealthBought = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
      key: string
      name: string
      unit: string
      means: string
      tier: string
      tierName: string
      lower: boolean
      decimals: number
      visible: boolean
      value: number | null
      history: number[] | null
    })[] | null
  }
  tier: {
    key: string
    name: string
    buys: string
    price: number
    owned: boolean
    forSale: boolean
  }
  spent: number
  money: number
}

/** Payload of the `health.lived` message. */
export type HealthLived = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
      key: string
      name: string
      unit: string
      means: string
      tier: string
      tierName: string
      lower: boolean
      decimals: number
      visible: boolean
      value: number | null
      history: number[] | null
    })[] | null
  }
  crossed: ({
    metric: string
    from: string
    to: string
    worse: boolean
  })[] | null
}

/** Payload of the `playbook.composed` message. */
export type PlaybookComposed = {
  playbook: {
    questions: ({
      key: string
      n: number
      ask: string
      note: string
      about: string
      conditions: ({
        key: string
        says: string
        values: ({
          key: string
          says: string
        })[] | null
      })[] | null
      actions: ({
        key: string
        says: string
        values: ({
          key: string
          says: string
        })[] | null
      })[] | null
    })[] | null
    sets: ({
      environment: string
      name: string
      rules: ({
        question: string
        when: ({
          term: string
          value: string
        } | null)[] | null
        join: string
        then: {
          term: string
          value: string
        } | null
        otherwise: {
          term: string
          value: string
        } | null
        inForce: boolean
      })[] | null
      conflicts: ({
        questions: string[] | null
        n: number[] | null
        always: boolean
        says: string
      })[] | null
      idle: ({
        question: string
        n: number
        says: string
      })[] | null
    })[] | null
  }
}

/** Payload of the `probe.recorded` message. */
export type ProbeRecorded = {
  probes: number
  label: string
  notedAt: string
}

/** Payload of the `rollover.ran` message. */
export type RolloverRan = {
  panel: {
    biologicalAge: number
    livedToday: boolean
    tiers: ({
      key: string
      name: string
      buys: string
      price: number
      owned: boolean
      forSale: boolean
    })[] | null
    metrics: ({
      key: string
      name: string
      unit: string
      means: string
      tier: string
      tierName: string
      lower: boolean
      decimals: number
      visible: boolean
      value: number | null
      history: number[] | null
    })[] | null
  }
  forecast: {
    days: number
    biologicalAge: number[] | null
    metrics: ({
      key: string
      values: number[] | null
    })[] | null
  }
  crossed: ({
    metric: string
    from: string
    to: string
    worse: boolean
  })[] | null
  days: number
  skipped: number
  character: {
    money: number
    at: {
      key: string
      name: string
      note: string
      layer: string
      in: string
      stop: boolean
      ways: ({
        to: string
        mode: string
        says: string
        fare: number
      })[] | null
    }
    home: {
      key: string
      name: string
      note: string
      layer: string
      in: string
      stop: boolean
      ways: ({
        to: string
        mode: string
        says: string
        fare: number
      })[] | null
    }
    earningsTaken: number
    earningsLeft: number
  }
  situations: {
    waiting: {
      id: number
      name: string
      line: string
      role: string
      environment: string
      why: string
      on: string
      at: string
      answers: ({
        key: string
        does: string
      })[] | null
    } | null
  }
}

/** Payload of the `routine.set` message. */
export type RoutineSet = {
  routines: {
    set: boolean
    routines: ({
      key: string
      name: string
      asks: string
      learn: string
      unlocked: boolean
      chosen: string
      options: ({
        key: string
        says: string
        costs: string
        chosen: boolean
      })[] | null
    })[] | null
  }
  forecast: {
    days: number
    biologicalAge: number[] | null
    metrics: ({
      key: string
      values: number[] | null
    })[] | null
  }
}

/** Payload of the `situation.answered` message. */
export type SituationAnswered = {
  situations: {
    waiting: {
      id: number
      name: string
      line: string
      role: string
      environment: string
      why: string
      on: string
      at: string
      answers: ({
        key: string
        does: string
      })[] | null
    } | null
  }
  name: string
  did: string
  came: string
}

/** Payload of the `social.answered` message. */
export type SocialAnswered = {
  endorsement: {
    offer: {
      id: number
      who: {
        name: string
        avatar: string
      }
      until: string
    } | null
    given: number
    cap: number
    opens: string
    received: number
    liked: ({
      name: string
      avatar: string
    })[] | null
  }
}

/** Payload of the `social.liked` message. */
export type SocialLiked = {
  by: {
    name: string
    avatar: string
  }
  endorsement: {
    offer: {
      id: number
      who: {
        name: string
        avatar: string
      }
      until: string
    } | null
    given: number
    cap: number
    opens: string
    received: number
    liked: ({
      name: string
      avatar: string
    })[] | null
  }
}

/** Payload of the `social.offered` message. */
export type SocialOffered = {
  endorsement: {
    offer: {
      id: number
      who: {
        name: string
        avatar: string
      }
      until: string
    } | null
    given: number
    cap: number
    opens: string
    received: number
    liked: ({
      name: string
      avatar: string
    })[] | null
  }
}

/** Payload of the `welcome` message. */
export type Welcome = {
  session: string
  since: number
  head: number
  gap: boolean
}

/** Every message the server can send, discriminated by `type`. */
export type ServerMessage =
  | { type: 'board.moved'; seq: number; payload: BoardMoved }
  | { type: 'career.changed'; seq: number; payload: CareerChanged }
  | { type: 'challenge.moved'; seq: number; payload: ChallengeMoved }
  | { type: 'character.earned'; seq: number; payload: CharacterEarned }
  | { type: 'character.moved'; seq: number; payload: CharacterMoved }
  | { type: 'event.answered'; seq: number; payload: EventAnswered }
  | { type: 'event.offered'; seq: number; payload: EventOffered }
  | { type: 'factor.stood'; seq: number; payload: FactorStood }
  | { type: 'health.bought'; seq: number; payload: HealthBought }
  | { type: 'health.lived'; seq: number; payload: HealthLived }
  | { type: 'playbook.composed'; seq: number; payload: PlaybookComposed }
  | { type: 'probe.recorded'; seq: number; payload: ProbeRecorded }
  | { type: 'rollover.ran'; seq: number; payload: RolloverRan }
  | { type: 'routine.set'; seq: number; payload: RoutineSet }
  | { type: 'situation.answered'; seq: number; payload: SituationAnswered }
  | { type: 'social.answered'; seq: number; payload: SocialAnswered }
  | { type: 'social.liked'; seq: number; payload: SocialLiked }
  | { type: 'social.offered'; seq: number; payload: SocialOffered }
  | { type: 'welcome'; seq: number; payload: Welcome }

/** The name of any server message. */
export type ServerMessageType = ServerMessage['type']
