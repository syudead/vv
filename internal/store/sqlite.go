package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	// CGO を必要としない SQLite ドライバ。CGO_ENABLED=0 を維持するために採用した。
	_ "modernc.org/sqlite"
)

// DatabaseFileName はデータベースのファイル名である。置き場所を1箇所に固定すると
// バックアップと削除の手順が単純になるため、設定項目にはしない。
const DatabaseFileName = "mdm.db"

// busyTimeout は書き込みが競合したときに待つ上限（ミリ秒）である。
const busyTimeout = 5000

// DatabasePath はデータディレクトリからデータベースファイルのパスを組み立てる。
func DatabasePath(dataDir string) string {
	return filepath.Join(dataDir, DatabaseFileName)
}

// DB は SQLite への接続を保持する。
type DB struct {
	sql      *sql.DB
	path     string
	folderMu sync.Mutex

	// jobsQueued は仕事を積んだ取引の確定後に呼ぶ。ワーカーはこれを受けて
	// 起きるので、待ち行列を一定間隔で問い合わせない。
	jobsQueuedMu sync.RWMutex
	jobsQueued   func(kinds []JobKind)
}

// OnJobsQueued は、仕事を積んだ取引が確定したときの知らせ先を設定する。
// 積んだ種類が渡る。同じ種類が重なることもある。
func (db *DB) OnJobsQueued(notify func(kinds []JobKind)) {
	db.jobsQueuedMu.Lock()
	defer db.jobsQueuedMu.Unlock()
	db.jobsQueued = notify
}

// notifyJobsQueued は知らせ先があれば呼ぶ。取引の確定後に呼ぶこと。確定前に
// 起こすと、ワーカーがまだ見えない行を探して空振りし、そのまま眠る。
func (db *DB) notifyJobsQueued(kinds ...JobKind) {
	if len(kinds) == 0 {
		return
	}
	db.jobsQueuedMu.RLock()
	notify := db.jobsQueued
	db.jobsQueuedMu.RUnlock()
	if notify != nil {
		notify(kinds)
	}
}

// dsn は接続時に適用する PRAGMA を含む DSN を組み立てる。
// modernc.org/sqlite は _pragma クエリを接続ごとに適用する。
func dsn(path string) string {
	q := url.Values{}
	// 書き込みトランザクションは最初に予約する。既定の deferred では、読み取り後に
	// worker の書き込みが割り込むと write への昇格が SQLITE_BUSY_SNAPSHOT になり、
	// busy_timeout の待機対象にならない。
	q.Set("_txlock", "immediate")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout))
	q.Add("_pragma", "foreign_keys(on)")
	return "file:" + path + "?" + q.Encode()
}

// Open はデータディレクトリ配下のデータベースを開く。ファイルが無ければ作成される。
// 呼び出し側は必ず Close を呼ぶこと。
func Open(dataDir string) (*DB, error) {
	path := DatabasePath(dataDir)

	handle, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("データベースを開けません (%s): %w", path, err)
	}

	db := &DB{sql: handle, path: path}
	if err := db.Ping(context.Background()); err != nil {
		_ = handle.Close()
		return nil, err
	}

	return db, nil
}

// SQL は database/sql のハンドルを返す。マイグレーションと問い合わせで使う。
func (db *DB) SQL() *sql.DB {
	return db.sql
}

// Path はデータベースファイルのパスを返す。記録に出す用途を想定している。
func (db *DB) Path() string {
	return db.path
}

// Ping は保存層への疎通を確認する。稼働確認（/api/health）が ok / degraded を
// 判定するために使う。
func (db *DB) Ping(ctx context.Context) error {
	if err := db.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("データベースへ疎通できません (%s): %w", db.path, err)
	}
	return nil
}

// Close は接続を閉じる。
func (db *DB) Close() error {
	return db.sql.Close()
}
