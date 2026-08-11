import { BoardsHint } from '../screens/Boards'
import { ChallengeHint } from '../screens/Challenge'
import { EventsHint } from '../screens/Events'
import { Ways } from '../screens/Map'
import { PhoneHint } from '../screens/Phone'
import { PlaybookHint } from '../screens/Playbook'
import { WorkHint } from '../screens/Work'
import { useWorld } from '../world/react'

/*
 * The key bar: what a key does right now, said at the foot of the screen.
 *
 * The shell has no rail and no top bar, so what a player can reach is a key
 * rather than a menu — and the bar reading differently *is* how this world
 * tells you something is there. The row layout is this file's; what each hint
 * says is its own screen's, which is why the hints live beside the screens
 * they open and are only assembled here.
 */
export function Keybar({ open, card }: { open: boolean; card: boolean }) {
  const session = useWorld((w) => w.session)

  return (
    <div className="keybar">
      {open || card ? (
        <KeyHint keys={['Esc']}>back to the world</KeyHint>
      ) : (
        <>
          <KeyHint keys={['←', '↑', '→', '↓']}>walk</KeyHint>
          <KeyHint keys={['O']}>use what you are next to</KeyHint>
          <PlaybookHint />
          <KeyHint keys={['H']}>your body</KeyHint>
          <WorkHint />
          <EventsHint />
          <ChallengeHint />
          <PhoneHint />
          <BoardsHint />
          <KeyHint keys={['?']}>all keys</KeyHint>
        </>
      )}
      {session.status === 'in' && !open && <Ways />}
    </div>
  )
}

/** One key, and what it does right now. */
export function KeyHint({ keys, children }: { keys: string[]; children: string }) {
  return (
    <span className="keybar-item">
      {keys.map((k) => (
        <kbd className="keycap" key={k}>
          {k}
        </kbd>
      ))}
      <span className="keybar-label">{children}</span>
    </span>
  )
}
