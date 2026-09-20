# Data Model: 設定画面

## MediaFolder

各行を独立して追加・変更・削除する。folder一覧全体のdraftやrevisionは持たない。

| Field        | Rule                                    |
| ------------ | --------------------------------------- |
| `id`         | primary key                             |
| `path`       | lexical clean済みserver絶対path、unique。Unicode表現は実在entryの綴りを保持 |
| `version`    | 1から始まり、その行のpath変更ごとに加算 |
| `created_at` | serverが設定                            |
| `updated_at` | serverが設定                            |

- 初回は0行。
- 同一pathと祖先・子孫関係のpathを同時に持たない。
- 表示順は`id`の昇順とし、並び替え設定は持たない。
- 1件以上ある場合だけscanを開始できる。

## Video and VideoLocation

`videos`は内容単位、`video_locations`はfilesystem上の実在path単位に分ける。同じ`content_key`の
ファイルが複数rootにあっても1つのvideoと複数locationとして保持する。

### videos

`content_key`、probe結果、再生可否、thumbnail stateなどpathに依存しない情報を持つ。既存の
`content_key` uniqueとvideo IDを維持する。path、title、size、mtimeはlocationへ移す。

### video_locations

| Field        | Rule                                      |
| ------------ | ----------------------------------------- |
| `id`         | autoincrement primary key、再利用しない   |
| `video_id`   | videosへのforeign key、video削除時cascade |
| `path`       | lexical clean済み絶対path、unique。Unicode表現は実在entryの綴りを保持 |
| `version`    | 1から始まり、内容・所属変更ごとに加算     |
| `title`      | pathのbasenameから導出                    |
| `size_bytes` | locationの走査時点の値                    |
| `mtime`      | locationの走査時点の値                    |
| `created_at` | serverが設定                              |
| `updated_at` | serverが設定                              |

locationのpathがMediaFolder path配下にあるとき、そのfolder由来と判定する。単純な文字列prefixではなく、
volume・separator・case sensitivityをOS規則に従って扱う共通helperを使う。

APIで1つだけpath/titleを示す場合は、現在登録済みroot内にあるlocationをpath昇順で選ぶ。streamは
同じ順序で各locationを安全検証し、最初に利用可能な実fileを配信する。1locationが消失しても別locationが
あれば同じvideoを表示・配信できる。

`videos_fts`はlocation単位のtitle/pathを索引し、検索結果を`video_id`で重複排除する。locationの
insert/update/deleteでFTSを同期する。

migrationは各既存videoのpath、title、size、mtimeを1件のVideoLocationへ移し、video ID、
content key、probe結果、job、playback progressを保持する。既存library dataを削除しない。
location IDは再利用しない。scannerが同じpathのcontent、size、mtime、またはvideo所属を変える場合は
location versionを加算する。
旧schemaへのDown migrationは1 videoにつき1 locationしか表現できないため、複数locationを持つvideoが
存在する場合はデータを縮約せずrollbackを拒否する。

### jobs location binding

既存jobにnullableな`location_id`、`location_version`、`location_path`を追加する。queue時点では未選択でも
よく、workerがclaimしてcurrent locationを選ぶtransactionで3値を記録する。retryで別locationを選ぶときは
3値を同じtransactionで置き換える。migration前からpending/runningだったjobは未選択へ戻し、次のclaimで
current locationを選び直す。
登録済みMediaFolderに属するcurrent locationがないjobはclaimせずqueuedで保持し、folder登録後に再開する。

## Atomic Folder Operations

### Add

1. running scanがないことを確認する
2. candidateのabsolute、exists、directory、readable、非symlink、非filesystem-root、重複・包含を検証する
3. MediaFolderを1行insertする
4. commitする

追加transactionは`videos`、`videos_fts`、`jobs`、`scans`、`playback_progress`へ書き込まない。

### Replace Existing Path

1. `id`とexpected `version`、running scanなしを確認する
2. candidateを検証する
3. 旧root配下にあるVideoLocationと影響を受けるvideo IDを確保する
4. 対象folderのpathとversionを更新する
5. 手順3のlocationsを削除し、location FTSを同期する
6. surviving videoのqueued jobが削除locationへbinding済みなら、bindingだけを未選択へ戻す
7. locationが0件になったvideosだけを削除し、そのjobsをcascadeさせる
8. commitする

### Delete

Replaceと同じ対象特定・従属データ削除を行い、MediaFolder行を削除してcommitする。

いずれも途中失敗時はfolderとライブラリDBの双方を変更しない。別rootのlocationが残るvideoと
そのjob state、thumbnail state、全`playback_progress`、全scan履歴を維持する。削除locationへbinding済みの
queued jobは別locationを選べるようbindingだけをclearする。content key名のthumbnail fileはfolder操作で
削除しない。参照確認とfile削除の競合を防げる専用GCを将来導入するまで、orphan cacheを残す。

## In-flight Job Write Protection

job workerはclaim時に処理対象の`video_id`、`content_key`、`location_id`、`location_version`、`path`を
記録し、そのpathだけを外部processへ渡す。probe結果、probe失敗、thumbnail stateの全write-back前に、
video identityに加えて同じID・version・pathのlocationが現在も対象videoに属することをtransaction内で
確認する。一致しない結果はstale completionとして破棄する。別のcurrent locationがあればjobをそこへ
再queueし、なければ削除済みjobの完了として終了する。

file消失、permission、open/stat失敗はlocation固有の失敗として扱い、論理videoのprobe/thumbnail stateを
failedへ変更しない。workerはjob開始時にsnapshotした未試行のcurrent locationsをpath順で試す。
有効なlocationを実際に読めたうえでmedia解析自体が失敗した場合だけ、identity再確認後にcontent単位の
failed stateを書き戻せる。job完了・失敗の記録も同じidentity条件を使う。
claim後にcurrent locationが追加された場合も、probe結果・probe失敗・thumbnail stateを書き戻さずjobを
再queueする。変更のないfileを再走査した場合、欠落jobを復旧するのはstateが`pending`のときだけとし、
retry上限へ達した`failed` jobはcontentが変わるまで復活させない。
location固有のI/O失敗でjobだけが`failed`、論理stateが`pending`の組合せでは、そのjobを再試行抑止記録として
保持期間後も残す。content変更時の通常enqueueが抑止記録を置き換え、再解析を開始する。

APIが返す代表locationのpathが変わる可能性がある操作では、同じtransaction内で`videos.container`と
probe済みvideoの`playable`・`unplayable_reason`を新しい代表pathに合わせて再計算する。対象はlocationの
追加・削除・別videoへの再割り当てと、MediaFolderの変更・削除による登録範囲の変更を含む。

## DirectoryListing

| Field         | Meaning                                                  |
| ------------- | -------------------------------------------------------- |
| `currentPath` | 現在位置。filesystem root一覧ではnull                    |
| `parentPath`  | 親。rootまたはroot一覧ではnull                           |
| `directories` | 直下の読取可能な実directory。ファイルとsymlinkを含まない |

## Scan Root Snapshot

scan開始時にMediaFolderの`id`、`path`、`version`を`id`順で複製する実行時値。走査中はfolder操作を
拒否するため、1回のscanは同じroot集合を最後まで使う。scannerはsymlinkを辿らず、全rootを通じて
seen location pathsと列挙不能prefixを管理する。

- 新しいpathはcontent keyに対応するvideoを作成または再利用し、locationをupsertする
- 同じcontentが別pathにあっても既存locationを移動せず、追加locationとして保持する
- rootを最後まで列挙できた場合だけ、そのroot内のmissing locationを削除候補にする
- root自体の列挙に失敗した場合は、そのroot配下の既存locationをすべて保持する
- subtreeの列挙に失敗した場合は、そのprefix配下の既存locationを保持する
- fileのstat/openに失敗した場合は、そのpathの既存locationを保持する
- missing locations削除後、locationが0件になったvideoだけを削除する
- DB/reporting errorやpanicでscan全体がfailedになった場合は、missing削除を実行しない

これにより、一時的なmount消失やpermission errorを「ファイルが削除された」と誤認しない。
