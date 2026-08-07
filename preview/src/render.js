/* Escape the Matrix — isometric renderer.
   Plain Canvas2D, no dependencies, runs off the filesystem. Stands in for the
   PixiJS/WebGL client described in technical.md §10: same fixed 3/4 camera, same
   painted-ground-plus-logic-layer construction, same two-tier lighting model. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  var TW = 64, TH = 32;          // tile diamond width / height
  E.TW = TW; E.TH = TH;

  /* ---------------------------------------------------------- projection */

  E.iso = function (x, y) {
    return { x: (x - y) * (TW / 2), y: (x + y) * (TH / 2) };
  };

  // centre of the tile at grid (x, y)
  E.isoc = function (x, y) {
    var p = E.iso(x, y);
    return { x: p.x, y: p.y + TH / 2 };
  };

  /* ------------------------------------------------------------- colour */

  function clamp(v, a, b) { return v < a ? a : v > b ? b : v; }
  E.clamp = clamp;

  function rgb(hex) {
    var h = hex.replace('#', '');
    if (h.length === 3) h = h[0] + h[0] + h[1] + h[1] + h[2] + h[2];
    var n = parseInt(h, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  E.rgb = rgb;

  function hex(c) {
    return '#' + c.map(function (v) {
      return clamp(Math.round(v), 0, 255).toString(16).padStart(2, '0');
    }).join('');
  }

  E.shade = function (color, f) {
    var c = rgb(color);
    return hex([c[0] * f, c[1] * f, c[2] * f]);
  };

  E.mix = function (a, b, t) {
    var ca = rgb(a), cb = rgb(b);
    return hex([
      ca[0] + (cb[0] - ca[0]) * t,
      ca[1] + (cb[1] - ca[1]) * t,
      ca[2] + (cb[2] - ca[2]) * t
    ]);
  };

  E.rgba = function (color, a) {
    var c = rgb(color);
    return 'rgba(' + c[0] + ',' + c[1] + ',' + c[2] + ',' + a + ')';
  };

  /* --------------------------------------------------- deterministic noise */

  E.hash = function (x, y) {
    var n = Math.sin(x * 127.1 + y * 311.7) * 43758.5453;
    return n - Math.floor(n);
  };

  /* -------------------------------------------------------- primitives */

  function poly(ctx, pts, fill, stroke) {
    ctx.beginPath();
    ctx.moveTo(pts[0].x, pts[0].y);
    for (var i = 1; i < pts.length; i++) ctx.lineTo(pts[i].x, pts[i].y);
    ctx.closePath();
    if (fill) { ctx.fillStyle = fill; ctx.fill(); }
    if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = 1; ctx.stroke(); }
  }
  E.poly = poly;

  // one ground tile
  E.tile = function (ctx, x, y, fill, stroke) {
    var p = E.iso(x, y);
    poly(ctx, [
      { x: p.x, y: p.y },
      { x: p.x + TW / 2, y: p.y + TH / 2 },
      { x: p.x, y: p.y + TH },
      { x: p.x - TW / 2, y: p.y + TH / 2 }
    ], fill, stroke);
  };

  // a flat quad covering a rectangular footprint, raised by h
  E.slab = function (ctx, x, y, w, d, h, fill) {
    var A = E.iso(x, y), B = E.iso(x + w, y), C = E.iso(x + w, y + d), D = E.iso(x, y + d);
    poly(ctx, [
      { x: A.x, y: A.y - h }, { x: B.x, y: B.y - h },
      { x: C.x, y: C.y - h }, { x: D.x, y: D.y - h }
    ], fill);
  };

  /* ------------------------------------------------------------- boxes */

  // Draws an axis-aligned iso box. Returns its visible faces so callers can
  // paste windows, doors and signage onto them in face-local coordinates.
  E.box = function (ctx, o) {
    var x = o.x, y = o.y, w = o.w, d = o.d, h = o.h;
    var A = E.iso(x, y), B = E.iso(x + w, y), C = E.iso(x + w, y + d), D = E.iso(x, y + d);
    var At = { x: A.x, y: A.y - h }, Bt = { x: B.x, y: B.y - h },
        Ct = { x: C.x, y: C.y - h }, Dt = { x: D.x, y: D.y - h };

    var base = o.color || '#2a221c';
    var faceL = o.left  || E.shade(base, 0.74);   // wall facing camera-left
    var faceR = o.right || E.shade(base, 0.52);   // wall facing camera-right
    var faceT = o.top   || E.shade(base, 1.06);

    var L = [Dt, Ct, C, D];
    var R = [Bt, Ct, C, B];

    poly(ctx, L, faceL);
    poly(ctx, R, faceR);
    if (h > 0) poly(ctx, [At, Bt, Ct, Dt], faceT);

    // soft edge where the two walls meet — reads as a corner highlight
    if (h > 6) {
      ctx.beginPath();
      ctx.moveTo(Ct.x, Ct.y); ctx.lineTo(C.x, C.y);
      ctx.strokeStyle = E.rgba('#f0a742', 0.06);
      ctx.lineWidth = 2; ctx.stroke();
    }
    return { L: L, R: R, T: [At, Bt, Ct, Dt] };
  };

  // bilinear point inside a face quad given as [TL, TR, BR, BL]
  function facePt(q, u, v) {
    var tx = q[0].x + (q[1].x - q[0].x) * u, ty = q[0].y + (q[1].y - q[0].y) * u;
    var bx = q[3].x + (q[2].x - q[3].x) * u, by = q[3].y + (q[2].y - q[3].y) * u;
    return { x: tx + (bx - tx) * v, y: ty + (by - ty) * v };
  }
  E.facePt = facePt;

  // a rectangle painted onto a face, sheared to match it
  E.faceRect = function (ctx, q, u, v, uw, vh, fill) {
    poly(ctx, [
      facePt(q, u, v), facePt(q, u + uw, v),
      facePt(q, u + uw, v + vh), facePt(q, u, v + vh)
    ], fill);
  };

  /* ---------------------------------------------------------- fixtures */

  E.lampPost = function (ctx, x, y, o) {
    o = o || {};
    var c = E.isoc(x, y);
    var h = o.h || 76;
    var post = o.color || '#4c5158';
    ctx.fillStyle = post;
    ctx.fillRect(c.x - 2, c.y - h, 4, h);
    ctx.beginPath();
    ctx.ellipse(c.x, c.y, 7, 3.5, 0, 0, Math.PI * 2);
    ctx.fill();
    // arm
    ctx.beginPath();
    ctx.moveTo(c.x, c.y - h);
    ctx.quadraticCurveTo(c.x + 4, c.y - h - 11, c.x + 17, c.y - h - 11);
    ctx.strokeStyle = post; ctx.lineWidth = 4; ctx.stroke();
    // head
    var lit = o.lit === undefined ? true : o.lit;
    ctx.beginPath();
    ctx.ellipse(c.x + 19, c.y - h - 9, 6.5, 4, 0, 0, Math.PI * 2);
    ctx.fillStyle = lit ? '#ffd699' : '#c8cdd2';
    ctx.fill();
    return { x: c.x + 19, y: c.y - h - 9 };
  };

  E.bench = function (ctx, x, y, o) {
    o = o || {};
    var c = E.isoc(x, y);
    ctx.save();
    ctx.translate(c.x, c.y);
    poly(ctx, [{ x: -24, y: -8 }, { x: 0, y: 4 }, { x: 24, y: -8 }, { x: 0, y: -20 }], '#a4763f');
    poly(ctx, [{ x: -24, y: -8 }, { x: 0, y: 4 }, { x: 0, y: 0 }, { x: -24, y: -12 }], '#6d4c28');
    poly(ctx, [{ x: 4, y: -22 }, { x: 22, y: -31 }, { x: 22, y: -44 }, { x: 4, y: -35 }], '#b98a4e');
    ctx.restore();
  };

  E.planter = function (ctx, x, y, o) {
    o = o || {};
    var c = E.isoc(x, y);
    E.box(ctx, { x: x, y: y, w: 0.9, d: 0.9, h: 14, color: '#9c8368' });
    ctx.save();
    ctx.translate(c.x, c.y - 14);
    var g = o.dry ? '#8a8a48' : '#568a4c';
    for (var i = 0; i < 7; i++) {
      var a = (i / 7) * Math.PI * 2;
      ctx.beginPath();
      ctx.ellipse(Math.cos(a) * 9, Math.sin(a) * 5 - 8, 8, 6, a, 0, Math.PI * 2);
      ctx.fillStyle = i % 2 ? g : E.shade(g, 0.72);
      ctx.fill();
    }
    ctx.restore();
  };

  E.railing = function (ctx, x, y, len, axis, color) {
    color = color || '#161210';
    for (var i = 0; i <= len; i++) {
      var gx = axis === 'x' ? x + i : x;
      var gy = axis === 'x' ? y : y + i;
      var c = E.isoc(gx, gy);
      ctx.fillStyle = color;
      ctx.fillRect(c.x - 1.5, c.y - 26, 3, 26);
    }
    var a = E.isoc(x, y), b = E.isoc(axis === 'x' ? x + len : x, axis === 'x' ? y : y + len);
    ctx.strokeStyle = color; ctx.lineWidth = 3;
    ctx.beginPath(); ctx.moveTo(a.x, a.y - 24); ctx.lineTo(b.x, b.y - 24); ctx.stroke();
    ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(a.x, a.y - 13); ctx.lineTo(b.x, b.y - 13); ctx.stroke();
  };

  E.rug = function (ctx, x, y, w, d, color, edge) {
    var A = E.iso(x, y), B = E.iso(x + w, y), C = E.iso(x + w, y + d), D = E.iso(x, y + d);
    poly(ctx, [A, B, C, D], color);
    poly(ctx, [
      { x: A.x, y: A.y + 5 }, { x: B.x - 10, y: B.y }, { x: C.x, y: C.y - 5 }, { x: D.x + 10, y: D.y }
    ], edge || E.shade(color, 0.8));
  };

  /* ------------------------------------------------------------ actors */

  /* The build a figure is drawn on. Silhouette is the only thing that reads at
     this size, so it is the only thing that carries who somebody is: a
     labourer's shoulders, a robe that reaches the floor, a torso with no shirt
     on it. `default` is everybody the world has not written down. */
  var BUILD = {
    default: { hip: 8, shoulder: 9, arm: 5, leg: 6 },
    heavy: { hip: 9.4, shoulder: 13.4, arm: 6.6, leg: 7.4 },  // trained, and it shows
    robe: { hip: 13.5, shoulder: 9.5, arm: 4.6, leg: 6 },     // a gown, hem on the ground
  };

  // A character. Deliberately a silhouette with a rim light: the art bible asks
  // for readable proportions and specificity from lighting, not from detail.
  E.actor = function (ctx, a, t) {
    var c = E.isoc(a.x, a.y);
    var moving = a.moving;
    var phase = a.phase || 0;
    var step = moving ? Math.sin(t * 9 + phase) : 0;
    var bob = moving ? Math.abs(Math.sin(t * 9 + phase)) * 2.2 : Math.sin(t * 1.6 + phase) * 0.9;
    var s = a.scale || 1;

    ctx.save();
    ctx.translate(c.x, c.y);
    ctx.scale(s, s);

    var body = a.color || '#3f4150';
    var rim = a.rim || '#fff0cf';
    var skin = a.skin || '#c39468';
    var B = BUILD[a.build] || BUILD.default;
    // A shirt is what covers the torso; sleeves are what cover the arms. A vest
    // is the second without the first, which is a different person entirely.
    var torsoFill = a.shirtless ? skin : body;
    var limb = (a.shirtless || a.sleeves === 'bare') ? skin : body;
    var y0 = -bob;

    // Who the pointer is over. Painted under the feet rather than around the
    // outline: a halo on a silhouette reads as selection in a strategy game,
    // and nobody here is a unit you have selected.
    if (a.hover) {
      ctx.strokeStyle = E.rgba(rim, 0.8);
      ctx.lineWidth = 1.6;
      ctx.beginPath();
      ctx.ellipse(0, 0, 13, 6.4, 0, 0, Math.PI * 2);
      ctx.stroke();
    }

    // legs — a gown has none to draw, because the hem is on the floor
    if (a.build !== 'robe') {
      ctx.strokeStyle = body; ctx.lineWidth = B.leg; ctx.lineCap = 'round';
      ctx.beginPath();
      ctx.moveTo(0, y0 - 22); ctx.lineTo(step * 6, y0 - 1);
      ctx.moveTo(0, y0 - 22); ctx.lineTo(-step * 6, y0 - 1);
      ctx.stroke();
    }

    // torso
    ctx.beginPath();
    if (a.build === 'robe') {
      // shoulders down to a hem that swings with the walk
      ctx.moveTo(-B.shoulder, y0 - 46);
      ctx.quadraticCurveTo(0, y0 - 50, B.shoulder, y0 - 46);
      ctx.lineTo(B.hip + step * 1.6, y0 - 1);
      ctx.lineTo(-B.hip + step * 1.6, y0 - 1);
    } else {
      ctx.moveTo(-B.hip, y0 - 22);
      ctx.quadraticCurveTo(-B.shoulder, y0 - 46, 0, y0 - 48);
      ctx.quadraticCurveTo(B.shoulder, y0 - 46, B.hip, y0 - 22);
    }
    ctx.closePath();
    ctx.fillStyle = torsoFill; ctx.fill();

    // A bare chest is one plane at this size, so the only thing that makes it
    // a chest rather than a flesh-coloured coat is where the light stops.
    if (a.shirtless) {
      ctx.strokeStyle = E.rgba(E.shade(skin, 0.74), 0.9);
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      ctx.moveTo(-4.4, y0 - 40); ctx.lineTo(4.4, y0 - 40);
      ctx.moveTo(0, y0 - 40); ctx.lineTo(0, y0 - 27);
      ctx.stroke();
    }

    // arms
    ctx.strokeStyle = limb;
    ctx.lineWidth = B.arm;
    var shoulder = B.hip - 1;
    ctx.beginPath();
    if (a.gesture) {
      var g = Math.sin(t * 3.4 + phase);
      ctx.moveTo(shoulder, y0 - 43); ctx.lineTo(shoulder + 8 + g * 5, y0 - 50 - g * 7);
      ctx.moveTo(-shoulder, y0 - 43); ctx.lineTo(-shoulder - 6, y0 - 28);
    } else if (a.arms === 'out') {
      // held away from the body, which is what carrying that much of it does
      ctx.moveTo(shoulder, y0 - 43); ctx.lineTo(shoulder + 5 - step * 3, y0 - 25);
      ctx.moveTo(-shoulder, y0 - 43); ctx.lineTo(-shoulder - 5 + step * 3, y0 - 25);
    } else {
      ctx.moveTo(shoulder, y0 - 43); ctx.lineTo(shoulder + 1 - step * 4, y0 - 25);
      ctx.moveTo(-shoulder, y0 - 43); ctx.lineTo(-shoulder - 1 + step * 4, y0 - 25);
    }
    ctx.stroke();

    // What the player owns, worn. At this size a strap and a brim read from
    // across the street where a face never will, which is the whole reason
    // the closet offers silhouette rather than detail (§12).
    if (a.bag) {
      ctx.strokeStyle = E.shade(a.bag, 0.72);
      ctx.lineWidth = 2.6;
      ctx.beginPath();
      ctx.moveTo(-6, y0 - 46); ctx.lineTo(7, y0 - 28);
      ctx.stroke();
      ctx.fillStyle = a.bag;
      ctx.beginPath();
      ctx.ellipse(10, y0 - 25, 6, 5.4, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = E.rgba('#ffffff', 0.12);
      ctx.beginPath();
      ctx.ellipse(11, y0 - 27, 3.4, 2.4, 0, 0, Math.PI * 2);
      ctx.fill();
    }

    // head
    ctx.beginPath();
    ctx.arc(0, y0 - 56, 8, 0, Math.PI * 2);
    ctx.fillStyle = skin; ctx.fill();
    ctx.beginPath();
    ctx.arc(0, y0 - 58, 8, Math.PI * 0.95, Math.PI * 2.15);
    ctx.fillStyle = a.hair || E.shade(skin, 0.42); ctx.fill();

    if (a.hat) {
      ctx.fillStyle = a.hat;
      ctx.beginPath();
      ctx.arc(0, y0 - 59, 8.4, Math.PI * 0.94, Math.PI * 2.06);
      ctx.fill();
      ctx.beginPath();
      ctx.ellipse(2, y0 - 58.5, 11.5, 2.4, 0, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = E.rgba(rim, 0.22);
      ctx.beginPath();
      ctx.ellipse(3, y0 - 60, 5.5, 1.6, 0, 0, Math.PI * 2);
      ctx.fill();
    }
    // the shaded side of the figure, away from the sun
    ctx.save();
    ctx.beginPath();
    ctx.rect(-B.hip - 8, y0 - 70, B.hip + 8, 72);
    ctx.clip();
    ctx.beginPath();
    if (a.build === 'robe') {
      ctx.moveTo(-B.shoulder, y0 - 46);
      ctx.quadraticCurveTo(0, y0 - 50, 0, y0 - 48);
      ctx.lineTo(0, y0 - 1);
      ctx.lineTo(-B.hip + step * 1.6, y0 - 1);
    } else {
      ctx.moveTo(-B.hip, y0 - 22);
      ctx.quadraticCurveTo(-B.shoulder, y0 - 46, 0, y0 - 48);
      ctx.lineTo(0, y0 - 22);
    }
    ctx.closePath();
    ctx.fillStyle = E.rgba(E.shade(torsoFill, 0.72), 0.55); ctx.fill();
    ctx.restore();

    // rim light down the lit side
    ctx.strokeStyle = E.rgba(rim, a.rimAlpha === undefined ? 0.8 : a.rimAlpha);
    ctx.lineWidth = 1.8; ctx.lineCap = 'round';
    var side = a.rimSide === 'left' ? -1 : 1;
    ctx.beginPath();
    if (a.build === 'robe') {
      ctx.moveTo(side * (B.hip + step * 1.6), y0 - 1);
      ctx.lineTo(side * B.shoulder, y0 - 46);
      ctx.quadraticCurveTo(side * 4, y0 - 50, 0, y0 - 49);
    } else {
      ctx.moveTo(side * B.hip, y0 - 23);
      ctx.quadraticCurveTo(side * B.shoulder, y0 - 46, 0, y0 - 48);
    }
    ctx.stroke();
    ctx.beginPath();
    ctx.arc(0, y0 - 56, 8, side > 0 ? -1.5 : 1.7, side > 0 ? 0.5 : 3.6);
    ctx.stroke();

    ctx.restore();
  };

  /* What a figure is made of, as opposed to where it happens to be standing. */
  var FIGURE = ['color', 'hair', 'hat', 'bag', 'skin', 'build', 'sleeves',
                'shirtless', 'arms', 'gesture', 'moving'];

  /* You, on your own, at a size where the silhouette is the whole point.
     The closet's mirror calls this every frame off the same `look` the street
     reads, and it is deliberately E.actor rather than a second, prettier
     drawing of the player: a coat that looks right here and wrong from across
     the road would be a lie the closet told you.

     `w`/`h` are CSS pixels; the caller owns the device-pixel transform.
     Returns where the figure landed so the caller can hang a nameplate over
     its head — the mirror has no idea how tall you are until it has drawn you. */
  E.mannequin = function (ctx, w, h, a, t) {
    ctx.clearRect(0, 0, w, h);

    // A figure is ~70 units tall from sole to crown, and wants air around it.
    var k = clamp(Math.min(h / 104, w / 62), 1, 4.2);
    var cx = Math.round(w / 2), cy = Math.round(h * 0.8);

    // the pool the figure is standing in: one lamp, on the lit side the actor
    // itself assumes, so the mirror is the same light as the street
    var pool = ctx.createRadialGradient(cx + 8 * k, cy - 6 * k, 2, cx, cy, 46 * k);
    pool.addColorStop(0, E.rgba('#f0a742', 0.13));
    pool.addColorStop(1, E.rgba('#f0a742', 0));
    ctx.fillStyle = pool;
    ctx.beginPath();
    ctx.ellipse(cx, cy, 46 * k, 17 * k, 0, 0, Math.PI * 2);
    ctx.fill();

    // contact shadow — without it the figure floats
    ctx.fillStyle = E.rgba('#2a1f14', 0.3);
    ctx.beginPath();
    ctx.ellipse(cx - 2 * k, cy + 0.5 * k, 15 * k, 4.6 * k, 0, 0, Math.PI * 2);
    ctx.fill();

    // Everything that says who somebody is, and nothing that says where they
    // are: the world's position, path and scale are left behind at the door.
    var who = { x: 0, y: 0, scale: k, phase: 0, rimAlpha: 0.85 };
    FIGURE.forEach(function (f) { if (a[f] !== undefined) who[f] = a[f]; });

    // E.actor plants its feet at the centre of tile (0,0), which sits TH/2
    // below the origin it is handed.
    ctx.save();
    ctx.translate(cx, cy - TH / 2);
    E.actor(ctx, who, t);
    ctx.restore();

    return { x: cx, y: cy, k: k, top: cy - 66 * k };
  };

  /* --------------------------------------------------------- particles */

  E.Particles = function (kind, n, bounds) {
    this.kind = kind;
    this.bounds = bounds;
    this.p = [];
    for (var i = 0; i < n; i++) this.p.push(this.spawn(true));
  };

  E.Particles.prototype.spawn = function (initial) {
    var b = this.bounds;
    if (this.kind === 'rain') {
      return {
        x: b.x + Math.random() * b.w,
        y: initial ? b.y + Math.random() * b.h : b.y - 20,
        v: 340 + Math.random() * 260,
        len: 8 + Math.random() * 12,
        a: 0.14 + Math.random() * 0.26
      };
    }
    return {
      x: b.x + Math.random() * b.w,
      y: b.y + Math.random() * b.h,
      vx: (Math.random() - 0.5) * 9,
      vy: -3 - Math.random() * 7,
      r: 0.6 + Math.random() * 1.5,
      a: 0.05 + Math.random() * 0.18,
      ph: Math.random() * 6.28
    };
  };

  E.Particles.prototype.update = function (dt, t) {
    var b = this.bounds;
    for (var i = 0; i < this.p.length; i++) {
      var q = this.p[i];
      if (this.kind === 'rain') {
        q.y += q.v * dt;
        q.x += q.v * dt * 0.16;
        if (q.y > b.y + b.h) this.p[i] = this.spawn(false);
      } else {
        q.x += (q.vx + Math.sin(t * 0.7 + q.ph) * 4) * dt;
        q.y += q.vy * dt;
        if (q.y < b.y) { q.y = b.y + b.h; q.x = b.x + Math.random() * b.w; }
      }
    }
  };

  E.Particles.prototype.draw = function (ctx, color) {
    ctx.save();
    if (this.kind === 'rain') {
      ctx.strokeStyle = color || '#9fb6d8';
      ctx.lineWidth = 1;
      for (var i = 0; i < this.p.length; i++) {
        var q = this.p[i];
        ctx.globalAlpha = q.a;
        ctx.beginPath();
        ctx.moveTo(q.x, q.y);
        ctx.lineTo(q.x - q.len * 0.16, q.y + q.len);
        ctx.stroke();
      }
    } else {
      ctx.fillStyle = color || '#f0a742';
      for (var j = 0; j < this.p.length; j++) {
        var d = this.p[j];
        ctx.globalAlpha = d.a;
        ctx.beginPath();
        ctx.arc(d.x, d.y, d.r, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    ctx.restore();
  };

})(window.ETM);
