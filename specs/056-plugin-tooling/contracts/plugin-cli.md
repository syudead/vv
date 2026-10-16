# Contract: Host commands for plugin authors

Source of truth: [cmd/mdm](../../../cmd/mdm) and the authors' guide
[docs/how-to/work-info-plugins.md](../../../docs/how-to/work-info-plugins.md),
which the implementation updates. Both commands are subcommands of the server
binary `mdm`, like `mdm account`
([016 account-cli.md](../../016-single-account-auth/contracts/account-cli.md));
`mdm` with no arguments still starts the server, and `VVMDM.exe` does not take
them ([R-1](../research.md#r-1-mdm-plugin-run-and-mdm-plugin-pack-reuse-the-servers-runtime-and-parser-and-open-no-database)).
Neither command reads `MDM_DATA_DIR` or needs `ffmpeg`.

The diagram shows an author's loop.

```mermaid
flowchart LR
  build[go build, wasip1] --> pack[mdm plugin pack]
  pack --> run[mdm plugin run]
  run -->|fix, rebuild| build
  run -->|works| add[Add on the settings screen]
```

## `mdm plugin pack <module.wasm> <manifest.json>`

Writes the manifest into the module as its `vv.plugin` section, replacing an
existing one, after the packed module passes the same inspection as an
upload ([R-2](../research.md#r-2-pack-replaces-the-vvplugin-section-and-writes-the-file-only-after-the-packed-module-passes-inspection)).

| Argument | Meaning |
| --- | --- |
| `<module.wasm>` | The built module; written in place through a temporary file and a rename |
| `<manifest.json>` | The manifest ([plugin-contract.md, Manifest](plugin-contract.md#manifest)) |

| Result | Exit code | Standard error |
| --- | --- | --- |
| Written | 0 | The plugin's name, version, operations and declared settings, as VVMDM would list them |
| The manifest is not a JSON object or breaks a rule | 2 | `manifest_invalid` or `contract_unsupported`, the field and the rule it breaks; the file is unchanged |
| The packed module fails inspection | 2 | The problem (`not_wasm`, `import_unsupported`, `export_invalid`, `table_too_large`); the file is unchanged |
| A file cannot be read or written | 1 | The reason; the module is unchanged when the rename did not happen |
| Wrong arguments | 2 | The reason and usage |

## `mdm plugin run <module.wasm> <operation> <input> [--set KEY=VALUE]... [--proxy URL]`

Runs the module once as VVMDM would, and prints what VVMDM would keep of the
response.

| Argument | Meaning |
| --- | --- |
| `<operation>` | An operation of the plugin contract: `search` or `fetch`; a later contract's operation is accepted when the manifest declares it |
| `<input>` | The operation's input: the words for `search`, the URL for `fetch`, checked by the same rules as the screen API |
| `--set KEY=VALUE` | A setting value, repeatable; `KEY` must be declared by the manifest, and a `required` key without a value or a default stops the command |
| `--proxy URL` | The proxy for the run, as `PUT /api/settings/plugin-proxy` takes it ([R-4](../research.md#r-4-the-proxy-is-one-url-in-settings-read-on-each-run-and-passed-to-the-runtime-as-a-run-option)); none without the flag |

The command inspects the module first and refuses what an upload refuses.
The run has the limits of [Limits](../../../docs/how-to/work-info-plugins.md#limits)
and the address rule of [Addresses](../../../docs/how-to/work-info-plugins.md#addresses);
the request carries the effective setting values
([R-3](../research.md#r-3-a-setting-declares-default-and-options-the-stored-value-or-the-default-is-what-the-run-receives)).

| Result | Exit code | Standard output | Standard error |
| --- | --- | --- | --- |
| The response is valid | 0 | The kept result as indented JSON: `{"candidates": […]}` or `{"workInfo": {…}}` after VVMDM's trimming and limits | The plugin's standard error, as written, then `truncated after 64 KiB` when it was |
| The run failed | 1 | Nothing | The plugin's standard error, then one line with the failure (`plugin_failed`, `timed_out`, `blocked_address`, `network_error`, `memory_exceeded`, `output_too_large`), the host for the two network failures, and the plugin's message for `plugin_failed` |
| The module fails inspection, an undeclared or missing setting, an input or proxy URL that breaks its rule | 2 | Nothing | The reason and usage |
| The file cannot be read | 1 | Nothing | The reason |

Secret values given with `--set` are printed as the plugin wrote them: the
author typed them, and the masking of
[R-6](../research.md#r-6-the-runtime-returns-each-runs-standard-error-and-the-app-keeps-the-last-one-per-plugin-with-the-secret-values-masked)
is for the owner's screen.
