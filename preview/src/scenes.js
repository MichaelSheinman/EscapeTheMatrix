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
    w: { a: '#5089b0', b: '#5f9bc2', water: true } // river
  };
  E.GROUND = GROUND;

  /* ------------------------------------------------------------ street */

  var street = {
    id: 'street',
    env: 'east',
    name: 'East Side',
    where: 'Rowan St',
    clock: '11:20 AM',
    day: true,
    sun: { x: -0.30, y: 0.15, warm: '#ffd9a0', shade: '#4c5468' },
    sky: ['#5f9ad4', '#9cc4e4', '#dde5e1'],
    motes: 30,
    // Two avenues, two cross streets, four blocks and a park along the river.
    // The near block is kept low so you can still read the far avenue over it.
    rows: [
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'rrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrr',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      '################rrrrr###################rrrrr###############',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'rrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrrr',
      'rrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrrrkkkrrrrrkkkrrrrrrrrrrrr',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'ssssssssssssssssrrrrrsssssssssssssssssssrrrrrsssssssssssssss',
      'ggggggggggggggggdddddgggggggggggggggggggdddddggggggggggggggg',
      'ggggggggggggggggdddddgggggggggggggggggggdddddggggggggggggggg',
      'ggggggdddddddddddddddddddddddddddddddddddddddddddddddggggggg',
      'ggggggdddddddddddddddddddddddddddddddddddddddddddddddggggggg',
      'ggggggggggggggggdddddgggggggggggggggggggdddddggggggggggggggg',
      'ggggggggggggggggdddddgggggggggggggggggggdddddggggggggggggggg',
      'pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp',
      'wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww',
      'wwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwwww',
      '............................................................'
    ],
    boxes: [
      { x: -2, y: 15, w: 6, d: 6, h: 137, color: '#d3c1a2', cols: 6, rows: 5, seed: 5, store: { u: 0.18, w: 0.46, awning: '#3f6b57', sign: '#e0a648' } },
      { x: 4, y: 15, w: 5, d: 6, h: 136, color: '#b5a08a', cols: 5, rows: 5, seed: 46 },
      { x: 9, y: 15, w: 4, d: 6, h: 120, color: '#cdb694', cols: 4, rows: 4, seed: 42 },
      { x: 13, y: 15, w: 3, d: 6, h: 140, color: '#b5a08a', cols: 3, rows: 5, seed: 14, store: { u: 0.18, w: 0.46, awning: '#7a5a86', sign: '#e8dcc4' } },
      { x: 21, y: 15, w: 4, d: 6, h: 132, color: '#c9b291', cols: 4, rows: 5, seed: 12 },
      { x: 25, y: 15, w: 6, d: 6, h: 101, color: '#cbb79a', cols: 6, rows: 3, seed: 95 },
      { x: 31, y: 15, w: 5, d: 6, h: 130, color: '#b5a08a', cols: 5, rows: 5, seed: 94, store: { u: 0.18, w: 0.46, awning: '#3f6b57', sign: '#e0a648' } },
      { x: 36, y: 15, w: 4, d: 6, h: 135, color: '#cbb79a', cols: 4, rows: 5, seed: 87 },
      { x: 45, y: 15, w: 6, d: 6, h: 104, color: '#b5a08a', cols: 6, rows: 4, seed: 38 },
      { x: 51, y: 15, w: 6, d: 6, h: 119, color: '#bda285', cols: 6, rows: 4, seed: 83, store: { u: 0.18, w: 0.46, awning: '#5b7fa6', sign: '#c6d4e2' } },
      { x: 57, y: 15, w: 4, d: 6, h: 124, color: '#cdb694', cols: 4, rows: 4, seed: 10 },
      { x: -2, y: 0, w: 5, d: 6, h: 181, color: '#d3c1a2', cols: 5, rows: 6, seed: 13, store: { u: 0.18, w: 0.46, awning: '#7a5a86', sign: '#e8dcc4' } },
      { x: 3, y: 0, w: 6, d: 6, h: 185, color: '#d3c1a2', cols: 6, rows: 7, seed: 5 },
      { x: 9, y: 0, w: 5, d: 6, h: 202, color: '#b5a08a', cols: 5, rows: 7, seed: 46 },
      { x: 21, y: 0, w: 4, d: 6, h: 204, color: '#cdb694', cols: 4, rows: 7, seed: 42, store: { u: 0.18, w: 0.46, awning: '#a8503f' } },
      { x: 25, y: 0, w: 6, d: 6, h: 218, color: '#b5a08a', cols: 6, rows: 8, seed: 14 },
      { x: 31, y: 0, w: 4, d: 6, h: 294, color: '#c9b291', cols: 4, rows: 11, seed: 12 },
      { x: 35, y: 0, w: 5, d: 6, h: 215, color: '#cbb79a', cols: 5, rows: 8, seed: 95, store: { u: 0.18, w: 0.46, awning: '#3f6b57', sign: '#e0a648' } },
      { x: 45, y: 0, w: 5, d: 6, h: 178, color: '#b5a08a', cols: 5, rows: 6, seed: 94 },
      { x: 50, y: 0, w: 4, d: 6, h: 219, color: '#cbb79a', cols: 4, rows: 8, seed: 87 },
      { x: 54, y: 0, w: 6, d: 6, h: 206, color: '#b5a08a', cols: 6, rows: 7, seed: 38, store: { u: 0.18, w: 0.46, awning: '#9c5f45' } },
      { x: -6, y: -11, w: 8, d: 4, h: 320, color: '#a8bacc', cols: 5, rows: 9, seed: 61, far: true },
      { x: 6, y: -11, w: 8, d: 4, h: 400, color: '#a8bacc', cols: 5, rows: 11, seed: 67, far: true },
      { x: 20, y: -11, w: 8, d: 4, h: 300, color: '#a8bacc', cols: 5, rows: 8, seed: 73, far: true },
      { x: 33, y: -11, w: 8, d: 4, h: 430, color: '#a8bacc', cols: 5, rows: 12, seed: 79, far: true },
      { x: 47, y: -11, w: 8, d: 4, h: 340, color: '#a8bacc', cols: 5, rows: 10, seed: 85, far: true },
      { x: 58, y: -11, w: 8, d: 4, h: 300, color: '#a8bacc', cols: 5, rows: 8, seed: 91, far: true }
    ],
    fixtures: [
      { kind: 'tree', x: 2.0, y: 7.6 },
      { kind: 'tree', x: 2.0, y: 22.6 },
      { kind: 'tree', x: 8.0, y: 7.6 },
      { kind: 'tree', x: 8.0, y: 22.6 },
      { kind: 'tree', x: 14.0, y: 7.6 },
      { kind: 'tree', x: 14.0, y: 22.6 },
      { kind: 'tree', x: 26.0, y: 7.6 },
      { kind: 'tree', x: 26.0, y: 22.6 },
      { kind: 'tree', x: 32.0, y: 7.6 },
      { kind: 'tree', x: 32.0, y: 22.6 },
      { kind: 'tree', x: 38.0, y: 7.6 },
      { kind: 'tree', x: 38.0, y: 22.6 },
      { kind: 'tree', x: 50.0, y: 7.6 },
      { kind: 'tree', x: 50.0, y: 22.6 },
      { kind: 'tree', x: 56.0, y: 7.6 },
      { kind: 'tree', x: 56.0, y: 22.6 },
      { kind: 'lamp', x: 5.0, y: 7.7 },
      { kind: 'lamp', x: 5.0, y: 13.7 },
      { kind: 'lamp', x: 5.0, y: 22.7 },
      { kind: 'lamp', x: 5.0, y: 28.7 },
      { kind: 'lamp', x: 13.0, y: 7.7 },
      { kind: 'lamp', x: 13.0, y: 13.7 },
      { kind: 'lamp', x: 13.0, y: 22.7 },
      { kind: 'lamp', x: 13.0, y: 28.7 },
      { kind: 'lamp', x: 21.0, y: 7.7 },
      { kind: 'lamp', x: 21.0, y: 13.7 },
      { kind: 'lamp', x: 21.0, y: 22.7 },
      { kind: 'lamp', x: 21.0, y: 28.7 },
      { kind: 'lamp', x: 29.0, y: 7.7 },
      { kind: 'lamp', x: 29.0, y: 13.7 },
      { kind: 'lamp', x: 29.0, y: 22.7 },
      { kind: 'lamp', x: 29.0, y: 28.7 },
      { kind: 'lamp', x: 37.0, y: 7.7 },
      { kind: 'lamp', x: 37.0, y: 13.7 },
      { kind: 'lamp', x: 37.0, y: 22.7 },
      { kind: 'lamp', x: 37.0, y: 28.7 },
      { kind: 'lamp', x: 45.0, y: 7.7 },
      { kind: 'lamp', x: 45.0, y: 13.7 },
      { kind: 'lamp', x: 45.0, y: 22.7 },
      { kind: 'lamp', x: 45.0, y: 28.7 },
      { kind: 'lamp', x: 53.0, y: 7.7 },
      { kind: 'lamp', x: 53.0, y: 13.7 },
      { kind: 'lamp', x: 53.0, y: 22.7 },
      { kind: 'lamp', x: 53.0, y: 28.7 },
      { kind: 'car', x: 9.0, y: 10.4, color: '#a05a4a' },
      { kind: 'car', x: 27.4, y: 10.4, color: '#4d6478' },
      { kind: 'car', x: 49.2, y: 10.4, color: '#c4b9a4' },
      { kind: 'car', x: 22.6, y: 25.4, color: '#5f6d58' },
      { kind: 'car', x: 46.0, y: 25.4, color: '#6b7d92' },
      { kind: 'bus', x: 33.0, y: 25.1, color: '#c98a3a' },
      { kind: 'busStop', x: 34.0, y: 28.3 },
      { kind: 'stall', x: 24.0, y: 28.6, awning: '#a8503f' },
      { kind: 'stall', x: 26.2, y: 28.6, awning: '#3f6b57' },
      { kind: 'stall', x: 28.4, y: 28.6, awning: '#2f5f86' },
      { kind: 'fountain', x: 33.0, y: 33.4 },
      { kind: 'tree', x: 9.0, y: 34.4, lush: true },
      { kind: 'tree', x: 14.0, y: 34.4, lush: true },
      { kind: 'tree', x: 22.0, y: 34.4, lush: true },
      { kind: 'tree', x: 30.0, y: 34.4, lush: true },
      { kind: 'tree', x: 38.0, y: 34.4, lush: true },
      { kind: 'tree', x: 46.0, y: 34.4, lush: true },
      { kind: 'tree', x: 53.0, y: 34.4, lush: true },
      { kind: 'bench', x: 11.0, y: 31.4 },
      { kind: 'bench', x: 20.0, y: 31.4 },
      { kind: 'bench', x: 36.0, y: 31.4 },
      { kind: 'bench', x: 44.0, y: 31.4 },
      { kind: 'bench', x: 51.0, y: 31.4 },
      { kind: 'planter', x: 7.0, y: 29.3 },
      { kind: 'planter', x: 17.0, y: 29.3 },
      { kind: 'planter', x: 27.0, y: 29.3 },
      { kind: 'planter', x: 41.0, y: 29.3 },
      { kind: 'planter', x: 55.0, y: 29.3 },
      { kind: 'hydrant', x: 12.6, y: 22.8 },
      { kind: 'hydrant', x: 31.6, y: 22.8 },
      { kind: 'hydrant', x: 50.6, y: 22.8 },
      { kind: 'bags', x: 24.5, y: 7.6 },
      { kind: 'bags', x: 43.5, y: 7.6 },
      { kind: 'bollard', x: 15.6, y: 22.9 },
      { kind: 'bollard', x: 21.4, y: 22.9 },
      { kind: 'bollard', x: 39.6, y: 22.9 },
      { kind: 'bollard', x: 45.4, y: 22.9 },
      { kind: 'railing', x: 0, y: 36.4, len: 59, axis: 'x', color: '#7c7466' }
    ],
    actors: [
      { id: 'npcA', x: 29.6, y: 23.4, color: '#3f4150', gesture: true, phase: 0.4 },
      { id: 'npcB', x: 30.7, y: 23.8, color: '#6b4536', gesture: true, phase: 2.1 },
      { id: 'walkA', path: [[56, 21.6], [6, 21.6]], speed: 1.5, color: '#4a4a58', phase: 1.2, scale: 0.95 },
      { id: 'walkB', path: [[6, 28.6], [56, 28.6]], speed: 1.2, color: '#6a4f3c', phase: 2.7, scale: 0.93 },
      { id: 'walkC', path: [[52, 6.8], [24, 6.8]], speed: 1.7, color: '#4c5f56', phase: 0.8, scale: 0.97 },
      { id: 'walkD', path: [[8, 13.8], [50, 13.8]], speed: 1.4, color: '#5a4a62', phase: 3.1, scale: 0.92 },
      { id: 'crossA', path: [[18, 2], [18, 27]], speed: 1.6, color: '#48505e', phase: 1.7, scale: 0.94 },
      { id: 'crossB', path: [[42.5, 27], [42.5, 2]], speed: 1.3, color: '#6d5240', phase: 0.3, scale: 0.95 },
      { id: 'jogger', path: [[8, 32.6], [50, 32.6]], speed: 3.2, color: '#54525c', phase: 0.2, scale: 0.96 },
      { id: 'parkSit', x: 20.6, y: 31.2, color: '#7a5340', phase: 1.9, scale: 0.94 },
      { id: 'shopper', x: 26.4, y: 29.6, color: '#455a6e', gesture: true, phase: 2.4, scale: 0.95 },
      { id: 'stallHand', x: 24.2, y: 28.0, color: '#6d5240', phase: 0.6, scale: 0.96 },
      { id: 'waiting', x: 34.6, y: 27.8, color: '#3d4a5c', phase: 4.2, scale: 0.95 },
      { id: 'smoker', x: 13.4, y: 21.7, color: '#40514c', phase: 3.3, scale: 0.98 }
    ],
    player: { path: [[8, 21.6], [54, 21.6]], start: [30, 21.6], speed: 2.2, color: '#38384a', rim: '#fff0cf' },
    markers: [
      { x: 12.0, y: 21.0, label: 'Apartment 3B', kind: 'portal', go: 'apartment' },
      { x: 29.0, y: 21.0, label: 'Verona Diner', kind: 'portal' },
      { x: 47.0, y: 21.0, label: 'Rowan St Clinic', kind: 'portal', panel: 'health' },
      { x: 24.0, y: 14.4, label: 'Depot yard', kind: 'portal', panel: 'work' },
      { x: 8.0, y: 6.4, label: 'Bodega', kind: 'portal' },
      { x: 36.0, y: 6.4, label: 'Lending library', kind: 'portal' },
      { x: 52.0, y: 14.4, label: 'Kestrel &amp; Vane', kind: 'portal' },
      { x: 26.2, y: 28.0, label: 'Saturday market', kind: 'use', panel: 'inbox' },
      { x: 34.0, y: 27.8, label: 'The 14 line', kind: 'use', panel: 'travel' },
      { x: 33.0, y: 32.6, label: 'Rowan Park', kind: 'use', panel: 'conflicts' },
      { x: 30.2, y: 23.6, kind: 'zone', radius: 5.4 }
    ],
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
    where: 'East Side',
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
      { x: 12.3, y: 6.4, w: 1.2, d: 1.2, h: 26, color: '#9a7450', label: 'side table' }
    ],
    fixtures: [
      { kind: 'sunbeam', x: 4.7, y: 1.6, w: 2.3, d: 3.5 },
      { kind: 'sunbeam', x: 8.8, y: 1.6, w: 2.0, d: 3.0 },
      { kind: 'rug', x: 4.6, y: 4.4, w: 4.4, d: 3.2, color: '#b07f51', edge: '#c89562' },
      { kind: 'deskLamp', x: 3.0, y: 2.4 },
      { kind: 'floorLamp', x: 12.6, y: 6.3 },
      { kind: 'board', x: 6.0, y: 1.9 },
      { kind: 'mug', x: 3.7, y: 2.5 },
      { kind: 'plant', x: 1.7, y: 8.0 }
    ],
    actors: [],
    player: { path: [[7.2, 6.4], [7.2, 6.4]], speed: 0, color: '#38384a', rim: '#fff0cf', idle: true },
    markers: [
      { x: 6.0, y: 2.2, label: 'Routines', kind: 'use', panel: 'routines' },
      { x: 2.6, y: 2.6, label: 'Playbook', kind: 'use', panel: 'playbook' },
      { x: 10.9, y: 3.0, label: 'Sleep', kind: 'use', panel: 'health' },
      { x: 1.2, y: 6.0, label: 'Out to the street', kind: 'portal', go: 'street' }
    ]
  };

  /* -------------------------------------------------------- riverfront */

  var riverfront = {
    id: 'riverfront',
    env: 'quay',
    name: 'Riverfront',
    where: 'Kestrel Quay',
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
      { x: -1, y: 2, w: 7, d: 5, h: 330, color: '#c3cbd4', cols: 6, rows: 11, seed: 7, glass: 0.82 },
      { x: 6,  y: 2, w: 5, d: 5, h: 268, color: '#cbd2da', cols: 5, rows: 9, seed: 13, glass: 0.72 },
      { x: 11, y: 2, w: 6, d: 5, h: 392, color: '#bcc5cf', cols: 6, rows: 13, seed: 23, glass: 0.88 },
      { x: 17, y: 2, w: 5, d: 5, h: 288, color: '#c8cfd7', cols: 5, rows: 9, seed: 31, glass: 0.66 },
      { x: 22, y: 2, w: 5, d: 5, h: 350, color: '#bfc7d1', cols: 5, rows: 11, seed: 37, glass: 0.82 },
      { x: 27, y: 2, w: 4, d: 5, h: 226, color: '#d0d6dd', cols: 4, rows: 8, seed: 43, glass: 0.6 },
      { x: 31, y: 2, w: 6, d: 5, h: 410, color: '#b8c1cc', cols: 6, rows: 14, seed: 89, glass: 0.9 }
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
      { kind: 'boat', x: 18.6, y: 17.6 }
    ],
    actors: [
      { id: 'pair1', x: 10.2, y: 10.6, color: '#3f4756', phase: 0.9 },
      { id: 'pair2', x: 11.2, y: 10.9, color: '#57525e', gesture: true, phase: 2.6 },
      { id: 'jogger', path: [[1, 9.4], [34, 9.4]], speed: 3.1, color: '#5c5a64', phase: 0.2, scale: 0.96 },
      { id: 'suitA', path: [[33, 10.2], [18, 10.2]], speed: 1.6, color: '#39404e', phase: 1.4, scale: 0.97 },
      { id: 'suitB', x: 24.4, y: 11.2, color: '#4a4f5c', gesture: true, phase: 3.0 },
      { id: 'onPier', x: 15.6, y: 15.4, color: '#5d5460', phase: 2.2, scale: 0.94 }
    ],
    player: { path: [[3, 11.4], [31, 11.4]], start: [16, 11.4], speed: 2.1, color: '#38384a', rim: '#fff0cf' },
    markers: [
      { x: 14.2, y: 7.0, label: 'Kestrel &amp; Vane', kind: 'portal' },
      { x: 28.4, y: 7.0, label: 'The Arbour', kind: 'portal' },
      { x: 15.8, y: 14.2, label: 'Old pier', kind: 'use', panel: 'travel' },
      { x: 6.2, y: 10.4, kind: 'zone', radius: 4.4 },
      { x: 1.6, y: 11.6, label: 'The 14 line', kind: 'use', panel: 'travel' }
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

  // The Quay room reuses the interior plate — same flat, different life. In
  // the real client this is its own painted location.
  var quayroom = JSON.parse(JSON.stringify(apartment));
  quayroom.id = 'quayroom';
  quayroom.env = 'quay';
  quayroom.name = 'Room above the Arbour';
  quayroom.where = 'Kestrel Quay';
  quayroom.clock = '7:40 AM';
  quayroom.boxes[0].color = '#d3d6da';
  quayroom.boxes[1].color = '#c9cdd2';
  quayroom.fixtures[2].color = '#8697a6';
  quayroom.fixtures[2].edge = '#9fadb9';
  quayroom.markers = [
    { x: 6.0, y: 2.2, label: 'Routines', kind: 'use', panel: 'routines' },
    { x: 2.6, y: 2.6, label: 'Playbook', kind: 'use', panel: 'playbook' },
    { x: 1.2, y: 6.0, label: 'Out to the quay', kind: 'portal', go: 'riverfront' }
  ];

  // An environment is a place you live, not a place you visit. You are in
  // exactly one, and moving between them is capped at once a day (§3.15).
  E.ENVIRONMENTS = {
    east: {
      id: 'east', name: 'East Side', rules: '7 rules', ruleTone: '',
      outdoor: 'street',
      places: [
        { id: 'street', label: 'Rowan St', glyph: '\u25c7' },
        { id: 'apartment', label: 'Apartment 3B', glyph: '\u2302' }
      ]
    },
    quay: {
      id: 'quay', name: 'Riverfront', rules: '1 rule', ruleTone: 'warn-text',
      outdoor: 'riverfront',
      places: [
        { id: 'riverfront', label: 'Kestrel Quay', glyph: '\u224b' },
        { id: 'quayroom', label: 'Room above the Arbour', glyph: '\u2302' }
      ]
    }
  };

  E.SCENES = { street: street, apartment: apartment, riverfront: riverfront, quayroom: quayroom };

})(window.ETM);
