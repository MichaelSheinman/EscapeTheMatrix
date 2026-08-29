/*
 * The layer between the canvas and the panels: markers over what is authored,
 * the action prompt over the character's head, and the plate that says which
 * of these people is you.
 *
 * Text stays DOM so it stays crisp, and nothing here is a menu — every marker
 * is pinned to a point in the painting, and acting on one is either walking
 * through a door or opening the screen the object *is*. Positions are written
 * imperatively on this layer's own animation frame, reading the renderer the
 * same way the renderer reads the store (§10.1): at its own cadence, owning
 * nothing.
 */

import { useEffect, useRef, useState } from 'react'

import { getRenderer } from '../render/handle'
import { chooseInChallenge, moveTo } from '../world/actions'
import { useWorld } from '../world/react'
import { world } from '../world/store'
import { markersFor, nearestTo, REACH, type MarkerScreen, type WorldMarker } from './markers'

/** How the marker layer explains each kind, in the tip's header and footer. */
const KIND = {
  portal: { kicker: 'Somewhere you can go in', foot: null },
  use: { kicker: 'Something you can use', foot: null },
  sign: { kicker: 'Just a name on a building', foot: 'Nothing to do here.' },
  soon: { kicker: 'Somewhere you will be able to go', foot: 'Not built yet.' },
} as const

/**
 * Act on a marker: a door is walked through, a panel is opened, a thing is
 * taken.
 *
 * The third one commits a choice in a run, so it reads the run rather than
 * trusting the marker — a marker is drawn from a plate baked months ago and the
 * run is what is true now.
 */
export function act(marker: WorldMarker, open: (screen: MarkerScreen) => void) {
  if (marker.go) {
    void moveTo(marker.go, 'foot')
    return
  }
  if (marker.screen) {
    open(marker.screen)
    return
  }
  if (!marker.item) return
  const run = world.getState().challenge?.run ?? null
  if (run && !run.done) void chooseInChallenge(run.challenge, run.scene, marker.item)
}

export function WorldOverlay({ onOpen }: { onOpen: (screen: MarkerScreen) => void }) {
  const signedIn = useWorld((w) => w.session.status === 'in')
  const places = useWorld((w) => w.places)

  /*
   * Which scene the run is standing at, so the markers can be rebuilt when it
   * moves.
   *
   * The frame loop below only rebuilds when the *location* changes, which is
   * right for a street — its doors do not come and go. A challenge's room is
   * different: every choice committed takes one of the things out of it, and a
   * marker for something already in the bag is a prompt that would be refused.
   * The scene key is what changes on every commit, so it is what this watches.
   */
  const scene = useWorld((w) => {
    const run = w.challenge?.run ?? null
    return run && !run.done ? run.scene : null
  })

  const [markers, setMarkers] = useState<WorldMarker[]>([])
  const [near, setNear] = useState<WorldMarker | null>(null)

  const layerEl = useRef<HTMLDivElement | null>(null)
  const promptEl = useRef<HTMLDivElement | null>(null)
  const plateEl = useRef<HTMLDivElement | null>(null)
  const markerEls = useRef(new Map<string, HTMLDivElement>())

  // Read by the frame loop without restarting it.
  const held = useRef({ markers, places, key: null as string | null, near: null as string | null })
  held.current.markers = markers
  held.current.places = places

  useEffect(() => {
    if (!signedIn) {
      setMarkers([])
      setNear(null)
      held.current.key = null
      held.current.near = null
      return
    }

    let frame = 0
    const tick = () => {
      frame = requestAnimationFrame(tick)
      const view = getRenderer()?.view() ?? null

      // A new location means a new set of markers. React renders them once;
      // every frame after that only moves them.
      // The location *and* the scene: a run committing a choice changes what is
      // left in the room without changing which room it is.
      const run = world.getState().challenge?.run ?? null
      const at = run && !run.done ? run.scene : ''
      const key = view ? `${view.key}|${at}` : null
      if (key !== held.current.key) {
        held.current.key = key
        const offering =
          run && !run.done ? new Set(run.choices.map((choice) => choice.key)) : undefined
        setMarkers(view ? markersFor(view, held.current.places, offering) : [])
        held.current.near = null
        setNear(null)
        return
      }
      if (!view) {
        // Nothing painted, nothing to pin: the plate and the prompt must not
        // linger where the last location left them.
        if (plateEl.current) plateEl.current.style.opacity = '0'
        return
      }

      const wide = layerEl.current?.clientWidth ?? 0
      const high = layerEl.current?.clientHeight ?? 0

      for (const marker of held.current.markers) {
        const el = markerEls.current.get(marker.id)
        if (!el) continue
        const s = view.toScreen(marker.at)
        el.style.transform = `translate(${s.x.toFixed(1)}px, ${s.y.toFixed(1)}px) translate(-50%, -100%)`
        el.style.opacity = s.x > -60 && s.x < wide + 60 && s.y > -40 && s.y < high + 40 ? '1' : '0'
        if (view.player) {
          const gap = Math.hypot(marker.at.x - view.player.x, marker.at.y - view.player.y)
          el.classList.toggle('near', marker.kind !== 'sign' && gap < REACH)
        }
        /*
         * The pointer is over the thing this marker names.
         *
         * A marker for an item has no size — the thing itself is painted on the
         * plate (render/things.ts) and the marker is only the point it stands
         * at — so `:hover` can never fire on it and the tip every other marker
         * gets was unreachable. The canvas knows what the pointer is over
         * because the canvas is what drew it; this is the same tip, opened by
         * the same gesture, on a class instead of a pseudo-class.
         *
         * The lift is how far the drawing rises above the point the marker is
         * pinned to. A marker sits at an object's *foot*, so a tip hung off it
         * opens across the object — the thing you are asking about, hidden by
         * the answer. Measured every frame because it is a screen distance: it
         * shrinks as the camera pulls out, and the thing it has to clear is
         * whatever is under the pointer now.
         */
        if (marker.item) {
          const on = view.hovering?.key === marker.item
          el.classList.toggle('over', on)
          if (on && view.hovering) {
            const top = view.toScreen(view.hovering.top)
            el.style.setProperty('--lift', `${Math.max(0, Math.round(s.y - top.y))}px`)
          }
        }
      }

      // What the O key would act on, said over the character's head.
      const next = nearestTo(held.current.markers, view.player)
      if ((next?.id ?? null) !== held.current.near) {
        held.current.near = next?.id ?? null
        setNear(next)
      }

      if (view.player) {
        const feet = view.toScreen(view.player)
        if (plateEl.current) {
          plateEl.current.style.transform = `translate(${feet.x.toFixed(1)}px, ${(feet.y - 66).toFixed(1)}px) translate(-50%, -100%)`
          plateEl.current.style.opacity = '1'
        }
        if (promptEl.current) {
          promptEl.current.style.transform = `translate(${feet.x.toFixed(1)}px, ${(feet.y - 92).toFixed(1)}px) translate(-50%, -100%)`
        }
      } else {
        if (plateEl.current) plateEl.current.style.opacity = '0'
      }
    }

    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [signedIn, scene])

  if (!signedIn) return null

  return (
    <>
      <div className="markers" ref={layerEl}>
        {markers.map((marker) => (
          <div
            key={marker.id}
            className={`marker ${marker.kind}${marker.item ? ' thing' : ''}`}
            style={{ opacity: 0 }}
            ref={(el) => {
              if (el) markerEls.current.set(marker.id, el)
              else markerEls.current.delete(marker.id)
            }}
            onClick={(e) => {
              e.preventDefault()
              act(marker, onOpen)
            }}
          >
            <span className="marker-tip">
              <span className="marker-tip-kicker">{KIND[marker.kind].kicker}</span>
              <p className="marker-tip-name">{marker.label}</p>
              {marker.tip ? <p className="marker-tip-body">{marker.tip}</p> : null}
              {KIND[marker.kind].foot ? (
                <span className="marker-tip-foot">{KIND[marker.kind].foot}</span>
              ) : (
                <span className="marker-tip-foot">
                  <kbd className="keycap">O</kbd>
                  <span>
                    {marker.kind === 'portal' ? 'or walk into the doorway' : 'when you are next to it'}
                  </span>
                </span>
              )}
              {marker.link ? (
                <a
                  className="marker-tip-credit"
                  href={marker.link.url}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(e) => e.stopPropagation()}
                >
                  {marker.link.label}
                </a>
              ) : null}
            </span>
            {/* A thing in a challenge's room draws itself on the canvas
                (render/things.ts), so the chrome pins nothing over it — a
                second copy in the DOM is what made these read as labels on a
                picture. What survives is the tip you get when you are near it. */}
            {marker.item ? null : <span className="marker-label">{marker.label}</span>}
            {marker.item ? null : <span className="marker-dot" />}
          </div>
        ))}
      </div>

      <div className={`prompt${near ? ' show' : ''}`} ref={promptEl}>
        {near && (
          <>
            <span className="prompt-key">{near.kind === 'portal' ? '↑' : 'O'}</span>
            <span className="prompt-text">
              <span className="prompt-label">{near.label}</span>
              <span className="prompt-hint">
                {near.kind === 'portal' ? (near.hint ? `${near.hint} · walk in` : 'walk in') : near.hint}
              </span>
            </span>
          </>
        )}
      </div>

      <div className="nameplate" ref={plateEl} style={{ opacity: 0 }}>
        <span className="nameplate-name">you</span>
        <span className="nameplate-tick" />
      </div>
    </>
  )
}
