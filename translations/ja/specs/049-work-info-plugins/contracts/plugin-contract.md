---
source: specs/049-work-info-plugins/contracts/plugin-contract.md
sourceHash: b72731f58cb4a41b352779fed732a80bd380d73c6bd0e756fcd269a10613718b
---

# 契約: 作品情報プラグインの契約、バージョン 1 {#contract-work-info-plugin-contract-version-1}

契約はプラグインの作者に向けて [作品情報プラグインを書く](../../../docs/how-to/work-info-plugins.md) で公開しており、それが正本である (P-3)。このファイルはそこへリンクし、内容を繰り返さない。契約がこの形である理由: [R-2](../research.md#r-2-a-plugin-is-a-wasi-command-that-reads-a-json-request-and-writes-a-json-response)、[R-3](../research.md#r-3-the-manifest-is-a-custom-section-read-without-running-the-module)、[R-4](../research.md#r-4-each-run-has-no-files-no-environment-and-fixed-limits)、[R-5](../research.md#r-5-plugins-reach-the-network-only-through-vv-and-only-public-addresses)、[R-13](../research.md#r-13-plugin-settings-are-declared-in-the-manifest-and-passed-only-in-that-plugins-request)。

以下の節は、この機能の成果物がリンクするアンカーを保つ。

## モジュール {#module}

[モジュール](../../../docs/how-to/work-info-plugins.md#module) を参照する。

## マニフェスト {#manifest}

[マニフェスト](../../../docs/how-to/work-info-plugins.md#manifest) を参照する。

## 要求 {#request}

[要求](../../../docs/how-to/work-info-plugins.md#request) を参照する。

## 応答 {#response}

[応答](../../../docs/how-to/work-info-plugins.md#response) を参照する。

## ホスト関数 {#host-functions}

[ホスト関数](../../../docs/how-to/work-info-plugins.md#host-functions) と [上限](../../../docs/how-to/work-info-plugins.md#limits) を参照する。アドレスの規則は [アドレス](../../../docs/how-to/work-info-plugins.md#addresses) にある。
