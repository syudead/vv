# Contract: 取り込みの状態と問題の一覧

正本は [api/openapi.yaml](../../../api/openapi.yaml) で、Go と TypeScript は `task generate` で作る。
この文書は、この feature が変える経路と形だけを書く。認可は変えない。どの経路も今と同じく
全体の `security: [sessionCookie]` で所有者だけが使え、ゲストには出ない（要件 11、
[internal/httpapi/auth.go](../../../internal/httpapi/auth.go) の `accessRoutes` に足さない）。
経路の名前を残す理由は [research.md R-9](../research.md#r-9-api-は-apiscans-を作り直しapiprocessing-と-sse-の-processing-をなくす) にある。

## 1. 変える・なくす・足す経路

| 経路 | 変更 |
| --- | --- |
| `POST /api/scans` | 応答の `Scan` が §2 の形になる。409・403 は変えない |
| `GET /api/scans/current` | 応答の `Scan` が §2 の形になる。一度も走査していなければ今と同じく 404 |
| `GET /api/scans/current/issues` | 新しく足す。§3 |
| `GET /api/processing` | なくす。`Processing` の schema も消す |
| `GET /api/events` | `processing` の event をなくす。`scan` は §4 |

## 2. `Scan`

直近の取り込みの、利用者から見た状態である。右下の表示と設定画面は、どちらもこの1つを読む
（要件 8）。

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `id` | int64, 必須 | 走査の id。取り込みが入れ替わったことの判定に使う |
| `status` | `finding` \| `running` \| `done` \| `partial` \| `failed`, 必須 | 利用者に見せる状態（[R-4](../research.md#r-4-完了は走査が閉じて集合に残りの仕事が無いときにする)） |
| `videos` | object \| 省略 | 進み具合。`finding` のあいだは省く |
| `videos.total` | int, 必須 | 対象の本数（[R-5](../research.md#r-5-走査中の分母は集合にまだ登録していない対象のファイルの数を足す)）。0 は「変化が無かった」 |
| `videos.settled` | int, 必須 | 済みの本数。`0 ≤ settled ≤ total` |
| `issues` | object, 必須 | 問題の本数。§3 のまとめた件を数える |
| `issues.failed` | int, 必須 | 重さが失敗の件数 |
| `issues.substituted` | int, 必須 | 重さが代用の件数 |
| `issues.revision` | int, 必須 | 問題の一覧の中身が変わるたびに増える番号（[data-model.md §1・§3](../data-model.md#3-scan_issues新しい表)）。本数が同じでも、種類や行が変われば増える |
| `settledAt` | date-time \| 省略 | 対象がすべて済んだ時刻。`done`・`partial` のときだけ返す（要件 4） |
| `activity` | object \| 省略 | 今の処理（[R-8](../research.md#r-8-今の処理は保存せずinternalapp-がメモリに持つ)）。何も動いていなければ省く |
| `activity.kind` | `registering` \| `probe` \| `thumbnail` \| `seekThumbnail` \| `preview`, 必須 | 何をしているか |
| `activity.fileName` | string, 必須 | ファイル名 |
| `activity.folder` | `VideoFolder` \| 省略 | ファイルが置かれたフォルダ。同じ名前のファイルを見分けるため |
| `activity.videoId` | int64 \| 省略 | 登録された動画なら、その id |
| `state` | `running` \| `done` \| `failed`, 必須 | 走査そのものの状態。一覧の読み直しと、取り込みを始められるかの判定に使い、画面には出さない |
| `errorCode`・`errorPath` | 省略可 | 走査が `failed` のときの理由。英語化（023）が `Scan` に足す形をそのまま引き継ぐ。023 が未導入なら、その時点の `error` を引き継ぐ |

今の `startedAt`・`finishedAt`・`total`・`completed`・`failed` はなくす。

`status` の決め方は次のとおりである。純粋関数で、`internal/domain` が持つ。上から順に最初に
当てはまるものにする。

1. `failed`: `state = failed`
2. `finding`: `state = running` で、走査がまだ対象を数え終えていない
3. `running`: `state = running`、または済んでいない対象がある
4. `partial`: `issues.failed > 0`
5. `done`: それ以外

## 3. `GET /api/scans/current/issues`

直近の取り込みの問題を、動画（未登録ならファイル）ごとに1件で返す
（[data-model.md §3](../data-model.md#3-scan_issues新しい表)）。

問い合わせの引数:

| 引数 | 意味 |
| --- | --- |
| `limit` | 1〜200、既定 50 |
| `cursor` | 前の応答の `nextCursor`。不透明な文字列 |

応答:

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `scanId` | int64, 必須 | どの取り込みの一覧か。`Scan.id` と違えば、画面は読み直す |
| `items` | `ScanIssue[]`, 必須 | 失敗を先に、同じ重さの中はファイル名、次にフォルダの順 |
| `nextCursor` | string \| 省略 | 続きがあるときだけ |

所在がどの登録フォルダにも含まれない件は、返さず、`Scan.issues` の本数にも数えない。例は、
取り込みの後にメディアフォルダを外した場合である。親 Issue の Edge Cases「対象から外れた動画は
…問題にも数えない」に従う。

一度も走査していなければ 404 を返す（`GET /api/scans/current` と同じ）。`cursor` が不正なら 400 を
返す。

`ScanIssue` の形:

| 項目 | 型 | 意味 |
| --- | --- | --- |
| `severity` | `failed` \| `substituted`, 必須 | まとめた件の重さ |
| `kinds` | `ScanIssueKind[]`, 必須, 1件以上 | data-model.md §3 の種類。重い順 |
| `fileName` | string, 必須 | |
| `folder` | `VideoFolder`, 必須 | 所在の置かれたフォルダ。同じ名前のファイルを見分けるため |
| `videoId` | int64 \| 省略 | 登録された動画なら、その id。画面は `/videos/{id}` へ移れる（要件 5） |

影響の言葉（「一覧に追加できませんでした」など）と理由の言葉は、`kinds` から SPA が組み立てる
（[R-10](../research.md#r-10-画面の言葉はサーバーが返す種類から-spa-が組み立てる)）。

## 4. `/api/events`

- `scan` の event は §2 の `Scan` を運ぶ。今と同じく、送る時点で読み、接続ごとに合流し、
  つながった直後に1回送る。
- `scan` を送る契機は次のとおりである。
  - 今の `domain.ScanChanged`
  - `domain.ProcessingChanged`（仕事の成否で済みの本数が変わるため）
  - 新しい `domain.ScanActivityChanged`
- 問題の一覧は SSE では送らない。画面は `Scan.id` か `Scan.issues.revision` が変わったら、
  §3 を読み直す。つながり直したあとも同じで、切れていた間に増えた問題が反映される。
- `video` の event は変えない。
