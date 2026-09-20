# Quickstart / 受け入れ検証: 絞られたコア機能

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-12

この機能が「できた」と言える状態を、実行して確かめる手順。各シナリオは
[spec.md](./spec.md) の受け入れ条件と成功基準に対応する。実装の詳細は書かない
（`plan.md` と実装で扱う）。

## 前提

| シナリオ                 | 必要なもの                                                         |
| ------------------------ | ------------------------------------------------------------------ |
| S0（検証用の動画を作る） | `ffmpeg`（コンテナ内で実行してもよい）                             |
| S1〜S9                   | Docker（`make up`）。手元で動かす場合は Go 1.26・Node 22・`ffmpeg` |

---

## S0: 検証用の動画をつくる

実データを使わずに検証する。`ffmpeg` で色と音だけの動画を作る。

```bash
mkdir -p ./media
# 再生できる mp4（H.264 + AAC）。日本語の題名を含める
ffmpeg -f lavfi -i testsrc=size=640x360:rate=30:duration=8 \
       -f lavfi -i sine=frequency=440:duration=8 \
       -c:v libx264 -pix_fmt yuv420p -c:a aac -y "./media/海辺の散歩.mp4"
ffmpeg -f lavfi -i testsrc=size=1280x720:rate=30:duration=12 \
       -c:v libx264 -pix_fmt yuv420p -y "./media/京都の街並み.mp4"
# 再生できない例（mkv コンテナ）
ffmpeg -f lavfi -i testsrc=size=640x360:rate=30:duration=5 \
       -c:v libx264 -y "./media/対応外の動画.mkv"
# 取り込み対象外の例（動画ではない、隠しファイル、書き込み途中）
touch "./media/メモ.txt" "./media/.hidden.mp4" "./media/途中.mp4.part"
```

**期待**: `./media` に 3 本の動画と 3 個の対象外ファイルができる。

---

## S1: 動画を手動で取り込んで一覧に並べる（US1 / FR-001・FR-002 / SC-001）

```bash
MDM_MEDIA_HOST_DIR=./media make up
# ブラウザで /settings を開き、コンテナ内の /media をメディアフォルダとして登録する
# 「取り込む」を押すか、別の端末で次を実行する
curl -sS -X POST "http://localhost:8080/api/scans"
curl -sS "http://localhost:8080/api/videos" | head -c 800
```

**期待**: 手動取り込みの開始から **30 秒以内**に最初の動画が `items` に現れる。最終的に 3 件
（`total: 3`）で、`メモ.txt`・`.hidden.mp4`・`途中.mp4.part` は含まれない。
各項目に `title`（拡張子なし）・`sizeBytes`・`durationMs`・`width`・`height` が入る。

ブラウザで `http://localhost:8080` を開くと、サムネイル・題名・長さ付きの一覧が並ぶ。

## S2: 取り込みの進捗が見える（FR-006・FR-007・FR-008 / SC-009）

```bash
curl -sS "http://localhost:8080/api/scans/current"
curl -sS -X POST "http://localhost:8080/api/scans"
```

**期待**: `state`・`total`・`completed`・`failed` が返る。実行中に `POST` しても
新しいスキャンは始まらず、実行中のものが返る。スキャン中でも S1 の一覧は応答する。

壊れたファイルを1つ混ぜて再スキャンすると、`failed` が 1 増え、残りは取り込まれる。

```bash
head -c 1024 /dev/urandom > ./media/壊れた動画.mp4 && curl -sS -X POST localhost:8080/api/scans
```

## S3: 再生できない形式がひと目で分かる（FR-003 / SC-007）

```bash
curl -sS "http://localhost:8080/api/videos" | grep -o '"playable":[a-z]*' | sort | uniq -c
```

**期待**: `対応外の動画.mkv` は `playable: false`、`unplayableReason: "container"`。
一覧画面でも再生できない旨が、再生を試みる前に表示される。

## S4: ブラウザで再生し、任意の位置へ飛べる（US2 / FR-016・FR-017 / SC-004）

```bash
ID=$(curl -sS "http://localhost:8080/api/videos" | sed -n 's/.*"id":\([0-9]*\).*/\1/p' | head -1)
curl -sS -o /dev/null -D - -H "Range: bytes=0-1023" "http://localhost:8080/api/videos/$ID/stream"
```

**期待**: `206 Partial Content`、`Content-Range: bytes 0-1023/<全体>`、
`Accept-Ranges: bytes`。範囲外（`bytes=99999999999-`）は `416`。
ブラウザで動画を開くと 3 秒以内に映像が出て、シークバーの任意の位置へ飛べる。

## S5: 続きから再生できる（FR-018・FR-019 / SC-005）

```bash
curl -sS -X PUT -H 'Content-Type: application/json' \
  -d '{"positionMs": 4000}' "http://localhost:8080/api/videos/$ID/progress"
curl -sS "http://localhost:8080/api/videos/$ID" | grep -o '"progress":{[^}]*}'
```

**期待**: `positionMs: 4000`、`completed: false`。ブラウザで同じ動画を開くと 4 秒地点
（誤差 5 秒以内）から再開できる。尺の 95% 以降を送ると
`completed: true` になり、一覧で視聴済みと分かる。

## S6: 題名で探せる（US3 / FR-021〜FR-024 / SC-006）

```bash
curl -sS "http://localhost:8080/api/videos?query=街並"   # 3文字（MATCH 経路）
curl -sS "http://localhost:8080/api/videos?query=京都"   # 2文字（LIKE 経路）
curl -sS "http://localhost:8080/api/videos?query=海"     # 1文字
curl -sS "http://localhost:8080/api/videos?query=該当なし"
```

**期待**: 最初の3つはそれぞれ該当1件（`total: 1`）。1〜2文字でも取り出せることが
[TD-001](../../docs/exec-plans/tech-debt.md) の2経路が動いている証拠になる。
最後は `total: 0` で、画面には該当なしと次の操作が示される。

## S7: 移動・改名しても同じ動画として扱われる（FR-004・FR-025 / SC-008）

```bash
mkdir -p ./media/2026 && mv "./media/海辺の散歩.mp4" "./media/2026/海辺の散歩（編集済み）.mp4"
curl -sS -X POST "http://localhost:8080/api/scans" && sleep 3
curl -sS "http://localhost:8080/api/videos" | grep -c '"id"'
curl -sS "http://localhost:8080/api/videos/$ID" | grep -o '"progress":{[^}]*}'
```

**期待**: 総件数は増えない（重複が作られない）。同じ `id` のまま題名とパスが更新され、
S5 で記録した再生位置がそのまま残っている。

## S8: 消したファイルは一覧から消える（FR-005）

```bash
rm "./media/対応外の動画.mkv" && curl -sS -X POST "http://localhost:8080/api/scans" && sleep 3
curl -sS "http://localhost:8080/api/videos" | grep -c '"id"'
```

**期待**: 件数が1つ減り、他の項目は影響を受けない。

## S9: 規模と応答（SC-002・SC-003・SC-006）

```bash
# 1,000 本の検証用ファイルを作る（同じ動画を複製すると content_key が衝突するため、
# 1本ずつ内容を変える）
for i in $(seq 1 1000); do
  ffmpeg -f lavfi -i "testsrc=size=320x180:rate=15:duration=1:decimals=$((i % 3))" \
    -c:v libx264 -pix_fmt yuv420p -metadata comment="$i" -y "./media/bulk/動画$i.mp4" 2>/dev/null
done
time curl -sS -o /dev/null "http://localhost:8080/api/videos?limit=60"
time curl -sS -o /dev/null "http://localhost:8080/api/videos?query=動画5"
```

**期待**: 初回取り込みは 1,000 本で **15 分以内**に出そろう。一覧の1ページ目は
**2 秒以内**、検索は **1 秒以内**に返る。

## S10: データを消しても復旧する（FR-026）

```bash
make down && docker volume rm vv_data && MDM_MEDIA_HOST_DIR=./media make up
```

設定画面で`/media`を登録し直し、手動取り込みを開始する。

**期待**: スキーマが起動時に作り直され、folder登録と手動取り込み後に一覧が復元する。
再生位置（利用者データ）は失われる — これは「索引は再構築できる／利用者データは
できない」という区分どおりの挙動であり、バックアップ手順の整備は Phase 3 で扱う。

---

## 自動検証との対応

| 検証                          | 手段                                                           |
| ----------------------------- | -------------------------------------------------------------- |
| S1・S2・S7・S8 の取り込み規則 | `internal/scanner` の単体テスト（一時ディレクトリ）            |
| S3 の再生可否判定             | `internal/domain` の表駆動テスト（外部プロセス不要）           |
| S4 の Range 配信              | `internal/httpapi` の `httptest`（先頭・途中・末尾・不正範囲） |
| S5 の完了判定・再開位置       | `internal/domain` の単体テスト                                 |
| S6 の2経路の検索              | `internal/store` のテスト（001 の `fts_test.go` を拡張）       |
| S9 の規模                     | 手動計測。自動化は Phase 3（E2E）で検討する                    |
