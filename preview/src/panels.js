/* Escape the Matrix — overlay panels.
   These sit above the canvas and never replace it: the world stays visible and
   dimmed behind them (technical.md §10.1). Static content — nothing computes. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  function lockedSlot(name, why, dim) {
    return '<div class="locked-slot"' + (dim ? ' style="opacity:.55"' : '') + '>' +
      '<span class="locked-name">' + name + '</span>' +
      '<span class="locked-why">' + why + '</span></div>';
  }

  function metric(name, sub, value, cls, pct, barCls) {
    return '<div class="metric-row"><div><div class="metric-name">' + name +
      '</div><div class="metric-sub">' + sub + '</div></div>' +
      '<div class="metric-value ' + (cls || '') + '">' + value + '</div>' +
      '<div class="bar"><span class="' + (barCls || '') + '" style="width:' + pct + '%"></span></div></div>';
  }

  E.PANELS = {

    /* ------------------------------------------------------------ health */
    health: {
      title: 'Health',
      sub: 'What you can see is a function of what you own, not of how healthy you are.',
      html:
        '<div class="grid wide-left">' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Baseline</span>' +
            '<span class="panel-note">free &middot; from the start</span></div><div class="panel-body">' +
            '<div class="readout"><div><div class="stat-big accent">31</div>' +
            '<div class="stat-unit">Biological age</div></div>' +
            '<p class="faint tiny">Derived from sleep, resting heart rate and weight.</p></div>' +
            metric('Sleep', '5.6 h average over 14 nights &middot; falling', '5.6 h', 'warn', 52, 'warn') +
            '<div class="spark">' +
              '<svg viewBox="0 0 280 62" preserveAspectRatio="none">' +
              '<line x1="0" y1="14.5" x2="280" y2="14.5" stroke="#3a2f25" stroke-dasharray="3 4"/>' +
              '<polyline fill="none" stroke="#c58836" stroke-width="1.6" stroke-linejoin="round" ' +
              'points="8,23.6 28,31.4 48,41.8 68,35.3 88,26.2 108,19.7 128,44.4 148,48.3 168,40.5 188,34 208,45.7 228,50.9 248,43.1 268,32.7"/>' +
              '<circle cx="228" cy="50.9" r="2.6" fill="#d3743f"/>' +
              '<text x="0" y="10" fill="#6e6053" font-size="8" font-family="monospace">7.5 h target</text></svg>' +
              '<div class="spark-axis"><span>14 nights ago</span><span>last night</span></div>' +
            '</div>' +
            metric('Resting heart rate', 'Up 6 bpm on your own baseline', '68 bpm', 'warn', 66, 'warn') +
            metric('Weight trend', 'Stable across 30 days', '&plusmn;0.4 kg', 'ok', 88, 'ok') +
          '</div></div>' +
          '<div><div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Wearable tier</span><span class="tag">Locked</span></div>' +
            '<div class="panel-body"><p class="mt0 faint tiny">Five metrics your body is already producing. You have no way to read them.</p>' +
            '<div class="slots">' + lockedSlot('HRV', 'no reader') + lockedSlot('SLEEP&nbsp;STAGES', 'no reader') +
            lockedSlot('RECOVERY', 'no reader') + lockedSlot('VO₂&nbsp;MAX', 'no reader') +
            lockedSlot('ACTIVITY&nbsp;LOAD', 'no reader') + '</div>' +
            '<div class="btn-row"><span class="btn primary">Fenwick GT4 &mdash; $340</span></div>' +
            '<p class="faint tiny mb0">It reads what your body is already doing.</p></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Clinical tier</span>' +
            '<span class="tag">Locked</span></div><div class="panel-body"><div class="slots">' +
            lockedSlot('GLUCOSE', 'bloodwork') + lockedSlot('INFLAMMATION', 'bloodwork') +
            lockedSlot('LIPIDS', 'bloodwork') + lockedSlot('HORMONAL', 'bloodwork') + '</div>' +
            '<div class="btn-row"><span class="btn">Rowan St Clinic &mdash; $180</span></div>' +
            '<p class="faint tiny mb0">Closed until Monday.</p></div></div></div>' +
        '</div>' +
        '<div class="footnote"><b>Demonstrating.</b> Biological age is the headline readout rather than a ' +
        'health score, shown as a range because low instrumentation means low resolution. The locked tiers ' +
        'are not locked content &mdash; the simulation is already computing HRV collapse from the sleep debt ' +
        'in that sparkline, and the player simply cannot see the cause. Money is the sink that buys ' +
        'the ability to see.</div>'
    },

    /* ------------------------------------------------------------- karma */
    karma: {
      title: 'Karma',
      sub: 'There is no level. This is a mix of several things about you, two of which you can see.',
      html:
        '<div class="grid wide-left"><div>' +
          '<div class="panel" style="margin-bottom:14px"><div class="panel-body readout">' +
            '<div><div class="stat-big accent">1,840</div><div class="stat-unit">Karma</div></div>' +
            '<div style="flex:1">' +
              '<div class="metric-row plain"><div><div class="metric-name">This week</div></div>' +
                '<div class="metric-value">+38</div><div class="bar"><span style="width:14%"></span></div></div>' +
              '<div class="metric-row plain"><div><div class="metric-name">Four weeks ago</div></div>' +
                '<div class="metric-value faint">+240</div><div class="bar"><span style="width:92%;opacity:.4"></span></div></div>' +
              '<p class="faint tiny mb0">Same effort. See <a data-panel="travel" href="#">where you live</a>.</p>' +
            '</div></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">What&rsquo;s in it</span>' +
            '<span class="panel-note">6 parts &middot; 2 legible</span></div><div class="panel-body">' +
            metric('Health', 'Every biomarker, collapsed into biological age', '31', 'accent', 61) +
            metric('Money', 'What you hold', '$4,212', '', 28) +
            '<div class="slots" style="margin-top:12px">' +
            lockedSlot('▒▒▒▒▒▒▒▒', 'moved four days ago') +
            lockedSlot('▒▒▒▒▒▒', 'moved yesterday') +
            lockedSlot('▒▒▒▒▒▒▒▒▒▒', 'hasn&rsquo;t moved in three weeks') +
            lockedSlot('▒▒▒▒▒', '&mdash;', true) + '</div>' +
            '<p class="faint tiny mb0">They are all real, and they are all counted.</p>' +
          '</div></div></div>' +
          '<div><div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Boards</span><span class="panel-note">one per part, plus the mix</span></div>' +
            '<div class="panel-body"><div class="tabs"><a class="active" href="#">Overall</a>' +
            '<a href="#">Health</a><a href="#">Money</a><a href="#" class="faint">▒▒▒</a>' +
            '<a href="#" class="faint">▒▒▒</a></div>' +
            '<div class="board"><span class="rank">1</span><span><b>tsvetan_k</b><i>Riverfront</i></span><span class="metric-value">9,104</span></div>' +
            '<div class="board"><span class="rank">2</span><span><b>mor_shalev</b><i>Downtown</i></span><span class="metric-value">8,877</span></div>' +
            '<div class="board"><span class="rank">3</span><span><b>quietferry</b><i>The Strip</i></span><span class="metric-value">8,640</span></div>' +
            '<div class="board gap"><span class="rank">&hellip;</span><span></span><span></span></div>' +
            '<div class="board you"><span class="rank">2,417</span><span><b>you</b><i>East Side &middot; up 61 this week</i></span><span class="metric-value accent">1,840</span></div>' +
            '<div class="board"><span class="rank">2,418</span><span><b>halcyon_r</b><i>East Side</i></span><span class="metric-value">1,836</span></div>' +
            '</div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Someone you met</span></div>' +
            '<div class="panel-body"><p class="mt0"><b>halcyon_r</b> covered your shift at the depot yard on Tuesday.</p>' +
            '<div class="btn-row"><span class="btn primary">◆ I like this player</span></div>' +
            '<p class="faint tiny mb0">You get one of these every few days. Spend it on someone who earned it.</p>' +
          '</div></div></div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> The composite replaces the level bar, and it can fall &mdash; ' +
        '+240 collapsing to +38 is the environment decaying, not the player getting worse. Health and Money are ' +
        'revealed; the other slots report only <i>when</i> they last moved, never what they are. Those questions ' +
        'are the hook, and challenges are where the answers leak out. Nothing states the weighting.</div>'
    },

    /* ---------------------------------------------------------- playbook */
    playbook: {
      title: 'Playbook',
      sub: 'Rules you composed. They run without you, and the world grades them.',
      html:
        '<div class="tabs"><a class="active" href="#">East Side &middot; 7 rules</a>' +
        '<a href="#">Riverfront &middot; 1 rule</a><a href="#" class="faint">+ new rule set</a></div>' +
        '<div class="grid wide-left"><div>' +
          '<div class="notice">Rule 2 and Rule 5 both match when a co-worker asks for cover on short notice, and they disagree. Whichever fires first wins.</div>' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">someone asks for help</span><span class="kw">and</span>' +
            '<span class="token cond">it costs me under 2 hours</span><span class="kw">then</span>' +
            '<span class="token act">say yes</span></div>' +
            '<div class="rule-meta"><span>fired 31&times;</span><span>outcome poor 4&times;</span><span>last: Tuesday, depot yard</span></div></div>' +
          '<div class="rule conflict"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">a co-worker is short-handed</span><span class="kw">then</span>' +
            '<span class="token act">cover the shift</span></div>' +
            '<div class="rule-meta"><span>fired 12&times;</span><span class="warn-text">conflicts with rule 5</span><span>last: Tuesday</span></div></div>' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">someone raises their voice</span><span class="kw">then</span>' +
            '<span class="token act">let them finish</span></div>' +
            '<div class="rule-meta"><span>fired 9&times;</span><span>outcome good 8&times;</span></div></div>' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">I am owed a favour</span><span class="kw">then</span>' +
            '<span class="token act">let it stand</span></div>' +
            '<div class="rule-meta"><span>fired 6&times;</span><span>outcome poor 5&times;</span><span class="warn-text">failing since you arrived at the Riverfront</span></div></div>' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">I am asked twice in one week</span><span class="kw">then</span>' +
            '<span class="token act">say no once</span></div>' +
            '<div class="rule-meta"><span>fired 3&times;</span></div></div>' +
        '</div>' +
        '<div><div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
          '<span class="panel-title">Vocabulary</span><span class="panel-note">11 of 40-odd</span></div>' +
          '<div class="panel-body"><p class="mt0 faint tiny">Conditions</p><div class="tokens">' +
          '<span class="token cond">someone asks for help</span><span class="token cond">a co-worker is short-handed</span>' +
          '<span class="token cond">someone raises their voice</span><span class="token cond">I am owed a favour</span>' +
          '<span class="token cond">I am asked twice in one week</span><span class="token cond">it costs me under 2 hours</span>' +
          '<span class="token locked">▒▒▒▒▒▒▒▒▒▒▒</span><span class="token locked">▒▒▒▒▒▒▒</span>' +
          '<span class="token locked">▒▒▒▒▒▒▒▒▒▒▒▒▒</span></div>' +
          '<p class="faint tiny" style="margin-top:14px">Actions</p><div class="tokens">' +
          '<span class="token act">say yes</span><span class="token act">cover the shift</span>' +
          '<span class="token act">let them finish</span><span class="token act">let it stand</span>' +
          '<span class="token act">say no once</span><span class="token locked">▒▒▒▒▒▒▒▒</span>' +
          '<span class="token locked">▒▒▒▒▒▒▒▒▒▒</span><span class="token locked">▒▒▒▒▒</span></div>' +
          '<p class="faint tiny mb0" style="margin-top:14px">Three of these became available last month.</p></div></div>' +
        '<div class="panel"><div class="panel-head"><span class="panel-title">Rule 4, last five days</span></div>' +
          '<div class="panel-body">' +
          '<div class="log-row"><span class="log-time">Mon</span><span><span class="log-what">Let a debt stand with Ilya</span>' +
            '<span class="log-detail">He took it as an opening and asked for more</span></span><span class="tag">poor</span></div>' +
          '<div class="log-row"><span class="log-time">Tue</span><span><span class="log-what">Let a debt stand with Ilya</span>' +
            '<span class="log-detail">Same again</span></span><span class="tag">poor</span></div>' +
          '<div class="log-row"><span class="log-time">Wed</span><span><span class="log-what">Let a debt stand with Marta</span>' +
            '<span class="log-detail">She stopped returning calls</span></span><span class="tag">poor</span></div>' +
          '<div class="log-row"><span class="log-time">Thu</span><span><span class="log-what">Let a debt stand with Ilya</span>' +
            '<span class="log-detail">He brought a friend to ask</span></span><span class="tag">poor</span></div>' +
          '<div class="log-row"><span class="log-time">Fri</span><span><span class="log-what">Let a debt stand with Ilya</span>' +
            '<span class="log-detail">&mdash;</span></span><span class="tag">poor</span></div>' +
          '<p class="faint tiny mb0" style="margin-top:12px">On Rowan St this rule fired six times and worked.</p>' +
        '</div></div></div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> Rules are composed from an unlocked vocabulary rather than ' +
        'written in prose, which is what makes failure diagnosable: you can see exactly which condition matched ' +
        'and which action fired. Rule sets are scoped per environment, and the same rule that worked on Rowan St ' +
        'has failed five days running at the Riverfront. One bad outcome is noise. Five is a pattern.</div>'
    },

    /* --------------------------------------------------------- conflicts */
    conflicts: {
      title: 'Today, among these people',
      sub: 'Friction follows from who owes whom. Your rules cover most of it; the gaps come to you.',
      html:
        '<div class="grid wide-left"><div>' +
          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Handled without you</span><span class="panel-note">4 of 5</span></div>' +
            '<div class="panel-body">' +
            '<div class="log-row"><span class="log-time">07:20</span><span><span class="log-what">Ilya asked you to move a delivery again</span>' +
              '<span class="log-detail"><span class="token cond small">someone asks for help</span> &rarr; ' +
              '<span class="token act small">say yes</span> &middot; he now expects Thursdays</span></span><span class="tag">rule 1</span></div>' +
            '<div class="log-row"><span class="log-time">11:05</span><span><span class="log-what">Marta raised her voice about the roster</span>' +
              '<span class="log-detail"><span class="token cond small">someone raises their voice</span> &rarr; ' +
              '<span class="token act small">let them finish</span> &middot; she apologised afterwards</span></span><span class="tag">rule 3</span></div>' +
            '<div class="log-row"><span class="log-time">14:40</span><span><span class="log-what">Kez wanted the good van</span>' +
              '<span class="log-detail"><span class="token cond small">I am asked twice in one week</span> &rarr; ' +
              '<span class="token act small">say no once</span> &middot; fine about it</span></span><span class="tag">rule 5</span></div>' +
            '<div class="log-row"><span class="log-time">18:15</span><span><span class="log-what">Ilya still owes you the Tuesday cover</span>' +
              '<span class="log-detail"><span class="token cond small">I am owed a favour</span> &rarr; ' +
              '<span class="token act small">let it stand</span> &middot; fourth time this week</span></span><span class="tag warn-text">poor</span></div>' +
            '</div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Nothing covers this one</span>' +
            '<span class="tag amber">Needs you</span></div><div class="panel-body">' +
            '<p class="mt0">Marta found out you covered for Ilya on Tuesday after telling her you were unavailable. ' +
            'She has not said anything about it, and she has stopped putting you on the good runs.</p>' +
            '<p class="faint tiny">No rule matches. You have nothing in your vocabulary about being caught out.</p>' +
            '<div class="btn-row"><span class="btn primary">Go and find her</span>' +
            '<span class="btn">Leave it a few days</span><span class="btn ghost">Write a rule for this</span></div>' +
          '</div></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Where you stand</span>' +
            '<span class="panel-note">East Side</span></div><div class="panel-body">' +
            metric('Ilya', 'Owes you three &middot; asks constantly', 'warm', '', 74) +
            metric('Marta', 'Trust down since Tuesday', 'cooling', 'warn', 38, 'warn') +
            metric('Kez', 'Even', 'steady', 'ok', 60, 'ok') +
            metric('Dana', 'You have not spoken in two weeks', 'faded', '', 22) +
            '<p class="faint tiny mb0" style="margin-top:12px">These four generate most of what happens to you here.</p>' +
          '</div></div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> This is the recurring engine of an environment rather than a ' +
        'quest log. Friction is generated from actual relationship state, rules auto-resolve what they cover, and ' +
        'only the gap escalates. Good rules buy time; the hole in your playbook is what demands attention. The ' +
        'entry marked poor is the fourth this week &mdash; that repetition is what makes the failure diagnosable.</div>'
    },

    /* ------------------------------------------------------------ travel */
    travel: {
      title: 'Where you live',
      sub: 'You live in one place at a time. The 14 line runs once a day.',
      html:

        '<div data-when="east">' +
        '<div class="grid wide-left"><div>' +
          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Karma from the East Side</span><span class="panel-note">per week</span></div>' +
            '<div class="panel-body">' +
            '<div class="decay-row"><span class="label">week 1</span><span class="decay-track"><span style="width:100%"></span></span><span class="amount">+240</span></div>' +
            '<div class="decay-row"><span class="label">week 2</span><span class="decay-track"><span style="width:75%"></span></span><span class="amount">+180</span></div>' +
            '<div class="decay-row"><span class="label">week 3</span><span class="decay-track"><span style="width:40%"></span></span><span class="amount">+95</span></div>' +
            '<div class="decay-row"><span class="label">week 4</span><span class="decay-track"><span style="width:17%"></span></span><span class="amount">+40</span></div>' +
            '<div class="decay-row"><span class="label">week 5</span><span class="decay-track"><span style="width:6%"></span></span><span class="amount">+12</span></div>' +
            '<div class="decay-row"><span class="label">this week</span><span class="decay-track"><span style="width:3%"></span></span><span class="amount">+4</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">You are doing the same amount of work as in week one.</p>' +
            '</div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Where the ceiling is</span></div>' +
            '<div class="panel-body">' +
            metric('Money', 'The depot pays what the depot pays', 'unchanged 6 wks', 'warn', 100, 'warn') +
            '<div class="locked-slot"><span class="locked-name">▒▒▒▒▒▒▒▒▒▒</span><span class="locked-why">hasn&rsquo;t moved in three weeks</span></div>' +
            '<div class="locked-slot"><span class="locked-name">▒▒▒▒▒▒▒▒</span><span class="locked-why">moves less each week</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">You know everyone here, and you know what they will do.</p>' +
          '</div></div></div>' +
          '<div><div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Dana, on the phone</span></div><div class="panel-body">' +
            '<p class="mt0 speech-sm">&ldquo;You&rsquo;re still on Rowan St? There&rsquo;s a room going on the Kestrel ' +
            'Quay side. Different crowd &mdash; you&rsquo;d hate half of them. Rent&rsquo;s worse. Come and see it.&rdquo;</p>' +
            '<div class="btn-row"><span class="btn primary" data-move="quay">Take the 14 line over</span>' +
            '<span class="btn">Not today</span></div>' +
            '<p class="faint tiny mb0">You would be living there. Your things, your job, your mornings.</p></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">What you would be taking</span></div>' +
            '<div class="panel-body"><p class="mt0 faint tiny">Your East Side rule set, in full.</p>' +
            '<div class="tokens">' +
            '<span class="token act small">say yes</span><span class="token act small">cover the shift</span>' +
            '<span class="token act small">let them finish</span><span class="token act small">let it stand</span>' +
            '<span class="token act small">say no once</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:14px">Seven rules, refined over six weeks. They work here.</p>' +
          '</div></div></div></div>' +
        '</div>' +

        '<div data-when="quay">' +
        '<div class="grid wide-left"><div>' +
          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Eleven days on the Quay</span></div><div class="panel-body">' +
            '<div class="log-row"><span class="log-time">Day 2</span><span><span class="log-what">Covered Petra&rsquo;s Saturday</span>' +
              '<span class="log-detail"><span class="token cond small">a co-worker is short-handed</span> &rarr; ' +
              '<span class="token act small">cover the shift</span> &middot; she took the next one too</span></span><span class="tag warn-text">poor</span></div>' +
            '<div class="log-row"><span class="log-time">Day 5</span><span><span class="log-what">Let the Vane invoice stand</span>' +
              '<span class="log-detail"><span class="token cond small">I am owed a favour</span> &rarr; ' +
              '<span class="token act small">let it stand</span> &middot; it was written off</span></span><span class="tag warn-text">poor</span></div>' +
            '<div class="log-row"><span class="log-time">Day 9</span><span><span class="log-what">Said yes to Rurik twice in a week</span>' +
              '<span class="log-detail"><span class="token cond small">someone asks for help</span> &rarr; ' +
              '<span class="token act small">say yes</span> &middot; he now introduces you as the one who does it</span></span><span class="tag warn-text">poor</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">Six of the eleven days. The same three rules each time.</p>' +
            '</div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">What this place pays</span></div>' +
            '<div class="panel-body">' +
            metric('Money', 'Kestrel &amp; Vane bill by the hour and everyone knows it', '$5,900 within reach', 'ok', 74, 'ok') +
            '<div class="locked-slot"><span class="locked-name">▒▒▒▒▒▒▒▒▒▒</span><span class="locked-why">moved twice this week</span></div>' +
            '<div class="locked-slot"><span class="locked-name">▒▒▒▒▒▒▒▒</span><span class="locked-why">moved yesterday</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">Nobody here has known you long enough to owe you anything.</p>' +
          '</div></div></div>' +
          '<div><div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Your old room</span></div><div class="panel-body">' +
            '<p class="mt0 speech-sm">Marta: &ldquo;3B&rsquo;s still empty if you want it. Ilya asks about you. ' +
            'I&rsquo;d have you back on the roster Monday.&rdquo;</p>' +
            '<div class="btn-row"><span class="btn primary" data-move="east">Take the 14 line back</span>' +
            '<span class="btn">Stay</span></div>' +
            '<p class="faint tiny mb0">The rules you are building here would come with you.</p></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">The set you are building</span>' +
            '<span class="tag warn-text">1 rule</span></div><div class="panel-body">' +
            '<div class="rule" style="margin:0"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">I am owed a favour</span><span class="kw">then</span>' +
            '<span class="token act">put it in writing</span></div>' +
            '<div class="rule-meta"><span>fired 2&times;</span><span>outcome good 2&times;</span></div></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">You wrote it on day six. It has not failed yet.</p>' +
          '</div></div></div></div>' +
        '</div>' +

        '<div class="footnote"><b>Demonstrating.</b> You live in one environment at a time and the crossing is capped ' +
        'at once a day, which is what makes a move a relocation rather than a trip. The ceiling is economic and ' +
        'legible &mdash; returns fall while effort stays flat, the same teaching language money uses past a ' +
        'threshold &mdash; and the nudge onward is diegetic: Dana says there is a room going, never that your rules ' +
        'are local. On the other side the same three rules fail six days out of eleven, which is the point: one bad ' +
        'outcome is noise, a pattern is diagnosable. Transfer runs both ways, so the rule set built on the Quay ' +
        'would misfire back on Rowan St just as badly.</div>'
    },

    /* ---------------------------------------------------------- routines */
    routines: {
      title: 'Routines',
      sub: 'Rules for the body. Same idea as the playbook, same interface.',
      html:
        '<div class="grid two"><div class="panel"><div class="panel-head">' +
          '<span class="panel-title">Configured</span></div><div class="panel-body">' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">it reaches 12:30 AM</span><span class="kw">then</span>' +
            '<span class="token act">go to bed</span></div>' +
            '<div class="rule-meta"><span>held 9 of the last 14 nights</span></div></div>' +
          '<div class="rule"><div class="rule-line"><span class="kw">when</span>' +
            '<span class="token cond">it is Mon / Wed / Sat</span><span class="kw">then</span>' +
            '<span class="token act">train for 45 minutes</span></div>' +
            '<div class="rule-meta"><span>held 12 of the last 12</span></div></div>' +
          '<p class="faint tiny mb0">Set once. You should not have to think about these again.</p>' +
        '</div></div>' +
        '<div class="panel"><div class="panel-head"><span class="panel-title">Not offered yet</span></div>' +
          '<div class="panel-body"><div class="slots">' +
          lockedSlot('DIET', '&mdash;') +
          lockedSlot('ORGANISATION', '&mdash;') + '</div>' +
          
          '<div class="notice cool">You have missed two shifts in a fortnight and both times you thought you had ' +
          'the day off. There is someone at the depot who has seen this before.</div>' +
          '<div class="btn-row"><span class="btn primary" data-panel="diagnosis">Go and talk to Marta</span></div>' +
        '</div></div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> Struggle before mastery. A routine cannot be configured until ' +
        'the player has experienced the problem it solves, and the way in is a conversation at the moment of ' +
        'consequence rather than a settings screen.</div>'
    },

    /* --------------------------------------------------------- diagnosis */
    diagnosis: {
      title: 'The depot, back office',
      sub: '',
      scene: true,
      html:
        '<div class="dialogue"><div class="portrait">' + E_PORTRAIT('marta') +
          '<div class="portrait-name">Marta<div class="portrait-role">Runs the roster</div></div></div>' +
        '<div><p class="speech">&ldquo;Second time this month. I&rsquo;m not writing you up, I want to know what ' +
          'you think is going on. You&rsquo;ve got all of it in front of you &mdash; show me.&rdquo;</p>' +
          '<p class="faint tiny" style="margin:-6px 0 14px">Point at what you think caused it. Your own records, not a list of answers.</p>' +
          '<div class="chips">' +
            '<div class="chip picked"><span class="chip-src">Sleep log &middot; Tue</span><span class="chip-val">4.7 h</span></div>' +
            '<div class="chip"><span class="chip-src">Sleep log &middot; 14-night avg</span><span class="chip-val">5.6 h</span></div>' +
            '<div class="chip picked"><span class="chip-src">Calendar &middot; Tue</span><span class="chip-val">no shift entered</span></div>' +
            '<div class="chip"><span class="chip-src">Rule 2 fired</span><span class="chip-val">cover the shift</span></div>' +
            '<div class="chip"><span class="chip-src">Resting HR</span><span class="chip-val">68 bpm, up 6</span></div>' +
            '<div class="chip"><span class="chip-src">Ilya, Mon</span><span class="chip-val">asked you late</span></div>' +
            '<div class="chip"><span class="chip-src">Money &middot; Tue</span><span class="chip-val">paid actions 3 / 3</span></div>' +
            '<div class="chip"><span class="chip-src">Walked</span><span class="chip-val">11 km, Mon</span></div>' +
          '</div>' +
          '<div class="commit"><p class="faint tiny mt0">Two picked. What you are saying is that you never wrote it ' +
          'down and you were too tired to notice.</p>' +
          '<div class="btn-row"><span class="btn primary">That&rsquo;s what happened</span>' +
          '<span class="btn ghost">Pick something else</span></div></div>' +
          '<p class="faint tiny" style="margin-top:14px">You can come back to this when you know more.</p>' +
        '</div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> The unlock gate is an insight, not a quiz. There are no four ' +
        'authored options &mdash; the player points at evidence they actually accumulated, several readings are ' +
        'partly right and unlock different tools, and nothing here ever says correct. Choose wrong and the game ' +
        'lets you act on it: train charisma for a fortnight and the shifts keep getting missed, because the cause ' +
        'was sleep. That failure is the lesson, and the conclusion can be revisited later.</div>'
    },

    /* --------------------------------------------------------- challenge */
    challenge: {
      title: '',
      sub: '',
      scene: true,
      html:
        '<div class="scene-head"><span class="tag amber">Resolve a conflict</span>' +
          '<span class="stage-dots"><i class="done"></i><i class="done"></i><i class="now"></i><i></i></span>' +
          '<span class="faint tiny">about 6 minutes &middot; once ever</span></div>' +
        '<div class="dialogue"><div class="portrait">' + E_PORTRAIT('nadia') +
          '<div class="portrait-name">Nadia<div class="portrait-role">Outside the Verona</div></div></div>' +
        '<div>' +
          '<p class="speech">&ldquo;He sold the van. Not <em>our</em> van, his van, he keeps saying &mdash; except ' +
          'I paid half of it and he knows I did. And now he&rsquo;s standing there telling me I never asked for ' +
          'anything in writing, like that&rsquo;s the part that matters.&rdquo;</p>' +
          '<p class="speech" style="font-size:14px;color:var(--text-dim)">Tomas, quieter: &ldquo;She was going to ' +
          'leave. I&rsquo;m not going to pretend I didn&rsquo;t know that when I did it.&rdquo;</p>' +
          '<hr class="soft">' +
          '<p class="faint tiny">You have heard enough of it. Pick the reading you are going to act on.</p>' +
          '<div class="option chosen"><div class="option-lead">Commit to this</div>' +
            '<div class="option-text">This is about the money. Whatever else is going on, she is owed half of ' +
            'what the van sold for, and that is a number they can both look at.</div></div>' +
          '<div class="option"><div class="option-lead">Commit to this</div>' +
            '<div class="option-text">He sold it because he thought she was leaving. Neither of them has said the ' +
            'actual thing out loud yet, and the van is standing in for it.</div></div>' +
          '<div class="option"><div class="option-lead">Commit to this</div>' +
            '<div class="option-text">She has been keeping score for a long time and this is the first item large ' +
            'enough to say aloud. Settling the van will not settle anything.</div></div>' +
          '<div class="btn-row" style="margin-top:14px"><span class="btn primary">Say it</span>' +
            '<span class="btn ghost">Say nothing and walk on</span></div>' +
        '</div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> A challenge is an authored multi-stage scenario ending in a ' +
        'committed judgement, not a dice roll. None of the three readings is correct and none is marked; each ' +
        'carries hidden factor deltas and each leads somewhere different, and the two of them will react to how ' +
        'they <i>perceive</i> what you said. Options are authored rather than free text, which is what keeps the ' +
        'whole game deterministic.</div>'
    },

    /* -------------------------------------------------------------- work */
    work: {
      title: 'Depot &middot; dispatch coordinator',
      sub: 'Tier 2. It runs while you are elsewhere, and you read back what your rules did.',
      html:
        '<div class="tabs"><a class="active" href="#">Your role</a>' +
        '<a data-panel="jobs" href="#">Other work &middot; 5 postings</a></div>' +
        '<div class="grid wide-left"><div>' +
          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">While you were out</span><span class="panel-note">since 07:00</span></div>' +
            '<div class="panel-body">' +
            '<div class="log-row"><span class="log-time">09:12</span><span><span class="log-what">Driver called in sick, two runs uncovered</span>' +
              '<span class="log-detail">Rule 2 fired &middot; you took one yourself &middot; both runs went out</span></span><span class="tag ok-text">held</span></div>' +
            '<div class="log-row"><span class="log-time">13:30</span><span><span class="log-what">Overtime request from Kez</span>' +
              '<span class="log-detail">Rule 1 fired &middot; approved &middot; $180 over budget for the week</span></span><span class="tag">cost</span></div>' +
            '<div class="log-row"><span class="log-time">16:02</span><span><span class="log-what">Marta asked who authorised the overtime</span>' +
              '<span class="log-detail">No rule matched &middot; the character said &ldquo;I&rsquo;ll look into it&rdquo;</span></span><span class="tag warn-text">gap</span></div>' +
            '</div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">The ladder</span></div>' +
            '<div class="panel-body">' +
            '<div class="ladder-row"><span class="ladder-tier">1</span><span><b>Warehouse nights</b>' +
              '<i>22:00&ndash;06:00, five on, two off. Loading bay.</i></span><span class="tag dim">done</span></div>' +
            '<div class="ladder-row current"><span class="ladder-tier">2</span><span><b>Dispatch coordinator</b>' +
              '<i>Phone on. Two or three calls a shift.</i></span><span class="tag amber">now &middot; $2,240 / mo</span></div>' +
            '<div class="ladder-row future"><span class="ladder-tier">3</span><span><b>Operations manager</b>' +
              '<i>Staffing, budget, and whoever just quit</i></span><span class="tag">interview</span></div>' +
            '<div class="ladder-row future"><span class="ladder-tier">4</span><span><b>Regional director</b>' +
              '<i>Quarterly numbers. You sign things.</i></span><span class="tag dim">&mdash;</span></div>' +
            '<div class="ladder-row future"><span class="ladder-tier">5</span><span><b>&mdash;</b>' +
              '<i>&nbsp;</i></span><span class="tag dim">&mdash;</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">Fenn posts a tier 3 about twice a year.</p>' +
          '</div></div></div>' +
          '<div class="panel"><div class="panel-head"><span class="panel-title">Standing</span></div>' +
            '<div class="panel-body">' +
            metric('Runs out on time', 'Last 30 days', '94%', 'ok', 94, 'ok') +
            metric('Overtime budget', 'Over by $180', 'over', 'warn', 118, 'warn') +
            metric('How Marta reads you', 'Down since Tuesday', 'cooling', 'warn', 38, 'warn') +
            '<hr class="soft">' +
            '<p class="faint tiny">The tier 3 interview exists because you applied. It is a challenge like any ' +
            'other &mdash; you will get through it or you will not.</p>' +
            '<div class="btn-row"><span class="btn" data-panel="jobs">See what else is going</span></div>' +
          '</div></div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> Active roles are delegated, not micromanaged. The player never ' +
        'sits inside the job &mdash; situations arise, rules handle what they cover, and this screen is the review. ' +
        'The 16:02 entry is a gap in the playbook, which is what sends the player back to compose a new rule. ' +
        'That loop never closes.</div>'
    },

      /* -------------------------------------------------------------- jobs */
    jobs: {
      title: 'Other work',
      sub: 'Nothing here is recommended to you. These are the postings that exist.',
      html:
        '<div class="tabs"><a data-panel="work" href="#">Your role</a>' +
        '<a class="active" href="#">Other work &middot; 5 postings</a></div>' +
        '<div class="grid wide-left"><div>' +

          '<div class="posting"><div class="posting-head">' +
            '<div><div class="posting-role">Operations manager</div>' +
            '<div class="posting-where">Fenn Logistics &middot; the depot &middot; tier 3</div></div>' +
            '<div class="right"><div class="posting-pay">$4,100 <span class="faint tiny">/ mo</span></div>' +
            '<div class="posting-where">staffing, budget, and whoever just quit</div></div></div>' +
            '<div class="req"><span class="met">✓ two years on the floor</span>' +
            '<span class="unmet">someone senior to vouch for you &mdash; Marta, cooling</span>' +
            '<span class="hidden">▒▒▒▒▒▒▒▒ &mdash; not high enough</span></div>' +
            '<div class="posting-foot"><span class="faint tiny">You applied on Tuesday. Closes Friday. ' +
            'There will be an interview.</span><span class="btn small">Interview pending</span></div>' +
          '</div>' +

          '<div class="posting"><div class="posting-head">' +
            '<div><div class="posting-role">Night dispatch</div>' +
            '<div class="posting-where">Corrin Freight &middot; the strip &middot; tier 2</div></div>' +
            '<div class="right"><div class="posting-pay">$2,380 <span class="faint tiny">/ mo</span></div>' +
            '<div class="posting-where">22:00&ndash;06:00, Sunday to Thursday</div></div></div>' +
            '<div class="req"><span class="met">✓ dispatch experience</span>' +
            '<span class="met">✓ own transport</span></div>' +
            '<div class="posting-foot"><span class="faint tiny">Your sleep routine runs 00:30 to 06:15. You have ' +
            'held it nine of the last fourteen nights.</span><span class="btn small">Apply</span></div>' +
          '</div>' +

          '<div class="posting"><div class="posting-head">' +
            '<div><div class="posting-role">Assistant to the desk</div>' +
            '<div class="posting-where">Kestrel &amp; Vane &middot; Kestrel Quay &middot; tier 3</div></div>' +
            '<div class="right"><div class="posting-pay">$5,900 <span class="faint tiny">/ mo</span></div>' +
            '<div class="posting-where">on call; they expect a reply inside the hour</div></div></div>' +
            '<div class="req"><span class="met">✓ they have met you</span>' +
            '<span class="unmet">you do not live on that side of the river</span>' +
            '<span class="hidden">▒▒▒▒▒▒ &mdash; unknown</span></div>' +
            '<div class="posting-foot"><span class="faint tiny">Nothing on Rowan St pays this. Nothing on Rowan ' +
            'St is going to.</span><span class="btn small" data-panel="travel">You would have to move first</span></div>' +
          '</div>' +

          '<div class="posting"><div class="posting-head">' +
            '<div><div class="posting-role">Bar work, evenings</div>' +
            '<div class="posting-where">Verona Diner &middot; Rowan St &middot; tier 1</div></div>' +
            '<div class="right"><div class="posting-pay">$1,480 <span class="faint tiny">/ mo + tips</span></div>' +
            '<div class="posting-where">17:00&ndash;01:00, Thursday to Sunday</div></div></div>' +
            '<div class="req"><span class="met">✓ nothing, really</span></div>' +
            '<div class="posting-foot"><span class="faint tiny">Nadia asked whether you wanted it. Tips are ' +
            'earnings, and earnings still cap at three a day.</span><span class="btn small">Apply</span></div>' +
          '</div>' +

          '<div class="posting"><div class="posting-head">' +
            '<div><div class="posting-role">Warehouse floor</div>' +
            '<div class="posting-where">Corrin Freight &middot; the strip &middot; tier 1</div></div>' +
            '<div class="right"><div class="posting-pay">$1,640 <span class="faint tiny">/ mo</span></div>' +
            '<div class="posting-where">06:00&ndash;14:00, five on, two off</div></div></div>' +
            '<div class="req"><span class="met">✓ you have done this one before</span></div>' +
            '<div class="posting-foot"><span class="faint tiny">You did two years of this. Nobody ever called you ' +
            'after two.</span><span class="btn small">Apply</span></div>' +
          '</div>' +

        '</div><div>' +

          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">Where you are now</span><span class="tag amber">tier 2</span></div>' +
            '<div class="panel-body">' +
            '<p class="mt0"><b>Dispatch coordinator</b><br><span class="faint tiny">Fenn Logistics &middot; ' +
            'fourteen months</span></p>' +
            metric('Pay', 'Structural &mdash; it follows the tier, not the hours', '$2,240 / mo', '', 38) +
            metric('Runs out on time', 'Last 30 days', '94%', 'ok', 94, 'ok') +
            metric('How Marta reads you', 'Down since Tuesday', 'cooling', 'warn', 38, 'warn') +
            '</div></div>' +

          '<div class="panel" style="margin-bottom:14px"><div class="panel-head">' +
            '<span class="panel-title">What leaving actually costs</span></div><div class="panel-body">' +
            '<p class="mt0 tiny">Fourteen months of standing goes to nothing. You start the new place unknown.</p>' +
            '<p class="tiny">Ilya owes you three. Those do not transfer.</p>' +
            '<p class="tiny">Your playbook comes with you &mdash; it is scoped to these people, not to this job. ' +
            'Move to the Quay and that is a different matter.</p>' +
            '<p class="mb0 tiny">A new role takes a couple of weeks before it starts generating anything worth ' +
            'reading back.</p>' +
            '</div></div>' +

          '<div class="panel"><div class="panel-head"><span class="panel-title">The trade</span>' +
            '<span class="panel-note">what the week looks like</span></div><div class="panel-body">' +
            '<div class="decay-row"><span class="label">tier 1</span><span class="decay-track">' +
            '<span style="width:8%"></span></span><span class="amount">off at 14:00</span></div>' +
            '<div class="decay-row"><span class="label">tier 2</span><span class="decay-track">' +
            '<span style="width:24%"></span></span><span class="amount">phone on</span></div>' +
            '<div class="decay-row"><span class="label">tier 3</span><span class="decay-track">' +
            '<span style="width:58%"></span></span><span class="amount">called at home</span></div>' +
            '<div class="decay-row"><span class="label">tier 4</span><span class="decay-track">' +
            '<span style="width:82%"></span></span><span class="amount">away Tue&ndash;Thu</span></div>' +
            '<div class="decay-row"><span class="label">tier 5</span><span class="decay-track">' +
            '<span style="width:100%"></span></span><span class="amount">no line at all</span></div>' +
            '<p class="faint tiny mb0" style="margin-top:12px">What people holding each of these said when asked ' +
            'about their week.</p>' +
          '</div></div>' +

        '</div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> Income is structural &mdash; it follows tier, performance and ' +
        'assets, not repeated actions &mdash; so changing your work is one of the few decisions that actually ' +
        'moves money. Every step up needs capability <i>plus</i> an authored interview challenge, never a dice ' +
        'roll. Some requirements are hidden factors, shown as a blanked slot that is simply not high enough, so ' +
        'the board teaches what those factors gate without ever naming them. The highest-paying posting is in ' +
        'the other environment, which is the economic ceiling doing its job: past a point the only way up is to ' +
        'go and live somewhere else. And stepping <i>down</i> is listed as plainly as stepping up, because the ' +
        'ladder\'s whole lesson is that more money costs more of your attention.</div>'
    },

    /* ------------------------------------------------------------- inbox */
    inbox: {
      title: 'Inbox',
      sub: 'The world got in touch. Nothing here is a quest list &mdash; it is what came to you.',
      html:
        '<div class="panel"><div class="panel-body">' +
        '<div class="log-row"><span class="log-time">18:40</span><span><span class="log-what">Dana called and you missed it</span>' +
          '<span class="log-detail">She left a message about a room on the Kestrel Quay side</span></span>' +
          '<span class="btn small" data-panel="travel">Listen</span></div>' +
        '<div class="log-row"><span class="log-time">16:02</span><span><span class="log-what">Marta wants to know who authorised the overtime</span>' +
          '<span class="log-detail">Your character said he would look into it. He has not.</span></span>' +
          '<span class="btn small" data-panel="work">Open</span></div>' +
        '<div class="log-row"><span class="log-time">11:15</span><span><span class="log-what">The depot posted the operations manager role</span>' +
          '<span class="log-detail">Closes Friday</span></span><span class="btn small" data-panel="work">Open</span></div>' +
        '<div class="log-row"><span class="log-time">Yesterday</span><span><span class="log-what faint">Ilya asked for Thursday as well</span>' +
          '<span class="log-detail">You walked past this one. It is still here.</span></span><span class="tag dim">declined</span></div>' +
        '</div></div>' +
        '<div class="footnote"><b>Demonstrating.</b> One of three delivery surfaces, and the only pushed one. The ' +
        'server decides what is released and how much of the backlog surfaces after an absence. Declining does not ' +
        'consume an event &mdash; the one you walked past is still there &mdash; and declining is itself signal that ' +
        'the release policy reads.</div>'
    }
  };

  /* Portraits: high-fidelity in the real game (§12 two-tier character art).
     Here, a lit silhouette that at least holds the palette and the key light. */
  function E_PORTRAIT(who) {
    var warm = who === 'marta' ? '#f0a742' : '#ffb45e';
    var skin = who === 'marta' ? '#6b4a33' : '#7a5540';
    return '<svg viewBox="0 0 148 176" preserveAspectRatio="xMidYMid slice">' +
      '<defs><linearGradient id="bg' + who + '" x1="0" y1="0" x2="0" y2="1">' +
      '<stop offset="0%" stop-color="#241c17"/><stop offset="100%" stop-color="#100c0a"/></linearGradient>' +
      '<radialGradient id="key' + who + '" cx="26%" cy="22%" r="72%">' +
      '<stop offset="0%" stop-color="' + warm + '" stop-opacity=".38"/>' +
      '<stop offset="100%" stop-color="' + warm + '" stop-opacity="0"/></radialGradient></defs>' +
      '<rect width="148" height="176" fill="url(#bg' + who + ')"/>' +
      '<rect width="148" height="176" fill="url(#key' + who + ')"/>' +
      '<path d="M20 176 q10 -52 54 -58 q44 6 54 58 z" fill="#1b1512"/>' +
      '<path d="M20 176 q8 -48 42 -56 l6 56 z" fill="#241b15"/>' +
      '<ellipse cx="74" cy="82" rx="27" ry="33" fill="' + skin + '"/>' +
      '<path d="M47 78 q4 -34 27 -34 q23 0 27 34 q-6 -18 -27 -18 q-21 0 -27 18z" fill="#15100d"/>' +
      '<ellipse cx="74" cy="82" rx="27" ry="33" fill="#000" opacity=".34"/>' +
      '<path d="M52 62 q10 -20 22 -20 q12 0 22 20 q-8 46 -22 52" fill="' + warm + '" opacity=".16"/>' +
      '<path d="M51 70 q6 -28 23 -28" stroke="' + warm + '" stroke-width="2.4" fill="none" opacity=".85"/>' +
      '<path d="M48 92 q3 20 14 27" stroke="' + warm + '" stroke-width="1.8" fill="none" opacity=".5"/>' +
      '<circle cx="64" cy="82" r="2.4" fill="#0b0806"/><circle cx="85" cy="82" r="2.4" fill="#0b0806"/>' +
      '<path d="M66 98 q8 4 15 0" stroke="#0b0806" stroke-width="1.6" fill="none" opacity=".7"/>' +
      '</svg>';
  }

})(window.ETM);
