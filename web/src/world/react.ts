/*
 * How React reads the world.
 *
 * One hook, over React's own external-store API. `useSyncExternalStore` is the
 * supported way for a component to read state React does not own — it handles
 * tearing under concurrent rendering, which a `useEffect`-plus-`setState`
 * subscription does not — and using it is what lets the store stay plain
 * TypeScript that the renderer can read at its own cadence (§10.1).
 *
 * Nothing else in the app subscribes. The canvas reads `world.getState()` in
 * its frame loop, and this is the DOM's half of the same store.
 */

import { useSyncExternalStore } from 'react'

import { world } from './store'
import type { World } from './state'

/**
 * Read a slice of the world, re-rendering when that slice changes.
 *
 * The selector must return something stable. React compares the previous
 * and next values with `Object.is` and re-renders while they differ, so a
 * selector building a fresh object or array every call — `w => ({ a: w.a })` —
 * never settles. Select a primitive or a slice the store hands back by
 * reference; the reducers in `state.ts` are written to preserve those
 * references precisely so this is cheap.
 *
 * There is no server-snapshot argument because there is no server rendering:
 * the app ships as static assets inside the binary and hydrates over its own
 * API (§2).
 */
export function useWorld<T>(select: (world: World) => T): T {
  return useSyncExternalStore(world.subscribe, () => select(world.getState()))
}
