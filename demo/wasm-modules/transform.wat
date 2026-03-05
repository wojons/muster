;; Transform WASM module
;; Compilation: wat2wasm transform.wat -o transform.wasm

(module
  ;; Import function from host
  (import "env" "log" (func $log (param i32 i32)))

  ;; Memory export
  (export "memory" (memory 1))

  ;; Export the transform function
  (export "transform" (func $transform))

  ;; Simple transformation function
  (func $transform (param $input i32) (result i32)
    ;; Return 0 for success
    i32.const 0
  )
)
