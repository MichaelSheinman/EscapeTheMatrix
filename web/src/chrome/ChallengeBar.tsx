import { Thing } from './Things'
import { abandonChallenge } from '../world/actions'
import { useWorld } from '../world/react'
import { sending, type ChallengeRun } from '../world/state'

/*
 * Challenge mode.
 *
 * While a run is going, the shell is not the shell. The screens, the shortcuts,
 * the key card and the notifications are all gone (see `App.tsx`), and what is
 * left is this: what you are carrying, and the two things you can do about
 * being here.
 *
 * # Why the whole shell goes
 *
 * Because a challenge is somewhere you *are*, and a player who can press M for
 * the map while standing in a burning flat is a player being told, by the
 * interface, that none of this is happening. It also fixes a plainer problem:
 * with eleven keys live and no visible way out, the way out was a keyboard
 * shortcut nobody had been told about.
 *
 * Stripping the screen is what makes two buttons enough. There is nothing else
 * on it, so there is nothing to find.
 *
 * # The two controls
 *
 * **Leave** puts the challenge back to never having happened. Not "you had your
 * go" — the run is deleted, and it can be met again. That is safe precisely
 * because a run pays nothing until it finishes, so walking out has moved
 * nothing in the world.
 *
 * **The way on** is the same commitment the room offers, said as a button for
 * the moment when the room has nothing left to offer: the bag is full, and what
 * is left is to get out. It is disabled until then, and what it commits is
 * whatever the run's single remaining option is — the server decides that, and
 * this reads it.
 */
export function ChallengeBar({ onOpen }: { onOpen: () => void }) {
  const run = useWorld((w) => w.challenge?.run ?? null)
  if (!run || run.done) return null
  return <Bar run={run} onOpen={onOpen} />
}

function Bar({ run, onOpen }: { run: ChallengeRun; onOpen: () => void }) {
  const busy = useWorld(sending)
  const held = run.held ?? []
  const holds = run.holds ?? 0

  /*
   * Whether the room still has anything to give.
   *
   * A bag with room in it means the choices are things to pick up, and they are
   * picked up out in the world by walking to them. A full bag means the only
   * thing left is what happens next — so the bar stops being a readout and
   * grows a way on.
   */
  const full = holds > 0 && held.length >= holds

  return (
    <div className="challenge-bar">
      <div className="challenge-carry">
        {Array.from({ length: holds }, (_, i) => held[i] ?? null).map((what, i) => (
          <div
            className={what ? 'challenge-slot filled' : 'challenge-slot'}
            key={what?.key ?? `free-${i}`}
            title={what?.name}
          >
            {what ? <Thing item={what.key} size={26} /> : <i className="challenge-dot" />}
          </div>
        ))}
      </div>

      <div className="challenge-acts">
        <button
          className="challenge-btn"
          disabled={busy}
          onClick={() => void abandonChallenge(run.challenge)}
        >
          Leave
        </button>
        {/* The panel stays reachable, because the second half of this challenge
            is decisions about things rather than things in a room — but it is a
            button now rather than a shortcut nobody was told about. */}
        <button className="challenge-btn primary" disabled={busy} onClick={onOpen}>
          {full ? 'Get out' : 'What is happening'}
        </button>
      </div>
    </div>
  )
}
