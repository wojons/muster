# .worktrees/ — Git Worktree Directory

This directory contains git worktrees for parallel development lanes. Each worktree is a full checkout of the repo on a dedicated branch, enabling multiple agents (or humans) to work on different features simultaneously without conflicts.

## Rules

1. **Each worktree MUST have a `WORKTREE.md`** at its root defining the lane's scope, objectives, and rules. Do NOT use `PROMPT.md` for scope overrides — that causes merge conflicts.

2. **Worktree naming**: Use descriptive lowercase names matching the work lane (e.g., `auth-refactor`, `api-v2`). Avoid generic names like `feature-1`.

3. **Branch naming**: Use `axiom/wt-<worktree-name>` convention (e.g., `axiom/wt-auth-refactor`).

4. **One primary work item per worktree**. Secondary items are allowed but must be listed in `WORKTREE.md`.

5. **Merge back to main when done**:
   ```bash
   git worktree remove --force .worktrees/<name>
   git branch -d axiom/wt-<name>
   ```

6. **Keep worktrees synced with main**:
   ```bash
   git -C .worktrees/<name> merge main --no-edit
   ```

7. **Do not cross-pollinate**. Each worktree should only modify files within its lane scope.

## WORKTREE.md Template

```markdown
# WORKTREE — <name>

<1-2 sentence summary of what this lane delivers.>

- **Worktree**: `.worktrees/<name>`
- **Branch**: `axiom/wt-<name>`
- **Primary work item**: `<WORK_ITEM_ID>`
- **Governing spec(s)**: `specs/<NN>-<Name>.md`

## Core Objectives

1. <Objective 1>
2. <Objective 2>

## Scope

**In scope:** <what this lane touches>
**Out of scope:** <what this lane must NOT touch>

axiom:trace work_item=<WORK_ITEM_ID> spec=<spec-ref>
```

## Creating a New Worktree

```bash
git branch axiom/wt-<name> main
git worktree add .worktrees/<name> axiom/wt-<name>
# Write WORKTREE.md in the worktree root
# Update this _index.md
```

## Merging a Worktree Back to Main

```bash
git -C .worktrees/<name> add -A && git -C .worktrees/<name> commit -m "..."
git merge axiom/wt-<name> --no-edit
git worktree remove --force .worktrees/<name>
git branch -d axiom/wt-<name>
```
