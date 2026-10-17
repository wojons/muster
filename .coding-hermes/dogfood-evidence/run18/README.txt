run 18 evidence bundle (tick muster-dogfood-2026-10-17-04-07-06, HEAD 3632f0dc->d32424a9)

integration-suite-v.log .... full `go test -tags=integration -run Integration ./test/e2e/ -v`
                             at HEAD 3632f0dc — 8 TestKeychainIntegration subtest FAILs,
                             every error 'credential not found: search items: dbus.Store: length mismatch'.
dbusprobe/main.go .......... standalone godbus v5.2.2 repro against the LIVE secret service:
                             OpenSession ok, CreateItem ok (item /login/195, cleaned up),
                             Collection.SearchItems .Store(&results, &locked) FAILS 'length mismatch'
                             — reply is a single 'ao' out param (busctl introspect).
fixtures/ .................. echo WSDLs + echo servers used for the --endpoint re-proof (R2)
                             and the SOAP-1.2 ingest check.
board-validate.txt ......... boardctl validate RESULT OK after the run-18 appends.

repro commands:
  cd /home/kara/wojons/muster && go test -tags=integration -run Integration ./test/e2e/ -v
  go run ./<copy of dbusprobe-main.go>          # needs godbus v5.2.2 in go.mod
