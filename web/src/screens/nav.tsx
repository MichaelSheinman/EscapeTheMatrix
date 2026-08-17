import { createContext, useContext, useMemo, type ReactNode } from 'react'

import type { Screen } from '../App'

/*
 * Where the heads-up display can take you.
 *
 * Every screen draws the same row, so the row is context rather than a prop:
 * threading which-screen-is-open and how-to-change-it through ten components
 * that do nothing with either would put the shell inside every panel.
 *
 * These five are the readings of the character and of what is on offer. The
 * rest of the game — the map, the phone, the events, a run — is a key, and the
 * key bar says which.
 */

const WHERE: { to: Exclude<Screen, null>; says: string }[] = [
  { to: 'standing', says: 'Standing' },
  { to: 'playbook', says: 'Playbook' },
  { to: 'body', says: 'Body' },
  { to: 'readout', says: 'Your life' },
  { to: 'work', says: 'Work' },
]

type Nav = { here: Screen; go: (to: Screen) => void }

const Where = createContext<Nav>({ here: null, go: () => {} })

export function Navigation({ here, go, children }: Nav & { children: ReactNode }) {
  // Held rather than rebuilt: a fresh object every render would re-render every
  // screen behind the open one on every keystroke the shell sees.
  const nav = useMemo(() => ({ here, go }), [here, go])
  return <Where.Provider value={nav}>{children}</Where.Provider>
}

export function HudNav() {
  const { here, go } = useContext(Where)

  return (
    <nav className="hud-nav glass pill">
      {WHERE.map((w) => (
        <button
          key={w.to}
          type="button"
          className={w.to === here ? 'on' : ''}
          aria-current={w.to === here}
          onClick={() => go(w.to)}
        >
          {w.says}
        </button>
      ))}
    </nav>
  )
}
