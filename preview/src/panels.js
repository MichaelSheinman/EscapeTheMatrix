/* Escape the Matrix — overlay panels.
   These sit above the canvas and never replace it: the world stays visible and
   dimmed behind them (technical.md §10.1). Static content — nothing computes.

   Every visual decision here comes from @etm/ui. This file contributes copy
   and arrangement only; if you find yourself writing a class name or a colour
   below, the component you need is missing from the library. */

import {
  Button, ButtonRow, Tag, Token, Tokens,
  Panel, Stack, Grid, Readout, Stat, Metric, Sparkline,
  LockedSlot, Slots, Notice, Footnote, Tabs, Note, Prose, Divider,
  LogRow, BoardRow, LadderRow, DecayRow, Posting,
  Rule, SceneHead, Dialogue, Speech, Option, Chip, Chips, Commit,
  Swatch, Swatches,
} from '../../ui/index.js';

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  /* An environment-scoped block: shown only when the player lives there.
     app.js toggles these on `data-when` at open time. */
  const When = (where, children) =>
    `<div data-when="${where}">${children}</div>`;

  /* The closet's vocabulary. Authored, closed and small, for the same reason
     the rule composer's is (§3.4): the player picks from what exists. Nothing
     here is bought yet — this is the drawer you arrived with. */
  const WEAR = [
    { slot: 'coat', label: 'Coat', options: [
      { label: 'Charcoal', color: '#38384a' },
      { label: 'Rust', color: '#8c4a34' },
      { label: 'Moss', color: '#4b5f4a' },
      { label: 'Slate', color: '#3f5570' },
      { label: 'Sand', color: '#97845f' },
    ] },
    { slot: 'hair', label: 'Hair', options: [
      { label: 'Black', color: '#1f1a17' },
      { label: 'Auburn', color: '#5e3220' },
      { label: 'Ash', color: '#6b6459' },
      { label: 'Fair', color: '#ae8a56' },
    ] },
    { slot: 'hat', label: 'Hat', options: [
      { label: 'None' },
      { label: 'Flat cap', color: '#34302a' },
      { label: 'Watch cap', color: '#4e5c68' },
    ] },
    { slot: 'bag', label: 'Bag', options: [
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

  E.PANELS = {

    /* ------------------------------------------------------------ closet
       The one panel that is a function of state rather than a fixed string:
       what you are wearing is the whole content, so the client hands it in. */
    closet: {
      title: 'The closet',
      sub: 'A coat, whatever the last place left you with, and a name the boards will print.',
      html: (look) =>
        Grid({ variant: 'wide-left', children: [
          Panel({ title: 'What you wear', note: 'silhouette, not detail', body:
            WEAR.map(({ slot, label, options }, i) =>
              Note(label, { first: i === 0 }) +
              Swatches(options.map((o) => Swatch({
                label: o.label,
                color: o.color,
                on: (look[slot] || null) === (o.color || null),
                pick: slot + ':' + (o.color || ''),
              })))
            ).concat([
              Note('Four slots, and everything in them is something you already own. ' +
                'What you can buy is on the other side of money.', { last: true }),
            ]),
          }),

          Stack([
            Panel({ title: 'What they call you', body: [
              Note('It is over your head in the street and on every board you appear on.',
                   { first: true }),
              Swatches(HANDLES.map((h) => Swatch({
                label: h.label, color: h.color,
                on: look.name === h.label, pick: 'name:' + h.label,
              }))),
              Note('Authored, like every other option in this game. Nothing here is typed in.',
                   { last: true, style: 'margin-top:14px' }),
            ] }),

            Panel({ title: 'Why any of this matters', body: [
              Prose('Everyone on Rowan St is drawn the same way you are. The coat is how ' +
                'you find yourself in a street with fourteen people in it.', { first: true }),
              Prose('It is also how <b>they</b> find you. Nadia describes the one in the ' +
                'rust coat; Marta remembers a bag.', { size: 'tiny', last: true }),
            ] }),
          ]),
        ] }) +
        Footnote('Appearance is silhouette-level on purpose &mdash; a coat, hair, a hat, a bag ' +
          '&mdash; because that is what reads at the fixed 3/4 camera distance, and because the ' +
          'two-tier character art (§12) has to hold up as a painted plate later without the ' +
          'wardrobe becoming a modelling problem. It is cosmetic and it stays cosmetic: nothing ' +
          'you wear moves a factor. The nameplate is the one label in the world attached to a ' +
          'person rather than a place, and it is there to answer <i>which one is me</i> in the ' +
          'first minute, not to be read for the next hundred hours.'),
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
        ] }) +
        Footnote('Biological age is the headline readout rather than a health score, shown as a ' +
          'range because low instrumentation means low resolution. The locked tiers are not locked ' +
          'content &mdash; the simulation is already computing HRV collapse from the sleep debt in ' +
          'that sparkline, and the player simply cannot see the cause. Money is the sink that buys ' +
          'the ability to see.'),
    },

    /* ------------------------------------------------------------- karma */
    karma: {
      title: 'Karma',
      sub: 'There is no level. This is a mix of several things about you, two of which you can see.',
      html:
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ bodyClass: 'readout', body: [
              Stat({ value: '1,840', unit: 'Karma', accent: true }),
              `<div style="flex:1">` +
                Metric({ name: 'This week', value: '+38', pct: 14, plain: true }) +
                Metric({ name: 'Four weeks ago', value: '+240', tone: 'faint',
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
              BoardRow({ rank: '2,417', name: 'you', where: 'Tanner Row &middot; up 61 this week',
                         value: '1,840', tone: 'accent', variant: 'you' }),
              BoardRow({ rank: '2,418', name: 'halcyon_r', where: 'Tanner Row', value: '1,836' }),
            ] }),

            Panel({ title: 'Someone you met', body: [
              Prose('<b>halcyon_r</b> covered your shift at the port yard on Tuesday.', { first: true }),
              ButtonRow([Button({ label: '◆ I like this player', variant: 'primary' })]),
              Note('You get one of these every few days. Spend it on someone who earned it.', { last: true }),
            ] }),
          ]),
        ] }) +
        Footnote('The composite replaces the level bar, and it can fall &mdash; +240 collapsing to ' +
          '+38 is the environment decaying, not the player getting worse. Health and Money are ' +
          'revealed; the other slots report only <i>when</i> they last moved, never what they are. ' +
          'Those questions are the hook, and challenges are where the answers leak out. Nothing ' +
          'states the weighting.'),
    },

    /* ---------------------------------------------------------- playbook */
    /* Rules for other people and rules for your own body are the same object
       with the same interface, so they are one screen with one way in: the
       board on your wall, or P. Routines is a tab here, never a destination
       of its own. */
    playbook: {
      title: 'Playbook',
      sub: 'Rules you composed. They run without you, and the world grades them.',
      html:
        Tabs([
          { label: 'Tanner Row &middot; 7 rules', active: true },
          { label: 'Riverfront &middot; 1 rule' },
          { label: 'Routines &middot; 2', panel: 'routines' },
          { label: '+ new rule set', faint: true },
        ]) +
        Grid({ variant: 'wide-left', children: [
          `<div>` +
            Notice({ text: 'Rule 2 and Rule 5 both match when a co-worker asks for cover on short ' +
              'notice, and they disagree. Whichever fires first wins.' }) +
            Rule({ when: ['someone asks for help', 'it costs me under 2 hours'], then: 'say yes',
                   meta: ['fired 31&times;', 'outcome poor 4&times;', 'last: Tuesday, port yard'] }) +
            Rule({ when: ['a co-worker is short-handed'], then: 'cover the shift', conflict: true,
                   meta: ['fired 12&times;', { text: 'conflicts with rule 5', tone: 'danger' }, 'last: Tuesday'] }) +
            Rule({ when: ['someone raises their voice'], then: 'let them finish',
                   meta: ['fired 9&times;', 'outcome good 8&times;'] }) +
            Rule({ when: ['I am owed a favour'], then: 'let it stand',
                   meta: ['fired 6&times;', 'outcome poor 5&times;',
                          { text: 'failing since you arrived at the Riverfront', tone: 'danger' }] }) +
            Rule({ when: ['I am asked twice in one week'], then: 'say no once', meta: ['fired 3&times;'] }) +
          `</div>`,

          Stack([
            Panel({ title: 'Vocabulary', note: '11 of 40-odd', body: [
              Note('Conditions', { first: true }),
              Tokens([
                Token({ label: 'someone asks for help' }),
                Token({ label: 'a co-worker is short-handed' }),
                Token({ label: 'someone raises their voice' }),
                Token({ label: 'I am owed a favour' }),
                Token({ label: 'I am asked twice in one week' }),
                Token({ label: 'it costs me under 2 hours' }),
                Token({ label: '▒▒▒▒▒▒▒▒▒▒▒', kind: 'locked' }),
                Token({ label: '▒▒▒▒▒▒▒', kind: 'locked' }),
                Token({ label: '▒▒▒▒▒▒▒▒▒▒▒▒▒', kind: 'locked' }),
              ]),
              Note('Actions', { style: 'margin-top:14px' }),
              Tokens([
                Token({ label: 'say yes', kind: 'act' }),
                Token({ label: 'cover the shift', kind: 'act' }),
                Token({ label: 'let them finish', kind: 'act' }),
                Token({ label: 'let it stand', kind: 'act' }),
                Token({ label: 'say no once', kind: 'act' }),
                Token({ label: '▒▒▒▒▒▒▒▒', kind: 'locked' }),
                Token({ label: '▒▒▒▒▒▒▒▒▒▒', kind: 'locked' }),
                Token({ label: '▒▒▒▒▒', kind: 'locked' }),
              ]),
              Note('Three of these became available last month.', { last: true, style: 'margin-top:14px' }),
            ] }),

            Panel({ title: 'Rule 4, last five days', body: [
              LogRow({ time: 'Mon', title: 'Let a debt stand with Ilya',
                       detail: 'He took it as an opening and asked for more', trailing: Tag({ label: 'poor' }) }),
              LogRow({ time: 'Tue', title: 'Let a debt stand with Ilya',
                       detail: 'Same again', trailing: Tag({ label: 'poor' }) }),
              LogRow({ time: 'Wed', title: 'Let a debt stand with Marta',
                       detail: 'She stopped returning calls', trailing: Tag({ label: 'poor' }) }),
              LogRow({ time: 'Thu', title: 'Let a debt stand with Ilya',
                       detail: 'He brought a friend to ask', trailing: Tag({ label: 'poor' }) }),
              LogRow({ time: 'Fri', title: 'Let a debt stand with Ilya',
                       detail: '&mdash;', trailing: Tag({ label: 'poor' }) }),
              Note('On Rowan St this rule fired six times and worked.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),
        ] }) +
        Footnote('Rules are composed from an unlocked vocabulary rather than written in prose, ' +
          'which is what makes failure diagnosable: you can see exactly which condition matched and ' +
          'which action fired. Rule sets are scoped per environment, and the same rule that worked ' +
          'on Rowan St has failed five days running at the Riverfront. One bad outcome is noise. ' +
          'Five is a pattern.'),
    },

    /* --------------------------------------------------------- conflicts */
    conflicts: {
      title: 'Today, among these people',
      sub: 'Friction follows from who owes whom. Your rules cover most of it; the gaps come to you.',
      html:
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Handled without you', note: '4 of 5', body: [
              LogRow({ time: '07:20', title: 'Ilya asked you to move a delivery again',
                detail: Token({ label: 'someone asks for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say yes', kind: 'act', size: 'small' }) + ' &middot; he now expects Thursdays',
                trailing: Tag({ label: 'rule 1' }) }),
              LogRow({ time: '11:05', title: 'Marta raised her voice about the roster',
                detail: Token({ label: 'someone raises their voice', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'let them finish', kind: 'act', size: 'small' }) + ' &middot; she apologised afterwards',
                trailing: Tag({ label: 'rule 3' }) }),
              LogRow({ time: '14:40', title: 'Kez wanted the good van',
                detail: Token({ label: 'I am asked twice in one week', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say no once', kind: 'act', size: 'small' }) + ' &middot; fine about it',
                trailing: Tag({ label: 'rule 5' }) }),
              LogRow({ time: '18:15', title: 'Ilya still owes you the Tuesday cover',
                detail: Token({ label: 'I am owed a favour', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'let it stand', kind: 'act', size: 'small' }) + ' &middot; fourth time this week',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
            ] }),

            Panel({ title: 'Nothing covers this one', tag: { label: 'Needs you', variant: 'accent' }, body: [
              Prose('Marta found out you covered for Ilya on Tuesday after telling her you were ' +
                'unavailable. She has not said anything about it, and she has stopped putting you ' +
                'on the good runs.', { first: true }),
              Note('No rule matches. You have nothing in your vocabulary about being caught out.'),
              ButtonRow([
                Button({ label: 'Go and find her', variant: 'primary' }),
                Button({ label: 'Leave it a few days' }),
                Button({ label: 'Write a rule for this', variant: 'ghost' }),
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
        ] }) +
        Footnote('This is the recurring engine of an environment rather than a quest log. Friction ' +
          'is generated from actual relationship state, rules auto-resolve what they cover, and ' +
          'only the gap escalates. Good rules buy time; the hole in your playbook is what demands ' +
          'attention. The entry marked poor is the fourth this week &mdash; that repetition is what ' +
          'makes the failure diagnosable.'),
    },

    /* ------------------------------------------------------------ travel */
    travel: {
      title: 'Where you live',
      sub: 'Wilmington, Delaware. You live in one place at a time, and the 6 runs once a day.',
      html:
        When('east', Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Karma from Tanner Row', note: 'per week', body: [
              DecayRow({ label: 'week 1', pct: 100, amount: '+240' }),
              DecayRow({ label: 'week 2', pct: 75, amount: '+180' }),
              DecayRow({ label: 'week 3', pct: 40, amount: '+95' }),
              DecayRow({ label: 'week 4', pct: 17, amount: '+40' }),
              DecayRow({ label: 'week 5', pct: 6, amount: '+12' }),
              DecayRow({ label: 'this week', pct: 3, amount: '+4' }),
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
              Note('Your Tanner Row rule set, in full.', { first: true }),
              Tokens([
                Token({ label: 'say yes', kind: 'act', size: 'small' }),
                Token({ label: 'cover the shift', kind: 'act', size: 'small' }),
                Token({ label: 'let them finish', kind: 'act', size: 'small' }),
                Token({ label: 'let it stand', kind: 'act', size: 'small' }),
                Token({ label: 'say no once', kind: 'act', size: 'small' }),
              ]),
              Note('Seven rules, refined over six weeks. They work here.', { last: true, style: 'margin-top:14px' }),
            ] }),
          ]),
        ] })) +

        When('quay', Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'Eleven days on the Riverfront', body: [
              LogRow({ time: 'Day 2', title: 'Covered Petra&rsquo;s Saturday',
                detail: Token({ label: 'a co-worker is short-handed', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'cover the shift', kind: 'act', size: 'small' }) + ' &middot; she took the next one too',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              LogRow({ time: 'Day 5', title: 'Let the Vane invoice stand',
                detail: Token({ label: 'I am owed a favour', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'let it stand', kind: 'act', size: 'small' }) + ' &middot; it was written off',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              LogRow({ time: 'Day 9', title: 'Said yes to Rurik twice in a week',
                detail: Token({ label: 'someone asks for help', size: 'small' }) + ' &rarr; ' +
                        Token({ label: 'say yes', kind: 'act', size: 'small' }) + ' &middot; he now introduces you as the one who does it',
                trailing: Tag({ label: 'poor', variant: 'danger' }) }),
              Note('Six of the eleven days. The same three rules each time.', { last: true, style: 'margin-top:12px' }),
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
              Note('The rules you are building here would come with you.', { last: true }),
            ] }),

            Panel({ title: 'The set you are building', tag: { label: '1 rule', variant: 'danger' }, body: [
              Rule({ when: ['I am owed a favour'], then: 'put it in writing',
                     meta: ['fired 2&times;', 'outcome good 2&times;'], style: 'margin:0' }),
              Note('You wrote it on day six. It has not failed yet.', { last: true, style: 'margin-top:12px' }),
            ] }),
          ]),
        ] })) +

        Footnote('You live in one environment at a time and the crossing is capped at once a day, ' +
          'which is what makes a move a relocation rather than a trip. Wilmington is the armature ' +
          'here: the port economy on one side of the Christina and the corporate riverfront on the ' +
          'other, a mile and a half apart. The districts and the people are invented. The ceiling ' +
          'is economic and legible &mdash; returns fall while effort stays flat, the same teaching ' +
          'language money uses past a threshold &mdash; and the nudge onward is diegetic: Dana says ' +
          'there is a room going, never that your rules are local. On the other side the same three ' +
          'rules fail six days out of eleven, which is the point: one bad outcome is noise, a ' +
          'pattern is diagnosable. Transfer runs both ways, so the rule set built on the Riverfront ' +
          'would misfire back on Rowan St just as badly.'),
    },

    /* ---------------------------------------------------------- routines */
    routines: {
      title: 'Playbook',
      sub: 'Rules for the body. Same object as the rest of the playbook, same interface.',
      html:
        Tabs([
          { label: 'Tanner Row &middot; 7 rules', panel: 'playbook' },
          { label: 'Riverfront &middot; 1 rule', panel: 'playbook' },
          { label: 'Routines &middot; 2', active: true },
          { label: '+ new rule set', faint: true },
        ]) +
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
        ] }) +
        Footnote('Struggle before mastery. A routine cannot be configured until the player has ' +
          'experienced the problem it solves, and the way in is a conversation at the moment of ' +
          'consequence rather than a settings screen.'),
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
            Chip({ src: 'Rule 2 fired', val: 'cover the shift' }),
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
        ] }) +
        Footnote('The unlock gate is an insight, not a quiz. There are no four authored options ' +
          '&mdash; the player points at evidence they actually accumulated, several readings are ' +
          'partly right and unlock different tools, and nothing here ever says correct. Choose ' +
          'wrong and the game lets you act on it: train charisma for a fortnight and the shifts ' +
          'keep getting missed, because the cause was sleep. That failure is the lesson, and the ' +
          'conclusion can be revisited later.'),
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
        ] }) +
        Footnote('A challenge is an authored multi-stage scenario ending in a committed judgement, ' +
          'not a dice roll. None of the three readings is correct and none is marked; each carries ' +
          'hidden factor deltas and each leads somewhere different, and the two of them will react ' +
          'to how they <i>perceive</i> what you said. Options are authored rather than free text, ' +
          'which is what keeps the whole game deterministic.'),
    },

    /* -------------------------------------------------------------- work */
    work: {
      title: 'Port &middot; dispatch coordinator',
      sub: 'Tier 2. It runs while you are elsewhere, and you read back what your rules did.',
      html:
        Tabs([
          { label: 'Your role', active: true },
          { label: 'Other work &middot; 5 postings', panel: 'jobs' },
        ]) +
        Grid({ variant: 'wide-left', children: [
          Stack([
            Panel({ title: 'While you were out', note: 'since 07:00', body: [
              LogRow({ time: '09:12', title: 'Driver called in sick, two runs uncovered',
                detail: 'Rule 2 fired &middot; you took one yourself &middot; both runs went out',
                trailing: Tag({ label: 'held', variant: 'success' }) }),
              LogRow({ time: '13:30', title: 'Overtime request from Kez',
                detail: 'Rule 1 fired &middot; approved &middot; $180 over budget for the week',
                trailing: Tag({ label: 'cost' }) }),
              LogRow({ time: '16:02', title: 'Marta asked who authorised the overtime',
                detail: 'No rule matched &middot; the character said &ldquo;I&rsquo;ll look into it&rdquo;',
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
        ] }) +
        Footnote('Active roles are delegated, not micromanaged. The player never sits inside the ' +
          'job &mdash; situations arise, rules handle what they cover, and this screen is the ' +
          'review. The 16:02 entry is a gap in the playbook, which is what sends the player back to ' +
          'compose a new rule. That loop never closes.'),
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
        ] }) +
        Footnote('Income is structural &mdash; it follows tier, performance and assets, not ' +
          'repeated actions &mdash; so changing your work is one of the few decisions that actually ' +
          'moves money. Every step up needs capability <i>plus</i> an authored interview challenge, ' +
          'never a dice roll. Some requirements are hidden factors, shown as a blanked slot that is ' +
          'simply not high enough, so the board teaches what those factors gate without ever naming ' +
          'them. The highest-paying posting is in the other environment, which is the economic ' +
          'ceiling doing its job: past a point the only way up is to go and live somewhere else. ' +
          'And stepping <i>down</i> is listed as plainly as stepping up, because the ladder&rsquo;s ' +
          'whole lesson is that more money costs more of your attention.'),
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
        ] }) +
        Footnote('One of three delivery surfaces, and the only pushed one. The server decides what ' +
          'is released and how much of the backlog surfaces after an absence. Declining does not ' +
          'consume an event &mdash; the one you walked past is still there &mdash; and declining is ' +
          'itself signal that the release policy reads.'),
    },
  };

})(window.ETM);
