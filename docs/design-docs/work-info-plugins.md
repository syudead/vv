# Work-info plugins: sandbox, limits and network

VVMDM runs work-info plugins, WebAssembly modules that read work information
from external sites, inside its own process with wazero; the adapter is
[`internal/wasmplugin`](../../internal/wasmplugin) behind the
`WorkInfoPluginRuntime` port in
[`internal/app/work_info_plugin_runtime.go`](../../internal/app/work_info_plugin_runtime.go).
The plugin contract is
[Write a work-info plugin](../how-to/work-info-plugins.md),
and the reasons for each choice are in the feature's
[research.md](../../specs/049-work-info-plugins/research.md) (R-1 to R-5).

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
Its only way to the network is the `vv` host functions, described in
[Plugins reach only public internet addresses](#plugins-reach-only-public-internet-addresses).

| Limit | Value | When it is reached |
| --- | --- | --- |
| Module size | 16 MiB | `Inspect` returns `domain.ErrWorkInfoPluginTooLarge` |
| Linear memory | 128 MiB (2048 pages) | `memory.grow` returns `-1`, the module is closed and the run fails with `memory_exceeded`, even when the plugin goes on to exit successfully |
| Run time | 30 s from taking the run slot, compiling and HTTP included | The compile or the module is stopped and the run fails with `timed_out` |
| Standard output | 1 MiB | The run is stopped with `output_too_large` |
| Standard error | 64 KiB kept, the rest discarded | Written to the server log and kept as the plugin's last run ([The last run's standard error is kept per plugin](#the-last-runs-standard-error-is-kept-per-plugin)) |
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

## The last run's standard error is kept per plugin

Each plugin keeps the standard error of its last run as one
`work_info_plugin_runs` row, with every secret setting value replaced by
`[secret]`, for the owner to read through
`GET /api/work-info-plugins/{id}/last-run`
([056 R-6](../../specs/056-plugin-tooling/research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked)).
The settings screen shows it in the last run dialog, opened from the plugin
row's `Last run output`
([056 UI design](../../specs/056-plugin-tooling/ui-design.md#last-run-dialog)).
The server log keeps receiving the same text unmasked, as before.

```mermaid
sequenceDiagram
  participant App as app.WorkInfoPlugins
  participant Rt as wasmplugin.Runtime
  participant St as store
  App->>Rt: Run(request, record)
  Rt->>Rt: take the run slot, run the module
  Rt->>App: record(run log)
  App->>App: mask the run's secret values
  App->>St: write the row if the plugin still has the generation that ran
  Rt->>Rt: release the run slot
  Rt-->>App: result, run log, error
```

| Rule | Where | Why |
| --- | --- | --- |
| `Run` returns the run log on success and on failure, and calls `record` only when the module started and ended in success or a run failure | [`run.go`](../../internal/wasmplugin/run.go) | A `busy` refusal, a module that failed to compile or ran out of time while compiling, and a cancelled request ran nothing worth replacing the last output with |
| The row is written inside `record`, before the run slot is released | `internal/app`, `internal/wasmplugin` | Rows are written in the order the runs end, so an older run never overwrites a newer one |
| Each secret value the run received is replaced wherever it occurs, and overlapping matches become one `[secret]`; in a truncated output, a tail that is the start of a secret value is replaced too | [`work_info_plugin_run_log.go`](../../internal/domain/work_info_plugin_run_log.go) | A value cut at 64 KiB leaves no part in clear; a value the plugin encodes first is not found |
| The row is written only while `work_info_plugins` has the row with the generation that ran, in one statement | [`work_info_plugin_runs.go`](../../internal/store/work_info_plugin_runs.go) | A run of a replaced (even with the same bytes), revoked or deleted plugin writes nothing |
| A replacing upload deletes the row in its transaction; deleting the plugin cascades | `internal/store` | The screen shows only output of the current module |

A failure to save the row is written to the server log and does not change
the run's result.

## Plugins reach only public internet addresses

A plugin has no sockets; vv performs each of its HTTP requests through the
`vv` host functions `http_send`, `http_meta` and `http_body`
([`host.go`](../../internal/wasmplugin/host.go)), with its own HTTP client
([`netclient.go`](../../internal/wasmplugin/netclient.go)). The client
resolves the URL's host and checks every resolved address in the dialer's
control hook, just before each connection, so a redirect and a DNS answer go
through the same check as the first request.

The diagram shows one `http_send`, from the plugin's request to its handle or
error.

```mermaid
flowchart LR
  send[http_send] --> valid{Valid request, under 16 sent?}
  valid -->|no| err1[-1 or -5]
  valid -->|yes| resolve[Resolve the host]
  resolve --> dial{Each address allowed at dial?}
  dial -->|no| refuse[-2, host recorded]
  dial -->|yes| site[Internet site]
  site -->|redirect, up to 5| resolve
  site --> body{Body within 4 MiB and 10 s?}
  body -->|yes| handle[Handle for http_meta and http_body]
  body -->|no| err2[-6 or -4]
  refuse --> fail[Run fails: blocked_address]
```

An address is allowed only when, after an IPv4-mapped IPv6 address is turned
back into IPv4, it is global unicast (for IPv6, inside `2000::/3`), it lies in
none of the ranges of `refusedPrefixes`, and it is not an address of the
server's own interfaces ([`address.go`](../../internal/wasmplugin/address.go)).
`refusedPrefixes` is one table: every range of the IANA IPv4 and IPv6
Special-Purpose Address Registries whose "Globally Reachable" is not `True`,
plus `fec0::/10` and the prefixes that embed an IPv4 address (`64:ff9b::/96`,
`64:ff9b:1::/48`, `2001::/32`, `2002::/16`). The server's interfaces are read at each check,
because they can change while vv runs.

A refused request answers `-2` to the plugin and makes the whole run fail with
`blocked_address` and the refused host, whatever the plugin writes afterwards,
so a plugin cannot hide from the owner that it tried to reach vv or the LAN. A
request that fails to connect answers `-3`; when the plugin then fails, the
run fails with `network_error` and the first host that could not be reached,
because the owner can act on the host and not on the plugin's message alone.

| Limit | Value | When it is reached |
| --- | --- | --- |
| Requests | 16 per run, `GET` and `POST`, `http` and `https` | A 17th request answers `-5`; another method or scheme answers `-1` |
| Redirects | 5 per request | The sixth redirect response is handed to the plugin as it is |
| Time | 10 s per request, from sending to the last byte of the body | The request answers `-4` |
| Response headers | 1 MiB | The request answers `-3` |
| Response body | 4 MiB | The request answers `-6` |

The client ignores `HTTP_PROXY` and the other proxy variables, because a
proxy would make the dial check see only the proxy's address. It drops `Host`
and the hop-by-hop headers a plugin sets. When the plugin sets no
`User-Agent`, it sends a common desktop browser's one (Chrome on Windows),
because a plugin reads the page the user opened, not a crawl, and a bot-like
value can get a different page or a refusal. The body reaches the plugin as
bytes with the response headers, so the plugin decodes the page's character
set itself.

The address check and the proxy support below live only in this adapter.
The ComfyUI and Ollama clients keep reaching the LAN, where those services
run, and never use the plugins' proxy.

| Rejected | Why |
| --- | --- |
| Resolving and checking the host name before a request without a proxy | The connection resolves the name again and may get another answer, and a redirect skips the check |
| A host-name allowlist per plugin | A public name can resolve to a LAN address |
| Returning the refusal to the plugin only | The plugin could ignore it, and the owner would not see that it tried |
| Separate connect, TLS and header timeouts | A slow site could then answer `-3` instead of `-4` before the 10 s request limit |

### A run can go through the operator's proxy

`Runtime.Run` takes `domain.WorkInfoRunNetwork`, whose `Proxy` is the proxy
the operator chose, or nil for none
([056 R-4](../../specs/056-plugin-tooling/research.md#r-4-the-proxy-is-one-url-in-settings-read-on-each-run-and-passed-to-the-runtime-as-a-run-option)).
`domain.ParseWorkInfoPluginProxyURL` holds the URL rule: the `http`, `https`
or `socks5` scheme, a host, an optional port and optional user information,
with no path, query or fragment, at most 2048 bytes; `""` means no proxy. A
run without a proxy uses the client above unchanged.

A run with a proxy gets its own transport, whose connections all go to the
proxy, so the dial-time check cannot see the destination. Instead, the
transport's `Proxy` callback, which Go calls for every request and every
followed redirect, resolves the destination host (or parses a literal
address) and refuses the request with `-2` and `blocked_address` when any
resolved address fails the same rule
([056 R-5](../../specs/056-plugin-tooling/research.md#r-5-through-a-proxy-the-destination-is-checked-by-resolving-its-name-before-each-request-and-redirect)).
The refused host is the name, and the proxy never sees the request. The
proxy's own address is not checked: the operator chose it, and it usually
sits on the LAN.

A proxy that cannot be reached, fails the TLS or SOCKS exchange, refuses the
`CONNECT` or answers `407` makes the request answer `-3`; when the plugin then
fails, the run fails with `network_error` and the proxy's host, because the
owner fixes the proxy setting, not the site. The transport and its
connections are closed when the run ends, so the next run uses the setting it
starts with.

The diagram shows one `http_send` with a proxy set.

```mermaid
flowchart LR
  send[http_send] --> resolve[Resolve the destination name]
  resolve --> allowed{Every address allowed?}
  allowed -->|no| refuse[-2, name recorded]
  allowed -->|yes| proxy[Operator proxy]
  proxy -->|unreachable, CONNECT refused, 407| fail[-3, proxy host recorded]
  proxy -->|redirect, up to 5| resolve
  proxy --> body[Response to the plugin]
```

The residual risk is a name whose answer changes between VVMDM's lookup and
the proxy's: VVMDM checks the address it resolved, and the proxy connects to
the address it resolves itself. A proxy that refuses private destinations
closes that gap; VVMDM cannot.

| Rejected | Why |
| --- | --- |
| Trusting the proxy to refuse LAN destinations | It holds only when the operator configured the proxy so, and VVMDM must refuse those destinations itself |
| Keeping the dial-time check through a proxy | It would see only the proxy, whose LAN address would fail every request |
| Honouring `HTTP_PROXY` | The operator could not set a proxy for plugins alone, and the Windows app has no environment to edit |

## The fetch keeps the work details and their matches

`WorkInfoPlugins.FetchWorkInfo` in
[`internal/app/work_info_fetch.go`](../../internal/app/work_info_fetch.go)
orders the fetched dates by kind and the people and organizations by role,
then matches the people and organizations through `CreditStore.Match`. The
details and the matches stay in the in-memory result with the title and the
tags, for the same 10 minutes
([049 research.md](../../specs/049-work-info-plugins/research.md) R-8). A
result without people or organizations does not read the store.

The matches are only shown: the screen marks each entry as linked, `New` or
`Not linked` from them
([052 screen-api.md](../../specs/052-plugin-work-info-fields/contracts/screen-api.md)).
The apply matches again inside its transaction, as the next section shows.
Before it writes anything, the apply refuses a selected field whose row has
no value in the result with `invalid_request`.

## The apply merges the selected fields in one transaction

`OverrideStore.ApplyFetchedWorkInfo` in
[`internal/store/work_info_fetch.go`](../../internal/store/work_info_fetch.go)
writes the selected title, tags and work details of a fetched result in one
transaction. The reasons are
[052 research.md](../../specs/052-plugin-work-info-fields/research.md) R-3 and
R-4.

The diagram shows the order of the steps inside the transaction.

```mermaid
flowchart LR
  recheck[Re-check the plugin row] --> title[Title and tags]
  title --> match[Match people and organizations again]
  match --> merge[domain.MergeFetchedWorkInfo]
  merge --> create[Create the new entities the merge kept]
  create --> write[Write the work information]
  write --> commit[Commit, then events]
```

1. The plugin row is read again, as before: a replaced, revoked or deleted
   plugin answers `ErrWorkInfoFetchStale` and nothing is written.
2. The fetched people and organizations are matched again with the same
   query as at fetch time (`matchCredits` in
   [`credit_match.go`](../../internal/store/credit_match.go)). An entity
   created between the fetch and the apply is linked, not created twice.
3. `domain.MergeFetchedWorkInfo` in
   [`work_info_merge.go`](../../internal/domain/work_info_merge.go) lays the
   selected fields over the stored work information. It has no store
   dependency, so its rules are unit-tested without SQLite.
4. Only the new entities the merge kept are created, with the fetched name
   and external IDs, through the same insert as `CreditStore.Create`.
5. The result is written with 047's replace path, only when it differs from
   the stored value. The edit time of the user key's members then advances.

After the commit the apply publishes `VideoOverrideChanged` as before and,
when the work information changed, one `VideoWorkInfoChanged`. A failure at
any step rolls back the title, the tags, the work information and the new
entities together.

| Field | Merge rule |
| --- | --- |
| `code`, `description`, a date kind | Replaces the stored value |
| A person or organization role | Appends the entities not linked in that role yet, in fetched order |
| `urls`, `externalIds` | Appends the values not stored yet, in fetched order |

A value the merge leaves out is returned as skipped with its field, the text
the comparison showed and a reason. The rest of the row is still applied.

| Reason | When |
| --- | --- |
| `invalid` | The value breaks a field rule of `NormalizeWorkInfo`, or a new entity's name or external IDs break `NormalizeCredit` |
| `too_many` | The entry is over the field's limit, counted after the stored entries, or a new entity would carry more external IDs than its limit |
| `ambiguous` | The match found several entities, or an external ID of a new entry already belongs to another new entity |

The merge decides the skips before anything is created, so a skipped entry
leaves no entity linked to no video. New entries in one result that share an
external ID or a folded name become one entity, linked once per role, and the
entity carries the external IDs of every entry folded into it, so a later
fetch that names it only by one of those IDs still finds it.
