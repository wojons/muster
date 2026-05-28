# Memory Bank Schema Reference

axiom:trace work_item=muster-bootstrap-01 spec=.memory-bank/_prompt.md

This file defines the canonical note types, frontmatter schema, and template references for the Muster memory bank. Local folder `_prompt.md` files may extend these schemas but must not violate the invariants.

## Note Types

| Type | Purpose | Folder(s) |
|------|---------|-----------|
| `note` | General durable knowledge | projects/, topics/ |
| `decision` | Recorded decision and rationale | decisionLog.md, projects/*/decisions/ |
| `log` | Session/run log | work-items/*/runs/, agents/*/ |
| `message` | Agent-to-agent communication | inbox/ |
| `reflection` | Agent self-improvement entry | agents/*/reflection.md |
| `reference` | Saved content, articles, research | reference/ (personal mode) |
| `capture` | Inbound item (Slack, email, idea) | captures/ (personal mode) |
| `signal` | Ongoing watch item | signals/ (personal mode) |
| `finding` | Adversarial/bug/discovery | findings/ |

## Frontmatter Schema

```yaml
---
mb: v2
type: <note|decision|log|message|reflection|reference|capture|signal|finding>
title: <human readable, 1 line>
created: YYYY-MM-DD
updated: YYYY-MM-DD
tags: [optional, lowercase, kebab-case]
links:
  - <relative paths to related notes>
source:
  - <docs/meeting/ticket/pr>
status: <active|paused|resolved|superseded|archived>  # for decisions/logs only
git:
  commit: <short hash or empty>
  paths: [optional]
  blame_hint: <stable phrase for future search>
---
```

### Required Fields by Type

| Type | Required Fields |
|------|----------------|
| `note` | type, title, created |
| `decision` | type, title, created, source |
| `log` | type, title, created |
| `message` | type, title, created, from, to |
| `reflection` | type, title, created |
| `reference` | type, title, created, source |
| `capture` | type, title, created, source |
| `signal` | type, title, created, status |
| `finding` | type, title, created, source |

## Template References

- Decision template: `.memory-bank/decisionLog.md` (template block at top)
- Inbox message template: `.memory-bank/inbox/_prompt.md`
- Reflection template: `.memory-bank/agents/_prompt.md`
- Finding template: `.memory-bank/findings/_prompt.md`
- Run log template: `.memory-bank/work-items/_prompt.md`
