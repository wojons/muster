;; Validate WASM module
;; Compilation: wat2wasm validate.wat -o validate.wasm

(module
  (memory (export "memory") 1)

  (export "validate_request" (func $validate_request))

  (func $validate_request (param $data i32) (result i32)
    i32.const 1
  )
)
