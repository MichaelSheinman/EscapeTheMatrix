import { useEffect, useState } from 'react'

import { useWorld } from '../world/react'

/*
 * How long is left on what the world is holding out.
 *
 * §4's offer window, said out loud. Most events stand for forty-five minutes
 * and nobody needs a clock for that; the fire stands for five, and a moment
 * that will be gone before you have finished deciding has to say so — otherwise
 * "it lapsed" is indistinguishable from "it broke".
 *
 * It ticks in the component and nowhere else. The world model holds `until`,
 * which is a fact the server stated; how many seconds that is from now is a
 * fact about this second, and putting it in the store would be sixty writes a
 * minute into a model the renderer reads every frame (§10.1).
 *
 * When it runs out this simply stops showing. It does not tell the server —
 * lapsing is the server's own bookkeeping, and a client that announced the
 * deadline had passed would be a second clock to disagree with the first.
 */
export function Deadline() {
  const offer = useWorld((w) => w.events?.offer ?? null)
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!offer) return
    const tick = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(tick)
  }, [offer?.id])

  if (!offer?.until) return null
  const left = Math.max(0, new Date(offer.until).getTime() - now)
  if (left <= 0) return null

  // Only for the ones that are actually short. A countdown on a forty-five
  // minute offer is a timer on a thing that is not timed, and it would teach
  // the player to read every offer as a stopwatch.
  if (left > 10 * 60 * 1000) return null

  const seconds = Math.ceil(left / 1000)
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60

  return (
    <div className={left <= 60_000 ? 'deadline urgent' : 'deadline'}>
      <span className="deadline-what">{offer.name}</span>
      <span className="deadline-left">
        {mins}:{String(secs).padStart(2, '0')}
      </span>
    </div>
  )
}
