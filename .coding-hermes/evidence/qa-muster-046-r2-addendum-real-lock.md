# QA-MUSTER-046 R2 — addendum: the shipped sweep acted on a REAL stale handle

Post-close measurement, tick `muster-foreman-2026-11-22-04-52-38`, 2026-11-22T06:0xZ.
The main evidence file `qa-muster-046-r2-20261122.md` is unchanged (its sha256
`2b6dbb60…fec09545` still binds row QA-MUSTER-046).

## What was found in the live lock directory

`/tmp/bunker-qa-orphans` held one real handle — not from this tick:

```
format=1
agent=52e2b429
tick_pid=599862        (dead: kill -0 599862 -> not alive)
server=bunker-las-02
evidence=/home/kara/.hermes/qa/dexdat-core-qa-2026-11-21-23-52-48/evidence/qa-evidence.jsonl
created=2026-11-22T00:07:15Z   (age ~5.5h > the 2h TTL)
```

## What the daemon says

`bunker list --server bunker-las-02 --status all` → `No agents found.`
The agent does not exist, so this is a ghost handle for a foreign lane's dead
tick — the exact class that used to be destroyed blindly.

## What the shipped sweep did

Running the functions extracted from the DEPLOYED harness
(`~/.hermes/scripts/bunker-qa.sh`, sha256 `ef803f52…109f0e9`) against the real
lock directory and the real `bunker` CLI:

```
locks before: 52e2b429.lock
  WATCHDOG: drop stale handle agent=52e2b429 lock=/tmp/bunker-qa-orphans/52e2b429.lock reason=agent-absent (no destroy issued)
locks after:
```

Arm 2 in production: list first, daemon proves absence, handle dropped, **zero
destroy calls**. The pre-fix sweep would have issued
`bunker destroy 52e2b429 --server bunker-las-02` for an agent that does not
exist and then kept the lock because the list still showed it.

Script: `~/.hermes/cache/scratch/qamuster046-real-lock.sh`.
