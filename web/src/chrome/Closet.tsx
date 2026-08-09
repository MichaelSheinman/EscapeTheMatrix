/*
 * The closet (§3's "the UI is part of the world"): the one panel whose subject
 * is the player rather than a number about them, and the only one you edit by
 * looking at the result. You stand in the mirror and the drawers are along the
 * bottom, within reach.
 *
 * Every pick is on the figure in the frame it is clicked, because there is
 * nothing else for it to be. No preview, no confirm, no save — the person
 * walking around behind the scrim is already wearing it (the scene reads the
 * same `look` slice every frame), and closing this is walking away rather than
 * committing.
 *
 * The vocabulary is authored, closed and small, like the rule composer's: the
 * player picks from what exists. Nothing here is bought yet — this is the
 * drawer you arrived with.
 */

import { useEffect, useRef, useState } from 'react'

import { canvasPen, draw, FIGURE_HEIGHT, paint, shadow, SKIN } from '../render/figure'
import { readTokens } from '../render/tokens'
import { useWorld } from '../world/react'
import { patch } from '../world/store'
import type { Look } from '../world/state'

type Slot = keyof Look

type Option = { label: string; color?: string }

const WEAR: { slot: Slot; label: string; note: string; options: Option[] }[] = [
  {
    slot: 'coat',
    label: 'Coat',
    note:
      'Everyone out there is drawn the way you are, and every one of them is somebody. ' +
      'The coat is how you find yourself among them.',
    options: [
      { label: 'Charcoal', color: '#38384a' },
      { label: 'Rust', color: '#8c4a34' },
      { label: 'Moss', color: '#4b5f4a' },
      { label: 'Slate', color: '#3f5570' },
      { label: 'Sand', color: '#97845f' },
    ],
  },
  {
    slot: 'hair',
    label: 'Hair',
    note: 'The one thing here nobody can take off you.',
    options: [
      { label: 'Black', color: '#1f1a17' },
      { label: 'Auburn', color: '#5e3220' },
      { label: 'Ash', color: '#6b6459' },
      { label: 'Fair', color: '#ae8a56' },
    ],
  },
  {
    slot: 'hat',
    label: 'Hat',
    note: 'A brim reads from across the road where a face never will.',
    options: [
      { label: 'None' },
      { label: 'Flat cap', color: '#34302a' },
      { label: 'Watch cap', color: '#4e5c68' },
    ],
  },
  {
    slot: 'bag',
    label: 'Bag',
    note: 'People remember a bag. They do not remember a coat.',
    options: [
      { label: 'None' },
      { label: 'Satchel', color: '#6b5540' },
      { label: 'Courier', color: '#45505c' },
    ],
  },
]

/** What a swatch is worth in the look: its paint, or null for the none option. */
const worth = (option: Option): string | null => option.color ?? null

/** The option currently on a slot. A slot left empty is wearing its none. */
function worn(slot: Slot, options: Option[], look: Look): Option {
  return options.find((o) => worth(o) === look[slot]) ?? options[0]
}

export function Closet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const look = useWorld((w) => w.look)
  const [drawer, setDrawer] = useState<Slot>('coat')
  const [pose, setPose] = useState<'stand' | 'walk'>('stand')

  const pick = (slot: Slot, option: Option) => {
    patch.look({ ...look, [slot]: worth(option) })
  }

  return (
    <div className={`overlay${open ? ' open' : ''}`}>
      <div className="overlay-scrim" onClick={onClose} />

      <div className="overlay-panel">
        <button className="overlay-close" onClick={onClose}>
          <kbd className="keycap">Esc</kbd> back to the world
        </button>

        <div className="page-head">
          <h1 className="page-title">The closet</h1>
          <p className="page-sub">
            A coat, whatever the last place left you with, and the figure everybody else sees.
          </p>
        </div>

        <div className="overlay-body">
          <div className="wardrobe">
            <div className="wardrobe-mirror">
              <div className="mirror">
                <div className="mirror-stage">
                  <Mirror look={look} walking={pose === 'walk'} live={open} />
                  <span className="mirror-name show">
                    <span className="nameplate-name">you</span>
                    <span className="nameplate-tick" />
                  </span>
                </div>
                <div className="mirror-controls">
                  <div className="btn-row">
                    <button
                      className={`btn small ${pose === 'stand' ? 'primary' : 'ghost'}`}
                      onClick={() => setPose('stand')}
                    >
                      Standing
                    </button>
                    <button
                      className={`btn small ${pose === 'walk' ? 'primary' : 'ghost'}`}
                      onClick={() => setPose('walk')}
                    >
                      Walking
                    </button>
                  </div>
                </div>
                <p className="mirror-note">
                  This is the size everybody else sees you at. Silhouette, not detail.
                </p>
              </div>
            </div>

            <div className="wardrobe-rack">
              <div className="rack">
                <div className="rack-tabs">
                  {WEAR.map((d) => {
                    const on = worn(d.slot, d.options, look)
                    return (
                      <span
                        key={d.slot}
                        className={`rack-tab${d.slot === drawer ? ' on' : ''}`}
                        role="button"
                        tabIndex={0}
                        onClick={() => setDrawer(d.slot)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') setDrawer(d.slot)
                        }}
                      >
                        <span
                          className={`swatch-chip${on.color ? '' : ' none'}`}
                          style={on.color ? { background: on.color } : undefined}
                        />
                        <span className="rack-tab-text">
                          <span className="rack-tab-label">{d.label}</span>
                          <span className="rack-tab-worn">{on.label}</span>
                        </span>
                      </span>
                    )
                  })}
                </div>

                <div className="rack-drawers">
                  {WEAR.map((d) => (
                    <div key={d.slot} className={`rack-drawer${d.slot === drawer ? ' open' : ''}`}>
                      <div className="swatches">
                        {d.options.map((option) => (
                          <span
                            key={option.label}
                            className={`swatch${worth(option) === look[d.slot] ? ' on' : ''}${option.color ? '' : ' none'}`}
                            role="button"
                            tabIndex={0}
                            onClick={() => pick(d.slot, option)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' || e.key === ' ') pick(d.slot, option)
                            }}
                          >
                            <span
                              className="swatch-chip"
                              style={option.color ? { background: option.color } : undefined}
                            />
                            <span className="swatch-label">{option.label}</span>
                          </span>
                        ))}
                      </div>
                      <p className="faint tiny mb0" style={{ marginTop: 12 }}>
                        {d.note}
                      </p>
                    </div>
                  ))}
                </div>

                <p className="rack-note faint tiny mb0">
                  Everything in these drawers is something you already own. What you can buy is on
                  the other side of money.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

/**
 * The figure in the mirror — the same figure the street draws, at the closet's
 * scale, live while the panel is open.
 *
 * `render/figure.ts` does the drawing, over a pen on this canvas. That is the
 * whole point of the mirror: a coat that looked right here and wrong from
 * across the road would be a lie the closet told you.
 */
function Mirror({ look, walking, live }: { look: Look; walking: boolean; live: boolean }) {
  const canvasEl = useRef<HTMLCanvasElement | null>(null)
  const state = useRef({ look, walking })
  state.current.look = look
  state.current.walking = walking

  useEffect(() => {
    if (!live) return
    const canvas = canvasEl.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return

    const tokens = readTokens()
    const pen = canvasPen(ctx)

    let frame = 0
    const began = performance.now()
    const tick = (now: number) => {
      frame = requestAnimationFrame(tick)
      const box = canvas.getBoundingClientRect()
      if (box.width === 0 || box.height === 0) return
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      if (canvas.width !== Math.round(box.width * dpr)) {
        canvas.width = Math.round(box.width * dpr)
        canvas.height = Math.round(box.height * dpr)
      }
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
      ctx.clearRect(0, 0, box.width, box.height)

      const t = (now - began) / 1000
      const { look, walking } = state.current
      const swing = walking ? Math.sin(t * 6.4) : 0
      const bob = walking ? -Math.abs(Math.sin(t * 6.4)) * 3 : 0

      // The figure is drawn from its feet up, in plate pixels; the mirror is
      // the same drawing enlarged until it fills the stage.
      const scale = (box.height * 0.72) / FIGURE_HEIGHT
      ctx.translate(box.width / 2, box.height * 0.9)
      ctx.scale(scale, scale)

      // The shadow stays put while the body bobs, exactly as it does outside.
      shadow(pen, tokens.color.text)
      ctx.translate(0, bob)

      draw(
        pen,
        {
          coat: paint(look.coat, tokens.color.accent),
          skin: SKIN,
          hair: look.hair === null ? null : paint(look.hair, tokens.color.text),
          hat: look.hat === null ? null : paint(look.hat, tokens.color.text),
          bag: look.bag === null ? null : paint(look.bag, tokens.color.text),
          rim: tokens.color.paper,
        },
        { step: swing, facing: 1, gesture: null },
      )
    }

    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [live])

  return <canvas className="mirror-figure" ref={canvasEl} />
}
