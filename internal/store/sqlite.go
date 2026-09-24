package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sync"

	"github.com/syudead/vv/internal/domain"

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

// DB は SQLite への接続を保持する土台である。業務の操作は持たず、役割ごとの型
// （roles.go）がこの接続と知らせの発行先を使って行う。
type DB struct {
	sql      *sql.DB
	path     string
	folderMu sync.Mutex

	// publisher は取引が確定した後に、その取引で起きた変化を受け取る。
	publisherMu sync.RWMutex
	publisher   Publisher
}

// Publisher は確定した変化の発行先である。保存層は誰がそれを受け取るかを
// 知らない。
type Publisher interface {
	Publish(events ...domain.Event)
}

// PublishTo は確定した変化の発行先を設定する。nil なら発行しない。
//
// 発行するのは、仕事が積まれたこと（domain.JobsQueued）、段階ごとの残りが
// 変わったこと（domain.ProcessingChanged）、動画の行が消えたこと
// （domain.VideoIngestChanged）と、それで内容の識別子の参照が無くなったこと
// （domain.ContentUnreferenced）である。どれも取引が確定した後にだけ発行し、
// ロールバックした取引からは発行しない。
func (db *DB) PublishTo(publisher Publisher) {
	db.publisherMu.Lock()
	defer db.publisherMu.Unlock()
	db.publisher = publisher
}

// changes は1つの取引の中で起きた変化を集める。確定した後に commit が
// まとめて発行し、確定しなければ捨てる。同じ種類の変化は1つにまとめるので、
// 取引の中で何度起きても、確定後の知らせは重ならない。
type changes struct {
	queued     []domain.JobKind
	processing bool
	deleted    []domain.DeletedVideo
}

// jobsQueued は仕事を積んだ（または取り出せるようにした）段階を記録する。
// 残りの仕事も変わる。
func (c *changes) jobsQueued(kinds ...domain.JobKind) {
	for _, kind := range kinds {
		if !slices.Contains(c.queued, kind) {
			c.queued = append(c.queued, kind)
		}
	}
	c.processing = true
}

// processingChanged は、仕事は積んでいないが、残りの仕事として数える範囲が
// 変わったことを記録する（メディアフォルダの登録を外したなど）。
func (c *changes) processingChanged() {
	c.processing = true
}

// videosDeleted は消した動画の行を記録する。その動画の仕事も残りから消える。
func (c *changes) videosDeleted(deleted []domain.DeletedVideo) {
	if len(deleted) == 0 {
		return
	}
	c.deleted = append(c.deleted, deleted...)
	c.processing = true
}

// events は集めた変化を発行する形にする。
func (c *changes) events() []domain.Event {
	var events []domain.Event
	var keys []string
	for _, video := range c.deleted {
		events = append(events, domain.VideoIngestChanged{VideoID: video.ID})
		if !slices.Contains(keys, video.ContentKey) {
			keys = append(keys, video.ContentKey)
		}
	}
	if len(keys) > 0 {
		events = append(events, domain.ContentUnreferenced{ContentKeys: keys})
	}
	if len(c.queued) > 0 {
		events = append(events, domain.JobsQueued{Kinds: slices.Clone(c.queued)})
	}
	if c.processing {
		events = append(events, domain.ProcessingChanged{})
	}
	return events
}

// commit は取引を確定し、確定できたときだけ集めた変化を発行する。確定前に
// 発行すると、ワーカーがまだ見えない行を探して空振りし、そのまま眠る。
func (db *DB) commit(tx *sql.Tx, c *changes) error {
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publish(c)
	return nil
}

// publish は集めた変化を発行する。取引の外の1文で書き換えた場合は、その文が
// 成功した後に呼ぶ。
func (db *DB) publish(c *changes) {
	events := c.events()
	if len(events) == 0 {
		return
	}
	db.publisherMu.RLock()
	publisher := db.publisher
	db.publisherMu.RUnlock()
	if publisher != nil {
		publisher.Publish(events...)
	}
}

// deleteOrphanVideos は所在が1つも無くなった動画の行を消し、消した動画を返す。
func deleteOrphanVideos(ctx context.Context, tx *sql.Tx) ([]domain.DeletedVideo, error) {
	return collectDeletedVideos(tx.QueryContext(ctx, `delete from videos where not exists (
		select 1 from video_locations where video_locations.video_id = videos.id)
		returning id, content_key`))
}

// collectDeletedVideos は returning id, content_key の結果を読み切る。
func collectDeletedVideos(rows *sql.Rows, err error) ([]domain.DeletedVideo, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var deleted []domain.DeletedVideo
	for rows.Next() {
		var video domain.DeletedVideo
		if err := rows.Scan(&video.ID, &video.ContentKey); err != nil {
			return nil, err
		}
		deleted = append(deleted, video)
	}
	return deleted, rows.Err()
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
