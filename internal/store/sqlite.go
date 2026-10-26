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

	// The pure-Go driver keeps CGO_ENABLED=0 builds supported.
	_ "modernc.org/sqlite"
)

// DatabaseFileName is fixed to keep backup and removal procedures simple.
const DatabaseFileName = "mdm.db"

const busyTimeout = 5000

func DatabasePath(dataDir string) string {
	return filepath.Join(dataDir, DatabaseFileName)
}

// DB owns connections and event publication; role types own business operations.
type DB struct {
	sql      *sql.DB
	read     *sql.DB
	path     string
	folderMu sync.Mutex

	revisionMu            sync.Mutex
	lastThumbnailRevision int64

	publisherMu sync.RWMutex
	publisher   Publisher
}

type Publisher interface {
	Publish(events ...domain.Event)
}

// PublishTo sets an optional publisher; only committed transactions emit events.
func (db *DB) PublishTo(publisher Publisher) {
	db.publisherMu.Lock()
	defer db.publisherMu.Unlock()
	db.publisher = publisher
}

// Coalesce transaction changes and publish after commit; recalculate import completion before committing relevant job changes.
type changes struct {
	queued     []domain.JobKind
	processing bool
	deleted    []domain.DeletedVideo
	// Internal marker that remaining work may have changed; not an event.
	settle bool
	// Issue presentation may change through cascades or registration changes without direct issue writes.
	issues bool
	groups []int64
	// Read all affected folder IDs before commit and notify afterward.
	matches      bool
	smartFolders []int64
	activity     bool
}

func (c *changes) activityChanged() {
	c.activity = true
}

func (c *changes) matchesChanged() {
	c.matches = true
}

func (c *changes) groupsChanged(ids []int64) {
	c.groups = normalizeIDs(append(c.groups, ids...))
	if len(ids) > 0 {
		c.matchesChanged()
	}
}

// Advance issue revision so the UI refreshes even when issue counts remain unchanged.
func (c *changes) issuesChanged() {
	c.issues = true
}

// Mark remaining-work changes without publishing an event.
func (c *changes) remainingChanged() {
	c.settle = true
}

func (c *changes) jobsQueued(kinds ...domain.JobKind) {
	for _, kind := range kinds {
		if !slices.Contains(c.queued, kind) {
			c.queued = append(c.queued, kind)
		}
	}
	c.processing = true
	c.settle = true
}

// Remaining-work eligibility can change without enqueueing jobs, such as after unregistering a folder.
func (c *changes) processingChanged() {
	c.processing = true
	c.settle = true
}

func (c *changes) videosDeleted(deleted []domain.DeletedVideo) {
	if len(deleted) == 0 {
		return
	}
	c.deleted = append(c.deleted, deleted...)
	c.processing = true
	c.settle = true
	c.issues = true
	c.matchesChanged()
}

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
	if c.activity {
		events = append(events, domain.ActivityChanged{})
	}
	events = append(events, groupsChangedEvents(c.groups)...)
	return append(events, smartFoldersChangedEvents(c.smartFolders)...)
}

// Publish only after commit so workers can see queued rows; refresh completion and issue revision in the same transaction.
func (db *DB) commit(ctx context.Context, tx *sql.Tx, c *changes) error {
	if err := settle(ctx, tx, c); err != nil {
		return err
	}
	if err := closeDeletedPairActivities(ctx, tx, c); err != nil {
		return err
	}
	if c != nil {
		if err := closeDeletedVideoImports(ctx, tx, c, c.deleted); err != nil {
			return err
		}
	}
	if c != nil && c.issues {
		if err := bumpIssuesRevision(ctx, tx); err != nil {
			return err
		}
	}
	if c != nil && c.matches {
		ids, err := allSmartFolderIDs(ctx, tx)
		if err != nil {
			return err
		}
		c.smartFolders = ids
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	db.publish(c)
	return nil
}

// Commit collected changes and extra role events together, adding smart-folder notices for match-changing events.
func (db *DB) commitWith(ctx context.Context, tx *sql.Tx, c *changes, events ...domain.Event) error {
	if changesMatches(events) {
		c.matchesChanged()
	}
	if err := db.commit(ctx, tx, c); err != nil {
		return err
	}
	db.publishEvents(events...)
	return nil
}

// Direct publication must not include remaining-work changes, which require commit-time completion recalculation.
func (db *DB) publish(c *changes) {
	db.publishEvents(c.events()...)
}

func (db *DB) publishEvents(events ...domain.Event) {
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

// Delete orphan videos and their unreferenced content indexes in the same transaction; a virtual video's projected
// row has no location by design and stays.
func deleteOrphanVideos(ctx context.Context, tx *sql.Tx) ([]domain.DeletedVideo, error) {
	released, err := collectDeletedVideos(tx.QueryContext(ctx, `delete from videos where not exists (
		select 1 from video_locations where video_locations.video_id = videos.id)
		and not exists (select 1 from virtual_videos where virtual_videos.user_key = videos.content_key)
		returning id, content_key`))
	if err != nil {
		return nil, err
	}
	if err := releaseContentIndex(ctx, tx, released); err != nil {
		return nil, err
	}
	return released, nil
}

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

// The driver applies _pragma parameters to every connection.
func dsn(path string, txLock string) string {
	q := url.Values{}
	// Use immediate writes and deferred snapshots; upgrading a deferred read can fail with SQLITE_BUSY_SNAPSHOT without honoring busy_timeout.
	q.Set("_txlock", txLock)
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout))
	q.Add("_pragma", "foreign_keys(on)")
	return "file:" + path + "?" + q.Encode()
}

// Open creates the database under the data directory if needed; callers must Close it.
func Open(dataDir string) (*DB, error) {
	return OpenContext(context.Background(), dataDir)
}

// OpenContext uses the caller's context for the initial connectivity check.
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

func (db *DB) Path() string {
	return db.path
}

func (db *DB) Ping(ctx context.Context) error {
	if err := db.sql.PingContext(ctx); err != nil {
		return fmt.Errorf("cannot reach the database (%s): %w", db.path, err)
	}
	if err := db.read.PingContext(ctx); err != nil {
		return fmt.Errorf("cannot reach the read-only database (%s): %w", db.path, err)
	}
	return nil
}

func (db *DB) Close() error {
	return errors.Join(db.read.Close(), db.sql.Close())
}
