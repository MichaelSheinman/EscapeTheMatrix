/* Escape the Matrix — client shell.
   One canvas that never unmounts, edge to edge, with no rail and no top bar
   around it. Panels dim the world rather than replacing it (technical.md
   §10.1). No game logic lives here: the character walks, the sun casts, and
   the panels are fixed content.

   The navigation model, which is the thing this file is actually about:

     · You walk. Arrows drive the character, clicking the ground routes them.
     · A door opens because you stepped into it, not because you clicked a
       menu item. Work is a building. Other work is the paper on the stand or
       the computer at home. A challenge happens where it happens.
     · Every screen that is reachable on foot also has a key, because the
       shortcut is a convenience for a player who already knows where the
       thing is — never the only way in, and never the first way you find it.
     · What is left over floats: your standing on the right, the phone in the
       corner, where you are in the top left, the keys along the bottom. */

import {
  Marker, WorldCard, StatPod, StatPods, PlaceChip, EnvChip,
  PhoneButton, HelpButton, Toast, ActionPrompt, KeyBar, KeyCard, NamePlate,
} from '../../ui/index.js';

(function (E) {
  'use strict';

  var ELEV = { s: 6, p: 5, q: 6, g: 7, d: 6, r: 0, k: 0, f: 0, t: 0, w: -5 };
  var WALKABLE = { street: 'srkpgd', apartment: 'f', quayroom: 'f', riverfront: 'q' };

  /* A doorway is a box, not a circle: you trigger it by stepping in towards
     the wall, so walking the length of a pavement past every shopfront in
     the row never opens one. REACH is how far the O key sees. */
  var DOOR = { x: 0.72, y: 0.6 };
  var REACH = 2.2;

  var cvs, ctx, dpr = 1, W = 0, H = 0, zoom = 1, baseZoom = 1, userZoom = 1;
  var scene, sceneId, actors = [], player, emissive = [];
  var cam = { x: 0, y: 0, set: false, free: false };
  var drag = null, walk = {}, suppressClick = false;
  var home = 'east', movedToday = false;   // you live in exactly one environment
  var t = 0, last = 0, fadeFrom = -1e9, quality = 'high';
  var motes = null, markerEls = [], card, cardZone = null, cardVisible = false;
  var promptEl, plateEl, near = null, delivered = 0, unread = 1, openName = null;

  /* Who you are, as far as the world is concerned: a silhouette and a name.
     It survives every scene change because it is a fact about the player, not
     about the place — and it is the answer to "which one of these is me". */
  var look = { coat: '#38384a', hair: '#1f1a17', hat: null, bag: null, name: 'you' };

  /* ------------------------------------------------------------- boot */

  function init() {
    cvs = document.getElementById('world');
    ctx = cvs.getContext('2d');
    card = document.getElementById('worldcard');
    promptEl = document.getElementById('prompt');
    plateEl = document.getElementById('nameplate');

    window.addEventListener('resize', resize);
    document.addEventListener('visibilitychange', function () { last = 0; });
    document.getElementById('viewport').addEventListener('pointerdown', onPointerDown);
    window.addEventListener('pointermove', onPointerMove);
    window.addEventListener('pointerup', onPointerUp);
    cvs.addEventListener('wheel', onWheel, { passive: false });

    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('keyup', function (e) {
      var k = walkKey(e.key);
      if (k) walk[k] = false;
    });
    // a key held while the tab loses focus would otherwise stay held
    window.addEventListener('blur', function () { walk = {}; });

    document.body.addEventListener('click', onClick);

    buildKeys();
    buildPods();
    syncPhone();
    syncName();
    setScene('street');
    resize();
    requestAnimationFrame(loop);
  }

  function onClick(e) {
    if (suppressClick) return;
    if (e.target.closest('[data-toast-close]')) { dismiss(e.target.closest('.toast'), true); return; }
    if (e.target.closest('#helpBtn')) { toggleKeyCard(); return; }
    var pick = e.target.closest('[data-pick]');
    if (pick) { e.preventDefault(); wear(pick.getAttribute('data-pick')); return; }
    var go = e.target.closest('[data-go]');
    if (go) { e.preventDefault(); setScene(go.getAttribute('data-go'), sceneId); closePanel(); return; }
    var p = e.target.closest('[data-panel]');
    if (p) {
      e.preventDefault();
      // acting on a toast is not the same as ignoring it: it does not go on
      // to sit unread in the phone
      dismiss(p.closest('.toast'), false);
      openPanel(p.getAttribute('data-panel'));
      return;
    }
    if (e.target.closest('[data-close]')) { closePanel(); return; }
    var mv = e.target.closest('[data-move]');
    if (mv) { e.preventDefault(); moveHome(mv.getAttribute('data-move')); return; }
    if (e.target.closest('[data-quality]')) { setQuality(quality === 'high' ? 'low' : 'high'); return; }
    if (e.target.closest('[data-follow]')) { recenter(); return; }
    var z = e.target.closest('[data-zoom]');
    if (z) {
      userZoom = E.clamp(userZoom * (z.getAttribute('data-zoom') === 'in' ? 1.25 : 0.8), 0.34, 3.2);
      zoom = baseZoom * userZoom;
      clampCam();
    }
  }

  /* ------------------------------------------------------------- keys */

  function walkKey(key) {
    switch (key) {
      case 'ArrowLeft': return 'l';
      case 'ArrowRight': return 'r';
      case 'ArrowUp': return 'u';
      case 'ArrowDown': return 'd';
    }
    return null;
  }

  /* Letters are destinations and toggles; the arrows are the character. The
     letters that used to pan the camera are gone — dragging does that, and a
     letter is worth more as a door than as a nudge. */
  var COMMANDS = {
    o: interact, e: interact,
    a: goIndoors,
    p: function () { openPanel('playbook'); },
    h: function () { openPanel('health'); },
    k: function () { openPanel('karma'); },
    c: openPhone,
    f: recenter,
    q: function () { setQuality(quality === 'high' ? 'low' : 'high'); },
    '?': toggleKeyCard,
    '/': toggleKeyCard,
  };

  function onKeyDown(e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.target.closest && e.target.closest('input, textarea')) return;

    if (e.key === 'Escape') {
      if (document.getElementById('keycard').classList.contains('open')) toggleKeyCard();
      else closePanel();
      return;
    }

    var k = walkKey(e.key);
    if (k) {
      walk[k] = true;
      if (player) player.route = null;   // taking the wheel cancels a click-route
      closePanel();                      // and puts you back in the world
      e.preventDefault();
      return;
    }

    var cmd = COMMANDS[e.key.toLowerCase()];
    if (cmd) { e.preventDefault(); cmd(); }
  }

  var KEYBAR = [
    { keys: ['←', '↑', '→', '↓'], label: 'walk' },
    { keys: ['O'], label: 'use what you are next to' },
    { keys: ['A'], label: 'your room' },
    { keys: ['P'], label: 'playbook' },
    { keys: ['H'], label: 'health' },
    { keys: ['C'], label: 'phone' },
    { keys: ['?'], label: 'all keys' },
  ];

  var KEYCARD = [
    { title: 'Moving', items: [
      { keys: ['←', '↑', '→', '↓'], label: 'Walk' },
      { keys: ['click'], label: 'Walk to that spot' },
      { keys: ['drag'], label: 'Look around' },
      { keys: ['F'], label: 'Follow the character again' },
    ] },
    { title: 'Where you are', items: [
      { keys: ['O'], label: 'Use the nearest thing' },
      { keys: ['A'], label: 'Go to your room' },
      { keys: ['C'], label: 'Your phone' },
    ] },
    { title: 'You', items: [
      { keys: ['P'], label: 'Playbook &amp; routines' },
      { keys: ['H'], label: 'Health' },
      { keys: ['K'], label: 'Karma' },
    ] },
    { title: 'View', items: [
      { keys: ['Q'], label: 'Effects tier' },
      { keys: ['Esc'], label: 'Back to the world' },
      { keys: ['?'], label: 'This card' },
    ] },
  ];

  function buildKeys() {
    document.getElementById('keybar').innerHTML = KeyBar(KEYBAR);
    document.getElementById('keycard').innerHTML = KeyCard(KEYCARD) +
      '<p class="keycard-note">Nothing here is the only way in. Work is a building you ' +
      'walk into, other work is the paper on the stand or the computer at home, the closet ' +
      'is in your room, and a challenge finds you where it happens.</p>';
    document.getElementById('helpBtn').innerHTML = HelpButton({ key: '?' });
  }

  // The ? beside the phone and the ? key are the same switch.
  function toggleKeyCard() {
    var open = document.getElementById('keycard').classList.toggle('open');
    document.getElementById('helpBtn').classList.toggle('on', open);
  }

  /* -------------------------------------------------------------- pods */

  /* Three numbers that are true wherever you are standing. They are the only
     state not local to a place, which is the whole reason they are chrome —
     and the bar for being here is that high. The paid-action cap is a rule
     the world enforces, not a number the player plays against, so it belongs
     where it bites rather than on screen all day. */
  var PODS = [
    { glyph: '◉', value: '31', label: 'Biological age', tone: 'accent', panel: 'health' },
    { glyph: '$', value: '4,212', label: 'USD', panel: 'work' },
    { glyph: '◈', value: '1,840', note: '+4 wk', label: 'Karma', panel: 'karma' },
  ];

  function buildPods() {
    document.getElementById('pods').innerHTML = StatPods(PODS.map(StatPod));
  }

  /* ------------------------------------------------------------ closet */

  /* "slot:value", straight off the swatch. An empty value is the none
     option, which is a choice rather than the absence of one. */
  function wear(spec) {
    var i = spec.indexOf(':');
    var slot = spec.slice(0, i), value = spec.slice(i + 1);
    look[slot] = value || null;
    dressPlayer();
    syncName();
    if (openName === 'closet') openPanel('closet');
  }

  // The look is a fact about the player, so it is re-applied to whatever
  // figure the current scene just built for them.
  function dressPlayer() {
    if (!player) return;
    player.color = look.coat;
    player.hair = look.hair;
    player.hat = look.hat;
    player.bag = look.bag;
  }

  function syncName() {
    plateEl.innerHTML = NamePlate({ name: look.name });
  }

  /* ------------------------------------------------------------- phone */

  function syncPhone(ring) {
    var el = document.getElementById('phone');
    el.innerHTML = PhoneButton({ count: unread, key: 'C' });
    if (ring) {
      el.classList.remove('ring');
      void el.offsetWidth;              // restart the animation
      el.classList.add('ring');
    }
  }

  function openPhone() {
    unread = 0;
    syncPhone();
    openPanel('inbox');
  }

  /* The pushed surface, on a timer. Everything else in the client waits for
     you to walk into it; this is the exception, so it is the only thing that
     interrupts — and ignoring one drops it into the phone rather than losing
     it, which is the same contract the inbox has (§8.2). */
  var FEED = [
    { at: 8, kicker: 'Dana &middot; missed call',
      title: 'She left a message about a room on the Riverfront side',
      action: 'Listen', panel: 'travel' },
    { at: 34, kicker: 'Marta &middot; the port',
      title: 'Who authorised the overtime?',
      body: 'Your character said he would look into it. He has not.',
      action: 'Open', panel: 'work' },
    { at: 76, kicker: 'Fenn Logistics',
      title: 'The operations manager role is posted',
      body: 'Closes Friday. You applied on Tuesday.',
      action: 'Open', panel: 'jobs' },
  ];

  function tickFeed() {
    if (delivered >= FEED.length || t < FEED[delivered].at) return;
    var item = FEED[delivered++];
    var el = document.createElement('div');
    el.className = 'toast';
    el.innerHTML = Toast(item);
    document.getElementById('toasts').appendChild(el);
    syncPhone(true);
    setTimeout(function () { dismiss(el, true); }, 13000);
  }

  function dismiss(el, toPhone) {
    if (!el || el.classList.contains('leaving')) return;
    el.classList.add('leaving');
    if (toPhone) { unread++; syncPhone(); }
    setTimeout(function () { el.remove(); }, 300);
  }

  /* ------------------------------------------------------------- view */

  function recenter() {
    cam.free = false;
    syncFollowChip();
  }

  function syncFollowChip() {
    var b = document.getElementById('followBtn');
    if (b) b.classList.toggle('on', !cam.free);
  }

  function setQuality(q) {
    quality = q;
    document.getElementById('qualityLabel').textContent = 'Effects: ' + q;
    document.body.classList.toggle('quality-low', q === 'low');
  }

  /* ------------------------------------------------------- destinations */

  // A is your room, wherever you currently live. It is the one shortcut that
  // changes meaning when you move, because "home" is a fact about you.
  function goIndoors() {
    var env = E.ENVIRONMENTS[home];
    if (sceneId === env.indoor) { closePanel(); return; }
    setScene(env.indoor, sceneId);
    closePanel();
  }

  // O reaches for whatever is closest. Doors also answer to it, so a player
  // who has not yet worked out that you can just walk in is never stuck.
  function interact() {
    if (!near) return;
    trigger(near);
  }

  function trigger(m) {
    if (m.go) { setScene(m.go, sceneId); closePanel(); return; }
    if (m.panel) openPanel(m.panel);
  }

  /* ------------------------------------------------------------ scenes */

  function setScene(id, from) {
    if (!E.SCENES[id] || id === sceneId) return;
    if (E.SCENES[id].env && E.SCENES[id].env !== home) return;
    sceneId = id;
    scene = E.SCENES[id];
    fadeFrom = performance.now();
    cam.set = false;
    cam.free = false;
    drag = null;

    player = clone(scene.player);
    player.path = null;    // the player is driven, never scripted
    var start = player.start || [1, 1];
    // Come in through the door you went out of: the scene you arrived from
    // names a portal on this side, and that is where you are standing.
    var back = from && (scene.markers || []).filter(function (m) { return m.go === from; })[0];
    if (back) start = settle([back.x, back.y]);
    player.x = start[0];
    player.y = start[1];
    player.wp = 1; player.moving = false; player.phase = 0;
    player.route = null; player.ri = 0;
    dressPlayer();

    actors = (scene.actors || []).map(function (a) {
      var c = clone(a);
      c.wp = 1;
      if (c.path) { c.x = c.path[0][0]; c.y = c.path[0][1]; }
      return c;
    });

    motes = scene.motes ? new E.Particles('dust', scene.motes, { x: 0, y: 0, w: 100, h: 100 }) : null;

    buildMarkers();
    syncChrome();
    syncFollowChip();
  }

  function clone(o) { return JSON.parse(JSON.stringify(o)); }

  // A door sits against the wall, so the point that names it is not always a
  // tile you can stand on. Step off it by the smallest amount that is — and
  // stay inside the doorway box, so arriving does not re-trigger it.
  function settle(p) {
    if (standable(p[0], p[1])) return p;
    var OFF = [[0, 0.35], [0, -0.35], [0.35, 0], [-0.35, 0], [0, 0.55], [0, -0.55]];
    for (var i = 0; i < OFF.length; i++) {
      var x = p[0] + OFF[i][0], y = p[1] + OFF[i][1];
      if (standable(x, y)) return [x, y];
    }
    return p;
  }

  // Moving is a relocation, capped at once a day. It was never a nav click,
  // and now there is no nav to click: you take the 6 from the stop it leaves.
  function moveHome(target) {
    if (movedToday || target === home) return;
    home = target;
    movedToday = true;
    setScene(E.ENVIRONMENTS[home].outdoor);
    closePanel();
  }

  function syncChrome() {
    document.getElementById('placeChip').innerHTML = PlaceChip({
      name: scene.name, where: scene.where, clock: scene.clock,
    });
    var env = E.ENVIRONMENTS[scene.env] || E.ENVIRONMENTS[home];
    document.getElementById('envChip').innerHTML = EnvChip({
      label: 'Rule set in force', name: env.name,
      rules: env.rules, ruleTone: env.ruleTone,
    });
  }

  /* ----------------------------------------------------------- markers */

  function buildMarkers() {
    var layer = document.getElementById('markers');
    layer.innerHTML = '';
    markerEls = [];
    cardZone = null;
    cardVisible = false;
    near = null;
    promptEl.classList.remove('show');
    card.classList.remove('show');

    (scene.markers || []).forEach(function (m) {
      if (m.kind === 'zone') { cardZone = m; return; }
      // A door you are already standing in is disarmed until you step out of
      // it — otherwise arriving through it would immediately send you back.
      if (m.kind === 'portal') m.armed = !inDoor(m);
      // Every location explains itself. A labelled point that will not say
      // what it is for reads as a destination and then refuses to be one, so
      // a missing tip is a content bug and says so out loud.
      if (!m.tip) {
        console.warn('[etm] location without a tip: ' + sceneId + ' / ' + m.label);
      }
      var el = document.createElement('div');
      el.className = 'marker ' + m.kind;
      el.innerHTML = Marker({
        label: m.label,
        tip: m.tip || 'Nobody has written down what this is yet.',
        kind: m.kind,
        key: 'O',
      });
      if (m.kind !== 'sign') {
        if (m.go) el.setAttribute('data-go', m.go);
        if (m.panel) el.setAttribute('data-panel', m.panel);
      }
      layer.appendChild(el);
      markerEls.push({ el: el, m: m });
    });

    if (cardZone && scene.zone) {
      var z = scene.zone;
      card.innerHTML = WorldCard({
        kicker: z.title, body: z.body, quote: z.quote,
        accept: z.accept, decline: z.decline, foot: z.foot, panel: z.panel,
      });
    }
  }

  /* ------------------------------------------------------------ layout */

  function resize() {
    var host = document.getElementById('viewport');
    var r = host.getBoundingClientRect();
    W = Math.max(320, Math.floor(r.width));
    H = Math.max(240, Math.floor(r.height));
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    cvs.width = Math.floor(W * dpr);
    cvs.height = Math.floor(H * dpr);
    cvs.style.width = W + 'px';
    cvs.style.height = H + 'px';
    baseZoom = E.clamp(Math.min(W / 1900, H / 1020), 0.48, 0.86);
    zoom = baseZoom * userZoom;
  }

  /* -------------------------------------------------------------- loop */

  function loop(ts) {
    var dt = last ? Math.min((ts - last) / 1000, 0.05) : 0;
    last = ts;
    t += dt;
    update(dt);
    draw();
    requestAnimationFrame(loop);
  }

  /* --------------------------------------------------------- navigation */

  function walkGrid() {
    if (scene._walk) return scene._walk;
    var codes = WALKABLE[sceneId] || '';
    scene._walk = scene.rows.map(function (row) {
      return row.split('').map(function (c) { return codes.indexOf(c) >= 0; });
    });
    return scene._walk;
  }

  function walkableAt(x, y) {
    var g = walkGrid();
    return y >= 0 && y < g.length && x >= 0 && x < g[0].length && g[y][x];
  }

  // sampled line-of-sight over the walk grid, used to straighten paths
  function clearLine(ax, ay, bx, by) {
    var steps = Math.ceil(Math.max(Math.abs(bx - ax), Math.abs(by - ay)) * 5) + 1;
    for (var i = 0; i <= steps; i++) {
      var t = i / steps;
      if (!walkableAt(Math.floor(ax + (bx - ax) * t), Math.floor(ay + (by - ay) * t))) return false;
    }
    return true;
  }

  var DIRS = [[1, 0], [-1, 0], [0, 1], [0, -1], [1, 1], [1, -1], [-1, 1], [-1, -1]];

  function findCells(sx, sy, tx, ty) {
    if (!walkableAt(sx, sy) || !walkableAt(tx, ty)) return null;
    var open = [{ x: sx, y: sy, g: 0, f: 0, p: null }];
    var best = {};
    best[sx + ',' + sy] = 0;
    var guard = 0;

    while (open.length && guard++ < 40000) {
      var bi = 0;
      for (var i = 1; i < open.length; i++) if (open[i].f < open[bi].f) bi = i;
      var cur = open.splice(bi, 1)[0];

      if (cur.x === tx && cur.y === ty) {
        var out = [];
        for (var n = cur; n; n = n.p) out.unshift([n.x, n.y]);
        return out;
      }

      for (var d = 0; d < DIRS.length; d++) {
        var dx = DIRS[d][0], dy = DIRS[d][1];
        var nx = cur.x + dx, ny = cur.y + dy;
        if (!walkableAt(nx, ny)) continue;
        // never squeeze diagonally between two blocked cells
        if (dx && dy && (!walkableAt(cur.x + dx, cur.y) || !walkableAt(cur.x, cur.y + dy))) continue;
        var cost = cur.g + (dx && dy ? 1.4142 : 1);
        var key = nx + ',' + ny;
        if (best[key] !== undefined && best[key] <= cost) continue;
        best[key] = cost;
        open.push({ x: nx, y: ny, g: cost, f: cost + Math.hypot(tx - nx, ty - ny), p: cur });
      }
    }
    return null;
  }

  // string-pulling: drop every waypoint the walker can see past
  function straighten(pts) {
    var out = [pts[0]], i = 0;
    while (i < pts.length - 1) {
      var j = pts.length - 1;
      while (j > i + 1 && !clearLine(pts[i][0], pts[i][1], pts[j][0], pts[j][1])) j--;
      out.push(pts[j]);
      i = j;
    }
    return out;
  }

  function routeTo(gx, gy) {
    if (!walkableAt(Math.floor(gx), Math.floor(gy))) return null;
    if (clearLine(player.x, player.y, gx, gy)) return [[gx, gy]];

    var cells = findCells(Math.floor(player.x), Math.floor(player.y), Math.floor(gx), Math.floor(gy));
    if (!cells) return null;

    var pts = [[player.x, player.y]];
    for (var i = 0; i < cells.length; i++) pts.push([cells[i][0] + 0.5, cells[i][1] + 0.5]);
    pts.push([gx, gy]);
    pts = straighten(pts);
    pts.shift();
    return pts.length ? pts : null;
  }

  function actorTarget(a) {
    if (a.route) return a.route[a.ri];
    if (!a.path || !a.speed) return null;
    return a.path[a.wp];
  }

  function stepActor(a, dt) {
    if (a.hold > 0) { a.hold -= dt; a.moving = false; return; }
    var tg = actorTarget(a);
    if (!tg) { a.moving = false; return; }
    var dx = tg[0] - a.x, dy = tg[1] - a.y;
    var d = Math.sqrt(dx * dx + dy * dy);
    if (d < 0.08) {
      if (a.route) {
        a.ri++;
        if (a.ri < a.route.length) return;      // straight on to the next leg
        a.route = null;
        a.hold = 3.2;
      } else {
        a.wp = 1 - a.wp;
        a.hold = 0.5 + Math.random();
      }
      a.moving = false;
      return;
    }
    var v = a.speed * dt;
    a.x += dx / d * v;
    a.y += dy / d * v;
    a.moving = true;
  }

  /* The arrows are read in screen space and applied in grid space: up the
     screen is away from the camera on both axes, which is the only reading
     that feels right on an isometric plate. Axes are resolved separately so
     a diagonal into a wall slides along it instead of stopping dead. */
  function steer(dt) {
    var a = (walk.r ? 1 : 0) - (walk.l ? 1 : 0);
    var b = (walk.d ? 1 : 0) - (walk.u ? 1 : 0);
    if (!a && !b) return false;

    var dx = (a + b) / 2, dy = (b - a) / 2;
    var len = Math.hypot(dx, dy);
    var v = (player.speed || 2.2) * dt / len;
    dx *= v; dy *= v;

    if (standable(player.x + dx, player.y)) player.x += dx;
    if (standable(player.x, player.y + dy)) player.y += dy;

    player.hold = 0;
    player.moving = true;
    if (!cam.free) return true;
    recenter();                 // walking is a reason to look at yourself again
    return true;
  }

  // The character has width, so all four corners of their footprint have to
  // be on walkable ground. Without this you can clip the corner of a wall.
  function standable(x, y) {
    var r = 0.2;
    return walkableAt(Math.floor(x + r), Math.floor(y + r)) &&
           walkableAt(Math.floor(x - r), Math.floor(y - r)) &&
           walkableAt(Math.floor(x + r), Math.floor(y - r)) &&
           walkableAt(Math.floor(x - r), Math.floor(y + r));
  }

  function update(dt) {
    if (!steer(dt)) stepActor(player, dt);
    for (var i = 0; i < actors.length; i++) stepActor(actors[i], dt);
    tickFeed();

    var pc = E.isoc(player.x, player.y);
    if (!cam.set) { cam.x = pc.x; cam.y = pc.y - 30; cam.set = true; }

    if (!cam.free && !drag) {
      cam.x += (pc.x - cam.x) * Math.min(1, dt * 2.4);
      cam.y += ((pc.y - 30) - cam.y) * Math.min(1, dt * 2.4);
    }
    clampCam();

    if (motes) {
      motes.bounds = { x: 0, y: 0, w: W, h: H };
      motes.update(dt, t);
    }
    updateMarkers();
  }

  /* A city this size holds a few hundred fixtures and a few dozen people,
     and almost none of them are on screen at once. Both the shadow pass and
     the sorted object layer walk the whole list every frame, so both cull
     against the window first — generously, because a lamp post is tall and
     its shadow is long. */
  function onScreen(gx, gy, pad) {
    var s = toScreen(gx, gy, 0);
    var p = pad || 280;
    return s.x > -p && s.x < W + p && s.y > -p && s.y < H + p;
  }

  function offscreen(b) {
    var near = toScreen(b.x + b.w, b.y + b.d, 0);
    var far = toScreen(b.x, b.y, b.h);
    var pad = 200;
    return near.x < -pad || far.x > W + pad || near.y < -pad || far.y > H + pad;
  }

  // grid range the viewport can actually see, so ground cost tracks the window
  function visibleRange() {
    var xs = [], ys = [];
    [[0, 0], [W, 0], [0, H], [W, H]].forEach(function (c) {
      var wx = (c[0] - W / 2) / zoom + cam.x;
      var wy = (c[1] - H / 2) / zoom + cam.y - E.TH / 2;
      xs.push((wx / (E.TW / 2) + wy / (E.TH / 2)) / 2);
      ys.push((wy / (E.TH / 2) - wx / (E.TW / 2)) / 2);
    });
    return {
      x0: Math.floor(Math.min.apply(null, xs)) - 2,
      x1: Math.ceil(Math.max.apply(null, xs)) + 2,
      y0: Math.floor(Math.min.apply(null, ys)) - 2,
      y1: Math.ceil(Math.max.apply(null, ys)) + 3
    };
  }

  function bounds() {
    if (scene._bounds) return scene._bounds;
    var R = scene.rows.length, C = scene.rows[0].length;
    scene._bounds = {
      minX: -R * E.TW / 2 - 120, maxX: C * E.TW / 2 + 120,
      minY: -300, maxY: (C + R) * E.TH / 2 + 120
    };
    return scene._bounds;
  }

  function clampCam() {
    var b = bounds();
    cam.x = E.clamp(cam.x, b.minX, b.maxX);
    cam.y = E.clamp(cam.y, b.minY, b.maxY);
  }

  function toScreen(gx, gy, z) {
    var p = E.isoc(gx, gy);
    return {
      x: (p.x - cam.x) * zoom + W / 2,
      y: (p.y - cam.y - (z || 0)) * zoom + H / 2
    };
  }

  /* Standing in a doorway: a box against the wall rather than a radius, so
     the pavement in front of the shopfronts stays walkable. */
  function inDoor(m) {
    return Math.abs(player.x - m.x) <= DOOR.x && Math.abs(player.y - m.y) <= DOOR.y;
  }

  /* What the O key is pointing at, and what the prompt names. Doors count as
     reachable so the key works on them too; the door still opens on its own
     the moment you actually step into it. */
  function updateReach() {
    var best = null, bestD = REACH;
    for (var i = 0; i < markerEls.length; i++) {
      var m = markerEls[i].m;
      if (m.kind === 'sign' || (!m.go && !m.panel)) continue;
      var d = Math.hypot(player.x - m.x, player.y - m.y);
      if (d < bestD) { bestD = d; best = m; }
      markerEls[i].el.classList.toggle('near', d < REACH);

      if (m.kind !== 'portal') continue;
      if (!inDoor(m)) { m.armed = true; continue; }
      if (m.armed) { m.armed = false; trigger(m); return; }
    }

    if (best !== near) {
      near = best;
      if (near) {
        promptEl.innerHTML = ActionPrompt({
          key: near.kind === 'portal' ? '↑' : 'O',
          label: near.label,
          hint: near.kind === 'portal' ? (near.hint ? near.hint + ' · walk in' : 'walk in') : near.hint,
        });
      }
      promptEl.classList.toggle('show', !!near);
    }
    if (near) {
      var p = toScreen(player.x, player.y, 146);
      promptEl.style.transform =
        'translate(' + p.x.toFixed(1) + 'px,' + p.y.toFixed(1) + 'px) translate(-50%,-100%)';
    }

    // the plate sits just off the head; the prompt stacks above it
    var np = toScreen(player.x, player.y, 74);
    plateEl.style.transform =
      'translate(' + np.x.toFixed(1) + 'px,' + np.y.toFixed(1) + 'px) translate(-50%,-100%)';
  }

  function updateMarkers() {
    for (var i = 0; i < markerEls.length; i++) {
      var m = markerEls[i];
      var s = toScreen(m.m.x, m.m.y, 8);
      m.el.style.transform = 'translate(' + s.x.toFixed(1) + 'px,' + s.y.toFixed(1) + 'px) translate(-50%,-100%)';
      m.el.style.opacity = (s.x > -60 && s.x < W + 60 && s.y > -40 && s.y < H + 40) ? 1 : 0;
    }
    updateReach();
    if (cardZone) {
      var d = Math.hypot(player.x - cardZone.x, player.y - cardZone.y);
      var near = d < (cardZone.radius || 4);
      if (near !== cardVisible) {
        cardVisible = near;
        card.classList.toggle('show', near);
      }
      // keep the prompt inside the viewport even when the zone is near an edge
      var s2 = toScreen(cardZone.x, cardZone.y, 120);
      var cw = card.offsetWidth || 330, chh = card.offsetHeight || 200;
      var cx = E.clamp(s2.x, cw / 2 + 14, Math.max(cw / 2 + 14, W - cw / 2 - 14));
      var cy = E.clamp(s2.y, chh + 14, Math.max(chh + 14, H - 14));
      card.style.transform = 'translate(' + cx.toFixed(1) + 'px,' + cy.toFixed(1) + 'px) translate(-50%,-100%)';
    }
  }

  /* -------------------------------------------------------------- draw */

  // One camera for everything on the plate. There is no parallax layer: a
  // second layer moving at its own rate only reads as depth from ground
  // level, and detaches into the sky at any other zoom.
  function applyCam() {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.translate(W / 2, H / 2);
    ctx.scale(zoom, zoom);
    ctx.translate(-cam.x, -cam.y);
  }

  function draw() {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    emissive.length = 0;

    drawSky();

    applyCam();
    drawGround();
    drawShadows();
    drawObjects();
    drawZoneRing();
    drawEmissive();

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    if (quality === 'high') {
      sunWash();
      if (motes) motes.draw(ctx, '#fff0cc');
    }

    // Wall-clock, not accumulated frame time: a throttled tab must never
    // resume holding a half-finished transition over the scene.
    var fade = 1 - E.clamp((performance.now() - fadeFrom) / 480, 0, 1);
    if (fade > 0) {
      ctx.fillStyle = 'rgba(240,234,222,' + (fade * 0.85).toFixed(3) + ')';
      ctx.fillRect(0, 0, W, H);
    }
  }

  /* --------------------------------------------------------------- sky */

  // buildings, sorted back-to-front for the painter
  function solids() {
    return (scene.boxes || []).slice()
      .sort(function (a, b) { return (a.y + a.d) - (b.y + b.d) || (a.x + a.w) - (b.x + b.w); });
  }

  function drawSky() {
    var s = scene.sky;
    var g = ctx.createLinearGradient(0, 0, 0, H);
    g.addColorStop(0, s[0]);
    g.addColorStop(0.58, s[1]);
    g.addColorStop(1, s[2]);
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, W, H);
    if (scene.interior) return;

    // the sun sits off to the camera-right, which is where every shadow points away from
    var sx = W * 0.86, sy = H * 0.10 - cam.y * 0.06;
    var sg = ctx.createRadialGradient(sx, sy, 0, sx, sy, W * 0.55);
    sg.addColorStop(0, 'rgba(255,246,214,0.55)');
    sg.addColorStop(0.35, 'rgba(255,232,180,0.14)');
    sg.addColorStop(1, 'rgba(255,232,180,0)');
    ctx.fillStyle = sg;
    ctx.fillRect(0, 0, W, H);

    drawClouds();
  }

  function drawClouds() {
    var pack = [
      [0.10, 0.14, 150, 0.55], [0.30, 0.09, 110, 0.4],
      [0.52, 0.17, 190, 0.5], [0.74, 0.11, 130, 0.35], [0.92, 0.22, 160, 0.42]
    ];
    ctx.save();
    for (var i = 0; i < pack.length; i++) {
      var c = pack[i];
      var drift = (t * (4 + i * 1.6)) % (W + 700) - 350;
      var x = c[0] * W + drift * 0.25 - cam.x * 0.06;
      var y = c[1] * H - cam.y * 0.03;
      var r = c[2] * zoom;
      ctx.fillStyle = 'rgba(255,255,255,' + c[3] + ')';
      for (var j = 0; j < 5; j++) {
        var o = E.hash(i * 7 + j, j * 3.3);
        ctx.beginPath();
        ctx.ellipse(x + (j - 2) * r * 0.34, y + (o - 0.5) * r * 0.18,
                    r * (0.32 + o * 0.2), r * (0.14 + o * 0.08), 0, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    ctx.restore();
  }

  /* ------------------------------------------------------------ ground */

  function drawGround() {
    var rows = scene.rows;
    var v = visibleRange();
    for (var y = Math.max(0, v.y0); y < Math.min(rows.length, v.y1); y++) {
      var row = rows[y];
      for (var x = Math.max(0, v.x0); x < Math.min(row.length, v.x1); x++) {
        var code = row[x];
        var g = E.GROUND[code];
        if (!g) continue;
        var e = ELEV[code] || 0;
        var n = E.hash(x * 1.7 + 0.3, y * 2.3 + 0.7);
        var col = E.mix(g.a, g.b, n);

        if (g.water) {
          var sh = 0.5 + 0.5 * Math.sin(t * 0.9 + x * 0.55 + y * 0.8);
          col = E.mix(col, '#8ec4de', 0.05 + sh * 0.16);
        }

        var p = E.iso(x, y);
        var below = rows[y + 1] ? rows[y + 1][x] : '.';
        var right = row[x + 1] || '.';
        var eb = ELEV[below] || 0, er = ELEV[right] || 0;
        var Bm = { x: p.x, y: p.y + E.TH - e };
        if (e > eb) {
          var L = { x: p.x - E.TW / 2, y: p.y + E.TH / 2 - e };
          E.poly(ctx, [L, Bm, { x: Bm.x, y: Bm.y + (e - eb) }, { x: L.x, y: L.y + (e - eb) }], E.shade(col, 0.72));
        }
        if (e > er) {
          var R = { x: p.x + E.TW / 2, y: p.y + E.TH / 2 - e };
          E.poly(ctx, [Bm, R, { x: R.x, y: R.y + (e - er) }, { x: Bm.x, y: Bm.y + (e - er) }], E.shade(col, 0.86));
        }

        ctx.save();
        ctx.translate(0, -e);
        E.tile(ctx, x, y, col);
        if (code === 'k' && x % 2 === 0) E.tile(ctx, x, y, E.rgba('#ffffff', 0.30));
        if (code === 'f' && (x + y) % 2 === 0) E.tile(ctx, x, y, E.rgba('#5a3a1d', 0.10));
        if (g.water && E.hash(x * 3.1, y * 5.9) > 0.8) {
          var c2 = E.isoc(x, y);
          ctx.fillStyle = E.rgba('#ffffff', 0.10 + 0.12 * Math.sin(t * 1.7 + x + y));
          ctx.fillRect(c2.x - 13, c2.y - 1, 26, 2);
        }
        ctx.restore();
      }
    }
  }

  /* ----------------------------------------------------------- shadows */

  function sunOff(h) {
    var s = scene.sun;
    return { x: s.x * h, y: s.y * h };
  }

  function shadowFill() { return E.rgba(scene.sun.shade, 1); }

  function withShadowInk(fn) {
    ctx.save();
    ctx.globalCompositeOperation = 'multiply';
    ctx.globalAlpha = 0.42;
    fn();
    ctx.restore();
  }

  function boxShadow(x, y, w, d, h) {
    var o = sunOff(h);
    var A = E.iso(x, y), B = E.iso(x + w, y), C = E.iso(x + w, y + d), D = E.iso(x, y + d);
    function off(p) { return { x: p.x + o.x, y: p.y + o.y }; }
    E.poly(ctx, [D, A, B, off(B), off(C), off(D)], shadowFill());
  }

  function blobShadow(gx, gy, h, rx, ry) {
    var o = sunOff(h);
    var c = E.isoc(gx, gy);
    ctx.beginPath();
    ctx.ellipse(c.x + o.x * 0.6, c.y + o.y * 0.6, rx, ry, 0, 0, Math.PI * 2);
    ctx.fillStyle = shadowFill();
    ctx.fill();
  }

  function drawShadows() {
    withShadowInk(function () {
      solids().forEach(function (b) { if (!offscreen(b)) boxShadow(b.x, b.y, b.w, b.d, b.h); });
      (scene.fixtures || []).forEach(function (f) {
        if (f.kind !== 'railing' && !onScreen(f.x, f.y)) return;
        switch (f.kind) {
          case 'tree': blobShadow(f.x, f.y, 78, 30, 15); break;
          case 'lamp': boxShadow(f.x, f.y, 0.16, 0.16, 80); break;
          case 'bench': blobShadow(f.x, f.y, 26, 26, 11); break;
          case 'planter': blobShadow(f.x, f.y, 20, 17, 9); break;
          case 'car': boxShadow(f.x, f.y, 2.4, 1.1, 22); break;
          case 'bus': boxShadow(f.x, f.y, 4.4, 1.5, 46); break;
          case 'busStop': boxShadow(f.x, f.y, 2.6, 1.0, 54); break;
          case 'stall': boxShadow(f.x, f.y, 1.7, 1.5, 52); break;
          case 'newsstand': boxShadow(f.x, f.y, 1.5, 1.0, 62); break;
          case 'plane':
            boxShadow(f.x, f.y, 7.2 * (f.small ? 0.62 : 1), 1.5, 17);
            blobShadow(f.x + 2.6, f.y + 0.6, 13, 44 * (f.small ? 0.62 : 1), 22); break;
          case 'fountain': blobShadow(f.x, f.y, 22, 34, 17); break;
          case 'boat': blobShadow(f.x, f.y, 14, 34, 13); break;
          case 'bollard': blobShadow(f.x, f.y, 14, 5, 3); break;
          case 'bags': blobShadow(f.x, f.y, 16, 21, 9); break;
          case 'hydrant': blobShadow(f.x, f.y, 18, 8, 5); break;
          case 'plant': blobShadow(f.x, f.y, 40, 16, 8); break;
        }
      });
      actors.concat([player]).forEach(function (a) {
        if (!onScreen(a.x, a.y, 120)) return;
        blobShadow(a.x, a.y, 58 * (a.scale || 1), 11, 5.5);
      });
    });
  }

  /* --------------------------------------------------------- buildings */

  function facePts(q, u, v, uw, vh) {
    return [E.facePt(q, u, v), E.facePt(q, u + uw, v),
            E.facePt(q, u + uw, v + vh), E.facePt(q, u, v + vh)];
  }

  /* A building is a box plus a facade painter. The box is the same for all
     of them; everything that makes a court read as a court and a gym as a
     gym is in the painter and in what stands on the roof or in front of it.

     Order matters and is the only fiddly part: anything standing ON the roof
     is drawn *before* the box, so the box hides its shaft and only the part
     clearing the roofline survives. Anything standing IN FRONT — a portico,
     a canopy, a flight of steps — is drawn after. */

  function drawBuilding(b) {
    var base = b.color;
    // the +x wall takes the sun; the street-facing +y wall sits in its own shade
    var tone = {
      base: base,
      lit: E.mix(base, scene.sun.warm, 0.26),
      shade: E.mix(E.shade(base, 0.7), scene.sun.shade, 0.3),
      sky: scene.sky[0]
    };

    var type = TYPES[b.type] || TYPES.flats;
    if (type.roof) type.roof(b, tone);

    var faces = E.box(ctx, {
      x: b.x, y: b.y, w: b.w, d: b.d, h: b.h,
      color: base,
      left: tone.shade,
      right: tone.lit,
      top: E.mix(base, '#ffffff', 0.3)
    });

    if (b.wall) { drawOpenings(b, faces); return; }

    type.face(b, faces, tone);
    if (b.store) shopfront(b, faces, tone);
    if (type.front) type.front(b, faces, tone);
    drawOpenings(b, faces);
  }

  /* ------------------------------------------------------- facade parts */

  function fr(q, u, v, uw, vh, fill) { E.faceRect(ctx, q, u, v, uw, vh, fill); }

  // glass as reflected sky, cooler and darker the further down the wall
  function pane(tone, v, bright) {
    return E.mix(E.mix('#22303f', tone.sky, 0.34 + bright * 0.5), '#ffffff', (1 - v) * 0.2);
  }

  function parapet(faces, tone) {
    fr(faces.L, 0, 0, 1, 0.022, E.shade(tone.shade, 0.86));
    fr(faces.R, 0, 0, 1, 0.022, E.shade(tone.lit, 0.88));
  }

  // a heavy classical lid: cornice, frieze, and a shadow line under both
  function cornice(faces, tone, v) {
    var stone = E.mix(tone.base, '#ffffff', 0.24);
    fr(faces.L, 0, v, 1, 0.036, stone);
    fr(faces.L, 0, v + 0.036, 1, 0.008, E.shade(tone.shade, 0.78));
    fr(faces.R, 0, v, 1, 0.036, E.mix(tone.lit, '#ffffff', 0.2));
    fr(faces.R, 0, v + 0.036, 1, 0.008, E.shade(tone.lit, 0.82));
  }

  // A real pediment, above the roofline rather than painted on the wall.
  function pediment(faces, tone) {
    var l = E.facePt(faces.L, 0.02, 0), r = E.facePt(faces.L, 0.98, 0);
    var rise = Math.abs(r.x - l.x) * 0.17;
    var apex = { x: (l.x + r.x) / 2, y: (l.y + r.y) / 2 - rise };
    E.poly(ctx, [l, r, apex], E.mix(tone.base, '#ffffff', 0.34));
    E.poly(ctx, [
      { x: l.x + (apex.x - l.x) * 0.1, y: l.y + (apex.y - l.y) * 0.1 },
      { x: r.x + (apex.x - r.x) * 0.1, y: r.y + (apex.y - r.y) * 0.1 },
      { x: apex.x, y: apex.y + rise * 0.18 }
    ], E.shade(E.mix(tone.base, '#ffffff', 0.34), 0.93));
  }

  // wide shallow steps running the length of the frontage
  function steps(b, colour) {
    var y = b.y + b.d;
    E.slab(ctx, b.x + 0.35, y, b.w - 0.7, 0.62, 10, E.mix(colour, '#ffffff', 0.2));
    E.slab(ctx, b.x + 0.2, y + 0.18, b.w - 0.4, 0.56, 6, colour);
    E.slab(ctx, b.x + 0.05, y + 0.36, b.w - 0.1, 0.5, 2, E.shade(colour, 0.94));
  }

  // a flat canopy on posts, over the entrance
  function canopy(b, colour, uw) {
    var w = b.w * (uw || 0.5), x = b.x + (b.w - w) / 2, y = b.y + b.d;
    E.slab(ctx, x, y - 0.1, w, 1.15, 34, colour);
    E.slab(ctx, x + 0.06, y - 0.04, w - 0.12, 1.03, 31, E.shade(colour, 0.82));
    [[x + 0.15, y + 0.9], [x + w - 0.15, y + 0.9]].forEach(function (p) {
      var c = E.isoc(p[0], p[1]);
      ctx.fillStyle = '#8d8677';
      ctx.fillRect(c.x - 1.5, c.y - 33, 3, 33);
    });
  }

  // small plant on the roof — the thing that stops a flat top reading as a lid
  function roofUnits(b, n, colour) {
    for (var i = 0; i < n; i++) {
      var w = Math.min(1.4, b.w / 4);
      var x = b.x + b.w * (0.22 + i * 0.34), y = b.y + b.d * 0.3;
      E.box(ctx, {
        x: x, y: y, w: w, d: Math.min(1.2, b.d * 0.3), h: b.h + 12 + i * 3,
        color: colour || '#9aa0a2',
        left: E.mix('#7c8285', scene.sun.shade, 0.2),
        right: E.mix('#a9afb1', scene.sun.warm, 0.18),
        top: '#c3c8c9'
      });
    }
  }

  function shopfront(b, faces, tone) {
    var s = b.store;
    fr(faces.L, s.u, 0.80, s.w, 0.185, E.mix('#2b3742', tone.sky, 0.42));
    fr(faces.L, s.u + 0.01, 0.815, s.w - 0.02, 0.09, E.mix('#39485a', '#ffffff', 0.22));
    // awning, one of the few saturated things in the frame
    fr(faces.L, s.u - 0.01, 0.745, s.w + 0.02, 0.055, s.awning);
    fr(faces.L, s.u - 0.01, 0.788, s.w + 0.02, 0.014, E.shade(s.awning, 0.7));
    if (s.sign) fr(faces.L, s.u + 0.03, 0.70, s.w * 0.5, 0.032, s.sign);
  }

  /* ------------------------------------------------------- facade types */

  // Punched windows in a grid: the ordinary residential block, and the
  // fallback for anything that has not asked to look like something else.
  function paintFlats(b, faces, tone) {
    var cols = b.cols || 4, rows = b.rows || 5;
    var topPad = 0.05, bottomLimit = b.store ? 0.74 : 0.94;
    var cw = 0.9 / cols, ch = (bottomLimit - topPad) / rows;

    for (var r = 0; r < rows; r++) {
      var v = topPad + r * ch + ch * 0.2, vh = ch * 0.56;
      for (var c = 0; c < cols; c++) {
        var u = 0.05 + c * cw + cw * 0.18, uw = cw * 0.64;
        var h1 = E.hash(b.seed + c * 3.1, r * 5.7 + b.seed * 0.3);
        var glass = pane(tone, r / rows, h1);
        fr(faces.L, u, v, uw, vh, h1 < (b.glass || 0.18) ? E.mix(glass, '#ffffff', 0.35) : glass);
        // reveal: a lit sill under each opening sells the wall thickness
        fr(faces.L, u, v + vh, uw, ch * 0.08, E.mix(tone.shade, '#ffffff', 0.22));
        // a few blinds, warm against all that cool glass
        if (h1 > 0.88) fr(faces.L, u, v, uw, vh * 0.55, E.mix('#e8d3ae', scene.sun.warm, 0.4));
      }

      // balcony: a recessed shadow with a pale rail across it
      if (b.balcony && r > 0) {
        fr(faces.L, 0.06, v + vh + ch * 0.1, 0.88, ch * 0.14, E.shade(tone.shade, 0.84));
        fr(faces.L, 0.06, v + vh + ch * 0.1, 0.88, ch * 0.035, E.mix(tone.shade, '#ffffff', 0.32));
        for (var q = 0; q < 9; q++) {
          fr(faces.L, 0.08 + q * 0.1, v + vh + ch * 0.1, 0.006, ch * 0.14, E.mix(tone.shade, '#ffffff', 0.18));
        }
      }

      fr(faces.L, 0.03, topPad + r * ch - ch * 0.06, 0.94, ch * 0.05, E.shade(tone.shade, 0.9));

      // the sunlit return wall gets a sparser grid, brighter glass
      for (var k = 0; k < 2; k++) {
        var h2 = E.hash(b.seed * 1.7 + k, r * 4.4);
        fr(faces.R, 0.22 + k * 0.4, v, 0.26, vh,
           E.mix(E.mix('#4c6a86', tone.sky, 0.5), '#ffffff', 0.28 + h2 * 0.3));
        fr(faces.R, 0.22 + k * 0.4, v + vh, 0.26, ch * 0.07, E.mix(tone.lit, '#ffffff', 0.3));
      }
    }
    parapet(faces, tone);
  }

  // Ribbon glazing and spandrel panels: no punched holes anywhere, which is
  // the whole difference between an office and a place people live.
  function paintOffice(b, faces, tone) {
    var rows = Math.max(4, b.rows || 8), ch = 0.9 / rows;
    for (var r = 0; r < rows; r++) {
      var v = 0.05 + r * ch;
      var g = pane(tone, r / rows, 0.6);
      fr(faces.L, 0.04, v + ch * 0.14, 0.92, ch * 0.5, g);
      fr(faces.L, 0.04, v + ch * 0.64, 0.92, ch * 0.26, E.mix(tone.shade, '#ffffff', 0.12));
      for (var m = 1; m < 9; m++) {
        fr(faces.L, 0.04 + m * 0.102, v + ch * 0.14, 0.005, ch * 0.5, E.shade(tone.shade, 0.85));
      }
      fr(faces.R, 0.06, v + ch * 0.14, 0.88, ch * 0.5,
         E.mix(E.mix('#4c6a86', tone.sky, 0.55), '#ffffff', 0.34));
      fr(faces.R, 0.06, v + ch * 0.64, 0.88, ch * 0.26, E.mix(tone.lit, '#ffffff', 0.18));
    }
    parapet(faces, tone);
  }

  // Stone, a giant order of columns, and a pediment over the roofline.
  function paintCivic(b, faces, tone) {
    var stone = E.mix(b.color, '#ffffff', 0.2);
    var n = Math.max(4, Math.min(9, Math.round(b.w)));
    var slot = 0.86 / n;

    // rusticated base, coursed
    fr(faces.L, 0, 0.74, 1, 0.26, E.shade(tone.shade, 0.95));
    for (var i = 0; i < 4; i++) fr(faces.L, 0, 0.78 + i * 0.05, 1, 0.005, E.shade(tone.shade, 0.8));

    // the shadowed wall behind the colonnade, then the columns in front of it
    fr(faces.L, 0.06, 0.30, 0.88, 0.46, E.shade(tone.shade, 0.72));
    for (var c = 0; c < n; c++) {
      var u = 0.07 + c * slot;
      fr(faces.L, u, 0.32, slot * 0.44, 0.42, stone);
      fr(faces.L, u, 0.32, slot * 0.13, 0.42, E.mix(stone, '#ffffff', 0.22));
      fr(faces.L, u - 0.006, 0.30, slot * 0.44 + 0.012, 0.022, E.mix(stone, '#ffffff', 0.34));
      fr(faces.L, u - 0.006, 0.735, slot * 0.44 + 0.012, 0.022, E.shade(stone, 0.9));
    }

    // doors, deep in the shade at the top of the steps
    fr(faces.L, 0.43, 0.55, 0.14, 0.21, E.shade(b.color, 0.42));
    fr(faces.L, 0.495, 0.55, 0.008, 0.21, E.shade(b.color, 0.6));

    // the return wall is plain stone with tall openings
    for (var k = 0; k < 3; k++) {
      fr(faces.R, 0.12 + k * 0.28, 0.36, 0.14, 0.32, pane(tone, 0.4, 0.3));
      fr(faces.R, 0.12 + k * 0.28, 0.68, 0.14, 0.02, E.mix(tone.lit, '#ffffff', 0.3));
    }

    cornice(faces, tone, 0.24);
    pediment(faces, tone);
  }

  // A clock tower over the middle of the roof.
  function civicRoof(b, tone) {
    if (!b.tower) return;
    var tw = Math.max(1.6, Math.min(3, b.w * 0.24));
    var x = b.x + b.w / 2 - tw / 2, y = b.y + b.d / 2 - tw / 2;
    var stone = E.mix(b.color, '#ffffff', 0.14);
    var f = E.box(ctx, {
      x: x, y: y, w: tw, d: tw, h: b.h + 78,
      color: stone,
      left: E.mix(E.shade(stone, 0.72), scene.sun.shade, 0.28),
      right: E.mix(stone, scene.sun.warm, 0.24),
      top: E.mix(stone, '#ffffff', 0.3)
    });
    // belfry openings
    fr(f.L, 0.22, 0.03, 0.24, 0.05, E.shade(stone, 0.45));
    fr(f.L, 0.54, 0.03, 0.24, 0.05, E.shade(stone, 0.45));
    // clock, and hands at ten past eleven
    var p = E.facePt(f.L, 0.5, 0.1);
    var rad = Math.max(7, tw * 4.4);
    ctx.beginPath(); ctx.arc(p.x, p.y, rad, 0, Math.PI * 2);
    ctx.fillStyle = '#efe7d2'; ctx.fill();
    ctx.strokeStyle = E.shade(stone, 0.6); ctx.lineWidth = 1.6; ctx.stroke();
    ctx.strokeStyle = '#3b332a'; ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.moveTo(p.x, p.y); ctx.lineTo(p.x - rad * 0.5, p.y - rad * 0.3);
    ctx.moveTo(p.x, p.y); ctx.lineTo(p.x + rad * 0.1, p.y - rad * 0.62);
    ctx.stroke();
    // spire
    var t = E.iso(x, y), t2 = E.iso(x + tw, y + tw);
    var mid = { x: (t.x + t2.x) / 2, y: (t.y + t2.y) / 2 - (b.h + 78) };
    E.poly(ctx, [
      { x: mid.x - tw * 16, y: mid.y }, { x: mid.x + tw * 16, y: mid.y },
      { x: mid.x, y: mid.y - 34 }
    ], E.mix('#6f7a6a', scene.sun.shade, 0.2));
  }

  // Mostly wall, by design: pilasters, a deep cornice, small high windows
  // and one arched way in.
  function paintBank(b, faces, tone) {
    var stone = E.mix(b.color, '#ffffff', 0.18);
    var n = Math.max(5, Math.min(11, Math.round(b.w * 1.4)));
    var slot = 0.9 / n;

    fr(faces.L, 0, 0.80, 1, 0.20, E.shade(tone.shade, 0.9));
    for (var c = 0; c < n; c++) {
      var u = 0.05 + c * slot;
      fr(faces.L, u, 0.16, slot * 0.34, 0.64, E.mix(stone, '#ffffff', 0.08));
      // the recess between pilasters, with one small window high in it
      if (c < n - 1) {
        fr(faces.L, u + slot * 0.4, 0.16, slot * 0.52, 0.64, E.shade(tone.shade, 0.92));
        fr(faces.L, u + slot * 0.5, 0.24, slot * 0.32, 0.16, pane(tone, 0.25, 0.2));
        fr(faces.L, u + slot * 0.5, 0.40, slot * 0.32, 0.014, E.mix(stone, '#ffffff', 0.3));
      }
    }

    // arched entrance
    var arch = [];
    for (var i = 0; i <= 10; i++) {
      var t = i / 10;
      arch.push(E.facePt(faces.L, 0.42 + t * 0.16, 0.60 - Math.sin(t * Math.PI) * 0.07));
    }
    arch.push(E.facePt(faces.L, 0.58, 0.96));
    arch.push(E.facePt(faces.L, 0.42, 0.96));
    E.poly(ctx, arch, E.shade(b.color, 0.4));
    E.poly(ctx, arch.slice(0, 11).concat([
      E.facePt(faces.L, 0.575, 0.62), E.facePt(faces.L, 0.425, 0.62)
    ]), E.mix('#b08a3f', '#ffffff', 0.15));

    // name band under the cornice
    fr(faces.L, 0.16, 0.115, 0.68, 0.035, E.mix('#8a6a34', '#ffffff', 0.1));
    for (var k = 0; k < 4; k++) {
      fr(faces.R, 0.1 + k * 0.22, 0.2, 0.1, 0.5, E.mix(tone.lit, '#ffffff', 0.12));
    }
    cornice(faces, tone, 0.06);
  }

  // Low, brick, one long band of high windows and a door big enough to get
  // equipment through. Everything about it says shed rather than office.
  function paintGym(b, faces, tone) {
    var brick = tone.shade;
    for (var i = 1; i < 14; i++) {
      fr(faces.L, 0, 0.04 + i * 0.07, 1, 0.006, E.shade(brick, 0.9));
    }
    // clerestory: one continuous band, mullioned
    fr(faces.L, 0.05, 0.16, 0.9, 0.19, pane(tone, 0.2, 0.55));
    for (var m = 1; m < 10; m++) fr(faces.L, 0.05 + m * 0.09, 0.16, 0.007, 0.19, E.shade(brick, 0.8));
    fr(faces.L, 0.05, 0.35, 0.9, 0.018, E.mix(brick, '#ffffff', 0.24));

    // sign board with a barbell on it
    fr(faces.L, 0.08, 0.42, 0.44, 0.1, E.mix('#2f3640', scene.sun.shade, 0.2));
    var a = E.facePt(faces.L, 0.14, 0.47), z = E.facePt(faces.L, 0.46, 0.47);
    ctx.strokeStyle = '#e0d6c2'; ctx.lineWidth = 2.4;
    ctx.beginPath(); ctx.moveTo(a.x, a.y); ctx.lineTo(z.x, z.y); ctx.stroke();
    [a, z].forEach(function (p) {
      ctx.fillStyle = '#e0d6c2';
      ctx.beginPath(); ctx.ellipse(p.x, p.y, 3.4, 4.6, 0, 0, Math.PI * 2); ctx.fill();
    });

    // roller shutter, slatted
    fr(faces.L, 0.6, 0.52, 0.32, 0.44, E.shade(brick, 0.72));
    for (var s = 0; s < 9; s++) {
      fr(faces.L, 0.6, 0.54 + s * 0.048, 0.32, 0.012, E.shade(brick, 0.62));
    }
    // a lit strip of the room behind, over the door
    fr(faces.L, 0.08, 0.56, 0.44, 0.3, E.shade(brick, 0.86));
    fr(faces.L, 0.12, 0.6, 0.36, 0.2, E.mix('#e8d3ae', scene.sun.warm, 0.35));
    fr(faces.R, 0.1, 0.2, 0.8, 0.16, pane(tone, 0.2, 0.5));
    parapet(faces, tone);
  }

  function gymRoof(b) { roofUnits(b, 2, '#8f9a8e'); }

  // Corrugated metal, loading bays, and a sign nobody has cleaned.
  function paintWorks(b, faces, tone) {
    var n = Math.round(b.w * 4);
    for (var i = 1; i < n; i++) {
      fr(faces.L, i / n, 0.03, 0.004, 0.94, E.shade(tone.shade, 0.88));
    }
    for (var j = 1; j < 10; j++) {
      fr(faces.R, j / 10, 0.03, 0.005, 0.94, E.shade(tone.lit, 0.88));
    }
    // a strip of high windows, the only daylight the floor gets
    fr(faces.L, 0.06, 0.1, 0.88, 0.1, pane(tone, 0.12, 0.4));
    for (var m = 1; m < 8; m++) fr(faces.L, 0.06 + m * 0.11, 0.1, 0.006, 0.1, E.shade(tone.shade, 0.8));

    // loading bays with a concrete lip and roller doors
    fr(faces.L, 0.05, 0.62, 0.9, 0.34, E.shade(tone.shade, 0.94));
    for (var k = 0; k < 3; k++) {
      var u = 0.09 + k * 0.3;
      fr(faces.L, u, 0.64, 0.22, 0.3, E.shade(tone.shade, 0.6));
      for (var s = 0; s < 6; s++) fr(faces.L, u, 0.66 + s * 0.045, 0.22, 0.012, E.shade(tone.shade, 0.52));
      fr(faces.L, u - 0.01, 0.94, 0.24, 0.02, E.mix(tone.shade, '#ffffff', 0.22));
    }
    fr(faces.L, 0.06, 0.24, 0.3, 0.05, E.mix('#5b6b74', '#ffffff', 0.14));
    parapet(faces, tone);
  }

  function worksRoof(b) { roofUnits(b, 3, '#98a09b'); }

  // Glass front to front, a canopy the length of the kerb, and a tower.
  function paintTerminal(b, faces, tone) {
    var bays = Math.round(b.w * 1.2);
    for (var i = 0; i < bays; i++) {
      var u = 0.02 + i * (0.96 / bays);
      fr(faces.L, u, 0.16, 0.96 / bays * 0.86, 0.72, pane(tone, 0.3, 0.75));
      fr(faces.L, u, 0.16, 0.96 / bays * 0.86, 0.06, E.mix('#ffffff', tone.sky, 0.3));
    }
    fr(faces.L, 0.02, 0.5, 0.96, 0.012, E.mix(tone.shade, '#ffffff', 0.4));
    fr(faces.L, 0.02, 0.1, 0.96, 0.05, E.mix(tone.lit, '#ffffff', 0.25));
    fr(faces.R, 0.06, 0.16, 0.88, 0.72, E.mix(E.mix('#4c6a86', tone.sky, 0.6), '#ffffff', 0.38));
    parapet(faces, tone);
  }

  function terminalRoof(b, tone) {
    var tw = 2.2;
    var x = b.x + b.w - 4, y = b.y + b.d * 0.3;
    var f = E.box(ctx, {
      x: x, y: y, w: tw, d: tw, h: b.h + 96,
      color: '#c2c8cc',
      left: E.mix('#98a0a6', scene.sun.shade, 0.24),
      right: E.mix('#cdd3d7', scene.sun.warm, 0.2),
      top: '#dfe4e7'
    });
    fr(f.L, 0.1, 0.02, 0.8, 0.05, pane(tone, 0.1, 0.8));
    // the cab, wider than the shaft
    var cab = E.box(ctx, {
      x: x - 0.45, y: y - 0.45, w: tw + 0.9, d: tw + 0.9, h: b.h + 118,
      color: '#38414a',
      left: E.mix('#5c7186', scene.sun.shade, 0.2),
      right: E.mix('#7d93a6', scene.sun.warm, 0.2),
      top: '#454f58'
    });
    fr(cab.L, 0.06, 0.12, 0.88, 0.5, E.mix('#8fb8d8', '#ffffff', 0.3));
    fr(cab.R, 0.06, 0.12, 0.88, 0.5, E.mix('#a8cbe4', '#ffffff', 0.34));
  }

  function terminalFront(b) { canopy(b, '#c9ced2', 0.72); }

  // Render, a canopy over the door, and a cross you can see from the road.
  function paintClinic(b, faces, tone) {
    paintFlats({ x: b.x, y: b.y, w: b.w, d: b.d, h: b.h, seed: b.seed, cols: b.cols, rows: b.rows, glass: 0.3 },
               faces, tone);
    fr(faces.L, 0.06, 0.1, 0.88, 0.05, E.mix('#ffffff', tone.base, 0.25));
    var c = E.facePt(faces.L, 0.85, 0.22);
    ctx.fillStyle = '#3f6b57';
    ctx.fillRect(c.x - 3, c.y - 9, 6, 18);
    ctx.fillRect(c.x - 9, c.y - 3, 18, 6);
  }

  function clinicFront(b) { canopy(b, '#dcd5c4', 0.42); }

  // Tall arched windows and a short flight of steps.
  function paintLibrary(b, faces, tone) {
    var stone = E.mix(b.color, '#ffffff', 0.16);
    fr(faces.L, 0, 0.82, 1, 0.18, E.shade(tone.shade, 0.93));
    var n = 5, slot = 0.86 / n;
    for (var c = 0; c < n; c++) {
      var u = 0.07 + c * slot, w = slot * 0.56;
      var arch = [];
      for (var i = 0; i <= 8; i++) {
        var t = i / 8;
        arch.push(E.facePt(faces.L, u + t * w, 0.34 - Math.sin(t * Math.PI) * 0.09));
      }
      arch.push(E.facePt(faces.L, u + w, 0.78));
      arch.push(E.facePt(faces.L, u, 0.78));
      E.poly(ctx, arch, pane(tone, 0.35, 0.35));
      fr(faces.L, u, 0.78, w, 0.02, E.mix(stone, '#ffffff', 0.3));
      fr(faces.L, u + w / 2 - 0.004, 0.28, 0.008, 0.5, E.mix(stone, '#ffffff', 0.2));
    }
    for (var k = 0; k < 3; k++) fr(faces.R, 0.14 + k * 0.28, 0.3, 0.12, 0.4, pane(tone, 0.35, 0.3));
    cornice(faces, tone, 0.14);
    parapet(faces, tone);
  }

  function libraryFront(b) { steps(b, '#c9c2ad'); }
  function civicFront(b) { steps(b, '#d2ccb8'); }

  // Low retail: the whole ground floor is glass, and a blade sign sticks out
  // over the pavement so you can read it from down the street.
  function paintShop(b, faces, tone) {
    var rows = Math.max(1, (b.rows || 2) - 1), ch = 0.52 / rows;
    for (var r = 0; r < rows; r++) {
      var v = 0.08 + r * ch;
      for (var c = 0; c < 4; c++) {
        var h1 = E.hash(b.seed + c * 2.7, r * 3.3);
        fr(faces.L, 0.09 + c * 0.22, v, 0.14, ch * 0.62, pane(tone, r / rows, h1));
        fr(faces.L, 0.09 + c * 0.22, v + ch * 0.62, 0.14, ch * 0.08, E.mix(tone.shade, '#ffffff', 0.24));
      }
    }
    // shopfront: glazing, stall riser, and the dark of the room behind
    fr(faces.L, 0.04, 0.66, 0.92, 0.3, E.mix('#2b3742', tone.sky, 0.36));
    fr(faces.L, 0.06, 0.69, 0.88, 0.16, E.mix('#39485a', '#ffffff', 0.26));
    fr(faces.L, 0.04, 0.93, 0.92, 0.05, E.shade(tone.shade, 0.8));
    fr(faces.L, 0.44, 0.68, 0.12, 0.28, E.shade(b.color, 0.5));
    if (!b.store) fr(faces.L, 0.04, 0.6, 0.92, 0.045, E.mix(tone.shade, '#ffffff', 0.3));
    parapet(faces, tone);
  }

  function shopFront(b) {
    if (!b.blade) return;
    // a sign hung square to the wall, out over the pavement
    var x = b.x + b.w * 0.78, y = b.y + b.d;
    var p = E.isoc(x, y), h = b.h * 0.62;
    ctx.fillStyle = E.mix(b.blade, scene.sun.shade, 0.1);
    ctx.fillRect(p.x - 2, p.y - h - 26, 4, 26);
    E.poly(ctx, [
      { x: p.x - 3, y: p.y - h - 24 }, { x: p.x + 26, y: p.y - h - 14 },
      { x: p.x + 26, y: p.y - h + 6 }, { x: p.x - 3, y: p.y - h - 4 }
    ], b.blade);
    E.poly(ctx, [
      { x: p.x + 2, y: p.y - h - 20 }, { x: p.x + 21, y: p.y - h - 13 },
      { x: p.x + 21, y: p.y - h - 5 }, { x: p.x + 2, y: p.y - h - 12 }
    ], E.mix(b.blade, '#ffffff', 0.5));
  }

  var TYPES = {
    flats: { face: paintFlats },
    office: { face: paintOffice },
    civic: { face: paintCivic, roof: civicRoof, front: civicFront },
    bank: { face: paintBank },
    gym: { face: paintGym, roof: gymRoof },
    works: { face: paintWorks, roof: worksRoof },
    terminal: { face: paintTerminal, roof: terminalRoof, front: terminalFront },
    clinic: { face: paintClinic, front: clinicFront },
    library: { face: paintLibrary, front: libraryFront },
    shop: { face: paintShop, front: shopFront },
  };

  function drawOpenings(b, faces) {
    if (!b.openings) return;
    b.openings.forEach(function (o) {
      var q = b.side ? faces.R : faces.L;
      if (o.kind === 'window') {
        var wp = facePts(q, o.u, 0.14, o.w, 0.36);
        E.poly(ctx, wp, E.mix('#9dc4e0', '#ffffff', 0.45));
        E.poly(ctx, facePts(q, o.u, 0.14, o.w, 0.10), '#ffffff');
        var frame = facePts(q, o.u + o.w / 2 - 0.005, 0.14, 0.01, 0.36);
        E.poly(ctx, frame, '#e6dcc9');
        emissive.push({ pts: wp, color: '#fff4d8', a: 0.22, glow: 0.5 });
      } else {
        E.poly(ctx, facePts(q, o.u, 0.40, o.w, 0.60), E.shade(b.color, 0.55));
      }
    });
  }

  /* ---------------------------------------------- sorted object layer */

  function drawObjects() {
    var list = [];

    (scene.boxes || []).forEach(function (b) {
      // centre-of-footprint key: for grid-aligned blocks next to streets this
      // orders correctly against people on either side of the wall
      var k = (b.x + b.w / 2) + (b.y + b.d / 2);
      if (b.cols || b.wall) {
        if (offscreen(b)) return;
        list.push({ k: k, f: function () { drawBuilding(b); } });
      } else {
        list.push({ k: k, f: function () { drawFurniture(b); } });
      }
    });

    (scene.fixtures || []).forEach(function (f) {
      if (f.kind !== 'railing' && !onScreen(f.x, f.y)) return;
      var k = f.kind === 'sunbeam' ? -9999 : f.kind === 'rug' ? -9000 : f.x + f.y;
      list.push({ k: k, f: function () { drawFixture(f); } });
    });

    actors.forEach(function (a) {
      if (!onScreen(a.x, a.y, 120)) return;
      list.push({ k: a.x + a.y, f: function () { E.actor(ctx, a, t); } });
    });
    list.push({ k: player.x + player.y, f: function () { E.actor(ctx, player, t); } });

    list.sort(function (a, b) { return a.k - b.k; });
    for (var i = 0; i < list.length; i++) list[i].f();
  }

  function drawFurniture(b) {
    E.box(ctx, {
      x: b.x, y: b.y, w: b.w, d: b.d, h: b.h,
      color: b.color,
      left: E.mix(E.shade(b.color, 0.76), scene.sun.shade, 0.22),
      right: E.mix(b.color, scene.sun.warm, 0.2),
      top: E.mix(b.color, '#ffffff', 0.24)
    });
  }

  function drawFixture(f) {
    switch (f.kind) {
      case 'lamp': E.lampPost(ctx, f.x, f.y, { lit: false, color: '#4c5158' }); break;
      case 'tree': drawTree(f); break;
      case 'bench': E.bench(ctx, f.x, f.y, { sun: scene.sun }); break;
      case 'planter': E.planter(ctx, f.x, f.y, {}); break;
      case 'railing': E.railing(ctx, f.x, f.y, f.len, f.axis, f.color); break;
      case 'rug': E.rug(ctx, f.x, f.y, f.w, f.d, f.color, f.edge); break;
      case 'hydrant': drawHydrant(f); break;
      case 'bags': drawBags(f); break;
      case 'car': drawCar(f); break;
      case 'bus': drawBus(f); break;
      case 'busStop': drawBusStop(f); break;
      case 'stall': drawStall(f); break;
      case 'newsstand': drawNewsstand(f); break;
      case 'screen': drawScreen(f); break;
      case 'plane': drawPlane(f); break;
      case 'fountain': drawFountain(f); break;
      case 'boat': drawBoat(f); break;
      case 'bollard': drawBollard(f); break;
      case 'deskLamp': drawDeskLamp(f); break;
      case 'floorLamp': drawFloorLamp(f); break;
      case 'board': drawBoard(f); break;
      case 'mug': drawMug(f); break;
      case 'plant': drawPlant(f); break;
      case 'sunbeam': drawSunbeam(f); break;
    }
  }

  // a shaft of morning light on the floorboards — the interior's key light
  function drawSunbeam(f) {
    var o = sunOff(96);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    ctx.globalAlpha = quality === 'high' ? 0.22 : 0.15;
    ctx.translate(o.x, o.y);
    E.slab(ctx, f.x, f.y, f.w, f.d, 0, '#ffe6b4');
    ctx.globalAlpha = 0.11;
    E.slab(ctx, f.x - 0.25, f.y - 0.25, f.w + 0.5, f.d + 0.5, 0, '#ffdda0');
    ctx.restore();
  }

  function drawTree(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#6b5540';
    ctx.fillRect(c.x - 3, c.y - 52, 6, 52);
    var leaf = f.lush ? '#4f7f42' : '#5d8546';
    var seedBase = f.x * 3.7 + f.y;
    for (var i = 0; i < 9; i++) {
      var n1 = E.hash(seedBase + i, i * 2.1), n2 = E.hash(i * 5.3, seedBase);
      var a = (i / 9) * Math.PI * 2 + n1;
      var sway = Math.sin(t * 0.9 + i + f.x) * 1.6;
      var rx = 20 + n2 * 12, ry = 14 + n1 * 8;
      var px = c.x + Math.cos(a) * (10 + n2 * 12) + sway;
      var py = c.y - 62 + Math.sin(a) * (7 + n1 * 8);
      // sun catches the upper-right of the canopy
      var tone = (Math.cos(a) > 0 && Math.sin(a) < 0)
        ? E.mix(leaf, scene.sun.warm, 0.34)
        : E.mix(leaf, scene.sun.shade, 0.22 + n1 * 0.2);
      ctx.fillStyle = tone;
      ctx.beginPath();
      ctx.ellipse(px, py, rx, ry, 0, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  function drawPlant(f) {
    var c = E.isoc(f.x, f.y);
    E.box(ctx, { x: f.x, y: f.y, w: 0.7, d: 0.7, h: 18, color: '#a2714c' });
    for (var i = 0; i < 6; i++) {
      var a = (i / 6) * Math.PI * 2;
      var sway = Math.sin(t * 1.1 + i) * 1.2;
      ctx.strokeStyle = i % 2 ? '#527f46' : '#69965a';
      ctx.lineWidth = 3;
      ctx.beginPath();
      ctx.moveTo(c.x, c.y - 18);
      ctx.quadraticCurveTo(c.x + Math.cos(a) * 10, c.y - 40, c.x + Math.cos(a) * 18 + sway, c.y - 34 + Math.sin(a) * 6);
      ctx.stroke();
    }
  }

  function drawHydrant(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#a8543c';
    ctx.fillRect(c.x - 4, c.y - 18, 8, 18);
    ctx.beginPath(); ctx.arc(c.x, c.y - 19, 4.5, 0, Math.PI * 2); ctx.fill();
    ctx.fillStyle = E.mix('#a8543c', '#ffffff', 0.3);
    ctx.fillRect(c.x + 1, c.y - 18, 3, 18);
  }

  function drawBags(f) {
    var c = E.isoc(f.x, f.y);
    ctx.save();
    ctx.translate(c.x, c.y);
    [[-9, 0, 11], [4, -2, 13], [14, 1, 9]].forEach(function (b, i) {
      ctx.fillStyle = i % 2 ? '#4a4a52' : '#565660';
      ctx.beginPath();
      ctx.ellipse(b[0], b[1] - b[2] * 0.55, b[2], b[2] * 0.9, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = E.rgba('#ffffff', 0.12);
      ctx.beginPath();
      ctx.ellipse(b[0] + 3, b[1] - b[2] * 0.8, b[2] * 0.4, b[2] * 0.3, 0, 0, Math.PI * 2);
      ctx.fill();
    });
    ctx.restore();
  }

  function drawCar(f) {
    var w = 2.4, d = 1.1, body = f.color;
    [[0.25, 0.05], [0.25, 1.05], [2.15, 0.05], [2.15, 1.05]].forEach(function (o) {
      var p = E.isoc(f.x + o[0], f.y + o[1]);
      ctx.fillStyle = '#2b2a2c';
      ctx.beginPath(); ctx.ellipse(p.x, p.y - 4, 6, 5, 0, 0, Math.PI * 2); ctx.fill();
    });

    E.box(ctx, {
      x: f.x, y: f.y, w: w, d: d, h: 15, color: body,
      left: E.mix(E.shade(body, 0.74), scene.sun.shade, 0.24),
      right: E.mix(body, scene.sun.warm, 0.26),
      top: E.mix(body, '#ffffff', 0.3)
    });
    var cab = E.box(ctx, {
      x: f.x + 0.55, y: f.y + 0.13, w: w - 1.2, d: d - 0.26, h: 29,
      color: body,
      left: E.mix(E.shade(body, 0.7), scene.sun.shade, 0.26),
      right: E.mix(body, scene.sun.warm, 0.2),
      top: E.mix(body, '#ffffff', 0.34)
    });
    E.faceRect(ctx, cab.L, 0.08, 0.06, 0.84, 0.44, E.mix('#37475a', scene.sky[0], 0.4));
    E.faceRect(ctx, cab.R, 0.12, 0.06, 0.76, 0.44, E.mix('#8fb8d8', '#ffffff', 0.35));
  }

  function drawBus(f) {
    var w = 4.4, d = 1.5, body = f.color;
    [[0.5, 0.1], [0.5, 1.4], [3.6, 0.1], [3.6, 1.4]].forEach(function (o) {
      var p2 = E.isoc(f.x + o[0], f.y + o[1]);
      ctx.fillStyle = '#2b2a2c';
      ctx.beginPath(); ctx.ellipse(p2.x, p2.y - 5, 7, 6, 0, 0, Math.PI * 2); ctx.fill();
    });
    var shell = E.box(ctx, {
      x: f.x, y: f.y, w: w, d: d, h: 46, color: body,
      left: E.mix(E.shade(body, 0.74), scene.sun.shade, 0.24),
      right: E.mix(body, scene.sun.warm, 0.26),
      top: E.mix(body, '#ffffff', 0.3)
    });
    for (var i = 0; i < 5; i++) {
      E.faceRect(ctx, shell.L, 0.06 + i * 0.185, 0.14, 0.14, 0.34,
                 E.mix('#37475a', scene.sky[0], 0.45));
    }
    E.faceRect(ctx, shell.R, 0.12, 0.14, 0.72, 0.34, E.mix('#8fb8d8', '#ffffff', 0.32));
    E.faceRect(ctx, shell.L, 0.06, 0.04, 0.36, 0.07, E.mix(body, '#ffffff', 0.6));
  }

  function drawBusStop(f) {
    var frame = E.box(ctx, {
      x: f.x, y: f.y, w: 2.6, d: 1.0, h: 54, color: '#8f979d',
      left: 'rgba(180,205,225,0.34)', right: 'rgba(200,222,238,0.44)',
      top: E.mix('#c3ccd4', '#ffffff', 0.35)
    });
    E.faceRect(ctx, frame.L, 0.0, 0.0, 0.03, 1, '#77808a');
    E.faceRect(ctx, frame.L, 0.97, 0.0, 0.03, 1, '#77808a');
    E.faceRect(ctx, frame.L, 0.10, 0.18, 0.3, 0.4, E.mix('#e0a648', '#ffffff', 0.2));
    var c = E.isoc(f.x + 2.85, f.y + 0.5);
    ctx.fillStyle = '#77808a';
    ctx.fillRect(c.x - 1.5, c.y - 60, 3, 60);
    E.poly(ctx, [{ x: c.x - 9, y: c.y - 74 }, { x: c.x + 9, y: c.y - 70 },
                 { x: c.x + 9, y: c.y - 58 }, { x: c.x - 9, y: c.y - 62 }], '#e8e2d4');
  }

  function drawStall(f) {
    var top = E.box(ctx, {
      x: f.x, y: f.y, w: 1.7, d: 1.5, h: 34, color: '#b99a72',
      left: E.mix('#8c7250', scene.sun.shade, 0.24),
      right: E.mix('#c9a97e', scene.sun.warm, 0.22),
      top: '#d8c39c'
    });
    // produce on the counter
    for (var i = 0; i < 7; i++) {
      var n = E.hash(f.x * 3.1 + i, i * 2.7);
      var p2 = E.facePt(top.T, 0.15 + (i % 4) * 0.24, 0.2 + Math.floor(i / 4) * 0.42);
      ctx.fillStyle = ['#c8562f', '#5f8a3c', '#d3a234', '#8e4f7a'][i % 4];
      ctx.beginPath(); ctx.ellipse(p2.x, p2.y - 2, 5 + n * 3, 4 + n * 2, 0, 0, Math.PI * 2); ctx.fill();
    }
    // striped awning above
    var aw = f.awning || '#a8503f';
    var A = E.iso(f.x - 0.1, f.y - 0.1), B = E.iso(f.x + 1.8, f.y - 0.1),
        C = E.iso(f.x + 1.8, f.y + 1.6), D = E.iso(f.x - 0.1, f.y + 1.6);
    var lift = 62;
    E.poly(ctx, [{ x: A.x, y: A.y - lift }, { x: B.x, y: B.y - lift },
                 { x: C.x, y: C.y - lift }, { x: D.x, y: D.y - lift }], aw);
    E.poly(ctx, [{ x: D.x, y: D.y - lift }, { x: C.x, y: C.y - lift },
                 { x: C.x, y: C.y - lift + 7 }, { x: D.x, y: D.y - lift + 7 }], E.shade(aw, 0.72));
    [[f.x, f.y], [f.x + 1.7, f.y], [f.x, f.y + 1.5], [f.x + 1.7, f.y + 1.5]].forEach(function (o) {
      var pc = E.isoc(o[0], o[1]);
      ctx.fillStyle = '#6f6252';
      ctx.fillRect(pc.x - 1.5, pc.y - lift, 3, lift);
    });
  }

  // The newsstand. Other work is a rack of papers you stop at, which is why
  // there is no jobs button anywhere in this client.
  function drawNewsstand(f) {
    var body = E.box(ctx, {
      x: f.x, y: f.y, w: 1.5, d: 1.0, h: 40, color: '#6f7a6a',
      left: E.mix('#57614f', scene.sun.shade, 0.24),
      right: E.mix('#7f8a78', scene.sun.warm, 0.22),
      top: '#8d977f'
    });
    // papers on the slanted rack, facing the pavement
    for (var i = 0; i < 4; i++) {
      E.faceRect(ctx, body.L, 0.08 + i * 0.22, 0.18, 0.17, 0.46,
                 E.mix(i % 2 ? '#e6dcc6' : '#d8cdb4', scene.sky[0], 0.12));
      E.faceRect(ctx, body.L, 0.09 + i * 0.22, 0.24, 0.15, 0.05, '#8a8172');
      E.faceRect(ctx, body.L, 0.09 + i * 0.22, 0.34, 0.11, 0.03, '#a39a89');
    }
    // a canopy over the rack, and the post holding it up
    var A = E.iso(f.x - 0.1, f.y - 0.1), B = E.iso(f.x + 1.6, f.y - 0.1),
        C = E.iso(f.x + 1.6, f.y + 1.1), D = E.iso(f.x - 0.1, f.y + 1.1);
    var lift = 66;
    E.poly(ctx, [{ x: A.x, y: A.y - lift }, { x: B.x, y: B.y - lift },
                 { x: C.x, y: C.y - lift }, { x: D.x, y: D.y - lift }], '#4f5c4c');
    E.poly(ctx, [{ x: D.x, y: D.y - lift }, { x: C.x, y: C.y - lift },
                 { x: C.x, y: C.y - lift + 6 }, { x: D.x, y: D.y - lift + 6 }], '#3d483b');
    [[f.x, f.y], [f.x + 1.5, f.y], [f.x, f.y + 1.0], [f.x + 1.5, f.y + 1.0]].forEach(function (o) {
      var pc = E.isoc(o[0], o[1]);
      ctx.fillStyle = '#5c5648';
      ctx.fillRect(pc.x - 1.5, pc.y - lift, 3, lift);
    });
  }

  /* An aircraft on the apron. It is scenery and it is meant to be: you can
     see the thing that leaves and there is nothing on this map that pays for
     a seat on it, which is the economic ceiling stated in one object. */
  function drawPlane(f) {
    var s = f.small ? 0.62 : 1;
    var w = 7.2 * s, d = 1.5 * s, body = '#e9edf0';

    // wing, laid flat at cabin height and drawn before the fuselage
    E.slab(ctx, f.x + 1.9 * s, f.y - 2.6 * s, 1.7 * s, 6.4 * s, 13 * s,
           E.mix(body, scene.sun.shade, 0.14));

    var fus = E.box(ctx, {
      x: f.x, y: f.y, w: w, d: d, h: 17 * s, color: body,
      left: E.mix(E.shade(body, 0.84), scene.sun.shade, 0.2),
      right: E.mix(body, scene.sun.warm, 0.18),
      top: '#ffffff'
    });
    for (var i = 0; i < 9; i++) {
      E.faceRect(ctx, fus.L, 0.1 + i * 0.085, 0.3, 0.042, 0.24, '#5b7186');
    }
    E.faceRect(ctx, fus.L, 0.02, 0.24, 0.05, 0.3, E.mix('#5b7186', '#ffffff', 0.35));

    // tail fin, over the rear of the fuselage
    var t0 = E.iso(f.x + 0.35 * s, f.y + 0.7 * s);
    E.poly(ctx, [
      { x: t0.x, y: t0.y - 16 * s }, { x: t0.x + 15 * s, y: t0.y - 42 * s },
      { x: t0.x + 25 * s, y: t0.y - 42 * s }, { x: t0.x + 21 * s, y: t0.y - 16 * s }
    ], '#b9563f');

    // engine slung under the near wing
    var e0 = E.isoc(f.x + 2.9 * s, f.y + 2.4 * s);
    ctx.fillStyle = '#aeb6bd';
    ctx.beginPath();
    ctx.ellipse(e0.x, e0.y - 11 * s, 9 * s, 4.5 * s, 0, 0, Math.PI * 2);
    ctx.fill();
  }

  // The computer on the desk. Same job as the newsstand, indoors.
  function drawScreen(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#4a423a';
    ctx.fillRect(c.x - 2, c.y - 34, 4, 6);
    E.poly(ctx, [{ x: c.x - 14, y: c.y - 32 }, { x: c.x + 14, y: c.y - 36 },
                 { x: c.x + 14, y: c.y - 55 }, { x: c.x - 14, y: c.y - 51 }], '#3b3630');
    var glow = E.mix('#7fa3bd', scene.sun.warm, 0.18);
    E.poly(ctx, [{ x: c.x - 11, y: c.y - 34 }, { x: c.x + 11, y: c.y - 37 },
                 { x: c.x + 11, y: c.y - 53 }, { x: c.x - 11, y: c.y - 50 }], glow);
    emissive.push({
      pts: [{ x: c.x - 11, y: c.y - 34 }, { x: c.x + 11, y: c.y - 37 },
            { x: c.x + 11, y: c.y - 53 }, { x: c.x - 11, y: c.y - 50 }],
      color: '#cfe0ee', a: 0.16, glow: 0.4
    });
  }

  function drawFountain(f) {
    var c = E.isoc(f.x, f.y);
    ctx.save();
    ctx.translate(c.x, c.y);
    E.poly(ctx, [{ x: -46, y: 0 }, { x: 0, y: -23 }, { x: 46, y: 0 }, { x: 0, y: 23 }], '#c8bda6');
    E.poly(ctx, [{ x: -38, y: 0 }, { x: 0, y: -19 }, { x: 38, y: 0 }, { x: 0, y: 19 }], '#5f9dc0');
    var r = 0.5 + 0.5 * Math.sin(t * 1.4);
    E.poly(ctx, [{ x: -24, y: 0 }, { x: 0, y: -12 }, { x: 24, y: 0 }, { x: 0, y: 12 }],
           E.rgba('#ffffff', 0.10 + r * 0.12));
    ctx.fillStyle = '#c8bda6';
    ctx.fillRect(-4, -34, 8, 34);
    ctx.beginPath(); ctx.ellipse(0, -36, 11, 5, 0, 0, Math.PI * 2); ctx.fill();
    // a thin plume that never quite repeats
    ctx.strokeStyle = E.rgba('#ffffff', 0.5);
    ctx.lineWidth = 2;
    for (var i = 0; i < 5; i++) {
      var a = (i / 5) * Math.PI * 2 + t * 0.6;
      ctx.beginPath();
      ctx.moveTo(0, -40);
      ctx.quadraticCurveTo(Math.cos(a) * 9, -58 - r * 5, Math.cos(a) * 20, -36);
      ctx.stroke();
    }
    ctx.restore();
  }

  function drawBoat(f) {
    var c = E.isoc(f.x, f.y);
    var bob = Math.sin(t * 0.9 + f.x) * 2;
    ctx.save();
    ctx.translate(c.x, c.y + bob);
    E.poly(ctx, [{ x: -40, y: 0 }, { x: -22, y: -13 }, { x: 30, y: -6 }, { x: 40, y: 4 },
                 { x: 24, y: 12 }, { x: -30, y: 8 }], '#e3dbc9');
    E.poly(ctx, [{ x: -30, y: 8 }, { x: 24, y: 12 }, { x: 24, y: 17 }, { x: -30, y: 13 }], '#8f8674');
    E.poly(ctx, [{ x: -14, y: -6 }, { x: 6, y: -3 }, { x: 6, y: -17 }, { x: -14, y: -20 }], '#7fa3bd');
    ctx.strokeStyle = '#6f6252'; ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(2, -14); ctx.lineTo(6, -56); ctx.stroke();
    ctx.restore();
  }

  function drawBollard(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#6f7681';
    ctx.fillRect(c.x - 2.5, c.y - 14, 5, 14);
    ctx.fillStyle = '#98a0aa';
    ctx.beginPath(); ctx.ellipse(c.x, c.y - 14, 2.8, 1.6, 0, 0, Math.PI * 2); ctx.fill();
  }

  function drawDeskLamp(f) {
    var c = E.isoc(f.x, f.y);
    ctx.strokeStyle = '#3d3630'; ctx.lineWidth = 3;
    ctx.beginPath(); ctx.moveTo(c.x, c.y - 30); ctx.quadraticCurveTo(c.x + 2, c.y - 48, c.x + 14, c.y - 50); ctx.stroke();
    E.poly(ctx, [{ x: c.x + 6, y: c.y - 52 }, { x: c.x + 22, y: c.y - 52 },
                 { x: c.x + 18, y: c.y - 42 }, { x: c.x + 10, y: c.y - 42 }], '#4a4038');
    emissive.push({ ellipse: { x: c.x + 14, y: c.y - 43 }, r: 5, color: '#ffd79a', a: 0.9, glow: 0.35 });
  }

  function drawFloorLamp(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#4a423a';
    ctx.fillRect(c.x - 1.5, c.y - 70, 3, 70);
    ctx.beginPath(); ctx.ellipse(c.x, c.y, 8, 3.5, 0, 0, Math.PI * 2); ctx.fill();
    E.poly(ctx, [{ x: c.x - 13, y: c.y - 88 }, { x: c.x + 13, y: c.y - 88 },
                 { x: c.x + 9, y: c.y - 68 }, { x: c.x - 9, y: c.y - 68 }], '#d9c9ae');
  }

  function drawBoard(f) {
    var a = E.iso(f.x, f.y), b = E.iso(f.x + 1.7, f.y);
    var top = 86, h = 40;
    E.poly(ctx, [{ x: a.x, y: a.y - top }, { x: b.x, y: b.y - top },
                 { x: b.x, y: b.y - top + h }, { x: a.x, y: a.y - top + h }], '#e6dcc6');
    for (var i = 0; i < 4; i++) {
      var u = 0.08 + i * 0.23;
      var p1 = { x: a.x + (b.x - a.x) * u, y: a.y + (b.y - a.y) * u - top + 8 };
      ctx.fillStyle = i < 2 ? '#d99a3f' : '#6f97c4';
      ctx.fillRect(p1.x, p1.y, 10, 22);
    }
  }

  function drawMug(f) {
    var c = E.isoc(f.x, f.y);
    ctx.fillStyle = '#e2d6bf';
    ctx.fillRect(c.x - 3, c.y - 36, 6, 7);
  }

  /* -------------------------------------------------------- emissive */

  function drawEmissive() {
    for (var i = 0; i < emissive.length; i++) {
      var e = emissive[i];
      if (e.pts) E.poly(ctx, e.pts, E.rgba(e.color, e.a));
      else if (e.ellipse) {
        ctx.fillStyle = E.rgba(e.color, e.a);
        ctx.beginPath();
        ctx.ellipse(e.ellipse.x, e.ellipse.y, e.r, e.r * 0.7, 0, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    if (quality !== 'high') return;
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    for (var j = 0; j < emissive.length; j++) {
      var q = emissive[j];
      if (!q.glow) continue;
      var cx, cy, rad;
      if (q.pts) {
        cx = (q.pts[0].x + q.pts[2].x) / 2; cy = (q.pts[0].y + q.pts[2].y) / 2;
        rad = Math.max(16, Math.hypot(q.pts[2].x - q.pts[0].x, q.pts[2].y - q.pts[0].y) * 1.5);
      } else {
        cx = q.ellipse.x; cy = q.ellipse.y; rad = q.r * 6;
      }
      var g = ctx.createRadialGradient(cx, cy, 0, cx, cy, rad);
      g.addColorStop(0, E.rgba(q.color, q.glow * 0.32));
      g.addColorStop(1, E.rgba(q.color, 0));
      ctx.fillStyle = g;
      ctx.fillRect(cx - rad, cy - rad, rad * 2, rad * 2);
    }
    ctx.restore();
  }

  // warm haze from the sun side, cool bounce in the near corner
  function sunWash() {
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    var g = ctx.createLinearGradient(W, 0, W * 0.15, H);
    g.addColorStop(0, 'rgba(255,224,170,0.16)');
    g.addColorStop(0.5, 'rgba(255,224,170,0.04)');
    g.addColorStop(1, 'rgba(255,224,170,0)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, W, H);
    ctx.restore();
  }

  /* ------------------------------------------------------- zone ring */

  function drawZoneRing() {
    if (!cardZone) return;
    var c = E.isoc(cardZone.x, cardZone.y);
    var pulse = 0.5 + 0.5 * Math.sin(t * 1.8);
    ctx.save();
    ctx.strokeStyle = E.rgba('#ffffff', 0.16 + pulse * 0.2);
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.ellipse(c.x, c.y, 42 + pulse * 6, 21 + pulse * 3, 0, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  }

  /* ------------------------------------------------------------ input */

  function onPointerDown(ev) {
    if (ev.button !== 0) return;
    // the floating card and the view buttons are UI, not world
    if (ev.target.closest('.world-card') || ev.target.closest('.view-controls')) return;
    drag = {
      sx: ev.clientX, sy: ev.clientY, cx: cam.x, cy: cam.y, moved: 0,
      onCanvas: ev.target === cvs
    };
    cvs.classList.add('dragging');
  }

  function onPointerMove(ev) {
    if (!drag) return;
    var dx = ev.clientX - drag.sx, dy = ev.clientY - drag.sy;
    drag.moved = Math.max(drag.moved, Math.abs(dx) + Math.abs(dy));
    if (drag.moved < 4) return;
    if (!cam.free) { cam.free = true; syncFollowChip(); }
    cam.x = drag.cx - dx / zoom;
    cam.y = drag.cy - dy / zoom;
    clampCam();
  }

  function onPointerUp(ev) {
    if (!drag) return;
    var moved = drag.moved, onCanvas = drag.onCanvas;
    drag = null;
    cvs.classList.remove('dragging');
    if (moved >= 4) {
      // a drag that happened to start on a marker must not also open it
      suppressClick = true;
      setTimeout(function () { suppressClick = false; }, 0);
      return;
    }
    if (onCanvas) onCanvasClick(ev);
  }

  // wheel zooms about the cursor, so you can pull in on something specific
  function onWheel(ev) {
    ev.preventDefault();
    var r = cvs.getBoundingClientRect();
    var sx = ev.clientX - r.left - W / 2, sy = ev.clientY - r.top - H / 2;
    var wx = sx / zoom + cam.x, wy = sy / zoom + cam.y;

    userZoom = E.clamp(userZoom * (ev.deltaY < 0 ? 1.12 : 1 / 1.12), 0.34, 3.2);
    zoom = baseZoom * userZoom;

    cam.x = wx - sx / zoom;
    cam.y = wy - sy / zoom;
    if (!cam.free) { cam.free = true; syncFollowChip(); }
    clampCam();
  }

  function onCanvasClick(ev) {
    var r = cvs.getBoundingClientRect();
    var sx = (ev.clientX - r.left - W / 2) / zoom + cam.x;
    var sy = (ev.clientY - r.top - H / 2) / zoom + cam.y - E.TH / 2;
    var gx = (sx / (E.TW / 2) + sy / (E.TH / 2)) / 2;
    var gy = (sy / (E.TH / 2) - sx / (E.TW / 2)) / 2;
    var route = routeTo(gx, gy);
    if (!route) return;                          // no way there on foot
    player.route = route;
    player.ri = 0;
    player.hold = 0;
  }

  /* ----------------------------------------------------------- panels */

  function openPanel(name) {
    var p = E.PANELS[name];
    if (!p) return;
    var ov = document.getElementById('overlay');
    var body = ov.querySelector('.overlay-panel');
    // re-opening the panel you are already in is a redraw, not a navigation:
    // picking a coat must not throw you back to the top of the page
    var keep = openName === name ? body.scrollTop : 0;
    openName = name;
    document.getElementById('panelTitle').innerHTML = p.title || '';
    document.getElementById('panelSub').innerHTML = p.sub || '';
    document.getElementById('panelHead').style.display = p.title ? '' : 'none';
    // most panels are a fixed string; the closet is a function of what you
    // are wearing, so the client hands the state in
    document.getElementById('panelBody').innerHTML =
      typeof p.html === 'function' ? p.html(look) : p.html;
    if (name === 'travel') decorateTravel();
    if (name === 'inbox') { unread = 0; syncPhone(); }
    ov.classList.add('open');
    ov.classList.toggle('scene-panel', !!p.scene);
    body.scrollTop = keep;
  }

  function decorateTravel() {
    document.querySelectorAll('#panelBody [data-when]').forEach(function (el) {
      el.style.display = el.getAttribute('data-when') === home ? '' : 'none';
    });
    document.querySelectorAll('#panelBody [data-move]').forEach(function (el) {
      if (!movedToday) return;
      el.classList.add('spent');
      el.removeAttribute('data-move');
      el.textContent = 'The 6 runs again tomorrow';
    });
  }

  function closePanel() {
    document.getElementById('overlay').classList.remove('open');
    openName = null;
  }

  E.openPanel = openPanel;

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();

})(window.ETM);
