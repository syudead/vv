package store

import (
	"context"
	"database/sql"
	"errors"
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
	read     *sql.DB
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
//
// 残りの仕事が変わりうる変化（settle）を記録した取引では、commit が確定の前に
// 直近の取り込みの完了の時刻を決め直す（refreshScanSettled）。
type changes struct {
	queued     []domain.JobKind
	processing bool
	deleted    []domain.DeletedVideo
	// settle は残りの仕事の数が変わりうることを表す。発行する知らせには出ない。
	settle bool
	// issues は、問題の行を直接書かずに問題の一覧が変わりうることを表す（動画の行の
	// 削除による連鎖、メディアフォルダの変更）。発行する知らせには出ない。
	issues bool
}

// issuesChanged は、問題の一覧に出る件が変わりうることを記録する。commit が直近の
// 走査の issues_revision を増やし、画面に一覧を読み直させる。
func (c *changes) issuesChanged() {
	c.issues = true
}

// remainingChanged は、知らせは出さないが、残りの仕事の数が変わりうることを
// 記録する（仕事の専有・成否の記録・走査の終了など）。
func (c *changes) remainingChanged() {
	c.settle = true
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
	c.settle = true
}

// processingChanged は、仕事は積んでいないが、残りの仕事として数える範囲が
// 変わったことを記録する（メディアフォルダの登録を外したなど）。
func (c *changes) processingChanged() {
	c.processing = true
	c.settle = true
}

// videosDeleted は消した動画の行を記録する。その動画の仕事も残りから消える。
func (c *changes) videosDeleted(deleted []domain.DeletedVideo) {
	if len(deleted) == 0 {
		return
	}
	c.deleted = append(c.deleted, deleted...)
	c.processing = true
	c.settle = true
	c.issues = true
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
//
// 残りの仕事が変わりうる変化があれば、確定の前に同じ取引で直近の取り込みの
// 完了の時刻を決め直す。問題の一覧が変わりうる変化があれば、その番号を増やす。
func (db *DB) commit(ctx context.Context, tx *sql.Tx, c *changes) error {
	if err := settle(ctx, tx, c); err != nil {
		return err
	}
	if c != nil && c.issues {
		if err := bumpIssuesRevision(ctx, tx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publish(c)
	return nil
}

// publish は集めた変化を発行する。commit を通らない書き込みから呼ぶときは、
// 残りの仕事が変わる変化を含めないこと（完了の時刻が決め直されない）。
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
func dsn(path string, txLock string) string {
	q := url.Values{}
	// 書き込み用には immediate、一覧のスナップショット用には deferred を渡す。
	// 書き込みを deferred にすると、読み取り後に worker の書き込みが割り込んだ際の
	// write への昇格が SQLITE_BUSY_SNAPSHOT になり、busy_timeout の待機対象にならない。
	q.Set("_txlock", txLock)
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout))
	q.Add("_pragma", "foreign_keys(on)")
	return "file:" + path + "?" + q.Encode()
}

// Open はデータディレクトリ配下のデータベースを開く。ファイルが無ければ作成される。
// 呼び出し側は必ず Close を呼ぶこと。
func Open(dataDir string) (*DB, error) {
	return OpenContext(context.Background(), dataDir)
}

// OpenContext は Open と同じだが、開いた直後の疎通の確認に ctx を使う。
// 要求や操作の context を持つ呼び出し側はこちらを使う。
func OpenContext(ctx context.Context, dataDir string) (*DB, error) {
	path := DatabasePath(dataDir)

	handle, err := sql.Open("sqlite", dsn(path, "immediate"))
	if err != nil {
		return nil, fmt.Errorf("cannot open the database (%s): %w", path, err)
	}

	read, err := sql.Open("sqlite", dsn(path, "deferred"))
	if err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("cannot open the read-only database (%s): %w", path, err)
	}

	db := &DB{sql: handle, read: read, path: path}
	if err := db.Ping(ctx); err != nil {
		_ = handle.Close()
		_ = read.Close()
		return nil, err
	}

	return db, nil
}

// Path はデータベースファイルのパスを返す。記録に出す用途を想定している。
func (db *DB) Path() string {
	return db.path
}

// Ping は保存層への疎通を確認する。稼働確認（/api/health）が ok / degraded を
// 判定するために使う。
func (db *DB) Ping(ctx context.Context) error {
	if err := db.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("cannot reach the database (%s): %w", db.path, err)
	}
	if err := db.read.PingContext(ctx); err != nil {
		return fmt.Errorf("cannot reach the read-only database (%s): %w", db.path, err)
	}
	return nil
}

// Close は接続を閉じる。
func (db *DB) Close() error {
	return errors.Join(db.read.Close(), db.sql.Close())
}
