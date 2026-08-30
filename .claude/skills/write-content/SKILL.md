---
name: write-content
description: Mandatory pipeline for writing or editing ANY player-visible prose in content/*.yaml (job postings, dialogue, event lines, reactions, environment notes). The main agent never composes content prose inline — a writer subagent drafts against the type's SPEC, a critic audits, and the result is pasted verbatim. Invoke before touching any prose field, even a one-sentence tweak.
---

# Write content

The game is a life simulator: every content type imitates its real-world counterpart (a job posting reads like a real posting), trimmed to game length.
The main agent should not freewritte under implementation momentum, as we've experienced issues with banned/unclear words showing up in the past. This pipeline is mandatory for any change to a player-visible prose field in `content/*.yaml`, including one-sentence tweaks. Mechanical fields (keys, tiers, tolls, weights, places, numbers) are authored directly and never by a writer subagent.

## Process

1. **Identify the content type** (job, environment, conflict, situation,
   event, challenge dialogue…) and read `content/<type>/SPEC.md`. No SPEC yet?
   Stop and write one first, with Michael — its decisions (real-world
   counterpart, payload, budgets, banned tells) are his. `content/jobs/SPEC.md`
   is the model.

2. **Build a facts sheet** — the mechanical truth the prose must encode:places, environment character, and any constraint the
   simulation owns. Never put mechanically-owned numbers (pay, hours, times) in the brief as things to write; the UI injects them.

3. **Spawn a writer subagent** whose ONLY inputs are the SPEC text and the
   facts sheet. Never show it existing content files — old prose teaches the
   old register. It returns structured output (YAML) with no commentary.

4. **Spawn a critic subagent** to audit the draft against the SPEC: realism,
   fluff tells, payload, toll legibility (present but unmarked — flag both
   illegible and too loud), mechanics leakage, register drift. A failing piece
   gets ONE rewrite round through the writer. Still failing → stop and surface
   the failure to Michael; do not settle it quietly.

5. **Paste verbatim.** The main agent copies the writer's strings into the
   YAML unchanged — it may fix YAML mechanics (indentation, quoting), never
   wording. If a string won't fit the structure, that goes back through the
   writer.

6. **First batch of a type**: present the whole diff to Michael before
   commit, and fold his corrections into the SPEC as banned tells and worked
   examples. After a SPEC has survived a batch, critic-only is enough.

## The words live in the content, not in the component

**Challenge text is authored in the YAML, never in a React component** — and
the same instinct applies to every content-bearing screen. Components lay out
and render; they supply no sentences of their own.

A SPEC governs `content/`, so prose hardcoded in `web/src` or `ui/` is
invisible to every writer and critic in this pipeline. That is how a challenge
question carried "None of these is the right one. Each of them goes somewhere
else, and you will not be told which was which." above it, and how "Commit to
this" appeared above all four options at once (2026-08-10). The first was also
false — an interview has a better answer — which is the deeper reason for the
rule: a sentence that cannot vary with the content it sits on will eventually
contradict it.

So whenever a content type is written or audited, open the component that
renders it and read the strings it supplies itself. For each one, either delete
it or move it into the content:

- **Does it repeat what the screen already shows?** Four cards under a question
  do not need a label each saying they are choices. Delete.
- **Does it explain how the game works?** That is teaching, in the one place
  nobody is checking for it. Delete.
- **Does it say something an author would want to vary?** Then it is a content
  field that was never authored. Move it.

## Hard rules

- The loader's budgets are boot-fatal; run the content through a build
  (`make check`, plus a boot of the owning loader) before calling it done.
- Every rewrite lands together with any schema change — the tree boots green
  or the work is not finished.
- Verify the result where the player meets it (the browser), not in the YAML.
