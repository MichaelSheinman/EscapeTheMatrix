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
 *
 * It is also where §10.4's asset pipeline lives: **per-location lazy loading
 * with adjacent-location preload, and a texture budget so the GPU never holds
 * the whole city.** All three need to know which locations are next to which,
 * which is the location graph the server sends with the world (§3.12) — so
 * until #12 there was nothing here but a map that never let go of anything.
 */

import { Application, Assets, type Texture } from 'pixi.js'

import { neighbours } from '../world/city'
import type { Standing, World } from '../world/state'
import { AT_THE_DOOR, mapFor } from './level/locations'
import { loadLevel, type Portal } from './level/tiled'
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
  /**
   * They walked onto a way out (§3.12).
   *
   * The canvas's one input that changes something the server holds, and it goes
   * through the same command layer a button does (§10.1) — which is why this is
   * a callback the app wires rather than a fetch in here. The renderer knows
   * that a door was walked onto; whether the journey happens, what it costs and
   * what is on the way are the city's answer to a command.
   */
  onPortal: (to: string) => void
}

/**
 * How many locations are held at once.
 *
 * §10.4's texture budget, as a number. A painted location is a plate of a
 * megapixel or two, and the whole city will be dozens of them — so the world
 * holds the handful you are moving through and lets go of the rest, which is
 * exactly the set the location graph makes namable. Small enough to be a budget;
 * large enough that walking a street, going indoors and coming back out again
 * costs one upload rather than three.
 */
const BUDGET = 6

/**
 * Start the renderer.
 *
 * Rejects when the device cannot run it. WebGL2 is a hard floor (§10.4): Pixi
 * v8 removed v7's Canvas2D fallback, so this is not a degradation to handle but
 * a fact to state — the caller puts it in the store and the UI says it in a
 * sentence.
 */
export async function startRenderer({ canvas, read, onStanding, onPortal }: Boot): Promise<Renderer> {
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

  const scene = createScene(tokens, {
    onStanding,
    // Walking onto a door is asking to go through it. The scene reports it and
    // the app sends the command; nothing here decides whether it is possible.
    onPortal: (portal: Portal) => onPortal(portal.to),
  })
  app.stage.addChild(scene.view)
  scene.resize(app.screen.width, app.screen.height)
  app.renderer.on('resize', (width: number, height: number) => {
    scene.resize(width, height)
    app.stage.hitArea = app.screen
  })

  /*
   * Loading a location, holding a few, and letting the rest go.
   *
   * §10.4 asks for three things and the location graph (§3.12) is what makes all
   * three sayable: load a location when it is needed, preload the ones one step
   * away, and hold no more than a budget of them. `held` is an insertion-ordered
   * Map used as an LRU — re-showing a location moves it to the end, and the
   * front is what goes when the budget is exceeded.
   */
  const held = new Map<string, Showing>()
  /** Which place the scene should be showing. */
  let wanted: string | null = null
  /** Which place it was showing before that, so an arrival can find its door. */
  let came: string | null = null
  /** Which places are being fetched, so a slow load is never started twice. */
  const loading = new Set<string>()

  function show(place: { key: string; name: string } | null, near: string[]) {
    const key = place?.key ?? AT_THE_DOOR
    if (key !== wanted) {
      came = wanted
      wanted = key

      const already = touch(key)
      if (already) scene.drawScene(already, came)
      else void mount(key, place?.name ?? 'The city', came)
    }

    // Everywhere one step from here, warmed but not shown. Started after the
    // location being looked at, so a preload never delays an arrival.
    if (loading.size === 0) {
      for (const next of near) {
        if (!held.has(next) && mapFor(next)) {
          void mount(next, next, null, { show: false })
          // One at a time. A city block of plates fetched at once is the same
          // bytes arriving later, on a connection the arrival is sharing.
          break
        }
      }
    }
  }

  /**
   * Fetch a location and put it on screen, unless it is only being warmed.
   *
   * `from` is where the player came from, which is what lets the scene put them
   * at the door they came in by rather than at the map's spawn.
   */
  async function mount(
    key: string,
    name: string,
    from: string | null,
    options: { show?: boolean } = {},
  ) {
    const showing = options.show !== false
    const url = mapFor(key)
    if (!url) {
      // Not an error and not a blank screen: §3.12's districts arrive one
      // painting at a time (#35), and until this one does the world says so.
      if (showing) settle(key, { painted: false, name, note: 'not painted yet' }, from)
      return
    }
    if (loading.has(key)) return
    loading.add(key)
    try {
      const level = await loadLevel(url)
      // The plate goes through Pixi's asset cache, so returning to a location
      // already held costs no upload.
      const plate = await Assets.load<Texture>(level.painting.url)
      settle(key, { painted: true, level, plate }, from)
    } catch (err) {
      // A location that will not load is a broken build or a broken export.
      // Say so where a developer will see it, and put the ground and the name
      // on screen rather than nothing.
      console.error(`the location for ${key} could not be drawn`, err)
      settle(key, { painted: false, name, note: 'this location would not load' }, from)
    } finally {
      loading.delete(key)
    }
  }

  function settle(key: string, showing: Showing, from: string | null) {
    held.set(key, showing)
    // The player may have walked on while this was in flight.
    if (wanted === key) scene.drawScene(showing, from)
    evict()
  }

  /** Mark a location as the most recently wanted, and hand it back. */
  function touch(key: string): Showing | undefined {
    const showing = held.get(key)
    if (showing) {
      held.delete(key)
      held.set(key, showing)
    }
    return showing
  }

  /**
   * Let go of everything past the budget, least recently shown first.
   *
   * The plate goes with it: `Assets.unload` is what actually returns the
   * texture to the GPU, and a cache that only forgot the JavaScript object
   * would be a budget in name only. The location being shown is never a
   * candidate, however old it is.
   */
  function evict() {
    for (const [key, showing] of held) {
      if (held.size <= BUDGET) return
      if (key === wanted) continue
      held.delete(key)
      if (showing.painted) void Assets.unload(showing.level.painting.url)
    }
  }

  // The whole surface is one target. A tap on the world is a walk to it, and it
  // goes to the scene rather than through the command layer because moving
  // about inside one location changes nothing the server holds. Stepping
  // *between* locations does — and that happens by walking onto a portal, which
  // the scene reports back out through `onPortal` above.
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
    // Where the player is, and everywhere one step from it. Both read from the
    // store every frame rather than subscribed to (§10.1) — the graph arrives
    // with the world's hydration and changes only when the server says so.
    show(world.character?.at ?? null, neighbours(world.character?.at ?? null))
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
