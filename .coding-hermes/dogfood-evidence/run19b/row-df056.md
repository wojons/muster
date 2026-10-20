Dogfood run 19b (tick muster-dogfood-2026-10-20-11-29-14, resumed turn; HEAD 2eb9f8f3, parent commit 69c36580 = run 19's first pass). Found by running the documented gate `./demo/run-all.sh` verbatim (README:101) and reading the output a user actually sees.

The Results block at the end of the gate prints RAW ANSI escape sequences. demo/run-all.sh:159 and :160 interpolate the color variables into a plain `echo` (no -e), so the escapes are emitted literally; :161 in the SAME block uses `echo -e`, so the bug is line-local, not environmental.

  demo/run-all.sh:159  echo "  ${GREEN}Passed: $PASS${NC}"
  demo/run-all.sh:160  [ $FAIL -gt 0 ] && echo "  ${RED}Failed: $FAIL${NC}"
  demo/run-all.sh:161  [ $FAIL -eq 0 ] && echo -e "  ${GREEN}All $PASS checks passed! ...${NC}"   <- this one is correct

Evidence (durable, committed): .coding-hermes/dogfood-evidence/run19/demo-timed.txt — `grep -c '033\[0;32m'` = 1 and the byte-exact summary line renders as:
  "  \033[0;32mPassed: 33\033[0m"
on the 33/33 green run (the same run whose "All 33 checks passed" line IS colored, which is what makes the leak obvious side by side). Reproduced in a second capture (demo-run-all.txt) of the same command.

Impact: cosmetic but it lands on the one line every user reads to decide whether the demo passed, and the failing variant at :160 is the one a user sees when things break — it prints red escapes to a terminal as text (harmless when piping to a file/CI, where the escapes become visible garbage instead of color).

Fix direction: add -e to both echoes (or use printf '%b'). Check for the same pattern elsewhere in demo/*.sh with `grep -n '\${GREEN}\|\${RED}\|\${CYAN}' demo/*.sh | grep -v 'echo -e'` — anything in the plain-echo set has the same leak.

AC: `./demo/run-all.sh 2>&1 | tail -5 | grep -c '\\033\['` = 0 on both a passing run and a run with FAIL>0, and the summary line still shows the same pass/fail count as before.
