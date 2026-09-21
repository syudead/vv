# Data Model: 絞られたコア機能（動画ライブラリの中核）

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-09-12

001 が作った最小スキーマを拡張し、利用者データとジョブの表を足す。007では
path依存列を`video_locations`へ分離する。列の設計方針は
[技術選定文書 3.2](../../docs/design-docs/tech-stack-selection.md)に従う。

## 2種類のデータを分ける

| 区分                       | 対象                                                    | 消えたときの回復       | バックアップ           |
| -------------------------- | ------------------------------------------------------- | ---------------------- | ---------------------- |
| 再構築できる索引           | `videos`・`videos_fts`・`jobs`・`scans`・サムネイル画像 | 再スキャンで作り直せる | 不要                   |
| 再構築できない利用者データ | `playback_progress`                                     | 作り直せない           | 対象（手順は Phase 3） |

この分離のため、`playback_progress` は `videos.id` ではなく **`content_key`** を鍵に
する。動画ファイルの移動・改名・削除と再追加のいずれでも再生位置が失われない
（FR-025／FR-026／SC-008）。

---

## 1. 永続化するデータ（マイグレーション `00002`）

### `videos`（既存表の拡張）

001 で作った行へ次を足す。`path` / `title` / `size_bytes` / `mtime` は007のmigrationで
`video_locations`へ移り、`videos`はcontent単位になる。

| 列                  | 型      | 必須 | 説明                                                   |
| ------------------- | ------- | ---- | ------------------------------------------------------ |
| `content_key`       | text    | ○    | 内容由来の識別子（[R-101](./research.md)）。`unique`   |
| `duration_ms`       | integer |      | 尺（ミリ秒）。`ffprobe` 取得前は `null`                |
| `width`             | integer |      | 映像の幅                                               |
| `height`            | integer |      | 映像の高さ                                             |
| `container`         | text    |      | 拡張子から決めたコンテナ（`mp4` `webm` など）          |
| `video_codec`       | text    |      | `ffprobe` の `codec_name`                              |
| `audio_codec`       | text    |      | 同上。音声が無い場合は `null`                          |
| `playable`          | integer | ○    | 0 / 1。既定 0（解析前は「再生できない」側に倒す）      |
| `unplayable_reason` | text    |      | `container` / `video_codec` / `audio_codec` のいずれか |
| `probe_state`       | text    | ○    | `pending` / `done` / `failed`                          |
| `probe_error`       | text    |      | 解析に失敗した理由（利用者に見せる）                   |
| `thumbnail_state`   | text    | ○    | `pending` / `done` / `failed`                          |
| `updated_at`        | integer | ○    | 更新時刻（Unix 秒）                                    |

- 検証規則:
  - locationの`path`は絶対パスで、ファイルシステムが返したUnicode表現を保持し、登録済みMediaFolderの内側であること
  - locationの`title`は拡張子を除いたファイル名（NFC）。空になる場合はファイル名をそのまま使う
  - `playable = 1` は `probe_state = done` のときだけ取り得る
  - `duration_ms` は 0 以上。取得できないものは `null` のままにし、0 で代用しない
- 状態遷移（`probe_state`）: `pending` → `done` ／ `pending` → `failed`。
  再スキャンで `size_bytes` か `mtime` が変わったら `pending` に戻す
- 索引: `(added_at desc, id desc)`、`(title asc, id asc)`、`content_key`（unique）

### `videos_fts`

007ではlocationの`title`と`path`をFTS5で索引し、検索結果を`video_id`で重複排除する。
検索の2経路は [R-110](./research.md)。

### `playback_progress`（新規・利用者データ）

| 列            | 型      | 必須 | 説明                                                        |
| ------------- | ------- | ---- | ----------------------------------------------------------- |
| `content_key` | text    | ○    | 主鍵。`videos` への外部キーは張らない（動画が消えても残す） |
| `position_ms` | integer | ○    | 最後に記録した再生位置                                      |
| `duration_ms` | integer |      | 記録時点で判明していた尺。完了判定の再計算に使う            |
| `completed`   | integer | ○    | 0 / 1                                                       |
| `updated_at`  | integer | ○    | 更新時刻（Unix 秒）                                         |

- 検証規則: `position_ms >= 0`。`duration_ms` が既知なら `position_ms <= duration_ms`
  に丸める
- 完了判定はサーバー側で行う（[R-111](./research.md)）。クライアントの申告は採らない
- 競合は最後の書き込みが残る（`upsert`）

### `jobs`（新規）

| 列                          | 型      | 必須 | 説明                                     |
| --------------------------- | ------- | ---- | ---------------------------------------- |
| `id`                        | integer | ○    | 主鍵                                     |
| `kind`                      | text    | ○    | `probe` / `thumbnail`                    |
| `video_id`                  | integer | ○    | 対象。`videos` 削除時に連鎖削除する      |
| `state`                     | text    | ○    | `queued` / `running` / `done` / `failed` |
| `attempts`                  | integer | ○    | 試行回数。既定 0                         |
| `last_error`                | text    |      | 直近の失敗理由                           |
| `created_at` / `updated_at` | integer | ○    | Unix 秒                                  |

- 状態遷移: `queued` → `running` → `done`。失敗時は `running` → `queued`（`attempts` +1）、
  3 回で `failed`
- 起動時に `running` の行は `queued` へ戻す（プロセス停止からの復帰）
- 同じ `(kind, video_id)` の未完了ジョブは1件だけ（部分ユニーク索引）
- 索引: `(state, id)`

### `scans`（新規）

| 列                           | 型      | 必須 | 説明                                                       |
| ---------------------------- | ------- | ---- | ---------------------------------------------------------- |
| `id`                         | integer | ○    | 主鍵                                                       |
| `state`                      | text    | ○    | `running` / `done` / `failed`                              |
| `started_at` / `finished_at` | integer |      | Unix 秒                                                    |
| `total`                      | integer | ○    | 走査で見つけたファイル数                                   |
| `completed`                  | integer | ○    | 取り込みを終えた数                                         |
| `failed`                     | integer | ○    | 取り込めなかった数                                         |
| `error`                      | text    |      | スキャン自体が失敗した理由（対象ディレクトリが読めない等） |

- 同時に `running` は1件だけ
- 進捗は [contracts/openapi.yaml](./contracts/openapi.yaml) の `Scan` として公開する

---

## 2. ファイルとして持つデータ

| 対象       | 置き場所                   | 命名                                          | 回復         |
| ---------- | -------------------------- | --------------------------------------------- | ------------ |
| サムネイル | `MDM_DATA_DIR/thumbnails/` | `<content_key の先頭2文字>/<content_key>.jpg` | 再生成できる |

`content_key` で名前を決めるので、ファイルの移動・改名では作り直さない。動画が
ライブラリから消えても画像は残るため、スキャン完了時に参照されない画像を削除する。

---

## 3. プロセス内の値（永続化しない）

### `domain.Video`

`internal/domain` が持つ値。外部 I/O に依存しない。`videos` の行と 1 対 1 で対応し、
`Playable()` や `ResumePosition()` のような判断はここに置く。

### `domain.Probe`

`ffprobe` から取り出した事実（尺・解像度・コーデック）。`internal/media` が組み立て、
再生可否の判定（[R-103](./research.md)）は `internal/domain` の関数で行う。
これにより、許可リストの規則は外部プロセスに触れずにテストできる。

### `domain.ScanResult`

走査1回の集計（総数・追加・更新・移動・削除・失敗）。`scans` 行の元になる。

---

## 4. 想定する規模

| 対象                | 規模                     | 影響                                                |
| ------------------- | ------------------------ | --------------------------------------------------- |
| `videos`            | 1万行                    | 一覧の1ページは 60 行。`count(*)` は索引走査で数 ms |
| `playback_progress` | 数千行                   | 実際に再生した本数だけ                              |
| `jobs`              | 取り込み直後に最大 2万行 | 完了行は 7 日で掃除する                             |
| サムネイル          | 1万枚・30〜60KB          | 合計 300〜600MB                                     |
