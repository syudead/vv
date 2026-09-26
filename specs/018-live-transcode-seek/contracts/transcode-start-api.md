# Contract: ライブ変換の実際の開始位置

正本は `api/openapi.yaml` で、この文書は変える引数と足す経路だけを書く。ライブ変換の本体
（`transcodeVideo`）の応答の形は変えない。

## 1. `transcodeVideo` の `attempt` 引数

`GET /api/videos/{id}/transcode.mp4?startMs=…&attempt=…`

- `attempt`: 任意。`[A-Za-z0-9_-]{1,64}`。プレイヤーが要求ごとに作る乱数で、§2 の鍵になる。
  形式に合わなければ 400 `invalid_request`。
- `attempt` があると、サーバーは変換の要求を始めた時点でそれを台帳に載せ、実際の開始位置が
  決まったとき（応答の本文を書き始める前）に記録する。同じ `attempt` で 2 度要求が来たら、後の
  要求の値で上書きする（同じ動画・同じ位置なので同じ値になる）。
- `attempt` が無い要求は今までどおり動き、台帳に載らない。

## 2. `GET /api/videos/{id}/transcode-start`

`operationId: getTranscodeStart`。`security` は `transcodeVideo` と同じ（所有者とゲスト）。

- 引数: `attempt`（必須、§1 と同じ形式）。
- 200: `{ "startMs": integer }`。変換の出力の時間軸の 0 が元動画のどの時刻かをミリ秒で返す。
  コピーで始めた変換では指定位置か直前のキーフレームの時刻（最も早く始まる track の時刻）、
  エンコードで始めた変換では指定位置そのもの。
- 待ち方: `attempt` がまだ台帳に無ければ載るまで、載っていて未決なら決まるまで待つ。上限は
  `transcodeStartupTimeout` と同じ 6 秒で、それまでに決まらなければ 404。
- 404 `not_found`: 動画が無い、ゲストが公開でない動画を指した（`transcodeVideo` と同じ判定）、
  `attempt` が上限までに現れない、変換が最初のデータを出せずに失敗した。
- 400 `invalid_request`: `attempt` の形式が違う。
- 応答は `Cache-Control: no-store`。
- 台帳の行は、変換の要求が終わってから 60 秒残して消す。再読み込み直後の報告の要求が、終わった変換の
  値をまだ引けるようにするためである。

## 3. プレイヤーの使い方

- `liveSource` は `startMs > 0` のとき `attempt` を作って URL に付け、`setSource` の直後に
  `getTranscodeStart` を呼ぶ。届くまでは現在時刻に指定位置（`pendingOffsetSeconds`）を返す。
- 200 が届いたら offset を `startMs` に置き換え、`vvOffsetChanged` で再生位置の保存にも同じ値を
  伝える。404 か誤りなら offset は指定位置のままにする（現行と同じ表示）。
- source を差し替えたあとに届いた古い `attempt` の応答は捨てる。
