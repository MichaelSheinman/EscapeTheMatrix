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
 * called `setState`. Which location to draw is decided the same way — by
 * looking at where the character says they are, on every frame, and noticing
 * when the answer changed.
 *
 * This is the only file in the client that imports Pixi, apart from the scene
 * it drives (§10.4: Pixi types stay behind a thin scene interface, so a
 * renderer swap stays reachable without a rewrite).
 */

import { Application, Assets, type Texture } from 'pixi.js'

import type { Standing, World } from '../world/state'
import { AT_THE_DOOR, PAINTED } from './level/locations'
import { loadLevel } from './level/tiled'
import { createScene, type Showing, type Spot } from './scene'
import { readTokens } from './tokens'

/** A running renderer. There is one, and the app owns it. */
export type Renderer = {
  /**
   * Draw the layer that is never drawn (§10.3) — a development view of the
   * walkable polygon, the trigger zones, the portals, the depth baselines and
   * the graph routes are found over. Off by default; `main.tsx` binds a key.
   */
  showLogic(on: boolean): void
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
   * The character is standing somewhere authored.
   *
   * The world layer's one write into the store. It carries no game rule — which
   * authored rectangle a walk ended inside is a fact about authored geometry —
   * and it is the seam the event system reads from when §3.9's placement
   * conditions become live (#21, #22).
   */
  onStanding: (standing: Standing | null) => void
}

/**
 * Start the renderer.
 *
 * Rejects when the device cannot run it. WebGL2 is a hard floor (§10.4): Pixi
 * v8 removed v7's Canvas2D fallback, so this is not a degradation to handle but
 * a fact to state — the caller puts it in the store and the UI says it in a
 * sentence.
 */
export async function startRenderer({ canvas, read, onStanding }: Boot): Promise<Renderer> {
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

  const scene = createScene(tokens, { onStanding })
  app.stage.addChild(scene.view)
  scene.resize(app.screen.width, app.screen.height)
  app.renderer.on('resize', (width: number, height: number) => {
    scene.resize(width, height)
    app.stage.hitArea = app.screen
  })

  /*
   * Loading a location.
   *
   * One texture budget consideration §10.4 states and this does not yet meet:
   * locations are held once loaded and never released, and nothing is
   * preloaded. With one painted street that is the right amount of machinery.
   * The graph that makes preloading and eviction meaningful is #12's.
   */
  const held = new Map<string, Showing>()
  /** Which place the scene should be showing. */
  let wanted: string | null = null
  /** Which place is being fetched, so a slow load is not started twice. */
  let loading: string | null = null

  function show(place: { key: string; name: string } | null) {
    const key = place?.key ?? AT_THE_DOOR
    if (key === wanted) return
    wanted = key

    const already = held.get(key)
    if (already) {
      scene.drawScene(already)
      return
    }

    const url = PAINTED[key]
    if (!url) {
      // Not an error and not a blank screen: §3.12's districts arrive one
      // painting at a time (#35), and until this one does the world says so.
      settle(key, { painted: false, name: place?.name ?? 'The city', note: 'not painted yet' })
      return
    }

    if (loading === key) return
    loading = key
    void mount(key, url)
  }

  async function mount(key: string, url: string) {
    try {
      const level = await loadLevel(url)
      // The plate goes through Pixi's asset cache, so returning to a location
      // already visited costs no upload.
      const plate = await Assets.load<Texture>(level.painting.url)
      settle(key, { painted: true, level, plate })
    } catch (err) {
      // A location that will not load is a broken build or a broken export.
      // Say so where a developer will see it, and put the ground and the name
      // on screen rather than nothing.
      console.error(`the location for ${key} could not be drawn`, err)
      settle(key, { painted: false, name: key, note: 'this location would not load' })
    } finally {
      if (loading === key) loading = null
    }
  }

  function settle(key: string, showing: Showing) {
    held.set(key, showing)
    // The player may have walked on while this was in flight.
    if (wanted === key) scene.drawScene(showing)
  }

  // The whole surface is one target. A tap on the world is a walk to it, which
  // is why it goes to the scene rather than through the command layer: moving
  // about inside one location changes nothing the server holds. Stepping
  // *between* locations does, and that is the command #12 wires the portals to.
  app.stage.eventMode = 'static'
  app.stage.hitArea = app.screen
  app.stage.on('pointertap', (event) => {
    const at: Spot = { x: event.global.x, y: event.global.y }
    scene.walkTo(at)
  })

  // The loop. `deltaMS` is real elapsed time, so animation runs at the same
  // speed on a 144Hz display as on a 60Hz one, and a backgrounded tab that
  // stopped ticking resumes without a jump.
  app.ticker.add((ticker) => {
    const world = read()
    show(world.character?.at ?? null)
    scene.update(world, ticker.deltaMS)
  })

  return {
    showLogic(on) {
      scene.showLogic(on)
    },

    destroy() {
      scene.destroy()
      // The canvas element outlives the renderer: it is in `index.html`, and
      // this module borrowed it.
      app.destroy({ removeView: false }, { children: true })
    },
  }
}
