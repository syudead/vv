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

// LibraryStore はライブラリ索引の読み出し（一覧・検索・フォルダ閲覧・関連動画・
// 所在）と、照合用の鍵の作り直しを受け持つ（listing.go・folders.go・related.go・
// search_keys.go）。
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
// 解除を、フォルダの索引の作り直しと同じ取引で保存する（folder_groups.go、
// specs/017-folder-groups/data-model.md §1〜§3）。
type FolderGroupStore struct{ db *DB }

// PlaybackStore は利用者の再生位置を保存する（progress.go）。共有する SQLite
// 接続だけを持ち、ライブラリ索引の型や通知には依存しない。
type PlaybackStore struct{ sql *sql.DB }

// TagStore はタグそのもの（作成・改名・削除・統合・シノニムの登録と解除・
// 本数つきの一覧）、動画への付与・取り外し・選んだ動画のタグの要約
// （AttachTagByID・AttachTagByName・DetachTag・Summary）、content_key の集合から
// 項目のタグをまとめて引く操作（TagsByContentKeys）、タグ名の照合用の鍵の
// 作り直しを保存する（tags.go）。PlaybackStore と同じく、共有する SQLite
// 接続だけを持ち、ライブラリ索引の型や通知には依存しない。
type TagStore struct{ sql *sql.DB }

// AuthStore は唯一のアカウントとログインセッションを保存する（auth.go）。初回設定、
// 資格情報の書き換え、セッションの追加・有効性の確認・削除・期限切れの掃除を持つ。
// PlaybackStore と同じく、共有する SQLite 接続だけを持ち、ライブラリ索引の型や
// 通知には依存しない。
type AuthStore struct{ sql *sql.DB }

// VisibilityStore は動画の公開フラグの切り替えを保存する（visibility.go、
// specs/016-single-account-auth/data-model.md §5）。公開フラグを読んで見せる動画を
// 絞るのは LibraryStore の読み出しである。TagStore と同じく、共有する SQLite
// 接続だけを持ち、ライブラリ索引の型や通知には依存しない。
type VisibilityStore struct{ sql *sql.DB }

func (db *DB) Ingest() *IngestStore       { return &IngestStore{db: db} }
func (db *DB) Library() *LibraryStore     { return &LibraryStore{db: db} }
func (db *DB) Scans() *ScanStore          { return &ScanStore{db: db} }
func (db *DB) ScanIndex() *ScanIndexStore { return &ScanIndexStore{db: db} }
func (db *DB) Settings() *SettingsStore   { return &SettingsStore{db: db} }
func (db *DB) Playback() *PlaybackStore   { return &PlaybackStore{sql: db.sql} }
func (db *DB) Tags() *TagStore            { return &TagStore{sql: db.sql} }
func (db *DB) Auth() *AuthStore           { return &AuthStore{sql: db.sql} }
func (db *DB) Visibility() *VisibilityStore {
	return &VisibilityStore{sql: db.sql}
}
func (db *DB) FolderGroups() *FolderGroupStore { return &FolderGroupStore{db: db} }
