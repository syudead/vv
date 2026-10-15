# Wikipedia work-info plugin

A work-info plugin that looks works up on Wikipedia. It follows
[the plugin contract](../../docs/how-to/work-info-plugins.md), version 1.

- `search` runs a full-text search on the Wikipedia of the `lang` setting
  (`ja` when unset) and returns up to 20 articles, with a short extract as the
  detail.
- `fetch` takes an article URL on any language's Wikipedia, including the
  mobile `xx.m.wikipedia.org` host, follows redirects, and returns the
  article's title and its visible categories as tags.

## Build

```bash
task build-plugins
```

The module is written to `bin/plugins/wikipedia.wasm` with
[manifest.json](manifest.json) appended as the `vv.plugin` section. Add it on
the settings screen and approve it; set `lang` to `en` or another language
code to search that Wikipedia.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | The `wasip1` entry point and the `vv` host function imports |
| `wiki/` | The search and fetch logic, tested without WebAssembly |
| `plugin_test.go` | Builds the module and checks that VVMDM accepts and runs it |
