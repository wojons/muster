;; Simple greeting WASM module
;; Compilation: wat2wasm hello.wat -o hello.wasm

(module
  ;; Import function from host for logging
  (import "env" "log" (func $log (param i32 i32)))

  ;; Memory export for host access
  (export "memory" (memory 1))

  ;; Export the add function
  (export "hello" (func $hello))

  ;; Function that returns a greeting
  (func $hello (param $namelen i32) (result i32)
    ;; Return 0 for success
    i32.const 0
  )
)
