# The House

A point-and-click adventure by Artur Kot, vendored as a fork and served inside
an iframe from a house in the city.

Upstream: https://github.com/arturkot/the-house-game

## Licenses

- Code: MIT, © 2019 Artur Kot. See `LICENSE.md`.
- Artwork: Creative Commons Attribution 3.0. Attribution is required wherever
  the game is shown.
- Sound: 36 files sourced from Freesound by their original uploaders, each
  under its own license. `README.md` lists every file with its author and
  source link. Each one needs its license confirmed before this ships beyond
  a local build.

## Serving

The iframe `src` ends in a slash: `/the-house/`.

`js/audio.js` builds every sound URL from `window.location.pathname`. A src of
`/the-house/index.html` resolves sounds to `/the-house/index.htmlsound/*.mp3`,
which 404s silently and leaves the game playing without audio.

## Local changes

- `index.html` loads the readable sources in `js/` rather than minified copies,
  so edits to `js/game.js` and its siblings take effect.
- Removed: `js/min/`, the `.ogg` copy of each sound, `swf/`, `js/room.bak.js`,
  `config.codekit*`, `README.html`, `CNAME`.
- Removed the Google Analytics snippet at the foot of `index.html`. It loaded
  `google-analytics.com/ga.js` and sent a pageview to the author's property.
  The game asks the network for nothing; every file it needs is served here.

## State

The game keeps a save under the `jStorage` key in localStorage, shared with the
app because both are the same origin. Our own keys are prefixed `etm.`.

`played` holds the story flags. `darkness_retract3` is written when the last
puzzle is solved, about fifteen seconds before the ending calls
`$.jStorage.flush()` and reloads. `flush()` resets the `jStorage` key alone.
