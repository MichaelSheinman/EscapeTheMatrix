/*
 * The renderer's lifecycle, which is not React's.
 *
 * technical.md §10.1: **Pixi's lifecycle is owned outside React entirely.**
 * Created at app boot, destroyed at teardown, never touched by a component
 * mounting or unmounting. React renders chrome above the canvas and never looks
 * inside it; nothing in the component tree can reach this module.
 *
 * That is not tidiness. Unmounting the canvas would rebuild the WebGL context
 * and the texture cache on every navigation — visibly slow, and a source of GPU
 * memory churn — so the canvas never unmounts and screens are overlays above
 * it. React's development-mode double-mount is the same hazard in miniature,
 * and the reason this is a plain function called from `main.tsx` rather than
 * anything with a `useEffect` in it.
 *
 * The other half of §10.1 is the read direction: the renderer reads the store
 * **directly, every frame**, and subscribes to nothing. A renderer limited to
 * React's update cadence is exactly the coupling that got `@pixi/react`
 * rejected, and it would arrive just as surely through a subscription that
 * called `setState`.
 *
 * This is the only file in the client that imports Pixi, apart from the scene
 * it drives (§10.4: Pixi types stay behind a thin scene interface, so a
 * renderer swap stays reachable without a rewrite).
 */

import { Application } from 'pixi.js'

import type { World } from '../world/state'
import { createScene, type Spot } from './scene'
import { readTokens } from './tokens'

/** A running renderer. There is one, and the app owns it. */
export type Renderer = {
  destroy(): void
}

export type Boot = {
  /** The canvas in `index.html`. It belongs to the page, not to this module. */
  canvas: HTMLCanvasElement
  /**
   * Read the world.
   *
   * Called once per frame. Deliberately a function rather than a subscription:
   * see the note above, and `../world/store.ts`.
   */
  read: () => World
  /**
   * Somebody tapped the world.
   *
   * The renderer does not know what that means. Canvas input dispatches through
   * the same command layer the UI uses (§10.1), and the wiring that says which
   * command lives in `main.tsx` with the rest of the app's plumbing — which is
   * what keeps this directory free of any knowledge of the server.
   */
  onTap: (at: Spot) => void
}

/**
 * Start the renderer.
 *
 * Rejects when the device cannot run it. WebGL2 is a hard floor (§10.4): Pixi
 * v8 removed v7's Canvas2D fallback, so this is not a degradation to handle but
 * a fact to state — the caller puts it in the store and the UI says it in a
 * sentence.
 */
export async function startRenderer({ canvas, read, onTap }: Boot): Promise<Renderer> {
  const tokens = readTokens()

  const app = new Application()
  await app.init({
    canvas,
    // The canvas is positioned by `@etm/ui`'s shell and fills its viewport;
    // Pixi follows that element rather than the window, so it stays right
    // whatever the layout does with it.
    resizeTo: canvas.parentElement ?? window,
    resolution: window.devicePixelRatio || 1,
    autoDensity: true,
    antialias: true,
    background: tokens.color.sky,
    // Stated rather than inferred, so the floor in §10.4 is the floor in the
    // code as well as in the document.
    preference: 'webgl',
    powerPreference: 'high-performance',
  })

  const scene = createScene(tokens)
  app.stage.addChild(scene.view)
  scene.resize(app.screen.width, app.screen.height)
  app.renderer.on('resize', (width: number, height: number) => {
    scene.resize(width, height)
    app.stage.hitArea = app.screen
  })

  // The whole surface is one target: this scene has nothing in it to hit, and
  // the objects that will — people, doors, trigger zones (§10.3) — become
  // targets of their own rather than cases in here.
  app.stage.eventMode = 'static'
  app.stage.hitArea = app.screen
  app.stage.on('pointertap', (event) => {
    const at = { x: event.global.x, y: event.global.y }
    // The flourish is immediate and local, because it is presentation. What the
    // tap *means* takes a round trip, and the world changes when the server
    // says it did — never before (§10.2).
    scene.playEffect('tap', at)
    onTap(at)
  })

  // The loop. `deltaMS` is real elapsed time, so animation runs at the same
  // speed on a 144Hz display as on a 60Hz one, and a backgrounded tab that
  // stopped ticking resumes without a jump.
  app.ticker.add((ticker) => scene.update(read(), ticker.deltaMS))

  return {
    destroy() {
      scene.destroy()
      // The canvas element outlives the renderer: it is in `index.html`, and
      // this module borrowed it.
      app.destroy({ removeView: false }, { children: true })
    },
  }
}
