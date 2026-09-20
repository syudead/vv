# Research: 設定画面

共通の技術選定は [ARCHITECTURE.md](../../ARCHITECTURE.md) と
[技術選定文書](../../docs/design-docs/tech-stack-selection.md)を引き継ぐ。

## R-701: SQLiteだけを設定の正本にする

**Decision**: メディアフォルダは初回0件でSQLiteに保持する。`MDM_MEDIA_DIR` は移行元や初期値としても
使わない。

**Rationale**: UI設定と環境変数という2つの正本を残さないため。

## R-702: MediaFolderを個別リソースとして保存する

**Decision**: 各MediaFolderにID、path、行単位versionを持たせ、追加・変更・削除をそれぞれ1件の
atomic operationにする。一覧全体のPUT、draft、global version、並び替えは実装しない。同じ
正規化pathと、祖先・子孫で探索範囲が重なるpathは拒否する。

**Rationale**: 1件の操作に無関係なfolderを更新対象にせず、競合と失敗の範囲を対象行へ限定するため。

## R-703: DB無効化を既存folderの変更・削除だけに限定する

**Decision**:

- 新規追加はMediaFolderをinsertするだけで、既存ライブラリDBを変更しない
- 既存folderのpath変更は、そのfolder由来のDBデータ削除とpath更新を1transactionで行う
- 削除は、そのfolder由来のDBデータ削除とfolder行削除を1transactionで行う
- 全folderの再生位置・視聴済み状態、他folderのlocationが残るvideo・job・thumbnail、完了済みscan履歴は
  変更しない

`videos`をcontent単位、`video_locations`を実在path単位に分ける。同じcontentが複数rootにある場合は
1 videoと複数locationsとして保持する。folder変更・削除では旧root配下のlocationsだけを消し、
locationが0件になったvideoだけをjobsとともに削除する。`playback_progress`は同じcontentが再発見された
ときに戻す再構築不能な利用者データなので削除しない。content key名のthumbnail fileはfolder操作で
削除せず、別locationや直後の再登録が生成した同名fileとの競合をなくす。orphan cacheのGCは別機能とする。

migrationは既存videoのpath、title、size、mtimeを1件のlocationへ移し、video ID、content key、
probe結果、jobs、playback progressを保持する。新規folder追加では既存video/locationへ書き込まない。

**Rationale**: 追加済みfolderのデータは新規rootを増やしても有効であり、全削除する理由がない。

**Rejected**: videoの単一pathだけでfolder所属を判定する案。同じcontentが別rootにも存在する事実を
表現できず、片方のfolder削除で未変更root側のvideoまで失うため。

## R-704: サーバー側directory browserを提供する

**Decision**: APIが返すserver filesystemのdirectoryを辿るfolder pickerを実装する。path省略時は
Linux rootまたはWindows driveをnavigation起点として返し、以後は親と直下の実directoryを返す。
filesystem rootとdrive rootは登録不可とする。`Lstat`でsymlinkを候補から除き、scannerもリンクを
辿らない。

## R-705: 選択時と操作実行時の両方で検証する

**Decision**: listing時に読取可能性を確認し、POST/PUT時にも`Lstat`で対象pathの存在・実directory・
readable・非filesystem-rootと、全既存folderに対する重複・包含を再検証する。API pathはOSの
絶対・正規化済み表現とする。

## R-706: 非同期走査を失敗境界で閉じる

**Decision**:

- scan開始時にfolder一覧をsnapshotする
- rootを1件ずつ処理し、1rootの失敗で残りrootを止めない
- directory/file単位の消失・permission・stat/open失敗をfailed countとlogへ記録して継続する
- root列挙失敗時はroot全体、subtree失敗時はそのprefix、file失敗時はそのpathの既存索引を保持する
- missing削除は完全に列挙できた範囲だけに適用し、scan全体のfatal error時は実行しない
- DB更新など継続不能なerrorはscan全体をfailedにする
- goroutine最上位でdeferによるfinalizeとpanic recoveryを行う
- panic/error/cancelの全出口でrunning scanをdoneまたはfailedへ確定し、processへpanicを伝播させない

**Rationale**: 稀なfilesystem競合や予期しないpanicでserver processと次回scanを利用不能にしないため。

## R-707: 行単位versionで同時変更を検出する

**Decision**: PUTとDELETEは対象MediaFolderのexpected versionを要求し、同じ行への古い操作を409にする。
POSTはpathのunique・overlap検証で競合を処理する。

## R-708: 非同期jobの古い結果をidentity条件で拒否する

**Decision**: workerは処理開始時の`video_id`、`content_key`、`location_id`、`location_version`、`path`を
保持し、probe・thumbnailの全write-back前にvideoとlocationの両identityをtransaction内で再確認する。
locationが消失・変更済みなら結果を破棄し、別のcurrent locationへ再queueする。file access failureは
location固有として論理videoをfailedにせず、実際に読めたcurrent locationでmedia解析が失敗した場合だけ
content単位のfailureを書き戻す。

**Rationale**: SQLiteのID再利用や同じIDの内容差し替えだけでなく、同じvideoの処理元locationが
削除された場合にも、古いI/O失敗で残存videoをfailedにしないため。

**Rejected**: video IDとcontent keyだけを検証する案。削除されたlocationと残存locationは同じ
video identityを共有するため、location固有の失敗を拒否できない。

## R-709: directory APIは既存のtrusted-network境界を引き継ぐ

**Decision**: 認証は既存roadmapどおり別featureとし、本機能だけの独自認証や新しいpath allowlist
設定は追加しない。認証導入までは家庭内の信頼できるnetworkだけで運用し、CORSを許可せず、
全mutationをsame-originに限定し、POST/PUTは`application/json`だけを受理する。public exposureは
非対応と明記する。

**Rationale**: directory APIは既存アプリよりfilesystem情報を多く扱うが、認証方式をこのfeature内で
部分実装するとアプリ全体のaccess boundaryが分裂する。既存の明示的な脅威モデルを維持しつつ、
browser由来のcross-origin操作は拒否する。

## R-710: thumbnail cache削除をfolder操作から分離する

**Decision**: content key名のthumbnail fileはfolder変更・削除で消さない。orphan cacheの回収は、
削除直前の参照確認とfile generationを扱う専用GCを導入する別featureまで行わない。

**Rationale**: cache fileが残ってもDBの可視性や再生状態は変わらず、同じcontentの再登録時に再利用できる。

**Rejected**: transaction commit後のbest-effort cleanup。同じcontentを直後に再登録して同名thumbnailを
生成した場合、遅延cleanupが新しいfileまで削除できるため。
