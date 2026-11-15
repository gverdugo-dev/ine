# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root, and
- **`docs/adr/`**: read ADRs that touch the area you're about to work in.

Neither exists yet. If they don't, **proceed silently**. Don't flag their absence; don't suggest
creating them upfront. The `/domain-modeling` skill creates them lazily when terms or decisions
actually get resolved.

## Where the vocabulary lives today

The terms are the INE's own, as the Tempus3 API names them: an **operation** (an
statistical operation such as the IPC or the EPA) has **tables**; a table has **series**; a
series is defined by **variables** and holds **points** (one per period). The Go types in
`types.go` carry those names in English (`Operation`, `Table`, `Series`, `Variable`, `Point`);
reuse them and do not invent synonyms such as "report" or "dataset" in code. The decisions
already taken are listed in `CLAUDE.md` and are the seed of the first ADRs.

## File structure

Single-context repo: one `CONTEXT.md` and one `docs/adr/` at the root, numbered `NNNN-slug.md` from
`0001`. There is no `CONTEXT-MAP.md`.

```
/
├── CONTEXT.md          (not created yet)
├── docs/adr/           (not created yet)
├── cmd/ine/
└── fetch/
```

## Flag ADR conflicts

If your output contradicts an ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0001, but worth reopening because…_
