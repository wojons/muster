#!/usr/bin/env bash
# QA-MUSTER-13-R2 hermetic matrix T1..T6.
# Runs the provisioning block EXACTLY as shipped inside the generated qa-run.sh
# (extracted between the QA-MUSTER-13-R2 marker and the jq bootstrap comment),
# against stubs, so the assertions are on shipped bytes rather than a copy.
set -uo pipefail
SRC="${1:?usage: matrix.sh <qa-run.generated.sh>}"
BASE="${2:?usage: matrix.sh <gen> <workdir>}"
DEBS="$(cd "${3:?usage: matrix.sh <gen> <workdir> <debs-dir>}" && pwd)"
BLOCK="$BASE/block.sh"
awk '/^# ─── QA-MUSTER-13-R2 \(2026-11-21\)/{f=1} /^# QA-CODING-HERMES-TOOLS-6 \(2026-10-24\)/{f=0} f' "$SRC" > "$BLOCK"
printf 'extracted block: %s lines\n' "$(wc -l < "$BLOCK")"
grep -q 'qa_needs_seccomp_dev()' "$BLOCK" || { echo "FATAL: block extraction failed"; exit 9; }

PASS=0; FAIL=0
ok()   { PASS=$((PASS+1)); printf 'PASS %s\n' "$1"; }
bad()  { FAIL=$((FAIL+1)); printf 'FAIL %s :: %s\n' "$1" "$2"; }
chk()  { if eval "$2"; then ok "$1"; else bad "$1" "assertion failed: $2"; fi; }

newhome() { mktemp -d "$BASE/home-$1.XXXXXX"; }

mkstub() { # <dir> <name> <body>
  printf '#!/bin/sh\n%s\n' "$3" > "$1/$2"; chmod +x "$1/$2"
}

# ── T1: prerequisite already present -> one INFO row, ZERO install attempts ──
t1() {
  local h s; h="$(newhome t1)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" pkg-config 'case "$1" in --exists) exit 0;; --modversion) echo 2.6.0; exit 0;; esac; exit 0'
  mkstub "$s" apt-get 'echo "APT-GET-CALLED" >> "$STUB_LOG"; exit 0'
  mkstub "$s" apk 'echo "APK-CALLED" >> "$STUB_LOG"; exit 1'
  printf 'module x\nrequire github.com/seccomp/libseccomp-golang v0.11.1\n' > "$h/go.mod"
  : > "$h/rows"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" EVID="$h/rows" STUB_LOG="$h/stub.log" LOGD="$h/logs"
    cell() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$EVID"; }
    . "$BLOCK" >/dev/null 2>&1
  )
  local rows; rows="$(cat "$h/rows")"
  chk "T1 already-present -> INFO no-install row" 'printf "%s" "$rows" | grep -q "toolchain-bootstrap|INFO|native build prereq already present"'
  chk "T1 no package-manager invocation" '[ ! -s "$h/stub.log" ]'
  chk "T1 no blocked marker" '! printf "%s" "$rows" | grep -q "ENV-BLOCKED"'
}

# ── T2: absent + no root + apt -> user-space sysroot provisions, resolves ──
t2() {
  local h s; h="$(newhome t2)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" apt-get 'for last; do :; done; cp '"$DEBS"'/*.deb . 2>/dev/null; echo "apt-get download $last" >> "$STUB_LOG"'
  printf 'module x\nrequire github.com/seccomp/libseccomp-golang v0.11.1\n' > "$h/go.mod"
  : > "$h/rows"
  local out
  out="$(
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" EVID="$h/rows" STUB_LOG="$h/stub.log" LOGD="$h/logs"
    export PKG_CONFIG_LIBDIR="$h/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$EVID"; }
    . "$BLOCK" >/dev/null 2>&1
    echo "READY=$(command -v pkg-config >/dev/null 2>&1 && pkg-config --exists libseccomp && echo yes || echo no)"
    echo "CFLAGS=$(pkg-config --cflags libseccomp 2>/dev/null)"
    echo "LIBS=$(pkg-config --libs libseccomp 2>/dev/null)"
    echo "LIBPATH=${LIBRARY_PATH:-}"
    echo "LD=${LD_LIBRARY_PATH:-}"
    echo "CGOLDF=${CGO_LDFLAGS:-}"
    cat > "$h/scprobe.c" <<'CEOF'
#include <seccomp.h>
#include <stdio.h>
int main(void) { const struct scmp_version *v = seccomp_version();
  printf("seccomp=%u.%u.%u\n", v->major, v->minor, v->micro); return 0; }
CEOF
    if cc $(pkg-config --cflags libseccomp 2>/dev/null) "$h/scprobe.c" -o "$h/scprobe" $(pkg-config --libs libseccomp 2>/dev/null) 2>"$h/cc.err"; then
      echo "COMPILE=ok"
      echo "RUN=$("$h/scprobe" 2>&1)"
      echo "SYSROOTLIB=$(ldd "$h/scprobe" 2>/dev/null | grep -c "$h/tools/sysroot")"
    else
      echo "COMPILE=failed"
      sed -n '1,4p' "$h/cc.err" >&2
    fi
    echo "MODVER=$(pkg-config --modversion libseccomp 2>/dev/null)"
    echo "PCPATH=${PKG_CONFIG_PATH:-}"
  )"
  local rows; rows="$(cat "$h/rows")"
  chk "T2 provisioned -> OK row" 'printf "%s" "$rows" | grep -q "toolchain-bootstrap|OK|native build prereq provisioned"'
  chk "T2 pkg-config resolves libseccomp from sysroot" 'printf "%s" "$out" | grep -qx "READY=yes"'
  chk "T2 cflags point INTO the sysroot" 'printf "%s" "$out" | grep -q "^CFLAGS=-I$h/tools/sysroot/usr/include"'
  printf "DEBUG-OUT[%s]\n" "$out"
  chk "T2 libs resolve (pkgconf may drop -L once LIBRARY_PATH carries it)" 'printf "%s" "$out" | grep -q "^LIBS=.*-lseccomp"'
  chk "T2 sysroot lib dir exported on LIBRARY_PATH (linker reaches -lseccomp)" 'printf "%s" "$out" | grep -q "^LIBPATH=$h/tools/sysroot/usr/lib/"'
  chk "T2 sysroot lib dir exported on LD_LIBRARY_PATH (binary runs)" 'printf "%s" "$out" | grep -q "^LD=$h/tools/sysroot/usr/lib/"'
  chk "T2 CGO_LDFLAGS carries the sysroot -L" 'printf "%s" "$out" | grep -q "^CGOLDF=.*-L$h/tools/sysroot/usr/lib/"'
  chk "T2 REAL C compile+link+run against the sysroot libseccomp" 'printf "%s" "$out" | grep -qx "COMPILE=ok" && printf "%s" "$out" | grep -q "^RUN=seccomp=[0-9]\+\.[0-9]\+\.[0-9]\+$"'
  chk "T2 built binary loads the SYSROOT libseccomp (not a host one)" 'printf "%s" "$out" | grep -qx "SYSROOTLIB=1"'
  chk "T2 .pc prefix relocated onto sysroot" 'grep -rq "^prefix=$h/tools/sysroot/usr$" "$h/tools/sysroot"'
  chk "T2 runtime lib present in sysroot" 'find "$h/tools/sysroot" -name "libseccomp.so.2*" | grep -q .'
  chk "T2 no ENV-BLOCKED row" '! printf "%s" "$rows" | grep -q "ENV-BLOCKED"'
  # T3: rerun in the SAME home reuses the sysroot instead of re-downloading
  : > "$h/stub.log"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" EVID="$h/rows2" STUB_LOG="$h/stub.log" LOGD="$h/logs"
    export PKG_CONFIG_LIBDIR="$h/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$EVID"; }
    . "$BLOCK" >/dev/null 2>&1
  )
  local rows2; rows2="$(cat "$h/rows2")"
  chk "T3 idempotent rerun reuses sysroot (no download)" '[ ! -s "$h/stub.log" ] && printf "%s" "$rows2" | grep -q "reused sysroot from an earlier battery"'
  chk "T3 rerun still grades OK" 'printf "%s" "$rows2" | grep -q "toolchain-bootstrap|OK|native build prereq provisioned"'

  # T4: provisioning genuinely impossible -> ENV-BLOCKED, never a silent pass
  local h4 s4; h4="$(newhome t4)"; s4="$h4/stubbin"; mkdir -p "$s4" "$h4/logs"
  mkstub "$s4" apt-get 'exit 100'
  printf 'module x\nrequire github.com/seccomp/libseccomp-golang v0.11.1\n' > "$h4/go.mod"
  : > "$h4/rows"
  (
    cd "$h4" || exit 1
    export HOME="$h4" PATH="$s4:$h4/tools/bin:/usr/bin:/bin" EVID="$h4/rows" LOGD="$h4/logs"
    export PKG_CONFIG_LIBDIR="$h4/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$EVID"; }
    . "$BLOCK" >/dev/null 2>&1
    echo "BLOCKED_SET=${QA_NATIVE_SYSDEPS_BLOCKED:+yes}" > "$h4/marker"
  )
  local rows4; rows4="$(cat "$h4/rows")"
  chk "T4 cannot-provision -> ENV-BLOCKED row (not silent pass)" 'printf "%s" "$rows4" | grep -q "toolchain-bootstrap|INFO|ENV-BLOCKED: pkg-config/libseccomp-dev unavailable"'
  chk "T4 blocked marker exported for the build cells" 'printf "%s" "$rows4" | grep -q "uid=" && grep -q "BLOCKED_SET=yes" "$h4/marker"'
  chk "T4 no false OK row" '! printf "%s" "$rows4" | grep -q "|OK|"'
}

# ── T5: the native-prereq classifier on the REAL production log lines ──
t5() {
  local h; h="$(newhome t5)"; mkdir -p "$h/logs"
  cat > "$h/up1.log" <<'LOG'
# github.com/seccomp/libseccomp-golang
github.com/seccomp/libseccomp-golang: exec: "pkg-config": executable file not found in $PATH
LOG
  cat > "$h/up1b.log" <<'LOG'
# github.com/seccomp/libseccomp-golang
Package libseccomp was not found in the pkg-config search path.
Perhaps you should add the directory containing `libseccomp.pc'
LOG
  printf 'package x\n./cmd/y.go:3:2: undefined: z\n' > "$h/plain.log"
  : > "$h/empty.log"
  (
    cd "$h" || exit 1
    export HOME="$h" LOGD="$h/logs"
    cell() { :; }
    . "$BLOCK" >/dev/null 2>&1
    qa_native_prereq_failure "$h/up1.log"  && echo "A=hit"  || echo "A=miss"
    qa_native_prereq_failure "$h/up1b.log" && echo "B=hit"  || echo "B=miss"
    qa_native_prereq_failure "$h/plain.log" && echo "C=hit" || echo "C=miss"
    qa_native_prereq_failure "$h/empty.log" && echo "D=hit" || echo "D=miss"
  ) > "$h/res.txt" 2>&1 || true
  local r; r="$(cat "$h/res.txt")"
  chk "T5 pkg-config-not-found line classified native-prereq" 'printf "%s" "$r" | grep -qx A=hit'
  chk "T5 libseccomp.pc-search-path line classified native-prereq" 'printf "%s" "$r" | grep -qx B=hit'
  chk "T5 ordinary compile error NOT classified (no false ENV-BLOCKED)" 'printf "%s" "$r" | grep -qx C=miss'
  chk "T5 empty log NOT classified" 'printf "%s" "$r" | grep -qx D=miss'
}

# ── T6: repo that does not link libseccomp pays nothing ──
t6() {
  local h s; h="$(newhome t6)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" apt-get 'echo "APT-GET-CALLED" >> "$STUB_LOG"; exit 0'
  printf 'module x\nrequire github.com/spf13/cobra v1.8.0\n' > "$h/go.mod"
  printf 'github.com/spf13/cobra v1.8.0 h1:abc=\n' > "$h/go.sum"
  : > "$h/rows"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" EVID="$h/rows" STUB_LOG="$h/stub.log" LOGD="$h/logs"
    export PKG_CONFIG_LIBDIR="$h/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { printf '%s|%s|%s\n' "$1" "$2" "$3" >> "$EVID"; }
    . "$BLOCK" >/dev/null 2>&1
    qa_needs_seccomp_dev && echo "NEEDS=yes" || echo "NEEDS=no"
  ) > "$h/res.txt" 2>&1 || true
  chk "T6 no libseccomp dependency -> block inert" 'grep -qx NEEDS=no "$h/res.txt"'
  chk "T6 no cell rows emitted" '[ ! -s "$h/rows" ]'
  chk "T6 no package-manager invocation" '[ ! -s "$h/stub.log" ]'
}

t1; t2; t5; t6

# ── T7: newest-go LTO rejection -> ONE retry with the go.mod-documented toolchain ──
t7() {
  local h s; h="$(newhome t7)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" go 'case "$1" in version) echo "go version go1.26.11 linux/amd64";; esac; exit 0'
  cat > "$s/lto-cmd" <<'EOF'
#!/bin/sh
if [ "${GOTOOLCHAIN:-}" = "go1.26.1" ]; then echo "BUILD-OK-GOTOOLCHAIN=$GOTOOLCHAIN"; exit 0; fi
echo 'github.com/bytecodealliance/wasmtime-go/v8: disallowed LTO section ".llvmbc" in #cgo LDFLAGS: /root/go/pkg/mod/x/libwasmtime.a' >&2
exit 1
EOF
  chmod +x "$s/lto-cmd"
  printf 'module x\n\ngo 1.26.1\n\nrequire github.com/seccomp/libseccomp-golang v0.11.1\n' > "$h/go.mod"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" LOGD="$h/logs"
    export PKG_CONFIG_LIBDIR="$h/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { :; }
    . "$BLOCK" >/dev/null 2>&1
    lto-cmd > "$h/logs/inst.log" 2>&1; rc=$?
    echo "FIRST_RC=$rc"
    qa_toolchain_lto_rejection "$h/logs/inst.log" && echo "CLASSIFIED=hit" || echo "CLASSIFIED=miss"
    if [ $rc -ne 0 ] && qa_toolchain_lto_rejection "$h/logs/inst.log"; then
      qa_retry_with_documented_go "$h/logs/inst.log" . 'lto-cmd' && rc=0 || rc=$?
    fi
    echo "FINAL_RC=$rc"
    echo "FALLBACK=${QA_TOOLCHAIN_FALLBACK:-none}"
  ) > "$h/res.txt" 2>&1 || true
  local log="$h/logs/inst.log" r; r="$(cat "$h/res.txt")"
  chk "T7 LTO rejection classified" 'printf "%s" "$r" | grep -qx CLASSIFIED=hit'
  chk "T7 retry recovers the build (FINAL_RC=0)" 'printf "%s" "$r" | grep -qx FINAL_RC=0'
  chk "T7 retry used the go.mod toolchain" 'grep -q "BUILD-OK-GOTOOLCHAIN=go1.26.1" "$log"'
  chk "T7 fallback recorded for the cell detail" 'printf "%s" "$r" | grep -qx "FALLBACK=go1.26.1"'
  chk "T7 both attempts kept in the cell log" 'grep -q "disallowed LTO section" "$log" && grep -q "QA-MUSTER-13-R2" "$log" && grep -q "TOOLCHAIN-FALLBACK GOTOOLCHAIN=go1.26.1 rc=0" "$log"'
}

# ── T8: no documented floor -> no retry, failure stays a failure ──
t8() {
  local h s; h="$(newhome t8)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" go 'echo "go version go1.26.11 linux/amd64"; exit 0'
  mkstub "$s" lto-cmd 'echo "disallowed LTO section \".llvmbc\"" >&2; exit 1'
  printf 'module x\n\nrequire github.com/seccomp/libseccomp-golang v0.11.1\n' > "$h/go.mod"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" LOGD="$h/logs"
    export PKG_CONFIG_LIBDIR="$h/no-such-libdir" PKG_CONFIG_PATH=""
    cell() { :; }
    . "$BLOCK" >/dev/null 2>&1
    lto-cmd > "$h/logs/inst.log" 2>&1; rc=$?
    qa_retry_with_documented_go "$h/logs/inst.log" . 'lto-cmd'; rrc=$?
    echo "FIRST_RC=$rc RETRY_RC=$rrc FALLBACK=${QA_TOOLCHAIN_FALLBACK:-none}"
  ) > "$h/res.txt" 2>&1 || true
  local log="$h/logs/inst.log"
  chk "T8 no go directive -> retry declines (no invented floor)" 'grep -qx "FIRST_RC=1 RETRY_RC=1 FALLBACK=none" "$h/res.txt"'
  chk "T8 log carries no bogus fallback marker" '! grep -q "TOOLCHAIN-FALLBACK" "$log"'
}

# ── T9: an ordinary build failure is never retried ──
t9() {
  local h s; h="$(newhome t9)"; s="$h/stubbin"; mkdir -p "$s" "$h/logs"
  mkstub "$s" go 'echo "go version go1.26.11 linux/amd64"; exit 0'
  mkstub "$s" plain-cmd 'echo "./cmd/x.go:3:2: undefined: y" >&2; exit 1'
  printf 'module x\n\ngo 1.26.1\n' > "$h/go.mod"
  (
    cd "$h" || exit 1
    export HOME="$h" PATH="$s:$h/tools/bin:/usr/bin:/bin" LOGD="$h/logs"
    cell() { :; }
    . "$BLOCK" >/dev/null 2>&1
    plain-cmd > "$h/logs/inst.log" 2>&1; rc=$?
    qa_toolchain_lto_rejection "$h/logs/inst.log" && echo "CLASSIFIED=hit" || echo "CLASSIFIED=miss"
    echo "RC=$rc"
  ) > "$h/res.txt" 2>&1 || true
  chk "T9 ordinary compile error not classified" 'grep -qx CLASSIFIED=miss "$h/res.txt"'
  chk "T9 no retry marker written" '! grep -q "TOOLCHAIN-FALLBACK" "$h/logs/inst.log"'
}

t7; t8; t9
printf '\nMATRIX RESULT: %d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
