# Torn — Player Sentiment Research

Competitive research on [torn.com](https://www.torn.com/), a text-based crime/life-sim MMORPG
running since 2004. Gathered from public reviews and forum discussion. This document records
what players say — it does not draw design conclusions for Escape the Matrix.

**Status:** research snapshot · **Gathered:** 2026-08-02

> **Sourcing caveat, read first.** Reddit blocks Anthropic's crawler and torn.com's own forums
> return HTTP 403 to automated fetch. This snapshot therefore rests on App Store reviews,
> review-site aggregators, and search-surfaced forum excerpts — not on the two richest veins of
> player sentiment. Google Play's listing is JS-rendered and yielded nothing. **Treat the volume
> numbers as thin.** The themes below recur consistently across every source reached, which is
> what gives them weight; the counts do not.

---

## 1. Scores

| Source | Score | n |
| --- | --- | --- |
| App Store — *Torn City* companion app | 3.2 / 5 | 76 ratings |
| MMOs.com | 3.41 / 5 | 384 votes |

The distribution matters more than the average: it is **bimodal**. Five-star evangelists and
one-star flameouts, with little in between. Torn is a game people either organize their day
around or bounce off entirely.

---

## 2. What players like

### 2.1 Depth, and a genuine sandbox

The most repeated praise. Players specialize into combat, stocks, crime, trading, or company
ownership, and the game does not push a lane. Reviewers consistently note that stock markets,
investments, and empire-building sit alongside the crime sim rather than decorating it.

### 2.2 Factions are the retention engine

The strongest single thread in the research, and the most transferable finding.

- Factions run ~100 players, each with distinct culture and identity.
- Many are **more than a decade old**, with storied histories.
- They run weekly wars, organized crimes, and territorial conquest.
- Armories lend gear; factions hand out free consumables (xanax for chains and wars, beer for
  crimes, medical items) so members spend income elsewhere.
- Faction conflicts pull in entire alliances and "redraw the city map for years."

> **The community's own answer to "why do people stay" is social, not mechanical.** Players who
> find a faction they click with stay. Players who don't, leave. Veterans give this as retention
> advice directly.

### 2.3 Low session cost

Playable in short daily bursts rather than marathon sessions. Reviewers frame this as a feature.
Jobs pay once daily; there is no punishment for logging off.

### 2.4 The social layer

One App Store review: *"Imagine if Facebook and BitLife had a baby, this would be the result."*
Long-term players report real friendships worldwide, very active chat and forums, and
player-run games inside the community.

### 2.5 Monetization is respected — by MMO standards

Unusually, this is a **praise** category. Premium currency mostly refills energy; players cannot
buy XP or equipment outright. Reviewers call the premium shop "respectable" and explicitly
contrast it with genre norms. Even critics concede this point.

---

## 3. What players dislike

Ordered by severity, not frequency.

### 3.1 Toxicity, harassment, and doxxing — the most severe complaint

Not merely "chat is mean." Concrete reports:

- An App Store reviewer describes being **doxxed with their real name**, despite no account
  association, and warns parents about the game.
- The described pattern: cross a faction, a job, or an in-game marriage, and *"it's very likely
  you'll have a mob blowing up your every point of contact."*
- Another reviewer reports sustained targeted bounty harassment. Moderators did eventually
  disable the offender's account.
- Newer players report being treated as beneath established players, with veteran forums full of
  complaints about newbies.

Moderation appears to work *eventually*, which is not the same as working.

### 3.2 Scripting and botting — the most corrosive structural complaint

Players describe the game as **"script city."** Top players reportedly automate gym training,
crimes, stocks, and attacking.

- The rule — any script completing an action you did not perform manually — is widely called too
  vague to enforce.
- Enforcement lands unevenly: casino auto-spinners get banned and stock-market access is revoked
  for caught scripters, while broader automation persists.
- Anti-script measures **frustrate legitimate players too**, which is the complaint players
  resent most.

> This is the finding that undermines §2.5. If whales aren't buying power but the top of the
> ladder is *automating* it, competitive integrity leaks out the other end regardless of how fair
> the shop is.

### 3.3 The new-player cliff

- The tutorial does not teach the efficient progression path.
- XP systems are partly hidden; the optimal path is not discoverable in play.
- The barrier into rankings and leaderboards is high — the competition is a 20-year-old
  established playerbase.
- Practical consequence: **new players need an external guide** to progress sensibly.

A forum thread exists titled *"New Player: Combat is making me want to quit."*

### 3.4 Pacing that is slow by design

Jobs pay once daily. Education courses take over a week. Resources regenerate slowly. Defenders
call this the point; detractors call it a wall. Both are accurate — it is a deliberate filter,
and it filters hard.

### 3.5 Endless grind

Cited by five-star and one-star reviews alike: 16-year veterans still have things to grind
toward. Loyalists frame this as longevity; leavers frame it as futility. Same fact, opposite
valence.

### 3.6 Pay-to-win, contested

Not a consensus complaint, but a live argument.

| Position | Argument |
| --- | --- |
| It is P2W | ~$1,000 converts to roughly 5.6bn in-game currency — past the 2bn bank cap and into meaningful passive stock income. Any paid advantage counts. |
| It is not | Donator packs won't make you a top player instantly; the advantage is far milder than comparable MMOs. |

Both camps agree it is mild **by genre standards**.

### 3.7 The app is dated and buggy

- Sign-in and account-creation failures, described by reviewers as an old unfixed bug with
  community-discovered workarounds.
- Attack actions requiring a full app restart to continue.
- Excessive data consumption — assets re-downloaded rather than cached.
- **Third-party clients (Torn PDA, Torn Lite) are reported as faster than the official app.**

It is a two-decade-old browser game in a wrapper, and reviewers say it reads that way.

### 3.8 Account recovery friction

A 5+ year player: the account recovery security measures are *"starting to turn this game into
more trouble than its worth."*

### 3.9 Presentation — criticized from both directions

Text-based with minimal graphics, requiring substantial reading. But some players complain the
**opposite**: that recent development has chased images, animations and visual effects at the
expense of the game's text-based identity.

---

## 4. The core tension

Nearly every complaint is the shadow of a strength:

| Strength | Its shadow |
| --- | --- |
| Systemic depth (§2.1) | The new-player cliff (§3.3) |
| Factions drive retention (§2.2) | Factions enable mob harassment (§3.1) |
| Low session cost / slow pacing (§2.3) | Reads as a wall to newcomers (§3.4) |
| Fair monetization (§2.5) | Undercut by scripters, not whales (§3.2) |

**The one complaint with no compensating virtue is doxxing and coordinated harassment (§3.1).**
Everything else is a tradeoff a designer chose. That one is a moderation failure.

---

## 5. Gaps in this research

Worth closing before treating any of this as settled:

- **Reddit (r/Torn) is unread.** Crawler-blocked. Likely the highest-signal source for churn
  reasons and current sentiment.
- **torn.com forums are unread at source.** Only search-engine excerpts were available; the
  quotes above are secondhand and should be verified before being quoted externally.
- **No Google Play sentiment.** JS-rendered listing; the Android playerbase is unsampled.
- **No player-count trend data.** Whether the playerbase is growing or declining is unknown.
  Torn Stats reported 141,453 registered members as of March 2026, but that measures a companion
  tool, not the game.
- **No time-series.** All sentiment is treated as flat; no read on whether complaints are
  worsening or being addressed.

---

## 6. Sources

- [App Store — Torn City reviews](https://apps.apple.com/us/app/torn-city/id6683303622?see-all=reviews&platform=iphone)
- [MMOs.com — Torn review](https://mmos.com/review/torn)
- [TextBased.com — Torn](https://textbased.com/games/torn/)
- [MMOHuts — Torn review](https://mmohuts.com/review/torn)
- [Into Indie Games — Torn City review](https://intoindiegames.com/reviews/torn-city-review/)
- [Torn forums — "is torn pay to win?"](https://www.torn.com/forums.php?p=threads&f=2&t=16418363&b=0&a=0)
- [Torn forums — Scripting Abuse](https://www.torn.com/forums.php?p=threads&f=2&t=16000718&b=0&a=0rh%3D68)
- [Torn forums — New Player: Combat is making me want to quit](https://www.torn.com/forums.php?p=threads&f=2&t=16478659&b=0&a=0rh%3D36)
- [Torn Wiki — Faction](https://wiki.torn.com/wiki/Faction)
- [Torn Wiki — Organized Crime](https://wiki.torn.com/wiki/Organized_Crime)
- [Google Play — TORN](https://play.google.com/store/apps/details?id=com.ionicframework.tornv2301860&hl=en_US)
