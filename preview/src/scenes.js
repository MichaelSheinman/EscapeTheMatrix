/* Escape the Matrix — location data.
   Each location is a ground grid, a set of boxes with faces you can paste
   windows onto, fixtures, cast shadows, actors and a scripted walk. In the real
   client this is what Tiled exports: the painted plate plus an invisible logic
   layer of walkables, portals and named trigger zones (technical.md §10.3).
   Here the plate is drawn procedurally so it moves.

   V1 is daylight. One dominant source — the sun — with real cast shadows,
   warm lit faces and cool shade: the art bible's lighting rule read at
   mid-morning rather than at midnight. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  /* Ground palettes, lit for full sun. Codes are single characters below.
     '.' is void — nothing is painted there. */
  var GROUND = {
    '.': null,
    '#': null,                                     // building footprint
    s: { a: '#bfb29a', b: '#cabda5' },             // pavement
    r: { a: '#79736c', b: '#847d75' },             // asphalt
    k: { a: '#ded6c4', b: '#eae3d2' },             // crosswalk paint
    p: { a: '#b3a58d', b: '#bfb198' },             // plaza paving
    f: { a: '#b98d5c', b: '#c99c68' },             // floorboards
    t: { a: '#aca396', b: '#b7aea1' },             // interior tile
    g: { a: '#7d9d57', b: '#8aa963' },             // park grass
    d: { a: '#c6b291', b: '#d0bd9e' },             // gravel path
    q: { a: '#c3baa8', b: '#cec5b3' },             // quay stone
    w: { a: '#5089b0', b: '#5f9bc2', water: true }, // river
    // Somebody else's floor, at midnight, with the hall alight. Its own codes
    // rather than a darker 'f', because the apartment's boards are lit by the
    // morning and these are lit by a fire — same material, different world.
    F: { a: '#8a5f3c', b: '#9a6c46' },             // floorboards, by firelight
    T: { a: '#726a62', b: '#7f776f' }              // kitchen tile under smoke
  };
  E.GROUND = GROUND;

  /* ═══════════════════════════════════════════════════════════ the city

     Wilmington's east side, laid out rather than typed out: four avenues,
     three cross streets, fifteen blocks, the airfield along the north apron
     and the Christina along the south edge. In the real client this plate is
     a Tiled export (§10.3) — the builder below is the preview standing in
     for it, which is also why the whole city resizes by changing the numbers
     at the top rather than by retyping sixty-seven rows of characters.

     A lot with a `place` on it is a location: it gets a marker and a door on
     the pavement below it. A lot without one is somewhere nobody has written
     anything about, and that is deliberately most of the map — a city you
     can walk across has to be mostly buildings you cannot go into. */

  var COLS = 120;
  var AVES = [16, 42, 68, 94];   // vertical roads, five wide
  var APRON = 40;                // airfield tarmac runs from here east

  // every horizontal band, north to south: top row, depth, what it is
  var BANDS = [
    { y: 0,  h: 9, kind: 'apron' },
    { y: 9,  h: 2, kind: 'walk' },
    { y: 11, h: 5, kind: 'road' },
    { y: 16, h: 2, kind: 'walk' },
    { y: 18, h: 6, kind: 'block' },
    { y: 24, h: 2, kind: 'walk' },
    { y: 26, h: 5, kind: 'road' },
    { y: 31, h: 2, kind: 'walk' },
    { y: 33, h: 6, kind: 'block' },
    { y: 39, h: 2, kind: 'walk' },
    { y: 41, h: 5, kind: 'road' },
    { y: 46, h: 2, kind: 'walk' },
    { y: 48, h: 6, kind: 'block' },
    { y: 54, h: 2, kind: 'walk' },
    { y: 56, h: 6, kind: 'park' },
    { y: 62, h: 1, kind: 'plaza' },
    { y: 63, h: 3, kind: 'water' },
    { y: 66, h: 1, kind: 'void' }
  ];

  var BLOCK_Y = [18, 33, 48];    // the three bands you can build on

  function inAve(x) {
    for (var i = 0; i < AVES.length; i++) if (x >= AVES[i] && x < AVES[i] + 5) return true;
    return false;
  }

  // paint stripes either side of every avenue mouth
  function crossing(x) {
    for (var i = 0; i < AVES.length; i++) {
      var a = AVES[i];
      if ((x >= a - 3 && x < a) || (x >= a + 5 && x < a + 8)) return true;
    }
    return false;
  }

  function groundAt(b, i, x) {
    switch (b.kind) {
      case 'apron': return (x >= APRON || inAve(x)) ? 'r' : '#';
      case 'walk':  return inAve(x) ? 'r' : 's';
      case 'road':  return ((i === 0 || i === b.h - 1) && crossing(x)) ? 'k' : 'r';
      case 'block': return inAve(x) ? 'r' : '#';
      case 'park':  return (inAve(x) || i === 2 || i === 3) ? 'd' : 'g';
      case 'plaza': return 'p';
      case 'water': return 'w';
    }
    return '.';
  }

  function plate() {
    var rows = [];
    BANDS.forEach(function (b) {
      for (var i = 0; i < b.h; i++) {
        var row = '';
        for (var x = 0; x < COLS; x++) row += groundAt(b, i, x);
        rows.push(row);
      }
    });
    // The terminal and the hangars stand on open tarmac, which is walkable
    // ground. Stamp their footprints out or you can stroll through them.
    AIRSIDE.forEach(function (b) {
      for (var y = b.y; y < b.y + b.d; y++) {
        var r = rows[y].split('');
        for (var x = b.x; x < b.x + b.w; x++) r[x] = '#';
        rows[y] = r.join('');
      }
    });
    return rows;
  }

  // the runs of pavement between the avenues, west to east
  function segments() {
    var s = [], from = 0;
    for (var i = 0; i < AVES.length; i++) { s.push([from, AVES[i]]); from = AVES[i] + 5; }
    s.push([from, COLS]);
    return s;
  }

  /* ------------------------------------------------------------- lots */

  var FACADE = ['#d3c1a2', '#b5a08a', '#cdb694', '#c9b291', '#cbb79a', '#bda285'];
  var AWNING = ['#3f6b57', '#7a5a86', '#a8503f', '#5b7fa6', '#9c5f45'];

  /* Every named place on the map. The ones with a panel or a scene behind
     them are locations you can act on; the ones marked `soon` are the city
     the design calls for and the client has not built yet, which is worth
     showing on the map rather than hiding — the world is meant to be bigger
     than the screens are. */

  var HOME = { label: 'Apartment 3B', kind: 'portal', go: 'apartment', hint: 'home', type: 'flats',
    tip: 'Your room. The board, the closet, the computer and the bed are all up there.',
    color: '#c9b291', h: 148, balcony: true };

  var FENN = { label: 'Fenn Logistics', kind: 'portal', panel: 'work', hint: 'where you work', type: 'works',
    tip: 'Where you work. Dispatch coordinator, tier 2 &mdash; the shift runs whether you are standing in it or not.',
    color: '#8e9299', h: 104 };

  var CLINIC = { label: 'Rowan St Clinic', kind: 'portal', panel: 'health', hint: 'bloodwork, $180', type: 'clinic',
    tip: 'Bloodwork you currently have no way to read. $180, and it is closed until Monday.',
    color: '#d6cdb8', h: 104 };

  var GYM = { label: 'Ironworks Gym', kind: 'portal', panel: 'routines', hint: 'train, 45 minutes', type: 'gym',
    tip: 'Where the training routine actually happens. Mondays, Wednesdays and Saturdays, twelve of the last twelve.',
    color: '#9c5f45', h: 84 };

  var CORRIN = { label: 'Corrin Freight', kind: 'portal', panel: 'jobs', hint: 'they are hiring', type: 'works',
    tip: 'Night dispatch and warehouse floor, both posted. You did two years on that floor and nobody called you after.',
    color: '#9aa09c', h: 96 };

  var KV = { label: 'Kestrel &amp; Vane', kind: 'portal', panel: 'jobs', hint: 'they have met you', type: 'office',
    tip: 'They bill by the hour and everyone knows it. The desk job pays $5,900 and you do not live on the right side of the river for it.',
    color: '#c3c8cd', h: 214, rows: 9 };

  var VERONA = { label: 'Verona Diner', kind: 'sign',
    tip: 'Nadia works the bar. There is evening work going here if you ever ask her for it.',
    type: 'shop', color: '#cdb694', h: 78, rows: 2, blade: '#a8503f',
    store: { u: 0.18, w: 0.5, awning: '#a8503f', sign: '#e0a648' } };

  var BODEGA = { label: 'Bodega', kind: 'sign',
    tip: 'Open at five. The man behind the counter knows every name on this street, including yours.',
    type: 'shop', color: '#c9b291', h: 74, rows: 2, blade: '#3f6b57',
    store: { u: 0.18, w: 0.5, awning: '#3f6b57', sign: '#e8dcc4' } };

  var LIBRARY = { label: 'Lending library', kind: 'sign',
    tip: 'Free, quiet, and open until six. Nothing in it is going anywhere.',
    type: 'library', color: '#d8cdb4', h: 112 };

  var CITY_HALL = { label: 'City Hall', kind: 'soon',
    tip: 'The mayor&rsquo;s office. Rules get set in there and the rest of the city lives inside them &mdash; the closest thing this game has to a playbook you did not write.',
    type: 'civic', tower: true, color: '#dcd3b6', h: 150 };

  var COURT = { label: 'County Court', kind: 'soon',
    tip: 'Where a disagreement goes when both playbooks have run out. A hearing would be a challenge like any other: authored, staged, and committed to.',
    type: 'civic', color: '#cfc7ae', h: 136 };

  var BANK_MUTUAL = { label: 'Delaware Mutual', kind: 'soon',
    tip: 'Savings, and the loan that would move you across the river years before the port could. Debt is the other end of the money system and it is not modelled yet.',
    type: 'bank', color: '#c8c2ac', h: 170 };

  var BANK_CHRISTINA = { label: 'First Christina', kind: 'soon',
    tip: 'The branch you actually use. Your $4,212 is in here, doing nothing, which is the point.',
    type: 'bank', color: '#c5bfa9', h: 132 };

  var AIRPORT = { label: 'Wilmington Airport', kind: 'soon',
    tip: 'Two gates and a runway. Leaving the state is the ceiling made literal &mdash; you can see the aircraft from Rowan St and there is nothing on this map that pays for a seat.' };

  var HALCYON = { label: 'Halcyon House', kind: 'soon',
    tip: 'Rooms going from the first of the month, all of them dearer than 3B. halcyon_r is on the fourth floor and is 2,418th on the same board you are.',
    type: 'flats', balcony: true, color: '#bfae94', h: 196 };

  var ROWAN_COURT = { label: 'Rowan Court', kind: 'soon',
    tip: 'Where Marta lives, and Kez two doors down. Everyone you argue with is within four minutes of your own front door.',
    type: 'flats', balcony: true, color: '#c6b598', h: 158 };

  var TANNER_APTS = { label: 'Tanner Row Apartments', kind: 'soon',
    tip: 'Ninety flats and a waiting list. This is what the environment is: a few hundred people whose returns all decay at the same rate.',
    type: 'flats', balcony: true, color: '#c2b195', h: 204 };

  // Widths in each run add up to the block it sits in, so blocks come out
  // full and there are no gaps between buildings on a street.
  var LOTS = [
    // north block — civic, money, and the road out
    [
      [{ w: 5 }, { w: 6, place: HALCYON }, { w: 5 }],
      [{ w: 6 }, { w: 9, place: CITY_HALL }, { w: 6 }],
      [{ w: 5 }, { w: 8, place: COURT }, { w: 8 }],
      [{ w: 6 }, { w: 7, place: BANK_MUTUAL }, { w: 8 }],
      [{ w: 5 }, { w: 8, place: CORRIN }, { w: 8 }]
    ],
    // middle block — where you live, work and eat
    [
      [{ w: 5 }, { w: 6, place: HOME }, { w: 5 }],
      [{ w: 5, place: VERONA }, { w: 5 }, { w: 5, place: BODEGA }, { w: 6 }],
      [{ w: 6 }, { w: 7, place: FENN }, { w: 8 }],
      [{ w: 6 }, { w: 6, place: GYM }, { w: 9 }],
      [{ w: 7 }, { w: 7, place: KV }, { w: 7 }]
    ],
    // south block — the park side
    [
      [{ w: 5, place: ROWAN_COURT }, { w: 5 }, { w: 6 }],
      [{ w: 6 }, { w: 6, place: CLINIC }, { w: 9 }],
      [{ w: 7 }, { w: 7, place: LIBRARY }, { w: 7 }],
      [{ w: 6, place: BANK_CHRISTINA }, { w: 7 }, { w: 8 }],
      [{ w: 6 }, { w: 7, place: TANNER_APTS }, { w: 8 }]
    ]
  ];

  // A door sits on the pavement directly below its block, hard against the
  // wall: you have to turn off the pavement and step into it (§ app.js DOOR).
  function city() {
    var boxes = [], markers = [], segs = segments();

    LOTS.forEach(function (band, bi) {
      var y = BLOCK_Y[bi];
      band.forEach(function (run, si) {
        var x = segs[si][0];
        run.forEach(function (lot) {
          var p = lot.place;
          var n = E.hash(x * 1.7 + bi * 9.3, si * 5.1 + bi * 2.7);

          /* A lot nobody has named is still not "generic": a street of one
             building repeated is the thing that made the old map read flat.
             Unnamed lots draw from the same three ordinary kinds a real block
             is made of — flats, a small office, a shop with rooms over it. */
          var type = (p && p.type) || (n < 0.16 ? 'office' : n < 0.34 ? 'shop' : 'flats');
          var h = (p && p.h) || (type === 'shop' ? Math.round(66 + n * 46)
                                                 : Math.round(92 + n * 156));
          var box = {
            x: x, y: y, w: lot.w, d: 6, h: h, type: type,
            color: (p && p.color) || FACADE[Math.floor(n * FACADE.length)],
            cols: Math.max(3, Math.min(7, lot.w - 1)),
            rows: (p && p.rows) || Math.max(2, Math.round(h / 32)),
            seed: Math.round(n * 97)
          };
          if (p && p.balcony) box.balcony = true;
          else if (!p && type === 'flats' && n > 0.5) box.balcony = true;
          if (p && p.tower) box.tower = true;
          if (p && p.blade) box.blade = p.blade;
          if (p && p.store) box.store = p.store;
          else if (!p && type === 'shop') {
            box.store = { u: 0.18, w: 0.46, awning: AWNING[Math.floor(n * 100) % AWNING.length] };
            if (n > 0.24) box.blade = AWNING[Math.floor(n * 57) % AWNING.length];
          }
          boxes.push(box);

          if (p) {
            markers.push({
              x: x + lot.w / 2, y: y + 6.1,
              label: p.label, kind: p.kind, go: p.go, panel: p.panel,
              hint: p.hint, tip: p.tip
            });
          }
          x += lot.w;
        });
      });
    });

    return { boxes: boxes, markers: markers };
  }

  /* --------------------------------------------------------- airfield */

  // Low, wide and set back behind its own apron, so it reads as the edge of
  // the map rather than as one more block.
  // Set between the avenues rather than across them: the roads have to run
  // clean up to the apron or the north end of the map is a cul-de-sac.
  var AIRSIDE = [
    { x: 74, y: 2, w: 18, d: 5, h: 62, type: 'terminal', tower: true,
      color: '#c7cdd2', cols: 7, rows: 2, seed: 21, glass: 0.7 },
    { x: 47, y: 2, w: 12, d: 5, h: 54, type: 'works', color: '#b6bcb8', cols: 4, rows: 2, seed: 44 },
    { x: 100, y: 2, w: 14, d: 5, h: 48, type: 'works', color: '#bcc2c6', cols: 5, rows: 2, seed: 63 }
  ];

  /* -------------------------------------------------------- furniture */

  function furnish() {
    var f = [];
    var x;

    BANDS.forEach(function (b) {
      if (b.kind === 'walk') {
        // lamps along the kerb, trees against the buildings
        for (x = 3; x < COLS; x += 8) {
          if (inAve(x) || inAve(x + 1)) continue;
          f.push({ kind: 'lamp', x: x, y: b.y + b.h - 0.3 });
        }
        for (x = 6; x < COLS; x += 7) {
          if (inAve(x) || inAve(x + 1)) continue;
          f.push({ kind: 'tree', x: x, y: b.y + 0.6, lush: E.hash(x, b.y) > 0.55 });
        }
        for (x = 11; x < COLS; x += 19) {
          if (inAve(x)) continue;
          f.push({ kind: E.hash(x, b.y * 3) > 0.5 ? 'hydrant' : 'bags', x: x + 0.6, y: b.y + b.h - 0.6 });
        }
      }

      if (b.kind === 'road') {
        // parked and moving traffic, kept off the crossings
        for (x = 6; x < COLS; x += 13) {
          if (inAve(x) || inAve(x + 3)) continue;
          var n = E.hash(x * 2.3, b.y);
          f.push({ kind: 'car', x: x, y: b.y + 1.1,
                   color: ['#a05a4a', '#4d6478', '#c4b9a4', '#5f6d58', '#6b7d92'][Math.floor(n * 5)] });
          if (n > 0.5 && x + 7 < COLS) {
            f.push({ kind: 'car', x: x + 7, y: b.y + 3.1,
                     color: ['#6b7d92', '#8a7d6a', '#4f5b63'][Math.floor(n * 3)] });
          }
        }
        f.push({ kind: 'bus', x: b.y === 26 ? 30 : 76, y: b.y + 3.0, color: '#c98a3a' });
      }
    });

    // the park along the water
    for (x = 4; x < COLS - 3; x += 7) {
      f.push({ kind: 'tree', x: x, y: 60.4, lush: true });
      if (x % 14 === 4) f.push({ kind: 'tree', x: x + 3, y: 56.6, lush: true });
    }
    for (x = 8; x < COLS - 6; x += 11) {
      f.push({ kind: 'bench', x: x, y: 57.4 });
      f.push({ kind: 'planter', x: x + 5, y: 55.3 });
    }
    f.push({ kind: 'fountain', x: 80, y: 59.4 });
    f.push({ kind: 'railing', x: 0, y: 62.9, len: COLS - 1, axis: 'x', color: '#7c7466' });

    // the market, the stand and the stop, all on the last pavement
    f.push({ kind: 'stall', x: 52.0, y: 57.4, awning: '#a8503f' });
    f.push({ kind: 'stall', x: 54.2, y: 57.4, awning: '#3f6b57' });
    f.push({ kind: 'stall', x: 56.4, y: 57.4, awning: '#2f5f86' });
    f.push({ kind: 'newsstand', x: 45.0, y: 54.4 });
    f.push({ kind: 'busStop', x: 61.4, y: 54.3 });

    // airside
    f.push({ kind: 'plane', x: 60, y: 3.6 });
    f.push({ kind: 'plane', x: 95, y: 7.0, small: true });
    for (x = 44; x < 116; x += 9) f.push({ kind: 'bollard', x: x, y: 8.6 });

    return f;
  }

  /* ------------------------------------------------------------ people

     There is no crowd. Everybody standing on a pavement in this city is
     somebody: a name, a handle the boards print, a Karma, a level, a reason
     to be there and three things you can ask them. The whole cast is
     authored in people.js and handed out per scene by `E.cast` — this file
     owns where the city is, never who is in it.

     Anonymous figures were traffic, and traffic teaches the player that
     people are scenery. A smaller street where every single figure can be
     asked how they got their number is the opposite lesson, and it is the
     one §3.5 depends on. */

  /* ----------------------------------------------------------- street */

  var built = city();

  var street = {
    id: 'street',
    env: 'east',
    name: 'Tanner Row',
    where: 'Rowan St, Wilmington',
    clock: '11:20 AM',
    day: true,
    sun: { x: -0.30, y: 0.15, warm: '#ffd9a0', shade: '#4c5468' },
    sky: ['#5f9ad4', '#9cc4e4', '#dde5e1'],
    motes: 30,
    rows: plate(),
    boxes: built.boxes.concat(AIRSIDE),
    fixtures: furnish(),
    actors: E.cast('street'),
    player: { start: [8, 40.4], speed: 3.4, color: '#38384a', rim: '#fff0cf' },
    markers: built.markers.concat([
      { x: 83.0, y: 7.4, label: AIRPORT.label, kind: AIRPORT.kind, tip: AIRPORT.tip },
      { x: 45.8, y: 55.0, label: 'Morning paper', kind: 'use', panel: 'jobs', hint: 'what else is going',
        tip: 'Every posting in the city, in print. Nothing in it is recommended to you.' },
      { x: 54.4, y: 56.9, label: 'Saturday market', kind: 'use', panel: 'conflicts', hint: 'everyone is here',
        tip: 'Ilya, Marta and Kez are all here on a Saturday, and so is whatever is unresolved between you.' },
      { x: 62.6, y: 55.1, label: 'The DART 6', kind: 'use', panel: 'travel', hint: 'runs once a day',
        tip: 'The bus across the Christina. It runs once a day, and taking it is moving house rather than a trip.' },
      { x: 25.2, y: 40.6, kind: 'zone', radius: 5.2 }
    ]),
    zone: {
      title: 'Outside the diner',
      body: 'They&rsquo;re loud enough that people are crossing the street to avoid it. The woman catches your eye as you pass.',
      quote: '&ldquo;You&rsquo;re not going to believe what he just said to me.&rdquo;',
      accept: 'Stop',
      decline: 'Keep walking',
      foot: 'Walking on costs nothing. This is still here tomorrow.',
      panel: 'challenge'
    }
  };

  /* --------------------------------------------------------- apartment */

  var apartment = {
    id: 'apartment',
    env: 'east',
    name: 'Apartment 3B',
    where: 'Tanner Row',
    clock: '8:05 AM',
    day: true,
    interior: true,
    sun: { x: -0.20, y: 0.11, warm: '#ffe6bc', shade: '#5d5a6a' },
    sky: ['#1b1714', '#1b1714', '#1b1714'],
    motes: 80,
    rows: [
      '...............',
      '.#############.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '.fffffffffffff.',
      '...............',
      '...............'
    ],
    boxes: [
      { x: 1, y: 1, w: 13, d: 0.5, h: 150, color: '#d5c4aa', wall: true,
        openings: [{ u: 0.26, w: 0.18, kind: 'window' }, { u: 0.58, w: 0.16, kind: 'window' }] },
      { x: 0.5, y: 1, w: 0.5, d: 9, h: 150, color: '#cbba9f', wall: true, side: true,
        openings: [{ u: 0.55, w: 0.18, kind: 'door' }] },
      { x: 9.4, y: 2, w: 3.4, d: 2.2, h: 20, color: '#ab7d52', label: 'bed' },
      { x: 9.4, y: 2, w: 3.4, d: 0.7, h: 34, color: '#bd8f61', label: 'headboard' },
      { x: 2,  y: 2.1, w: 2.6, d: 1.1, h: 30, color: '#95704b', label: 'desk' },
      { x: 2.9, y: 3.4, w: 0.8, d: 0.8, h: 22, color: '#805f41', label: 'chair' },
      { x: 6.2, y: 1.9, w: 1.6, d: 0.7, h: 46, color: '#a17a54', label: 'shelf' },
      { x: 12.3, y: 6.4, w: 1.2, d: 1.2, h: 26, color: '#9a7450', label: 'side table' },
      { x: 1.1, y: 4.3, w: 1.2, d: 1.9, h: 64, color: '#8d6844', label: 'closet' }
    ],
    fixtures: [
      { kind: 'sunbeam', x: 4.7, y: 1.6, w: 2.3, d: 3.5 },
      { kind: 'sunbeam', x: 8.8, y: 1.6, w: 2.0, d: 3.0 },
      { kind: 'rug', x: 4.6, y: 4.4, w: 4.4, d: 3.2, color: '#b07f51', edge: '#c89562' },
      { kind: 'deskLamp', x: 3.0, y: 2.4 },
      { kind: 'floorLamp', x: 12.6, y: 6.3 },
      { kind: 'board', x: 6.0, y: 1.9 },
      { kind: 'screen', x: 2.4, y: 2.5 },
      { kind: 'mug', x: 3.9, y: 2.6 },
      { kind: 'plant', x: 1.7, y: 8.0 }
    ],
    actors: [],
    player: { path: [[7.2, 6.4], [7.2, 6.4]], start: [2.6, 6.2], speed: 2.7, color: '#38384a', rim: '#fff0cf' },
    /* Four things in a room, and each of them is a screen you would
       otherwise have opened from a menu. The board is the playbook — rules
       and routines are the same object, so they are the same wall. */
    markers: [
      { x: 6.0, y: 2.4, label: 'The board', kind: 'use', panel: 'playbook', hint: 'answers &amp; routines',
        tip: 'The standing questions and whatever you have answered them with, on the wall where you composed them. Three of them run without you.' },
      { x: 2.4, y: 2.7, label: 'The computer', kind: 'use', panel: 'jobs', hint: 'what else is going',
        tip: 'The postings board, from your desk. The same listings as the paper on Rowan St.' },
      { x: 2.3, y: 5.2, label: 'The closet', kind: 'use', panel: 'closet', hint: 'what you wear',
        tip: 'What you wear and what the boards call you. It is how you find yourself in a street where everybody else is somebody too.' },
      { x: 10.9, y: 3.0, label: 'Bed', kind: 'use', panel: 'health', hint: 'how you are sleeping',
        tip: '5.6 hours a night across the last fortnight, and every other thing your body is doing that you can currently measure.' },
      { x: 1.2, y: 6.0, label: 'Out to the street', kind: 'portal', go: 'street', hint: 'Rowan St',
        tip: 'Down three flights and out onto Rowan St.' }
    ]
  };

  /* -------------------------------------------------------- riverfront */

  var riverfront = {
    id: 'riverfront',
    env: 'quay',
    name: 'The Riverfront',
    where: 'Christina Riverwalk',
    clock: '3:40 PM',
    day: true,
    sun: { x: -0.28, y: 0.14, warm: '#ffe0ae', shade: '#4a5568' },
    sky: ['#4f92d2', '#93bfe2', '#d8e7ea'],
    motes: 20,
    rows: [
      '....................................',
      '....................................',
      '####################################',
      '####################################',
      '####################################',
      '####################################',
      '####################################',
      '####################################',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq',
      'wwwwwwwwwwwwwwqqqqwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwqqqqwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwqqqqwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwqqqqwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwqqqqwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww',
      '....................................'
    ],
    boxes: [
      { x: -1, y: 2, w: 7, d: 5, h: 330, type: 'office', color: '#c3cbd4', cols: 6, rows: 11, seed: 7, glass: 0.82 },
      { x: 6,  y: 2, w: 5, d: 5, h: 268, color: '#cbd2da', cols: 5, rows: 9, seed: 13, glass: 0.72 },
      { x: 11, y: 2, w: 6, d: 5, h: 392, type: 'office', color: '#bcc5cf', cols: 6, rows: 13, seed: 23, glass: 0.88 },
      { x: 17, y: 2, w: 5, d: 5, h: 288, color: '#c8cfd7', cols: 5, rows: 9, seed: 31, glass: 0.66 },
      { x: 22, y: 2, w: 5, d: 5, h: 350, type: 'office', color: '#bfc7d1', cols: 5, rows: 11, seed: 37, glass: 0.82 },
      { x: 27, y: 2, w: 4, d: 5, h: 226, color: '#d0d6dd', cols: 4, rows: 8, seed: 43, glass: 0.6 },
      { x: 31, y: 2, w: 6, d: 5, h: 410, type: 'office', color: '#b8c1cc', cols: 6, rows: 14, seed: 89, glass: 0.9 }
    ],
    fixtures: [
      { kind: 'tree', x: 2.4, y: 9.6, lush: true },
      { kind: 'tree', x: 10.4, y: 9.6, lush: true },
      { kind: 'tree', x: 18.4, y: 9.6, lush: true },
      { kind: 'tree', x: 26.4, y: 9.6, lush: true },
      { kind: 'tree', x: 32.4, y: 9.6, lush: true },
      { kind: 'lamp', x: 4.4, y: 12.4 },
      { kind: 'lamp', x: 12.4, y: 12.4 },
      { kind: 'lamp', x: 20.4, y: 12.4 },
      { kind: 'lamp', x: 28.4, y: 12.4 },
      { kind: 'railing', x: 0, y: 12.9, len: 13, axis: 'x', color: '#9aa0a6' },
      { kind: 'railing', x: 18, y: 12.9, len: 17, axis: 'x', color: '#9aa0a6' },
      { kind: 'bench', x: 8.5, y: 11.4 },
      { kind: 'bench', x: 16.5, y: 11.4 },
      { kind: 'bench', x: 24.5, y: 11.4 },
      { kind: 'bench', x: 30.5, y: 11.4 },
      { kind: 'planter', x: 6.6, y: 11.5 },
      { kind: 'planter', x: 22.6, y: 11.5 },
      { kind: 'planter', x: 28.6, y: 11.5 },
      { kind: 'boat', x: 12.0, y: 16.5 },
      { kind: 'boat', x: 18.6, y: 17.6 },
      { kind: 'newsstand', x: 20.6, y: 8.6 }
    ],
    actors: E.cast('riverfront'),
    player: { path: [[3, 11.4], [31, 11.4]], start: [16, 11.4], speed: 3.0, color: '#38384a', rim: '#fff0cf' },
    markers: [
      { x: 28.4, y: 8.2, label: 'The Arbour', kind: 'portal', go: 'quayroom', hint: 'your room is above it',
        tip: 'The pub with your room above it. Eleven days in and you still knock at the side door.' },
      { x: 14.2, y: 8.2, label: 'Kestrel &amp; Vane', kind: 'portal', panel: 'jobs', hint: 'they have met you',
        tip: 'They bill by the hour and everyone here knows it. The desk job pays $5,900 and they have met you.' },
      { x: 20.6, y: 8.3, label: 'Riverwalk kiosk', kind: 'use', panel: 'jobs', hint: 'the papers',
        tip: 'The papers, and a coffee you would not buy twice.' },
      { x: 15.8, y: 14.2, label: 'Old pier', kind: 'sign',
        tip: 'Two boats and no one on them. The only quiet on this side of the river.' },
      { x: 6.2, y: 10.4, kind: 'zone', radius: 4.4 },
      { x: 1.6, y: 11.6, label: 'The DART 6', kind: 'use', panel: 'travel', hint: 'runs once a day',
        tip: 'The bus back across the Christina. It runs once a day, and taking it is moving house rather than a trip.' }
    ],
    zone: {
      title: 'On the quay',
      body: 'The man from the fourth-floor office recognises you and slows down. He has an arrangement in mind, and he is very relaxed about it.',
      quote: '&ldquo;You did me a favour last week. Let me do you one back &mdash; you just have to not ask where it came from.&rdquo;',
      accept: 'Hear him out',
      decline: 'Say you&rsquo;re late',
      foot: 'The rules you built on Rowan St have an answer for this. It is the wrong one.',
      panel: 'travel'
    }
  };

  // The Riverfront room reuses the interior plate — same flat, different life. In
  // the real client this is its own painted location.
  var quayroom = JSON.parse(JSON.stringify(apartment));
  quayroom.id = 'quayroom';
  quayroom.env = 'quay';
  quayroom.name = 'Room above the Arbour';
  quayroom.where = 'The Riverfront';
  quayroom.clock = '7:40 AM';
  quayroom.boxes[0].color = '#d3d6da';
  quayroom.boxes[1].color = '#c9cdd2';
  quayroom.fixtures[2].color = '#8697a6';
  quayroom.fixtures[2].edge = '#9fadb9';
  quayroom.markers = [
    { x: 6.0, y: 2.4, label: 'The board', kind: 'use', panel: 'playbook', hint: 'one answer so far',
      tip: 'One answer carried across on day one, and three more that were worked out for a different set of people.' },
    { x: 2.4, y: 2.7, label: 'The computer', kind: 'use', panel: 'jobs', hint: 'what else is going',
      tip: 'The postings board. The one that pays $5,900 is four hundred metres from this desk.' },
    { x: 2.3, y: 5.2, label: 'The closet', kind: 'use', panel: 'closet', hint: 'what you wear',
      tip: 'What you wear and what the boards call you. Nobody on this side has learned either yet.' },
    { x: 10.9, y: 3.0, label: 'Bed', kind: 'use', panel: 'health', hint: 'how you are sleeping',
      tip: 'You sleep worse here and the numbers say so. Everything your body is doing that you can measure.' },
    { x: 1.2, y: 6.0, label: 'Out to the quay', kind: 'portal', go: 'riverfront', hint: 'the Riverwalk',
      tip: 'Down the side stairs and out onto the Christina Riverwalk.' }
  ];

  // An environment is a place you live, not a place you visit. You are in
  // exactly one, and moving between them is capped at once a day (§3.15).
  // `indoor` is what the A key goes to: the shortcut is to *your* room, and
  // which room that is depends on where you currently live.
  // How many answers are in force here is not written down: it is counted off
  // the playbook when the chip is drawn, so committing a rule changes what the
  // street says about you rather than leaving two numbers to drift apart.
  E.ENVIRONMENTS = {
    east: {
      id: 'east', name: 'Tanner Row',
      outdoor: 'street', indoor: 'apartment', indoorLabel: 'Apartment 3B'
    },
    quay: {
      id: 'quay', name: 'The Riverfront',
      outdoor: 'riverfront', indoor: 'quayroom', indoorLabel: 'Room above the Arbour'
    }
  };

  /* ------------------------------------------------------- the house fire

     Dolores's flat at the end of the landing, and the only location in this
     build that is not somewhere you can go. You are put here by a challenge
     and you are here for about six minutes; there is no way in from the city
     and there is no door out of it that a player takes. That is why it is not
     a city place — `internal/city` is where there is *to be*, and this is
     somewhere that happens to you.

     It is deliberately not the apartment with the lights off. Your flat is one
     square room with four things in it and nothing on the walls, because you
     moved in with a bag. This is an L — a long room that turns a corner into a
     galley kitchen — and it is full, because somebody has been in it for thirty
     years: a china cabinet, a sideboard with drawers, a dining table nobody
     eats at any more, an armchair pointed at a window rather than at a screen.
     The point of the layout is that you have to cross it, twice, deciding.

     The fire is in the hall, which is where the way out is. Nothing in this
     scene animates — the plate is baked once (#39) — so the fire has to read as
     dangerous while standing perfectly still, which is what the ember glow on
     the boards and the smoke coming down to head height are doing. */

  var housefire = {
    id: 'housefire',
    env: 'east',
    name: 'The flat at the end',
    where: 'Tanner Row',
    clock: '11:52 PM',
    day: false,
    interior: true,
    /* The key light is the hall doorway rather than the sky: low, hard and
       from the west, so everything in the room throws a long shadow away from
       the only way out. */
    sun: { x: 0.46, y: -0.14, warm: '#ffb15c', shade: '#2b1a1e' },
    sky: ['#160c09', '#1e100c', '#2a1510'],
    motes: 240,
    rows: [
      '.....................',
      '.###################.',
      '.FFFFFFFFFF#########.',
      '.FFFFFFFFFF#########.',
      '.FFFFFFFFFF#########.',
      '.FFFFFFFFFF#########.',
      '.FFFFFFFFFFFFFFFF###.',
      '.FFFFFFFFFFFFFFFF###.',
      '.FFFFFFFFFFFFFFFF###.',
      '.FFFFFFFFTTTTTTTT###.',
      '.FFFFFFFFTTTTTTTT###.',
      '.FFFFFFFFTTTTTTTT###.',
      '.###################.',
      '.....................'
    ],
    boxes: [
      /* The shell. Two north walls because the room steps, and a west wall
         with the front door in it — which is the doorway the fire is in. */
      { x: 1, y: 1, w: 10, d: 0.5, h: 150, color: '#7a6350', wall: true,
        openings: [{ u: 0.30, w: 0.16, kind: 'window' }, { u: 0.68, w: 0.14, kind: 'window' }] },
      { x: 11, y: 6, w: 6, d: 0.5, h: 150, color: '#755e4c', wall: true,
        openings: [{ u: 0.46, w: 0.18, kind: 'window' }] },
      { x: 0.5, y: 1, w: 0.5, d: 11, h: 150, color: '#6d5745', wall: true, side: true,
        openings: [{ u: 0.72, w: 0.20, kind: 'door' }] },
      { x: 10.5, y: 2, w: 0.5, d: 4, h: 150, color: '#6a5443', wall: true, side: true },

      /* Thirty years of somebody's things. */
      { x: 1.5, y: 2.2, w: 3.4, d: 1.0, h: 32, color: '#5d3f28', label: 'sideboard' },
      { x: 5.6, y: 2.1, w: 1.9, d: 0.9, h: 76, color: '#523522', label: 'china cabinet' },
      { x: 8.2, y: 2.4, w: 1.7, d: 1.6, h: 30, color: '#6b3f3a', label: 'armchair' },
      { x: 2.6, y: 6.4, w: 3.8, d: 2.1, h: 26, color: '#67462c', label: 'dining table' },
      { x: 1.8, y: 6.8, w: 0.8, d: 0.8, h: 22, color: '#573a24', label: 'chair' },
      { x: 6.6, y: 6.8, w: 0.8, d: 0.8, h: 22, color: '#573a24', label: 'chair' },
      { x: 12.9, y: 6.4, w: 2.6, d: 0.8, h: 70, color: '#543823', label: 'bookcase' },
      { x: 9.4, y: 9.4, w: 6.2, d: 1.0, h: 34, color: '#5a544c', label: 'counter' },
      { x: 15.0, y: 10.6, w: 1.3, d: 1.2, h: 58, color: '#6c665e', label: 'fridge' },
      { x: 1.4, y: 9.6, w: 1.5, d: 0.8, h: 28, color: '#5d3f28', label: 'hall table' }
    ],
    fixtures: [
      /* The fire, in the doorway and along the wall it came in through. */
      { kind: 'emberGlow', x: 1.6, y: 9.4, r: 7 },
      { kind: 'emberGlow', x: 1.4, y: 4.0, r: 5 },
      /* Two more further in, weak. Not because a fire in the hall would light
         the far corner — it would not — but because the far corner has three
         of the nine things in it, and a room you cannot choose in is not a
         choice. The fiction bends here and it should. */
      { kind: 'emberGlow', x: 7.6, y: 6.4, r: 8 },
      { kind: 'emberGlow', x: 13.4, y: 8.6, r: 7 },
      { kind: 'flames', x: 1.15, y: 9.3, w: 2.0, h: 150 },
      { kind: 'flames', x: 1.15, y: 8.1, w: 1.3, h: 96 },
      { kind: 'flames', x: 1.15, y: 3.4, w: 1.1, h: 74 },
      { kind: 'flames', x: 5.7, y: 2.1, w: 1.4, h: 88 },

      { kind: 'rug', x: 2.2, y: 5.9, w: 5.0, d: 3.4, color: '#6d3f3a', edge: '#8a5049' },
      { kind: 'plant', x: 9.8, y: 6.6 },
      { kind: 'floorLamp', x: 9.9, y: 2.2 },
      { kind: 'mug', x: 4.2, y: 6.9 },
      { kind: 'bags', x: 13.2, y: 10.2 },

      /* Over the lot, coming down. Two of them, because the room is an L and
         one rectangle of smoke would hang in the void outside the short arm. */
      { kind: 'smoke', x: 1, y: 2, w: 10, d: 10, h: 150 },
      { kind: 'smoke', x: 11, y: 6, w: 6, d: 6, h: 150 }
    ],
    actors: [],
    /* You come in through the hall door and you are standing in it. */
    player: { path: [[3.4, 8.6], [3.4, 8.6]], start: [2.0, 9.6], speed: 2.7, color: '#38384a', rim: '#ffd9a0' },
    /* The nine things, where they actually are. These are what the salvage
       challenge's items are named after — the marker key is the item key in
       content/challenges/house_fire/a/salvage.yaml, so walking up to the dog
       and taking it is the same commitment as pressing it in the panel. */
    markers: [
      { x: 4.5, y: 5.0, label: 'The dog', kind: 'use', item: 'dog', hint: 'on the boards',
        tip: 'He has come out from under the table and he will not go further than this.' },
      { x: 7.2, y: 4.8, label: 'The urn', kind: 'use', item: 'urn', hint: 'on the floor',
        tip: 'Sealed, with a name taped to the lid.' },
      { x: 2.5, y: 5.2, label: 'The photographs', kind: 'use', item: 'photos', hint: 'spilled',
        tip: 'A shoebox of loose prints. There is no other copy of any of it.' },
      { x: 9.0, y: 5.2, label: 'The papers', kind: 'use', item: 'papers', hint: 'in a folder',
        tip: 'A ziplock folder somebody put up on purpose, years ago, for tonight.' },
      { x: 8.5, y: 7.0, label: 'The phone', kind: 'use', item: 'phone', hint: 'still ringing',
        tip: 'Face down, still plugged into the wall.' },
      { x: 11.5, y: 7.2, label: 'The wallet', kind: 'use', item: 'wallet', hint: 'dropped',
        tip: 'Worn, and thick with cards rather than with money.' },
      { x: 14.0, y: 8.4, label: 'The roll of notes', kind: 'use', item: 'cash', hint: 'from a drawer',
        tip: 'A drawer somebody left open on their way out.' },
      { x: 4.0, y: 10.5, label: 'The flowers', kind: 'use', item: 'flowers', hint: 'out of the jug',
        tip: 'Cut flowers, water and all.' },
      { x: 6.5, y: 10.5, label: 'The armchair', kind: 'use', item: 'chair', hint: 'by the window',
        tip: 'The only good thing in the room, and it weighs what a chair weighs.' }
    ]
  };

  E.SCENES = {
    street: street, apartment: apartment, riverfront: riverfront, quayroom: quayroom,
    housefire: housefire
  };

})(window.ETM);
