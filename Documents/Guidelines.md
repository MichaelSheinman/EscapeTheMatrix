# Guidelines

Recurring AI mistakes, and the rule that prevents each. This list grows as we
notice more; every change is expected to respect it.

- **No fluff text.** No redundant text that does not need to be in the game.
  The design is as simple and as minimal as possible; if a sentence can be cut
  without losing meaning, cut it.

- **No unrequested functionality.** Do not implement anything that was not
  asked for. A ticket that needs a demonstration vehicle gets the smallest
  possible one, clearly marked as scaffolding — not a stand-in wearing the name
  of a future mechanic (see #7's "Take a day's work" button).

- **DO NOT teach users.** The game makes the player think about
 interactions by living through them; it does not analyze the player's
  choices and hand them the conclusions in advance. A UI that states what
  *could* happen — before any of it has — is doing the player's thinking for
  them (see the playbook's "Where these answers meet" panel, which listed
  every pair of answers that could disagree before a single moment had fired).

- **Features should be tested E2E**. For every new feature that is 
implemented on the backend, you should use chrome and make sure there is actually
a user path to experience the results of this feature. Only commit your code 
after you have verified that this code can be reached and used from the UI.
 

 - **Do not start a server for testing**. There is already a server running in a separate 
 terminal. You can simply use that port in order to test your changes. 


- **Avoid multi-column layout**. Never implement a user interface using a multi 
column layout. It does not look good. The one exception is a list beside the
single item opened out of it — the job board, the playbook's questions — which
is one thing rather than two columns of content, and the list always goes on
the left. See `.split` in ui/components.css; there is no second pattern.Here, I am referring specifically to the spae
where two completely different views appear in two columns. There are exceptions: if 
the left column is a navbar for the right, that's okay. The key is if there are two columns,
they should be connected. 

- **Do not narrate your own discoveries.** A realization you had while
  implementing is not a fact the player is missing. The tell is a sentence that
  only parses as a diff — "you no longer need to…", "leaving is not what taking
  another job needs", "unlike before" — because the player has no changelog and
  never met the old rule. It is the same mistake as teaching, aimed backwards:
  instead of explaining what *could* happen, it explains what *used to*. Nobody
  can be reassured about a confusion they never had. If the discovery is worth
  saying, say it in the commit message or in chat, where it is actually news. 

- **Never show locked features**. There may be hidden features, however, 
there are never "locked" features. Hidden features may be shipped out
later in the game. That's what discoveries are for. But we never show a user
a feature they cannot use.  


-**Do not write tests**. There is no need for a test for this project, and it's 
never been asked for. All projects should be tested using E2E tests by opening 
the browser - writing unit tests is just a waste of time. Unit tests may only 
be written if they are needed to debug something, and must be removed afterwards. 



-**Use the least amount of text possible**. Do not introduce extra views for text.
The AI development has a tendency to develop an extra view with text, which was never
asked for. 


- **Content imitates life, and is never freewritten.** This is a life
  simulator: authored content imitates its real-world counterpart (a job
  posting reads like a real posting), trimmed to game length — the literary
  register is banned. Player-visible prose in `content/*.yaml` is written only
  through the `write-content` skill (writer subagent against the type's
  `content/<type>/SPEC.md`, critic audit, pasted verbatim) — never composed
  inline, not even a one-sentence tweak. Numbers the mechanics own (pay,
  hours, shift times) are never written in prose; the UI injects them.


  **Things need to happen**. This is the most important thing for V1.
  We are still building the app, so we need to build lots of features.
  While highlevel overview and technical describe an ideal end state,
  that's only relevant for later versions. For V1, which we are on, 
  the biggest thing is to make sure there are pathways and we show users
  lots of things happenning and lots of oppurtunities, so they don't get
  bored. We can always lock things later .