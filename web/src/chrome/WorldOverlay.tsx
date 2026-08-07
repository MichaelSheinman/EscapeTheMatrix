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
import { moveTo } from '../world/actions'
import { useWorld } from '../world/react'
import { markersFor, nearestTo, REACH, type MarkerScreen, type WorldMarker } from './markers'

/** How the marker layer explains each kind, in the tip's header and footer. */
const KIND = {
  portal: { kicker: 'Somewhere you can go in', foot: null },
  use: { kicker: 'Something you can use', foot: null },
  sign: { kicker: 'Just a name on a building', foot: 'Nothing to do here.' },
  soon: { kicker: 'Somewhere you will be able to go', foot: 'Not built yet.' },
} as const

/** Act on a marker: a door is walked through, an object is opened. */
export function act(marker: WorldMarker, open: (screen: MarkerScreen) => void) {
  if (marker.go) void moveTo(marker.go, 'foot')
  else if (marker.screen) open(marker.screen)
}

export function WorldOverlay({ onOpen }: { onOpen: (screen: MarkerScreen) => void }) {
  const signedIn = useWorld((w) => w.session.status === 'in')
  const places = useWorld((w) => w.places)

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
      const key = view?.key ?? null
      if (key !== held.current.key) {
        held.current.key = key
        setMarkers(view ? markersFor(view, held.current.places) : [])
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
  }, [signedIn])

  if (!signedIn) return null

  return (
    <>
      <div className="markers" ref={layerEl}>
        {markers.map((marker) => (
          <div
            key={marker.id}
            className={`marker ${marker.kind}`}
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
              <p className="marker-tip-body">{marker.tip}</p>
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
            </span>
            <span className="marker-label">{marker.label}</span>
            <span className="marker-dot" />
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
