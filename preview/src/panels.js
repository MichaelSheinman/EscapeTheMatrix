/* Escape the Matrix — overlay panels.
   These sit above the canvas and never replace it: the world stays visible and
   dimmed behind them (technical.md §10.1). Static content, with three
   exceptions: the closet, the playbook and whoever you are standing in front
   of are functions of state, because in each case the state *is* the content.
   The cast those person pages are drawn from is authored in people.js — this
   file arranges a dossier, it does not know anybody. The playbook's
   vocabulary — the standing questions,
   the terms each one takes, the numbers a threshold may hold — is
   authored here beside the copy; which of it is currently selected, and what
   the player has committed, belongs to app.js.

   Every visual decision here comes from @etm/ui. This file contributes copy
   and arrangement only; if you find yourself writing a class name or a colour
   below, the component you need is missing from the library. */

import {
  Button, ButtonRow, Tag, Token, Tokens, Kw,
  Panel, Stack, Grid, Readout, Stat, Metric, Sparkline,
  LockedSlot, Slots, Notice, Tabs, Note, Prose, Divider,
  LogRow, BoardRow, LadderRow, DecayRow, Posting,
  Rule, SceneHead, Dialogue, Speech, Option, Chip, Chips, Commit,
  Ask, Sentence, Clause, Editable, Num, Menu, Question, Reply,
  Swatch, Swatches, Wardrobe, Mirror, Rack, RackTab, RackDrawer,
} from '../../ui/index.js';

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  /* Thousands separators. The same one-liner app.js has, and deliberately not
     imported from it: a panel is a pure function of what it is handed. */
  const commas = (n) => String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ',');

  /* An environment-scoped block: shown only when the player lives there.
     app.js toggles these on `data-when` at open time. */
  const When = (where, children) =>
    `<div data-when="${where}">${children}</div>`;

  /* The closet's vocabulary. Authored, closed and small, for the same reason
     the rule composer's is (§3.4): the player picks from what exists. Nothing
     here is bought yet — this is the drawer you arrived with. */
  const WEAR = [
    { slot: 'coat', label: 'Coat',
      note: 'Everyone on Rowan St is drawn the way you are, and every one of them is ' +
            'somebody. The coat is how you find yourself among them &mdash; and how ' +
            'Nadia describes you to somebody who was not there.',
      options: [
        { label: 'Charcoal', color: '#38384a' },
        { label: 'Rust', color: '#8c4a34' },
        { label: 'Moss', color: '#4b5f4a' },
        { label: 'Slate', color: '#3f5570' },
        { label: 'Sand', color: '#97845f' },
      ] },
    { slot: 'hair', label: 'Hair',
      note: 'The one thing here nobody can take off you.',
      options: [
        { label: 'Black', color: '#1f1a17' },
        { label: 'Auburn', color: '#5e3220' },
        { label: 'Ash', color: '#6b6459' },
        { label: 'Fair', color: '#ae8a56' },
      ] },
    { slot: 'hat', label: 'Hat',
      note: 'A brim reads from across the road where a face never will.',
      options: [
        { label: 'None' },
        { label: 'Flat cap', color: '#34302a' },
        { label: 'Watch cap', color: '#4e5c68' },
      ] },
    { slot: 'bag', label: 'Bag',
      note: 'Marta remembers a bag. She does not remember a coat.',
      options: [
        { label: 'None' },
        { label: 'Satchel', color: '#6b5540' },
        { label: 'Courier', color: '#45505c' },
      ] },
  ];

  const HANDLES = [
    { label: 'you', color: '#b26a22' },
    { label: 'wren_a', color: '#4b5f4a' },
    { label: 'lowtide', color: '#3f5570' },
    { label: 'ilva_m', color: '#8c4a34' },
  ];

  /* A name is not a garment, but it is picked from a closed list the same
     way and it is on you in the same places, so it is the fifth drawer
     rather than a panel of its own. */
  const DRAWERS = WEAR.concat([
    { slot: 'name', label: 'Name', kind: 'name',
      note: 'It is over your head in the street and on every board you appear on. ' +
            'Change it here and the boards print the new one from tomorrow.',
      options: HANDLES },
  ]);

  /* What a swatch in this drawer is worth: the paint for a garment, the word
     itself for a name. An empty string is the none option, which is a choice
     rather than the absence of one. */
  const worth = (d, o) => (d.kind === 'name' ? o.label : (o.color || ''));

  /* Which option is currently on you. There is always one — a slot you have
     left empty is wearing its none. */
  const worn = (d, look) =>
    d.options.filter((o) => (worth(d, o) || null) === (look[d.slot] || null))[0] ||
    d.options[0];


  /* ═════════════════════════════════════════════ THE PLAYBOOK'S CONTENT

     The player does not act day to day; the playbook does, and the player
     comes back to it after the fact and changes it. So the playbook is not a
     list of rules the player thought of — it is a fixed, small set of
     questions the world will definitely ask, and the answer to each one is
     composed from a closed vocabulary.

     None of the answers is correct. Each is effective in some environment and
     ruinous in another, and environments drift, so the loop is: answer, walk
     around for a week, read what happened, answer again. Nothing in this file
     says which answer suits which place — that is what the evidence panel is
     for, and working it out is the game. */

  /* The numbers a threshold may take. Closed and authored for the same reason
     the vocabulary is: the interesting question is *which* threshold, never
     how precisely you can name one. */
  const VALUES = {
    fee:   { label: 'paid at least', options: ['above $20', 'above $60', 'above $150', 'above $400'] },
    karma: { label: 'their standing', options: ['above 50', 'above 200', 'above 800', 'above 2,000'] },
    hours: { label: 'how long it takes', options: ['under 1 hour', 'under 2 hours', 'under 4 hours', 'under 8 hours'] },
    spare: { label: 'spare in hand', options: ['over $200', 'over $800', 'over $2,000'] },
    cap:   { label: 'the ceiling', options: ['up to $10', 'up to $40', 'up to $100'] },
    rate:  { label: 'multiple of my rate', options: ['1.2&times;', '1.5&times;', '2&times;', '3&times;'] },
    asked: { label: 'how often', options: ['once', 'twice', 'three times'] },
  };

  /* The standing questions. Every one of them is something that happens to a
     person constantly, which is why the answer has to run without you.
     Nothing in the client ever states how many there are: the set grows, and
     copy that counts it is wrong the day another one lands. */
  const QUESTIONS = [
    { id: 'help', n: 1,
      ask: 'Somebody asks you for help with a task. When do you say yes?',
      note: 'It happens two or three times a week and you are not there for any of them.',
      about: 'someone asks me for help',
      // What the player cannot say yet is not listed, blanked or hinted at:
      // it turns up in the menu the day it unlocks. Reserving a row for it
      // lengthens every menu and spends the surprise early.
      conds: [
        { id: 'fee',    text: 'they are paying me {v}', param: 'fee' },
        { id: 'useful', text: 'they could be useful to me later' },
        { id: 'karma',  text: 'their Karma is {v}', param: 'karma' },
        { id: 'short',  text: 'it takes me {v}', param: 'hours' },
        { id: 'legal',  text: 'the task is legal' },
        { id: 'friend', text: 'they could become a friend' },
      ],
      acts: [
        { id: 'yes',   text: 'say yes' },
        { id: 'once',  text: 'say yes, this once' },
        { id: 'price', text: 'name a price' },
        { id: 'no',    text: 'say no' },
      ],
    },

    { id: 'voice', n: 2,
      ask: 'Somebody raises their voice at you. What do you do?',
      note: 'Whatever you pick, you will have done it eleven times before you read about it.',
      about: 'someone raises their voice at me',
      conds: [
        { id: 'above',    text: 'they are above me at work' },
        { id: 'karma',    text: 'their Karma is {v}', param: 'karma' },
        { id: 'watching', text: 'other people are watching' },
        { id: 'before',   text: 'they have done this to me before' },
        { id: 'leave',    text: 'I could walk out of the room' },
        { id: 'owe',      text: 'I owe them something' },
      ],
      // the first of these is what an unanswered question opens with, so it
      // is the least committal one rather than the most interesting
      acts: [
        { id: 'finish', text: 'let them finish' },
        { id: 'feel',   text: 'ask them how they feel' },
        { id: 'leave',  text: 'leave the situation' },
        { id: 'quiet',  text: 'stay quiet' },
        { id: 'back',   text: 'raise my voice back' },
      ],
    },

    { id: 'money', n: 3,
      ask: 'Somebody asks you for money. What do you do?',
      note: 'You have no answer to this one, so every time it happens it comes to you.',
      about: 'someone asks me for money',
      conds: [
        { id: 'spare',  text: 'I am holding {v} spare', param: 'spare' },
        { id: 'repaid', text: 'they have paid me back before' },
        { id: 'karma',  text: 'their Karma is {v}', param: 'karma' },
        { id: 'public', text: 'they asked in front of other people' },
        { id: 'owe',    text: 'I owe them something' },
        { id: 'again',  text: 'they have asked me {v} this month', param: 'asked' },
      ],
      acts: [
        { id: 'give',     text: 'give them {v}', param: 'cap' },
        { id: 'interest', text: 'lend it and ask for interest' },
        { id: 'think',    text: 'say I will think about it' },
        { id: 'none',     text: 'give them nothing' },
      ],
    },

    { id: 'overtime', n: 4,
      ask: 'You are asked to work overtime. What do you do?',
      note: 'The port asks late in the day, which is exactly when nobody decides well.',
      about: 'I am asked to work overtime',
      conds: [
        { id: 'rate',    text: 'it pays {v} my rate', param: 'rate' },
        { id: 'direct',  text: 'the manager asked me directly' },
        { id: 'already', text: 'I have already worked overtime this week' },
        { id: 'short',   text: 'it is {v}', param: 'hours' },
        { id: 'owed',    text: 'I am owed time back' },
      ],
      acts: [
        { id: 'accept', text: 'accept it' },
        { id: 'once',   text: 'accept it once' },
        { id: 'time',   text: 'ask for the time back instead' },
        { id: 'refuse', text: 'refuse' },
      ],
    },
  ];

  /* The two places you can live, and the rule set each one holds. Ids match
     E.ENVIRONMENTS, because a rule set is not a folder — it is where you are
     standing (§3.15). */
  const SETS = [
    { id: 'east', label: 'Tanner Row' },
    { id: 'quay', label: 'Riverfront' },
  ];

  /* What the answers already in force have actually done. This is the only
     feedback the game gives, and it is deliberately evidence rather than a
     verdict: the entries say what happened, never what you should have
     answered instead. */
  const EVIDENCE = {
    east: {
      help: {
        rows: [
          { time: 'Mon', title: 'Said yes to Ilya', detail: '$80 for two hours at the yard', tag: 'good' },
          { time: 'Tue', title: 'Said yes to Marta', detail: 'No money in it. She asked because you always say yes', tag: 'poor' },
          { time: 'Thu', title: 'Said no to Kez', detail: 'Four hours, unpaid, and he found somebody else', tag: 'good' },
          { time: 'Fri', title: 'Said yes to Ilya', detail: 'He has started assuming Thursdays', tag: 'poor' },
        ],
        note: 'Fired 31 times here. Nine went badly, and seven of the nine were the same person.',
      },
      voice: {
        rows: [
          { time: 'Wed', title: 'Marta, about the roster', detail: 'You stayed quiet. She apologised the next morning', tag: 'good' },
          { time: 'Thu', title: 'A driver at the gate', detail: 'You stayed quiet. He took the good van and did it again on Friday', tag: 'poor' },
        ],
        note: 'Fired 9 times here, and 8 of those went well enough that you stopped noticing it.',
      },
      money: {
        rows: [
          { time: 'Sat', title: 'Ilya asked for $40 until payday', detail: 'Nothing covered it, so it came to you in the street', tag: 'you' },
          { time: 'Tue', title: 'Ilya asked again', detail: 'Nothing covered it. You were late for the shift', tag: 'you' },
        ],
        note: 'Two interruptions in nine days. An answer here would have handled both without you.',
      },
      overtime: {
        rows: [
          { time: 'Mon', title: 'Accepted a late run', detail: '1.6&times; rate, four hours, home at one', tag: 'good' },
          { time: 'Wed', title: 'Refused a late run', detail: 'Flat rate. Marta put somebody else on it and said nothing', tag: 'good' },
          { time: 'Fri', title: 'Accepted a late run', detail: '1.5&times; rate. You slept five hours and the week never recovered', tag: 'poor' },
        ],
        note: 'The money says this answer works. Your sleep chart says something else.',
      },
    },
    quay: {
      help: {
        rows: [
          { time: 'Day 4', title: 'Said yes to Rurik', detail: 'Paid, legal, and he now introduces you as the one who does it', tag: 'poor' },
          { time: 'Day 9', title: 'Said yes to Petra', detail: 'Paid. She has asked twice since', tag: 'poor' },
        ],
        note: 'Two for two. The same answer went 22 for 31 on Tanner Row.',
      },
      voice: { rows: [], note: 'Nothing yet. It has not come up in eleven days, which is itself a fact about this place.' },
      money: { rows: [], note: 'Nothing yet. Nobody here has known you long enough to ask.' },
      overtime: {
        rows: [
          { time: 'Day 6', title: 'Refused a late night', detail: 'Kestrel &amp; Vane bill by the hour. Two people noticed', tag: 'poor' },
        ],
        note: 'One data point. One is noise.',
      },
    },
  };

  /* A conflict is two answers that both match the same moment. The client
     cannot resolve it for you; it can only say which one fires first. */
  const OVERLAP = {
    east: 'Your answer to 1 and your answer to 4 both match when Marta asks you to stay late ' +
      'as a favour. The first one checked wins, and that is 1.',
    quay: null,
  };

  /* What you arrived with. Tanner Row has been lived in; the Riverfront has
     one answer carried across and nothing else. */
  const IN_FORCE = {
    east: {
      help: { conds: [{ id: 'fee', v: 1 }, { id: 'short', v: 1 }], join: 'and',
              then: { id: 'yes' }, otherwise: { id: 'no' } },
      voice: { conds: [{ id: 'above' }], join: 'and',
               then: { id: 'quiet' }, otherwise: { id: 'finish' } },
      overtime: { conds: [{ id: 'rate', v: 1 }], join: 'and',
                  then: { id: 'accept' }, otherwise: { id: 'refuse' } },
    },
    quay: {
      help: { conds: [{ id: 'fee', v: 2 }, { id: 'legal' }], join: 'and',
              then: { id: 'yes' }, otherwise: { id: 'no' } },
    },
  };

  const clone = (x) => JSON.parse(JSON.stringify(x));
  const questionOf = (id) => QUESTIONS.filter((q) => q.id === id)[0] || QUESTIONS[0];
  const termOf = (list, id) => list.filter((x) => x.id === id)[0];

  /* An unanswered question starts as a readable sentence with one hole in
     it: WHEN ___ THEN say yes, otherwise ___. The first action is filled in
     because every one of these questions has an obvious opening move, and a
     sentence with three blanks in it is a form again. */
  const blank = (qId) => ({
    conds: [null], join: 'and',
    then: { id: questionOf(qId).acts[0].id }, otherwise: null,
  });

  /* A term with its threshold written in: "they are paying me above $60". */
  const label = (list, ref) => {
    const term = ref && termOf(list, ref.id);
    if (!term) return '';
    return term.param
      ? term.text.replace('{v}', VALUES[term.param].options[ref.v || 0])
      : term.text;
  };

  /* The condition side of a rule, read back rather than edited: one term,
     or two joined by the word the player picked. */
  const expression = (q, ans, size) => ans.conds.map((ref, i) =>
    (i ? Kw(ans.join) : '') +
    (ref ? Token({ label: label(q.conds, ref), size }) : Token({ label: '&mdash;', size }))
  ).join('');

  /* A rule is committable when nothing in the sentence is still a blank.
     Nothing partial ever runs. */
  const ready = (q, ans) =>
    ans.conds.every(Boolean) && !!(ans.then && ans.otherwise);

  const answered = (pb, set) =>
    QUESTIONS.filter((q) => ready(q, pb.answers[set][q.id])).length;

  /* app.js owns the mutable copy; this is the shape of it and the authored
     starting point.

     There is one tree, not a draft and a committed copy. An edit is the
     change — the player alters the sentence and that is what the character
     does from the next time the situation comes up. A commit step would only
     be honest if the player could sit and think in a paused world, and they
     cannot: the world is running behind this panel either way. What replaces
     the commit is the blank. A sentence with a hole in it simply is not
     covering the question, which the roster says out loud, so an unfinished
     edit fails safe instead of half-running. */
  E.PLAYBOOK = {
    questions: QUESTIONS,
    values: VALUES,
    sets: SETS,
    ready: ready,
    answered: answered,
    fresh: function (set) {
      const state = { set: set || 'east', q: 'help', menu: null, answers: {} };
      SETS.forEach(function (s) {
        state.answers[s.id] = {};
        QUESTIONS.forEach(function (q) {
          const held = (IN_FORCE[s.id] || {})[q.id];
          state.answers[s.id][q.id] = held ? clone(held) : blank(q.id);
        });
      });
      return state;
    },
  };

  /* The tab strip both playbook screens share: one per rule set, then the
     routines, which are the same object with a different subject. */
  const setTabs = (pb, active) => Tabs(
    SETS.map((s) => ({
      label: s.label + ' &middot; ' + answered(pb, s.id) + ' answered',
      active: active === s.id,
      act: 'env:' + s.id,
    })).concat([
      { label: 'Routines &middot; 2', active: active === 'routines', panel: 'routines' },
      { label: '+ new rule set', faint: true },
    ])
  );

  /* ─── the builder ─────────────────────────────────────────────────── */

  /* The whole editor is the sentence. Every part of it opens a short list of
     what that part could be, in place, under the word — so the rule stays
     readable while it is being changed, and there is never a form to fill in
     and submit. Two conditions is the ceiling for now. */

  const builder = (pb, set) => {
    const q = questionOf(pb.q);
    const ans = pb.answers[set.id][q.id];
    const running = ready(q, ans);
    const pair = ans.conds.length > 1;
    const showing = (target) => pb.menu === target;

    /* A term, written out, with its threshold left live inside it. Clicking
       the term changes which condition it is; clicking the number changes
       only the number. Two grains, one object. */
    const written = (list, ref, target) => {
      const term = termOf(list, ref.id);
      if (!term.param) return term.text;
      const cut = term.text.split('{v}');
      return cut[0] + Num({
        label: VALUES[term.param].options[ref.v || 0],
        act: 'open:num:' + target,
        menu: showing('num:' + target) ? Menu({
          items: VALUES[term.param].options.map((o, vi) => ({
            label: o, on: (ref.v || 0) === vi, act: 'value:' + target + ':' + vi,
          })),
        }) : null,
      }) + cut[1];
    };

    /* One part of the sentence, and the list it opens. `extra` is what this
       part can become rather than what it can say — growing a second
       condition, or dropping one. */
    const part = (target, kind, list, ref, extra) => Editable({
      kind: kind,
      empty: !ref,
      body: ref ? written(list, ref, target) : '',
      act: 'open:' + target,
      menu: showing(target) ? Menu({
        items: list.map((t) => ({
          label: label(list, ref && ref.id === t.id ? ref : { id: t.id }),
          on: !!ref && ref.id === t.id,
          act: 'put:' + target + ':' + t.id,
        })),
        extra: extra,
      }) : null,
    });

    /* The joint is the only word in the sentence that changes the logic, so
       it is the only piece of grammar that is also a control. */
    const joint = Editable({
      kind: 'joint',
      body: ans.join,
      act: 'open:join',
      menu: showing('join') ? Menu({
        items: [
          { label: '<b>and</b> &mdash; both have to be true', on: ans.join === 'and', act: 'join:and' },
          { label: '<b>or</b> &mdash; either one is enough', on: ans.join === 'or', act: 'join:or' },
        ],
        extra: [{ label: 'Drop the second condition', act: 'drop:c1' }],
      }) : null,
    });

    return Panel({
      title: 'Your answer',
      body: [
        // the stake only needs saying while nothing is covering the question
        Ask({ n: q.n, text: q.ask, note: running ? null : q.note }),

        Sentence([
          Clause([Kw('when'), part('c0', 'cond', q.conds, ans.conds[0], pair
            ? [{ label: 'Drop this condition', act: 'drop:c0' }]
            : [{ label: '&hellip; and another condition', act: 'add' }])]),
          pair ? Clause([joint, part('c1', 'cond', q.conds, ans.conds[1],
            [{ label: 'Drop this condition', act: 'drop:c1' }])]) : '',
          Clause([Kw('then'), part('then', 'act', q.acts, ans.then)]),
          Clause([Kw('otherwise'), part('else', 'act', q.acts, ans.otherwise)]),
        ]),

        // No save, and no instructions. The sentence is the rule, so changing
        // it is the change; the only line worth printing is the one saying
        // the rule is not covering anything yet.
        running ? '' : Note('While there is a blank in it, nothing is covering this question, ' +
          'and it comes to you instead.', { last: true }),
      ],
    });
  };

  /* ─── the standing questions, and what the answers have done ──────── */

  const roster = (pb, set) => Panel({
    title: 'Standing questions',
    body: QUESTIONS.map((q) => {
      const ans = pb.answers[set.id][q.id];
      const running = ready(q, ans);
      return Question({
        n: q.n,
        ask: q.ask,
        answer: running
          ? expression(q, ans, 'small') + Kw('then') +
            Token({ label: label(q.acts, ans.then), kind: 'act', size: 'small' })
          : '<i>no answer &mdash; every one of these comes to you instead</i>',
        // the one accent on this screen, and it marks the only state that
        // wants the player: a question nothing is covering
        state: running ? { label: 'In force' } : { label: 'Open', variant: 'accent' },
        active: q.id === pb.q,
        act: 'q:' + q.id,
      });
    }),
  });

  const evidence = (pb, set) => {
    const q = questionOf(pb.q);
    const ev = (EVIDENCE[set.id] || {})[q.id] || { rows: [], note: '' };
    const TONE = { good: {}, poor: { variant: 'danger' }, you: { variant: 'accent' } };
    return Panel({
      title: 'What answer ' + q.n + ' has done here',
      note: ev.rows.length ? 'last nine days' : '&mdash;',
      body: [
        ev.rows.length
          ? ev.rows.map((r) => LogRow({
              time: r.time, title: r.title, detail: r.detail,
              trailing: Tag(Object.assign({ label: r.tag === 'you' ? 'came to you' : r.tag },
                                          TONE[r.tag] || {})),
            })).join('')
          : Prose('Nothing has happened here yet that this answer covers.', { first: true }),
        Note(ev.note, { last: true, style: 'margin-top:12px' }),
      ],
    });
  };

  const playbook = (pb) => {
    const set = SETS.filter((s) => s.id === pb.set)[0] || SETS[0];
    return setTabs(pb, set.id) +
      Grid({ variant: 'wide-left', children: [
        builder(pb, set),
        Stack([
          roster(pb, set),
          OVERLAP[set.id] ? Notice({ text: OVERLAP[set.id] }) : '',
          evidence(pb, set),
        ]),
      ] });
  };


  /* ═══════════════════════════════════════════════════ SOMEBODY IN THE STREET

     Everybody out there is somebody, so pointing at any of them opens the
     same screen: who they are, what the boards say about them, and the short
     closed list of things you can ask.

     The questions are theirs rather than yours, and each has exactly one
     authored answer (§7 — nothing in this client is generated). That is the
     whole reason the list is short: three questions a person can actually
     answer beats a text box that would have to invent something.

     Nothing here costs you anything and nothing is waiting on you. Asking is
     reading — the closest thing in the client to looking somebody up, except
     that you had to walk over and they had to be a person first. */

  const dossier = (who) => Stack([
    Panel({ title: 'What the boards say', body: [
      Readout([
        Stat({ value: commas(who.karma), unit: 'Karma', accent: true }),
        Stat({ value: who.level, unit: 'Level' }),
      ]),
      Note('Level is what the boards round somebody to in public. Karma is the ' +
           'number underneath it, and it is the same composite you are scored on.',
           { last: true }),
    ] }),
    Panel({ title: 'Who they are', body: [
      Prose(who.story, { first: true, last: true }),
    ] }),
  ]);

  const conversation = (who, asked) =>
    Panel({ title: 'What you could ask', note: who.ask.length + ' &middot; nothing costs you anything', body:
      [Note('They will answer any of these the same way tomorrow. People here are ' +
            'not vending machines for advice &mdash; what you get is what they think.',
            { first: true })]
        .concat(who.ask.map((x, i) =>
          Option({ lead: 'Ask', text: x.q, chosen: i === asked, ask: who.id + ':' + i }) +
          (i === asked ? Reply({ name: who.name + ' &middot; ' + who.handle, text: x.a }) : '')))
    });

  E.PANELS = {

    /* ------------------------------------------------------------ person */
    person: {
      title: (s) => (s.who ? s.who.name : ''),
      sub: (s) => (s.who ? s.who.role + ' &middot; ' + s.who.note : ''),
      html: ({ who, talk }) => who ? Grid({ variant: 'wide-left', children: [
        conversation(who, talk && talk.who === who.id ? talk.asked : -1),
        Stack([
          Mirror({ name: who.handle,
                   note: 'The same figure you were just looking at in the street.' }),
          dossier(who),
        ]),
      ] }) : '',
    },

    /* ------------------------------------------------------------ closet
       The one panel whose subject is the player rather than a number about
       them, and the only one you edit by looking at the result: you stand in
       the middle of it and the drawers are along the bottom, within reach.

       Every pick is on the figure in the frame it is clicked, because there
       is nothing else for it to be. No preview, no confirm, no save — the
       person walking around behind the scrim is already wearing it, and
       closing this is walking away rather than committing. */
    closet: {
      title: 'The closet',
      sub: 'A coat, whatever the last place left you with, and a name the boards will print.',
      html: ({ look, closet }) => {
        const open = (closet && closet.slot) || DRAWERS[0].slot;
        const walking = !!(closet && closet.pose === 'walk');
        return Wardrobe({
          mirror: Mirror({
            name: look.name,
            /* Standing is how you check a colour; walking is how anybody else
               will ever see it. Both are the street's own figure, not a
               dressed-up version of it. */
            controls: ButtonRow([
              Button({ label: 'Standing', size: 'small',
                       variant: walking ? 'ghost' : 'primary', pose: 'stand' }),
              Button({ label: 'Walking', size: 'small',
                       variant: walking ? 'primary' : 'ghost', pose: 'walk' }),
            ]),
            note: 'This is the size everybody else sees you at. Silhouette, not detail.',
          }),
          rack: Rack({
            tabs: DRAWERS.map((d) => {
              const o = worn(d, look);
              return RackTab({
                label: d.label, worn: o.label, color: o.color,
                on: d.slot === open, slot: d.slot,
              });
            }),
            drawers: DRAWERS.map((d) => RackDrawer({
              drawer: d.slot, on: d.slot === open, children: [
                Swatches(d.options.map((o) => Swatch({
                  label: o.label, color: o.color,
                  on: (worth(d, o) || null) === (look[d.slot] || null),
                  pick: d.slot + ':' + worth(d, o),
                }))),
                Note(d.note, { last: true, style: 'margin-top:12px' }),
              ],
            })),
            note: 'Everything in these drawers is something you already own. ' +
                  'What you can buy is on the other side of money.',
          }),
        });
      },
    },

    /* ------------------------------------------------------------ health */
    health: {
      title: 'Health',
      sub: 'What you can see is a function of what you own, not of how healthy you are.',
      html:
        Grid({ variant: 'wide-left', children: [
          Panel({ title: 'Baseline', note: 'free &middot; from the start', body: [
            Readout([
              Stat({ value: '31', unit: 'Biological age', accent: true }),
              Note('Derived from sleep, resting heart rate and weight.'),
            ]),
            Metric({ name: 'Sleep', sub: '5.6 h average over 14 nights &middot; falling',
                     value: '5.6 h', tone: 'warn', pct: 52, barTone: 'warn' }),
            Sparkline({
              points: [[8, 23.6], [28, 31.4], [48, 41.8], [68, 35.3], [88, 26.2], [108, 19.7],
                       [128, 44.4], [148, 48.3], [168, 40.5], [188, 34], [208, 45.7],
                       [228, 50.9], [248, 43.1], [268, 32.7]],
              target: 14.5, targetLabel: '7.5 h target', mark: [228, 50.9],
              from: '14 nights ago', to: 'last night',
            }),
            Metric({ name: 'Resting heart rate', sub: 'Up 6 bpm on your own baseline',
                     value: '68 bpm', tone: 'warn', pct: 66, barTone: 'warn' }),
            Metric({ name: 'Weight trend', sub: 'Stable across 30 days',
                     value: '&plusmn;0.4 kg', tone: 'ok', pct: 88, barTone: 'ok' }),
          ] }),

          Stack([
            Panel({ title: 'Wearable tier', tag: { label: 'Locked' }, body: [
              Note('Five metrics your body is already producing. You have no way to read them.', { first: true }),
              Slots([
                LockedSlot({ name: 'HRV', why: 'no reader' }),
                LockedSlot({ name: 'SLEEP&nbsp;STAGES', why: 'no reader' }),
                LockedSlot({ name: 'RECOVERY', why: 'no reader' }),
                LockedSlot({ name: 'VO₂&nbsp;MAX', why: 'no reader' }),
                LockedSlot({ name: 'ACTIVITY&nbsp;LOAD', why: 'no reader' }),
              ]),
              ButtonRow([Button({ label: 'Fenwick GT4 &mdash; $340', variant: 'primary' })]),
              Note('It reads what your body is already doing.', { last: true }),
            ] }),

            Panel({ title: 'Clinical tier', tag: { label: 'Locked' }, body: [
              Slots([
                LockedSlot({ name: 'GLUCOSE', why: 'bloodwork' }),
                LockedSlot({ name: 'INFLAMMATION', why: 'bloodwork' }),
                LockedSlot({ name: 'LIPIDS', why: 'bloodwork' }),
                LockedSlot({ name: 'HORMONAL', why: 'bloodwork' }),
              ]),
              ButtonRow([Button({ label: 'Rowan St Clinic &mdash; $180' })]),
              Note('Closed until Monday.', { last: true }),
            ] }),
          ]),
        ] })
    },

    /* ------------------------------------------------------------- karma */
    karma: {
      title: 'Karma',
      sub: 'There is no level. This is a mix of several things about you, two of which you can see.',
      html:
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ bodyClass: 'readout', body: [
              Stat({ value: '12', unit: 'Karma', accent: true }),
              `<div style="flex:1">` +
                Metric({ name: 'This week', value: '+2', pct: 14, plain: true }) +
                Metric({ name: 'Four weeks ago', value: '+9', tone: 'faint',
                         pct: 92, barOpacity: '.4', plain: true }) +
                Note('Same effort. See <a data-panel="travel" href="#">where you live</a>.', { last: true }) +
              `</div>`,
            ] }),

            Panel({ title: 'What&rsquo;s in it', note: '6 parts &middot; 2 legible', body: [
              Metric({ name: 'Health', sub: 'Every biomarker, collapsed into biological age',
                       value: '31', tone: 'accent', pct: 61 }),
              Metric({ name: 'Money', sub: 'What you hold', value: '$4,212', pct: 28 }),
              Slots([
                LockedSlot({ name: '▒▒▒▒▒▒▒▒', why: 'moved four days ago' }),
                LockedSlot({ name: '▒▒▒▒▒▒', why: 'moved yesterday' }),
                LockedSlot({ name: '▒▒▒▒▒▒▒▒▒▒', why: 'hasn&rsquo;t moved in three weeks' }),
                LockedSlot({ name: '▒▒▒▒▒', why: '&mdash;', dim: true }),
              ], { style: 'margin-top:12px' }),
              Note('They are all real, and they are all counted.', { last: true }),
            ] }),
          ]),

          Stack([
            Panel({ title: 'Boards', note: 'one per part, plus the mix', body: [
              Tabs([
                { label: 'Overall', active: true },
                { label: 'Health' },
                { label: 'Money' },
                { label: '▒▒▒', faint: true },
                { label: '▒▒▒', faint: true },
              ]),
              BoardRow({ rank: 1, name: 'tsvetan_k', where: 'Riverfront', value: '9,104' }),
              BoardRow({ rank: 2, name: 'mor_shalev', where: 'Downtown', value: '8,877' }),
              BoardRow({ rank: 3, name: 'quietferry', where: 'the port road', value: '8,640' }),
              BoardRow({ rank: '&hellip;', variant: 'gap' }),
              BoardRow({ rank: '2,417', name: 'you', where: 'Tanner Row &middot; up 2 this week',
                         value: '12', tone: 'accent', variant: 'you' }),
              BoardRow({ rank: '2,418', name: 'halcyon_r', where: 'Tanner Row', value: '11' }),
            ] }),

            Panel({ title: 'Someone you met', body: [
              Prose('<b>halcyon_r</b> covered your shift at the port yard on Tuesday.', { first: true }),
              ButtonRow([Button({ label: '◆ I like this player', variant: 'primary' })]),
              Note('You get one of these every few days. Spend it on someone who earned it.', { last: true }),
            ] }),
          ]),
        ] })
    },

    /* ---------------------------------------------------------- playbook */
    /* Rules for other people and rules for your own body are the same object
       with the same interface, so they are one screen with one way in: the
       board on your wall, or P. Routines is a tab here, never a destination
       of its own.

       This is the only panel the player can change from, which is deliberate:
       the playbook is where a session's thinking happens, and everywhere else
       in the client is either the world or a readout of it. */
    playbook: {
      title: 'Playbook',
      sub: 'How your character behaves day to day.',
      html: ({ pb }) => playbook(pb),
    },

    /* --------------------------------------------------------- conflicts */
    conflicts: {
      title: 'Today, among these people',
      sub: 'Friction follows from who owes whom. Your answers cover most of it; the gaps come to you.',
      html:
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Handled without you', note: '4 of 5', body: [
              LogRow({ time: '07:20', title: 'Ilya asked you to move a delivery again',
                detail: Token({ label: 'someone asks me for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say yes', kind: 'act', size: 'small' }) + ' &middot; $80 and ninety minutes, so both halves matched',
                trailing: Tag({ label: 'answer 1' }) }),
              LogRow({ time: '11:05', title: 'Marta raised her voice about the roster',
                detail: Token({ label: 'someone raises their voice at me', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'stay quiet', kind: 'act', size: 'small' }) + ' &middot; she apologised afterwards',
                trailing: Tag({ label: 'answer 2' }) }),
              LogRow({ time: '14:40', title: 'Kez wanted a hand with the good van',
                detail: Token({ label: 'someone asks me for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say no', kind: 'act', size: 'small' }) + ' &middot; unpaid and four hours, so it fell through to the otherwise',
                trailing: Tag({ label: 'answer 1' }) }),
              LogRow({ time: '16:30', title: 'A late run came up at the port',
                detail: Token({ label: 'I am asked to work overtime', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'accept it', kind: 'act', size: 'small' }) + ' &middot; 1.6&times; rate, and you are home at one again',
                trailing: Tag({ label: 'answer 4' }) }),
            ] }),

            Panel({ title: 'Nothing covers this one', tag: { label: 'Needs you', variant: 'accent' }, body: [
              Prose('Ilya asked you for $40 until payday, in the street, in front of Kez. He has ' +
                'asked twice this month and paid you back once.', { first: true }),
              Note('You have no answer to question 3, so this one waited for you to arrive.'),
              ButtonRow([
                Button({ label: 'Give him the $40', variant: 'primary' }),
                Button({ label: 'Tell him no' }),
                Button({ label: 'Answer question 3', variant: 'ghost', panel: 'playbook' }),
              ]),
            ] }),
          ]),

          Panel({ title: 'Where you stand', note: 'Tanner Row', body: [
            Metric({ name: 'Ilya', sub: 'Owes you three &middot; asks constantly', value: 'warm', pct: 74 }),
            Metric({ name: 'Marta', sub: 'Trust down since Tuesday', value: 'cooling', tone: 'warn', pct: 38, barTone: 'warn' }),
            Metric({ name: 'Kez', sub: 'Even', value: 'steady', tone: 'ok', pct: 60, barTone: 'ok' }),
            Metric({ name: 'Dana', sub: 'You have not spoken in two weeks', value: 'faded', pct: 22 }),
            Note('These four generate most of what happens to you here.', { last: true, style: 'margin-top:12px' }),
          ] }),
        ] })
    },

    /* ------------------------------------------------------------ travel */
    travel: {
      title: 'Where you live',
      sub: 'Wilmington, Delaware. You live in one place at a time, and the 6 runs once a day.',
      html:
        When('east', Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Karma from Tanner Row', note: 'per week', body: [
              DecayRow({ label: 'week 1', pct: 100, amount: '+6' }),
              DecayRow({ label: 'week 2', pct: 75, amount: '+4.5' }),
              DecayRow({ label: 'week 3', pct: 40, amount: '+2.4' }),
              DecayRow({ label: 'week 4', pct: 17, amount: '+1' }),
              DecayRow({ label: 'week 5', pct: 6, amount: '+0.4' }),
              DecayRow({ label: 'this week', pct: 3, amount: '+0.2' }),
              Note('You are doing the same amount of work as in week one.', { last: true, style: 'margin-top:12px' }),
            ] }),

            Panel({ title: 'Where the ceiling is', body: [
              Metric({ name: 'Money', sub: 'The port pays what the port pays',
                       value: 'unchanged 6 wks', tone: 'warn', pct: 100, barTone: 'warn' }),
              LockedSlot({ name: '▒▒▒▒▒▒▒▒▒▒', why: 'hasn&rsquo;t moved in three weeks' }),
              LockedSlot({ name: '▒▒▒▒▒▒▒▒', why: 'moves less each week' }),
              Note('You know everyone here, and you know what they will do.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),

          Stack([
            Panel({ title: 'Dana, on the phone', body: [
              Speech('&ldquo;You&rsquo;re still on Rowan St? There&rsquo;s a room going over on the ' +
                'Riverfront side. Different crowd &mdash; you&rsquo;d hate half of them. Rent&rsquo;s ' +
                'worse. Come and see it.&rdquo;', { size: 'small', first: true }),
              ButtonRow([
                Button({ label: 'Take the 6 over', variant: 'primary', move: 'quay' }),
                Button({ label: 'Not today' }),
              ]),
              Note('A mile and a half across the Christina. Twenty minutes on the 6, and you would ' +
                'be living there &mdash; your things, your job, your mornings.', { last: true }),
            ] }),

            Panel({ title: 'What you would be taking', body: [
              Note('Your Tanner Row answers, and the actions they fire.', { first: true }),
              Tokens([
                Token({ label: 'say yes', kind: 'act', size: 'small' }),
                Token({ label: 'stay quiet', kind: 'act', size: 'small' }),
                Token({ label: 'accept it', kind: 'act', size: 'small' }),
              ]),
              Note('Three answers, refined over six weeks. They work here, and here is the only ' +
                'place they have ever been tested.', { last: true, style: 'margin-top:14px' }),
            ] }),
          ]),
        ] })) +

        When('quay', Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Eleven days on the Riverfront', body: [
              LogRow({ time: 'Day 2', title: 'Said yes to Petra&rsquo;s Saturday',
                detail: Token({ label: 'someone asks me for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say yes', kind: 'act', size: 'small' }) + ' &middot; paid and legal, so it matched. She took the next one too',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              LogRow({ time: 'Day 6', title: 'Refused a late night at Kestrel &amp; Vane',
                detail: Token({ label: 'I am asked to work overtime', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'nothing &mdash; unanswered here', size: 'small' }) + ' &middot; it came to you, and you said no on the spot',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              LogRow({ time: 'Day 9', title: 'Said yes to Rurik twice in a week',
                detail: Token({ label: 'someone asks me for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say yes', kind: 'act', size: 'small' }) + ' &middot; he now introduces you as the one who does it',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              Note('Six of the eleven days. Your answer to 1 each time, and it is the answer that ' +
                'went 22 for 31 on the other side of the river.', { last: true, style: 'margin-top:12px' }),
            ] }),

            Panel({ title: 'What this place pays', body: [
              Metric({ name: 'Money', sub: 'Kestrel &amp; Vane bill by the hour and everyone knows it',
                       value: '$5,900 within reach', tone: 'ok', pct: 74, barTone: 'ok' }),
              LockedSlot({ name: '▒▒▒▒▒▒▒▒▒▒', why: 'moved twice this week' }),
              LockedSlot({ name: '▒▒▒▒▒▒▒▒', why: 'moved yesterday' }),
              Note('Nobody here has known you long enough to owe you anything.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),

          Stack([
            Panel({ title: 'Your old room', body: [
              Speech('Marta: &ldquo;3B&rsquo;s still empty if you want it. Ilya asks about you. ' +
                'I&rsquo;d have you back on the roster Monday.&rdquo;', { size: 'small', first: true }),
              ButtonRow([
                Button({ label: 'Take the 6 back', variant: 'primary', move: 'east' }),
                Button({ label: 'Stay' }),
              ]),
              Note('The answers you are building here would come with you.', { last: true }),
            ] }),

            Panel({ title: 'The set you are building', tag: { label: '1 answered', variant: 'accent' }, body: [
              Rule({ about: 'someone asks me for help',
                     expr: Token({ label: 'they are paying me above $150' }) + Kw('and') +
                           Token({ label: 'the task is legal' }),
                     then: 'say yes', otherwise: 'say no',
                     meta: ['fired 2&times;', { text: 'outcome poor 2&times;', tone: 'danger' }],
                     style: 'margin:0' }),
              Note('You carried it across on day one and tightened the price on day four. ' +
                'It has failed both times since.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),
        ] }))
    },

    /* ---------------------------------------------------------- routines */
    routines: {
      title: 'Playbook',
      sub: 'Rules for the body. Same object as the rest of the playbook, same interface.',
      html: ({ pb }) =>
        setTabs(pb, 'routines') +
        Grid({ variant: 'two', children: [
          Panel({ title: 'Configured', body: [
            Rule({ when: ['it reaches 12:30 AM'], then: 'go to bed',
                   meta: ['held 9 of the last 14 nights'] }),
            Rule({ when: ['it is Mon / Wed / Sat'], then: 'train for 45 minutes',
                   meta: ['held 12 of the last 12'] }),
            Note('Set once. You should not have to think about these again.', { last: true }),
          ] }),

          Panel({ title: 'Not offered yet', body: [
            Slots([
              LockedSlot({ name: 'DIET', why: '&mdash;' }),
              LockedSlot({ name: 'ORGANISATION', why: '&mdash;' }),
            ]),
            Notice({ tone: 'cool', text: 'You have missed two shifts in a fortnight and both times ' +
              'you thought you had the day off. There is someone at the port who has seen this before.' }),
            ButtonRow([Button({ label: 'Go and talk to Marta', variant: 'primary', panel: 'diagnosis' })]),
          ] }),
        ] })
    },

    /* --------------------------------------------------------- diagnosis */
    diagnosis: {
      title: 'The port, back office',
      sub: '',
      scene: true,
      html:
        Dialogue({ who: 'marta', name: 'Marta', role: 'Runs the roster', children: [
          Speech('&ldquo;Second time this month. I&rsquo;m not writing you up, I want to know what ' +
            'you think is going on. You&rsquo;ve got all of it in front of you &mdash; show me.&rdquo;'),
          Note('Point at what you think caused it. Your own records, not a list of answers.',
               { style: 'margin:-6px 0 14px' }),
          Chips([
            Chip({ src: 'Sleep log &middot; Tue', val: '4.7 h', picked: true }),
            Chip({ src: 'Sleep log &middot; 14-night avg', val: '5.6 h' }),
            Chip({ src: 'Calendar &middot; Tue', val: 'no shift entered', picked: true }),
            Chip({ src: 'Answer 4 fired', val: 'accept it' }),
            Chip({ src: 'Resting HR', val: '68 bpm, up 6' }),
            Chip({ src: 'Ilya, Mon', val: 'asked you late' }),
            Chip({ src: 'Money &middot; Tue', val: 'paid actions 3 / 3' }),
            Chip({ src: 'Walked', val: '11 km, Mon' }),
          ]),
          Commit([
            Note('Two picked. What you are saying is that you never wrote it down and you were too ' +
              'tired to notice.', { first: true }),
            ButtonRow([
              Button({ label: 'That&rsquo;s what happened', variant: 'primary' }),
              Button({ label: 'Pick something else', variant: 'ghost' }),
            ]),
          ]),
          Note('You can come back to this when you know more.', { style: 'margin-top:14px' }),
        ] })
    },

    /* --------------------------------------------------------- challenge */
    challenge: {
      title: '',
      sub: '',
      scene: true,
      html:
        SceneHead({
          tag: { label: 'Resolve a conflict', variant: 'accent' },
          stages: ['done', 'done', 'now', null],
          note: 'about 6 minutes &middot; once ever',
        }) +
        Dialogue({ who: 'nadia', name: 'Nadia', role: 'Outside the Verona', children: [
          Speech('&ldquo;He sold the van. Not <em>our</em> van, his van, he keeps saying &mdash; ' +
            'except I paid half of it and he knows I did. And now he&rsquo;s standing there telling ' +
            'me I never asked for anything in writing, like that&rsquo;s the part that matters.&rdquo;'),
          Speech('Tomas, quieter: &ldquo;She was going to leave. I&rsquo;m not going to pretend I ' +
            'didn&rsquo;t know that when I did it.&rdquo;', { size: 'small', variant: 'aside' }),
          Divider(),
          Note('You have heard enough of it. Pick the reading you are going to act on.'),
          Option({ chosen: true, text: 'This is about the money. Whatever else is going on, she is ' +
            'owed half of what the van sold for, and that is a number they can both look at.' }),
          Option({ text: 'He sold it because he thought she was leaving. Neither of them has said ' +
            'the actual thing out loud yet, and the van is standing in for it.' }),
          Option({ text: 'She has been keeping score for a long time and this is the first item ' +
            'large enough to say aloud. Settling the van will not settle anything.' }),
          ButtonRow([
            Button({ label: 'Say it', variant: 'primary' }),
            Button({ label: 'Say nothing and walk on', variant: 'ghost' }),
          ], { style: 'margin-top:14px' }),
        ] })
    },

    /* -------------------------------------------------------------- work */
    work: {
      title: 'Port &middot; dispatch coordinator',
      sub: 'Tier 2. It runs while you are elsewhere, and you read back what your answers did.',
      html:
        Tabs([
          { label: 'Your role', active: true },
          { label: 'Other work &middot; 5 postings', panel: 'jobs' },
        ]) +
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'While you were out', note: 'since 07:00', body: [
              LogRow({ time: '09:12', title: 'Driver called in sick, two runs uncovered',
                detail: 'Answer 1 fired &middot; you took one yourself &middot; both runs went out',
                trailing: Tag({ label: 'held', variant: 'success' }) }),
              LogRow({ time: '13:30', title: 'Overtime request from Kez',
                detail: 'Answer 4 fired &middot; approved &middot; $180 over budget for the week',
                trailing: Tag({ label: 'cost' }) }),
              LogRow({ time: '16:02', title: 'Marta asked who authorised the overtime',
                detail: 'Nothing matched &middot; the character said &ldquo;I&rsquo;ll look into it&rdquo;',
                trailing: Tag({ label: 'gap', variant: 'danger' }) }),
            ] }),

            Panel({ title: 'The ladder', body: [
              LadderRow({ tier: 1, title: 'Warehouse nights',
                detail: '22:00&ndash;06:00, five on, two off. Loading bay.',
                trailing: Tag({ label: 'done', variant: 'dim' }) }),
              LadderRow({ tier: 2, title: 'Dispatch coordinator', variant: 'current',
                detail: 'Phone on. Two or three calls a shift.',
                trailing: Tag({ label: 'now &middot; $2,240 / mo', variant: 'accent' }) }),
              LadderRow({ tier: 3, title: 'Operations manager', variant: 'future',
                detail: 'Staffing, budget, and whoever just quit',
                trailing: Tag({ label: 'interview' }) }),
              LadderRow({ tier: 4, title: 'Regional director', variant: 'future',
                detail: 'Quarterly numbers. You sign things.',
                trailing: Tag({ label: '&mdash;', variant: 'dim' }) }),
              LadderRow({ tier: 5, title: '&mdash;', variant: 'future', detail: '&nbsp;',
                trailing: Tag({ label: '&mdash;', variant: 'dim' }) }),
              Note('Fenn posts a tier 3 about twice a year.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),

          Panel({ title: 'Standing', body: [
            Metric({ name: 'Runs out on time', sub: 'Last 30 days', value: '94%', tone: 'ok', pct: 94, barTone: 'ok' }),
            Metric({ name: 'Overtime budget', sub: 'Over by $180', value: 'over', tone: 'warn', pct: 118, barTone: 'warn' }),
            Metric({ name: 'How Marta reads you', sub: 'Down since Tuesday', value: 'cooling', tone: 'warn', pct: 38, barTone: 'warn' }),
            Divider(),
            Note('The tier 3 interview exists because you applied. It is a challenge like any other ' +
              '&mdash; you will get through it or you will not.'),
            ButtonRow([Button({ label: 'See what else is going', panel: 'jobs' })]),
          ] }),
        ] })
    },

    /* -------------------------------------------------------------- jobs */
    jobs: {
      title: 'Other work',
      sub: 'Nothing here is recommended to you. These are the postings that exist.',
      html:
        Tabs([
          { label: 'Your role', panel: 'work' },
          { label: 'Other work &middot; 5 postings', active: true },
        ]) +
        Grid({ variant: 'wide-left', children: [
          `<div>` +
            Posting({
              role: 'Operations manager', where: 'Fenn Logistics &middot; the port &middot; tier 3',
              pay: '$4,100', payNote: '/ mo', hours: 'staffing, budget, and whoever just quit',
              reqs: [
                { state: 'met', label: '✓ two years on the floor' },
                { state: 'unmet', label: 'someone senior to vouch for you &mdash; Marta, cooling' },
                { state: 'hidden', label: '▒▒▒▒▒▒▒▒ &mdash; not high enough' },
              ],
              foot: 'You applied on Tuesday. Closes Friday. There will be an interview.',
              action: Button({ label: 'Interview pending', variant: 'spent', size: 'small' }),
            }) +
            Posting({
              role: 'Night dispatch', where: 'Corrin Freight &middot; the port road &middot; tier 2',
              pay: '$2,380', payNote: '/ mo', hours: '22:00&ndash;06:00, Sunday to Thursday',
              reqs: [
                { state: 'met', label: '✓ dispatch experience' },
                { state: 'met', label: '✓ own transport' },
              ],
              foot: 'Your sleep routine runs 00:30 to 06:15. You have held it nine of the last fourteen nights.',
              action: Button({ label: 'Apply', size: 'small' }),
            }) +
            Posting({
              role: 'Assistant to the desk', where: 'Kestrel &amp; Vane &middot; the Riverfront &middot; tier 3',
              pay: '$5,900', payNote: '/ mo', hours: 'on call; they expect a reply inside the hour',
              reqs: [
                { state: 'met', label: '✓ they have met you' },
                { state: 'unmet', label: 'you do not live on that side of the river' },
                { state: 'hidden', label: '▒▒▒▒▒▒ &mdash; unknown' },
              ],
              foot: 'Nothing on Rowan St pays this. Nothing on Rowan St is going to.',
              action: Button({ label: 'You would have to move first', size: 'small', panel: 'travel' }),
            }) +
            Posting({
              role: 'Bar work, evenings', where: 'Verona Diner &middot; Rowan St &middot; tier 1',
              pay: '$1,480', payNote: '/ mo + tips', hours: '17:00&ndash;01:00, Thursday to Sunday',
              reqs: [{ state: 'met', label: '✓ nothing, really' }],
              foot: 'Nadia asked whether you wanted it. Tips are earnings, and earnings still cap at three a day.',
              action: Button({ label: 'Apply', size: 'small' }),
            }) +
            Posting({
              role: 'Warehouse floor', where: 'Corrin Freight &middot; the port road &middot; tier 1',
              pay: '$1,640', payNote: '/ mo', hours: '06:00&ndash;14:00, five on, two off',
              reqs: [{ state: 'met', label: '✓ you have done this one before' }],
              foot: 'You did two years of this. Nobody ever called you after two.',
              action: Button({ label: 'Apply', size: 'small' }),
            }) +
          `</div>`,

          Stack([
            Panel({ title: 'Where you are now', tag: { label: 'tier 2', variant: 'accent' }, body: [
              Prose('<b>Dispatch coordinator</b>',
                    { sub: 'Fenn Logistics &middot; fourteen months', first: true }),
              Metric({ name: 'Pay', sub: 'Structural &mdash; it follows the tier, not the hours',
                       value: '$2,240 / mo', pct: 38 }),
              Metric({ name: 'Runs out on time', sub: 'Last 30 days', value: '94%', tone: 'ok', pct: 94, barTone: 'ok' }),
              Metric({ name: 'How Marta reads you', sub: 'Down since Tuesday', value: 'cooling', tone: 'warn', pct: 38, barTone: 'warn' }),
            ] }),

            Panel({ title: 'What leaving actually costs', body: [
              Prose('Fourteen months of standing goes to nothing. You start the new place unknown.',
                    { size: 'tiny', first: true }),
              Prose('Ilya owes you three. Those do not transfer.', { size: 'tiny' }),
              Prose('Your playbook comes with you &mdash; it is scoped to these people, not to this ' +
                'job. Move to the Riverfront and that is a different matter.', { size: 'tiny' }),
              Prose('A new role takes a couple of weeks before it starts generating anything worth ' +
                'reading back.', { size: 'tiny', last: true }),
            ] }),

            Panel({ title: 'The trade', note: 'what the week looks like', body: [
              DecayRow({ label: 'tier 1', pct: 8, amount: 'off at 14:00' }),
              DecayRow({ label: 'tier 2', pct: 24, amount: 'phone on' }),
              DecayRow({ label: 'tier 3', pct: 58, amount: 'called at home' }),
              DecayRow({ label: 'tier 4', pct: 82, amount: 'away Tue&ndash;Thu' }),
              DecayRow({ label: 'tier 5', pct: 100, amount: 'no line at all' }),
              Note('What people holding each of these said when asked about their week.',
                   { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),
        ] })
    },

    /* ------------------------------------------------------------- inbox */
    inbox: {
      title: 'Inbox',
      sub: 'The world got in touch. Nothing here is a quest list &mdash; it is what came to you.',
      html:
        Panel({ body: [
          LogRow({ time: '18:40', title: 'Dana called and you missed it',
            detail: 'She left a message about a room on the Riverfront side',
            trailing: Button({ label: 'Listen', size: 'small', panel: 'travel' }) }),
          LogRow({ time: '16:02', title: 'Marta wants to know who authorised the overtime',
            detail: 'Your character said he would look into it. He has not.',
            trailing: Button({ label: 'Open', size: 'small', panel: 'work' }) }),
          LogRow({ time: '11:15', title: 'The port posted the operations manager role',
            detail: 'Closes Friday',
            trailing: Button({ label: 'Open', size: 'small', panel: 'work' }) }),
          LogRow({ time: 'Yesterday', title: 'Ilya asked for Thursday as well', dim: true,
            detail: 'You walked past this one. It is still here.',
            trailing: Tag({ label: 'declined', variant: 'dim' }) }),
        ] })
    },
  };

})(window.ETM);
