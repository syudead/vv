package store

import "database/sql"

// 役割ごとの型。業務の操作は、それを受け持つ型のメソッドとして置き、*DB は
// 接続・マイグレーション・疎通確認・知らせの発行先だけを持つ土台にとどめる。
//
// 役割の型は、別の役割の型の公開メソッドを呼ばない。複数の役割が同じデータを
// 読む操作（listMediaFolders・getVideo・contentKeyReferenced）と、役割をまたぐ
// 1取引の中で使う SQL の断片（removeLocationsUnder・refreshSearchKeysUnder・
// rebuildFolderIndex など）は、パッケージ内の非公開の関数として共有する。

// IngestStore はジョブの待ち行列と、取り込みの段階の結果の索引への反映を
// 受け持つ（jobs.go・ingest_results.go）。
type IngestStore struct{ db *DB }

// LibraryStore はライブラリ索引の読み出し（一覧・ライブラリの項目・検索・フォルダ
// 閲覧・関連動画・所在）と、照合用の鍵の作り直しを受け持つ（listing.go・
// library_items.go・folders.go・related.go・search_keys.go）。
type LibraryStore struct{ db *DB }

// ScanStore は走査の実行状態を保存する（scans.go）。
type ScanStore struct{ db *DB }

// ScanIndexStore はファイルシステムの走査結果を再構築可能な索引へ反映する
// （scan_index.go）。スキャンを閉じる直前と起動時のフォルダの索引の作り直しも
// 受け持つ（folder_groups.go）。
type ScanIndexStore struct{ db *DB }

// SettingsStore はメディアフォルダの登録を保存する（media_folders.go）。
type SettingsStore struct{ db *DB }

// FolderGroupStore はフォルダごとの例外（まとめを解除・直下をまとめる）の設定と
// 解除を、フォルダの索引の作り直しと同じ取引で保存し、フォルダのまとめ方を読み出す
// （folder_groups.go、docs/design-docs/catalog-organization.md）。
type FolderGroupStore struct{ db *DB }

// PlaybackStore は利用者の再生位置を保存する（progress.go）。共有する SQLite
// 接続を持ち、ライブラリ索引の型には依存しない。視聴状態が変わる保存の確定後に
// domain.SmartFoldersChanged を発行するため、発行先を持つ *DB も持つ。
type PlaybackStore struct {
	sql *sql.DB
	db  *DB
}

// TagStore はタグそのもの（作成・改名・削除・統合・シノニムの登録と解除・
// 本数つきの一覧）、動画への付与・取り外し・選んだ動画のタグの要約
// （AttachTagByID・AttachTagByName・DetachTag・Summary）、利用者データの鍵の集合から
// 項目のタグをまとめて引く操作（TagsByContentKeys）、タグ名の照合用の鍵の
// 作り直しを保存する（tags.go）。PlaybackStore と同じく、共有する SQLite
// 接続を持ち、ライブラリ索引の型には依存せず、タグの書き込みの確定後に
// domain.SmartFoldersChanged を発行するため *DB も持つ。ページの読み出しは件数と行を
// 同じスナップショットから返すため、書き込みの枠を取らない読み取り用の接続 read
// （deferred）で取引を開く。動画のタグを変える操作は所有者の記録を同じ取引で書き
// （owner_activity.go）、確定後に domain.ActivityChanged も発行する。
type TagStore struct {
	sql, read *sql.DB
	db        *DB
}

// AuthStore は唯一のアカウントとログインセッションと API トークンを保存する（auth.go・
// api_tokens.go）。初回設定、資格情報の書き換え、セッションの追加・有効性の確認・削除・
// 期限切れの掃除、API トークンの発行・一覧・失効・有効性の確認・最終使用日時の記録を持つ。
// PlaybackStore と同じく、共有する SQLite 接続だけを持ち、ライブラリ索引の型や
// 通知には依存しない。
type AuthStore struct{ sql *sql.DB }

// VisibilityStore は動画の公開フラグの切り替えを保存する（visibility.go、
// docs/design-docs/authentication.md）。公開フラグを読んで見せる動画を
// 絞るのは LibraryStore の読み出しである。TagStore と同じく、共有する SQLite
// 接続を持ち、ライブラリ索引の型には依存せず、確定後に domain.SmartFoldersChanged を
// 発行するため *DB も持つ。
type VisibilityStore struct {
	sql *sql.DB
	db  *DB
}

// FavoriteStore は動画とグループのお気に入りの付け外しを保存する（favorites.go、
// docs/design-docs/catalog-organization.md）。お気に入りを読んで Video.Favorite・LibraryGroup.Favorite
// に写すのは各役割の読み出しである。VisibilityStore と同じく、共有する SQLite 接続を持ち、
// ライブラリ索引の型には依存せず、確定後に domain.SmartFoldersChanged だけを発行する。
type FavoriteStore struct {
	sql *sql.DB
	db  *DB
}

// PlaylistStore はプレイリストとその項目の作成・改名・削除・追加・並べ替え・取り外しと読み出しを
// 受け持つ（playlists.go、docs/design-docs/catalog-organization.md「Store operations」）。FavoriteStore と
// 同じく、共有する SQLite 接続だけを持ち、ライブラリ索引の型や通知には依存せず、ドメインイベントを
// 発行しない。束ねと引き継ぎによる項目の付け替えは、それぞれの取引の中で rekeyPlaylistItems が行う。
// プレイリストの読み出しは要約と項目を同じスナップショットから返すため、TagStore と同じく
// 書き込みの枠を取らない読み取り用の接続 read（deferred）で取引を開く。
type PlaylistStore struct{ sql, read *sql.DB }

// AutoTagStore は自動タグ付けの待ち行列（積む・専有する・終える・失敗にする・数える）と、
// 判定の結果のタグの付与を受け持つ（auto_tags.go、docs/design-docs/auto-tagging.md）。
// 付与は TagStore と同じ非公開の関数で行い、確定後に domain.AutoTagApplied を発行するため、
// 発行先を持つ *DB を通して使う。設定の保存は SettingsStore が持つ。
type AutoTagStore struct{ db *DB }

// OverrideStore は動画の上書き（表示名）を、その内容の動画の全所在の照合用の鍵の
// 書き直しと同じ取引で保存する（overrides.go、docs/design-docs/catalog-organization.md）。
// 動画を返す読み出しがそれを Video.Title などへ写すのは、各役割の読み出しである。
// VisibilityStore と同じく SQL 接続を使うが、確定後に domain.VideoOverrideChanged を
// 発行するため、発行先を持つ *DB を通して使う。ライブラリ索引の型には依存しない。
// 作品情報プラグインの取得の結果の反映（work_info_fetch.go）も受け持ち、表示名と仮のタグを
// 1 つの取引で書く。
type OverrideStore struct{ db *DB }

// VersionStore は同じ動画の別バージョンの集まりを束ねる・代表を替える・外す操作と、
// 集まりのバージョンの読み出しを受け持つ（versions.go、docs/design-docs/catalog-organization.md）。
// 集まりは利用者データで、集まりの値は利用者データの鍵（user_keys.go の userKeyExpr）で
// 既存の表に置く。確定後に domain.VideoBundleChanged を発行するため、発行先を持つ *DB を
// 通して使う。
type VersionStore struct{ db *DB }

// GroupStore は任意のグループの作成・改名・追加・取り外し・並べ替え・解散・タグ化を受け持つ
// （group_edits.go、docs/design-docs/catalog-organization.md「Edits」）。確定後に domain.GroupsChanged を
// 発行するため、発行先を持つ *DB を通して使う。自動グループをフォルダとそろえる同期は、索引を
// 作り直す各取引の中で syncFolderGroups が行う（groups.go）。
type GroupStore struct{ db *DB }

// SmartFolderStore はスマートフォルダの作成・変更（移動を含む）・削除と、その行と欠けた参照の
// 読み出しを受け持つ（smart_folders.go、docs/design-docs/catalog-organization.md「Edits」）。
// 確定後に domain.SmartFoldersChanged を発行するため、発行先を持つ *DB を通して使う。
type SmartFolderStore struct{ db *DB }

// OrganizeStore はドラッグの操作（並べ替えなど）と、その取り消しの像と復元を受け持つ（organize.go・
// organize_image.go、docs/design-docs/catalog-organization.md「Operations」「Undo image」）。操作は
// グループと集まりの表にまたがるので、GroupStore・VersionStore の公開メソッドを呼ばず、パッケージ
// 内の関数を共有する。像は organize_image.go だけがバイト列にし、読み戻す。確定後に
// domain.GroupsChanged を発行するため、発行先を持つ *DB を通して使う。
type OrganizeStore struct{ db *DB }

// CreditStore は人物と団体（名前・別名・外部 ID・団体の親）の一覧・読み出し・作成・更新・統合・削除と、
// その名前の照合用の鍵の作り直しを受け持つ（credits.go・credit_listing.go、
// docs/design-docs/catalog-organization.md「Entities」）。2 つの種類を domain.CreditKind で受け、
// SQL は 1 回だけ書く。TagStore と同じく、共有する SQLite 接続と読み取り用の接続 read を持ち、
// ライブラリ索引の型には依存しない。発行するのは改名・削除・統合の確定後の
// domain.SmartFoldersChanged だけである（R-11、docs/design-docs/catalog-organization.md「Notices」）。
type CreditStore struct {
	sql, read *sql.DB
	db        *DB
}

// WorkInfoStore は動画の作品情報（品番・説明・日付・人物・団体・URL・外部 ID）の書き込みと、利用者データの
// 鍵の集合からの読み出しを受け持つ（work_info.go、docs/design-docs/catalog-organization.md「Writes
// (WorkInfoStore)」）。全画面の編集画面の保存（SetDetails）は表示名も同じ取引で、OverrideStore を呼ばずに
// パッケージ内の関数で書く（R-14）。確定後に domain.VideoWorkInfoChanged（と domain.VideoOverrideChanged）を
// 発行するため、OverrideStore と同じく発行先を持つ *DB を通して使う。
type WorkInfoStore struct{ db *DB }

// WorkInfoPluginStore は作品情報プラグインの行（追加・名前による差し替え・承認・取り消し・削除）と、
// プラグインごとの設定の値を受け持つ（work_info_plugins.go、docs/design-docs/work-info-plugins.md）。
// 承認と値の書き込みの判断は internal/app が行い、ここは取引の中で行を読み直して同じ規則を確かめる。
// 共有する SQLite 接続と、一覧のための読み取り用の接続 read だけを持ち、ライブラリ索引の型や通知には
// 依存せず、ドメインイベントを発行しない。
type WorkInfoPluginStore struct{ sql, read *sql.DB }

// ActivityStore はアクティビティの記録の一覧と件数・1 件の読み出し・起動時の回復を受け持つ（activity.go、
// docs/design-docs/activity.md）。記録の書き込みはこの型のメソッドではなく、記録する各役割が自分の取引から
// 呼ぶパッケージ内の関数（recordActivity・setActivityState）である。確定後に domain.ActivityChanged を発行する
// ため、発行先を持つ *DB を通して使う。
type ActivityStore struct{ db *DB }

func (db *DB) Ingest() *IngestStore       { return &IngestStore{db: db} }
func (db *DB) Library() *LibraryStore     { return &LibraryStore{db: db} }
func (db *DB) Scans() *ScanStore          { return &ScanStore{db: db} }
func (db *DB) ScanIndex() *ScanIndexStore { return &ScanIndexStore{db: db} }
func (db *DB) Settings() *SettingsStore   { return &SettingsStore{db: db} }
func (db *DB) Playback() *PlaybackStore   { return &PlaybackStore{sql: db.sql, db: db} }
func (db *DB) Tags() *TagStore            { return &TagStore{sql: db.sql, read: db.read, db: db} }
func (db *DB) Auth() *AuthStore           { return &AuthStore{sql: db.sql} }
func (db *DB) Visibility() *VisibilityStore {
	return &VisibilityStore{sql: db.sql, db: db}
}
func (db *DB) Favorites() *FavoriteStore       { return &FavoriteStore{sql: db.sql, db: db} }
func (db *DB) Playlists() *PlaylistStore       { return &PlaylistStore{sql: db.sql, read: db.read} }
func (db *DB) FolderGroups() *FolderGroupStore { return &FolderGroupStore{db: db} }
func (db *DB) Overrides() *OverrideStore       { return &OverrideStore{db: db} }
func (db *DB) AutoTags() *AutoTagStore         { return &AutoTagStore{db: db} }
func (db *DB) Versions() *VersionStore         { return &VersionStore{db: db} }
func (db *DB) Groups() *GroupStore             { return &GroupStore{db: db} }
func (db *DB) Organize() *OrganizeStore        { return &OrganizeStore{db: db} }
func (db *DB) SmartFolders() *SmartFolderStore { return &SmartFolderStore{db: db} }
func (db *DB) Credits() *CreditStore           { return &CreditStore{sql: db.sql, read: db.read, db: db} }
func (db *DB) WorkInfo() *WorkInfoStore        { return &WorkInfoStore{db: db} }
func (db *DB) WorkInfoPlugins() *WorkInfoPluginStore {
	return &WorkInfoPluginStore{sql: db.sql, read: db.read}
}
func (db *DB) Activity() *ActivityStore { return &ActivityStore{db: db} }
