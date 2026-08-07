# Challenges

One directory per family, one directory beneath it per variant — and those two
directory names *are* the challenge's identity (technical.md §5.3). Nothing
inside the files repeats them, so a rename is a `git mv` rather than an edit in
two places that can half-happen.

```
content/challenges/resolve_conflict/a/
  challenge.yaml     # mechanic type, availability, the event that opens it
  dialogue.yaml      # whatever the mechanic type reads — its schema, its file
```

`challenge.yaml` is the one file every challenge has, whatever runs it:

```yaml
name: The Argument on the Stoop
mechanic: dialogue_branch      # a registered mechanic type — code, not content
opens: a-knock-at-the-door     # optional: the awaiting event this is behind
where:                         # optional: where its own entry point stands
  districts: [east_side]
  zones: [doorway]
```

A challenge names `opens`, `where`, or both. Naming neither is a failed build:
technical.md §4 keeps availability and offer as separate concepts, and a
definition with neither is content nobody can reach.

Everything else belongs to the mechanic type, which reads its own documents out
of this directory and validates them against its own schema. **New content in an
existing type is data only. A new mechanic is code** (§5.2) — and the discipline
is to extract a type on the *second* instance, not the first, so the first
challenge of a shape is written as Go.

There are no challenges here yet. The framework that runs them is
`internal/challenge`; the first one to be authored is issue #24.
