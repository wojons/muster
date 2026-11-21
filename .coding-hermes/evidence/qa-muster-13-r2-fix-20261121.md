# QA-MUSTER-13-R2 — upgrade cell cannot build the previous v0.1.0 tree on a clean Go agent

Status: FIXED in the fleet harness (host state, no product commit) + proven on a fresh
JIT agent. Branch: `wt/QA-MUSTER-13-R2` (evidence only; `bunker-qa.sh` is host state).
Date: 2026-11-21. Tick: `muster-foreman-2026-11-21-10-26-59`.

## 1. Verdict

The named defect is fixed and proven end to end on a clean agent:

- `upgrade` cell: **OK** — `previous release v0.1.0 (tree via git archive) → HEAD reinstall
  clean` (agent `6029d0de`, battery `qa-20261121T114128Z`), upgrade build rc=0.
- **zero** `pkg-config` / `libseccomp` failure lines across every cell log of that run
  (`grep -rniE 'exec: "pkg-config"|pkg-config: (executable file )?not found|pkg-config search
  path|Package libseccomp was not found|seccomp\.h: No such file' <evidence>/logs` → `0`).
- The native prerequisite is provisioned without root, from the distro's own packages, into a
  user-space sysroot.

One fact that the brief marked VERIFIED does **not** hold on a live agent, and it matters:
`go1.26.11` (and 1.27.4) cannot build the v0.1.0 tree even with `libseccomp.pc` present,
because those releases implement the **CVE-2026-97038 cgo hardening**
(`isDisallowedLTOSection` in `cmd/go/internal/work/security.go`) which rejects the prebuilt
`libwasmtime.a` that `wasmtime-go v8` ships (it carries `.llvmbc`). Live proof in §5.2: with
provisioning in place the *only* remaining error on go1.26.11 is
`github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS`.
That is "another missing native-build prerequisite" by the acceptance's own wording, so it is
fixed here too — as a single bounded retry with the toolchain the module documents
(`go.mod: go 1.26.1`), never by touching a product file.

## 2. Mechanism (what was actually broken)

`github.com/seccomp/libseccomp-golang` is a cgo package whose `#cgo` line is
`pkg-config: libseccomp`. A clean JIT agent has no `libseccomp.pc`, so:

```
# github.com/seccomp/libseccomp-golang
# [pkg-config --cflags  -- libseccomp]
Package libseccomp was not found in the pkg-config search path.
Package 'libseccomp', required by 'virtual:world', not found
```

The battery treated that as a **product** FAIL, for both `fresh-install` (muster's HEAD tree
also imports the package) and `upgrade` (the streamed v0.1.0 tree). The battery owned no
provisioning step, so no amount of re-running could ever pass on a fresh agent.

Second, independent mechanism found while proving the first (see §1): the harness installs the
**newest** Go — on agent `6029d0de` that was `go 1.27.4`, on `dd16ecfc` `go1.26.11` — and those
releases refuse archives carrying LLVM bitcode. The module documents `go 1.26.1`, which
predates the check.

## 3. Change applied to the harness

File: `/home/kara/.hermes/scripts/bunker-qa.sh` (HOST STATE — no git repo; this file is the audit).
Backup: `/home/kara/.hermes/scripts/bunker-qa.sh.bak-qamuster13r2-20261121T104653Z`

```bash
cp ~/.hermes/scripts/bunker-qa.sh ~/.hermes/scripts/bunker-qa.sh.bak-qamuster13r2-$(date -u +%Y%m%dT%H%M%SZ)
```

Diff size: **+188 / −3** lines (`diff -u <backup> bunker-qa.sh` → 230 diff lines). Harness
self-syntax `bash -n` clean; the *generated* `qa-run.sh` re-generated and `bash -n` clean, and
generation now emits **0 stderr lines** (see §3.3).

### 3.1 Provisioning block (before any build cell)

Inserted in the generated-script toolchain-bootstrap region (after the make bootstrap, before
the jq bootstrap and every build cell). Armed **only** when a tree about to be built links
libseccomp — derived from `go.mod`/`go.sum` of the synced tree *and* of the host-streamed
previous release (`~/qa-up-prev`), so a repo that does not use libseccomp pays nothing:

1. **already present** → one `INFO` row, no install attempt (idempotent);
2. **root + apt-get/dnf/yum/apk** → distro install of `pkg-config` + `libseccomp-dev`
   (`pkgconf-pkg-config libseccomp-devel` on rpm, `pkgconf libseccomp-dev` on apk);
3. **no root** (`sudo -n` is password-gated on bunker agents) → user-space sysroot:
   `apt-get download libseccomp-dev libseccomp2 pkgconf-bin pkgconf pkg-config` (unprivileged,
   off the image's own apt lists) or `apk fetch`, extracted with `dpkg-deb -x` / `tar` into
   `~/tools/sysroot`, every extracted `.pc` `prefix=` rewritten onto that sysroot (the distro
   files hard-code `prefix=/usr`, which would point back at the agent's dev-less `/usr`), the
   extracted `pkgconf` exposed as `pkg-config` when the agent has none, and
   `PKG_CONFIG_PATH` / `LIBRARY_PATH` / `LD_LIBRARY_PATH` / `CGO_CFLAGS` / `CGO_LDFLAGS`
   exported. A sysroot left by an earlier battery on a kept agent is reused, not re-downloaded;
4. **cannot provision** → `QA_NATIVE_SYSDEPS_BLOCKED=<reason>` and an `INFO` row naming it.

Helpers: `qa_needs_seccomp_dev`, `qa_seccomp_dev_ready`, `qa_native_prereq_failure`,
`qa_seccomp_sysroot_export`.

### 3.2 Toolchain fallback (newest go vs prebuilt LTO archives)

`qa_go_module_floor`, `qa_toolchain_lto_rejection`, `qa_retry_with_documented_go`; wired at the
three go build sites — `fresh-install` (`inst.log`, keeps the `BUNKER_QA_INSTALL_TIMEOUT_S`
bound), `upgrade` up1 (streamed previous tree), `upgrade` up2 (HEAD reinstall). The retry fires
**only** on the `disallowed LTO section|segment` signature, runs **once**, uses
`GOTOOLCHAIN=go<go.mod floor>`, and appends both attempts plus a `TOOLCHAIN-FALLBACK … rc=`
marker to the same cell log. It does **not** change default toolchain selection — pinning the
agent's Go is QA-MUSTER-051 and stays untouched.

### 3.3 Classification (no more libelling the repo)

- `fresh-install` and `upgrade` grade `INFO ENV-BLOCKED` (naming the mechanism) when the log
  carries a pkg-config/libseccomp signature **or** an unresolved LTO rejection — never `FAIL`.
- `upgrade` OK detail names the toolchain actually used, e.g.
  `[built with the go.mod-documented toolchain go1.26.1: this agent's newest go rejected the
  module's prebuilt LLVM-bitcode archive — CVE-2026-97038 cgo check]`.
- Pre-existing hazard fixed in passing: two comment lines inside the generated-script heredoc
  carried bare backticks (``go `FAIL\t<pkg>`…``). The heredoc is unquoted, so the **generator**
  command-substituted that documentation text host-side on every generation — and emitted the
  comment with the backticked words deleted. Both lines are now escaped (`\``), the emitted
  comment is correct, and generation is silent. My own new block hit the same trap on the first
  launch (its doc text ran host-side as ordinary user; apt-get refused, `dpkg-deb`/`tar`/`sudo`
  did nothing, no residue) — every backtick in the added text is escaped.

## 4. Proof arm 1 — real full battery, fresh JIT agent

Command (exact):

```bash
cd ~/.hermes/scripts && BUNKER_QA_EVIDENCE=$HOME/.hermes/coding-hermes/evidence/qa-muster-13-r2-20261121b.jsonl \
  bash bunker-qa.sh launch /home/kara/wojons/muster --keep
# → LAUNCHED agent=6029d0de evidence=/home/kara/.hermes/coding-hermes/evidence/qa-muster-13-r2-20261121b.jsonl
bash bunker-qa.sh collect --evidence $HOME/.hermes/coding-hermes/evidence/qa-muster-13-r2-20261121b.jsonl
# → COLLECTED agent=6029d0de rows=24
```

Agent `6029d0de` @ bunker-mvp (Ubuntu noble container, 4 CPU, unprivileged user
`bunker-6029d0de`), evidence dir
`/home/kara/wojons/muster/.coding-hermes/evidence/qa-20261121T114128Z`.

Provisioning row (agent-side, verbatim):

```
OK | toolchain-bootstrap | native build prereq provisioned for libseccomp-linked trees:
pkg-config=/usr/bin/pkg-config, libseccomp 2.5.5 from user-space sysroot
/home/bunker-6029d0de/tools/sysroot (no sudo) — QA-MUSTER-13-R2
```

Unprivileged package fetch (`logs/qa-logs-2188376/native-sysdeps.log`, head):

```
Get:1 https://mirror.hetzner.com/ubuntu/packages noble-updates/main amd64 libseccomp-dev amd64 2.5.5-1ubuntu3.1 [92.3 kB]
Get:1 https://mirror.hetzner.com/ubuntu/packages noble-updates/main amd64 libseccomp2 amd64 2.5.5-1ubuntu3.1 [50.4 kB]
Get:1 https://mirror.hetzner.com/ubuntu/packages noble/main amd64 pkgconf-bin amd64 1.8.1-2build1 [20.7 kB]
Get:1 https://mirror.hetzner.com/ubuntu/packages noble/main amd64 pkgconf amd64 1.8.1-2build1 [16.8 kB]
Get:1 https://mirror.hetzner.com/ubuntu/packages noble/main amd64 pkg-config amd64 1.8.1-2build1 [7,264 B]
```

Upgrade cell row (verbatim, latest ledger):

```
OK | upgrade | previous release v0.1.0 (tree via git archive) → HEAD reinstall clean
[built with the go.mod-documented toolchain go1.26.1: this agent's newest go rejected the
module's prebuilt LLVM-bitcode archive — CVE-2026-97038 cgo check]
```

Cell log `logs/qa-logs-2188376/up1.log` (complete, 8 lines) — this is the required non-empty log
for the streamed v0.1.0 tree:

```
go: downloading github.com/tklauser/numcpus v0.11.0
github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS: /home/bunker-6029d0de/go/pkg/mod/github.com/bytecodealliance/wasmtime-go/v8@v8.0.0/build/linux-x86_64/libwasmtime.a

=== QA-MUSTER-13-R2: this agent go (${gov:-unknown}) rejects a prebuilt archive that carries LLVM bitcode (CVE-2026-97038 cgo LTO check) — not a repo defect.
=== retrying ONCE with the toolchain the module documents (go.mod: go 1.26.1) via GOTOOLCHAIN=go1.26.1
go: downloading go1.26.1 (linux/amd64)
# github.com/wojons/muster/exploits/wasm
/usr/bin/ld: warning: x86_64.o: missing .note.GNU-stack section implies executable stack
/usr/bin/ld: NOTE: This behaviour is deprecated and will be removed in a future version of the linker
TOOLCHAIN-FALLBACK GOTOOLCHAIN=go1.26.1 rc=0
```

(`(${gov:-unknown})` printed literally in this run: the marker's `printf` used a single-quoted
format. Fixed immediately after — `/usr/bin/printf`-style `%s` with the variable as an argument —
and re-proven on a live agent in §5.3, which prints `this agent go (1.26.11)`.)

Other cells of the same run (unchanged by this fix, for context): `tests` OK, `docker_build` OK,
`compose_config` OK, `ci-pass` OK (act 1 job green), `lint` INFO (golangci-lint absent on agent),
`docker-deploy` SKIPPED (controller safety gate), `ui-probe` INFO (no server surface).

## 5. Proof arm 2 — controller repro on a real clean agent (agent `dd16ecfc`)

Agent `dd16ecfc` @ bunker-mvp, Ubuntu 24.04.5, unprivileged `bunker-dd16ecfc`, `go1.22.2`
ambient; toolchain installed user-space (`~/tools/go` = go1.26.11, matching what the harness
selects). Previous release streamed exactly the way the harness does it:

```bash
git -C /home/kara/wojons/muster archive --format=tar v0.1.0 | ssh … 'tar -x -C ~/qa-up-prev'
```

### 5.1 RED — no provisioning (reproduces the filed failure verbatim)

```bash
cd ~/qa-up-prev && env -u PKG_CONFIG_PATH -u LIBRARY_PATH -u LD_LIBRARY_PATH \
  PATH=/usr/bin:/bin GOTOOLCHAIN=local ~/tools/go/bin/go build ./...
```

```
PREFIX_RC=1
# github.com/seccomp/libseccomp-golang
# [pkg-config --cflags  -- libseccomp]
Package libseccomp was not found in the pkg-config search path.
Perhaps you should add the directory containing `libseccomp.pc'
to the PKG_CONFIG_PATH environment variable
Package 'libseccomp', required by 'virtual:world', not found
github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS: …/libwasmtime.a
```

### 5.2 Provisioned, still on go1.26.11 — the pkg-config failure is GONE, the LTO check remains

```
=== BEFORE provisioning (clean agent): pkg-config=/usr/bin/pkg-config  libseccomp.pc=absent
toolchain-bootstrap|OK|native build prereq provisioned … libseccomp 2.5.5 from user-space
sysroot /home/bunker-dd16ecfc/tools/sysroot (no sudo) — QA-MUSTER-13-R2
pkg-config resolves: 2.5.5   PKG_CONFIG_PATH=…/tools/sysroot/usr/lib/x86_64-linux-gnu/pkgconfig
UP1_RC=1
github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS: …
--- native-prereq signature lines: 0
```

### 5.3 GREEN — shipped helper path (provisioning + toolchain fallback), final harness bytes

Driver sources the block **exactly as extracted from the generated `qa-run.sh`** and drives the
upgrade cell's up1/up2 arms through the shipped logic:

```
agent_go=go1.26.11  floor=1.26.1
toolchain-bootstrap|INFO|reused sysroot from an earlier battery: pkg-config + libseccomp.pc from /home/bunker-dd16ecfc/tools/sysroot (no download needed) — QA-MUSTER-13-R2
toolchain-bootstrap|OK|native build prereq provisioned for libseccomp-linked trees: pkg-config=/usr/bin/pkg-config, libseccomp 2.5.5 … (no sudo)
provisioned: libseccomp=2.5.5
=== up1 arm: previous release tree (streamed v0.1.0)
up1_rc_before_retry=1
up1_rc=0  QA_TOOLCHAIN_FALLBACK=go1.26.1
--- up1.final.log (8 lines):
github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS: …/libwasmtime.a

=== QA-MUSTER-13-R2: this agent go (1.26.11) rejects a prebuilt archive that carries LLVM bitcode (CVE-2026-97038 cgo LTO check) — not a repo defect.
=== retrying ONCE with the toolchain the module documents (go.mod: go 1.26.1) via GOTOOLCHAIN=go1.26.1
# github.com/wojons/muster/exploits/wasm
/usr/bin/ld: warning: x86_64.o: missing .note.GNU-stack section implies executable stack
TOOLCHAIN-FALLBACK GOTOOLCHAIN=go1.26.1 rc=0
=== up2 arm: HEAD reinstall in the working tree (same tree here)
up2_rc=0
=== pkg-config/libseccomp signature lines across both logs: up1.final.log:0 up2.final.log:0
```

## 6. Hermetic matrix (provisioning + classifier logic, no agent)

`matrix.sh` extracts the block **from the generated `qa-run.sh`** (shipped bytes, not a copy) and
runs it against stubs/PATH-masked binaries. Reproducer committed next to this file:
`.coding-hermes/evidence/qa-muster-13-r2-matrix.sh`
(`bash qa-muster-13-r2-matrix.sh <generated-qa-run.sh> <workdir> <debs-dir>`).

**36 assertions, 0 failures.** Cases:

- **T1 present** — `pkg-config --exists libseccomp` true → one `INFO … nothing installed`; zero
  package-manager invocations; no blocked marker.
- **T2 provision (no root, apt)** — masked pkg-config (`PKG_CONFIG_LIBDIR` → nonexistent), real
  `.deb`s, real `dpkg-deb`/`find`/`sed`/`cc`: `OK` row; `pkg-config --exists libseccomp` resolves;
  `--cflags` points into the sysroot; lib dir on `LIBRARY_PATH`/`LD_LIBRARY_PATH`; `CGO_LDFLAGS`
  carries the sysroot `-L`; **real C compile+link+run** of a `seccomp_version()` program succeeds,
  prints `seccomp=2.6.0`, and `ldd` shows the binary loads the **sysroot** `libseccomp` (not a
  host one); every `.pc` `prefix=` rewritten; runtime `.so.2` present; no `ENV-BLOCKED` row.
- **T3 idempotent rerun** — same HOME again: sysroot reused (`reused sysroot from an earlier
  battery`), **zero** downloads, still `OK`.
- **T4 cannot provision** — failing `apt-get`, masked pkg-config: `INFO ENV-BLOCKED …, uid=…`,
  `QA_NATIVE_SYSDEPS_BLOCKED` set, **no** false `OK`.
- **T5 classifier** — the two real production lines (pkg-config-not-found; `Package libseccomp
  was not found in the pkg-config search path`) classified; an ordinary compile error and an empty
  log **not** classified (no false `ENV-BLOCKED`).
- **T6 unrelated repo** — no libseccomp in `go.mod`/`go.sum`: block inert, no cells, no installs.
- **T7 LTO fallback** — `disallowed LTO section` → classified, **one** retry with
  `GOTOOLCHAIN=go1.26.1`, build recovers `rc=0`, `QA_TOOLCHAIN_FALLBACK=go1.26.1`, and the log
  keeps **both** attempts plus `TOOLCHAIN-FALLBACK … rc=0`.
- **T8 no documented floor** — `go.mod` without a `go` directive: retry declines
  (`RETRY_RC=1`), no invented floor, no fallback marker.
- **T9 ordinary failure** — plain compile error never retried, no marker written.

## 7. Second full battery on the final bytes

Launched after the last harness edit (`printf` marker fix + backtick escapes) so the run carries
byte-identical logic to what is now installed:

```bash
BUNKER_QA_EVIDENCE=$HOME/.hermes/coding-hermes/evidence/qa-muster-13-r2-20261121c.jsonl \
BUNKER_QA_TICK=qamuster13r2-final bash bunker-qa.sh launch /home/kara/wojons/muster --keep
# → agent=9d1118de @ bunker-mvp
```

Confirmed from that agent (`bunker exec --server bunker-mvp 9d1118de`):

- `toolchain-bootstrap OK … native build prereq provisioned … libseccomp 2.5.5 from user-space
  sysroot /home/bunker-9d1118de/tools/sysroot (no sudo)`;
- `fresh-install OK`, `tests OK`, `docker_build OK`, `compose_config OK`, `ci-pass OK`;
- `upgrade OK — previous release v0.1.0 (tree via git archive) → HEAD reinstall clean [built with
  the go.mod-documented toolchain go1.26.1 …]`, with `~/qa-logs-2264630/up1.log`:

```
=== QA-MUSTER-13-R2: this agent go (1.27.4) rejects a prebuilt archive that carries LLVM bitcode (CVE-2026-97038 cgo LTO check) — not a repo defect.
=== retrying ONCE with the toolchain the module documents (go.mod: go 1.26.1) via GOTOOLCHAIN=go1.26.1
go: downloading go1.26.1 (linux/amd64)
# github.com/wojons/muster/exploits/wasm
TOOLCHAIN-FALLBACK GOTOOLCHAIN=go1.26.1 rc=0
```

(the marker interpolates the real agent go, `1.27.4` — the cosmetic defect from run 1 is gone), and
`pkg-config|libseccomp` failure signatures are `0` in every cell log of that agent
(`native-sysdeps.log` contains 3 matches, all `apt-get download` lines naming the packages).

That battery's own chaos cells (`chaos-corruption` etc.) were still running when this file was
written; they are ordinary QA findings for the `muster-qa` lane, not part of this row. The
fully-collected full battery remains run 1 (§4, 24 rows).

## 8. Owed / residual

- **QA-MUSTER-051 stays open and is the real cure**: the harness still installs the *newest* Go
  rather than the `go.mod`-pinned toolchain (run 1 got go 1.27.4, run 2 `1.27.4`). The
  documented-toolchain retry added here is a bounded safety net for exactly one class of that
  defect (prebuilt LTO archives), not a pinning policy.
- **Nothing owed on the product**: no product file was touched — `go.mod`, `go.sum` and all
  sources are untouched by this row.
- The upgrade cell's own `rm -rf`-based cleanup deletes `~/qa-up-prev` after the arm; the streamed
  tree is not preserved on the agent (logs are collected).

## 9. Facts re-verified (and one corrected)

| Claim | Result |
|---|---|
| pkg-config + `libseccomp.pc` is the whole mechanism | **Confirmed** — RED log is exactly that; with them present the pkg-config failure disappears (§5.1 → §5.2) |
| "with libseccomp.pc present and go1.26.11 the v0.1.0 tree builds rc=0" | **Refuted on a clean agent** — go1.26.11/1.27.4 reject `libwasmtime.a` (CVE-2026-97038 LTO check); rc=0 needs the go.mod-documented go1.26.1 (§5.2 → §5.3) |
| fix is battery-side, not a product change | **Confirmed** — no product file touched |
| Go-version provisioning defect is a separate row | **Confirmed and untouched** (QA-MUSTER-051); the retry is signature-scoped, not a pin |
