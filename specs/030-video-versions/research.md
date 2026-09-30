# Research: 同じ動画の別バージョンを束ねる

技術スタック、境界と依存方向、索引と利用者データの区分、生成物の置き場、認証の境界は正本に従う
（[docs/design-docs/tech-stack-selection.md](../../docs/design-docs/tech-stack-selection.md)、
[ARCHITECTURE.md](../../ARCHITECTURE.md)、[internal/artifacts/store.go](../../internal/artifacts/store.go)、
[specs/016-single-account-auth/contracts/guest-api.md](../016-single-account-auth/contracts/guest-api.md)）。
先行の検討（ブランチ `claude/video-detection-synonym-zgwmlw` の `docs/design-docs/video-identity.md`）は
方式の候補を並べたもので、ここにはこの feature が確定する決定だけを書く。

## R-1: 集まりは利用者データの表に持ち、集まりの値は集まり自身の鍵で既存の表に置く

- Decision: 集まりは `video_bundles`（`user_key`・`representative_key`）と `video_bundle_members`
  （`content_key` → 集まり）の 2 つの利用者データの表で持つ（[data-model.md §1](data-model.md)）。
  集まりのタグ・再生位置・公開の設定は、`video_tags`・`playback_progress`・`public_videos` の既存の表に
  `user_key`（`bundle:<id>`）を鍵として置く。束ねる操作は、選んだ代表の値をこの鍵へ写す。各メンバーの
  `content_key` の行はそのまま残す。
- Rationale: 要件 2・3・7 と Edge Case は、集まりの値がどのメンバーにも属さないことを求める。代表を
  替えても値は変わらず（要件 7）、外したメンバーは自分の値に戻り（要件 3）、代表のファイルが消えても
  集まりと値は残る（Edge Case）。値を代表の `content_key` に置くと代表の変更で移し替えが要り、外した
  ときに戻す値と集まりの値を分けられない。既存の表に置けば、読み出しは鍵の引き直し（R-2）だけで済み、
  `playback_progress` などの列と規則（完了の判定、`updated_at` の並び）を 2 重に持たない。
- Alternatives considered: 先行の検討の案 A（別名の `content_key` を正の鍵へ付け替え、行を統合する。
  要件 3 の「外したときに戻る」を満たせず、代表の変更で正の鍵を動かすと利用者データの鍵が変わる）。
  案 B（利用者データの表の鍵を同一性の id に替える。3 つの表の移行と「利用者データは内容の識別子に
  結ぶ」という前提の書き換えが要り、束ねていない動画にも新しい id が要る）。

## R-2: 利用者データの読み書きは「利用者データの鍵」を 1 つの式で引き、`Video.UserKey` に載せる

- Decision: 動画の利用者データの鍵は「集まりのメンバーならその `user_key`、そうでなければ
  `content_key`」で、`internal/store` の 1 つの式（`userKeyExpr`、[data-model.md §3](data-model.md)）が
  決める。動画を返す全読み出しはこれを `domain.Video.UserKey` に載せ、`internal/httpapi` の再生位置と
  タグの引き当て（`progressFor`・`tagsFor`・`withProgress`・`withTags`）と再生位置の保存はこの鍵を使う。
  タグの付け外し・要約・公開の切り替えの `registeredContentKeysForVideoIDs` はこの鍵へ引き直す
  `userKeysForVideoIDs` に替え、表示名の上書き（内容ごと）だけが `content_key` の引き直しを使い続ける。
  公開の判定（`publicVideoCondition`・`publicColumn`）、タグの絞り込みと検索欄の照合、視聴状態の結合も
  同じ式で結ぶ。
- Rationale: 集まりの値を既存の表に置く（R-1）以上、読む側と書く側のどちらか 1 か所でも
  `content_key` のまま残ると、「タグは見えるのにタグで絞ると出ない」「公開したのにゲストに見えない」の
  食い違いになる。式を 1 つにし、`Video` に載せてしまえば、応答を組み立てる側は鍵を選ばない。
  表示名とサムネイルの位置はファイルの事実に近く（要件 7 で題名は新しい代表のものになる）、内容ごとに
  残す。
- Alternatives considered: `internal/httpapi` で集まりを引いて鍵を差し替える（一覧の SQL が直接引く
  タグ・視聴状態・公開の条件には効かない）。`Video.ContentKey` 自体を集まりの鍵にする
  （生成物・指紋・上書きは内容ごとで、鍵を混ぜると置き場を引けなくなる）。

## R-3: 一覧は「見せる動画」を代表で決め、検索式は集まりの全所在に、範囲は代表の所在に掛ける

- Decision: 一覧・検索・フォルダ・関連・フォルダの索引は「見せる動画」（集まりに属さない動画と、各集まりの
  実効の代表）だけを対象にする（[data-model.md §4](data-model.md)）。実効の代表は
  `representative_key` の動画に見る人に見せてよい所在があればそれ、無ければ見せてよい所在を持つ
  メンバーのうち `videos.id` の最小のもの。`chosenLocationsCTE` は、見せる動画の所在に範囲（登録・公開・
  フォルダ）を掛け、検索式は「同じ集まりのどれかの登録の所在が当たる」に変える。タグ・視聴状態・
  並び順は見せる動画の `UserKey` の値で判定する。`GET /api/videos/{id}` と `versions`・再生・配信は
  代表以外のバージョンも返す。
- Rationale: 要件 5 は、どの一覧にも代表以外を出さず、検索語がどのバージョンに当たっても 1 件が出て、
  フォルダの画面では代表のフォルダにだけ出ることを求める。範囲を代表の所在に、検索式を集まりの所在に
  分けると、この 3 つを 1 つの CTE で満たせる。Edge Case「代表のファイルが消えても集まりは残り、
  残っているバージョンを代わりに見せる」は、実効の代表を読み出しのたびに決めることで、索引にも
  利用者データにも状態を足さずに済む。
- Alternatives considered: 代表以外のバージョンの所在を `video_locations` から外す（所在は走査の事実で、
  スキャンのたびに戻る）。検索式を代表の所在だけに掛ける（要件 5 の「どのバージョンの題名・パスに
  当たっても」に反する）。実効の代表を `video_bundles` に書く（走査で所在が消える・戻るたびに書き直す
  ことになり、利用者データの表を索引の都合で更新する）。

## R-4: フォルダの索引は見せる動画の所在だけで作り、束ねの変更と同じ取引で作り直す

- Decision: `folderIndexLocations` は見せる動画（所有者から見た実効の代表、R-3）の所在だけを返し、
  `domain.BuildFolderIndex` は変えない。`VersionStore` の束ねる・代表を替える・外す操作は、
  `SettingsStore`・`FolderGroupStore` と同じく、同じ取引の中で `rebuildFolderIndex` を呼ぶ。
  `domain.FolderIndexVersion` を 2 に上げ、起動時に作り直す。
- Rationale: 要件 5 は代表以外のバージョンがフォルダのグループに加わらないこと、Edge Case は代表以外
  だけのフォルダが動画の無いフォルダになることを求める。索引の入力から所在を落とせば、グループの割り当て
  （直下の本数、子フォルダの有無）と祖先フォルダ名の両方がそろって変わる。束ねの操作で代表が別の
  フォルダに移れば直下の本数が変わるので、その取引で作り直す（017 §3 の「例外の設定・解除」と同じ扱い）。
- Alternatives considered: 索引はそのままで読み出しのときにメンバーを除く（グループの成立条件
  「直下 2 本以上」が代表以外を数えてしまい、1 本のグループが残る）。スキャンを閉じるときだけ作り直す
  （束ねてから次のスキャンまで、外したバージョンがグループに戻らない）。

## R-5: スキャン時の引き継ぎは `UpsertVideo` で後継を記録し、解析の結果を書く取引で尺を比べて決める

- Decision: `UpsertVideo` が、同じパスの中身が変わって新しい動画の行を作り、前の動画の行が所在を失って
  消えるとき、前の尺が分かっていれば `video_successions (new_key, old_key, old_duration_ms)` に
  記録する（[data-model.md §5](data-model.md)）。前の鍵の動画が同じ走査の中で別のパスに現れたら
  （`newVideo` で作る鍵が `old_key` に一致）、その記録を消す。`ApplyProbe`・`ApplyProbeForJob` は、
  新しい鍵の記録があれば消したうえで、`domain.DurationsMatch(old, new)`（`max(1 秒, 尺の 0.5%)`）で
  尺が合い、かつ `old_key` を参照する動画が無いときだけ引き継ぐ。引き継ぎは、前の鍵が集まりの
  メンバーなら `video_bundle_members` と `representative_key` の鍵を付け替え、そうでなければ
  `playback_progress`・`public_videos` の行を置き換え、`video_tags` を和にする。前の鍵自身の行
  （集まりに入る前の値と却下の記録）も同じく付け替える。
- Rationale: 要件 8 は、尺がほぼ同じときだけ引き継ぎ、尺が大きく違えば別の動画とみなすことを求める。
  新しい中身の尺は解析が終わるまで分からないので、走査の時点では判定できず、記録を残して解析の結果を
  書く取引で決める。前の尺は前の行を消す前にしか読めないので、記録に写す。Edge Case「ファイルの
  入れ替え」は、前の鍵が別のパスの動画として残るときに引き継がない条件で満たす。集まりのメンバーの
  位置を引き継ぐ（Edge Case）のは鍵の付け替えだけで済み、集まりの値には触れない。
- Alternatives considered: 走査の時点で `ffprobe` を呼んで尺を比べる（走査は読むだけで、重い仕事は
  job に積む前提。ARCHITECTURE.md）。前の動画の行を消さずに残す（所在の無い動画が一覧の条件から
  漏れる箇所が増え、内容の参照の判定が変わる）。指紋の一致も条件にする（指紋はスプライトのあと、
  つまり解析よりずっと後にできる。要件 8 は尺だけで判定すると書いている）。

## R-6: 指紋はシーク用スプライトのコマの pHash で、取り込みの段階 `fingerprint` として作る

- Decision: 取り込みの段階 `fingerprint` を `JobKinds` の最後に足す。取り出しの条件は登録の所在と
  シーク用サムネイルの完了（`seek_thumbnail_state = done`）で、シーク用サムネイルの完了を書く取引と
  移行（完成したスプライトの動画）が job を積む。`app.Ingest.Fingerprint` は生成物の置き場から
  スプライトの配置とシートを読み、`internal/media` が各コマを 32×32 の輝度に縮め（縮める前に上下左右の
  黒い帯を落とす）、`internal/domain` が 2 次元 DCT の低周波 8×8（直流を除く）の中央値との大小で
  64 ビットのハッシュを作る。輝度の分散が小さいコマは「単色」の印を付ける。指紋は
  `video_fingerprints (content_key, version, interval_ms, hashes)` に置く（[data-model.md §6](data-model.md)）。
  2 本の比較（`domain.CompareFingerprints`）は、同じ `version`・`interval_ms` で、両方が単色でない
  同じ番号のコマのハミング距離の中央値をとり、比べたコマが 3 未満なら比べない。閾値は中央値 12 以下。
  値は `internal/domain` の定数で `FingerprintVersion` に結び、変えるときは版を上げて移行で作り直す。
- Rationale: 要件 9 の対象（再エンコード・解像度違い・コンテナ違い）は尺と映像の並びが同じで、
  スプライトの間隔は尺から決まるのでコマの時刻がそろう。取り込みはすでに各動画のスプライトを作って
  いるので、ffmpeg を 1 回も余計に起動せず、ネットワークドライブのファイルも読み直さない。pHash は
  明るさ・コントラスト・ぼかしの違いに dHash より強い（先行の検討 §1.2）。単色のコマを外すのは、
  フェードや黒画面がどの動画でも一致してしまうため。閾値を実データで詰める必要は残るので、定数と
  版で持つ。段階にするのは、走査の進捗・残りの仕事・失敗の記録・起床の仕組みをそのまま使うためである。
- Alternatives considered: 専用の抽出ステージ（`ffmpeg` で一定間隔の小さな灰色画像を取る。動画ごとに
  もう 1 回ファイル全体を読む。一部だけ同じ動画は対象外なので、間隔を尺に依らず固定する利点が無い）。
  音声の指紋（同梱の ffmpeg に Chromaprint が無く、対象外）。シーク用サムネイルの job の中で指紋も
  作る（スプライトが完成しているのに指紋だけ作り直す経路が無くなり、閾値の版を上げたときに
  スプライトごと作り直すことになる）。

## R-7: 候補は指紋を書く取引の中で SQLite の関数で求めて表に置き、却下は利用者データに残す

- Decision: `ApplyFingerprintForJob` は指紋を書いた同じ取引で、その内容の候補を作り直す。相手は
  同じ版・同じ間隔の指紋を持ち、尺が `DurationsMatch` の幅にある内容で、却下された組と同じ集まりの
  組を除き、`vv_fingerprint_distance(a, b)`（`vv_shuffle_key` と同じく `modernc.org/sqlite` に登録する
  決定的な関数。`CompareFingerprints` を呼び、比べられなければ -1）が閾値以下の組を
  `video_version_candidates (key_a, key_b, distance)` に入れる（[data-model.md §7](data-model.md)）。
  「違う動画」は `video_version_dismissals (key_a, key_b)` に残し、候補の算出と `Candidates` の
  読み出しの両方で除く。束ねる操作は同じ集まりになった組の候補を消し、内容の参照が無くなるときは
  その鍵の候補・指紋・後継を消す。
- Rationale: 要件 10 は却下した組を二度と候補に出さないことを求め、再スキャンで動画の行が
  作り直されても効くよう内容の鍵に結ぶ利用者データにする。候補を表に置くのは、候補の一覧が指紋の
  全組み合わせを読まずに済み、Edge Case「片方が消えたら候補は消える」を行の削除で満たすため。
  比較を SQL の関数にすると、尺の索引で絞った相手だけを Go の関数で比べる問い合わせが 1 文になり、
  候補の集合と指紋が同じ取引で確定する。
- Alternatives considered: 候補を読み出しのたびに指紋から求める（一覧のたびに尺の近い全組を比べる。
  却下の除外と片方の削除の扱いは同じで、表に置く方が安い）。候補を利用者データにする（指紋から
  作り直せる索引で、却下だけが作り直せない）。

## R-8: 集まりは動画の id で指し、バージョンの一覧・束ねる・代表・外す・候補の経路を足す

- Decision: 画面の API は集まりの id を出さず、動画の id で操作する
  （[contracts/screen-api.md](contracts/screen-api.md)）。`GET /api/videos/{id}/versions`（ゲストも可）が
  集まりの全バージョンを代表を先頭に返し、`POST /api/video-bundles`（`videoIds` と `representativeId`）が
  束ね、`POST /api/videos/{id}/make-representative` と `POST /api/videos/{id}/unbundle` が代表の変更と
  解除、`GET /api/version-candidates` と `POST /api/version-candidates/dismiss` が候補の一覧と却下である。
  候補の「同じ動画」は `POST /api/video-bundles` と同じ経路で、束ねると候補が消える。詳細の `Video` に
  `versions {count, representativeId}` を足し、一覧の項目には入れない。
- Rationale: 画面はどれも動画の id を持っていて（動画ページ、一覧の選択、候補の 2 本）、集まりの id を
  持ち回る画面が無い。`VideoGroupRef` がグループの id を出さないのと同じ考え。候補の判断を束ねる
  経路と分けないのは、判断の結果が束ねることそのものだからで、経路を分けると同じ規則を 2 か所に持つ。
- Alternatives considered: `PUT /api/video-bundles/{bundleId}` の形（集まりの id を応答に出すことになり、
  外したときに集まりが解ける（Edge Case）と id が消える）。候補の判断を 1 つの `decide` 経路にする
  （「同じ」の応答は `VideoVersions`、「違う」は空で、形が揃わない）。

## R-9: 束ねの変化は `domain.VideoBundleChanged` を発行し、画面の `video` の知らせに写す

- Decision: `VersionStore` の操作と引き継ぎ（R-5）は、確定後に `VideoBundleChanged{VideoIDs}`
  （影響した全メンバーの動画の id）を発行し、`cmd/mdm/events.go` の画面の購読が各 id の `video` の
  知らせにする。ワーカーの起床には結ばない。候補の増減は `fingerprint` の job の成否で
  `ProcessingChanged` が `scan` の知らせになるので、候補の画面は `scan` で取り直す。
- Rationale: 一覧と動画ページは `video` の知らせで項目を取り直す仕組みを持っている
  （`useItemRefresh`・`useVideoDetail`）。束ねると一覧の項目が消えたり題名が変わったりするので、全
  メンバーについて流せば既存の取り直しで画面が追いつく。`VideoOverrideChanged` と同じ形である。
- Alternatives considered: 新しい種類の知らせ `versions` を足す（受け取る側を画面に足すことになり、
  `video` で足りる）。候補の変化ごとに知らせを足す（候補の画面だけのために種類を増やす。`scan` で
  取り直せば遅れは job 1 件分）。

## R-10: 外部連携 API は畳まず、値だけが集まりのものになる

- Decision: `api/external-v1.yaml` は変えない。`GET /api/v1/videos` と `lookup` は今までどおり登録の
  所在を持つ全動画を返し、`tags` と `POST /api/v1/video-tags` は `UserKey` で読み書きする（R-2）。
  束ねる・外す操作と MCP のツールは足さない。
- Rationale: 外部連携 API はファイルと内容の鍵で動画を引く道具向けの契約で（026 の契約）、代表以外の
  バージョンを隠すと、そのファイルを持つ道具が `lookup` で引けなくなる。親 Issue の要件は画面の
  一覧についてで、外部連携 API に触れていない。
- Alternatives considered: 外部連携 API でも代表の 1 件に畳む（互換を壊し、要件に無い）。`ExternalVideo` に
  集まりの項目を足す（要件に無く、道具側の使い道が決まっていない。要るときに項目を足す変更は互換）。

## R-11: 集まりの再生位置がそのバージョンの尺以上なら、画面が最初から再生する

- Decision: サーバーは集まりの再生位置をそのまま返し、動画ページ（`web/src/player` の純粋な判断）が
  再開の位置を決めるときに、位置がそのバージョンの `durationMs` 以上なら 0 から再生する。
  `PUT /api/videos/{id}/progress` は再生したバージョンの尺で完了を判定する。
- Rationale: Edge Case「バージョンの長さが違うとき、集まりの再生位置が再生するバージョンの尺を超えて
  いたら最初から再生する」。位置は集まりの事実で、一覧の視聴状態はそれから決まるので、応答で丸めると
  一覧と詳細で値が食い違う。再開の位置の判断は既に画面が持っている。
- Alternatives considered: 応答の `progress` をバージョンの尺で丸める（一覧の「途中まで」の判定と
  ずれる）。
