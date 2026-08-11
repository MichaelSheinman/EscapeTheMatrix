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
 * Pixi is confined to this file, the scene it drives, and the High tier that
 * scene builds (§10.4: Pixi types stay behind a thin scene interface, so a
 * renderer swap stays reachable without a rewrite). Nothing outside `render/`
 * imports it, and the three files that do are the three that draw.
 *
 * It is also where §10.4's asset pipeline is *run*. The pipeline's decisions —
 * which location to warm next, what has fallen out of the texture budget — are
 * `level/holding.ts`, which is arithmetic over strings and can be driven a
 * hundred locations deep in a test. What is left here is the half that needs a
 * GPU: fetching, uploading, and handing textures back.
 */

import { Application, Assets, type Texture } from 'pixi.js'

import { neighbours } from '../world/city'
import type { Phase, Standing, World } from '../world/state'
import { createBudget, nextToWarm } from './level/holding'
import { AT_THE_DOOR, mapFor } from './level/locations'
import { loadLevel, type Level, type Portal } from './level/tiled'
import { createGovernor, type Tier } from './quality'
import { createScene, type SceneView, type Showing, type Spot } from './scene'
import { readTokens } from './tokens'

/** A running renderer. There is one, and the app owns it. */
export type Renderer = {
  /**
   * Draw the layer that is never drawn (§10.3) — a development view of the
   * walkable polygon, the trigger zones, the portals, the depth baselines and
   * the graph routes are found over. Off by default; `main.tsx` binds a key.
   */
  showLogic(on: boolean): void
  /**
   * What is on screen, for the chrome floating over it (see `SceneView`).
   *
   * A read at the caller's own cadence — the markers layer asks on its own
   * animation frame, exactly as the renderer reads the store on its (§10.1).
   */
  view(): SceneView | null
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
  /**
   * They clicked the thing that is on fire (§4's offer, answered in the world).
   *
   * The canvas's second input that changes something the server holds, and it
   * goes through the same command layer as everything else (§10.1). The
   * renderer knows a fire was pressed; whether the offer is still standing and
   * what accepting opens are the server's answer to a command.
   */
  onFire: () => void
  /** They took one of the things in a challenge's room. */
  onThing: (key: string) => void
  /**
   * How fast the world is drawing, in frames a second (§10.4).
   *
   * Reported a couple of times a second rather than every frame: it is read by
   * a React panel, and a number that changed sixty times a second would be a
   * render sixty times a second — which is the coupling §10.1 rejected, arriving
   * through a status readout.
   */
  onFrames: (fps: number) => void
  /**
   * The machine could not carry the High tier, so the world dropped back.
   *
   * §10.4's Low tier is the floor every device gets, and this is what happens
   * when a device that asked for more turns out not to have it. Said outwards
   * rather than decided here — what the world does about it, and how it tells
   * the player, belongs to the app.
   */
  onDemoted: () => void
}

/**
 * Start the renderer.
 *
 * Rejects when the device cannot run it. WebGL2 is a hard floor (§10.4): Pixi
 * v8 removed v7's Canvas2D fallback, so this is not a degradation to handle but
 * a fact to state — the caller puts it in the store and the UI says it in a
 * sentence.
 */
export async function startRenderer({
  canvas,
  read,
  onStanding,
  onPortal,
  onFire,
  onThing,
  onFrames,
  onDemoted,
}: Boot): Promise<Renderer> {
  // `let`, not `const`: the palette turns at nightfall (§3.1). The night class
  // is already on the document by now — `startDayCycle` runs before this does
  // — so a session opened after dark reads the night palette from its first
  // frame.
  let tokens = readTokens()

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

  /**
   * The frame clock, and what it is allowed to conclude (§10.4).
   *
   * Declared before the scene because `show` below asks it to hold its judgement
   * whenever a location is mounted.
   */
  const governor = createGovernor()

  const scene = createScene(tokens, {
    onStanding,
    // Walking onto a door is asking to go through it. The scene reports it and
    // the app sends the command; nothing here decides whether it is possible.
    onPortal: (portal: Portal) => onPortal(portal.to),
    onFire,
    onThing,
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
   * away, and hold no more than a budget of them. The policy is
   * `level/holding.ts`; what is here is the fetching and the unloading, which
   * is the part that needs a network and a GPU.
   */
  /**
   * What the budget holds: the pictures, and the URLs they were loaded by.
   *
   * The URLs travel with the textures because they are no longer a function
   * of the location alone — at night the `-night` bake is loaded when the
   * build ships one — and `Assets.unload` wants exactly the name
   * `Assets.load` was given.
   */
  type Held = { showing: Showing; plate: string | null; props: string | null }

  const budget = createBudget<Held>()
  /** Which place the scene should be showing. */
  let wanted: string | null = null
  /** Which place it was showing before that, so an arrival can find its door. */
  let came: string | null = null
  /** Which places are being fetched, so a slow load is never started twice. */
  const loading = new Set<string>()
  /**
   * The pictures still on screen while their other-half-of-the-day
   * replacement is in flight. Released when the replacement settles —
   * unloading them the moment the phase flips would destroy the very
   * textures the player is looking at.
   */
  let fading: Held | null = null
  /**
   * Night bakes this build turned out not to ship, so a walk past the same
   * place after dark is one miss rather than one per visit.
   */
  const unbaked = new Set<string>()

  function show(place: { key: string; name: string } | null, near: string[]) {
    const key = place?.key ?? AT_THE_DOOR
    if (key !== wanted) {
      came = wanted
      wanted = key
      budget.showing(key)
      // Mounting a location uploads a plate, which is a slow frame that says
      // nothing about whether this machine can carry the tier it is drawing at.
      governor.settle()

      const already = budget.use(key)
      if (already) scene.drawScene(already.showing, came)
      else void mount(key, place?.name ?? 'The city', came)
    }

    // Everywhere one step from here, warmed but not shown. Started after the
    // location being looked at, so a preload never delays an arrival.
    const warm = nextToWarm(budget, loading, near, (next) => mapFor(next) !== null)
    if (warm) void mount(warm, warm, null, { show: false })
  }

  /**
   * Fetch a location and put it on screen, unless it is only being warmed.
   *
   * `from` is where the player came from, which is what lets the scene put them
   * at the door they came in by rather than at the map's spawn. `repaint` is
   * the other way in: the same place at the other time of day, under a
   * character who must not move for it.
   */
  async function mount(
    key: string,
    name: string,
    from: string | null,
    options: { show?: boolean; repaint?: boolean } = {},
  ) {
    const showing = options.show !== false
    const url = mapFor(key)
    if (!url) {
      // Not an error and not a blank screen: §3.12's districts arrive one
      // painting at a time (#35), and until this one does the world says so.
      if (showing) {
        settle(
          key,
          { showing: { painted: false, name, note: 'not painted yet' }, plate: null, props: null },
          from,
          options.repaint,
        )
      }
      return
    }
    if (loading.has(key)) return
    loading.add(key)
    try {
      const level = await loadLevel(url)
      settle(key, await pictures(level), from, options.repaint)
    } catch (err) {
      // A location that will not load is a broken build or a broken export.
      // Say so where a developer will see it, and put the ground and the name
      // on screen rather than nothing.
      console.error(`the location for ${key} could not be drawn`, err)
      settle(
        key,
        {
          showing: { painted: false, name, note: 'this location would not load' },
          plate: null,
          props: null,
        },
        from,
        options.repaint,
      )
    } finally {
      loading.delete(key)
    }
  }

  /**
   * Both of a location's pictures, at the time of day the world is in.
   *
   * The plate goes through Pixi's asset cache, so returning to a location
   * already held costs no upload. The props sheet beside it is what the
   * cut-outs are taken from (§10.3) — asked for together, because a location
   * drawing its ground without the things standing on it would be a frame
   * nobody should see.
   *
   * At night the `-night` pair baked beside the day one is asked for first.
   * A location nobody has baked at night falls back to its day pictures, and
   * the dark is carried by the sky, the grade and the chrome instead — true
   * of the deprecated riverfront, and of any place the bake has not reached.
   */
  async function pictures(level: Level): Promise<Held> {
    if (phase === 'night') {
      const plateUrl = nightUrl(level.painting.url)
      const propsUrl = level.props ? nightUrl(level.props.url) : null
      if (!unbaked.has(plateUrl)) {
        try {
          const [plate, props] = await Promise.all([
            Assets.load<Texture>(plateUrl),
            propsUrl ? Assets.load<Texture>(propsUrl) : Promise.resolve(null),
          ])
          return { showing: { painted: true, level, plate, props }, plate: plateUrl, props: propsUrl }
        } catch {
          unbaked.add(plateUrl)
        }
      }
    }
    const [plate, props] = await Promise.all([
      Assets.load<Texture>(level.painting.url),
      level.props ? Assets.load<Texture>(level.props.url) : Promise.resolve(null),
    ])
    return {
      showing: { painted: true, level, plate, props },
      plate: level.painting.url,
      props: level.props?.url ?? null,
    }
  }

  /**
   * `plate.webp` → `plate-night.webp`, whatever the extension.
   *
   * Note for whoever just baked: `make watch` does not watch `.webp`, and
   * dist embeds at compile time — a night plate that landed in `web/public`
   * after the last rebuild is not being served until the next one.
   */
  function nightUrl(url: string): string {
    return url.replace(/\.(\w+)$/, '-night.$1')
  }

  /**
   * A location has arrived. Put it in the budget, and let go of what fell out.
   *
   * The pictures go with it: `Assets.unload` is what actually returns the
   * textures to the GPU, and a budget that only forgot the JavaScript object
   * would be a budget in name only.
   */
  function settle(key: string, held: Held, from: string | null, repaint = false) {
    for (const gone of budget.hold(key, held)) release(gone.value)
    // The player may have walked on while this was in flight.
    if (wanted === key) {
      if (repaint) scene.repaint(held.showing)
      else scene.drawScene(held.showing, from)
      // Whatever was on screen before a phase flip is now safely underneath
      // its replacement. A location with no night bake came back under the
      // same URLs, and unloading those would destroy what was just mounted.
      if (fading) {
        if (fading.plate && fading.plate !== held.plate) void Assets.unload(fading.plate)
        if (fading.props && fading.props !== held.props) void Assets.unload(fading.props)
        fading = null
      }
    }
  }

  /** Both of a location's pictures, back to the GPU. */
  function release(held: Held) {
    if (held.plate) void Assets.unload(held.plate)
    if (held.props) void Assets.unload(held.props)
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
  // The wheel pulls the view in and out, the way the preview's does. The scene
  // clamps it, so there is no state to keep here.
  app.stage.on('wheel', (event) => {
    scene.zoomBy(event.deltaY < 0 ? 1.12 : 1 / 1.12)
  })

  /** What the world is drawing at, as last applied. Read from the store. */
  let quality: Tier = 'low'
  /** Which half of the day the world was last drawn in. */
  let phase: Phase = read().phase
  /** The last rate said outwards, so nothing is said twice. */
  let said = 0

  // The loop. `deltaMS` is real elapsed time, so animation runs at the same
  // speed on a 144Hz display as on a 60Hz one, and a backgrounded tab that
  // stopped ticking resumes without a jump.
  app.ticker.add((ticker) => {
    const world = read()

    // The effects tier, read from the store like everything else the renderer
    // needs (§10.1) rather than pushed at it. A player switching tiers is a
    // patch into the world model, and this is the frame that notices.
    if (world.quality.tier !== quality) {
      quality = world.quality.tier
      scene.setQuality(quality)
      // Building or tearing down the High tier compiles shaders and allocates
      // render targets. Judging those frames would demote every machine that
      // ever tried it.
      governor.settle()
    }

    /*
     * Day turned to night, or back (§3.1). The document's class flipped
     * before the store did (`clock.ts`), so the tokens read here are already
     * the new palette. The sky repaints, the scene re-inks, and every held
     * picture is from the wrong half of the day: the ones off screen are
     * simply let go — they re-warm exactly as they first loaded — and the one
     * on screen is remounted as a repaint, so the character keeps standing
     * where they stand while the plate underneath them turns.
     */
    if (world.phase !== phase) {
      phase = world.phase
      tokens = readTokens()
      app.renderer.background.color = tokens.color.sky
      scene.setTokens(tokens)
      governor.settle()
      for (const gone of budget.empty()) {
        if (gone.key === wanted) fading = gone.value
        else release(gone.value)
      }
      if (wanted) {
        budget.showing(wanted)
        void mount(wanted, wanted, null, { repaint: true })
      }
    }

    /*
     * Where the player is, and everywhere one step from it — unless a challenge
     * has taken them somewhere the city does not have.
     *
     * A run whose challenge names a canvas is stood in that canvas for as long
     * as it is running, and the moment it finishes the world underneath comes
     * back. It is deliberately *not* a move: nothing about `character.at`
     * changes, no journey is made, no fare is paid, and the player is on their
     * own landing the whole time. What changes is what they are looking at,
     * which is the honest description of walking into a burning flat for four
     * minutes and coming back out.
     *
     * Nothing is warmed around it, because there is nowhere to go from here.
     * Read every frame like everything else the renderer needs (§10.1), so the
     * swap happens on the frame the run begins and reverses on the frame it
     * ends, without anything having to remember to tell it.
     */
    const run = world.challenge?.run ?? null
    const inside = run && !run.done && run.canvas ? run.canvas : null
    if (inside && run) show({ key: inside, name: run.name }, [])
    else show(world.character?.at ?? null, neighbours(world.character?.at ?? null))
    scene.update(world, ticker.deltaMS)

    // Measured after the frame's work is asked for rather than before it, so
    // what is weighed is the tier's own cost.
    if (governor.measured(ticker.deltaMS, quality) === 'drop') onDemoted()

    const rate = governor.rate()
    if (rate !== said) {
      said = rate
      onFrames(rate)
    }
  })

  return {
    showLogic(on) {
      scene.showLogic(on)
    },

    view() {
      return scene.observe()
    },

    destroy() {
      scene.destroy()
      // Every plate goes back, not only the ones past the budget. A dev reload
      // replaces this module under a running page, and a texture cache that
      // survived it would be the whole city held by nobody.
      for (const gone of budget.empty()) release(gone.value)
      // The canvas element outlives the renderer: it is in `index.html`, and
      // this module borrowed it.
      app.destroy({ removeView: false }, { children: true })
    },
  }
}
