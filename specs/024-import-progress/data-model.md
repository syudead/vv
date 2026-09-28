# Data Model: 取り込みの進捗と結果

SQLite の既存の表は [internal/store/migrations](../../internal/store/migrations) が正本である。
`scans` は [00002_core.sql](../../internal/store/migrations/00002_core.sql)、`jobs` と `videos` の
状態列は同じ場所の各移行にある。この feature が足すのは、次の2つの表と、`scans` の1列だけである。
ほかの表は変えない。

この feature の3つは、ARCHITECTURE.md の「Rebuildable and user data」で作り直せる側に入る
（走査と準備をやり直せば同じものができる）。

## 1. `scans.settled_at`（列の追加）

| 列 | 型 | 意味 |
| --- | --- | --- |
| `settled_at` | `integer null` | この走査の対象の動画がすべて済んだ時刻（Unix 秒） |

規則（[research.md R-4](research.md#r-4-完了は走査が閉じて集合に残りの仕事が無いときにする)）:

- 設定するのは `SettleCurrentScan` だけである。条件は、直近の走査が `state <> 'running'` で、
  `scan_videos` の動画に `queued`・`running` の仕事が無いことである。すでに値があれば変えない。
- 直近の走査の集合に未完了の仕事が加わったトランザクションは、`settled_at` を `null` に戻す。
- 走査が `failed` で閉じた場合も、残りの準備が済んだ時点で設定する。画面は `failed` を優先して示す。
- 移行は、既存の閉じた走査の `settled_at` に `finished_at` を入れる。残りの仕事があれば、次の
  `SettleCurrentScan` が正しい値で上書きする（`null` に戻すのは、仕事が加わったときだけである）。

## 2. `scan_videos`（新しい表）

直近の走査の対象の動画の集合である（[R-1](research.md#r-1-取り込みの対象を走査の記録に紐づく動画の集合として保存する)、
[R-3](research.md#r-3-集合は直近の走査の分だけを持ち新しい走査の開始で入れ替える)）。

| 列 | 型 | 意味 |
| --- | --- | --- |
| `scan_id` | `integer not null references scans(id) on delete cascade` | 走査 |
| `video_id` | `integer not null references videos(id) on delete cascade` | 対象の動画 |

主キーは `(scan_id, video_id)` である。

**加わる時点**: `jobs` へ `queued` の行を入れるトランザクション、または `queued` の仕事を
着手できるようにするトランザクションである。そのトランザクションの中で、直近の走査
（`max(scans.id)`、無ければ加えない）へ `insert ... on conflict do nothing` する。
今の該当箇所は次のとおりで、`internal/store` の中で1つの補助関数を通す。今後、仕事を積む
移行を書くときも同じ規則に従う。

- 走査の登録: `UpsertVideo` の結果に従う積み込みと、変化の無いファイルへの `EnsureJob`
- `EnqueueJob`（解析の結果と同じトランザクションで積むプレビューを含む。R-2）
- 見つからないプレビューとシーク用サムネイルの積み直し（`RequeueMissingPreview`、`requeueJob`）
- 解析のやり直し（`RetryProbe`）
- メディアフォルダの追加・付け替え: 登録された所在ができて着手できるようになった
  `queued` の仕事の動画
- `StartScan`: 前の走査から持ち越す、`queued`・`running` の仕事が残っている動画

**済みの判定**: 集合の動画のうち、`jobs` に `state in ('queued','running')` の行が無いもの。
登録された所在の無い（着手できない）仕事は、今の `Processing` と同じく残りに数えない。

**抜ける時点**: 動画の行が消えると `on delete cascade` で抜ける（元のファイルの削除や内容の変化）。
移動の場合、動画の行は内容で同じとされて残るので、新しい所在で数え続ける。`StartScan` は、
前の走査の行を消す。

## 3. `scan_issues`（新しい表）

直近の走査で起きた、利用者に知らせる出来事である（[R-6](research.md#r-6-問題は出来事ごとの行で保存し読み出しで動画ごとの1件にまとめる)）。

| 列 | 型 | 意味 |
| --- | --- | --- |
| `id` | `integer primary key` | |
| `scan_id` | `integer not null references scans(id) on delete cascade` | 走査 |
| `video_id` | `integer null references videos(id) on delete cascade` | 登録された動画。未登録のファイルは `null` |
| `path` | `text not null` | 出来事の時点のファイルの絶対パス。未登録のファイルを見分け、表示の所在にする |
| `kind` | `text not null` | 下の種類 |
| `created_at` | `integer not null` | |

一意の制約は、`(scan_id, video_id, kind)`（`video_id` が非 null のとき）と、
`(scan_id, path, kind)`（`video_id` が null のとき）の2つの部分索引である。

**種類**（`internal/domain` の列挙で、重さも domain が決める）:

| `kind` | 重さ | 記録する時点 |
| --- | --- | --- |
| `unreadable` | 失敗 | 走査がファイルの情報や内容を読めなかった |
| `changed_during_import` | 失敗 | 登録の途中でファイルが変わった |
| `register_failed` | 失敗 | 索引への書き込みや仕事の積み込みに失敗した |
| `probe_failed` | 失敗 | 解析の仕事がやり直しの上限まで失敗した |
| `thumbnail_failed` | 失敗 | 代表サムネイルの仕事が上限まで失敗した |
| `seek_thumbnail_failed` | 失敗 | シーク用サムネイルの仕事が上限まで失敗した |
| `preview_failed` | 失敗 | 一覧用プレビューの仕事が上限まで失敗した |
| `thumbnail_first_frame` | 代用 | 代表サムネイルを先頭のコマで作った |
| `seek_thumbnail_full_decode` | 代用 | シーク用サムネイルを全編から作り直した |

規則:

- 走査の3つの種類は、今ログにだけ出している `scanner.Scan` の各分岐（情報を読めない、変化の無い
  ファイルの仕事を確かめられない、`ingest` の失敗）から記録する。走査が知っている既存の動画が
  あれば、`video_id` を入れる。
- `*_failed` は `recordTerminalFailure` と同じトランザクションで入れる。同じ段階が後で成功したら、
  結果を書くトランザクションで、その動画のその段階の `*_failed` を消す。
- 代用は、その段階の成功を書くトランザクションで入れる。同じ段階が代用なしで作り直されたら、
  その行を消す。
- 前の走査の行は `StartScan` で消す（R-3）。

**まとめた1件**（読み出しの形。保存はしない）: `coalesce(video_id, path)` ごとに、種類の集合、
重さ（失敗を1つでも含めば失敗）、表示の所在を返す。所在は、動画なら今の代表の所在、未登録なら
`path` である。どちらも `domain.LocateVideoFolder` と同じ規則で、登録フォルダの表示名と相対パスに
直す。本数の数え方も同じ単位（まとめた件の数）である。
