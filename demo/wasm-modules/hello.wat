;; Simple greeting WASM module
;; Compilation: wat2wasm hello.wat -o hello.wasm

(module
  (memory (export "memory") 1)

  (export "hello" (func $hello))

  (func $hello (result i32)
    i32.const 42
  )
)
