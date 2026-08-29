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

Every game below has a live playable build, each link checked and loading a real game page.
Repos whose demo is dead or missing are under Dropped.

**Programming and logic**

| Repo | Genre | Play | Note |
|---|---|---|---|
| increpare/PuzzleScript | puzzle engine | https://www.puzzlescript.net/ | first target, above |
| medmunds/puzzles-web | 40 logic puzzles | https://puzzles.twistymaze.com/ | seeded game IDs, server-verifiable |
| magwo/elevatorsaga | elevator programming | http://play.elevatorsaga.com/ | `condition.evaluate()` already returns true/false/null |
| AlexNisnevich/untrusted | JS roguelike | https://alexnisnevich.github.io/untrusted/ | levels are individual JS files |
| blockly-games/blockly-games | block programming | https://blockly.games/ | levels are data |
| sharkdp/cube-composer | functional-programming puzzle | https://david-peter.de/cube-composer | |
| Q42/0hh1, Q42/0hn0 | logic puzzles | https://0hh1.com/ https://0hn0.com/ | small, clean |
| baruchel/sudoku-js | sudoku | http://baruchel.insomnia247.nl/sudoku-js/sudoku.html | |
| ondras/custom-tetris | tetris | http://ondras.github.io/custom-tetris/ | parameterised variants |
| Hextris/hextris | tetris variant | https://hextris.github.io/hextris | |
| gabrielecirulli/2048 | slide puzzle | http://gabrielecirulli.github.io/2048/ | |

**Board and card, vs computer**

| Repo | Genre | Play | Note |
|---|---|---|---|
| op12no2/lozza | chess engine | — (engine, not a game) | pick for speed chess. One self-contained JS file with the NNUE weights inlined, runs in a web worker. Hand-written and small enough to tune the search down to a beatable level |
| nmrugg/stockfish.js | chess engine | — (engine, not a game) | much stronger, fully offline WASM. Lite build ≈7MB. The threaded build needs `SharedArrayBuffer`, so cross-origin isolation (COOP/COEP) on both the frame and the parent — awkward against the sandboxed separate-origin setup. Single-threaded build avoids that. Limiting its strength to a target rating is fiddly |
| kenrick95/c4 | connect four | https://kenrick95.github.io/c4/ | AI opponent |
| Shahir-47/Battleship | battleship | https://shahir-47.github.io/Battleship/ | vs AI, small |
| playpip/pip-web | texas hold'em | https://playpip.io | single-player vs AI |

Neither chess engine ships a board. Pair with chessboard.js or chessground.

Rejected: `jonradoff/chessmata` is a platform with a Go backend, JWT and agent API keys, not an
embeddable game; its built-in opponent is only 2-ply minimax.
`juliangarnier/3D-Hartwig-chess-set` has no opponent at all — chess.js is rules only — and is
webkit-only. Avoid `kasuken/BattleshipAI` and `oil-oil/wolfcha`; both drive their opponent with an
LLM.

**Shooter and action arcade**

| Repo | Genre | Play | Note |
|---|---|---|---|
| MattSurabian/DuckHunt-JS | light-gun shooter | http://duckhuntjs.com | ships a level creator. Verified this works correctly! Though the graphics are slightly lacking for my taste. |
| dmcinnes/HTML5-Asteroids | asteroids | http://dougmcinnes.com/html-5-asteroids/ | http only, no https |
| Kuberwastaken/backdooms | doom-like | https://kuberwastaken.github.io/backdooms | |
| kripken/BananaBread | 3D FPS | (heavy, Cube 2 via Emscripten) | tested, it works well, but doesn't quite give the level-like progression I am hoping. |
| leereilly/Coil | arcade | | this is a cool game too! I am not too sure how it fits well within the theme of our game though |

**Escape room and adventure**

Thin genre, and the demo filter thinned the list further — the point-and-click engines all died. Two
finished games survive. For volume here, author our own rooms (PuzzleScript, or js13k entries).

| Repo | Genre | Play | Note |
|---|---|---|---|
| arturkot/the-house-game | escape room | http://the-house.arturkot.pl | finished game, closest direct fit. Added. |
| doublespeakgames/adarkroom | text adventure | http://adarkroom.doublespeakgames.com | strong tonal match | nope, skipping bad UX

**Racing, platformer, strategy**

| Repo | Genre | Play | Note |
|---|---|---|---|
| BKcore/HexGL | racing | http://hexgl.bkcore.com | WebGL, well built | This is a really good game. Doesn't quite fit with the theme for now, but keep it here for future consideration. 
| Casmo/tower-defense | tower defense | https://couchfriends.com/src/games/tower-defense/index.html | Three.js | hmm couldn't quite figure it out and no fit just yet. 
| R74nCom/sandboxels | falling-sand sandbox | https://sandboxels.r74n.com/ | no win condition, needs one invented | doesn't fit in yet. 

**Gym building**

No open-source game is literally about working out; the genre that carries the theme is rhythm
(cadence, reps, stamina) and boxing. All links verified live.

| Repo | Genre | Play | Note |
|---|---|---|---|
| bemusic/bemuse | rhythm | https://bemuse.ninja/ | the most polished candidate in the whole document. Keyboard mode, song select, scoring, ranking already built |
| religa/stepfever | DDR-style rhythm | https://stepfever.vercel.app/ | TypeScript, web-first, plays .sm/.ssc charts with keyboard or dance pad. Charts are data, so authoring a "workout" per machine is possible |
| joeheyming/stepmania | DDR-style rhythm | https://joeheyming.github.io/stepmania/ | web StepMania clone with scoring |
| justin-austria/PunchOut | boxing | https://justin-austria.github.io/PunchOut/ | NES Punch-Out port; sprites, music and sounds are Nintendo's, so we could only keep the mechanics, not the assets | nah, bad UX
| S-Stefan/punch-out-game | boxing | https://s-stefan.github.io/punch-out-game/ | simple Punch-Out-like, same Nintendo-assets caveat |
| JonJWong/In-the-Browser | rhythm | https://jonjwong.github.io/In-the-Browser/ | small StepMania-inspired |

Rhythm fits gym mechanics directly: accuracy percentage and combo map to a strength/stamina gain,
and per-song scores carry a leaderboard the same way puzzles do. `moe-khan123/StreetPhyter`
(fighting) is dead — 404.

**Library building**

Words, reading, knowledge. All links verified live.

| Repo | Genre | Play | Note |
|---|---|---|---|
| curiousdannii/parchment | interactive fiction player | https://iplayif.com/ | plays Z-Code, Glulx, TADS and more. Built for self-hosting, with a generator that outputs a single-file playable game. Thousands of free games on the IF Archive; a shelf in the library where each book is a playable story |
| monkeytypegame/monkeytype | typing test | https://monkeytype.com/ | self-hostable, WPM and accuracy are exact measured outcomes — the cleanest score signal of any candidate. Custom text mode means passages can come from our own content |
| pisimulation/WordChallenge | anagram sprint | https://pisimulation.github.io/WordChallenge/ | type words from given letters in 50 seconds, Scrabble letter scoring | This is a good one! we'll probably integrate it in the future, maybe in a future environment. Not my current priority for the first environment. 
| cwackerfuss/reactle | wordle | https://reactle.vercel.app/ | the standard open-source Wordle clone; word list is data, so daily words can be ours |
| frozenpandaman/puz | crossword player | https://frozenpandaman.github.io/puz/ | plays standard .puz files, NYT-style UI. Player only — crosswords themselves must be authored or licensed |

Trivia repos exist but every good one leans on the Open Trivia DB; pub-quiz questions written
through our content pipeline against a plain quiz UI would fit the game better than any of them.
`habibieamrullah/HTML5-Crossword-Puzzle-Game` demo 404s.

Interactive fiction is the standout theme fit — reading a book that plays. The catch is the hook:
Z-machine games have no standard win event, so completion detection is per-game (score line,
ending text), unlike the single-function patches elsewhere in this document.

**Verona diner**

The weakest genre for open-source browser games. Diner Dash clones are Java desktop student
projects; Overcooked clones are all Unity. Nothing self-hostable survives the live-demo filter
except two js13k 2025 café entries, both mirrored in the js13kGames org.

| Repo | Genre | Play | Note |
|---|---|---|---|
| js13kGames/kikis-cafe | café serving | https://js13kgames.com/2025/games/kikis-cafe | 13kB entry, untested |
| js13kGames/dinner-for-two | café/cooking | https://js13kgames.com/2025/games/dinner-for-two | 13kB entry, untested |

If neither plays well, the diner is the strongest case yet for authoring: an order-matching
game under time pressure is a small bespoke build, and serving-under-pressure maps directly onto
the diner job the building already carries.

**Dropped — demo dead or missing**

Working game link in the README was the filter. Failures, so the reason survives if one comes back
up: `lostdecade/onslaught_arena`, `StrykerKKD/SpaceInvaders`, `Couchfriends/breakout`,
`isovector/adventure`, `mitallast/diablo-js`, `dart-lang/pop-pop-win`, `robertkleffner/marioHTML5`
have no demo link at all. `stephank/arashi-js` and `CamHenlin/ExecutiveMan` 404.
`CamHenlin/Roguish` hangs on a redirect page. `operasoftware/Emberwind` points at a domain now
squatted by an unrelated site. `cykod/AlienInvasion` is a dead bit.ly redirect.
`adityaravishankar/command-and-conquer` redirects to the author's book page; the playable build
was taken down. `CodeArtemis/TriggerRally`: tried it, game is old, UX is very poor.

**Volume, once the shim pattern is settled**

https://github.com/js13kGames/games — ~2,700 competition entries, every genre including shooters
and escape-room styles, each self-contained and under 13kB. Individually licensed, many with no
license. https://github.com/Siorki/js13kgames is one author's entries across many years.

Lists worth re-reading when we want more: michelpereira/awesome-open-source-games,
leereilly/games, OpenSourceVideoGames/list.

`medmunds/puzzles-web` is the only candidate where the server can generate the puzzle, know the
solution, and verify the submission. That tier is the one where a real reward can safely ride on
the outcome.
