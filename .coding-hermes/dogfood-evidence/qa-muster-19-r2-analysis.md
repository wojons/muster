# QA-MUSTER-19-R2 — root cause capture (not inference)

Captured 2026-10-23 in the wt/QA-MUSTER-19-R2 worktree, pre-fix tree,
`GOTOOLCHAIN=go1.27.3`, via a temporary debug test that replayed
`TestServer_Run_HandlerReturnsError`'s exact scenario (os.Pipe stdin,
io.Pipe stdout, one `test/generic-error` request, stdin closed, response
decoded) and wrote `runtime.Stack(all=true)` at the 3s deadline.

Dump: `qa-muster-19-r2-goroutine-dump.txt` (same directory).

## What the dump shows — file:line attribution

The board row's hypothesis (request goroutine parked on the
`s.concurrencySem <- struct{}{}` send at pkg/mcp/server.go:157) is
REFUTED. The dump shows:

1. Run's caller — `goroutine 51 [sync.WaitGroup.Wait]`:
   `sync.(*WaitGroup).Wait` ← `(*Server).Run` at **pkg/mcp/server.go:164**
   (the post-EOF `s.wg.Wait()`).

2. The request goroutine — `goroutine 39 [select]`:
   `io.(*pipe).write` (io/pipe.go:86) ← `io.(*PipeWriter).Write` ←
   `fmt.Fprintln` ← `(*Server).writeResponse` at **pkg/mcp/server.go:307** ←
   `sendError` (server.go:294) ← `handleRequest` (server.go:267) ←
   `Run.func1` (server.go:159).

   The goroutine already acquired the semaphore; it is parked WRITING THE
   RESPONSE to stdout (an io.Pipe), 0x41 = 65 bytes in flight.

## Why go1.27.3 wedges and go1.26.10 passes

Instrumenting the test's reader (loggingReader around the io.Pipe read end)
under go1.27.3 shows the client's `json.Decoder` issuing its first Read with
a 64-byte buffer — `READ req=64 got=64` — consuming exactly the marshaled
response value and leaving the trailing '\n' (byte 65 of the Fprintln write)
undrained. `Decode` completes (the newline is not part of the value), the
test moves on, and nobody ever reads the last byte. io.Pipe has no buffer:
the writer stays parked until all 65 bytes are consumed, so the handler
goroutine never reaches `wg.Done()` and `s.wg.Wait()` at server.go:164 never
returns.

go1.26.10's `json.Decoder` reads with a 512-byte buffer, draining all 65
bytes in one Read, which completes the write and hides the defect.

## Root cause (lifecycle)

Run's shutdown wait coupled server return to the CLIENT CONSUMING every byte
of every response. Client backpressure / partial drains (or, in production, a
crashed MCP client holding its end of the stdout pipe open) wedged shutdown.

## Fix

pkg/mcp/server.go: responses are marshaled and queued on `writeCh` to a
dedicated `responseWriter` goroutine that owns every stdout write. `s.wg`
covers handler completion only; `done` (closed on Run return / Shutdown)
tells the writer to drain queued responses and exit. A parked client write
now parks the writer goroutine alone — Run returns after handlers finish,
regardless of client drain behavior. No timeouts were added or enlarged;
`runReturnTimeout` is unchanged.

Regression coverage: `TestServer_Run_PartialResponseDrainDoesNotWedgeShutdown`
(pkg/mcp/server_test.go) reproduces the wedge deterministically on ANY
toolchain by reading exactly the response value bytes and never the trailing
newline, then asserting Run returns. Red-proven against the pre-fix server
(FAIL, 3.01s, go1.26.10) and green post-fix on go1.26.10 and go1.27.3.
`requireRunReturned` now also dumps all goroutine stacks at the deadline
before failing, so any future wedge is self-diagnosing.
