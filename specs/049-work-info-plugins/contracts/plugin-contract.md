# Contract: Work-info plugin contract, version 1

The contract is published for plugin authors in
[Write a work-info plugin](../../../docs/how-to/work-info-plugins.md), which is
its source of truth (P-3); this file links there and does not restate it. Why
the contract has this shape:
[R-2](../research.md#r-2-a-plugin-is-a-wasi-command-that-reads-a-json-request-and-writes-a-json-response),
[R-3](../research.md#r-3-the-manifest-is-a-custom-section-read-without-running-the-module),
[R-4](../research.md#r-4-each-run-has-no-files-no-environment-and-fixed-limits),
[R-5](../research.md#r-5-plugins-reach-the-network-only-through-vv-and-only-public-addresses),
[R-13](../research.md#r-13-plugin-settings-are-declared-in-the-manifest-and-passed-only-in-that-plugins-request).

The sections below keep the anchors this feature's artifacts link to.

## Module

See [Module](../../../docs/how-to/work-info-plugins.md#module).

## Manifest

See [Manifest](../../../docs/how-to/work-info-plugins.md#manifest).

## Request

See [Request](../../../docs/how-to/work-info-plugins.md#request).

## Response

See [Response](../../../docs/how-to/work-info-plugins.md#response).

## Host functions

See [Host functions](../../../docs/how-to/work-info-plugins.md#host-functions)
and [Limits](../../../docs/how-to/work-info-plugins.md#limits); the address
rule is in [Addresses](../../../docs/how-to/work-info-plugins.md#addresses).
