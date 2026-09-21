# Task command migration

## Summary

GNU Make を開発者コマンドの入口から外し、`Taskfile.yml` をローカル実行と CI の
唯一のコマンド契約にする。既存の Make ターゲットは同名の Task タスクへ移し、
README、仕様、コード内案内、CI を同時に更新する。

## Decisions

### Taskfile をコマンド契約の正本にする

`task check` と `task test-e2e` を CI から直接呼び、個別検査も `task lint-go` のように
手元から再現できる形を保つ。

却下した案: Makefile を互換ラッパーとして残す。入口が二つ残ると両者の差分を検査し続ける
必要があり、今回の単一化の目的を満たさないため採らない。

### OS 差を伴う処理は PowerShell スクリプトに置く

Taskfile は依存関係と公開タスクを表し、生成物差分やビルド環境変数など引用符の扱いが
複雑な処理は `scripts/` に置く。PowerShell 7.4 は既に開発者コマンドの前提である。

却下した案: Taskfile のシェル文字列へすべて埋め込む。Windows と Linux で異なるシェルの
引用規則がコマンド契約へ漏れるため採らない。

## Implementation Work

### Make ターゲットを Task へ移行する

範囲: `Taskfile.yml`、補助スクリプト、Makefile の削除。

依存: なし。

受け入れ証拠: `task check` が完走した。`task build` は `bin/mdm` を生成し、ビルド前後で
`web/dist` が同一であることを確認した。`task test-e2e` は実ブラウザ検査2件に成功した。

### 利用箇所を Task に統一する

範囲: CI、セットアップフック、README、仕様、設計文書、コード内案内。

依存: Make ターゲットを Task へ移行する。

受け入れ証拠: 実行対象のソースと現行文書に旧Makeコマンドへの参照が残らず、CI がTaskを
固定バージョンで導入して `task check` と `task test-e2e` を呼ぶ。
