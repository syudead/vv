package store

import "database/sql"

// IngestStore はジョブの待ち行列と生成結果の索引への反映を受け持つ。
type IngestStore struct{ db *DB }

// LibraryStore はライブラリ索引の読み出しと参照確認を受け持つ。
type LibraryStore struct{ db *DB }

// ScanStore は走査の実行状態を保存する。
type ScanStore struct{ db *DB }

// ScanIndexStore はファイルシステムの走査結果を再構築可能な索引へ反映する。
type ScanIndexStore struct{ db *DB }

// SettingsStore はメディアフォルダなどの設定を保存する。
type SettingsStore struct{ db *DB }

// PlaybackStore は利用者の再生位置を保存する。共有する SQLite 接続だけを持ち、
// ライブラリ索引の型や通知には依存しない。
type PlaybackStore struct{ sql *sql.DB }

func (db *DB) Ingest() *IngestStore       { return &IngestStore{db: db} }
func (db *DB) Library() *LibraryStore     { return &LibraryStore{db: db} }
func (db *DB) Scans() *ScanStore          { return &ScanStore{db: db} }
func (db *DB) ScanIndex() *ScanIndexStore { return &ScanIndexStore{db: db} }
func (db *DB) Settings() *SettingsStore   { return &SettingsStore{db: db} }
func (db *DB) Playback() *PlaybackStore   { return &PlaybackStore{sql: db.sql} }
