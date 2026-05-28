# Decision Log

Record of key technical and product decisions. Newest first.

axiom:trace work_item=muster-bootstrap-01

## Template

<!-- Copy this block for each new decision:
### YYYY-MM-DD: Decision Title

**Status:** accepted | proposed | superseded
**Context:** Why did this come up?
**Decision:** What was decided?
**Alternatives considered:** What else was evaluated?
**Consequences:** What are the implications?
-->

---

### 2026-05-28: Memory Bank Bootstrap/Repair Run

**Status:** accepted

**Context:** `/axiom-bootstrap` command executed on Muster repository with extensive pre-existing Axiom infrastructure. The memory bank had 859 entries (mostly sweep/session report files) but was missing several canonical files.

**Decision:** Created missing files (`soul.md`, `README.md`, `_schema.md`), consolidated duplicate `projectbrief.md` → `projectBrief.md`, updated root `_index.md` to list `findings/` and new files, populated bare templates with project-specific content derived from existing `projectbrief.md` and `activeContext.md`.

**Alternatives considered:**
1. Leave bare templates as-is — rejected; blank templates provide zero context to agents.
2. Reorganize sweep files into subfolders — deferred; too risky without explicit user approval, and sweep files may still be referenced by active work items.

**Consequences:**
- Agents now have operator doctrine (`soul.md`) defining priorities and invariants.
- Memory bank schema is documented for consistent note creation.
- Canonical `projectBrief.md` contains actual project context instead of a bare template.
- Duplicate resolved with redirect stub.
