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
       corner, where you are in the top left, the keys along the bottom.

   Two cameras, one world. S swaps between the 3/4 plate above and the
   character's own eyes (fpv.js). Both read the same scene, walk the same
   grid, open the same doors and answer the same keys — the only thing that
   changes is where you are standing to look at it. */

import {
  Marker, WorldCard, StatPod, StatPods, PlaceChip, EnvChip,
  PhoneButton, HelpButton, Toast, ActionPrompt, KeyBar, KeyCard, NamePlate,
  PersonTip, Bark,
} from '../../ui/index.js';

(function (E) {
  'use strict';

  var WALKABLE = { street: 'srkpgd', apartment: 'f', quayroom: 'f', riverfront: 'q' };

  /* A doorway is a box, not a circle: you trigger it by stepping in towards
     the wall, so walking the length of a pavement past every shopfront in
     the row never opens one. REACH is how far the O key sees. */
  var DOOR = { x: 0.72, y: 0.6 };
  var REACH = 2.2;

  var cvs, ctx, dpr = 1, W = 0, H = 0, zoom = 1, baseZoom = 1, userZoom = 1;
  var scene, sceneId, actors = [], player, emissive = [];
  var cam = { x: 0, y: 0, set: false, free: false };

  /* The eye. `view` is which camera is live; `fpv` is where the character is
     looking when it is theirs, and it survives a scene change because facing
     is a fact about the player rather than about the room. Yaw is a grid
     bearing: 0 is +x, and it grows towards +y. */
  var view = 'iso';
  var fpv = { yaw: 0.9, pitch: -0.03 };
  var TURN = 2.5;               // radians a second on the arrow keys
  var LOOK = 0.0032;            // radians a pixel when dragging
  var drag = null, walk = {}, suppressClick = false;
  var home = 'east', movedToday = false;   // you live in exactly one environment
  var t = 0, last = 0, fadeFrom = -1e9, quality = 'high';
  var motes = null, markerEls = [], card, cardZone = null, cardVisible = false;
  var promptEl, plateEl, near = null, delivered = 0, unread = 1, openName = null;

  /* The people layer: who the pointer is on, and who is currently saying
     something. Both are DOM over the canvas for the same reason the markers
     are — text stays crisp and nothing has to be hit-tested in a shader. */
  var tipEl, peopleEl, speakers = [], hover = null, pointer = null;

  /* Who you are, as far as the world is concerned: a silhouette and a name.
     It survives every scene change because it is a fact about the player, not
     about the place — and it is the answer to "which one of these is me". */
  var look = { coat: '#38384a', hair: '#1f1a17', hat: null, bag: null, name: 'you' };

  /* How the closet is currently being looked at, which is state about the
     screen rather than about the player: which drawer is open and whether the
     figure in the mirror is standing still or walking. */
  var closet = { slot: 'coat', pose: 'stand' };

  /* Who you are talking to and which of their questions is open. Nothing here
     is spent or remembered: they answer the same tomorrow. */
  var talk = { who: null, asked: -1 };

  /* The canvas a panel draws a figure on — the closet's mirror, or the person
     you clicked. It exists only while such a panel is open. */
  var stage = null;

  /* Your standing on the one board everything feeds into (§3.5). Held here
     rather than typed into the pod, because the pod and the tip over your own
     head have to be the same number or one of them is lying. */
  var KARMA = 12;

  /* The playbook. The only state in this client the player can actually
     change, which is the point: the character acts, and the player edits the
     thing that decides how. There is no draft copy and no save: an edit is
     the change, and a sentence with a blank left in it is simply not covering
     its question. The vocabulary it is composed from is authored in
     panels.js; this is the mutable copy. */
  var pb = null;

  /* The painter (paint.js), and the one live state object it reads. app.js
     keeps S's fields in step wherever it mutates the originals: `cam` and
     `emissive` are mutated in place, so handing over the object once is
     enough; the rest are reassigned below whenever their sources are. */
  var S, P;

  function commas(n) {
    return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  }

  /* ------------------------------------------------------------- boot */

  function init() {
    cvs = document.getElementById('world');
    ctx = cvs.getContext('2d');
    S = { ctx: ctx, scene: null, t: 0, W: 0, H: 0, cam: cam, quality: quality,
          zoom: zoom, emissive: emissive, actors: actors, player: null };
    P = E.Painter(S);
    card = document.getElementById('worldcard');
    promptEl = document.getElementById('prompt');
    plateEl = document.getElementById('nameplate');
    tipEl = document.getElementById('persontip');
    peopleEl = document.getElementById('people');

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
    // the pointer leaving the page is not the pointer resting on somebody
    document.addEventListener('mouseleave', function () { pointer = null; });

    document.body.addEventListener('click', onClick);

    pb = E.PLAYBOOK.fresh(home);

    buildKeys();
    buildPods();
    syncPhone();
    syncName();
    syncViewChrome();
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
    var q = e.target.closest('[data-ask]');
    if (q) { e.preventDefault(); askOf(q.getAttribute('data-ask')); return; }
    var drawer = e.target.closest('[data-slot]');
    if (drawer) { e.preventDefault(); openDrawer(drawer.getAttribute('data-slot')); return; }
    var pose = e.target.closest('[data-pose]');
    if (pose) { e.preventDefault(); setPose(pose.getAttribute('data-pose')); return; }
    var act = e.target.closest('[data-act]');
    if (act) { e.preventDefault(); compose(act.getAttribute('data-act')); return; }
    if (dismissMenu(e)) { e.preventDefault(); return; }
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
    if (e.target.closest('[data-view]')) { toggleView(); return; }
    if (e.target.closest('[data-follow]')) { recenter(); return; }
    if (view === 'fpv') return;                  // zoom belongs to the plate
    var z = e.target.closest('[data-zoom]');
    if (z) {
      userZoom = E.clamp(userZoom * (z.getAttribute('data-zoom') === 'in' ? 1.25 : 0.8), 0.34, 3.2);
      zoom = baseZoom * userZoom;
      S.zoom = zoom;
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
    s: toggleView,
    q: function () { setQuality(quality === 'high' ? 'low' : 'high'); },
    '?': toggleKeyCard,
    '/': toggleKeyCard,
  };

  function onKeyDown(e) {
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.target.closest && e.target.closest('input, textarea')) return;

    if (e.key === 'Escape') {
      if (document.getElementById('keycard').classList.contains('open')) toggleKeyCard();
      // an open menu is the innermost thing on screen, so it goes first
      else if (openName === 'playbook' && pb && pb.menu) { pb.menu = null; openPanel('playbook'); }
      else closePanel();
      return;
    }

    var k = walkKey(e.key);
    if (k) {
      walk[k] = true;
      // Taking the wheel cancels a click-route and puts you back in the
      // world. At eye level the sideways arrows turn the head rather than
      // drive the feet, so looking around does not abandon the walk.
      if (player && (view !== 'fpv' || k === 'u' || k === 'd')) player.route = null;
      closePanel();
      e.preventDefault();
      return;
    }

    var cmd = COMMANDS[e.key.toLowerCase()];
    if (cmd) { e.preventDefault(); cmd(); }
  }

  var KEYBAR = [
    { keys: ['O'], label: 'use what you are next to' },
    { keys: ['S'], label: 'eye level' },
    { keys: ['A'], label: 'your room' },
    { keys: ['P'], label: 'playbook' },
    { keys: ['H'], label: 'health' },
    { keys: ['C'], label: 'phone' },
    { keys: ['?'], label: 'all keys' },
  ];

  // The arrows mean something different behind the character's eyes: the
  // sideways pair turns the head instead of sliding along the plate.
  function keybar() {
    var move = view === 'fpv'
      ? [{ keys: ['↑', '↓'], label: 'walk' }, { keys: ['←', '→'], label: 'turn' }]
      : [{ keys: ['←', '↑', '→', '↓'], label: 'walk' }];
    return move.concat(KEYBAR);
  }

  var KEYCARD = [
    { title: 'Moving', items: [
      { keys: ['←', '↑', '→', '↓'], label: 'Walk &mdash; at eye level, ← → turn' },
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
      { keys: ['S'], label: 'Eye level, or the view from above' },
      { keys: ['Q'], label: 'Effects tier' },
      { keys: ['Esc'], label: 'Back to the world' },
      { keys: ['?'], label: 'This card' },
    ] },
  ];

  function buildKeys() {
    document.getElementById('keybar').innerHTML = KeyBar(keybar());
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
    { glyph: '◈', value: commas(KARMA), note: '+2 wk', label: 'Karma', panel: 'karma' },
  ];

  function buildPods() {
    document.getElementById('pods').innerHTML = StatPods(PODS.map(StatPod));
  }

  /* ------------------------------------------------------------ closet */

  /* "slot:value", straight off the swatch. An empty value is the none
     option, which is a choice rather than the absence of one.

     Nothing is redrawn from scratch here. The figure in the mirror is painted
     from `look` every frame, so by the time the click has been handled it is
     already wearing the coat; the panel only has to move its ticks. That is
     the whole difference between this and a form: there is no version of the
     outfit that exists after you close the editor. */
  function wear(spec) {
    var i = spec.indexOf(':');
    var slot = spec.slice(0, i), value = spec.slice(i + 1);
    look[slot] = value || null;
    dressPlayer();
    syncName();
    if (openName === 'closet') syncCloset();
  }

  /* Which drawer of the rack is open. Every drawer is already in the DOM, so
     this is a class toggle: the figure above it never blinks. */
  function openDrawer(slot) {
    closet.slot = slot;
    each('[data-slot]', function (el) {
      el.classList.toggle('on', el.getAttribute('data-slot') === slot);
    });
    each('[data-drawer]', function (el) {
      el.classList.toggle('open', el.getAttribute('data-drawer') === slot);
    });
    sizeStage();           // a taller drawer is a shorter mirror
  }

  /* Standing or walking. A colour you chose standing still is a colour you
     have not actually seen: the street only ever shows you people mid-stride. */
  function setPose(pose) {
    closet.pose = pose;
    each('[data-pose]', function (el) {
      var on = el.getAttribute('data-pose') === pose;
      el.classList.toggle('primary', on);
      el.classList.toggle('ghost', !on);
    });
  }

  function each(sel, fn) {
    document.querySelectorAll('#panelBody ' + sel).forEach(fn);
  }

  /* The panel's ticks, brought back in line with `look` after a pick. The
     rack's tabs carry what is in each drawer, so they read whatever the
     drawer itself now says is on — one source, patched in one place. */
  function syncCloset() {
    each('[data-pick]', function (el) {
      var spec = el.getAttribute('data-pick');
      var i = spec.indexOf(':');
      el.classList.toggle('on',
        (look[spec.slice(0, i)] || null) === (spec.slice(i + 1) || null));
    });
    each('[data-slot]', function (tab) {
      var on = document.querySelector(
        '#panelBody [data-drawer="' + tab.getAttribute('data-slot') + '"] .swatch.on');
      if (!on) return;
      var chip = tab.querySelector('.swatch-chip');
      var from = on.querySelector('.swatch-chip');
      chip.style.background = from.style.background;
      chip.classList.toggle('none', on.classList.contains('none'));
      tab.querySelector('.rack-tab-worn').textContent =
        on.querySelector('.swatch-label').textContent;
    });
    var plate = document.querySelector('#panelBody .mirror-name .nameplate-name');
    if (plate) plate.textContent = look.name;
  }

  /* ------------------------------------------------------------- stage */

  /* A panel that has a figure in it has this canvas, and it is alive only
     while that panel is open. Two panels use it and they are the same
     mechanism: the closet draws you, and a person's page draws them. Both go
     through E.actor, which is the point — nobody gets a flattering portrait
     the street cannot produce. */
  function mountStage() {
    var el = document.querySelector('#panelBody .mirror-figure');
    if (!el) { stage = null; return; }
    stage = { el: el, ctx: el.getContext('2d'), w: 0, h: 0,
              plate: document.querySelector('#panelBody .mirror-name') };
    sizeStage();
  }

  function sizeStage() {
    if (!stage) return;
    var r = stage.el.getBoundingClientRect();
    if (!r.width || !r.height) return;
    stage.w = r.width;
    stage.h = r.height;
    stage.el.width = Math.round(r.width * dpr);
    stage.el.height = Math.round(r.height * dpr);
  }

  // Who the open panel is about. The closet is about you, and it is the live
  // `look` rather than a copy of it — that is what makes a pick instant.
  function stageSubject() {
    if (openName === 'closet') {
      return { color: look.coat, hair: look.hair, hat: look.hat, bag: look.bag,
               moving: closet.pose === 'walk' };
    }
    if (openName === 'person') {
      var who = E.person(talk.who);
      return who ? who.figure : null;
    }
    return null;
  }

  function drawStage() {
    if (!stage) return;
    var a = stageSubject();
    if (!a) return;
    if (!stage.w) sizeStage();       // the stage may not have been laid out yet
    if (!stage.w) return;
    var c = stage.ctx;
    c.setTransform(dpr, 0, 0, dpr, 0, 0);
    var at = E.mannequin(c, stage.w, stage.h, a, t);
    // the plate hangs off the head, and only the drawing knows where that is
    if (stage.plate) {
      stage.plate.style.left = at.x + 'px';
      stage.plate.style.top = (at.top - 10) + 'px';
      stage.plate.classList.add('show');
    }
  }

  /* ----------------------------------------------------------- talking */

  /* Somebody in the street, asked something. Which person you are in front of
     and which of their questions is open are state like any other, so walking
     up to them and choosing a question go through the same path — and asking
     the open question again puts it away.

     There is nothing to commit and nothing is spent: they will answer the
     same way tomorrow. The cost of the interaction was walking over. */
  function talkTo(a) {
    if (!a || a === player || !E.person(a.id)) return;
    if (talk.who !== a.id) { talk.who = a.id; talk.asked = -1; }
    openPanel('person');
  }

  function askOf(spec) {
    var i = spec.indexOf(':');
    var n = Number(spec.slice(i + 1));
    talk.who = spec.slice(0, i);
    talk.asked = talk.asked === n ? -1 : n;
    openPanel('person');
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

  /* ---------------------------------------------------------- playbook */

  /* Every edit the sentence can take arrives as one string — "verb:argument",
     read here and nowhere else, the same idiom the closet's swatches use.
     The panel is a pure function of `pb`, so the whole interaction model is:
     apply the change, draw it again. Nothing is validated on the way in
     because nothing can be typed; the vocabulary is the validation.

     A target is a part of the sentence: "c0" and "c1" are the conditions,
     "then" and "else" are the two actions. Which menu is open is state like
     any other, so a click that opens one and a click that changes a word go
     through exactly the same path — and there is no third kind of click that
     saves, because there is nothing to save. */
  function compose(spec) {
    var bits = spec.split(':');
    var ans = pb.answers[pb.set][pb.q];

    switch (bits[0]) {
      case 'env':                          // look at another environment's answers
        pb.set = bits[1]; pb.menu = null;
        break;
      case 'q':                            // answer a different question
        pb.q = bits[1]; pb.menu = null;
        break;
      // Clicking the part you already have open closes it again, which is the
      // only way a menu with nothing you want in it should behave.
      case 'open':
        var target = bits.slice(1).join(':');
        pb.menu = pb.menu === target ? null : target;
        break;
      case 'stay':                         // a locked term: seen, not takeable
        break;
      case 'put':                          // a term into a part of the sentence
        put(ans, bits[1], { id: bits[2] });
        pb.menu = null;
        break;
      case 'value':                        // a threshold, from its authored set
        var ref = refOf(ans, bits[1]);
        if (ref) ref.v = +bits[2];
        pb.menu = null;
        break;
      // The second condition is grown from the first one's own menu, so the
      // sentence is never longer than what the player has asked it to say.
      case 'add':
        ans.conds[1] = null;
        pb.menu = 'c1';
        break;
      case 'join':
        ans.join = bits[1];
        pb.menu = null;
        break;
      // Dropping the first of a pair promotes the second, because what is
      // left has to remain a sentence.
      case 'drop':
        if (bits[1] === 'c1') ans.conds.length = 1;
        else if (ans.conds.length > 1) ans.conds.shift();
        else ans.conds[0] = null;
        pb.menu = null;
        break;
    }

    // Every edit is live the moment it is made, so the count on the chip out
    // in the street can go stale on any of them, not just on a save.
    syncChrome();
    openPanel('playbook');
  }

  function refOf(ans, target) {
    return target === 'then' ? ans.then
      : target === 'else' ? ans.otherwise
      : ans.conds[+target.slice(1)];
  }

  function put(ans, target, ref) {
    if (target === 'then') ans.then = ref;
    else if (target === 'else') ans.otherwise = ref;
    else ans.conds[+target.slice(1)] = ref;
  }

  // An open menu is dismissed by clicking anywhere that is not part of one.
  // Redrawing is only worth it when the click landed inside the panel; a
  // click out in the world has already closed the panel over it.
  function dismissMenu(e) {
    if (!pb || !pb.menu || e.target.closest('[data-act]')) return false;
    pb.menu = null;
    if (openName !== 'playbook' || !e.target.closest('#panelBody')) return false;
    openPanel('playbook');
    return true;
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
    S.quality = quality;
    document.getElementById('qualityLabel').textContent = 'Effects: ' + q;
    document.body.classList.toggle('quality-low', q === 'low');
  }

  /* S is a camera, not a mode. Nothing else changes: the same character is
     standing on the same tile facing the same way, the doors are the same
     doors and the panels are the same panels — you have either climbed up to
     look at the block or come back down into it. That is why it is one key
     and why it is instant. */
  function toggleView() {
    view = view === 'iso' ? 'fpv' : 'iso';
    // a short version of the scene-change wash, so the swap lands rather than cuts
    fadeFrom = performance.now() - 240;
    drag = null;
    fpv.pitch = E.clamp(fpv.pitch, -0.34, 0.34);
    buildKeys();
    syncViewChrome();
  }

  // Zoom and follow are the overhead camera's controls and mean nothing from
  // the pavement, so they leave rather than sit there dead.
  function syncViewChrome() {
    var isFpv = view === 'fpv';
    document.body.classList.toggle('view-fpv', isFpv);
    document.getElementById('viewBtn').textContent = isFpv ? 'Overhead' : 'Eye level';
    document.getElementById('viewBtn').classList.toggle('on', isFpv);
    ['followBtn', 'zoomIn', 'zoomOut'].forEach(function (id) {
      document.getElementById(id).hidden = isFpv;
    });
    if (isFpv) plateEl.style.opacity = '0';
    else plateEl.style.opacity = '';
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
    // At eye level you arrive facing the door you just came through, which is
    // a wall. Turn round on the way in: what you walked into is behind you.
    if (back) fpv.yaw = Math.atan2(start[1] - back.y, start[0] - back.x) || fpv.yaw;
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

    // the painter reads the scene through S, so a new scene is handed over here
    S.scene = scene;
    S.player = player;
    S.actors = actors;

    clearHover();
    buildPeople();

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
    // Your answers are scoped to where you live, so the playbook opens on the
    // set that is now in force rather than on the one you left behind.
    pb.set = home;
    pb.menu = null;
    setScene(E.ENVIRONMENTS[home].outdoor);
    closePanel();
  }

  function syncChrome() {
    document.getElementById('placeChip').innerHTML = PlaceChip({
      name: scene.name, where: scene.where, clock: scene.clock,
    });
    var env = E.ENVIRONMENTS[scene.env] || E.ENVIRONMENTS[home];
    // The chip counts the answers actually running here, so committing one in
    // the playbook is visible from the street a second later. Anything less
    // and the chrome would be quoting a number the playbook had moved past.
    var held = pb ? E.PLAYBOOK.answered(pb, env.id) : 0;
    document.getElementById('envChip').innerHTML = EnvChip({
      label: 'Rule set in force', name: env.name,
      rules: held + ' answered', ruleTone: held < 2 ? 'amber' : '',
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

  /* ------------------------------------------------------------ people

     Nobody out there is nobody: every figure on a pavement is somebody the
     world has written down (people.js). Three things get you to them, and all
     three are pull rather than push:

       · Hover names them and reads out their Karma.
       · Clicking one asks them something. The questions are theirs and the
         answers are authored — you are reading a person, not opening a menu.
       · They talk. Nothing is waiting on a bark and there is nothing to
         answer, which is the whole point: it is how a street sounds
         inhabited without becoming a queue of things to deal with. */

  function buildPeople() {
    peopleEl.innerHTML = '';
    speakers = actors.filter(function (a) { return a.lines && a.lines.length; })
      .map(function (a) {
        var el = document.createElement('div');
        el.className = 'bark';
        peopleEl.appendChild(el);
        a.said = -1;
        a.sayFor = 0;
        a.next = 1.6 + Math.random() * 2.4;
        return { a: a, el: el };
      });
  }

  /* Barks only happen near you. A line delivered to an empty street two
     blocks away has been spent on nobody — and the ones that are about you
     ("nice coat") only land if you are close enough to be wearing it. */
  function tickBarks(dt) {
    for (var i = 0; i < speakers.length; i++) {
      var s = speakers[i], a = s.a;
      var close = Math.hypot(player.x - a.x, player.y - a.y) < (a.barkRange || 13);

      if (a.sayFor > 0) a.sayFor -= dt;
      if (!close) {
        a.sayFor = 0;
        a.next = 1.2 + Math.random() * 2.2;      // armed again for the next pass
      } else if (a.sayFor <= 0) {
        a.next -= dt;
        if (a.next <= 0) {
          // never the same line twice running: that is what makes a loop audible
          var n = a.lines.length;
          var held = a.said >= 0 && n > 1;
          var pick = Math.floor(Math.random() * (held ? n - 1 : n));
          if (held && pick >= a.said) pick++;
          a.said = pick;
          s.el.innerHTML = Bark(a.lines[pick]);
          a.sayFor = 4.6;
          a.next = (a.barkEvery || 12) + Math.random() * 7;
        }
      }

      /* The tip and the bubble want the same air over the same head, so the
         question you asked wins over the line you were only overhearing. And
         a bubble is a paragraph rather than a dot: at the edge of the frame
         it slides under the chips and the keybar, and half a line of
         somebody's heckling is worse than none of it. */
      var live = a.sayFor > 0 && a !== hover;
      var p = toScreen(a.x, a.y, 96 * (a.scale || 1));
      var vis = live && p.x > 140 && p.x < W - 140 && p.y > 130 && p.y < H - 130 &&
                (view !== 'fpv' || (p.ok && p.d < 20));
      s.el.classList.toggle('show', vis);
      if (vis) {
        s.el.style.transform =
          'translate(' + p.x.toFixed(1) + 'px,' + p.y.toFixed(1) + 'px) translate(-50%,-100%)';
      }
    }
  }

  /* Who is under the pointer. The figure is a silhouette about 74 iso pixels
     tall drawn straight up from the feet, so the box is derived from the two
     projected ends of it rather than guessed — that way it is the same test
     overhead and at eye level, where a person's size on the glass is a
     function of how far away they are. */
  function personAt(px, py) {
    var list = actors.concat([player]);
    var best = null, bestKey = -Infinity;
    for (var i = 0; i < list.length; i++) {
      var a = list[i];
      if (a !== player && !a.name) continue;
      var feet = toScreen(a.x, a.y, 0);
      if (!feet.ok) continue;
      var head = toScreen(a.x, a.y, 74 * (a.scale || 1));
      var h = feet.y - head.y;
      if (h < 8) continue;                      // too far off to be pointed at
      var halfW = Math.max(6, h * 0.22);
      if (px < feet.x - halfW || px > feet.x + halfW) continue;
      if (py < head.y || py > feet.y + h * 0.08) continue;
      // whoever is in front is who you meant, which is the paint order
      var key = view === 'fpv' ? -feet.d : a.x + a.y;
      if (key > bestKey) { bestKey = key; best = a; }
    }
    return best;
  }

  function clearHover() {
    if (hover) hover.hover = false;
    hover = null;
    tipEl.classList.remove('show');
    cvs.classList.remove('on-person');
  }

  function updateHover() {
    var want = null;
    // not while you are dragging the camera, and not under an open panel
    if (pointer && !drag && !openName) {
      var r = cvs.getBoundingClientRect();
      want = personAt(pointer.x - r.left, pointer.y - r.top);
    }

    if (want !== hover) {
      if (hover) hover.hover = false;
      hover = want;
      if (hover) {
        hover.hover = true;
        var you = hover === player;
        tipEl.innerHTML = PersonTip({
          name: you ? look.name : hover.name,
          note: you ? 'the one you are walking' : hover.note,
          karma: commas(you ? KARMA : hover.karma),
          // what you can do about them belongs on the tip, not on a label
          // over every head: it is the answer to pointing, like the rest of it
          hint: you ? null : 'Click to ask',
        });
      }
      tipEl.classList.toggle('show', !!hover);
      cvs.classList.toggle('on-person', !!hover);
    }

    // your own plate would sit inside the tip that already names you. Read
    // every frame rather than on the change, because the camera key hides
    // and restores the plate on its own account.
    if (view !== 'fpv') plateEl.style.opacity = hover === player ? '0' : '';

    if (!hover) return;
    // over your own head the plate is already there, so the tip clears it
    var lift = (hover === player && view !== 'fpv' ? 124 : 80) * (hover.scale || 1);
    var s = toScreen(hover.x, hover.y, lift);
    var ok = s.ok && (view !== 'fpv' || s.d < 24);
    tipEl.style.opacity = ok ? '' : '0';
    if (ok) {
      tipEl.style.transform =
        'translate(' + s.x.toFixed(1) + 'px,' + s.y.toFixed(1) + 'px) translate(-50%,-100%)';
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
    S.W = W; S.H = H; S.zoom = zoom;
    sizeStage();
  }

  /* -------------------------------------------------------------- loop */

  function loop(ts) {
    var dt = last ? Math.min((ts - last) / 1000, 0.05) : 0;
    last = ts;
    t += dt;
    S.t = t;
    update(dt);
    draw();
    drawStage();           // no-op unless a panel with a figure is open
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
        /* A path of two points is somebody going up and down a pavement. Any
           more of them is a circuit, and it wraps — which is the difference
           between a pedestrian and somebody with a reason to be here. How
           long they stand at each end is part of who they are: a man between
           sets waits, and a judge between listings does not. */
        a.wp = (a.wp + 1) % a.path.length;
        a.hold = (0.5 + Math.random()) * (a.linger || 1);
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
  /* Behind the character's eyes there is no screen space to read the arrows
     in: forward is wherever they are facing. Sideways turns the head, which
     is the one control that has no meaning at all on the plate above. */
  function steerFPV(dt) {
    var turn = (walk.r ? 1 : 0) - (walk.l ? 1 : 0);
    if (turn) fpv.yaw += turn * TURN * dt;

    var fwd = (walk.u ? 1 : 0) - (walk.d ? 1 : 0);
    if (!fwd) { if (turn) player.moving = false; return false; }

    // backing up is a shuffle, not a sprint
    var v = (player.speed || 2.8) * dt * (fwd > 0 ? 1 : -0.55);
    var dx = Math.cos(fpv.yaw) * v, dy = Math.sin(fpv.yaw) * v;
    if (standable(player.x + dx, player.y)) player.x += dx;
    if (standable(player.x, player.y + dy)) player.y += dy;
    player.hold = 0;
    player.moving = true;
    return true;
  }

  function steer(dt) {
    if (view === 'fpv') return steerFPV(dt);
    var a = (walk.r ? 1 : 0) - (walk.l ? 1 : 0);
    var b = (walk.d ? 1 : 0) - (walk.u ? 1 : 0);
    if (!a && !b) return false;

    var dx = (a + b) / 2, dy = (b - a) / 2;
    var len = Math.hypot(dx, dy);
    var v = (player.speed || 2.8) * dt / len;
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

    // Clicking the pavement still routes you at eye level, so the character
    // turns into the walk on their own — otherwise you arrive somewhere
    // facing the way you set off.
    if (view === 'fpv' && player.route && !walk.l && !walk.r) {
      var tg = player.route[player.ri];
      if (tg) {
        var want = Math.atan2(tg[1] - player.y, tg[0] - player.x);
        var off = Math.atan2(Math.sin(want - fpv.yaw), Math.cos(want - fpv.yaw));
        fpv.yaw += off * Math.min(1, dt * 4);
      }
    }

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
    updateHover();
    tickBarks(dt);
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

  /* Where a point in the world lands on the glass. Everything floating over
     the canvas — markers, the prompt, the name plate, the world card — asks
     this and nothing else, so both cameras stay one line apart. */
  function toScreen(gx, gy, z) {
    if (view === 'fpv') return E.FPV.screen(gx, gy, z || 0);
    var p = E.isoc(gx, gy);
    return {
      x: (p.x - cam.x) * zoom + W / 2,
      y: (p.y - cam.y - (z || 0)) * zoom + H / 2,
      ok: true, d: 0
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
      /* Overhead the prompt hangs above your own head, because you can see
         your own head. At eye level you cannot, so it hangs on the thing
         itself — and falls back to the foot of the screen if the thing you
         are standing in is already past the edge of the frame. */
      var px, py;
      if (view === 'fpv') {
        var np2 = toScreen(near.x, near.y, 132);
        px = np2.ok ? E.clamp(np2.x, 140, Math.max(140, W - 140)) : W / 2;
        py = np2.ok ? E.clamp(np2.y, 96, Math.max(96, H - 152)) : H - 168;
      } else {
        var p = toScreen(player.x, player.y, 146);
        px = p.x; py = p.y;
      }
      promptEl.style.transform =
        'translate(' + px.toFixed(1) + 'px,' + py.toFixed(1) + 'px) translate(-50%,-100%)';
    }

    // the plate sits just off the head; the prompt stacks above it. There is
    // no head in shot at eye level, so there is nothing to label.
    if (view !== 'fpv') {
      var np = toScreen(player.x, player.y, 74);
      plateEl.style.transform =
        'translate(' + np.x.toFixed(1) + 'px,' + np.y.toFixed(1) + 'px) translate(-50%,-100%)';
    }
  }

  function updateMarkers() {
    for (var i = 0; i < markerEls.length; i++) {
      var m = markerEls[i];
      // At eye level a label three streets away is noise rather than
      // signage: the sign is on the building, and you have to be near enough
      // to read it.
      var s = toScreen(m.m.x, m.m.y, view === 'fpv' ? 96 : 8);
      var vis = s.x > -60 && s.x < W + 60 && s.y > -40 && s.y < H + 40 &&
                (view !== 'fpv' || (s.ok && s.d < 22));
      m.el.style.transform = 'translate(' + s.x.toFixed(1) + 'px,' + s.y.toFixed(1) + 'px) translate(-50%,-100%)';
      m.el.style.opacity = vis ? 1 : 0;
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

    if (view === 'fpv') drawEyes();
    else drawPlate();

    // Wall-clock, not accumulated frame time: a throttled tab must never
    // resume holding a half-finished transition over the scene.
    var fade = 1 - E.clamp((performance.now() - fadeFrom) / 480, 0, 1);
    if (fade > 0) {
      ctx.fillStyle = 'rgba(240,234,222,' + (fade * 0.85).toFixed(3) + ')';
      ctx.fillRect(0, 0, W, H);
    }
  }

  function drawPlate() {
    P.drawSky();

    applyCam();
    P.drawGround();
    P.drawShadows();
    P.drawObjects();
    drawZoneRing();
    P.drawEmissive();

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    if (quality === 'high') {
      P.sunWash();
      if (motes) motes.draw(ctx, '#fff0cc');
    }
  }

  /* The same scene, from inside it. fpv.js owns the projection; what it does
     not own is the art — the fixtures and the people are drawn by the very
     functions the plate uses, billboarded into perspective, so a bench added
     to render.js is a bench in both views and nobody has to remember that. */
  function drawEyes() {
    var bob = player.moving ? Math.abs(Math.sin(t * 9)) * 0.035 : Math.sin(t * 1.5) * 0.008;
    E.FPV.frame(ctx, {
      W: W, H: H, dpr: dpr,
      scene: scene, elev: E.ELEV,
      x: player.x, y: player.y, yaw: fpv.yaw, pitch: fpv.pitch, zoff: bob,
      t: t, quality: quality, actors: actors, zone: cardZone,
      drawFixture: P.drawFixture,
      drawActor: function (a) { E.actor(ctx, a, t); }
    });
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
      yaw: fpv.yaw, pitch: fpv.pitch,
      onCanvas: ev.target === cvs
    };
    cvs.classList.add('dragging');
  }

  /* Dragging is "look around" in both views. On the plate that means sliding
     the camera off the character; at eye level it means turning their head,
     which is the same gesture meaning the same thing. */
  function onPointerMove(ev) {
    // where the pointer is is a question the people layer asks every frame,
    // whether or not this move is part of a drag
    pointer = { x: ev.clientX, y: ev.clientY };
    if (!drag) return;
    var dx = ev.clientX - drag.sx, dy = ev.clientY - drag.sy;
    drag.moved = Math.max(drag.moved, Math.abs(dx) + Math.abs(dy));
    if (drag.moved < 4) return;
    if (view === 'fpv') {
      fpv.yaw = drag.yaw - dx * LOOK;
      fpv.pitch = E.clamp(drag.pitch - dy * LOOK, -0.34, 0.34);
      return;
    }
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
    if (view === 'fpv') return;      // there is no zoom inside your own head
    var r = cvs.getBoundingClientRect();
    var sx = ev.clientX - r.left - W / 2, sy = ev.clientY - r.top - H / 2;
    var wx = sx / zoom + cam.x, wy = sy / zoom + cam.y;

    userZoom = E.clamp(userZoom * (ev.deltaY < 0 ? 1.12 : 1 / 1.12), 0.34, 3.2);
    zoom = baseZoom * userZoom;
    S.zoom = zoom;

    cam.x = wx - sx / zoom;
    cam.y = wy - sy / zoom;
    if (!cam.free) { cam.free = true; syncFollowChip(); }
    clampCam();
  }

  function onCanvasClick(ev) {
    var r = cvs.getBoundingClientRect();
    // A person is the first thing a click can mean. Everybody out there is
    // somebody, so pointing at one of them asks them something rather than
    // walking you through the space they are standing in.
    var who = personAt(ev.clientX - r.left, ev.clientY - r.top);
    if (who && who !== player) { talkTo(who); return; }
    var gx, gy;
    if (view === 'fpv') {
      var g = E.FPV.ground(ev.clientX - r.left, ev.clientY - r.top);
      if (!g) return;                            // you clicked the sky
      gx = g.x; gy = g.y;
    } else {
      var sx = (ev.clientX - r.left - W / 2) / zoom + cam.x;
      var sy = (ev.clientY - r.top - H / 2) / zoom + cam.y - E.TH / 2;
      gx = (sx / (E.TW / 2) + sy / (E.TH / 2)) / 2;
      gy = (sy / (E.TH / 2) - sx / (E.TW / 2)) / 2;
    }
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
    /* Most panels are a fixed string. The ones that are not are functions of
       everything the client currently knows — what you are wearing, what you
       have composed, who you are standing in front of — so there is one bag
       of state and each panel takes what it needs out of it. A panel about a
       person needs its title from that bag too, so titles read the same way. */
    var s = { look: look, pb: pb, closet: closet, talk: talk, who: E.person(talk.who) };
    var read = function (v) { return (typeof v === 'function' ? v(s) : v) || ''; };
    document.getElementById('panelTitle').innerHTML = read(p.title);
    document.getElementById('panelSub').innerHTML = read(p.sub);
    document.getElementById('panelHead').style.display = p.title ? '' : 'none';
    document.getElementById('panelBody').innerHTML =
      typeof p.html === 'function' ? p.html(s) : p.html;
    // a panel with a figure in it keeps drawing after it has been built
    mountStage();
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
    stage = null;
  }

  E.openPanel = openPanel;

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();

})(window.ETM);
