# Work Items Index

Inventory of work items under `.memory-bank/work-items/`.

Conventions
- Folder name is the `WORK_ITEM_ID` (Jira key when available, otherwise a stable id).
- Each work item folder typically contains:
  - `meta-planning.md`
  - `plan.md`
  - `plan.yaml`
  - `verification.md` (rolling)
  - `runs/<RUN_ID>/verification.md` (immutable snapshots)

Work items
- `onboarding-01/` — Initial repo bootstrapping (completed)
- `prod-readiness-01/` — Overall production readiness (meta-planning done, parent scope)
- `prod-readiness-01-fix-linux-security-sec/` — **Active**: Fix Linux security gaps (seccomp/SELinux/keychain)
