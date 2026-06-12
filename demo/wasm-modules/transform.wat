;; Transform WASM module
;; Compilation: wat2wasm transform.wat -o transform.wasm

(module
  (memory (export "memory") 1)

  (export "transform" (func $transform))

  (func $transform (param $input i32) (result i32)
    local.get $input
    i32.const 1
    i32.add
  )
)
