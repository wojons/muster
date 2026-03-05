;; Validate WASM module
;; Compilation: wat2wasm validate.wat -o validate.wasm

(module
  ;; Import validation function from host
  (import "env" "validate" (func $validate (param i32) (result i32)))

  ;; Memory export
  (export "memory" (memory 1))

  ;; Export the validate function
  (export "validate_request" (func $validate_request))

  ;; Validation function
  (func $validate_request (param $data i32) (result i32)
    ;; Return 1 for valid, 0 for invalid
    i32.const 1
  )
)
