# The bake pipeline

The shipped world is generated from the preview's scene data (#34/#35/#39): the
preview's own painter draws each location at full plate resolution, and the
logic layer over it — where you can walk, what occludes, which spots are named
— is derived from the same data. The repo stores the recipe, never an image
library: change the scene, re-run the two steps, and the shipped location has
changed with it.

## The two steps

**1. The logic layer and the markers** (Node, no browser):

    node tools/bake/gen-tmj.mjs

Writes `web/public/locations/<key>/<key>.tmj` for the street and the apartment
— walkable polygon traced off the grid, furniture holes, occluders with
baselines, trigger zones in `internal/city`'s vocabulary, portals, spawn,
lights — plus `web/src/chrome/markers.gen.ts`, the labels the chrome pins over
the plates.

**2. The pictures** (a browser, because the painter is Canvas2D):

    node tools/bake/serve.mjs
    # then open, in any browser:
    #   http://localhost:8090/tools/bake/bake.html?bake=street
    #   http://localhost:8090/tools/bake/bake.html?bake=apartment

Each bake draws the scene twice with `preview/src/paint.js` — the same painter
the interactive preview runs — and PUTs both WebPs straight back to
`serve.mjs`, which writes them into `web/public/locations/<key>/`. The page
shows what it baked, so a bad frame is seen before it ships.

- `plate.webp` is the location, whole and opaque: what everybody is drawn on.
- `props.webp` is the same picture without the sky, the ground or the shadows,
  so everything a character can walk behind stands on transparency. The
  renderer cuts its occluders out of this one. An occluder is authored as a
  bounding box, and a box cut from the opaque plate paints a rectangle of
  street over anyone standing beside a tree — which is exactly what it used to
  do.

## The agreements that keep the two halves aligned

- `frame.mjs` is the *only* place the plate transform lives: iso projection,
  margins, per-scene scale. Both steps import it; the bake and the map cannot
  disagree about where a wall is.
- The two pictures are the same size and the same transform, so one occluder
  frame indexes both. `tiled.ts` refuses a props sheet that is not.
- People are never baked. The cast is live; a plate holds the place.
- Zone keys must be `internal/city`'s — `internal/city`'s own test reads every
  shipped `.tmj` and refuses a key the city does not name, and
  `web/src/render/level/shipped.test.ts` holds the same line plus the
  invariants (spawn on the floor, every doorway standable, the pair of
  locations leading to each other).
