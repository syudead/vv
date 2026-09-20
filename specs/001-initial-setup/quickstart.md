# Quickstart / 受け入れ検証: 初期セットアップ（Phase 0 骨組み）

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-12

Phase 0 が「できた」と言える状態を、実行して確かめる手順。
各シナリオは [spec.md](./spec.md) の受け入れ条件と成功基準に対応する。
実装の詳細はここに書かない（`plan.md` と実装で扱う）。

## 前提

| シナリオ             | 必要なもの                                       |
| -------------------- | ------------------------------------------------ |
| S1〜S5（起動と動作） | Docker のみ                                      |
| S6〜S9（検証と CI）  | Go 1.26 以上、Node 22、`golangci-lint`、`ffmpeg` |

設定は不要。すべて既定値で動く（[contracts/configuration.md](./contracts/configuration.md)）。

---

## S1: clone から1コマンドで起動する（US1 / FR-001 / SC-001）

```bash
git clone <repository-url> && cd vv
make up
```

**期待**: 依存の取得とビルドを含めて起動が完了し、`http://localhost:8080` が開ける。
計測: clone からブラウザで稼働表示を見るまで **15 分以内**、実行したコマンドは
`make up` の **1つだけ**。

## S2: 稼働確認の入口が機械可読な応答を返す（FR-002）

```bash
curl -sS -i http://localhost:8080/api/health
```

**期待**: `200`、`Content-Type: application/json; charset=utf-8`、`Cache-Control: no-store`。
本文は `status` と `version` を必ず含む
（[contracts/openapi.yaml](./contracts/openapi.yaml) の `Health`）。

```bash
# 契約との一致を機械的に確認する（任意）
curl -sS http://localhost:8080/api/health | jq -e '.status == "ok" and (.version | length > 0)'
```

## S3: ブラウザから稼働が見える（FR-003）

`http://localhost:8080` を開く。

**期待**: アプリケーション名と稼働状態が表示される。
未知のパス（例: `http://localhost:8080/anything`）を開いても同じ画面が表示される。
一方 `curl -sS -i http://localhost:8080/api/nope` は **HTML ではなく** JSON の
`404` を返す（[contracts/http-routes.md](./contracts/http-routes.md)）。

## S4: データを消しても手作業なしで復帰する（FR-005 / SC-006）

```bash
make down
docker volume rm vv_data   # 実際のボリューム名は compose.yaml に合わせる
make up
curl -sS http://localhost:8080/api/health | jq -r .status
```

**期待**: 追加の操作なしに起動が完了し、`ok` が返る。
記録にマイグレーションを適用した旨が出ている。

## S5: 安全に停止する（FR-006 / 停止の契約）

```bash
make down          # あるいは起動中のプロセスに SIGTERM
```

**期待**: 処理中の要求を打ち切らずに終了し、終了コードは `0`。
再起動しても S2 が同じ結果になる（データを壊していない）。

## S6: 前提ツールが欠けていれば原因が分かる形で止まる（FR-008 / SC-008）

コンテナ外で、`PATH` から `ffprobe` を外して起動する。

```bash
env PATH=/usr/bin:/bin MDM_DATA_DIR=/tmp/mdm ./bin/mdm; echo "exit=$?"
```

**期待**: 終了コードが非0。出力に **不足しているコマンド名**と導入方法が含まれる。
「なぜ落ちたか分からない」出力になっていないこと。

## S7: 検証一式が1コマンドで通る（FR-011 / SC-002）

```bash
time make check
```

**期待**: 書式・静的検査・テスト・生成物の差分確認がすべて成功し、
**5 分以内**に終わる。

## S8: 依存方向の違反が止まる（US2 / FR-010 / FR-013 / SC-003）

`internal/domain` のいずれかのファイルに、禁止された import を一時的に足す。

```go
import _ "net/http"
```

```bash
make lint; echo "exit=$?"
```

**期待**: 失敗する。出力に「`internal/domain` から `net/http` を import してはならない」
という**理由の文言**が含まれる。確認後は変更を戻し、`make lint` が再び成功すること。

同じことを `database/sql` と `os/exec` でも確認する。

## S9: 日本語の部分一致検索の前提が成り立つ（US3 / FR-014 / SC-007）

```bash
go test ./internal/store/ -run FTS -v
```

**期待**: 以下がすべて期待どおりであること（[R-001](./research.md) の実測に対応）。

| 検証                                  | 期待                                     |
| ------------------------------------- | ---------------------------------------- |
| `tokenize='trigram'` の仮想表を作成   | 成功                                     |
| 3文字の語（`夏休み`）で `MATCH`       | 一致する                                 |
| 語中の3文字（`みの旅`）で `MATCH`     | 一致する                                 |
| 2文字の語（`旅行`）で `MATCH`         | **一致しない**（この挙動自体を固定する） |
| 2文字の語で FTS5 表に `LIKE '%旅行%'` | 一致する                                 |

追加のミドルウェアを起動していない状態で通ること。

## S10: 変更提案で同じ検証が自動実行される（US2 / FR-012 / SC-004）

任意の変更で Pull Request を作る。

**期待**: `make check` と同じ検査が自動で走り、結果が PR 上で見える。
完了まで **10 分以内**。S8 の違反を含む PR は失敗する。

---

## 完了の判定

S1〜S10 がすべて期待どおりであれば、Phase 0 は完了とみなす。
結果と、その過程で判明した妥協点は
[docs/exec-plans/completed/001-initial-setup.md](../../docs/exec-plans/completed/001-initial-setup.md)
と [docs/exec-plans/tech-debt.md](../../docs/exec-plans/tech-debt.md) に記録する。
