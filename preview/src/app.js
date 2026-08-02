/* Escape the Matrix — client shell.
   One canvas that never unmounts, UI layered over it. Panels dim the world
   rather than replacing it (technical.md §10.1). No game logic lives here:
   the character walks, the sun casts, and the panels are fixed content. */

(function (E) {
  'use strict';

  var ELEV = { s: 6, p: 5, q: 6, g: 7, d: 6, r: 0, k: 0, f: 0, t: 0, w: -5 };
  var WALKABLE = { street: 'srkpgd', apartment: 'f', quayroom: 'f', riverfront: 'q' };

  var cvs, ctx, dpr = 1, W = 0, H = 0, zoom = 1, baseZoom = 1, userZoom = 1;
  var scene, sceneId, actors = [], player, emissive = [];
  var cam = { x: 0, y: 0, set: false, free: false };
  var drag = null, keys = {}, suppressClick = false;
  var home = 'east', movedToday = false;   // you live in exactly one environment
  var t = 0, last = 0, fadeFrom = -1e9, quality = 'high';
  var motes = null, markerEls = [], card, cardZone = null, cardVisible = false;

  /* ------------------------------------------------------------- boot */

  function init() {
    cvs = document.getElementById('world');
    ctx = cvs.getContext('2d');
    card = document.getElementById('worldcard');

    window.addEventListener('resize', resize);
    document.addEventListener('visibilitychange', function () { last = 0; });
    document.getElementById('viewport').addEventListener('pointerdown', onPointerDown);
    window.addEventListener('pointermove', onPointerMove);
    window.addEventListener('pointerup', onPointerUp);
    cvs.addEventListener('wheel', onWheel, { passive: false });

    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { closePanel(); return; }
      if (e.target.closest && e.target.closest('input, textarea')) return;
      var k = panKey(e.key);
      if (k) { keys[k] = true; e.preventDefault(); }
      if (e.key === 'f' || e.key === 'F') recenter();
    });
    document.addEventListener('keyup', function (e) {
      var k = panKey(e.key);
      if (k) keys[k] = false;
    });

    document.body.addEventListener('click', function (e) {
      if (suppressClick) return;
      var go = e.target.closest('[data-go]');
      if (go) { e.preventDefault(); setScene(go.getAttribute('data-go')); closePanel(); return; }
      var p = e.target.closest('[data-panel]');
      if (p) { e.preventDefault(); openPanel(p.getAttribute('data-panel')); return; }
      if (e.target.closest('[data-close]')) { closePanel(); return; }
      var mv = e.target.closest('[data-move]');
      if (mv) { e.preventDefault(); moveHome(mv.getAttribute('data-move')); return; }
      if (e.target.closest('[data-quality]')) { setQuality(quality === 'high' ? 'low' : 'high'); return; }
      if (e.target.closest('[data-follow]')) { recenter(); return; }
      var z = e.target.closest('[data-zoom]');
      if (z) {
        userZoom = E.clamp(userZoom * (z.getAttribute('data-zoom') === 'in' ? 1.25 : 0.8), 0.55, 3.2);
        zoom = baseZoom * userZoom;
        clampCam();
      }
    });

    buildRail();
    setScene('street');
    resize();
    requestAnimationFrame(loop);
  }

  function panKey(key) {
    switch (key) {
      case 'ArrowLeft': case 'a': case 'A': return 'l';
      case 'ArrowRight': case 'd': case 'D': return 'r';
      case 'ArrowUp': case 'w': case 'W': return 'u';
      case 'ArrowDown': case 's': case 'S': return 'd';
    }
    return null;
  }

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
    document.getElementById('qualityLabel').textContent = q === 'high' ? 'High' : 'Low';
    document.body.classList.toggle('quality-low', q === 'low');
  }

  /* ------------------------------------------------------------ scenes */

  function setScene(id) {
    if (!E.SCENES[id] || id === sceneId) return;
    if (E.SCENES[id].env && E.SCENES[id].env !== home) return;
    sceneId = id;
    scene = E.SCENES[id];
    fadeFrom = performance.now();
    cam.set = false;
    cam.free = false;
    drag = null;

    player = clone(scene.player);
    var start = player.start || player.path[0];
    player.x = start[0];
    player.y = start[1];
    player.wp = 1; player.moving = false; player.phase = 0;
    player.route = null; player.ri = 0;

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

  function buildRail() {
    var env = E.ENVIRONMENTS[home];
    var other = E.ENVIRONMENTS[home === 'east' ? 'quay' : 'east'];

    document.getElementById('placeList').innerHTML = env.places.map(function (pl) {
      return '<a data-go="' + pl.id + '" data-scene="' + pl.id + '" href="#">' +
        '<span class="rail-glyph">' + pl.glyph + '</span> ' + pl.label + '</a>';
    }).join('');

    document.getElementById('elsewhereList').innerHTML =
      '<a class="locked" data-panel="travel" href="#">' +
        '<span class="rail-row"><span class="rail-glyph">' + other.places[0].glyph + '</span> ' +
        other.places[0].label + '</span>' +
        '<span class="rail-note">' + (movedToday
          ? 'you have already moved today'
          : 'you live on the ' + (home === 'east' ? 'East Side' : 'Quay')) + '</span>' +
      '</a>';
  }

  // Moving is a relocation, capped at once a day. It is never a nav click.
  function moveHome(target) {
    if (movedToday || target === home) return;
    home = target;
    movedToday = true;
    buildRail();
    setScene(E.ENVIRONMENTS[home].outdoor);
    closePanel();
  }

  function syncChrome() {
    document.getElementById('placeName').textContent = scene.name;
    document.getElementById('placeWhere').textContent = scene.where + ' · ' + scene.clock;
    document.querySelectorAll('[data-scene]').forEach(function (el) {
      el.classList.toggle('active', el.getAttribute('data-scene') === sceneId);
    });
    var env = E.ENVIRONMENTS[scene.env] || E.ENVIRONMENTS[home];
    document.getElementById('envName').textContent = env.name;
    var chip = document.getElementById('envRules');
    chip.textContent = env.rules;
    chip.className = 'tag ' + env.ruleTone;
  }

  /* ----------------------------------------------------------- markers */

  function buildMarkers() {
    var layer = document.getElementById('markers');
    layer.innerHTML = '';
    markerEls = [];
    cardZone = null;
    cardVisible = false;
    card.classList.remove('show');

    (scene.markers || []).forEach(function (m) {
      if (m.kind === 'zone') { cardZone = m; return; }
      var el = document.createElement('div');
      el.className = 'marker ' + m.kind;
      el.innerHTML = '<span class="marker-label">' + m.label + '</span><span class="marker-dot"></span>';
      if (m.go) el.setAttribute('data-go', m.go);
      if (m.panel) el.setAttribute('data-panel', m.panel);
      layer.appendChild(el);
      markerEls.push({ el: el, m: m });
    });

    if (cardZone && scene.zone) {
      var z = scene.zone;
      card.innerHTML =
        '<p class="card-kicker">' + z.title + '</p>' +
        '<p>' + z.body + '</p>' +
        '<p class="quiet">' + z.quote + '</p>' +
        '<div class="btn-row"><span class="btn primary" data-panel="' + z.panel + '">' + z.accept + '</span>' +
        '<span class="btn ghost">' + z.decline + '</span></div>' +
        '<p class="quiet tiny mb0">' + z.foot + '</p>';
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

  function update(dt) {
    stepActor(player, dt);
    for (var i = 0; i < actors.length; i++) stepActor(actors[i], dt);

    var pc = E.isoc(player.x, player.y);
    if (!cam.set) { cam.x = pc.x; cam.y = pc.y - 30; cam.set = true; }

    var pan = 620 * dt / zoom;
    if (keys.l || keys.r || keys.u || keys.d) {
      if (keys.l) cam.x -= pan;
      if (keys.r) cam.x += pan;
      if (keys.u) cam.y -= pan;
      if (keys.d) cam.y += pan;
      if (!cam.free) { cam.free = true; syncFollowChip(); }
    }

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

  function updateMarkers() {
    for (var i = 0; i < markerEls.length; i++) {
      var m = markerEls[i];
      var s = toScreen(m.m.x, m.m.y, 8);
      m.el.style.transform = 'translate(' + s.x.toFixed(1) + 'px,' + s.y.toFixed(1) + 'px) translate(-50%,-100%)';
      m.el.style.opacity = (s.x > -60 && s.x < W + 60 && s.y > -40 && s.y < H + 40) ? 1 : 0;
    }
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

  function applyCam(parallax) {
    var k = parallax === undefined ? 1 : parallax;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.translate(W / 2, H / 2);
    ctx.scale(zoom, zoom);
    ctx.translate(-cam.x * k, -cam.y * k);
  }

  function draw() {
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    emissive.length = 0;

    drawSky();

    applyCam(0.55);
    (scene.boxes || []).forEach(function (b) { if (b.far) drawBuilding(b, true); });

    applyCam(1);
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

  // near buildings, sorted back-to-front for the painter
  function solids() {
    return (scene.boxes || []).filter(function (b) { return !b.far; })
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
        switch (f.kind) {
          case 'tree': blobShadow(f.x, f.y, 78, 30, 15); break;
          case 'lamp': boxShadow(f.x, f.y, 0.16, 0.16, 80); break;
          case 'bench': blobShadow(f.x, f.y, 26, 26, 11); break;
          case 'planter': blobShadow(f.x, f.y, 20, 17, 9); break;
          case 'car': boxShadow(f.x, f.y, 2.4, 1.1, 22); break;
          case 'bus': boxShadow(f.x, f.y, 4.4, 1.5, 46); break;
          case 'busStop': boxShadow(f.x, f.y, 2.6, 1.0, 54); break;
          case 'stall': boxShadow(f.x, f.y, 1.7, 1.5, 52); break;
          case 'fountain': blobShadow(f.x, f.y, 22, 34, 17); break;
          case 'boat': blobShadow(f.x, f.y, 14, 34, 13); break;
          case 'bollard': blobShadow(f.x, f.y, 14, 5, 3); break;
          case 'bags': blobShadow(f.x, f.y, 16, 21, 9); break;
          case 'hydrant': blobShadow(f.x, f.y, 18, 8, 5); break;
          case 'plant': blobShadow(f.x, f.y, 40, 16, 8); break;
        }
      });
      actors.concat([player]).forEach(function (a) {
        blobShadow(a.x, a.y, 58 * (a.scale || 1), 11, 5.5);
      });
    });
  }

  /* --------------------------------------------------------- buildings */

  function facePts(q, u, v, uw, vh) {
    return [E.facePt(q, u, v), E.facePt(q, u + uw, v),
            E.facePt(q, u + uw, v + vh), E.facePt(q, u, v + vh)];
  }

  function drawBuilding(b, far) {
    var base = b.color;
    // the +x wall takes the sun; the street-facing +y wall sits in its own shade
    var lit = E.mix(base, scene.sun.warm, far ? 0.14 : 0.26);
    var shade = E.mix(E.shade(base, 0.7), scene.sun.shade, far ? 0.18 : 0.3);

    var faces = E.box(ctx, {
      x: b.x, y: b.y, w: b.w, d: b.d, h: b.h,
      color: base,
      left: shade,
      right: lit,
      top: E.mix(base, '#ffffff', far ? 0.2 : 0.3)
    });

    if (b.wall) { drawOpenings(b, faces); return; }

    var cols = b.cols || 4, rows = b.rows || 5;
    var topPad = 0.05, bottomLimit = b.store ? 0.74 : 0.94;
    var span = bottomLimit - topPad;
    var cw = 0.9 / cols, ch = span / rows;
    var skyTint = scene.sky[0];

    for (var r = 0; r < rows; r++) {
      for (var c = 0; c < cols; c++) {
        var u = 0.05 + c * cw + cw * 0.18;
        var uw = cw * 0.64;
        var v = topPad + r * ch + ch * 0.2;
        var vh = ch * 0.56;
        var h1 = E.hash(b.seed + c * 3.1, r * 5.7 + b.seed * 0.3);
        var glassy = b.glass || 0.18;

        // shaded wall: glass reads as reflected sky, darker toward the base
        var glass = E.mix(E.mix('#22303f', skyTint, 0.35 + h1 * 0.5), '#ffffff', (1 - r / rows) * 0.18);
        var pane = h1 < glassy ? E.mix(glass, '#ffffff', 0.35) : glass;
        E.poly(ctx, facePts(faces.L, u, v, uw, vh), pane);
        // reveal: a lit sill under each opening sells the wall thickness
        E.poly(ctx, facePts(faces.L, u, v + vh, uw, ch * 0.08), E.mix(shade, '#ffffff', 0.22));

        if (!far && h1 > 0.88) {
          // a few blinds / curtains, warm against all that cool glass
          E.poly(ctx, facePts(faces.L, u, v, uw, vh * 0.55), E.mix('#e8d3ae', scene.sun.warm, 0.4));
        }
      }
      // floor band
      var bandV = topPad + r * ch;
      E.poly(ctx, facePts(faces.L, 0.03, bandV - ch * 0.06, 0.94, ch * 0.05), E.shade(shade, 0.9));

      // the sunlit return wall gets a sparser grid, brighter glass
      for (var k = 0; k < 2; k++) {
        var uu = 0.22 + k * 0.4;
        var h2 = E.hash(b.seed * 1.7 + k, r * 4.4);
        var sunGlass = E.mix(E.mix('#4c6a86', skyTint, 0.5), '#ffffff', 0.28 + h2 * 0.3);
        E.poly(ctx, facePts(faces.R, uu, v, 0.26, vh), sunGlass);
        E.poly(ctx, facePts(faces.R, uu, v + vh, 0.26, ch * 0.07), E.mix(lit, '#ffffff', 0.3));
      }
    }

    // parapet: a lip at the roofline so the block is not a bare prism
    E.poly(ctx, facePts(faces.L, 0, 0, 1, 0.022), E.shade(shade, 0.86));
    E.poly(ctx, facePts(faces.R, 0, 0, 1, 0.022), E.shade(lit, 0.88));

    if (b.store) {
      var s = b.store;
      // shopfront glazing
      var sp = facePts(faces.L, s.u, 0.80, s.w, 0.185);
      E.poly(ctx, sp, E.mix('#2b3742', skyTint, 0.42));
      E.poly(ctx, facePts(faces.L, s.u + 0.01, 0.815, s.w - 0.02, 0.09), E.mix('#39485a', '#ffffff', 0.22));
      // awning, one of the few saturated things in the frame
      var aw = facePts(faces.L, s.u - 0.01, 0.745, s.w + 0.02, 0.055);
      E.poly(ctx, aw, s.awning);
      E.poly(ctx, facePts(faces.L, s.u - 0.01, 0.788, s.w + 0.02, 0.014), E.shade(s.awning, 0.7));
      if (s.sign) {
        E.poly(ctx, facePts(faces.L, s.u + 0.03, 0.70, s.w * 0.5, 0.032), s.sign);
      }
    }

    drawOpenings(b, faces);
  }

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
      if (b.far) return;
      // centre-of-footprint key: for grid-aligned blocks next to streets this
      // orders correctly against people on either side of the wall
      var k = (b.x + b.w / 2) + (b.y + b.d / 2);
      if (b.cols || b.wall) {
        if (offscreen(b)) return;
        list.push({ k: k, f: function () { drawBuilding(b, false); } });
      } else {
        list.push({ k: k, f: function () { drawFurniture(b); } });
      }
    });

    (scene.fixtures || []).forEach(function (f) {
      var k = f.kind === 'sunbeam' ? -9999 : f.kind === 'rug' ? -9000 : f.x + f.y;
      list.push({ k: k, f: function () { drawFixture(f); } });
    });

    actors.forEach(function (a) { list.push({ k: a.x + a.y, f: function () { E.actor(ctx, a, t); } }); });
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

    userZoom = E.clamp(userZoom * (ev.deltaY < 0 ? 1.12 : 1 / 1.12), 0.55, 3.2);
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
    document.getElementById('panelTitle').innerHTML = p.title || '';
    document.getElementById('panelSub').innerHTML = p.sub || '';
    document.getElementById('panelHead').style.display = p.title ? '' : 'none';
    document.getElementById('panelBody').innerHTML = p.html;
    if (name === 'travel') decorateTravel();
    ov.classList.add('open');
    ov.classList.toggle('scene-panel', !!p.scene);
    document.querySelectorAll('.rail a[data-panel]').forEach(function (a) {
      a.classList.toggle('active', a.getAttribute('data-panel') === name);
    });
  }

  function decorateTravel() {
    document.querySelectorAll('#panelBody [data-when]').forEach(function (el) {
      el.style.display = el.getAttribute('data-when') === home ? '' : 'none';
    });
    document.querySelectorAll('#panelBody [data-move]').forEach(function (el) {
      if (!movedToday) return;
      el.classList.add('spent');
      el.removeAttribute('data-move');
      el.textContent = 'The 14 line runs again tomorrow';
    });
  }

  function closePanel() {
    document.getElementById('overlay').classList.remove('open');
    document.querySelectorAll('.rail a[data-panel]').forEach(function (a) {
      a.classList.remove('active');
    });
  }

  E.openPanel = openPanel;

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();

})(window.ETM);
