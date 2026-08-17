---
name: change-shared-code
description: Caller-sweep workflow for modifying any existing function, method,
  API handler, type, or YAML schema that is used in more than one place. Use
  whenever changing the signature, behavior, return value, or shape of shared
  code — before making the edit, not after. Triggers include "change this
  function", "update this API", "refactor", "rename", "add a parameter",
  "make X also do Y", or any edit to code in internal/ or content schemas that
  other files reference. Not needed for brand-new code with no callers yet.
---

# Changing shared code

A change tested against one code path (A → B → C) can silently break the
other paths into the same code (D → B → C). Enumerate the paths before
editing, not after.

## Workflow

1. **Find every caller.** Before editing the target, search for all
   references: direct calls, route registrations, YAML fields that map to the
   struct, frontend fetches of the endpoint. Use the Explore agent for a wide
   sweep when the name is common or the usage crosses layers (Go ↔ YAML ↔
   React).

2. **List the distinct paths.** Write out each caller and what that caller
   assumes: argument values it passes, fields it reads from the result, errors
   it expects, ordering or timing it relies on. Two callers that pass
   different shapes of input are two paths, even if the code line is the same.

3. **Check the change against each path.** For every caller from step 2, ask:
   does this caller still get what it assumed? A caller that never exercises
   the new behavior still counts — confirm the change is invisible to that
   caller, don't assume.

4. **Edit once, covering all paths.** Make the change with every caller in
   mind. If one caller needs different behavior than the others, that is a
   design decision to surface to Michael, not something to special-case
   quietly.

5. **Verify beyond the entry point you started from.** When checking in the
   browser, exercise at least one caller that is *not* the one that prompted
   the change.

## Sweep checklist for this codebase

- Go: grep the function/method name across `internal/` and `cmd/`.
- HTTP handlers: a handler change means checking every `web/` fetch of that
  route, not just the screen being worked on.
- Content schemas: a field change in `content/*.go` means checking every YAML
  file that sets the field and every reader of the parsed struct.
- Shared React components/hooks: grep imports across `web/src/`.
