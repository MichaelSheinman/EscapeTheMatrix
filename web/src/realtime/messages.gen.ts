// Code generated from the Go message definitions in internal/realtime.
// DO NOT EDIT — change the Go struct and run `make generate`.
//
// technical.md §8: every server→client message is {type, seq, payload}, and
// these types are generated rather than hand-maintained because two lists of
// the same protocol drift and you find out in production.
//
// A list or record here is never null. internal/wire encodes everything the
// server sends and replaces a nil collection with an empty one, so `?? []`
// on a field of these types is guarding against something that cannot arrive.

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
  })[]
}

/** Payload of the `career.changed` message. */
export type CareerChanged = {
  career: {
    held: {
      key: string
      name: string
      posting: {
        intro: string
        duties: string[]
        qualifications: string[]
      }
      pay: number
      hours: number
      nights: boolean
      shift: string
    } | null
    since: string
    paid: number
    here: {
      role: {
        key: string
        name: string
        posting: {
          intro: string
          duties: string[]
          qualifications: string[]
        }
        pay: number
        hours: number
        nights: boolean
        shift: string
      }
      wants: ({
        label: string
        state: string
      })[]
      ready: boolean
      walk: {
        place: string
        zone: string
      } | null
      take: boolean
      interview: string
      experience: ({
        needs: ({
          role: string
          name: string
          days: number
        })[]
        short: ({
          role: string
          name: string
          days: number
        })[]
      })[]
      worked: boolean
    } | null
    board: ({
      role: {
        key: string
        name: string
        posting: {
          intro: string
          duties: string[]
          qualifications: string[]
        }
        pay: number
        hours: number
        nights: boolean
        shift: string
      }
      wants: ({
        label: string
        state: string
      })[]
      ready: boolean
      walk: {
        place: string
        zone: string
      } | null
      take: boolean
      interview: string
      experience: ({
        needs: ({
          role: string
          name: string
          days: number
        })[]
        short: ({
          role: string
          name: string
          days: number
        })[]
      })[]
      worked: boolean
    })[]
    work: ({
      key: string
      name: string
      posting: {
        intro: string
        duties: string[]
        qualifications: string[]
      }
      pay: number
      hours: number
      nights: boolean
      shift: string
    })[]
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
      })[]
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
      })[]
    }
    timezone: string
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
      door: string
      enter: string
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
      })[]
      ask: string
      choices: ({
        key: string
        text: string
      })[]
      karma?: number
      earned?: number
      stage: number
      stages: number
      mechanic: string
      canvas?: string
      held?: ({
        key: string
        name: string
      })[]
      holds?: number
      done: boolean
      closing: string
      began: string
      moved: string
    } | null
  }
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
    })[]
  }
  by: string
  money: number
}

/** Payload of the `conflict.answered` message. */
export type ConflictAnswered = {
  conflicts: {
    waiting: {
      id: number
      name: string
      line: string
      who: string
      where: string
      environment: string
      why: string
      standing: string[]
      on: string
      at: string
      answers: ({
        key: string
        does: string
      })[]
    } | null
    among: {
      environment: string
      name: string
      note: string
      holds: string[]
      people: ({
        key: string
        name: string
        note: string
        lives: string
        known: string
        where: string
        standing: string[]
        met: number
        good: number
        poor: number
      })[]
      questions: ({
        question: string
        ask: string
        met: number
        covered: number
        good: number
        poor: number
      })[]
      since: string
    } | null
  }
  name: string
  who: string
  did: string
  came: string
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
      because: string
      about: {
        name: string
        says: string
        did: string
        came: string
        who: string
        environment: string
        person: string
        message: number
        rules: ({
          environment: string
          question: string
          says: string
          did: string
          state: string
        })[]
      } | null
      place: string
      zone: string
      glyph?: string
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
      because: string
      about: {
        name: string
        says: string
        did: string
        came: string
        who: string
        environment: string
        person: string
        message: number
        rules: ({
          environment: string
          question: string
          says: string
          did: string
          state: string
        })[]
      } | null
      place: string
      zone: string
      glyph?: string
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
    })[]
    curve: ({
      day: string
      karma: number
    })[]
    where: string
    among: string
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
    })[]
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
      history: number[]
    })[]
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

/** Payload of the `playbook.composed` message. */
export type PlaybookComposed = {
  playbook: {
    questions: ({
      key: string
      ask: string
      note: string
      about: string
      conditions: ({
        key: string
        says: string
        values: ({
          key: string
          says: string
        })[]
      })[]
      actions: ({
        key: string
        says: string
        values: ({
          key: string
          says: string
        })[]
      })[]
    })[]
    sets: ({
      environment: string
      name: string
      rules: ({
        question: string
        when: ({
          term: string
          value: string
        } | null)[]
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
      })[]
      conflicts: ({
        questions: string[]
        always: boolean
        says: string
      })[]
    })[]
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
    })[]
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
      history: number[]
    })[]
  }
  forecast: {
    days: number
    biologicalAge: number[]
    metrics: ({
      key: string
      values: number[]
    })[]
  }
  crossed: ({
    metric: string
    from: string
    to: string
    worse: boolean
  })[]
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
      })[]
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
      })[]
    }
    timezone: string
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
      })[]
    } | null
  }
  conflicts: {
    waiting: {
      id: number
      name: string
      line: string
      who: string
      where: string
      environment: string
      why: string
      standing: string[]
      on: string
      at: string
      answers: ({
        key: string
        does: string
      })[]
    } | null
    among: {
      environment: string
      name: string
      note: string
      holds: string[]
      people: ({
        key: string
        name: string
        note: string
        lives: string
        known: string
        where: string
        standing: string[]
        met: number
        good: number
        poor: number
      })[]
      questions: ({
        question: string
        ask: string
        met: number
        covered: number
        good: number
        poor: number
      })[]
      since: string
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
      chosen: string
      options: ({
        key: string
        says: string
        costs: string
        chosen: boolean
      })[]
    })[]
  }
  forecast: {
    days: number
    biologicalAge: number[]
    metrics: ({
      key: string
      values: number[]
    })[]
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
      })[]
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
    })[]
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
    })[]
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
    })[]
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
  | { type: 'character.moved'; seq: number; payload: CharacterMoved }
  | { type: 'conflict.answered'; seq: number; payload: ConflictAnswered }
  | { type: 'event.answered'; seq: number; payload: EventAnswered }
  | { type: 'event.offered'; seq: number; payload: EventOffered }
  | { type: 'factor.stood'; seq: number; payload: FactorStood }
  | { type: 'health.bought'; seq: number; payload: HealthBought }
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
