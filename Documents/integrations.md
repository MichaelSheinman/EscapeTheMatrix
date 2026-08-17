## Third-party games

Open-source games, self-hosted, embedded in a sandboxed iframe. Each one is patched at its
existing win/lose point to report the outcome back to the parent. Side content, not the core loop.

### Constraints

No runtime LLM in any integrated game. Every opponent, generator and hint system must be local
search or authored content, running offline in the browser with no API key and no per-play cost.
Rules out LLM-driven opponents, and rules out any game whose difficulty depends on a hosted model.

Every candidate below meets this. The ones that do not are named as rejected in their section.

### Contract

Parent creates a `MessageChannel` and hands `port2` to the frame on load. The game posts:

```js
{ v: 1, type: 'etm.result', session, outcome, score, meta }
```

`session` is a server-issued single-use nonce. The result posts to the Go server with that nonce,
never to client state. Frame is `sandbox="allow-scripts"` on a separate origin.

Score comes from client code the player can edit. Server caps reward per session and per day,
and checks elapsed time against claimed score. Keep the reward ceiling low.

### PuzzleScript — first target

https://github.com/increpare/PuzzleScript — MIT.

A DSL for tile-based puzzle games, not a game. A puzzle is ~40 lines of text, so we author our own
against our own settings. One patch to `src/js/engine.js` instruments every puzzle we ever write.

Hook: `DoWin()`. Related: `checkWin()`, `nextLevel()`, `goToTitleScreen()`, globals `winning`,
`curlevel`, `curlevelTarget`. No existing `postMessage` in the engine.

Gives the player something to be sharp at rather than something to grind. Deterministic puzzles
with a fixed solution mean a completion time is comparable across players, so puzzles carry a
leaderboard.

Fork with mouse input and a level solver: https://github.com/Auroriax/PuzzleScriptPlus — MIT.

### Candidate repos

Browser-playable, single-player or vs-computer, self-hostable. Licensing not yet checked — several
are single-author hobby projects where asking the author directly is the realistic path, and the
ones carrying no license at all can only be used by asking.

Nothing here ships outcome events. All need the shim. What separates a cheap integration from an
expensive one is whether the game has one clean win/lose function to patch.

**Programming and logic**

| Repo | Genre | Note |
|---|---|---|
| increpare/PuzzleScript | puzzle engine | first target, above |
| medmunds/puzzles-web | 40 logic puzzles | seeded game IDs, server-verifiable |
| magwo/elevatorsaga | elevator programming | `condition.evaluate()` already returns true/false/null |
| AlexNisnevich/untrusted | JS roguelike | levels are individual JS files |
| blockly-games/blockly-games | block programming | levels are data |
| sharkdp/cube-composer | functional-programming puzzle | |
| Q42/0hh1, Q42/0hn0 | logic puzzles | small, clean |
| baruchel/sudoku-js | sudoku | |
| ondras/custom-tetris | tetris | parameterised variants |
| Hextris/hextris | tetris variant | |
| gabrielecirulli/2048 | slide puzzle | |

**Board and card, vs computer**

| Repo | Genre | Note |
|---|---|---|
| op12no2/lozza | chess engine | pick for speed chess. One self-contained JS file with the NNUE weights inlined, runs in a web worker. Hand-written and small enough to tune the search down to a beatable level |
| nmrugg/stockfish.js | chess engine | much stronger, fully offline WASM. Lite build ≈7MB. The threaded build needs `SharedArrayBuffer`, so cross-origin isolation (COOP/COEP) on both the frame and the parent — awkward against the sandboxed separate-origin setup. Single-threaded build avoids that. Limiting its strength to a target rating is fiddly |
| kenrick95/c4 | connect four | AI opponent |
| Shahir-47/Battleship | battleship | vs AI, small |
| dart-lang/pop-pop-win | minesweeper | |
| playpip/pip-web | texas hold'em | single-player vs AI |

Neither chess engine ships a board. Pair with chessboard.js or chessground.

Rejected: `jonradoff/chessmata` is a platform with a Go backend, JWT and agent API keys, not an
embeddable game; its built-in opponent is only 2-ply minimax.
`juliangarnier/3D-Hartwig-chess-set` has no opponent at all — chess.js is rules only — and is
webkit-only. Avoid `kasuken/BattleshipAI` and `oil-oil/wolfcha`; both drive their opponent with an
LLM.

**Shooter and action arcade**

| Repo | Genre | Note |
|---|---|---|
| MattSurabian/DuckHunt-JS | light-gun shooter | ships a level creator |
| lostdecade/onslaught_arena | arena shooter | |
| cykod/AlienInvasion | space shooter | |
| dmcinnes/HTML5-Asteroids | asteroids | |
| StrykerKKD/SpaceInvaders | space invaders | |
| stephank/arashi-js | tempest-style | |
| kripken/BananaBread | 3D FPS | Cube 2 via Emscripten, heavy |
| Kuberwastaken/backdooms | doom-like | |
| leereilly/Coil | arcade | |
| Couchfriends/breakout | breakout | |

**Escape room and adventure**

Thin genre. Finished games are scarce; engines are the better buy, same argument as PuzzleScript —
adopt the engine, author our own rooms against our own setting.

| Repo | Genre | Note |
|---|---|---|
| arturkot/the-house-game | escape room | finished game, closest direct fit |
| doublespeakgames/adarkroom | text adventure | strong tonal match |
| isovector/adventure | point-and-click engine | Lua scripting, content editor |
| mitallast/diablo-js | isometric dungeon | |
| CamHenlin/Roguish | roguelike | |

**Racing, platformer, strategy**

| Repo | Genre | Note |
|---|---|---|
| BKcore/HexGL | racing | WebGL, well built |
| CodeArtemis/TriggerRally | rally racing | |
| robertkleffner/marioHTML5 | platformer | infinite Mario |
| operasoftware/Emberwind | platformer | |
| CamHenlin/ExecutiveMan | platformer | megaman clone |
| Casmo/tower-defense | tower defense | Three.js |
| adityaravishankar/command-and-conquer | RTS | |
| R74nCom/sandboxels | falling-sand sandbox | no win condition, needs one invented |

**Volume, once the shim pattern is settled**

https://github.com/js13kGames/games — ~2,700 competition entries, every genre including shooters
and escape-room styles, each self-contained and under 13kB. Individually licensed, many with no
license. https://github.com/Siorki/js13kgames is one author's entries across many years.

Lists worth re-reading when we want more: michelpereira/awesome-open-source-games,
leereilly/games, OpenSourceVideoGames/list.

`medmunds/puzzles-web` is the only candidate where the server can generate the puzzle, know the
solution, and verify the submission. That tier is the one where a real reward can safely ride on
the outcome.
