/* Escape the Matrix — the eye-level view.

   The same plate as render.js, stood inside rather than looked down on. There
   is no second world here: it reads the identical scene data — the ground
   grid, the boxes, the fixtures, the actors — and projects it through a
   perspective camera sitting on the character's shoulders instead of through
   the fixed 3/4 iso transform. That is the whole point. A first-person mode
   built on its own geometry drifts from the isometric one in a week; built on
   the same rows and boxes, a lot added to scenes.js shows up in both views
   without anyone touching this file.

   Two constants carry the translation, and they are judgement calls rather
   than measurements:

     U     how many pixels of iso height make one grid cell. The plate is not
           metric — it was drawn to read at 3/4, so a figure is 64px tall and
           a cell is a comfortable stride. At U = 34 a cell lands near 0.9m,
           which puts pavements at 1.8m and the character at 1.7m.

     LIFT  the iso plate compresses storeys: a "tower" is 214px, barely three
           times a person. That is correct for a painted 3/4 plate and wrong
           the moment you stand under it, so building heights — and only
           building heights — are exaggerated on the way in. Fixtures and
           people are drawn at true scale.

   Everything else is ordinary: near-plane clipping, painter's order by
   distance, and a fog that reaches full haze before the drawn plate runs out
   so the edge of the world is weather rather than a cliff. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  var U = 34;            // iso pixels of height to one grid cell
  var LIFT = 2.2;        // storey exaggeration, buildings only
  var EYE = 1.62;        // camera height in cells — 1.46m on a 1.7m figure
  var CEIL = 2.9;        // interior ceiling, in cells
  var FOVY = 0.95;       // vertical field of view, radians
  var NEAR = 0.07;
  var GROUND_R = 24;     // cells of ground painted around the camera
  var FAR = 44;          // cells beyond which nothing is drawn at all
  var FOG0 = 6, FOG1 = 30, FOG_MAX = 0.94;

  /* The sun, as a direction on the grid rather than as the screen-space
     offset the iso shadows use. East and north, which is the reading that
     makes the +x wall the lit one and drops every shadow towards +y — the
     same two facts render.js paints by hand. */
  var SUNX = 0.55, SUNY = -0.83;

  var cam = {
    x: 0, y: 0, z: EYE, yaw: 0, pitch: 0,
    cy: 1, sy: 0, cp: 1, sp: 0, fl: 600, W: 0, H: 0, hx: 1
  };

  var FPV = E.FPV = {};
  FPV.U = U;
  FPV.EYE = EYE;

  /* ------------------------------------------------------------- camera */

  function setCam(W, H, x, y, yaw, pitch, z) {
    cam.W = W; cam.H = H;
    cam.x = x; cam.y = y; cam.z = z;
    cam.yaw = yaw; cam.pitch = pitch;
    cam.cy = Math.cos(yaw); cam.sy = Math.sin(yaw);
    cam.cp = Math.cos(pitch); cam.sp = Math.sin(pitch);
    cam.fl = (H / 2) / Math.tan(FOVY / 2);
    cam.hx = (W / 2) / cam.fl;      // tan of the half horizontal field
  }

  /* Grid position plus height in cells, into camera space. `f` is depth down
     the view axis, `r` is lateral, `u` is up; `d` is the flat ground distance,
     which is what fog and level-of-detail are keyed on rather than `f` (a
     thing beside you is near even though it is barely in front of you). */
  function toCam(gx, gy, z) {
    var dx = gx - cam.x, dy = gy - cam.y, dz = z - cam.z;
    var f = dx * cam.cy + dy * cam.sy;
    return {
      r: -dx * cam.sy + dy * cam.cy,
      f: f * cam.cp + dz * cam.sp,
      u: -f * cam.sp + dz * cam.cp,
      d: Math.sqrt(dx * dx + dy * dy)
    };
  }
  FPV.toCam = toCam;

  function projX(c) { return cam.W / 2 + cam.fl * c.r / c.f; }
  function projY(c) { return cam.H / 2 - cam.fl * c.u / c.f; }

  // where the horizon sits, which is also where the sky stops
  function horizon() { return cam.H / 2 + cam.fl * cam.sp / cam.cp; }

  /* Nothing behind the eye may be projected — it comes back mirrored and
     smeared across the screen. Sutherland-Hodgman against the near plane,
     which is the one clip a software painter cannot skip. */
  function clipNear(poly) {
    var out = [], n = poly.length;
    for (var i = 0; i < n; i++) {
      var a = poly[i], b = poly[(i + 1) % n];
      var ain = a.f >= NEAR, bin = b.f >= NEAR;
      if (ain) out.push(a);
      if (ain !== bin) {
        var t = (NEAR - a.f) / (b.f - a.f);
        out.push({ r: a.r + (b.r - a.r) * t, f: NEAR, u: a.u + (b.u - a.u) * t });
      }
    }
    return out;
  }

  // cheap frustum reject before the expensive part
  function outside(poly) {
    var i, allBehind = true, allL = true, allR = true, allUp = true, allDn = true;
    for (i = 0; i < poly.length; i++) {
      var p = poly[i];
      if (p.f >= NEAR) allBehind = false;
      var lim = Math.max(p.f, NEAR) * cam.hx;
      if (p.r > -lim) allL = false;
      if (p.r < lim) allR = false;
      var vlim = Math.max(p.f, NEAR) * (cam.H / 2) / cam.fl;
      if (p.u < vlim) allUp = false;
      if (p.u > -vlim) allDn = false;
    }
    return allBehind || allL || allR || allUp || allDn;
  }

  function fillCam(ctx, poly, fill) {
    if (outside(poly)) return;
    var p = clipNear(poly);
    if (p.length < 3) return;
    ctx.beginPath();
    ctx.moveTo(projX(p[0]), projY(p[0]));
    for (var i = 1; i < p.length; i++) ctx.lineTo(projX(p[i]), projY(p[i]));
    ctx.closePath();
    ctx.fillStyle = fill;
    ctx.fill();
  }

  // world-space quad, given as four [gx, gy, z] triples
  function quad(ctx, a, b, c, d, fill) {
    fillCam(ctx, [
      toCam(a[0], a[1], a[2]), toCam(b[0], b[1], b[2]),
      toCam(c[0], c[1], c[2]), toCam(d[0], d[1], d[2])
    ], fill);
  }

  /* Screen point back onto the ground plane, for click-to-walk. Only defined
     when the ray actually points below the horizon. */
  FPV.ground = function (sx, sy) {
    var rr = (sx - cam.W / 2) / cam.fl, uu = -(sy - cam.H / 2) / cam.fl;
    var den = cam.sp + uu * cam.cp;
    if (den >= -1e-4) return null;                 // at or above the horizon
    var f = -cam.z / den;
    if (f < 0 || f > FAR) return null;
    var pre = f * (cam.cp - uu * cam.sp), lat = rr * f;
    return {
      x: cam.x + pre * cam.cy - lat * cam.sy,
      y: cam.y + pre * cam.sy + lat * cam.cy
    };
  };

  /* What app.js's toScreen needs: a grid point and an iso-pixel height, in
     screen coordinates, with a flag for "this is behind you" so the HTML
     markers can hide rather than appear mirrored on the wrong side. */
  FPV.screen = function (gx, gy, zpx) {
    var c = toCam(gx, gy, (zpx || 0) / U);
    var miss = { x: -9999, y: -9999, ok: false, d: c.d };
    if (c.f < 0.2 || c.d > FAR) return miss;
    var x = projX(c), y = projY(c);
    // Just in front of the eye and hard to one side, the projection is
    // technically valid and thousands of pixels off the glass. Call it a miss
    // rather than hand a caller a number it will try to clamp into frame.
    if (x < -cam.W || x > cam.W * 2 || y < -cam.H || y > cam.H * 2) return miss;
    return { x: x, y: y, ok: true, d: c.d };
  };

  /* ---------------------------------------------------------------- fog */

  var haze = '#cfd8db';

  function fogT(d) {
    return E.clamp((d - FOG0) / (FOG1 - FOG0), 0, 1) * FOG_MAX;
  }

  function fogged(color, d) {
    var t = fogT(d);
    return t <= 0 ? color : E.mix(color, haze, t);
  }

  /* ---------------------------------------------------------------- sky */

  function drawSky(ctx, scene, t) {
    var W = cam.W, H = cam.H;
    var hz = horizon();

    // The gradient is hung off the horizon rather than off the window, so
    // looking up does not slide the whole sky with the camera.
    var top = hz - H * 1.35;
    var g = ctx.createLinearGradient(0, top, 0, hz);
    g.addColorStop(0, scene.sky[0]);
    g.addColorStop(0.62, scene.sky[1]);
    g.addColorStop(1, scene.sky[2]);
    ctx.fillStyle = g;
    ctx.fillRect(0, Math.min(0, hz), W, Math.max(0, hz) + 1);

    // below the horizon: the haze the ground fades into, so the drawn plate
    // ending is weather rather than a hole
    if (hz < H) {
      var gg = ctx.createLinearGradient(0, hz, 0, hz + H * 0.5);
      gg.addColorStop(0, haze);
      gg.addColorStop(1, E.mix(haze, scene.sun.shade, 0.24));
      ctx.fillStyle = gg;
      ctx.fillRect(0, hz, W, H - hz + 1);
    }

    if (scene.interior) return;

    // the sun itself, wherever the camera happens to have left it
    var sf = SUNX * cam.cy + SUNY * cam.sy;
    var sr = -SUNX * cam.sy + SUNY * cam.cy;
    if (sf > 0.05) {
      var sx = W / 2 + cam.fl * sr / sf;
      var sy = hz - cam.fl * 0.72;                  // ~36° above the horizon
      var sg = ctx.createRadialGradient(sx, sy, 0, sx, sy, W * 0.6);
      sg.addColorStop(0, 'rgba(255,246,214,0.62)');
      sg.addColorStop(0.3, 'rgba(255,232,180,0.16)');
      sg.addColorStop(1, 'rgba(255,232,180,0)');
      ctx.fillStyle = sg;
      ctx.fillRect(0, 0, W, H);
    }

    drawClouds(ctx, t, hz);
  }

  /* Clouds live at fixed compass bearings and drift, so turning on the spot
     moves past them instead of dragging them along. */
  var CLOUDS = [
    [0.4, 0.62, 150, 0.5], [1.7, 0.50, 120, 0.38], [3.0, 0.72, 190, 0.46],
    [4.1, 0.55, 130, 0.34], [5.4, 0.80, 160, 0.4], [2.4, 0.9, 140, 0.3]
  ];

  function drawClouds(ctx, t, hz) {
    for (var i = 0; i < CLOUDS.length; i++) {
      var c = CLOUDS[i];
      var az = c[0] + t * 0.006 * (1 + i * 0.2) - cam.yaw;
      az = Math.atan2(Math.sin(az), Math.cos(az));
      if (Math.abs(az) > 1.15) continue;
      var x = cam.W / 2 + cam.fl * Math.tan(az);
      var y = hz - cam.fl * c[1];
      var r = c[2];
      ctx.fillStyle = 'rgba(255,255,255,' + c[3] + ')';
      for (var j = 0; j < 5; j++) {
        var o = E.hash(i * 7 + j, j * 3.3);
        ctx.beginPath();
        ctx.ellipse(x + (j - 2) * r * 0.34, y + (o - 0.5) * r * 0.18,
                    r * (0.32 + o * 0.2), r * (0.14 + o * 0.08), 0, 0, Math.PI * 2);
        ctx.fill();
      }
    }
  }

  /* ------------------------------------------------------------- ground */

  /* Flat coplanar tiles cannot overlap on screen, so the ground needs no
     sort at all — only the skirts between two elevations do, and those go in
     a second pass after every tile is down. */
  function drawGround(ctx, scene, elev, t) {
    var rows = scene.rows;
    var cx = Math.floor(cam.x), cy = Math.floor(cam.y);
    var x0 = Math.max(0, cx - GROUND_R), x1 = Math.min(rows[0].length - 1, cx + GROUND_R);
    var y0 = Math.max(0, cy - GROUND_R), y1 = Math.min(rows.length - 1, cy + GROUND_R);
    var skirts = [];

    for (var y = y0; y <= y1; y++) {
      var row = rows[y];
      for (var x = x0; x <= x1; x++) {
        var code = row[x];
        var g = E.GROUND[code];
        if (!g) continue;

        var dx = x + 0.5 - cam.x, dy = y + 0.5 - cam.y;
        var d = Math.sqrt(dx * dx + dy * dy);
        if (d > GROUND_R) continue;
        // behind the eye, with a cell of slack so the tile underfoot survives
        if (dx * cam.cy + dy * cam.sy < -1.5) continue;

        var e = (elev[code] || 0) / U;
        var n = E.hash(x * 1.7 + 0.3, y * 2.3 + 0.7);
        var col = E.mix(g.a, g.b, n);

        if (g.water) {
          /* On the plate every tile picks its own value off the hash, which
             reads as broken light. Level with the water it reads as a
             chequerboard, so the river is a smooth field instead — adjacent
             tiles land next to each other rather than anywhere. */
          var sw = 0.5 + 0.5 * Math.sin(x * 0.42 + y * 0.66 + t * 0.5);
          col = E.mix(E.mix(g.a, g.b, sw), '#8ec4de',
                      0.05 + (0.5 + 0.5 * Math.sin(t * 0.9 + x * 0.3 + y * 0.5)) * 0.12);
        } else if (code === 'k' && x % 2 === 0) {
          col = E.mix(col, '#ffffff', 0.30);
        } else if (code === 'f' && (x + y) % 2 === 0) {
          col = E.mix(col, '#5a3a1d', 0.10);
        }

        // overlapped by a hair: abutting antialiased edges leave a lighter
        // seam, and a whole plate of them reads as a grid drawn on the ground
        var o = 0.006;
        quad(ctx, [x - o, y - o, e], [x + 1 + o, y - o, e],
                  [x + 1 + o, y + 1 + o, e], [x - o, y + 1 + o, e], fogged(col, d));

        // a step down to a neighbour is a visible face at ground level
        var below = rows[y + 1] ? rows[y + 1][x] : '.';
        var right = row[x + 1] || '.';
        var eb = (elev[below] || 0) / U, er = (elev[right] || 0) / U;
        if (e - eb > 0.08) skirts.push([x, y + 1, x + 1, y + 1, e, eb, col, d]);
        if (e - er > 0.08) skirts.push([x + 1, y, x + 1, y + 1, e, er, col, d]);
      }
    }

    for (var i = 0; i < skirts.length; i++) {
      var s = skirts[i];
      quad(ctx, [s[0], s[1], s[4]], [s[2], s[3], s[4]], [s[2], s[3], s[5]], [s[0], s[1], s[5]],
           fogged(E.shade(s[6], 0.74), s[7]));
    }
  }

  /* -------------------------------------------------------------- walls */

  // bilinear point inside a world-space face given as [TL, TR, BR, BL]
  function fp(q, u, v) {
    var i, o = [0, 0, 0];
    for (i = 0; i < 3; i++) {
      var top = q[0][i] + (q[1][i] - q[0][i]) * u;
      var bot = q[3][i] + (q[2][i] - q[3][i]) * u;
      o[i] = top + (bot - top) * v;
    }
    return o;
  }

  function faceRect(ctx, q, u, v, uw, vh, fill) {
    quad(ctx, fp(q, u, v), fp(q, u + uw, v), fp(q, u + uw, v + vh), fp(q, u, v + vh), fill);
  }

  function wallTone(scene, base, nx, ny) {
    var l = (nx * SUNX + ny * SUNY) * 0.5 + 0.5;
    return E.mix(E.mix(base, scene.sun.shade, 0.36 * (1 - l)), scene.sun.warm, 0.26 * l);
  }

  // glass reads as reflected sky, cooler and darker further down the wall
  function pane(scene, v, bright) {
    return E.mix(E.mix('#22303f', scene.sky[0], 0.34 + bright * 0.5), '#ffffff', (1 - v) * 0.2);
  }

  /* The four walls of a box, outward normal first. `u` runs left to right as
     seen from outside the wall, `v` runs top to bottom — the same convention
     E.faceRect uses on the iso plate, so the facade code below reads the same
     way render.js's does. */
  function walls(b, top) {
    var x0 = b.x, x1 = b.x + b.w, y0 = b.y, y1 = b.y + b.d;
    return [
      { nx: 0, ny: 1, span: b.w, q: [[x0, y1, top], [x1, y1, top], [x1, y1, 0], [x0, y1, 0]] },
      { nx: 0, ny: -1, span: b.w, q: [[x1, y0, top], [x0, y0, top], [x0, y0, 0], [x1, y0, 0]] },
      { nx: 1, ny: 0, span: b.d, q: [[x1, y1, top], [x1, y0, top], [x1, y0, 0], [x1, y1, 0]] },
      { nx: -1, ny: 0, span: b.d, q: [[x0, y0, top], [x0, y1, top], [x0, y1, 0], [x0, y0, 0]] }
    ];
  }

  function facing(b, w) {
    if (w.nx > 0) return cam.x > b.x + b.w;
    if (w.nx < 0) return cam.x < b.x;
    if (w.ny > 0) return cam.y > b.y + b.d;
    return cam.y < b.y;
  }

  function boxTop(b, scene) {
    var h = b.h / U;
    if (b.wall || b.cols) {
      // a building. Interior walls are a room, not a storey: the plate gives
      // them the same 150px it gives a block of flats.
      return scene.interior ? Math.min(h, CEIL) : h * LIFT;
    }
    return h;                                   // furniture is already true size
  }

  function drawBox(ctx, scene, b, d) {
    var top = boxTop(b, scene);
    var base = b.color || '#c9b291';
    var ws = walls(b, top);
    var isBuilding = !!(b.cols || b.wall);

    for (var i = 0; i < ws.length; i++) {
      var w = ws[i];
      if (!facing(b, w)) continue;
      var tone = fogged(wallTone(scene, base, w.nx, w.ny), d);
      quad(ctx, w.q[0], w.q[1], w.q[2], w.q[3], tone);
      if (isBuilding) facade(ctx, scene, b, w, tone, d, top);
    }

    // The roof only matters when you can see onto it, but it costs one quad
    // and a missing one reads as a hole in the sky from a balcony.
    var t = top;
    quad(ctx, [b.x, b.y, t], [b.x + b.w, b.y, t], [b.x + b.w, b.y + b.d, t], [b.x, b.y + b.d, t],
         fogged(E.mix(base, '#ffffff', 0.3), d));
  }

  /* A facade at eye level is windows, a shopfront and a lid. It is not the
     ten hand-painted types render.js has — those are 3/4 paintings and half
     of what they draw is roofline silhouette that a street-level camera never
     sees. What survives the trip is the massing and the rhythm: a nine-row
     tower stays a nine-row tower, a shop keeps its awning, and the colour is
     the same colour. */
  function facade(ctx, scene, b, w, tone, d, top) {
    if (d > FOG1) return;                       // already all but haze

    /* A `wall` box is a partition rather than a block: plaster, and whatever
       openings the scene put in it. Never the generated grid — the same rule
       render.js applies before it reaches its own facade painters. */
    if (b.wall) {
      if (b.openings) drawOpenings(ctx, b, w, b.openings);
      return;
    }

    var cols = Math.max(2, Math.round((b.cols || 4) * (w.span / Math.max(b.w, b.d))));
    var rws = b.rows || 3;
    var seed = b.seed || 3;
    var store = b.store && w.ny > 0;            // shopfronts face the street

    // the lid: a parapet band, and a shadow line under it
    faceRect(ctx, w.q, 0, 0, 1, 0.03, E.mix(tone, '#ffffff', 0.22));
    faceRect(ctx, w.q, 0, 0.03, 1, 0.012, E.shade(tone, 0.8));

    var v0 = 0.07, v1 = store ? 0.66 : 0.92;
    var vs = (v1 - v0) / rws;
    var ww = 0.62 / cols, wh = vs * 0.56;
    var detail = d < 15;

    for (var j = 0; j < rws; j++) {
      var v = v0 + j * vs + (vs - wh) / 2;
      for (var i = 0; i < cols; i++) {
        var u = (i + 0.5) / cols - ww / 2;
        var n = E.hash(seed + i * 3.1, j * 2.7 + seed * 0.3);
        var lit = n > 0.86;
        var glass = lit ? E.mix('#ffe6b4', scene.sun.warm, 0.3) : pane(scene, v, 0.3 + n * 0.4);
        if (detail) {
          faceRect(ctx, w.q, u - ww * 0.1, v - wh * 0.09, ww * 1.2, wh * 1.18,
                   fogged(E.mix(tone, '#ffffff', 0.26), d));
        }
        faceRect(ctx, w.q, u, v, ww, wh, fogged(glass, d));
      }
    }

    if (b.balcony && w.ny > 0) {
      for (var k = 1; k < rws; k++) {
        faceRect(ctx, w.q, 0.12, v0 + k * vs - 0.012, 0.76, 0.014,
                 fogged(E.shade(tone, 0.66), d));
      }
    }

    if (!store) return;

    var s = b.store;
    // the glazed ground floor, in panes — one sheet of blue two storeys wide
    // is a swimming pool rather than a shop
    var panes = 4, pw = 0.88 / panes;
    faceRect(ctx, w.q, 0.06, 0.7, 0.88, 0.24, fogged(E.mix(tone, '#ffffff', 0.2), d));
    for (var g = 0; g < panes; g++) {
      faceRect(ctx, w.q, 0.075 + g * pw, 0.715, pw - 0.03, 0.21,
               fogged(pane(scene, 0.9, 0.14 + g * 0.03), d));
    }
    faceRect(ctx, w.q, 0.42, 0.76, 0.13, 0.24, fogged(E.shade(base(b), 0.5), d));
    if (s.sign) faceRect(ctx, w.q, 0.1, 0.62, 0.5, 0.05, fogged(s.sign, d));
    awning(ctx, b, top, s, d);
  }

  /* An awning painted on the wall is a stripe. From the pavement it has to
     come out over your head or the shop reads as a mural, so it is the one
     piece of facade that is geometry rather than paint. */
  function awning(ctx, b, top, s, d) {
    var col = s.awning;
    if (!col) return;
    var y1 = b.y + b.d, out = 0.62;
    var x0 = b.x + s.u * b.w, x1 = x0 + s.w * b.w;
    var zt = top * 0.33, zf = zt - 0.26;
    quad(ctx, [x0, y1, zt], [x1, y1, zt], [x1, y1 + out, zf], [x0, y1 + out, zf], fogged(col, d));
    quad(ctx, [x0, y1 + out, zf], [x1, y1 + out, zf],
         [x1, y1 + out, zf - 0.3], [x0, y1 + out, zf - 0.3], fogged(E.shade(col, 0.76), d));
  }

  function base(b) { return b.color || '#c9b291'; }

  // interior walls carry named openings rather than a generated grid
  function drawOpenings(ctx, b, w, openings) {
    for (var i = 0; i < openings.length; i++) {
      var o = openings[i];
      if (o.kind === 'window') {
        faceRect(ctx, w.q, o.u, 0.1, o.w, 0.42, E.mix('#9dc4e0', '#ffffff', 0.5));
        faceRect(ctx, w.q, o.u + o.w / 2 - 0.006, 0.1, 0.012, 0.42, '#e6dcc9');
      } else {
        faceRect(ctx, w.q, o.u, 0.32, o.w, 0.68, E.shade(base(b), 0.5));
      }
    }
  }

  /* ---------------------------------------------------- interior shell */

  /* An interior plate is a floor with two painted walls: the other two sides
     are the corner the 3/4 camera looks in through, and they were never built
     because nothing could ever see them. At eye level you turn round, so the
     room is closed off here — around the floor's own extent, and only on the
     sides where the scene has not already put a wall. */
  function shell(scene) {
    if (scene._fpvShell) return scene._fpvShell;
    var rows = scene.rows, minX = 1e9, maxX = -1e9, minY = 1e9, maxY = -1e9;
    for (var y = 0; y < rows.length; y++) {
      for (var x = 0; x < rows[y].length; x++) {
        var g = E.GROUND[rows[y][x]];
        if (!g || g.water) continue;
        if (x < minX) minX = x;
        if (x + 1 > maxX) maxX = x + 1;
        if (y < minY) minY = y;
        if (y + 1 > maxY) maxY = y + 1;
      }
    }
    var out = [];
    if (minX > maxX) { scene._fpvShell = out; return out; }

    var boxes = scene.boxes || [];
    function covered(nx, ny, edge) {
      for (var i = 0; i < boxes.length; i++) {
        var b = boxes[i];
        if (!b.wall) continue;
        var mid = ny ? b.y + b.d / 2 : b.x + b.w / 2;
        if (Math.abs(mid - edge) < 1.8) return true;
      }
      return false;
    }

    var col = '#cdbca1';
    if (!covered(0, 1, minY)) out.push({ x: minX, y: minY - 0.4, w: maxX - minX, d: 0.4, h: CEIL * U, color: col, wall: true });
    if (!covered(0, 1, maxY)) out.push({ x: minX, y: maxY, w: maxX - minX, d: 0.4, h: CEIL * U, color: col, wall: true });
    if (!covered(1, 0, minX)) out.push({ x: minX - 0.4, y: minY - 0.4, w: 0.4, d: maxY - minY + 0.8, h: CEIL * U, color: col, wall: true });
    if (!covered(1, 0, maxX)) out.push({ x: maxX, y: minY - 0.4, w: 0.4, d: maxY - minY + 0.8, h: CEIL * U, color: col, wall: true });
    out.ceiling = [minX - 0.4, minY - 0.4, maxX + 0.4, maxY + 0.4];
    scene._fpvShell = out;
    return out;
  }

  /* ----------------------------------------------------------- fixtures */

  /* Most street furniture is already a sprite drawn upright around its own
     grid point — a bench, a tree, a hydrant. Those are billboarded: the iso
     drawing is run through a scale that puts its base on the projected ground
     and its height in perspective. It is a flat cutout, and at the distance
     you see a hydrant from that is exactly what a hydrant looks like.

     The number is a size correction. The plate draws scenery short for the
     same reason it draws buildings short, and a 2m tree is a shrub.

     Indoors the correction runs the other way. The plate lifts a pinboard and
     a desk lamp well above where they really sit so they clear the furniture
     they stand behind at 3/4 — from a chair in the same room that reads as a
     board mounted near the ceiling, so those come back down. */
  var SPRITE = {
    tree: 2.1, plant: 1.15, bench: 1.15, planter: 1.2, hydrant: 1.2, bags: 1.2,
    fountain: 1.5, boat: 1.6, bollard: 1.1,
    board: 0.62, screen: 0.8, deskLamp: 0.8, floorLamp: 0.85, mug: 0.8
  };

  /* The rest are volumes rather than cutouts — you walk around a car and
     under a canopy, and a flat one gives the game away immediately. */
  var SOLID = {
    car:      { w: 2.4, d: 1.1, h: 1.35, top: 0.72, color: null },
    bus:      { w: 4.4, d: 1.5, h: 2.7, color: '#c98a3a' },
    busStop:  { w: 2.6, d: 1.0, h: 2.5, color: '#59636b', roof: true },
    stall:    { w: 1.7, d: 1.5, h: 2.3, color: '#8a8172', roof: true },
    newsstand: { w: 1.5, d: 1.0, h: 2.4, color: '#7d7566', roof: true },
    plane:    { w: 7.2, d: 1.5, h: 1.5, color: '#e9edf0' }
  };

  function drawSolid(ctx, scene, f, d) {
    var s = SOLID[f.kind];
    var w = s.w * (f.small ? 0.62 : 1), dp = s.d * (f.small ? 0.62 : 1);
    var col = f.color || s.color || '#8a8172';
    var b = { x: f.x, y: f.y, w: w, d: dp, h: 0, color: col };
    var ws = walls(b, s.h);
    var i;

    if (s.roof) {
      // an open shelter: four posts and a lid rather than a solid block
      var posts = [[f.x, f.y], [f.x + w - 0.12, f.y], [f.x, f.y + dp - 0.12], [f.x + w - 0.12, f.y + dp - 0.12]];
      for (i = 0; i < posts.length; i++) {
        var pb = { x: posts[i][0], y: posts[i][1], w: 0.12, d: 0.12 };
        var pw = walls(pb, s.h);
        for (var j = 0; j < pw.length; j++) {
          if (facing(pb, pw[j])) {
            quad(ctx, pw[j].q[0], pw[j].q[1], pw[j].q[2], pw[j].q[3],
                 fogged(wallTone(scene, col, pw[j].nx, pw[j].ny), d));
          }
        }
      }
      // the back panel, and whatever is on the counter
      var back = { x: f.x, y: f.y, w: w, d: 0.14 };
      var bw = walls(back, s.h * 0.86);
      for (i = 0; i < bw.length; i++) {
        if (facing(back, bw[i])) {
          quad(ctx, bw[i].q[0], bw[i].q[1], bw[i].q[2], bw[i].q[3],
               fogged(wallTone(scene, E.shade(col, 0.9), bw[i].nx, bw[i].ny), d));
        }
      }
      var awn = f.awning || '#a8503f';
      quad(ctx, [f.x - 0.2, f.y - 0.2, s.h], [f.x + w + 0.2, f.y - 0.2, s.h],
           [f.x + w + 0.2, f.y + dp + 0.2, s.h], [f.x - 0.2, f.y + dp + 0.2, s.h], fogged(awn, d));
      quad(ctx, [f.x - 0.2, f.y + dp + 0.2, s.h], [f.x + w + 0.2, f.y + dp + 0.2, s.h],
           [f.x + w + 0.2, f.y + dp + 0.2, s.h - 0.12], [f.x - 0.2, f.y + dp + 0.2, s.h - 0.12],
           fogged(E.shade(awn, 0.78), d));
      return;
    }

    for (i = 0; i < ws.length; i++) {
      if (!facing(b, ws[i])) continue;
      var tone = fogged(wallTone(scene, col, ws[i].nx, ws[i].ny), d);
      quad(ctx, ws[i].q[0], ws[i].q[1], ws[i].q[2], ws[i].q[3], tone);
      // glazing, which is what makes a box read as a vehicle
      if (s.top) {
        faceRect(ctx, ws[i].q, 0.16, 0.06, 0.68, 0.34, fogged(pane(scene, 0.3, 0.5), d));
      }
    }
    quad(ctx, [f.x, f.y, s.h], [f.x + w, f.y, s.h], [f.x + w, f.y + dp, s.h], [f.x, f.y + dp, s.h],
         fogged(E.mix(col, '#ffffff', 0.3), d));

    if (f.kind === 'plane') {
      // one wing, flat, so the apron reads as an airfield from the ground
      var wy = f.y - 2.6 * (f.small ? 0.62 : 1), wd = 6.4 * (f.small ? 0.62 : 1);
      quad(ctx, [f.x + 1.9, wy, s.h * 0.72], [f.x + 3.6, wy, s.h * 0.72],
           [f.x + 3.6, wy + wd, s.h * 0.72], [f.x + 1.9, wy + wd, s.h * 0.72],
           fogged(E.mix(col, scene.sun.shade, 0.16), d));
    }
  }

  // lamps and railings are thin verticals, cheapest to state directly
  function drawLamp(ctx, scene, f, d) {
    var pb = { x: f.x - 0.06, y: f.y - 0.06, w: 0.12, d: 0.12 };
    var h = 2.9;
    var pw = walls(pb, h);
    for (var i = 0; i < pw.length; i++) {
      if (!facing(pb, pw[i])) continue;
      quad(ctx, pw[i].q[0], pw[i].q[1], pw[i].q[2], pw[i].q[3],
           fogged(wallTone(scene, '#4c5158', pw[i].nx, pw[i].ny), d));
    }
    quad(ctx, [f.x - 0.34, f.y - 0.12, h], [f.x + 0.34, f.y - 0.12, h],
         [f.x + 0.34, f.y + 0.12, h], [f.x - 0.34, f.y + 0.12, h], fogged('#e8dcc0', d));
  }

  /* One bay of railing: a post and the two rails leaving it. A railing runs
     the length of the map, so it cannot be one object in the painter's list —
     the far end of it would sort with the near end and lay itself over
     everything between. Each bay sorts on its own distance. */
  function drawRailBay(ctx, scene, f, gx, gy, d) {
    var horiz = f.axis === 'x';
    var col = f.color || '#7c7466';
    var pb = { x: gx - 0.05, y: gy - 0.05, w: 0.1, d: 0.1 };
    var pw = walls(pb, 0.8);
    for (var j = 0; j < pw.length; j++) {
      if (facing(pb, pw[j])) {
        quad(ctx, pw[j].q[0], pw[j].q[1], pw[j].q[2], pw[j].q[3],
             fogged(wallTone(scene, col, pw[j].nx, pw[j].ny), d));
      }
    }
    var bx = horiz ? gx + 1 : gx, by = horiz ? gy : gy + 1;
    [0.78, 0.44].forEach(function (z) {
      quad(ctx, [gx, gy, z], [bx, by, z], [bx, by, z - 0.06], [gx, gy, z - 0.06], fogged(col, d));
    });
  }

  /* -------------------------------------------------------- billboards */

  /* The scale that turns one iso pixel into one screen pixel at this
     distance. Measured from two projected points rather than derived, so it
     stays honest when the camera is pitched. */
  function billScale(gx, gy) {
    var g = toCam(gx, gy, 0), h = toCam(gx, gy, 1);
    if (g.f < NEAR || h.f < NEAR) return null;
    var gy2 = projY(g), hy2 = projY(h);
    var k = (gy2 - hy2) / U;
    if (!(k > 0)) return null;
    return { k: k, x: projX(g), y: gy2 };
  }

  function billboard(ctx, gx, gy, lift, d, fn) {
    var s = billScale(gx, gy);
    if (!s) return;
    var k = s.k * (lift || 1);
    var c = E.isoc(gx, gy);
    ctx.save();
    /* A quad takes its fog in the fill colour; a cutout cannot, because the
       art picks its own colours as it draws. Distance is spent on opacity
       instead, and deliberately only half of it: `ctx.filter` would give a
       proper haze tint and costs a compositing layer per sprite per frame,
       which is not affordable at forty of them. Half the fade reads as air
       without turning a tree into something you can see the windows through. */
    ctx.globalAlpha = 1 - fogT(d) * 0.5;
    ctx.translate(s.x, s.y);
    ctx.scale(k, k);
    ctx.translate(-c.x, -c.y);
    fn();
    ctx.restore();
  }

  /* A pool of shade under anything standing up. Drawn as a soft screen-space
     ellipse rather than a projected ring: a ground circle a metre across is
     only a few pixels tall in this camera, and a polygon that small reads as
     the octagon it is. Squashed by how far away it is, which is the same
     thing as how shallow your angle onto the ground is. */
  function blob(ctx, gx, gy, r, scene, d) {
    var s = billScale(gx, gy);
    if (!s) return;
    var rx = r * s.k * U;
    if (rx < 0.6) return;
    var ry = rx * E.clamp(EYE / Math.max(d, 0.4), 0.06, 0.85);
    var g = ctx.createRadialGradient(s.x, s.y, 0, s.x, s.y, rx);
    g.addColorStop(0, E.rgba(scene.sun.shade, 0.34 * (1 - fogT(d))));
    g.addColorStop(0.65, E.rgba(scene.sun.shade, 0.2 * (1 - fogT(d))));
    g.addColorStop(1, E.rgba(scene.sun.shade, 0));
    ctx.save();
    ctx.translate(s.x, s.y);
    ctx.scale(1, ry / rx);
    ctx.translate(-s.x, -s.y);
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(s.x, s.y, rx, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();
  }

  /* ------------------------------------------------------------- frame */

  /* One pass, painted back to front. Ground first because it is flat and
     cannot occlude anything; then everything that stands up, sorted by how
     far away it is. Buildings key on their centre and people on themselves,
     which puts a pedestrian in front of the block they are walking past. */
  FPV.frame = function (ctx, o) {
    var scene = o.scene, W = o.W, H = o.H;
    setCam(W, H, o.x, o.y, o.yaw, o.pitch, EYE + (o.zoff || 0));

    ctx.setTransform(o.dpr, 0, 0, o.dpr, 0, 0);
    haze = scene.interior
      ? E.mix(scene.sky[2], scene.sun.shade, 0.5)
      : E.mix(scene.sky[2], scene.sun.warm, 0.22);

    drawSky(ctx, scene, o.t);

    /* Interiors are painted a floor short: the plate's walls stand half a
       cell outside the first floorboard because at 3/4 you never see the
       join. From a chair you do, and it is a slot of nothing at the skirting.
       One slab of boards under the whole room, and the tiles go over it. */
    if (scene.interior) {
      var sh = shell(scene), room = sh.ceiling;
      if (room) {
        var g = E.GROUND[scene.rows[Math.floor(cam.y)] ? scene.rows[Math.floor(cam.y)][Math.floor(cam.x)] : 'f'];
        quad(ctx, [room[0], room[1], 0], [room[2], room[1], 0], [room[2], room[3], 0], [room[0], room[3], 0],
             E.shade(g ? E.mix(g.a, g.b, 0.5) : '#b98d5c', 0.9));
      }
    }

    drawGround(ctx, scene, o.elev, o.t);

    var list = [];
    function add(d, fn) { if (d < FAR) list.push({ d: d, f: fn }); }
    function dist(gx, gy) { return Math.hypot(gx - cam.x, gy - cam.y); }

    var boxes = (scene.boxes || []).slice();
    if (scene.interior) boxes = boxes.concat(shell(scene));

    boxes.forEach(function (b) {
      var d = dist(b.x + b.w / 2, b.y + b.d / 2);
      add(d, function () { drawBox(ctx, scene, b, d); });
    });

    (scene.fixtures || []).forEach(function (f) {
      function bay(bx, by, bd) { add(bd, function () { drawRailBay(ctx, scene, f, bx, by, bd); }); }
      var d = dist(f.x, f.y);
      if (f.kind === 'railing') {
        var horiz = f.axis === 'x';
        for (var i = 0; i < f.len; i++) {
          var bx = horiz ? f.x + i : f.x, by = horiz ? f.y : f.y + i;
          var bd = dist(bx, by);
          if (bd > 26) continue;
          bay(bx, by, bd);
        }
        return;
      }
      if (d > FAR) return;
      if (f.kind === 'rug') {
        add(FAR - 0.01, function () {
          quad(ctx, [f.x, f.y, 0.008], [f.x + f.w, f.y, 0.008],
               [f.x + f.w, f.y + f.d, 0.008], [f.x, f.y + f.d, 0.008], fogged(f.color, d));
        });
        return;
      }
      if (f.kind === 'sunbeam') {
        /* A beam is authored where the window is and slid onto the floor by
           the iso shadow offset, which is a screen-space trick. On the grid
           that offset is due +y, so the pool goes there — otherwise it lands
           inside the wall it came through. */
        add(FAR - 0.02, function () {
          ctx.save();
          ctx.globalCompositeOperation = 'lighter';
          ctx.globalAlpha = o.quality === 'high' ? 0.2 : 0.13;
          var y = f.y + 1.5;
          quad(ctx, [f.x, y, 0.01], [f.x + f.w, y, 0.01],
               [f.x + f.w, y + f.d, 0.01], [f.x, y + f.d, 0.01], '#ffe6b4');
          ctx.restore();
        });
        return;
      }
      if (f.kind === 'lamp') { add(d, function () { drawLamp(ctx, scene, f, d); }); return; }
      if (SOLID[f.kind]) { add(d, function () { drawSolid(ctx, scene, f, d); }); return; }
      var lift = SPRITE[f.kind];
      if (lift === undefined) return;
      add(d, function () {
        if (d < 20) blob(ctx, f.x, f.y, 0.4 * lift, scene, d);
        billboard(ctx, f.x, f.y, lift, d, function () { o.drawFixture(f); });
      });
    });

    (o.actors || []).forEach(function (a) {
      var d = dist(a.x, a.y);
      if (d > 34) return;
      add(d, function () {
        blob(ctx, a.x, a.y, 0.34, scene, d);
        billboard(ctx, a.x, a.y, 1, d, function () { o.drawActor(a); });
      });
    });

    // The zone ring is the one piece of chrome painted into the world, and it
    // has to stay painted into it: a ring on the pavement, not a circle on
    // the glass.
    if (o.zone) {
      var zd = dist(o.zone.x, o.zone.y);
      /* Stroked, never punched out of a disc: an annulus cut with
         destination-out takes the ground with it and leaves a hole straight
         through the canvas. */
      add(zd - 0.001, function () {
        var r = (o.zone.radius || 4) * 0.42;
        var n = 36, pts = [];
        for (var i = 0; i <= n; i++) {
          var ang = (i / n) * Math.PI * 2;
          var c = toCam(o.zone.x + Math.cos(ang) * r, o.zone.y + Math.sin(ang) * r, 0.02);
          if (c.f < NEAR) return;              // the ring is around your feet
          pts.push([projX(c), projY(c)]);
        }
        var pulse = 0.5 + 0.5 * Math.sin(o.t * 1.8);
        ctx.save();
        ctx.globalAlpha = (0.22 + pulse * 0.26) * (1 - fogT(zd));
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 1.6;
        ctx.beginPath();
        ctx.moveTo(pts[0][0], pts[0][1]);
        for (var j = 1; j < pts.length; j++) ctx.lineTo(pts[j][0], pts[j][1]);
        ctx.stroke();
        ctx.restore();
      });
    }

    list.sort(function (a, b) { return b.d - a.d; });
    for (var i = 0; i < list.length; i++) list[i].f();

    if (scene.interior) {
      var c = shell(scene).ceiling;
      if (c) {
        quad(ctx, [c[0], c[1], CEIL], [c[2], c[1], CEIL], [c[2], c[3], CEIL], [c[0], c[3], CEIL],
             E.mix(scene.sky[0], '#e8dcc4', 0.34));
      }
    }

    if (o.quality === 'high') sunWash(ctx, W, H);
  };

  // the same warm haze the iso view lays over the plate, aimed at the sun
  function sunWash(ctx, W, H) {
    var sf = SUNX * cam.cy + SUNY * cam.sy;
    if (sf <= 0) return;
    var sr = -SUNX * cam.sy + SUNY * cam.cy;
    var sx = W / 2 + cam.fl * sr / Math.max(sf, 0.2);
    ctx.save();
    ctx.globalCompositeOperation = 'lighter';
    var g = ctx.createRadialGradient(sx, horizon() - cam.fl * 0.5, 0, sx, horizon() - cam.fl * 0.5, W * 0.85);
    g.addColorStop(0, 'rgba(255,224,170,0.15)');
    g.addColorStop(1, 'rgba(255,224,170,0)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, W, H);
    ctx.restore();
  }

})(window.ETM);
