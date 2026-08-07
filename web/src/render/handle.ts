/*
 * Where the chrome finds the renderer.
 *
 * The renderer is created at boot in `main.tsx` and owned there (§10.1) — no
 * component creates or destroys it. But two pieces of chrome need to *read* it:
 * the markers layer asks where authored points land on the glass, and the
 * interact key asks what is nearest. This module is that one reference,
 * write-once at boot, null before the renderer starts and after teardown.
 *
 * It carries the `Renderer` interface only — Pixi stays confined to `render/`,
 * and nothing reachable from here can mount, unmount or draw.
 */

import type { Renderer } from './renderer'

let current: Renderer | null = null

/** `main.tsx` only: the renderer started, or was torn down. */
export function setRenderer(renderer: Renderer | null) {
  current = renderer
}

/** The running renderer, if there is one. */
export function getRenderer(): Renderer | null {
  return current
}
