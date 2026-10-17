;; CLI argument validation only needs a valid module and its manifest.
;; Keep this fixture free of a language runtime; tests attach their manifest.
;; Regenerate from the repository root:
;; wat2wasm cmd/mdm/testdata/minimal.wat -o cmd/mdm/testdata/minimal.wasm
(module
  (memory (export "memory") 1)
  (func (export "_start")))
