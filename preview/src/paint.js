/* Escape the Matrix — scene painter.
   Everything that puts a scene onto a 2D canvas, lifted out of the client
   shell so an offline bake script can draw any scene without the interactive
   app. E.Painter(S) reads one live state object the caller mutates between
   frames — ctx, scene, t, W, H, cam, zoom, quality, emissive, actors,
   player — fresh on every call; nothing is copied at construction. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  var ELEV = { s: 6, p: 5, q: 6, g: 7, d: 6, r: 0, k: 0, f: 0, t: 0, w: -5 };
  E.ELEV = ELEV;

  E.Painter = function (S) {

    /* Where a grid point lands on the glass under the plate's camera. The
       culling below only ever runs while the plate is being painted — the
       eye-level view culls for itself inside fpv.js — so this is the iso
       projection and nothing else. */
    function toScreen(gx, gy, z) {
      var p = E.isoc(gx, gy);
      return {
        x: (p.x - S.cam.x) * S.zoom + S.W / 2,
        y: (p.y - S.cam.y - (z || 0)) * S.zoom + S.H / 2,
        ok: true, d: 0
      };
    }

    /* A city this size holds a few hundred fixtures and a few dozen people,
       and almost none of them are on screen at once. Both the shadow pass and
       the sorted object layer walk the whole list every frame, so both cull
       against the window first — generously, because a lamp post is tall and
       its shadow is long. */
    function onScreen(gx, gy, pad) {
      var s = toScreen(gx, gy, 0);
      var p = pad || 280;
      return s.x > -p && s.x < S.W + p && s.y > -p && s.y < S.H + p;
    }

    function offscreen(b) {
      var near = toScreen(b.x + b.w, b.y + b.d, 0);
      var far = toScreen(b.x, b.y, b.h);
      var pad = 200;
      return near.x < -pad || far.x > S.W + pad || near.y < -pad || far.y > S.H + pad;
    }

    // grid range the viewport can actually see, so ground cost tracks the window
    function visibleRange() {
      var xs = [], ys = [];
      [[0, 0], [S.W, 0], [0, S.H], [S.W, S.H]].forEach(function (c) {
        var wx = (c[0] - S.W / 2) / S.zoom + S.cam.x;
        var wy = (c[1] - S.H / 2) / S.zoom + S.cam.y - E.TH / 2;
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

    /* --------------------------------------------------------------- sky */

    // buildings, sorted back-to-front for the painter
    function solids() {
      return (S.scene.boxes || []).slice()
        .sort(function (a, b) { return (a.y + a.d) - (b.y + b.d) || (a.x + a.w) - (b.x + b.w); });
    }

    function drawSky() {
      var s = S.scene.sky;
      var g = S.ctx.createLinearGradient(0, 0, 0, S.H);
      g.addColorStop(0, s[0]);
      g.addColorStop(0.58, s[1]);
      g.addColorStop(1, s[2]);
      S.ctx.fillStyle = g;
      S.ctx.fillRect(0, 0, S.W, S.H);
      if (S.scene.interior) return;
      if (S.scene.night) { drawNight(); return; }

      // the sun sits off to the camera-right, which is where every shadow points away from
      var sx = S.W * 0.86, sy = S.H * 0.10 - S.cam.y * 0.06;
      var sg = S.ctx.createRadialGradient(sx, sy, 0, sx, sy, S.W * 0.55);
      sg.addColorStop(0, 'rgba(255,246,214,0.55)');
      sg.addColorStop(0.35, 'rgba(255,232,180,0.14)');
      sg.addColorStop(1, 'rgba(255,232,180,0)');
      S.ctx.fillStyle = sg;
      S.ctx.fillRect(0, 0, S.W, S.H);

      drawClouds();
    }

    /* The night exterior's sky. The moon hangs high on the camera-left —
       opposite the warm the lamps own — with thin deterministic stars above
       the roofline and no clouds, which on a plate this dark would only read
       as smears. */
    function drawNight() {
      for (var i = 0; i < 46; i++) {
        var n1 = E.hash(i * 3.1, 7.7), n2 = E.hash(11.3, i * 1.9);
        S.ctx.fillStyle = 'rgba(216,226,240,' + (0.2 + E.hash(i * 1.3, 2.1) * 0.4).toFixed(2) + ')';
        S.ctx.fillRect(n1 * S.W, n2 * S.H * 0.45, 1.5, 1.5);
      }
      // Sized off the frame, like the sun's glow is — a fixed radius that
      // reads on the preview's viewport is a dot on a full-width plate.
      var mx = S.W * 0.20, my = S.H * 0.13 - S.cam.y * 0.06;
      var mr = Math.max(13, S.W * 0.014);
      var mg = S.ctx.createRadialGradient(mx, my, 0, mx, my, S.W * 0.28);
      mg.addColorStop(0, 'rgba(216,226,240,0.28)');
      mg.addColorStop(0.35, 'rgba(216,226,240,0.07)');
      mg.addColorStop(1, 'rgba(216,226,240,0)');
      S.ctx.fillStyle = mg;
      S.ctx.fillRect(0, 0, S.W, S.H);
      S.ctx.beginPath(); S.ctx.arc(mx, my, mr, 0, Math.PI * 2);
      S.ctx.fillStyle = '#e3eaf4'; S.ctx.fill();
      // two maria, which is what makes a pale disc read as the moon
      S.ctx.beginPath(); S.ctx.arc(mx - mr * 0.3, my - mr * 0.22, mr * 0.24, 0, Math.PI * 2);
      S.ctx.fillStyle = 'rgba(159,175,198,0.55)'; S.ctx.fill();
      S.ctx.beginPath(); S.ctx.arc(mx + mr * 0.36, my + mr * 0.3, mr * 0.16, 0, Math.PI * 2);
      S.ctx.fillStyle = 'rgba(159,175,198,0.4)'; S.ctx.fill();
    }

    function drawClouds() {
      var pack = [
        [0.10, 0.14, 150, 0.55], [0.30, 0.09, 110, 0.4],
        [0.52, 0.17, 190, 0.5], [0.74, 0.11, 130, 0.35], [0.92, 0.22, 160, 0.42]
      ];
      S.ctx.save();
      for (var i = 0; i < pack.length; i++) {
        var c = pack[i];
        var drift = (S.t * (4 + i * 1.6)) % (S.W + 700) - 350;
        var x = c[0] * S.W + drift * 0.25 - S.cam.x * 0.06;
        var y = c[1] * S.H - S.cam.y * 0.03;
        var r = c[2] * S.zoom;
        S.ctx.fillStyle = 'rgba(255,255,255,' + c[3] + ')';
        for (var j = 0; j < 5; j++) {
          var o = E.hash(i * 7 + j, j * 3.3);
          S.ctx.beginPath();
          S.ctx.ellipse(x + (j - 2) * r * 0.34, y + (o - 0.5) * r * 0.18,
                        r * (0.32 + o * 0.2), r * (0.14 + o * 0.08), 0, 0, Math.PI * 2);
          S.ctx.fill();
        }
      }
      S.ctx.restore();
    }

    /* ------------------------------------------------------------ ground */

    function drawGround() {
      var rows = S.scene.rows;
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
            var sh = 0.5 + 0.5 * Math.sin(S.t * 0.9 + x * 0.55 + y * 0.8);
            col = E.mix(col, '#8ec4de', 0.05 + sh * 0.16);
          }

          var p = E.iso(x, y);
          var below = rows[y + 1] ? rows[y + 1][x] : '.';
          var right = row[x + 1] || '.';
          var eb = ELEV[below] || 0, er = ELEV[right] || 0;
          var Bm = { x: p.x, y: p.y + E.TH - e };
          if (e > eb) {
            var L = { x: p.x - E.TW / 2, y: p.y + E.TH / 2 - e };
            E.poly(S.ctx, [L, Bm, { x: Bm.x, y: Bm.y + (e - eb) }, { x: L.x, y: L.y + (e - eb) }], E.shade(col, 0.72));
          }
          if (e > er) {
            var R = { x: p.x + E.TW / 2, y: p.y + E.TH / 2 - e };
            E.poly(S.ctx, [Bm, R, { x: R.x, y: R.y + (e - er) }, { x: Bm.x, y: Bm.y + (e - er) }], E.shade(col, 0.86));
          }

          S.ctx.save();
          S.ctx.translate(0, -e);
          E.tile(S.ctx, x, y, col);
          if (code === 'k' && x % 2 === 0) E.tile(S.ctx, x, y, E.rgba('#ffffff', 0.30));
          if (code === 'f' && (x + y) % 2 === 0) E.tile(S.ctx, x, y, E.rgba('#5a3a1d', 0.10));
          if (g.water && E.hash(x * 3.1, y * 5.9) > 0.8) {
            var c2 = E.isoc(x, y);
            S.ctx.fillStyle = E.rgba('#ffffff', 0.10 + 0.12 * Math.sin(S.t * 1.7 + x + y));
            S.ctx.fillRect(c2.x - 13, c2.y - 1, 26, 2);
          }
          S.ctx.restore();
        }
      }
    }

    /* ----------------------------------------------------------- shadows */

    function sunOff(h) {
      var s = S.scene.sun;
      return { x: s.x * h, y: s.y * h };
    }

    function shadowFill() { return E.rgba(S.scene.sun.shade, 1); }

    function withShadowInk(fn) {
      S.ctx.save();
      S.ctx.globalCompositeOperation = 'multiply';
      S.ctx.globalAlpha = 0.42;
      fn();
      S.ctx.restore();
    }

    function boxShadow(x, y, w, d, h) {
      var o = sunOff(h);
      var A = E.iso(x, y), B = E.iso(x + w, y), C = E.iso(x + w, y + d), D = E.iso(x, y + d);
      function off(p) { return { x: p.x + o.x, y: p.y + o.y }; }
      E.poly(S.ctx, [D, A, B, off(B), off(C), off(D)], shadowFill());
    }

    function blobShadow(gx, gy, h, rx, ry) {
      var o = sunOff(h);
      var c = E.isoc(gx, gy);
      S.ctx.beginPath();
      S.ctx.ellipse(c.x + o.x * 0.6, c.y + o.y * 0.6, rx, ry, 0, 0, Math.PI * 2);
      S.ctx.fillStyle = shadowFill();
      S.ctx.fill();
    }

    function drawShadows() {
      withShadowInk(function () {
        solids().forEach(function (b) { if (!offscreen(b)) boxShadow(b.x, b.y, b.w, b.d, b.h); });
        (S.scene.fixtures || []).forEach(function (f) {
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
            case 'shower': boxShadow(f.x, f.y, 1.4, 1.4, 6); break;
            case 'basin': blobShadow(f.x + 0.5, f.y + 0.3, 16, 13, 7); break;
            case 'toilet': blobShadow(f.x + 0.5, f.y + 0.5, 17, 20, 10); break;
          }
        });
        S.actors.concat([S.player]).forEach(function (a) {
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
        lit: E.mix(base, S.scene.sun.warm, 0.26),
        shade: E.mix(E.shade(base, 0.7), S.scene.sun.shade, 0.3),
        sky: S.scene.sky[0]
      };

      var type = TYPES[b.type] || TYPES.flats;
      if (type.roof) type.roof(b, tone);

      var faces = E.box(S.ctx, {
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

    function fr(q, u, v, uw, vh, fill) { E.faceRect(S.ctx, q, u, v, uw, vh, fill); }

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
      E.poly(S.ctx, [l, r, apex], E.mix(tone.base, '#ffffff', 0.34));
      E.poly(S.ctx, [
        { x: l.x + (apex.x - l.x) * 0.1, y: l.y + (apex.y - l.y) * 0.1 },
        { x: r.x + (apex.x - r.x) * 0.1, y: r.y + (apex.y - r.y) * 0.1 },
        { x: apex.x, y: apex.y + rise * 0.18 }
      ], E.shade(E.mix(tone.base, '#ffffff', 0.34), 0.93));
    }

    // wide shallow steps running the length of the frontage
    function steps(b, colour) {
      var y = b.y + b.d;
      E.slab(S.ctx, b.x + 0.35, y, b.w - 0.7, 0.62, 10, E.mix(colour, '#ffffff', 0.2));
      E.slab(S.ctx, b.x + 0.2, y + 0.18, b.w - 0.4, 0.56, 6, colour);
      E.slab(S.ctx, b.x + 0.05, y + 0.36, b.w - 0.1, 0.5, 2, E.shade(colour, 0.94));
    }

    // a flat canopy on posts, over the entrance
    function canopy(b, colour, uw) {
      var w = b.w * (uw || 0.5), x = b.x + (b.w - w) / 2, y = b.y + b.d;
      E.slab(S.ctx, x, y - 0.1, w, 1.15, 34, colour);
      E.slab(S.ctx, x + 0.06, y - 0.04, w - 0.12, 1.03, 31, E.shade(colour, 0.82));
      [[x + 0.15, y + 0.9], [x + w - 0.15, y + 0.9]].forEach(function (p) {
        var c = E.isoc(p[0], p[1]);
        S.ctx.fillStyle = '#8d8677';
        S.ctx.fillRect(c.x - 1.5, c.y - 33, 3, 33);
      });
    }

    // small plant on the roof — the thing that stops a flat top reading as a lid
    function roofUnits(b, n, colour) {
      for (var i = 0; i < n; i++) {
        var w = Math.min(1.4, b.w / 4);
        var x = b.x + b.w * (0.22 + i * 0.34), y = b.y + b.d * 0.3;
        E.box(S.ctx, {
          x: x, y: y, w: w, d: Math.min(1.2, b.d * 0.3), h: b.h + 12 + i * 3,
          color: colour || '#9aa0a2',
          left: E.mix('#7c8285', S.scene.sun.shade, 0.2),
          right: E.mix('#a9afb1', S.scene.sun.warm, 0.18),
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
          if (h1 > 0.88) fr(faces.L, u, v, uw, vh * 0.55, E.mix('#e8d3ae', S.scene.sun.warm, 0.4));
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
      var f = E.box(S.ctx, {
        x: x, y: y, w: tw, d: tw, h: b.h + 78,
        color: stone,
        left: E.mix(E.shade(stone, 0.72), S.scene.sun.shade, 0.28),
        right: E.mix(stone, S.scene.sun.warm, 0.24),
        top: E.mix(stone, '#ffffff', 0.3)
      });
      // belfry openings
      fr(f.L, 0.22, 0.03, 0.24, 0.05, E.shade(stone, 0.45));
      fr(f.L, 0.54, 0.03, 0.24, 0.05, E.shade(stone, 0.45));
      // clock, and hands at ten past eleven
      var p = E.facePt(f.L, 0.5, 0.1);
      var rad = Math.max(7, tw * 4.4);
      S.ctx.beginPath(); S.ctx.arc(p.x, p.y, rad, 0, Math.PI * 2);
      S.ctx.fillStyle = '#efe7d2'; S.ctx.fill();
      S.ctx.strokeStyle = E.shade(stone, 0.6); S.ctx.lineWidth = 1.6; S.ctx.stroke();
      S.ctx.strokeStyle = '#3b332a'; S.ctx.lineWidth = 1.6;
      S.ctx.beginPath();
      S.ctx.moveTo(p.x, p.y); S.ctx.lineTo(p.x - rad * 0.5, p.y - rad * 0.3);
      S.ctx.moveTo(p.x, p.y); S.ctx.lineTo(p.x + rad * 0.1, p.y - rad * 0.62);
      S.ctx.stroke();
      // spire
      var t = E.iso(x, y), t2 = E.iso(x + tw, y + tw);
      var mid = { x: (t.x + t2.x) / 2, y: (t.y + t2.y) / 2 - (b.h + 78) };
      E.poly(S.ctx, [
        { x: mid.x - tw * 16, y: mid.y }, { x: mid.x + tw * 16, y: mid.y },
        { x: mid.x, y: mid.y - 34 }
      ], E.mix('#6f7a6a', S.scene.sun.shade, 0.2));
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
      E.poly(S.ctx, arch, E.shade(b.color, 0.4));
      E.poly(S.ctx, arch.slice(0, 11).concat([
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
      fr(faces.L, 0.08, 0.42, 0.44, 0.1, E.mix('#2f3640', S.scene.sun.shade, 0.2));
      var a = E.facePt(faces.L, 0.14, 0.47), z = E.facePt(faces.L, 0.46, 0.47);
      S.ctx.strokeStyle = '#e0d6c2'; S.ctx.lineWidth = 2.4;
      S.ctx.beginPath(); S.ctx.moveTo(a.x, a.y); S.ctx.lineTo(z.x, z.y); S.ctx.stroke();
      [a, z].forEach(function (p) {
        S.ctx.fillStyle = '#e0d6c2';
        S.ctx.beginPath(); S.ctx.ellipse(p.x, p.y, 3.4, 4.6, 0, 0, Math.PI * 2); S.ctx.fill();
      });

      // roller shutter, slatted
      fr(faces.L, 0.6, 0.52, 0.32, 0.44, E.shade(brick, 0.72));
      for (var s = 0; s < 9; s++) {
        fr(faces.L, 0.6, 0.54 + s * 0.048, 0.32, 0.012, E.shade(brick, 0.62));
      }
      // a lit strip of the room behind, over the door
      fr(faces.L, 0.08, 0.56, 0.44, 0.3, E.shade(brick, 0.86));
      fr(faces.L, 0.12, 0.6, 0.36, 0.2, E.mix('#e8d3ae', S.scene.sun.warm, 0.35));
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
      var f = E.box(S.ctx, {
        x: x, y: y, w: tw, d: tw, h: b.h + 96,
        color: '#c2c8cc',
        left: E.mix('#98a0a6', S.scene.sun.shade, 0.24),
        right: E.mix('#cdd3d7', S.scene.sun.warm, 0.2),
        top: '#dfe4e7'
      });
      fr(f.L, 0.1, 0.02, 0.8, 0.05, pane(tone, 0.1, 0.8));
      // the cab, wider than the shaft
      var cab = E.box(S.ctx, {
        x: x - 0.45, y: y - 0.45, w: tw + 0.9, d: tw + 0.9, h: b.h + 118,
        color: '#38414a',
        left: E.mix('#5c7186', S.scene.sun.shade, 0.2),
        right: E.mix('#7d93a6', S.scene.sun.warm, 0.2),
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
      S.ctx.fillStyle = '#3f6b57';
      S.ctx.fillRect(c.x - 3, c.y - 9, 6, 18);
      S.ctx.fillRect(c.x - 9, c.y - 3, 18, 6);
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
        E.poly(S.ctx, arch, pane(tone, 0.35, 0.35));
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
      S.ctx.fillStyle = E.mix(b.blade, S.scene.sun.shade, 0.1);
      S.ctx.fillRect(p.x - 2, p.y - h - 26, 4, 26);
      E.poly(S.ctx, [
        { x: p.x - 3, y: p.y - h - 24 }, { x: p.x + 26, y: p.y - h - 14 },
        { x: p.x + 26, y: p.y - h + 6 }, { x: p.x - 3, y: p.y - h - 4 }
      ], b.blade);
      E.poly(S.ctx, [
        { x: p.x + 2, y: p.y - h - 20 }, { x: p.x + 21, y: p.y - h - 13 },
        { x: p.x + 21, y: p.y - h - 5 }, { x: p.x + 2, y: p.y - h - 12 }
      ], E.mix(b.blade, '#ffffff', 0.5));
    }

    // A mushroom house. The box under it is a sliver whose top face is the
    // lawn; the mushroom itself is drawn whole by mushroomCap, because a stem
    // is round and a box face is not.
    function paintMushroom(b, faces, tone) { }

    // The stem, its window and door, the cap, and the chimney behind it.
    function mushroomCap(b) {
      var c = E.iso(b.x + b.w / 2, b.y + b.d / 2);
      var half = E.iso(b.x + b.w, b.y).x - c.x;
      var cx = c.x;
      var by = c.y + 44;
      var hS = 148;
      var ty = by - hS;
      var baseR = 108, topR = 82;
      var rx = half - 4, capH = 112, lip = 30;
      var cy = ty + 10;
      var glass = S.scene.night ? '#f2b768' : E.mix('#9dc4e0', '#ffffff', 0.25);
      var stem = '#d3c19c';

      // contact shadow, or the stem floats
      S.ctx.fillStyle = E.rgba('#2a1f14', 0.26);
      S.ctx.beginPath();
      S.ctx.ellipse(cx + 10, by + 8, baseR + 24, 26, 0, 0, Math.PI * 2);
      S.ctx.fill();

      // the stem: waisted under the cap, flared at the foot
      function stemPath() {
        S.ctx.beginPath();
        S.ctx.moveTo(cx - baseR, by);
        S.ctx.bezierCurveTo(cx - baseR + 26, by - hS * 0.62, cx - topR - 6, ty + hS * 0.3, cx - topR, ty);
        S.ctx.lineTo(cx + topR, ty);
        S.ctx.bezierCurveTo(cx + topR + 6, ty + hS * 0.3, cx + baseR - 26, by - hS * 0.62, cx + baseR, by);
        S.ctx.quadraticCurveTo(cx, by + 30, cx - baseR, by);
        S.ctx.closePath();
      }
      stemPath();
      S.ctx.fillStyle = stem;
      S.ctx.fill();
      S.ctx.save();
      S.ctx.beginPath();
      S.ctx.rect(cx - baseR - 6, ty - 6, baseR * 0.62, hS + 44);
      S.ctx.clip();
      stemPath();
      S.ctx.fillStyle = E.rgba(E.mix(E.shade(stem, 0.72), S.scene.sun.shade, 0.3), 0.5);
      S.ctx.fill();
      S.ctx.restore();
      S.ctx.strokeStyle = E.rgba('#fff0cf', 0.22);
      S.ctx.lineWidth = 2.4;
      S.ctx.beginPath();
      S.ctx.moveTo(cx + baseR - 8, by - 10);
      S.ctx.quadraticCurveTo(cx + topR + 16, by - hS * 0.55, cx + topR - 2, ty + 10);
      S.ctx.stroke();

      // the arched window, mullioned, warm behind the glass
      var wx = cx + 34, wa = by - hS * 0.60, wb = by - hS * 0.28;
      S.ctx.fillStyle = E.mix(stem, '#ffffff', 0.2);
      S.ctx.beginPath();
      S.ctx.moveTo(wx - 27, wb);
      S.ctx.lineTo(wx - 27, wa);
      S.ctx.arc(wx, wa, 27, Math.PI, 0);
      S.ctx.lineTo(wx + 27, wb);
      S.ctx.closePath();
      S.ctx.fill();
      S.ctx.fillStyle = glass;
      S.ctx.beginPath();
      S.ctx.moveTo(wx - 21, wb - 5);
      S.ctx.lineTo(wx - 21, wa);
      S.ctx.arc(wx, wa, 21, Math.PI, 0);
      S.ctx.lineTo(wx + 21, wb - 5);
      S.ctx.closePath();
      S.ctx.fill();
      S.ctx.strokeStyle = E.mix(stem, '#ffffff', 0.2);
      S.ctx.lineWidth = 2.6;
      S.ctx.beginPath();
      S.ctx.moveTo(wx, wa - 21); S.ctx.lineTo(wx, wb - 5);
      S.ctx.moveTo(wx - 21, wa + 2); S.ctx.lineTo(wx + 21, wa + 2);
      S.ctx.stroke();
      if (S.scene.night) {
        S.emissive.push({
          pts: [{ x: wx - 20, y: wa - 16 }, { x: wx + 20, y: wa - 16 },
                { x: wx + 20, y: wb - 6 }, { x: wx - 20, y: wb - 6 }],
          color: '#ffcf8e', a: 0.3, glow: 0.5,
        });
      }

      // the round-topped plank door
      var dx = cx - 46, dy = by + 18;
      S.ctx.fillStyle = '#7c5130';
      S.ctx.beginPath();
      S.ctx.moveTo(dx - 24, dy);
      S.ctx.lineTo(dx - 24, dy - 44);
      S.ctx.arc(dx, dy - 44, 24, Math.PI, 0);
      S.ctx.lineTo(dx + 24, dy);
      S.ctx.closePath();
      S.ctx.fill();
      S.ctx.strokeStyle = E.shade('#7c5130', 0.72);
      S.ctx.lineWidth = 1.6;
      S.ctx.beginPath();
      for (var pl = -1; pl <= 1; pl++) {
        S.ctx.moveTo(dx + pl * 12, dy - (pl === 0 ? 66 : 52));
        S.ctx.lineTo(dx + pl * 12, dy);
      }
      S.ctx.stroke();
      S.ctx.fillStyle = glass;
      S.ctx.beginPath();
      S.ctx.ellipse(dx, dy - 48, 5, 4.6, 0, 0, Math.PI * 2);
      S.ctx.fill();

      // the dome
      var L = { x: cx - rx, y: cy }, R = { x: cx + rx, y: cy };
      var capColor = E.mix('#a8492a', S.scene.sun.warm, 0.08);
      S.ctx.beginPath();
      S.ctx.moveTo(L.x, L.y);
      S.ctx.bezierCurveTo(cx - rx * 0.82, cy - capH * 1.16, cx + rx * 0.82, cy - capH * 1.16, R.x, R.y);
      S.ctx.bezierCurveTo(cx + rx * 0.5, cy + lip, cx - rx * 0.5, cy + lip, L.x, L.y);
      S.ctx.closePath();
      S.ctx.fillStyle = capColor;
      S.ctx.fill();

      // the under-edge, in its own shade
      S.ctx.beginPath();
      S.ctx.moveTo(L.x, L.y);
      S.ctx.bezierCurveTo(cx - rx * 0.5, cy + lip, cx + rx * 0.5, cy + lip, R.x, R.y);
      S.ctx.bezierCurveTo(cx + rx * 0.6, cy + lip * 0.35, cx - rx * 0.6, cy + lip * 0.35, L.x, L.y);
      S.ctx.closePath();
      S.ctx.fillStyle = E.mix('#77341c', S.scene.sun.shade, 0.2);
      S.ctx.fill();

      // shade on the camera-left half, rim light on the sun side
      S.ctx.save();
      S.ctx.beginPath();
      S.ctx.rect(L.x, cy - capH * 1.02, rx * 0.86, capH * 1.1);
      S.ctx.clip();
      S.ctx.beginPath();
      S.ctx.moveTo(L.x, L.y);
      S.ctx.bezierCurveTo(cx - rx * 0.82, cy - capH * 1.16, cx + rx * 0.82, cy - capH * 1.16, R.x, R.y);
      S.ctx.closePath();
      S.ctx.fillStyle = E.rgba(E.shade(capColor, 0.72), 0.4);
      S.ctx.fill();
      S.ctx.restore();
      S.ctx.strokeStyle = E.rgba(E.mix('#ffd9a8', capColor, 0.3), 0.6);
      S.ctx.lineWidth = 3;
      S.ctx.beginPath();
      S.ctx.ellipse(cx, cy - capH * 0.12, rx * 0.86, capH * 0.9, 0, -1.25, -0.25);
      S.ctx.stroke();

      // the spots
      for (var s = 0; s < 8; s++) {
        var t = 0.09 + s * 0.11;
        var n = E.hash(b.seed + s * 5.1, s * 2.9);
        var lift = Math.sqrt(Math.max(0, 1 - Math.pow((t - 0.5) * 2, 2)));
        var px = cx + (t - 0.5) * 2 * rx * 0.86;
        var py = cy - capH * lift * (0.3 + n * 0.42);
        var r = (7 + n * 8) * (0.6 + lift * 0.4);
        S.ctx.fillStyle = E.rgba('#e9d9bd', 0.9);
        S.ctx.beginPath();
        S.ctx.ellipse(px, py, r, r * 0.62, 0, 0, Math.PI * 2);
        S.ctx.fill();
      }
    }

    // Cream and maroon over a pink storefront: tall studio windows above a
    // marquee band of bulbs, pink double doors between columns, and a gabled
    // attic with a fan window on the roof.
    function paintDance(b, faces, tone) {
      var maroon = '#8e3352', pink = '#dd7ba0';
      var cream = E.mix(b.color, '#ffffff', 0.18);

      fr(faces.L, 0, 0.02, 1, 0.035, maroon);
      fr(faces.L, 0, 0.055, 1, 0.01, E.shade(maroon, 0.7));

      // the studio floor: three tall dark windows, a gridded wing window each side
      for (var i = 0; i < 3; i++) {
        var u = 0.28 + i * 0.16;
        fr(faces.L, u, 0.10, 0.11, 0.30, E.mix('#232a3e', tone.sky, 0.16));
        fr(faces.L, u, 0.10, 0.11, 0.03, E.mix('#ffffff', tone.sky, 0.5));
      }
      [0.07, 0.82].forEach(function (wu) {
        fr(faces.L, wu, 0.13, 0.11, 0.22, '#d9a94e');
        fr(faces.L, wu + 0.012, 0.145, 0.086, 0.19, '#1d222e');
        for (var g = 1; g < 3; g++) {
          fr(faces.L, wu + 0.012 + g * 0.029, 0.145, 0.005, 0.19, '#d9a94e');
          fr(faces.L, wu + 0.012, 0.145 + g * 0.063, 0.086, 0.006, '#d9a94e');
        }
      });

      // the marquee: fascia, band, and the row of bulbs
      fr(faces.L, 0.02, 0.415, 0.96, 0.028, E.mix(cream, '#ffffff', 0.3));
      fr(faces.L, 0.02, 0.443, 0.96, 0.075, E.mix(maroon, '#c14d6e', 0.45));
      fr(faces.L, 0.02, 0.512, 0.96, 0.012, E.shade(maroon, 0.65));
      for (var m = 0; m < 7; m++) {
        var bp = E.facePt(faces.L, 0.24 + m * 0.087, 0.482);
        S.ctx.fillStyle = S.scene.night ? '#ffe9b0' : '#f6efe2';
        S.ctx.beginPath();
        S.ctx.ellipse(bp.x, bp.y, 2.6, 2.6, 0, 0, Math.PI * 2);
        S.ctx.fill();
        if (S.scene.night) {
          S.emissive.push({ ellipse: { x: bp.x, y: bp.y }, r: 2.6, color: '#ffd98e', a: 0.9, glow: 1.2 });
        }
      }

      // the ground floor: pink glass either side of pink double doors
      fr(faces.L, 0, 0.94, 1, 0.06, E.shade(tone.shade, 0.85));
      [[0.05, 0.24], [0.71, 0.24]].forEach(function (w) {
        fr(faces.L, w[0], 0.60, w[1], 0.30, E.shade(maroon, 0.55));
        fr(faces.L, w[0] + 0.012, 0.615, w[1] - 0.024, 0.27, pink);
        fr(faces.L, w[0] + 0.03, 0.615, w[1] * 0.28, 0.27, E.mix(pink, '#ffffff', 0.35));
      });
      [0.30, 0.66].forEach(function (u2) {
        fr(faces.L, u2, 0.55, 0.04, 0.39, cream);
        fr(faces.L, u2, 0.55, 0.012, 0.39, E.mix(cream, '#ffffff', 0.4));
      });
      fr(faces.L, 0.36, 0.555, 0.28, 0.065, E.shade(maroon, 0.5));
      fr(faces.L, 0.38, 0.62, 0.24, 0.32, E.shade(pink, 0.8));
      fr(faces.L, 0.386, 0.63, 0.108, 0.30, pink);
      fr(faces.L, 0.506, 0.63, 0.108, 0.30, pink);
      fr(faces.L, 0.497, 0.63, 0.006, 0.30, E.shade(maroon, 0.5));
      fr(faces.L, 0.395, 0.64, 0.03, 0.27, E.mix(pink, '#ffffff', 0.3));
      if (S.scene.night) {
        S.emissive.push({ pts: facePts(faces.L, 0.386, 0.63, 0.228, 0.30), color: '#ffbcd4', a: 0.25, glow: 0.8 });
      }

      // the sunlit return wall: plain, three bright panes
      fr(faces.R, 0, 0.02, 1, 0.035, E.shade(maroon, 0.85));
      for (var k = 0; k < 3; k++) {
        fr(faces.R, 0.14 + k * 0.28, 0.18, 0.14, 0.34, E.mix(E.mix('#4c6a86', tone.sky, 0.5), '#ffffff', 0.3));
        fr(faces.R, 0.14 + k * 0.28, 0.52, 0.14, 0.02, E.mix(tone.lit, '#ffffff', 0.3));
      }

      // the gable over the roofline, its fan window, and an antenna each end
      var l = E.facePt(faces.L, 0.26, 0), r = E.facePt(faces.L, 0.74, 0);
      var run = Math.abs(r.x - l.x);
      var apex = { x: (l.x + r.x) / 2, y: (l.y + r.y) / 2 - run * 0.5 };
      E.poly(S.ctx, [l, r, apex], maroon);
      E.poly(S.ctx, [
        { x: l.x + (apex.x - l.x) * 0.18, y: l.y + (apex.y - l.y) * 0.18 },
        { x: r.x + (apex.x - r.x) * 0.18, y: r.y + (apex.y - r.y) * 0.18 },
        { x: apex.x, y: apex.y + run * 0.08 }
      ], cream);
      var mid = { x: apex.x, y: (l.y + r.y) / 2 - run * 0.04 };
      S.ctx.beginPath();
      S.ctx.ellipse(mid.x, mid.y, run * 0.22, run * 0.19, 0, Math.PI, 0);
      S.ctx.closePath();
      S.ctx.fillStyle = '#7fd0cc';
      S.ctx.fill();
      S.ctx.strokeStyle = maroon;
      S.ctx.lineWidth = 1.4;
      S.ctx.stroke();
      [-0.55, 0, 0.55].forEach(function (a) {
        S.ctx.beginPath();
        S.ctx.moveTo(mid.x, mid.y);
        S.ctx.lineTo(mid.x + Math.sin(a) * run * 0.21, mid.y - Math.cos(a) * run * 0.18);
        S.ctx.stroke();
      });
      [0.05, 0.95].forEach(function (u3) {
        var pm = E.facePt(faces.L, u3, 0);
        S.ctx.fillStyle = '#3a3f4a';
        S.ctx.fillRect(pm.x - 1, pm.y - 34, 2, 34);
        S.ctx.beginPath();
        S.ctx.arc(pm.x, pm.y - 36, 2.4, 0, Math.PI * 2);
        S.ctx.fill();
      });
    }

    function danceFront(b) { steps(b, '#e6ddc6'); }

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
      mushroom: { face: paintMushroom, front: mushroomCap },
      dance: { face: paintDance, front: danceFront },
    };

    function drawOpenings(b, faces) {
      if (!b.openings) return;
      b.openings.forEach(function (o) {
        var q = b.side ? faces.R : faces.L;
        if (o.kind === 'window') {
          var wp = facePts(q, o.u, 0.14, o.w, 0.36);
          if (S.scene.night) {
            /* Somebody is up behind roughly half of them; the rest hold the
               night. Which half is which is hashed off the geometry, so the
               same window is awake on every bake. A room's own windows are
               never awake — from inside, a window is the night. */
            var awake = !S.scene.interior &&
              E.hash(b.x * 1.7 + o.u * 13.7, b.y * 2.3 + o.w * 7.1) > 0.55;
            E.poly(S.ctx, wp, awake ? '#f2b768' : '#1c2431');
            E.poly(S.ctx, facePts(q, o.u + o.w / 2 - 0.005, 0.14, 0.01, 0.36),
                   awake ? '#caa06a' : '#2a2f3a');
            if (awake) S.emissive.push({ pts: wp, color: '#ffcf8e', a: 0.42, glow: 1.1 });
            return;
          }
          E.poly(S.ctx, wp, E.mix('#9dc4e0', '#ffffff', 0.45));
          E.poly(S.ctx, facePts(q, o.u, 0.14, o.w, 0.10), '#ffffff');
          var frame = facePts(q, o.u + o.w / 2 - 0.005, 0.14, 0.01, 0.36);
          E.poly(S.ctx, frame, '#e6dcc9');
          S.emissive.push({ pts: wp, color: '#fff4d8', a: 0.22, glow: 0.5 });
        } else {
          E.poly(S.ctx, facePts(q, o.u, 0.40, o.w, 0.60), E.shade(b.color, 0.55));
        }
      });
    }

    /* ---------------------------------------------- sorted object layer */

    function drawObjects() {
      var list = [];

      (S.scene.boxes || []).forEach(function (b) {
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

      (S.scene.fixtures || []).forEach(function (f) {
        if (f.kind !== 'railing' && !onScreen(f.x, f.y)) return;
        // The fire's two flat layers bracket the room: the glow lies on the
        // boards under everything (with the sunbeams, which is the same kind
        // of thing), and the smoke sits over the lot including the walls.
        var k = f.kind === 'sunbeam' || f.kind === 'emberGlow' ? -9999
          : f.kind === 'rug' ? -9000
            : f.kind === 'smoke' ? 9999
              : f.x + f.y;
        list.push({ k: k, f: function () { drawFixture(f); } });
      });

      S.actors.forEach(function (a) {
        if (!onScreen(a.x, a.y, 120)) return;
        list.push({ k: a.x + a.y, f: function () { E.actor(S.ctx, a, S.t); } });
      });
      list.push({ k: S.player.x + S.player.y, f: function () { E.actor(S.ctx, S.player, S.t); } });

      list.sort(function (a, b) { return a.k - b.k; });
      for (var i = 0; i < list.length; i++) list[i].f();
    }

    function drawFurniture(b) {
      E.box(S.ctx, {
        x: b.x, y: b.y, w: b.w, d: b.d, h: b.h,
        color: b.color,
        left: E.mix(E.shade(b.color, 0.76), S.scene.sun.shade, 0.22),
        right: E.mix(b.color, S.scene.sun.warm, 0.2),
        top: E.mix(b.color, '#ffffff', 0.24)
      });
    }

    function drawFixture(f) {
      switch (f.kind) {
        case 'lamp':
          if (S.scene.night) {
            // Lit, and the whole reason the street is walkable after dark:
            // the head glows and the pavement under it gets its pool.
            var head = E.lampPost(S.ctx, f.x, f.y, { lit: true, color: '#3a3e44' });
            S.emissive.push({ ellipse: head, r: 6.5, color: '#ffd699', a: 0.9, glow: 1.5 });
            var foot = E.isoc(f.x, f.y);
            S.emissive.push({ ellipse: { x: foot.x + 12, y: foot.y + 2 }, r: 30, color: '#ffb15c', a: 0.08, glow: 0.55 });
          } else {
            E.lampPost(S.ctx, f.x, f.y, { lit: false, color: '#4c5158' });
          }
          break;
        case 'tree': drawTree(f); break;
        case 'bench': E.bench(S.ctx, f.x, f.y, { sun: S.scene.sun }); break;
        case 'planter': E.planter(S.ctx, f.x, f.y, {}); break;
        case 'railing': E.railing(S.ctx, f.x, f.y, f.len, f.axis, f.color); break;
        case 'rug': E.rug(S.ctx, f.x, f.y, f.w, f.d, f.color, f.edge); break;
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
        case 'shower': drawShower(f); break;
        case 'basin': drawBasin(f); break;
        case 'toilet': drawToilet(f); break;
        case 'sunbeam': drawSunbeam(f); break;
        case 'emberGlow': drawEmberGlow(f); break;
        case 'flames': drawFlames(f); break;
        case 'smoke': drawSmoke(f); break;
      }
    }

    /* ═════════════════════════════════════════════════════════ fire

       A flat that is going, as three fixtures rather than one — because a
       fire is three things that sort to three different depths, and one
       fixture drawing all of it would put the smoke behind the furniture.

         emberGlow   the pool of light thrown on the floor. Under everything,
                     like a sunbeam, because it is the same kind of object:
                     the room's key light, lying on the boards.
         flames      tongues standing where something is actually alight.
                     Sorted by x+y with the furniture, so a chair in front of
                     the doorway is in front of the fire in it.
         smoke       the layer sitting on the ceiling. Over the lot.

       Nothing here draws from a random source. The bake has to be repeatable
       (#39) — the same scene in, the same picture out — so every wobble in
       every tongue comes off the fixture's own position and the tongue's
       index, which is the same trick the crowd uses to be a crowd without
       being different every time it is painted. */

    /* the light a fire lies on the floor around itself */
    function drawEmberGlow(f) {
      var c = E.isoc(f.x, f.y);
      var r = (f.r || 5) * E.TW * 0.5;
      var g = S.ctx.createRadialGradient(c.x, c.y, 0, c.x, c.y, r);
      g.addColorStop(0, 'rgba(255,176,86,0.50)');
      g.addColorStop(0.45, 'rgba(226,106,44,0.24)');
      g.addColorStop(1, 'rgba(150,52,20,0)');

      S.ctx.save();
      S.ctx.globalCompositeOperation = 'lighter';
      S.ctx.globalAlpha = S.quality === 'high' ? 1 : 0.7;
      S.ctx.fillStyle = g;
      // Squashed to the ground plane, so the pool lies on the boards rather
      // than standing up as a disc facing the camera.
      S.ctx.translate(c.x, c.y);
      S.ctx.scale(1, 0.5);
      S.ctx.beginPath();
      S.ctx.arc(0, 0, r, 0, Math.PI * 2);
      S.ctx.fill();
      S.ctx.restore();
    }

    /* tongues of flame standing where something is alight */
    function drawFlames(f) {
      var c = E.isoc(f.x, f.y);
      var w = (f.w || 1.6) * E.TW * 0.5;
      var h = f.h || 120;
      var n = S.quality === 'high' ? 9 : 5;

      S.ctx.save();
      S.ctx.translate(c.x, c.y);
      S.ctx.globalCompositeOperation = 'lighter';

      for (var i = 0; i < n; i++) {
        // Deterministic scatter: the tongue's place in the fixture decides
        // its lean and its height, so this paints the same twice.
        var t = (i + 1) / (n + 1);
        var wobble = Math.sin((f.x + f.y) * 2.7 + i * 2.399);
        var lean = wobble * w * 0.22;
        var tall = h * (0.55 + 0.45 * Math.abs(Math.sin((f.x - f.y) + i * 1.7)));
        var foot = (t - 0.5) * w * 2;
        var wide = w * 0.30 * (0.6 + 0.4 * Math.abs(wobble));

        var g = S.ctx.createLinearGradient(0, 0, 0, -tall);
        g.addColorStop(0, 'rgba(255,238,196,0.95)');
        g.addColorStop(0.35, 'rgba(255,166,58,0.80)');
        g.addColorStop(0.75, 'rgba(214,74,26,0.42)');
        g.addColorStop(1, 'rgba(150,40,14,0)');

        S.ctx.fillStyle = g;
        S.ctx.beginPath();
        S.ctx.moveTo(foot - wide, 0);
        S.ctx.quadraticCurveTo(foot - wide * 0.5, -tall * 0.55, foot + lean, -tall);
        S.ctx.quadraticCurveTo(foot + wide * 0.5, -tall * 0.55, foot + wide, 0);
        S.ctx.closePath();
        S.ctx.fill();
      }

      // The white heart of it, low and small, which is what stops the whole
      // thing reading as orange paper.
      var core = S.ctx.createRadialGradient(0, -h * 0.14, 0, 0, -h * 0.14, w * 0.7);
      core.addColorStop(0, 'rgba(255,247,224,0.85)');
      core.addColorStop(1, 'rgba(255,160,60,0)');
      S.ctx.fillStyle = core;
      S.ctx.beginPath();
      S.ctx.arc(0, -h * 0.14, w * 0.7, 0, Math.PI * 2);
      S.ctx.fill();
      S.ctx.restore();
    }

    /*
      The layer of smoke sitting on the room, head height and coming down.

      Stacked slabs in the iso plane rather than one rectangle on the screen.
      A rectangle is the obvious way to do it and it is wrong: the screen has
      no idea where the room is, so a rectangle covers the walls, the void
      outside them and half the sky, and reads as a grey box laid over the
      picture. A slab is the room's own footprint, so a stack of them from the
      ceiling down is a layer *in* the room — thick at the top, thinning to
      nothing at about the height of somebody's face, which is the fact about
      a house fire that matters.
    */
    function drawSmoke(f) {
      var layers = S.quality === 'high' ? 8 : 4;
      var top = f.h || 150;
      // How far down it has come. Not to the floor: the bottom of the smoke is
      // the reason you can still see the room at all.
      var reach = top * 0.46;

      S.ctx.save();
      for (var i = 0; i < layers; i++) {
        var t = i / (layers - 1);
        // Deliberately thin. The whole challenge is choosing between nine
        // things you can see, so smoke that obscured the room would be
        // atmosphere bought with the mechanic — it has to say "this room is
        // filling up" without taking the furniture away.
        S.ctx.globalAlpha = 0.13 * (1 - t) + 0.02;
        E.slab(S.ctx, f.x, f.y, f.w || 6, f.d || 6, top - t * reach,
          i % 2 ? '#241713' : '#31201a');
      }
      S.ctx.restore();
    }

    // a shaft of morning light on the floorboards — the interior's key light
    function drawSunbeam(f) {
      // No sun, no shaft: at night the lamps are the room's key light.
      if (S.scene.night) return;
      var o = sunOff(96);
      S.ctx.save();
      S.ctx.globalCompositeOperation = 'lighter';
      S.ctx.globalAlpha = S.quality === 'high' ? 0.22 : 0.15;
      S.ctx.translate(o.x, o.y);
      E.slab(S.ctx, f.x, f.y, f.w, f.d, 0, '#ffe6b4');
      S.ctx.globalAlpha = 0.11;
      E.slab(S.ctx, f.x - 0.25, f.y - 0.25, f.w + 0.5, f.d + 0.5, 0, '#ffdda0');
      S.ctx.restore();
    }

    function drawTree(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#6b5540';
      S.ctx.fillRect(c.x - 3, c.y - 52, 6, 52);
      var leaf = f.lush ? '#4f7f42' : '#5d8546';
      var seedBase = f.x * 3.7 + f.y;
      for (var i = 0; i < 9; i++) {
        var n1 = E.hash(seedBase + i, i * 2.1), n2 = E.hash(i * 5.3, seedBase);
        var a = (i / 9) * Math.PI * 2 + n1;
        var sway = Math.sin(S.t * 0.9 + i + f.x) * 1.6;
        var rx = 20 + n2 * 12, ry = 14 + n1 * 8;
        var px = c.x + Math.cos(a) * (10 + n2 * 12) + sway;
        var py = c.y - 62 + Math.sin(a) * (7 + n1 * 8);
        // sun catches the upper-right of the canopy
        var tone = (Math.cos(a) > 0 && Math.sin(a) < 0)
          ? E.mix(leaf, S.scene.sun.warm, 0.34)
          : E.mix(leaf, S.scene.sun.shade, 0.22 + n1 * 0.2);
        S.ctx.fillStyle = tone;
        S.ctx.beginPath();
        S.ctx.ellipse(px, py, rx, ry, 0, 0, Math.PI * 2);
        S.ctx.fill();
      }
    }

    function drawPlant(f) {
      var c = E.isoc(f.x, f.y);
      E.box(S.ctx, { x: f.x, y: f.y, w: 0.7, d: 0.7, h: 18, color: '#a2714c' });
      for (var i = 0; i < 6; i++) {
        var a = (i / 6) * Math.PI * 2;
        var sway = Math.sin(S.t * 1.1 + i) * 1.2;
        S.ctx.strokeStyle = i % 2 ? '#527f46' : '#69965a';
        S.ctx.lineWidth = 3;
        S.ctx.beginPath();
        S.ctx.moveTo(c.x, c.y - 18);
        S.ctx.quadraticCurveTo(c.x + Math.cos(a) * 10, c.y - 40, c.x + Math.cos(a) * 18 + sway, c.y - 34 + Math.sin(a) * 6);
        S.ctx.stroke();
      }
    }

    function drawHydrant(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#a8543c';
      S.ctx.fillRect(c.x - 4, c.y - 18, 8, 18);
      S.ctx.beginPath(); S.ctx.arc(c.x, c.y - 19, 4.5, 0, Math.PI * 2); S.ctx.fill();
      S.ctx.fillStyle = E.mix('#a8543c', '#ffffff', 0.3);
      S.ctx.fillRect(c.x + 1, c.y - 18, 3, 18);
    }

    function drawBags(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.save();
      S.ctx.translate(c.x, c.y);
      [[-9, 0, 11], [4, -2, 13], [14, 1, 9]].forEach(function (b, i) {
        S.ctx.fillStyle = i % 2 ? '#4a4a52' : '#565660';
        S.ctx.beginPath();
        S.ctx.ellipse(b[0], b[1] - b[2] * 0.55, b[2], b[2] * 0.9, 0, 0, Math.PI * 2);
        S.ctx.fill();
        S.ctx.fillStyle = E.rgba('#ffffff', 0.12);
        S.ctx.beginPath();
        S.ctx.ellipse(b[0] + 3, b[1] - b[2] * 0.8, b[2] * 0.4, b[2] * 0.3, 0, 0, Math.PI * 2);
        S.ctx.fill();
      });
      S.ctx.restore();
    }

    function drawCar(f) {
      var w = 2.4, d = 1.1, body = f.color;
      [[0.25, 0.05], [0.25, 1.05], [2.15, 0.05], [2.15, 1.05]].forEach(function (o) {
        var p = E.isoc(f.x + o[0], f.y + o[1]);
        S.ctx.fillStyle = '#2b2a2c';
        S.ctx.beginPath(); S.ctx.ellipse(p.x, p.y - 4, 6, 5, 0, 0, Math.PI * 2); S.ctx.fill();
      });

      E.box(S.ctx, {
        x: f.x, y: f.y, w: w, d: d, h: 15, color: body,
        left: E.mix(E.shade(body, 0.74), S.scene.sun.shade, 0.24),
        right: E.mix(body, S.scene.sun.warm, 0.26),
        top: E.mix(body, '#ffffff', 0.3)
      });
      var cab = E.box(S.ctx, {
        x: f.x + 0.55, y: f.y + 0.13, w: w - 1.2, d: d - 0.26, h: 29,
        color: body,
        left: E.mix(E.shade(body, 0.7), S.scene.sun.shade, 0.26),
        right: E.mix(body, S.scene.sun.warm, 0.2),
        top: E.mix(body, '#ffffff', 0.34)
      });
      E.faceRect(S.ctx, cab.L, 0.08, 0.06, 0.84, 0.44, E.mix('#37475a', S.scene.sky[0], 0.4));
      E.faceRect(S.ctx, cab.R, 0.12, 0.06, 0.76, 0.44, E.mix('#8fb8d8', '#ffffff', 0.35));
    }

    function drawBus(f) {
      var w = 4.4, d = 1.5, body = f.color;
      [[0.5, 0.1], [0.5, 1.4], [3.6, 0.1], [3.6, 1.4]].forEach(function (o) {
        var p2 = E.isoc(f.x + o[0], f.y + o[1]);
        S.ctx.fillStyle = '#2b2a2c';
        S.ctx.beginPath(); S.ctx.ellipse(p2.x, p2.y - 5, 7, 6, 0, 0, Math.PI * 2); S.ctx.fill();
      });
      var shell = E.box(S.ctx, {
        x: f.x, y: f.y, w: w, d: d, h: 46, color: body,
        left: E.mix(E.shade(body, 0.74), S.scene.sun.shade, 0.24),
        right: E.mix(body, S.scene.sun.warm, 0.26),
        top: E.mix(body, '#ffffff', 0.3)
      });
      for (var i = 0; i < 5; i++) {
        E.faceRect(S.ctx, shell.L, 0.06 + i * 0.185, 0.14, 0.14, 0.34,
                   E.mix('#37475a', S.scene.sky[0], 0.45));
      }
      E.faceRect(S.ctx, shell.R, 0.12, 0.14, 0.72, 0.34, E.mix('#8fb8d8', '#ffffff', 0.32));
      E.faceRect(S.ctx, shell.L, 0.06, 0.04, 0.36, 0.07, E.mix(body, '#ffffff', 0.6));
    }

    function drawBusStop(f) {
      var frame = E.box(S.ctx, {
        x: f.x, y: f.y, w: 2.6, d: 1.0, h: 54, color: '#8f979d',
        left: 'rgba(180,205,225,0.34)', right: 'rgba(200,222,238,0.44)',
        top: E.mix('#c3ccd4', '#ffffff', 0.35)
      });
      E.faceRect(S.ctx, frame.L, 0.0, 0.0, 0.03, 1, '#77808a');
      E.faceRect(S.ctx, frame.L, 0.97, 0.0, 0.03, 1, '#77808a');
      E.faceRect(S.ctx, frame.L, 0.10, 0.18, 0.3, 0.4, E.mix('#e0a648', '#ffffff', 0.2));
      var c = E.isoc(f.x + 2.85, f.y + 0.5);
      S.ctx.fillStyle = '#77808a';
      S.ctx.fillRect(c.x - 1.5, c.y - 60, 3, 60);
      E.poly(S.ctx, [{ x: c.x - 9, y: c.y - 74 }, { x: c.x + 9, y: c.y - 70 },
                     { x: c.x + 9, y: c.y - 58 }, { x: c.x - 9, y: c.y - 62 }], '#e8e2d4');
    }

    function drawStall(f) {
      var top = E.box(S.ctx, {
        x: f.x, y: f.y, w: 1.7, d: 1.5, h: 34, color: '#b99a72',
        left: E.mix('#8c7250', S.scene.sun.shade, 0.24),
        right: E.mix('#c9a97e', S.scene.sun.warm, 0.22),
        top: '#d8c39c'
      });
      // produce on the counter
      for (var i = 0; i < 7; i++) {
        var n = E.hash(f.x * 3.1 + i, i * 2.7);
        var p2 = E.facePt(top.T, 0.15 + (i % 4) * 0.24, 0.2 + Math.floor(i / 4) * 0.42);
        S.ctx.fillStyle = ['#c8562f', '#5f8a3c', '#d3a234', '#8e4f7a'][i % 4];
        S.ctx.beginPath(); S.ctx.ellipse(p2.x, p2.y - 2, 5 + n * 3, 4 + n * 2, 0, 0, Math.PI * 2); S.ctx.fill();
      }
      // striped awning above
      var aw = f.awning || '#a8503f';
      var A = E.iso(f.x - 0.1, f.y - 0.1), B = E.iso(f.x + 1.8, f.y - 0.1),
          C = E.iso(f.x + 1.8, f.y + 1.6), D = E.iso(f.x - 0.1, f.y + 1.6);
      var lift = 62;
      E.poly(S.ctx, [{ x: A.x, y: A.y - lift }, { x: B.x, y: B.y - lift },
                     { x: C.x, y: C.y - lift }, { x: D.x, y: D.y - lift }], aw);
      E.poly(S.ctx, [{ x: D.x, y: D.y - lift }, { x: C.x, y: C.y - lift },
                     { x: C.x, y: C.y - lift + 7 }, { x: D.x, y: D.y - lift + 7 }], E.shade(aw, 0.72));
      [[f.x, f.y], [f.x + 1.7, f.y], [f.x, f.y + 1.5], [f.x + 1.7, f.y + 1.5]].forEach(function (o) {
        var pc = E.isoc(o[0], o[1]);
        S.ctx.fillStyle = '#6f6252';
        S.ctx.fillRect(pc.x - 1.5, pc.y - lift, 3, lift);
      });
    }

    // The newsstand. Other work is a rack of papers you stop at, which is why
    // there is no jobs button anywhere in this client.
    function drawNewsstand(f) {
      var body = E.box(S.ctx, {
        x: f.x, y: f.y, w: 1.5, d: 1.0, h: 40, color: '#6f7a6a',
        left: E.mix('#57614f', S.scene.sun.shade, 0.24),
        right: E.mix('#7f8a78', S.scene.sun.warm, 0.22),
        top: '#8d977f'
      });
      // papers on the slanted rack, facing the pavement
      for (var i = 0; i < 4; i++) {
        E.faceRect(S.ctx, body.L, 0.08 + i * 0.22, 0.18, 0.17, 0.46,
                   E.mix(i % 2 ? '#e6dcc6' : '#d8cdb4', S.scene.sky[0], 0.12));
        E.faceRect(S.ctx, body.L, 0.09 + i * 0.22, 0.24, 0.15, 0.05, '#8a8172');
        E.faceRect(S.ctx, body.L, 0.09 + i * 0.22, 0.34, 0.11, 0.03, '#a39a89');
      }
      // a canopy over the rack, and the post holding it up
      var A = E.iso(f.x - 0.1, f.y - 0.1), B = E.iso(f.x + 1.6, f.y - 0.1),
          C = E.iso(f.x + 1.6, f.y + 1.1), D = E.iso(f.x - 0.1, f.y + 1.1);
      var lift = 66;
      E.poly(S.ctx, [{ x: A.x, y: A.y - lift }, { x: B.x, y: B.y - lift },
                     { x: C.x, y: C.y - lift }, { x: D.x, y: D.y - lift }], '#4f5c4c');
      E.poly(S.ctx, [{ x: D.x, y: D.y - lift }, { x: C.x, y: C.y - lift },
                     { x: C.x, y: C.y - lift + 6 }, { x: D.x, y: D.y - lift + 6 }], '#3d483b');
      [[f.x, f.y], [f.x + 1.5, f.y], [f.x, f.y + 1.0], [f.x + 1.5, f.y + 1.0]].forEach(function (o) {
        var pc = E.isoc(o[0], o[1]);
        S.ctx.fillStyle = '#5c5648';
        S.ctx.fillRect(pc.x - 1.5, pc.y - lift, 3, lift);
      });
    }

    /* An aircraft on the apron. It is scenery and it is meant to be: you can
       see the thing that leaves and there is nothing on this map that pays for
       a seat on it, which is the economic ceiling stated in one object. */
    function drawPlane(f) {
      var s = f.small ? 0.62 : 1;
      var w = 7.2 * s, d = 1.5 * s, body = '#e9edf0';

      // wing, laid flat at cabin height and drawn before the fuselage
      E.slab(S.ctx, f.x + 1.9 * s, f.y - 2.6 * s, 1.7 * s, 6.4 * s, 13 * s,
             E.mix(body, S.scene.sun.shade, 0.14));

      var fus = E.box(S.ctx, {
        x: f.x, y: f.y, w: w, d: d, h: 17 * s, color: body,
        left: E.mix(E.shade(body, 0.84), S.scene.sun.shade, 0.2),
        right: E.mix(body, S.scene.sun.warm, 0.18),
        top: '#ffffff'
      });
      for (var i = 0; i < 9; i++) {
        E.faceRect(S.ctx, fus.L, 0.1 + i * 0.085, 0.3, 0.042, 0.24, '#5b7186');
      }
      E.faceRect(S.ctx, fus.L, 0.02, 0.24, 0.05, 0.3, E.mix('#5b7186', '#ffffff', 0.35));

      // tail fin, over the rear of the fuselage
      var t0 = E.iso(f.x + 0.35 * s, f.y + 0.7 * s);
      E.poly(S.ctx, [
        { x: t0.x, y: t0.y - 16 * s }, { x: t0.x + 15 * s, y: t0.y - 42 * s },
        { x: t0.x + 25 * s, y: t0.y - 42 * s }, { x: t0.x + 21 * s, y: t0.y - 16 * s }
      ], '#b9563f');

      // engine slung under the near wing
      var e0 = E.isoc(f.x + 2.9 * s, f.y + 2.4 * s);
      S.ctx.fillStyle = '#aeb6bd';
      S.ctx.beginPath();
      S.ctx.ellipse(e0.x, e0.y - 11 * s, 9 * s, 4.5 * s, 0, 0, Math.PI * 2);
      S.ctx.fill();
    }

    // The computer on the desk. Same job as the newsstand, indoors.
    function drawScreen(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#4a423a';
      S.ctx.fillRect(c.x - 2, c.y - 34, 4, 6);
      E.poly(S.ctx, [{ x: c.x - 14, y: c.y - 32 }, { x: c.x + 14, y: c.y - 36 },
                     { x: c.x + 14, y: c.y - 55 }, { x: c.x - 14, y: c.y - 51 }], '#3b3630');
      var glow = E.mix('#7fa3bd', S.scene.sun.warm, 0.18);
      E.poly(S.ctx, [{ x: c.x - 11, y: c.y - 34 }, { x: c.x + 11, y: c.y - 37 },
                     { x: c.x + 11, y: c.y - 53 }, { x: c.x - 11, y: c.y - 50 }], glow);
      S.emissive.push({
        pts: [{ x: c.x - 11, y: c.y - 34 }, { x: c.x + 11, y: c.y - 37 },
              { x: c.x + 11, y: c.y - 53 }, { x: c.x - 11, y: c.y - 50 }],
        color: '#cfe0ee', a: 0.16, glow: 0.4
      });
    }

    function drawFountain(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.save();
      S.ctx.translate(c.x, c.y);
      E.poly(S.ctx, [{ x: -46, y: 0 }, { x: 0, y: -23 }, { x: 46, y: 0 }, { x: 0, y: 23 }], '#c8bda6');
      E.poly(S.ctx, [{ x: -38, y: 0 }, { x: 0, y: -19 }, { x: 38, y: 0 }, { x: 0, y: 19 }], '#5f9dc0');
      var r = 0.5 + 0.5 * Math.sin(S.t * 1.4);
      E.poly(S.ctx, [{ x: -24, y: 0 }, { x: 0, y: -12 }, { x: 24, y: 0 }, { x: 0, y: 12 }],
             E.rgba('#ffffff', 0.10 + r * 0.12));
      S.ctx.fillStyle = '#c8bda6';
      S.ctx.fillRect(-4, -34, 8, 34);
      S.ctx.beginPath(); S.ctx.ellipse(0, -36, 11, 5, 0, 0, Math.PI * 2); S.ctx.fill();
      // a thin plume that never quite repeats
      S.ctx.strokeStyle = E.rgba('#ffffff', 0.5);
      S.ctx.lineWidth = 2;
      for (var i = 0; i < 5; i++) {
        var a = (i / 5) * Math.PI * 2 + S.t * 0.6;
        S.ctx.beginPath();
        S.ctx.moveTo(0, -40);
        S.ctx.quadraticCurveTo(Math.cos(a) * 9, -58 - r * 5, Math.cos(a) * 20, -36);
        S.ctx.stroke();
      }
      S.ctx.restore();
    }

    function drawBoat(f) {
      var c = E.isoc(f.x, f.y);
      var bob = Math.sin(S.t * 0.9 + f.x) * 2;
      S.ctx.save();
      S.ctx.translate(c.x, c.y + bob);
      E.poly(S.ctx, [{ x: -40, y: 0 }, { x: -22, y: -13 }, { x: 30, y: -6 }, { x: 40, y: 4 },
                     { x: 24, y: 12 }, { x: -30, y: 8 }], '#e3dbc9');
      E.poly(S.ctx, [{ x: -30, y: 8 }, { x: 24, y: 12 }, { x: 24, y: 17 }, { x: -30, y: 13 }], '#8f8674');
      E.poly(S.ctx, [{ x: -14, y: -6 }, { x: 6, y: -3 }, { x: 6, y: -17 }, { x: -14, y: -20 }], '#7fa3bd');
      S.ctx.strokeStyle = '#6f6252'; S.ctx.lineWidth = 2;
      S.ctx.beginPath(); S.ctx.moveTo(2, -14); S.ctx.lineTo(6, -56); S.ctx.stroke();
      S.ctx.restore();
    }

    function drawBollard(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#6f7681';
      S.ctx.fillRect(c.x - 2.5, c.y - 14, 5, 14);
      S.ctx.fillStyle = '#98a0aa';
      S.ctx.beginPath(); S.ctx.ellipse(c.x, c.y - 14, 2.8, 1.6, 0, 0, Math.PI * 2); S.ctx.fill();
    }

    function drawDeskLamp(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.strokeStyle = '#3d3630'; S.ctx.lineWidth = 3;
      S.ctx.beginPath(); S.ctx.moveTo(c.x, c.y - 30); S.ctx.quadraticCurveTo(c.x + 2, c.y - 48, c.x + 14, c.y - 50); S.ctx.stroke();
      E.poly(S.ctx, [{ x: c.x + 6, y: c.y - 52 }, { x: c.x + 22, y: c.y - 52 },
                     { x: c.x + 18, y: c.y - 42 }, { x: c.x + 10, y: c.y - 42 }], '#4a4038');
      S.emissive.push({ ellipse: { x: c.x + 14, y: c.y - 43 }, r: 5, color: '#ffd79a', a: 0.9, glow: 0.35 });
    }

    function drawFloorLamp(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#4a423a';
      S.ctx.fillRect(c.x - 1.5, c.y - 70, 3, 70);
      S.ctx.beginPath(); S.ctx.ellipse(c.x, c.y, 8, 3.5, 0, 0, Math.PI * 2); S.ctx.fill();
      E.poly(S.ctx, [{ x: c.x - 13, y: c.y - 88 }, { x: c.x + 13, y: c.y - 88 },
                     { x: c.x + 9, y: c.y - 68 }, { x: c.x - 9, y: c.y - 68 }], '#d9c9ae');
      // Switched on after dark — with the sunbeams out, this and the desk
      // lamp are what the room sees by.
      if (S.scene.night) {
        S.emissive.push({ ellipse: { x: c.x, y: c.y - 78 }, r: 9, color: '#ffd79a', a: 0.75, glow: 1.2 });
      }
    }

    function drawBoard(f) {
      var a = E.iso(f.x, f.y), b = E.iso(f.x + 1.7, f.y);
      var top = 86, h = 40;
      E.poly(S.ctx, [{ x: a.x, y: a.y - top }, { x: b.x, y: b.y - top },
                     { x: b.x, y: b.y - top + h }, { x: a.x, y: a.y - top + h }], '#e6dcc6');
      for (var i = 0; i < 4; i++) {
        var u = 0.08 + i * 0.23;
        var p1 = { x: a.x + (b.x - a.x) * u, y: a.y + (b.y - a.y) * u - top + 8 };
        S.ctx.fillStyle = i < 2 ? '#d99a3f' : '#6f97c4';
        S.ctx.fillRect(p1.x, p1.y, 10, 22);
      }
    }

    function drawMug(f) {
      var c = E.isoc(f.x, f.y);
      S.ctx.fillStyle = '#e2d6bf';
      S.ctx.fillRect(c.x - 3, c.y - 36, 6, 7);
    }

    /* ------------------------------------------------------- the washroom

       Three fittings, drawn at the scale the rest of the furniture is: a
       tray under a curtain, a basin on a pedestal, a pan and its cistern.
       Porcelain is a tone off the walls, so the room reads as tile.

       `f.x, f.y` is the back-left corner of the footprint, and every part
       is placed off the projection of a point inside it. */

    var PORCELAIN = '#e7e3db';
    var CHROME = '#b0aa9e';

    function drawShower(f) {
      var w = 1.4;
      E.box(S.ctx, { x: f.x, y: f.y, w: w, d: w, h: 6, color: '#dcd8d0',
                     left: '#c9c4bb', right: '#d3cec5', top: '#eae7e0' });

      // riser and head, against the wall the tray backs onto
      var back = E.iso(f.x + w / 2, f.y + 0.15);
      S.ctx.fillStyle = CHROME;
      S.ctx.fillRect(back.x - 2, back.y - 96, 4, 90);
      E.poly(S.ctx, [{ x: back.x - 11, y: back.y - 96 }, { x: back.x + 11, y: back.y - 96 },
                     { x: back.x + 6, y: back.y - 87 }, { x: back.x - 6, y: back.y - 87 }], '#cfc9bc');

      // the rail across the open side, and the curtain gathered at its end
      var a = E.iso(f.x, f.y + w), b = E.iso(f.x + w, f.y + w);
      S.ctx.strokeStyle = CHROME;
      S.ctx.lineWidth = 2.5;
      S.ctx.beginPath();
      S.ctx.moveTo(a.x, a.y - 100); S.ctx.lineTo(b.x, b.y - 100); S.ctx.stroke();
      S.ctx.fillStyle = '#bcc8c8';
      S.ctx.fillRect(b.x - 15, b.y - 100, 30, 92);
      S.ctx.fillStyle = E.rgba('#ffffff', 0.3);
      S.ctx.fillRect(b.x - 15, b.y - 100, 12, 92);
      S.ctx.fillStyle = E.rgba('#6f8489', 0.26);
      for (var i = 0; i < 5; i++) S.ctx.fillRect(b.x - 12 + i * 6, b.y - 100, 2.2, 92);
      S.ctx.fillStyle = E.rgba('#6f8489', 0.35);
      S.ctx.fillRect(b.x - 15, b.y - 11, 30, 3);
    }

    function drawBasin(f) {
      var c = E.iso(f.x + 0.5, f.y + 0.31);
      S.ctx.fillStyle = '#d5d1c9';
      S.ctx.fillRect(c.x - 7, c.y - 33, 14, 33);
      E.slab(S.ctx, f.x, f.y, 1, 0.62, 33, PORCELAIN);
      S.ctx.fillStyle = '#cbc6bd';
      S.ctx.beginPath(); S.ctx.ellipse(c.x, c.y - 34, 17, 8, 0, 0, Math.PI * 2); S.ctx.fill();
      // the tap, and the mirror over it
      S.ctx.fillStyle = CHROME;
      S.ctx.fillRect(c.x - 1.5, c.y - 50, 3, 14);
      S.ctx.fillRect(c.x - 1.5, c.y - 51, 9, 2.5);
      E.poly(S.ctx, [{ x: c.x - 15, y: c.y - 98 }, { x: c.x + 15, y: c.y - 90 },
                     { x: c.x + 15, y: c.y - 62 }, { x: c.x - 15, y: c.y - 70 }], '#9fb0b6');
    }

    function drawToilet(f) {
      E.box(S.ctx, { x: f.x, y: f.y + 0.08, w: 0.34, d: 0.84, h: 34,
                     color: '#ded9d1', top: '#efece5' });
      E.box(S.ctx, { x: f.x + 0.34, y: f.y + 0.2, w: 0.6, d: 0.6, h: 17,
                     color: PORCELAIN, top: '#f1eee7' });
      var seat = E.iso(f.x + 0.64, f.y + 0.5);
      S.ctx.fillStyle = '#cbc6bd';
      S.ctx.beginPath();
      S.ctx.ellipse(seat.x, seat.y - 17, 15, 7.5, 0, 0, Math.PI * 2);
      S.ctx.fill();
    }

    /* -------------------------------------------------------- emissive */

    function drawEmissive() {
      for (var i = 0; i < S.emissive.length; i++) {
        var e = S.emissive[i];
        if (e.pts) E.poly(S.ctx, e.pts, E.rgba(e.color, e.a));
        else if (e.ellipse) {
          S.ctx.fillStyle = E.rgba(e.color, e.a);
          S.ctx.beginPath();
          S.ctx.ellipse(e.ellipse.x, e.ellipse.y, e.r, e.r * 0.7, 0, 0, Math.PI * 2);
          S.ctx.fill();
        }
      }
      if (S.quality !== 'high') return;
      S.ctx.save();
      S.ctx.globalCompositeOperation = 'lighter';
      for (var j = 0; j < S.emissive.length; j++) {
        var q = S.emissive[j];
        if (!q.glow) continue;
        var cx, cy, rad;
        if (q.pts) {
          cx = (q.pts[0].x + q.pts[2].x) / 2; cy = (q.pts[0].y + q.pts[2].y) / 2;
          rad = Math.max(16, Math.hypot(q.pts[2].x - q.pts[0].x, q.pts[2].y - q.pts[0].y) * 1.5);
        } else {
          cx = q.ellipse.x; cy = q.ellipse.y; rad = q.r * 6;
        }
        var g = S.ctx.createRadialGradient(cx, cy, 0, cx, cy, rad);
        g.addColorStop(0, E.rgba(q.color, q.glow * 0.32));
        g.addColorStop(1, E.rgba(q.color, 0));
        S.ctx.fillStyle = g;
        S.ctx.fillRect(cx - rad, cy - rad, rad * 2, rad * 2);
      }
      S.ctx.restore();
    }

    // warm haze from the sun side, cool bounce in the near corner
    function sunWash() {
      S.ctx.save();
      S.ctx.globalCompositeOperation = 'lighter';
      var g = S.ctx.createLinearGradient(S.W, 0, S.W * 0.15, S.H);
      g.addColorStop(0, 'rgba(255,224,170,0.16)');
      g.addColorStop(0.5, 'rgba(255,224,170,0.04)');
      g.addColorStop(1, 'rgba(255,224,170,0)');
      S.ctx.fillStyle = g;
      S.ctx.fillRect(0, 0, S.W, S.H);
      S.ctx.restore();
    }

    return {
      drawSky: drawSky,
      drawGround: drawGround,
      drawShadows: drawShadows,
      drawObjects: drawObjects,
      drawEmissive: drawEmissive,
      sunWash: sunWash,
      drawFixture: drawFixture,
      drawBuilding: drawBuilding,
      onScreen: onScreen,
      offscreen: offscreen,
    };
  };

})(window.ETM);
