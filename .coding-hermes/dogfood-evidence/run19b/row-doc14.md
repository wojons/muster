Dogfood run 19b (tick muster-dogfood-2026-10-20-11-29-14, resumed turn; HEAD 2eb9f8f3). Found by reading the platform doc as a user who wants to relocate config/cache/data on a server.

specs/PLATFORM-LINUX.md:2891-2897 ("Environment Variables" table) documents five variables. FOUR of them are read by NO source file in the tree — the only mentions anywhere are the spec line itself (and, for MUSTER_CONFIG_DIR, one test that believes it works):

  var                  hits (whole tree, excl. .git)   where
  MUSTER_CONFIG_DIR    2                              specs/PLATFORM-LINUX.md:2893 + test/production/production_simulation_test.go:191
  MUSTER_CACHE_DIR     1                              specs/PLATFORM-LINUX.md:2894 (spec only)
  MUSTER_DATA_DIR      1                              specs/PLATFORM-LINUX.md:2895 (spec only)
  MUSTER_NO_COLOR      1                              specs/PLATFORM-LINUX.md:2897 (spec only)
  MUSTER_LOG_LEVEL     26                             implemented (pkg/config/loader.go; systemd/muster-daemon.service:67) <- the control that proves the grep method

Command: `for v in MUSTER_CONFIG_DIR MUSTER_CACHE_DIR MUSTER_DATA_DIR MUSTER_NO_COLOR MUSTER_LOG_LEVEL; do grep -rn "$v" . | grep -v '^\./\.git/'; done`

(a) MUSTER_NO_COLOR is contradicted by the shipped implementation: color is controlled by the STANDARD variable at pkg/cli/error_formatter.go:206,225 (`os.Getenv("NO_COLOR")`). A user who sets MUSTER_NO_COLOR per the spec gets no effect (and no error) while `NO_COLOR=1` — undocumented in this table — is what actually works.
(b) MUSTER_CONFIG_DIR is used by test/production/production_simulation_test.go:191 as if it isolated the run, but that test's isolation really comes from the `HOME=tmpDir` in the same env block (line 190); the variable is a no-op the test could not detect.
(c) Related, tested live (not the same mechanism, no row of its own): `auth add` credential metadata is NOT redirected by MUSTER_CONFIG/MUSTER_CONFIG_FILE — with MUSTER_CONFIG=/tmp/mf19/config.yaml set, `openapi-cli auth add --name X` still wrote the entry into ~/.config/muster/credentials.json. The working isolation knob is XDG_CONFIG_HOME (pkg/auth/credential.go:19-33 CredentialConfigDir). So the spec's "configuration directory" promise has no counterpart in the credential path either.

Sibling, not duplicate: DOC-11 (closed, c2180a15) found the same class of rot in docs/guides/configuration.md (5 var names the code never reads) and fixed THAT file. This row is a different artifact (the platform spec, cited as design authority by AGENTS.md) with a different variable set, still live; the fix is a spec-surface sweep, not a re-fix of configuration.md.

Fix direction (either is acceptable): implement the three dir/color overrides following the XDG-first pattern already used in pkg/auth/credential.go + pkg/config, or mark each unimplemented variable with an explicit status column ("not implemented") and drop MUSTER_NO_COLOR in favour of the standard NO_COLOR. Then add a doc test so a table entry cannot rot again: for every `MUSTER_*` name in specs/PLATFORM-LINUX.md's env table, assert at least one non-test Go file references it (MUSTER_LOG_LEVEL passes, the four above fail today).

AC: the spec table and the code agree — either each documented variable is read by a non-test source file (proved by a test that greps the table against the tree), or it is labelled unimplemented; `MUSTER_NO_COLOR` is either honoured or replaced by/documented as `NO_COLOR`.
