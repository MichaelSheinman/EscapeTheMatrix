import { describe, expect, it } from 'vitest'

import { district, districts, inside, named, neighbours, wayTo, waysBy } from './city'
import type { Place } from './state'

/*
 * Reading the city the server sent.
 *
 * Everything under test here is a lookup or a grouping. What is worth asserting
 * is the line it must not cross: no route is found, no fare is added up, and
 * nothing decides whether a journey is allowed (§10.2, rule 2). A screen offers
 * what the server already said and asks for everything else.
 */

const place = (over: Partial<Place> & { key: string }): Place => ({
  name: over.key,
  note: '',
  layer: 'interior',
  in: '',
  stop: false,
  ways: [],
  ...over,
})

const city: Place[] = [
  place({
    key: 'east-side',
    name: 'East Side',
    layer: 'district',
    stop: true,
    ways: [
      { to: 'east-side-apartment', mode: 'foot', says: 'in through the door', fare: 0 },
      { to: 'east-side-shop', mode: 'foot', says: 'in through the door', fare: 0 },
      { to: 'downtown', mode: 'foot', says: 'on foot to Downtown', fare: 0 },
      { to: 'downtown', mode: 'transit', says: 'the Crosstown to Downtown', fare: 275 },
    ],
  }),
  place({ key: 'downtown', name: 'Downtown', layer: 'district', stop: true }),
  place({ key: 'east-side-shop', name: 'The corner shop', in: 'east-side' }),
  place({
    key: 'east-side-apartment',
    name: 'Your apartment',
    layer: 'home',
    in: 'east-side',
    ways: [{ to: 'east-side', mode: 'foot', says: 'out to the East Side', fare: 0 }],
  }),
]

const here = city[0]

describe('the ways out of where you are', () => {
  it('are split by how they are taken, with the fare the server put on them', () => {
    expect(waysBy(here, 'foot').map((w) => w.to)).toEqual([
      'east-side-apartment',
      'east-side-shop',
      'downtown',
    ])
    expect(waysBy(here, 'transit').map((w) => w.fare)).toEqual([275])
    // The same destination, two prices. The trade is the pair, not the place.
    expect(wayTo(here, 'downtown', 'foot')?.fare).toBe(0)
    expect(wayTo(here, 'downtown', 'transit')?.fare).toBe(275)
  })

  it('offer nothing from somewhere the network does not call', () => {
    const flat = city[3]
    expect(waysBy(flat, 'transit')).toEqual([])
    expect(wayTo(flat, 'downtown', 'transit')).toBeNull()
  })

  it('are nothing at all before the world has been hydrated', () => {
    expect(waysBy(null, 'foot')).toEqual([])
    expect(neighbours(null)).toEqual([])
  })
})

describe('what is one step away', () => {
  it('names each place once, however many ways lead to it', () => {
    expect(neighbours(here)).toEqual(['east-side-apartment', 'east-side-shop', 'downtown'])
  })
})

describe('the city as a screen reads it', () => {
  it('is the streets, and what is inside each of them', () => {
    expect(districts(city).map((p) => p.key)).toEqual(['east-side', 'downtown'])
    // Home first: it is the one interior a player has a relationship with.
    expect(inside(city, 'east-side').map((p) => p.key)).toEqual([
      'east-side-apartment',
      'east-side-shop',
    ])
    expect(inside(city, 'downtown')).toEqual([])
  })

  it('says which street a place is on, whichever layer it is', () => {
    expect(district(here)).toBe('east-side')
    expect(district(city[3])).toBe('east-side')
    expect(district(null)).toBe('')
  })

  it('names a key the city no longer has rather than dropping it', () => {
    // §17 leaves the city open, and a player standing somewhere that was
    // renamed still has to be able to read where they are.
    expect(named(city, 'atlantis').name).toBe('atlantis')
    expect(named(city, 'atlantis').ways).toEqual([])
    expect(named(city, 'downtown').name).toBe('Downtown')
  })
})
