/* Escape the Matrix — the cast.

   There is nobody in this city who is nobody. Every figure on a pavement has
   a name, a handle the boards print, a Karma, a level and a reason to be
   standing where they are standing — and three things you can ask them.

   That is a decision rather than an oversight of scale. A crowd of anonymous
   coats makes a street look busy and teaches the player that people are
   scenery; the moment one of them can be asked how they got to 64,891, the
   board in §3.5 stops being a scoreboard and becomes a set of lives that
   happen to be measurable. Ten of them is enough to hold every end of the
   spectrum at once: a man the composite has left at zero, a woman five weeks
   in with eleven, and a judge three orders of magnitude above both.

   Answers are hardcoded and there is exactly one per question (§7: no runtime
   LLM, ever). What a person says is content, checked into the repo, the same
   as a challenge or a posting.

   Two numbers, and they are not the same number:
     · karma — the composite from §3.5, the fine-grained thing that moves
     · level — what the boards round that to in public

   The world figure and the dossier live in one object here and are split at
   `E.cast()`: the actor the renderer walks around is small, and the story,
   the questions and the answers are looked up by id when somebody clicks. */

window.ETM = window.ETM || {};

(function (E) {
  'use strict';

  var PEOPLE = [

    /* ═══════════════════════════════════════════════════════ TANNER ROW */

    /* The pair outside the diner. The argument the world-card drops you into
       is theirs, and both of them will tell you their half of it. */
    {
      id: 'marta', name: 'Marta', handle: 'marta_o',
      note: 'Rowan Court, two doors from Kez',
      karma: 1412, level: 21, scene: 'street',
      role: 'Runs the roster at the port yard',
      story: 'Twenty-two years of keeping the shift book for the port yard, which is ' +
             'the kind of work that is invisible until the week it is done badly. She ' +
             'is the reason a shift you did not turn up for gets marked the way it ' +
             'gets marked, and she is standing outside the diner because Kez sold a ' +
             'van that was half hers.',
      figure: { x: 24.6, y: 40.2, gesture: true, color: '#6a4f3c', skin: '#a9724a',
                hair: '#2b201a', scale: 0.96, phase: 0.9 },
      lines: [
        'You did not hear that from me, and you did hear it.',
        'It was half mine. That is the whole of it.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Twenty-two years of a shift book that is right. Nobody hands you anything ' +
             'for that &mdash; it accrues, the way rust does. The week I was ill it went down, ' +
             'and it took four months to come back.' },
        { q: 'What is the argument about?',
          a: 'He sold the van. It was half mine, he did not ask, and he will tell you he ' +
             'did me a favour because the brakes were going. Both of those are true. ' +
             'That is why we are still standing here three weeks later.' },
        { q: 'Is there work at the port?',
          a: 'There is always work at the port. There is not always work worth taking, ' +
             'and I am not going to be the one who tells you which is which. Look at what ' +
             'a shift pays and then look at what it costs you to be there.' },
      ],
    },

    {
      id: 'kez', name: 'Kez', handle: 'kez_ndl',
      note: 'the other half of that argument',
      karma: 1388, level: 19, scene: 'street',
      role: 'Drives for a haulier off the port road',
      story: 'Fourteen years of runs and not one missed, which is a fact he will give ' +
             'you before his name. He got two thousand for the van and told nobody, and ' +
             'Marta heard it from Ilya on a Saturday, which is the part of it he cannot ' +
             'argue his way out of.',
      figure: { x: 25.7, y: 40.6, gesture: true, color: '#4c5f56', skin: '#7d5236',
                hair: '#1d1713', scale: 0.98, phase: 2.2 },
      lines: [
        'I am not saying I was right. I am saying the brakes were going.',
        'Two thousand. It was worth two thousand and she knows it.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Fourteen years without missing a run. It is not a virtue, it is an ' +
             'attendance record. The board cannot tell the difference between those two ' +
             'things, and most people cannot either.' },
        { q: 'Why did you sell the van?',
          a: 'The brakes were going and neither of us had the eight hundred. I got two ' +
             'thousand for it and she found out from Ilya. The selling was fine. The ' +
             'finding out from Ilya is the part I did wrong.' },
        { q: 'What would you do differently?',
          a: 'Told her on the Tuesday. Everything else was going to happen exactly the ' +
             'way it happened.' },
      ],
    },

    {
      id: 'nadia', name: 'Nadia', handle: 'nadia_b',
      note: 'remembers what you were wearing, not what you said',
      karma: 640, level: 14, scene: 'street',
      role: 'Runs the diner on Rowan St',
      story: 'Eleven years of feeding this street at a price it can pay. She is the ' +
             'reason anybody on Rowan St can be described to anybody else: she does not ' +
             'know your name, she knows your order and your coat, and that turns out to ' +
             'be most of knowing somebody.',
      figure: { x: 23.2, y: 40.9, color: '#5a4a62', skin: '#c8a184', hair: '#3b2a20',
                scale: 0.95, phase: 1.7 },
      lines: [
        'Three weeks of this, and neither of them has ordered anything.',
        'I know the coat. I do not know the name. It is usually enough.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Eleven years of feeding people at a fair price. It moves about two points ' +
             'a month. Anybody who tells you they jumped a thousand in a season is ' +
             'selling you something, and it is not food.' },
        { q: 'How do you know everybody?',
          a: 'I do not know them. I know what they order and what they were wearing, and ' +
             'it turns out that is most of knowing somebody. People change their minds ' +
             'about who they are. They do not change their coat.' },
        { q: 'What do you hear?',
          a: 'Everything, twice, and half of it wrong. Those two have been having the ' +
             'same argument in my window since the van went, and there is a man at the ' +
             'market who could end it in a sentence and is enjoying himself too much.' },
      ],
    },

    {
      id: 'ilya', name: 'Ilya', handle: 'ilya_v',
      note: 'thirty years on the same six feet of pavement',
      karma: 2104, level: 26, scene: 'street',
      role: 'The middle stall at the Saturday market',
      story: 'Thirty years on the same six feet of pavement, selling whatever came off ' +
             'the boats on Thursday. The stall is not what earns him his standing on ' +
             'this street: being findable for thirty years is.',
      figure: { x: 53.4, y: 58.0, color: '#40514c', skin: '#b98a5e', hair: '#6b6459',
                hat: '#34302a', scale: 0.98, phase: 0.4 },
      lines: [
        'Saturday. I am here on Saturday, same as I was last Saturday.',
        'It came off a boat on Thursday. That is all I will swear to.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Thirty years on the same six feet of pavement. People know where I will be ' +
             'on a Saturday, and being findable for thirty years is worth more on those ' +
             'boards than anything I have ever sold off this table.' },
        { q: 'What do you sell?',
          a: 'Whatever came off the boats on Thursday. Ask me on a Saturday and I will ' +
             'have it. Ask me on a Tuesday and I am a man standing in an empty market.' },
        { q: 'Who should I know here?',
          a: 'Marta, because the roster goes through her. The judge, because everybody ' +
             'is polite to her and nobody talks to her. And Boris &mdash; not because he can ' +
             'do anything for you, but because you will find out what you are like.' },
      ],
    },

    /* Jamel trains at Ironworks and does not go far from it. The gym door is
       at (82, 39.1), so his whole day is the pavement in front of it: out to
       the corner, back to the step, stand for a while. Built heavy and in a
       vest, because at this size the only thing that says what a man does all
       week is his silhouette. */
    {
      id: 'jamel', name: 'Jamel', handle: 'jamel_t',
      note: 'twenty-six, and every point of it is health',
      karma: 57, level: 8, scene: 'street',
      role: 'Six mornings a week at Ironworks',
      story: 'Twenty-six, no money, and he has moved four times this year, so four of ' +
             'the six parts of the composite are doing nothing for him at all. The one ' +
             'he can move he moves six mornings a week, on the theory that health is the ' +
             'only part of it nobody can take back off you.',
      figure: { path: [[86.4, 40.4], [78.2, 40.4], [82.0, 40.2]], speed: 1.15, linger: 2.6,
                color: '#3a4450', skin: '#8a5a3c', hair: '#241a14',
                build: 'heavy', sleeves: 'bare', arms: 'out', scale: 1.06, phase: 1.4 },
      lines: [
        'Two more sets and then I&rsquo;m done. Same as yesterday.',
        'You training today, or just walking past it again?',
        'Twelve weeks. Twelve. That&rsquo;s the whole trick.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Fifty-seven, and every point of it is health. No money, and I have moved ' +
             'four times this year, so two parts of six are carrying the whole thing. ' +
             'Give it two years and ask me again.' },
        { q: 'Why six mornings?',
          a: 'Because five is a plan and six is a habit. The one you skip is always the ' +
             'one you were going to do.' },
        { q: 'Is the gym worth it?',
          a: 'Forty a month, and it is the only forty I spend that pays me back with ' +
             'interest. Not in Karma. In the rest of it.' },
      ],
    },

    /* Lindsey sits at the County Court, whose door is at (56, 24.1). She walks
       a circuit of the block between listings rather than pacing one strip of
       pavement — the gown reaches the floor, which is the read at this size.
       64,891 is what the board looks like from a bench: seven times the top of
       the ladder the player can currently see. */
    {
      id: 'lindsey', name: 'Lindsey', handle: 'lindsey_a',
      note: 'the biggest number anybody here stands next to',
      karma: 64891, level: 71, scene: 'street',
      role: 'Sits at the County Court',
      story: 'Thirty years on a bench, eleven of them in other countries. She is the ' +
             'highest number anybody on Rowan St has ever stood next to, and the only ' +
             'person on this street who will tell you plainly that the number took a ' +
             'whole life and cannot be hurried.',
      figure: { path: [[52.6, 24.9], [61.4, 24.9], [61.4, 25.6], [53.0, 25.6]],
                speed: 0.95, linger: 1.8,
                color: '#26242c', skin: '#c8a184', hair: '#4d4038',
                build: 'robe', scale: 1.0, phase: 0.6 },
      lines: [
        'Ten past. They will wait, and they will be told why.',
        'It is not what you meant. It is what you wrote down.',
        'Both of them think they are the reasonable one. They always do.',
      ],
      ask: [
        { q: 'How did you get so much Karma?',
          a: 'I sat in courts in eleven countries before this one. Every one of them was ' +
             'certain it had human beings worked out, and every one of them was wrong in ' +
             'a different and useful way &mdash; so I learned eleven things instead of ' +
             'defending one. You do not get a number like mine by being right. You get ' +
             'it from thirty years of being corrected in public and writing down what ' +
             'corrected you.' },
        { q: 'What do you actually do?',
          a: 'I listen to two people who are each completely certain, and I write down ' +
             'which of them the rules cover. It is a smaller job than it looks from the ' +
             'back of the room, and it takes a whole life to do quietly.' },
        { q: 'How do I raise mine?',
          a: 'Not quickly. Pick the thing you will still be doing in ten years and do it ' +
             'where people can see you. The board is a slow record of what you actually ' +
             'are, and it is very hard to lie to for long.' },
      ],
    },

    /* Boris is on the same pavement as the market and the paper stand, which
       is where the food is. He has nothing on above the waist and nothing
       under him: Karma 0 is not a rounding — it is what the composite does to
       somebody with no health, no money and nowhere that counts as staying.
       He is the only one who says things at you rather than near you. */
    {
      id: 'boris', name: 'Boris', handle: 'boris_k',
      note: 'asks everyone for food. Not advice',
      karma: 0, level: 1, scene: 'street',
      role: 'Sleeps behind the market',
      story: 'The yard closed, and then the room went, and then the rest of it went in ' +
             'an order he does not remember. Four months, start to finish. He is the ' +
             'floor of the same composite the judge is at the top of, which is the ' +
             'argument for reading the number as a life rather than a score.',
      figure: { path: [[41.0, 54.9], [50.4, 54.7], [58.6, 55.1], [47.2, 54.8]],
                speed: 0.8, linger: 1.4,
                color: '#5b5340', skin: '#b8875e', hair: '#3a2f26',
                shirtless: true, gesture: true, scale: 0.97, phase: 2.1, barkEvery: 7 },
      lines: [
        'Nice coat. Bet it cost more than a week of eating.',
        'You&rsquo;ve got a face like a man with a sandwich in his bag.',
        'They put me on a board too. Zero. Nought. A perfect score, if you think about it.',
        'Spare anything? Food, I mean. Not advice. I have had all the advice.',
        'Everyone here is climbing something. I am the ground floor. You are welcome.',
        'The judge walks past me twice a day and has never once looked down.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Zero. Not low &mdash; zero. No health, no money, and nowhere that counts as ' +
             'staying, and the number is the honest sum of those three. It is the only ' +
             'thing in this city that has never once lied to me.' },
        { q: 'What happened?',
          a: 'The yard closed. Then the room went. Then the rest of it went in an order ' +
             'I could not tell you now. Four months, the whole way down. Everybody thinks ' +
             'it takes years.' },
        { q: 'What do you want?',
          a: 'Food. Not advice. I have had all the advice, from people who were entirely ' +
             'correct, and here I am.' },
      ],
    },

    {
      id: 'halina', name: 'Halina', handle: 'halcyon_r',
      note: 'covered your shift on Tuesday',
      karma: 11, level: 3, scene: 'street',
      role: 'Five weeks on Tanner Row',
      story: 'Arrived five weeks ago from somewhere nobody here has been. She took your ' +
             'Tuesday shift at the port yard because Marta asked her to and she had the ' +
             'day, which is the only reason you were not marked absent twice. She is one ' +
             'rank below you on the board and has not noticed.',
      figure: { path: [[10.4, 40.4], [22.6, 40.4], [12.8, 40.4]], speed: 1.2, linger: 2.2,
                color: '#48505e', skin: '#d0a87e', hair: '#4d3a2a',
                bag: '#6b5540', scale: 0.93, phase: 3.1 },
      lines: [
        'Five weeks. Everybody here already knows that about me.',
        'It was a Tuesday. It was not a favour, it was a Tuesday.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Eleven, and I have been here five weeks. It is a number that means nobody ' +
             'has any reason to think anything about me yet. I would rather that than a ' +
             'bad one, which is what you get for arriving loudly.' },
        { q: 'Why did you cover my shift?',
          a: 'Marta asked and I had the day. You would have been marked absent twice, ' +
             'and it is the second one that sticks to you. You can get it back to me on ' +
             'some Tuesday when it is inconvenient.' },
        { q: 'How is Tanner Row?',
          a: 'Cheap, and everybody watches. Those are the same fact, not two facts. I ' +
             'came from a place where nobody watched and the rent was double.' },
      ],
    },

    /* ═══════════════════════════════════════════════════════ RIVERFRONT */

    {
      id: 'tsvetan', name: 'Tsvetan', handle: 'tsvetan_k',
      note: 'top of the board. Twenty-six years on this side',
      karma: 9104, level: 52, scene: 'riverfront',
      role: 'Prices risk on the fourth floor',
      story: 'The name at the top of the overall board, walking the quay at four in the ' +
             'afternoon like anybody else. Twenty-six years in the same postcode, which ' +
             'is most of the number: the composite pays for staying still, and this is ' +
             'the most expensive place in the city to stay still in.',
      figure: { path: [[31.2, 10.2], [17.4, 10.2], [28.6, 10.2]], speed: 1.5, linger: 2.0,
                color: '#39404e', skin: '#c39468', hair: '#5c5a50', scale: 1.0, phase: 1.1 },
      lines: [
        'Twenty-six years and I still take the long way round the block.',
        'It is a good river. It is not a good reason.',
      ],
      ask: [
        { q: 'How did you get so much Karma?',
          a: 'Nine thousand and change, and the boring truth is that most of it is money ' +
             'and health, and I got both by living in the same postcode for twenty-six ' +
             'years. This side of the river will do that for you. It will also cost you ' +
             'every friend who could not afford to be here.' },
        { q: 'Is the Riverfront worth moving to?',
          a: 'For the boards, yes, immediately, and anybody who tells you otherwise is ' +
             'being polite. For you &mdash; ask me what I do here in the evenings. I have had ' +
             'twenty-six years to prepare an answer and I still do not have a good one.' },
        { q: 'What do you do?',
          a: 'I price other people&rsquo;s risk for a firm on the fourth floor. It pays what ' +
             'it pays because somebody has to be wrong for any of us to be paid at all.' },
      ],
    },

    {
      id: 'yannick', name: 'Yannick', handle: 'yannick_m',
      note: 'has an arrangement in mind, and is relaxed about it',
      karma: 3318, level: 33, scene: 'riverfront',
      role: 'Fourth-floor office, Kestrel &amp; Vane',
      story: 'The man who slows down when he recognises you. He is not a villain and ' +
             'that is the difficulty: he is relaxed, he is generous, and the favour he ' +
             'is offering has a second half that arrives about six weeks later, by ' +
             'which time your own rules will have an answer ready for it.',
      figure: { x: 24.4, y: 11.2, gesture: true, color: '#2f3a44', skin: '#9c6a44',
                hair: '#241a14', hat: '#3b3630', scale: 1.0, phase: 3.0 },
      lines: [
        'You did me a favour last week. Let me do you one back.',
        'Nobody is asking you where anything came from. That is the point of it.',
      ],
      ask: [
        { q: 'How did you get your Karma?',
          a: 'Three and a bit thousand, and about four hundred of it I would not want ' +
             'itemised. It counts exactly the same as the rest. That is either the best ' +
             'or the worst thing about the whole system and I have never decided which.' },
        { q: 'What is the arrangement?',
          a: 'You did me a favour, so I do you one back and you do not ask where it came ' +
             'from. Then in six weeks somebody asks you for the same thing, and you say ' +
             'yes, because you already took one. That is the entire shape of it. I have ' +
             'just told you, which is more than anybody told me.' },
        { q: 'Should I take it?',
          a: 'No. And your rules will say yes &mdash; mine did, and I never went back and ' +
             'looked at why. Go and read what you actually wrote down, and then decide ' +
             'whether it is still yours.' },
      ],
    },
  ];

  /* Looked up by id when somebody is clicked: the dossier stays here rather
     than riding around on an actor that only needs to know where its feet are. */
  var BY_ID = {};
  PEOPLE.forEach(function (p) { BY_ID[p.id] = p; });

  /* What the renderer walks around. Everything the world needs to draw them,
     name them on hover and let them talk — and nothing it does not. */
  E.cast = function (scene) {
    return PEOPLE.filter(function (p) { return p.scene === scene; }).map(function (p) {
      var a = { id: p.id, name: p.name, note: p.note, karma: p.karma, lines: p.lines };
      Object.keys(p.figure).forEach(function (k) { a[k] = p.figure[k]; });
      return a;
    });
  };

  E.PEOPLE = PEOPLE;
  E.person = function (id) { return BY_ID[id] || null; };

})(window.ETM);
