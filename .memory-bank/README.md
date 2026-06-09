# Memory Bank — Muster

axiom:trace work_item=muster-bootstrap-01

The Memory Bank is the durable, self-describing flat-file system that preserves project context across agent sessions. It is the source of truth for what has been decided, what is being worked on, what works, and what remains to be done.

## Navigating

Start with the **[root index](./_index.md)** — it maps everything in here.

## Core Files

| File | What it holds |
|------|---------------|
| `projectBrief.md` | Project scope, goals, definitions of done |
| `productContext.md` | Why this exists, UX goals, domain terms |
| `soul.md` | Operator doctrine: priorities, invariants, defaults under pressure |
| `systemPatterns.md` | Architecture, design patterns, component connections |
| `techContext.md` | Languages, frameworks, build tools, deployment |
| `activeContext.md` | Current focus, open questions, active constraints |
| `progress.md` | What works, what's next, known issues |
| `decisionLog.md` | Append-only decisions and rationale |
| `TODO.md` | Working execution backlog |

## Key Directories

| Directory | Purpose |
|-----------|---------|
| `work-items/` | Per-ticket folders with plans, evidence, run history |
| `implementation-plans/` | Phase-level plans aligned to TODO |
| `agents/` | Agent profiles, preferences, reflections |
| `inbox/` | Agent-to-agent messages |
| `findings/` | Durable lessons learned (adversarial, anti-patterns, process) |
| `projects/` | Project-specific durable context |
| `topics/` | Cross-project evergreen knowledge |
| `best-practices/` | Reusable engineering playbooks |
| `known-gaps/` | Known gaps scorecards and gap TODOs |
| `requests/` | Durable user requests before promotion to specs/work-items |

## Conventions

- **No secrets**. Redact as `[REDACTED]`.
- **Trace it**. Every fact should link to a spec, commit, or ticket.
- **Update indexes**. When you create or move a file, update the relevant `_index.md`.
- **Map of maps**. Read the root index first, then drill down.
