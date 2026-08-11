---
name: write-content
description: Mandatory pipeline for writing or editing ANY player-visible prose in content/*.yaml (job postings, dialogue, event lines, reactions, environment notes). The main agent never composes content prose inline — a writer subagent drafts against the type's SPEC, a critic audits, and the result is pasted verbatim. Invoke before touching any prose field, even a one-sentence tweak.
---

# Write content

The game is a life simulator: every content type imitates its real-world
counterpart (a job posting reads like a real posting), trimmed to game length.
The main agent freewriting under implementation momentum is how the banned
literary register gets back in — so it never freewrites. This pipeline is
mandatory for any change to a player-visible prose field in `content/*.yaml`,
including one-sentence tweaks. Mechanical fields (keys, tiers, tolls, weights,
places, numbers) are authored directly and never by a writer subagent.

## Process

1. **Identify the content type** (job, environment, conflict, situation,
   event, challenge dialogue…) and read `content/<type>/SPEC.md`. No SPEC yet?
   Stop and write one first, with Michael — its decisions (real-world
   counterpart, payload, budgets, banned tells) are his. `content/jobs/SPEC.md`
   is the model.

2. **Build a facts sheet** — the mechanical truth the prose must encode:
   tier/tier mechanics, tolls or factor moves (with the unmarked-encoding
   rules from the SPEC), places, environment character, and any constraint the
   simulation owns. Never put mechanically-owned numbers (pay, hours, times)
   in the brief as things to write; the UI injects them.

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

## Hard rules

- The loader's budgets are boot-fatal; run the content through a build
  (`make check`, plus a boot of the owning loader) before calling it done.
- Every rewrite lands together with any schema change — the tree boots green
  or the work is not finished.
- Verify the result where the player meets it (the browser), not in the YAML.
