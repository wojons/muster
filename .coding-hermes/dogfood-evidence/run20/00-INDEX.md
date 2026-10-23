# Run 20 — dogfood tick `muster-dogfood-2026-10-23-12-37-51` (HEAD 0bd841fe)

All product legs ran against a **clean detached worktree of HEAD 0bd841fe**
(`git worktree add --detach /tmp/mf20/src 0bd841fe`) built with
`go build -o /tmp/mf20/bin/openapi-cli ./cmd/openapi-cli`, under
`HOME=/tmp/mf20/home` (+ per-leg `MUSTER_AUDIT_LOG`), because the shared
checkout was dirty and in use by a concurrent worker. `bin/openapi-cli` in the
shared checkout reports `v0.1.1-539-g99196361-dirty`, so it is NOT the binary
these numbers describe.

Probe credentials in these logs are **fake** and were replaced with
`FAKE-PROBE-WEBHOOK-SECRET` before the commit. The byte-true facts (value
lengths, sha256 prefixes, `has_secret` booleans) are in
`legC-bytes.txt` / `legC2-webhook-console-bytes.txt` and are unaffected by the
substitution.

| file | what it proves |
|---|---|
| `legA-audit-chain-redaction.txt` | 7 rows with `id`+`hash`+`previous_hash`, links valid; `user` populated (DF-055 fixed); real success bit; `Sensitive fields: redacted`; token in `--header` absent from sink |
| `legB-tamper-silent.txt` | tamper drive: rewritten action and deleted middle record both render silently (rc=0 everywhere) — no verifier surface |
| `legC-webhook-credential-in-sink.txt` | `webhook create/test/delete` write `context.metadata.url` verbatim into the sink; `REDACTED` count 0 |
| `legC-bytes.txt` | byte-true per-field check: `url` len 73, `has_secret=True`, sha12 `4cf91d0d7d52` |
| `legC2-webhook-console-bytes.txt` | byte-true console check: 152-byte stdout / 140-byte list output both carry the full URL (no product masking) |
| `legD-failure-identity-matrix.txt` | 8 failing invocations → recorded actions; multi-word leaves collapse to `command.openapi-cli webhook` |
| `legE-audit-show-vs-log.txt` | `audit show --limit 5` = oldest five; `audit log --limit 5` = newest five, same 2,200-row sink |
| `legF-quickstart-verbatim.txt` | README:66-90 as written: discover/generate rc=0, `get-pet-by-id 1` rc=1 HTTP 404 ×5 |
| `legG-local-demo-generic-request.txt` | documented `request … -o json` works (rc=0, `[]`); 404 emits valid JSON on stdout, human error on stderr |
| `legPerf-measurements.txt` | startup 10.1 ms; audit off/on 9.6/11.0 ms; writer 13→14 ms at 2,000 rows; reader 11.7→27.1 ms at 2,200 rows |
| `demo-gate-33of33.txt` + `demo-time2.txt` | `./demo/run-all.sh` 33/33, 36.7 s wall, DF-056 colour fix (no literal escapes) |
| `doc-drift-census.txt` | go.mod `go 1.26.10` vs 9 doc sites at 1.26.9, 0 at 1.26.10, no guard |
| `install-leg-launch.txt` | bunker battery launch: agent 53097b60 on bunker-mvp, tested HEAD 0bd841fe, evidence path |

Install leg state at close: **fresh-install cell graded OK on rc only** (its tree had
an EMPTY `bin/`, inst.log = 90 module-download lines), so this tick produced the
artifact-level proof itself on a COPY of the harness tree:
`install-artifact.log` — `make build-cli` rc=0 in **203 s**, `bin/openapi-cli`
28,012,809 bytes, `version` = `970720d (970720d)`, `discover` rc=0, audit row written
with a hash. The rest of the battery (`ci-pass`, `docker-deploy`, …) was still running
at close; agent `53097b60` left alive on purpose (`--keep`, ttl 4h). Finish with
`bash ~/.hermes/scripts/bunker-qa.sh collect --evidence /tmp/bunker-qa-evidence-20261023T124416Z-3104346.jsonl`.

Board rows filed by this run: DF-057, DF-050-R2, DF-059, DF-054-R2, DOC-15,
README-058, GAP-033-R3.
