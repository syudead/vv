# Research: 動画の表示名と代表サムネイルの上書き

技術スタック、境界と依存方向、索引と利用者データの区分、生成物の置き場、認証の境界は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、[internal/artifacts/store.go](../../internal/artifacts/store.go)、
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)）。
ここにはこの feature が足す決定だけを書く。

## R-1: 上書きは `content_key` に結ぶ 1 つの利用者データの表に置く

- Decision: 表示名とサムネイルの位置は、`video_overrides (content_key primary key, display_name,
  thumbnail_position_ms, thumbnail_revision, updated_at)` の 1 行で持つ（[data-model.md §1](data-model.md#1-video_overrides)）。
  `videos` への外部キーは張らず、両方の列が null になった行は消す。
- Rationale: 要件 4 と Edge Case「同じ内容が複数の所在にある」は、上書きが再スキャン・移動・改名・
  所在の追加を越えて残り、所在によらず同じであることを求める。それは `playback_progress`・`video_tags`・
  `public_videos` と同じ「作り直せない利用者データ」で、同じく内容の識別子に結ぶ
  （ARCHITECTURE.md「Rebuildable and user data」）。行の有無がそのまま「上書きされている動画」
  （要件 7）の答えになる。
- Alternatives considered: `videos` に列を足す（動画の行は索引で、最後の所在を失うと消える。
  内容を置き直したとき上書きが戻らず、要件 4 に反する）。表示名とサムネイルを別々の表にする
  （読み出しの結合が 2 つになり、「上書きされているか」を 2 か所から集めることになる。両方を
  1 回の操作で扱う外部 API も無い）。

## R-2: 有効な題名は保存層が決め、`domain.Video` が表示名とファイル名由来の題名を両方持つ

- Decision: 動画を返す読み出し（一覧・詳細・関連・外部連携の一覧と引き当て）は `video_overrides` を
  左結合し、`Video.Title` を「表示名があればそれ、無ければ所在の題名」にする。`Video.FileTitle`
  にファイル名由来の題名、`Video.DisplayName` に表示名（未設定は空）、`Video.ThumbnailPositionMs` に
  サムネイルの位置（未設定は nil）を載せる（[data-model.md §2](data-model.md#2-domain-に足す値)）。
  `Video.Public` を `public_videos` から埋めるのと同じ作りである。
- Rationale: 題名は一覧・フォルダ表示・動画ページ・関連動画・ゲストの画面・外部 API と MCP のすべてに
  出る（要件 1）。読み出しの 1 か所で決めれば、応答を組み立てる側は `Title` をそのまま出すだけで、
  出し忘れが起きない。要件 7・8 の「ファイル名由来の題名」と「設定されているか」も同じ行から出る。
- Alternatives considered: `internal/httpapi` で `title` を差し替える（並び替えと検索が保存層にあり、
  そこでは差し替えが効かない。要件 3 に反する）。`video_locations.title` を書き換える（題名は所在の
  事実で、スキャンのたびに書き直される。上書きが次のスキャンで消える）。

## R-3: 並び替えと検索は、所在ごとの `title_key`・`search_key` に表示名を織り込む

- Decision: `video_locations.title_key` は有効な題名（表示名があればそれ）の `domain.NaturalSortKey`
  にする。`search_key` は今の「題名 `\n` 相対パス」に表示名を 3 つ目の部分として足す
  （`domain.SearchKeyVersion` は上げない。表示名の無い所在の鍵は今と同じ値になる）。鍵を書き直すのは
  3 か所で、表示名を書き換える取引（その内容の動画の全所在）と、取り込みの所在の追加・更新
  （`refreshSearchKeysByPath` が表示名を読む）と、メディアフォルダの追加・変更
  （`AddMediaFolder`・`ReplaceMediaFolder` の `refreshSearchKeysUnder` が表示名を読む）である（[data-model.md §4](data-model.md#4-並び替えと検索の鍵)）。
- Rationale: 要件 3 は、題名順では表示名の位置に並び、検索は表示名とファイル名の両方に当たることを
  求める。並び替えは `loc.title_key` を、検索は `location_search_fts` を読んでいて
  （[specs/013-library-search/data-model.md](../013-library-search/data-model.md)）、そこに値を入れれば
  一覧・フォルダ・ライブラリ項目のどの問い合わせも変えずに済む。鍵を作り直す経路は既にある。
- Alternatives considered: `video_overrides` に FTS を張り、検索式で OR する（`chosenLocationsCTE` と
  ライブラリ項目の CTE の両方を変え、検索の照合を 2 つの索引に分ける）。並び替えを `coalesce(表示名,
  title_key)` にする（表示名の自然順の鍵は SQL では作れない。`title_key` が自然順で保存されているのは
  Go が `NaturalSortKey` を書くからである）。

## R-4: サムネイルの位置の指定は、要求の中で生成してから記録する

- Decision: 位置の設定と解除は `app.Ingest.SetThumbnailPosition` が行う。生成の錠
  （`artifacts.generate` の `artifactThumbnail`）の中で、`PublishThumbnail` に新しい画像を書かせ
  （指定なら `ThumbnailAt`、解除なら今の自動の規則の `Thumbnail`）、公開できたら 1 つの取引で
  `video_overrides` の位置を書き、その動画の `thumbnail_state` を `done` にし、`thumbnail_first_frame`
  の代用の行を消す。生成に失敗したら何も書かず、誤りを返す
  （[contracts/screen-api.md §2](contracts/screen-api.md#2-put-apivideosidthumbnail-position)）。
  応答は生成が終わってから返す。
- Rationale: Edge Cases は「失敗したらそれまでの画像を残し、失敗したことが所有者に分かる」
  「生成が終わる前の別の指定は最後の指定が勝つ」を求める。`PublishThumbnail` は一時置き場に書いて
  置き換えるので、失敗しても前の画像は残る。要求の中で生成すれば、失敗はその応答で伝わり、同じ内容の
  指定は錠で直列になって、最後に記録した位置の画像が残る。`-ss` を入力の前に置く 1 コマの抽出は
  数秒で終わる（`internal/media/thumbnail.go`）。
- Alternatives considered: `thumbnail` の job を積んで worker に作らせる（失敗すると
  `thumbnail_state` が `failed` になり、前の画像が `thumbnailUrl` から消える。所有者への失敗の伝達は
  取り込みの問題の一覧に回り、動画ページでは分からない。実行中の job と次の指定は
  `jobs_pending_kind_video_idx` でぶつかる）。画面の API が直接 `internal/media` を呼ぶ
  （生成の錠と削除の直列化を持つのは `internal/app` で、httpapi は生成の判断をしない。ARCHITECTURE.md）。

## R-5: 取り込みのサムネイル job も指定の位置で作り、自動の規則は `internal/media` に残す

- Decision: `Ingest.Thumbnail`（job）は `Video.ThumbnailPositionMs` があればその位置で
  （`generator.ThumbnailAt`）、無ければ今の規則で作る。位置と `thumbnail_state` は、生成の錠
  （`artifacts.generate`）に入ってから `IngestStore.GetVideo` で読み直した値で決める。錠の外で先に
  読んだ `video` は尺と存在の確認にだけ使う。`internal/media` に
  `ThumbnailAt(ctx, path, positionMs, output) error` を足す。指定の位置で取れないときの先頭のコマへの
  代用は行わず、失敗は今の再試行と `failed` の記録に従う。自動の位置の規則（10%・1 秒〜60 秒）は
  `internal/media` の定数のまま、`internal/domain` には移さない。
- Rationale: 動画の行が消えて生成物が片付けられたあと、同じ内容を置き直すと `thumbnail` の job が
  画像を作り直す。job が位置を知らないと、行（利用者データ）は指定の位置を持つのに画像は自動の
  場面になる（Edge Case「どの所在から見ても同じサムネイル」）。同じ内容のファイルで一度取れた位置は
  もう一度取れるので、job の側に別の代用は要らない。錠の中で読み直すのは、job が錠の外で動画を
  読んだあと、錠を待つ間に `SetThumbnailPosition` が先に錠を取って位置と `done` を記録しうるためで
  ある。前に読んだ値のままだと、job は古い状態（`done` でない）と古い位置で画像を上書きし、記録された
  位置と画像が食い違う（Edge Case「最後の指定が勝つ」）。読み直せば、記録済みの `done` を見て生成を
  飛ばすか、記録された位置で作る。位置の規則は生成の都合（ffmpeg が先頭で 0 枚
  出力する問題）で、判断ではないので移さない。
- Alternatives considered: 指定の画像を別のファイル（`<s>.pick.jpg`）に置く（置き場の並べ方を変える
  ことになり、`RemoveContent`・ETag・配信の分岐が増える。1 つの内容に 1 つの代表画像で足りる）。
  job で指定の位置に失敗したら行を消す（利用者データを job が書き換える。取り込みの失敗で
  設定が消える）。

## R-6: `thumbnailUrl` の版に指定の改版番号を含める

- Decision: `video_overrides.thumbnail_revision` は位置を記録するたびに書く改版番号で、
  「今のミリ秒時刻と前の値 + 1 の大きい方」にする（解除では null）。`thumbnailURL` の `v` は、位置が
  指定されているとき `<内容鍵の先頭>-r<thumbnail_revision>` にする。未指定は今のまま。位置の値そのものは
  URL に入れない。
- Rationale: 画像は `private, no-cache` と `ETag` で配るが、`<img>` の `src` が同じ文字列なら
  ブラウザは要求し直さない。位置を変えた直後の一覧と動画ページで新しい画像を出すには `src` が
  変わる必要がある。解除は「位置なし」の URL に戻るので、こちらも変わる。`thumbnailUrl` はゲストにも
  渡るので、ゲストの応答から省く `thumbnailPositionMs` を URL で漏らさないよう、版は位置と無関係な値に
  する。記録のたびに増えるので、同じ位置を指定し直しても URL は変わり、行を消して作り直しても時刻で
  前の値と重ならない。
- Alternatives considered: 版に `positionMs` をそのまま入れる（ゲストに所有者だけの項目が見える）。
  `updated_at` を版にする（表示名の変更でもサムネイルの URL が変わり、要求し直しが無駄に起きる）。
  応答のたびに `updated_at` を版にする（`video_overrides` の無い動画も
  含めて URL が安定しなくなり、ETag による 304 の意味が薄れる）。

## R-7: 上書きの変化は `domain.VideoOverrideChanged` を発行し、画面の `video` 通知に写す

- Decision: `video_overrides` を書く取引の確定後に、保存層が `VideoOverrideChanged{VideoID}` を
  発行する（表示名・位置とも）。`cmd/mdm/events.go` の画面の購読がそれを `/api/events` の `video` に
  写し、一覧の再取得と動画ページの再取得は今の仕組みのまま動く。worker の起床には結ばない。
- Rationale: 一覧はスナップショットから復元されるので（`listSnapshot.ts`）、動画ページで変えた
  題名とサムネイルは `video` 通知が無いと戻ったときに古いままになる。別のタブや外部 API からの変更
  （Edge Case）も同じ通知で届く。
- Alternatives considered: `VideoIngestChanged{Stage: thumbnail}` を流用する（表示名の変更は
  取り込みの段階の変化ではなく、seek_thumbnail の worker の起床がそれを購読している）。
  画面が保存の応答だけで更新する（他のタブと外部 API からの変更が届かない）。

## R-8: 画面の API は動画ごとの 2 つの `PUT` にし、空の表示名と `null` の位置が解除である

- Decision: `PUT /api/videos/{id}/display-name`（本文 `{ displayName }`。整えて空なら解除）と
  `PUT /api/videos/{id}/thumbnail-position`（本文 `{ positionMs }`。`null` で解除）。どちらも所有者
  だけで、応答は `GET /api/videos/{id}` と同じ `Video`（[contracts/screen-api.md](contracts/screen-api.md)）。
- Rationale: Edge Case が「空文字や空白だけの表示名は解除として扱う」と決めているので、解除に別の
  操作は要らない。位置の設定は生成を伴い数秒かかり、名前の保存は即時なので、1 つの操作にまとめると
  片方だけ失敗した応答を作ることになる。応答に詳細の `Video` を返すのは、動画ページが移動せずに
  題名・ファイル名・サムネイルを置き換えるため（UI 品質「表示名の変更は動画ページから移動せずに
  完了する」）。
- Alternatives considered: `PATCH /api/videos/{id}`（上と同じ理由で分ける）。`DELETE` を足す
  （空・`null` で解除できるので操作が増えるだけ）。

## R-9: 外部連携 API は一括の 2 操作を足し、MCP は同じ 2 ツールを足す

- Decision: `POST /api/v1/video-display-names`（1〜20000 件、1 つの取引）と
  `POST /api/v1/video-thumbnails`（1〜20 件。先に全件を検証し、順に生成して、最初の生成の失敗で
  止まる）を足し、`ExternalVideo` に `fileTitle`・`displayName`・`thumbnailPositionMs` を足す。MCP には
  `update_video_display_names`・`update_video_thumbnails` を足す
  （[contracts/external-api.md](contracts/external-api.md)）。
- Rationale: 要件 8 と対象外「一括の変更は外部 API と MCP で行う」。動画の指定は `VideoRef`、誤りの
  `index`、全件検証してから反映する形は `POST /api/v1/video-tags` と同じにし、利用者が覚える形を
  増やさない。サムネイルは 1 件ごとに ffmpeg を走らせるので、タグと同じ 20000 件は 1 つの要求に
  収まらず、20 件で区切る。生成の失敗は途中で起こりうるので、そこまでを反映したまま `index` で
  止まった位置を返し、残りは利用者が送り直す。
- Alternatives considered: 生成の失敗も全件を戻す（公開済みの画像は取引で戻せない）。1 件ごとの
  結果を 200 で返す（利用者が配列を読んで誤りを探すことになり、他の操作の「誤りは状態コード」と
  食い違う）。表示名とサムネイルを 1 つの操作にする（サムネイルの件数の上限が表示名の一括を縛る）。

## R-10: 表示名の規則はタグ名の規則にそろえ、上限は 200 符号位置にする

- Decision: `domain.NormalizeDisplayName` は、制御文字を含む入力を拒み
  （`display_name_control_characters`）、前後の空白を取り除き、空なら「解除」を返し、
  `DisplayNameMaxLength = 200` 符号位置を超える入力を拒む（`display_name_too_long`、`limit`）。
- Rationale: 改行を含む題名はカードの 1 行に収まらず検索の語とも合わないので、タグ名と同じ理由で
  拒む（[specs/014-video-tags/data-model.md §2](../014-video-tags/data-model.md#2-名前の規則)）。空を
  解除にするのは Edge Case の決め。上限は、置き換える対象がファイル名（255 バイト）で、
  カードと動画ページは今その長さの題名を収めているので、それを下回らない 200 にする。
- Alternatives considered: タグ名と同じ 100（ファイル名由来の題名より短くなり、今出ている題名を
  表示名にできないことがある）。上限なし（カードの 1 行と `title_key` の索引が際限なく長くなる）。

## R-11: 位置の検証は `internal/domain` の純粋関数が持ち、誤りは 3 つに分ける

- Decision: `domain.CheckThumbnailPosition(video Video, positionMs int64) error` は、`ProbeState` が
  `done` でないか `DurationMs` が無い・0 なら `ErrDurationUnknown`（409 `conflict` /
  `duration_unknown`）、`positionMs < 0` か `>= DurationMs` なら `ErrThumbnailPositionOutOfRange`
  （400 `invalid_request` / `thumbnail_position_out_of_range`、`limit` = 尺）を返す。生成の失敗は
  `ErrThumbnailFrameUnavailable`（409 `conflict` / `thumbnail_frame_unavailable`）。
- Rationale: Edge Case「解析前で長さが分からない動画や、尺を超える位置は受け付けず理由を返す」と
  「生成に失敗したら分かる」。尺の外は入力の誤り、解析前と生成の失敗は動画の今の状態の問題なので
  コードを分ける。既存の `ErrSeekFrameUnavailable` が 409 なのにそろえる。
- Alternatives considered: 尺の外を末尾に丸める（利用者が指した場面と違う画像になり、理由が
  返らない）。解析前も 400 にする（入力は正しく、後で解析が終われば通る）。
