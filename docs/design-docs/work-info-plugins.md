# Work-info plugins: sandbox and limits

VVMDM runs work-info plugins, WebAssembly modules that read work information
from external sites, inside its own process with wazero; the adapter is
[`internal/wasmplugin`](../../internal/wasmplugin) behind the
`WorkInfoPluginRuntime` port in
[`internal/app/work_info_plugin_runtime.go`](../../internal/app/work_info_plugin_runtime.go).
The plugin contract is
[contracts/plugin-contract.md](../../specs/049-work-info-plugins/contracts/plugin-contract.md),
and the reasons for each choice are in the feature's
[research.md](../../specs/049-work-info-plugins/research.md) (R-1 to R-4).

The diagram shows what a module goes through, from upload to a run's result.

```mermaid
flowchart LR
  upload[Module bytes] --> inspect[Inspect: no code runs]
  inspect -->|problem| refused[Manifest withheld, problem returned]
  inspect -->|manifest| approved[Approved by the owner]
  approved --> slot[One-run slot, waits up to 10 s]
  slot --> run[Fresh instance: stdin request, stdout response]
  run --> limits{First limit reached}
  limits -->|none| result[Candidates or work information]
  limits -->|time, memory, output| failure[Run failure]
```

## Inspection never runs the module

`Inspect` compiles the module and reads its `vv.plugin` manifest, imports,
exports and tables, and returns the first `problem` of
[data-model.md](../../specs/049-work-info-plugins/data-model.md#work_info_plugins)
it finds ([`runtime.go`](../../internal/wasmplugin/runtime.go)).

The owner approves a plugin from its name, version and settings, so these come
from a custom section rather than from code the owner has not approved yet.
wazero's compiled module shows neither tables nor non-function imports, so
[`binary.go`](../../internal/wasmplugin/binary.go) reads the import and table
sections from the bytes.

| Checked | Problem when it fails |
| --- | --- |
| The bytes compile as a WebAssembly module whose memory starts at 128 MiB or less | `not_wasm` |
| A `vv.plugin` section exists | `manifest_missing` |
| The manifest is a JSON object that follows the Manifest rules | `manifest_invalid` |
| `contract` is `1`, checked before the other fields | `contract_unsupported` |
| Every import is a `wasi_snapshot_preview1` or `vv` function with the same signature as vv's | `import_unsupported` |
| `_start` takes and returns nothing, and `memory` is exported | `export_invalid` |
| Every table's minimum, and its declared maximum when it has one, is 100,000 elements or less, and so is the sum over all tables of each table's declared maximum, or 100,000 for a table without one | `table_too_large` |

## Tables get a maximum before compiling

A table without a declared maximum, as Go's `wasip1` linker writes the
function table, gets a maximum of 100,000 elements written into the module's
bytes before it is compiled, both at inspection and before a run
(`capTables` in [`binary.go`](../../internal/wasmplugin/binary.go)).

Table elements are host memory outside the linear memory cap, so an uncapped
`table.grow` could use the server's memory without bound. Refusing such tables
would refuse every Go-built plugin. For the same reason the cap holds for the
module as a whole: many tables, each within 100,000 elements, would otherwise
add up past it.

## A run has no files, no environment and fixed limits

Each run starts a fresh instance, writes the request JSON to standard input
and reads the response JSON from standard output
([`run.go`](../../internal/wasmplugin/run.go)). The instance gets clocks and a
random source, and no preopened directory, environment variable or argument.
The `vv` host functions `http_send`, `http_meta` and `http_body` answer `-1`
until vv's HTTP client is added.

| Limit | Value | When it is reached |
| --- | --- | --- |
| Module size | 16 MiB | `Inspect` returns `domain.ErrWorkInfoPluginTooLarge` |
| Linear memory | 128 MiB (2048 pages) | `memory.grow` returns `-1`, the module is closed and the run fails with `memory_exceeded`, even when the plugin goes on to exit successfully |
| Run time | 30 s from taking the run slot, compiling and HTTP included | The compile or the module is stopped and the run fails with `timed_out` |
| Standard output | 1 MiB | The run is stopped with `output_too_large` |
| Standard error | 64 KiB kept, the rest discarded | Written to the server log only |
| Runs at the same time | 1; a second run waits up to 10 s | The second run fails with `busy` |

The memory cap is enforced by vv's memory allocator rather than by wazero's
page limit, because wazero refuses a `memory.grow` past its limit before the
allocator sees it, and vv could then not tell a memory failure from any other
non-zero exit. When a run reaches both the output and the memory limit, the
first one reached decides the failure. A run whose response is a plugin error,
a non-zero exit or output outside the contract fails with `plugin_failed`.

The run time starts when the run takes the slot, so compiling a module that is
not cached yet counts toward it. wazero checks the context between functions
only when it compiles with more than one worker, so a run compiles with two.

| Rejected | Why |
| --- | --- |
| A separate process per run | The container and the Windows app would need different OS sandboxes |
| Running a `describe` operation at upload | The name and version shown for approval would come from unapproved code |
| Several runs at the same time | Each run may use 128 MiB, and one at a time bounds the plugins' memory to one module's |
