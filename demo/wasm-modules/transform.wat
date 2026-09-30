;; transform.wat — WASI stdin/stdout response-transformation filter.
;;
;; WHY A COMMAND MODULE
;; The Muster WASM runtime executes every plugin as a WASI *command*
;; (pkg/wasm/module.go, runWASMModule): the response body is piped to the
;; module's stdin and whatever the module writes to stdout replaces it. A
;; module that only exports a helper function and defines no `_start` is
;; instantiated successfully, executes nothing and writes nothing — so the
;; interceptor silently replaced real response bodies with EMPTY output
;; (REVIEW-MUSTER-6). This module is a real command filter instead: it reads
;; stdin to EOF and writes a deterministic, non-empty JSON envelope to stdout.
;;
;;   in:  {"hello":"world"}
;;   out: {"wasm":true,"payload":{"hello":"world"}}
;;
;; The envelope keeps the transformed body valid JSON for any JSON input
;; (object, array, scalar) and contains the `"wasm"` marker the demo and the
;; interceptor's callers look for. Empty stdin produces empty stdout, so a
;; body that arrives empty is never turned into output.
;;
;; Only WASI `fd_read`/`fd_write` are imported — no ProcExit, no args, no
;; clocks, no filesystem. Input larger than the 128 KiB linear-memory buffer
;; is truncated at the buffer bound (the payload is then malformed JSON, but
;; output is never silently empty).
;;
;; Compilation: wat2wasm transform.wat -o transform.wasm

(module
  (import "wasi_snapshot_preview1" "fd_read"
    (func $fd_read (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "fd_write"
    (func $fd_write (param i32 i32 i32 i32) (result i32)))

  ;; 4 pages = 256 KiB of linear memory; the layout below fits well inside it.
  (memory (export "memory") 4)

  ;; Fixed memory layout (offsets are shared with the data segments below):
  ;;   0   ..  8    iovec used for the stdin read      [base, len]
  ;;   8   .. 16    iovec used for stdout writes       [base, len]
  ;;   32  .. 36    *size out-parameter of fd_read
  ;;   36  .. 40    *size out-parameter of fd_write
  ;;   64  .. 87    prefix  {"wasm":true,"payload":   (23 bytes)
  ;;   88  .. 89    suffix  }                          (1 byte)
  ;;   1024..      input buffer (128 KiB)
  (data (i32.const 64) "{\"wasm\":true,\"payload\":")
  (data (i32.const 88) "}")

  (global $iov_in i32 (i32.const 0))        ;; stdin iovec address
  (global $iov_out i32 (i32.const 8))       ;; stdout iovec address
  (global $nread i32 (i32.const 32))        ;; fd_read *size out-param
  (global $nwritten i32 (i32.const 36))     ;; fd_write *size out-param
  (global $prefix i32 (i32.const 64))
  (global $prefix_len i32 (i32.const 23))
  (global $suffix i32 (i32.const 88))
  (global $suffix_len i32 (i32.const 1))
  (global $buf i32 (i32.const 1024))
  (global $buf_cap i32 (i32.const 131072))

  ;; write_all writes the whole [ptr, ptr+len) range to stdout (fd 1),
  ;; retrying after a short write and stopping on error or EOF-of-writer.
  (func $write_all (param $ptr i32) (param $len i32)
    (local $rc i32)
    (local $n i32)
    (block $done
      (loop $loop
        (br_if $done (i32.eqz (local.get $len)))
        (i32.store (global.get $iov_out) (local.get $ptr))
        (i32.store (i32.add (global.get $iov_out) (i32.const 4)) (local.get $len))
        (local.set $rc (call $fd_write
          (i32.const 1)                  ;; fd: stdout
          (global.get $iov_out)
          (i32.const 1)                  ;; iov count
          (global.get $nwritten)))
        (br_if $done (local.get $rc))    ;; errno != 0 stops the loop
        (local.set $n (i32.load (global.get $nwritten)))
        (br_if $done (i32.eqz (local.get $n)))
        (local.set $ptr (i32.add (local.get $ptr) (local.get $n)))
        (local.set $len (i32.sub (local.get $len) (local.get $n)))
        (br $loop))))

  (func (export "_start")
    (local $total i32)
    (local $got i32)

    ;; Point the stdin iovec at the input buffer.
    (i32.store (global.get $iov_in) (global.get $buf))
    (i32.store (i32.add (global.get $iov_in) (i32.const 4)) (global.get $buf_cap))

    ;; Read stdin to EOF, appending into the buffer by advancing the iovec.
    (block $read_done
      (loop $read_loop
        ;; a non-zero errno ends the read
        (br_if $read_done
          (call $fd_read
            (i32.const 0)                ;; fd: stdin
            (global.get $iov_in)
            (i32.const 1)                ;; iov count
            (global.get $nread)))
        (local.set $got (i32.load (global.get $nread)))
        ;; a zero-length read means EOF
        (br_if $read_done (i32.eqz (local.get $got)))
        (local.set $total (i32.add (local.get $total) (local.get $got)))
        (i32.store (global.get $iov_in)
          (i32.add (i32.load (global.get $iov_in)) (local.get $got)))
        (i32.store (i32.add (global.get $iov_in) (i32.const 4))
          (i32.sub
            (i32.load (i32.add (global.get $iov_in) (i32.const 4)))
            (local.get $got)))
        ;; the buffer is full: stop reading
        (br_if $read_done
          (i32.eqz (i32.load (i32.add (global.get $iov_in) (i32.const 4)))))
        (br $read_loop)))

    ;; Empty input stays empty — never emit an envelope for no input.
    (if (i32.gt_u (local.get $total) (i32.const 0))
      (then
        (call $write_all (global.get $prefix) (global.get $prefix_len))
        (call $write_all (global.get $buf) (local.get $total))
        (call $write_all (global.get $suffix) (global.get $suffix_len)))))
)
