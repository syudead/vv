package store

import "database/sql"

// Role types own business operations; share private transaction helpers rather than calling another role's public methods.

// IngestStore owns job queues and ingest-stage index results.
type IngestStore struct{ db *DB }

// LibraryStore owns audience-aware catalog reads and search-key refreshes.
type LibraryStore struct{ db *DB }

// ScanStore persists scan execution state.
type ScanStore struct{ db *DB }

// ScanIndexStore applies scan results to rebuildable indexes and refreshes the folder index.
type ScanIndexStore struct{ db *DB }

// SettingsStore persists media-folder registrations and application settings.
type SettingsStore struct{ db *DB }

// FolderGroupStore atomically updates folder overrides and the folder index.
type FolderGroupStore struct{ db *DB }

// PlaybackStore persists playback positions and notifies smart folders after watch-state changes.
type PlaybackStore struct {
	sql *sql.DB
	db  *DB
}

// TagStore owns tags and attachments; page reads share a snapshot, and owner edits record activity in the same transaction.
type TagStore struct {
	sql, read *sql.DB
	db        *DB
}

// AuthStore owns the singleton account, sessions and API tokens without domain notifications.
type AuthStore struct{ sql *sql.DB }

// VisibilityStore writes public flags; LibraryStore enforces them on reads.
type VisibilityStore struct {
	sql *sql.DB
	db  *DB
}

// FavoriteStore writes favorites and notifies smart folders after commit.
type FavoriteStore struct {
	sql *sql.DB
	db  *DB
}

// PlaylistStore owns playlists and snapshot reads without domain events; private helpers rekey items within bundle and succession transactions.
type PlaylistStore struct{ sql, read *sql.DB }

// AutoTagStore owns the judgement queue and atomic tag application; SettingsStore owns its settings.
type AutoTagStore struct{ db *DB }

// FaceStore owns the face analysis queue and stores each video's result, with its people, links and activity, in
// one transaction; SettingsStore owns its settings.
type FaceStore struct{ db *DB }

// OverrideStore writes display overrides and fetched work information atomically with dependent search keys and tags.
type OverrideStore struct{ db *DB }

// VersionStore owns persistent version bundles and their shared user data, publishing changes after commit.
type VersionStore struct{ db *DB }

// GroupStore owns group edits; folder-index transactions synchronize automatic groups through private helpers.
type GroupStore struct{ db *DB }

// SmartFolderStore owns smart-folder edits and reference reads, publishing changes after commit.
type SmartFolderStore struct{ db *DB }

// CollectionStore removes groups and smart folders together through the group and smart-folder helpers.
type CollectionStore struct{ db *DB }

// OnboardingStore saves setup progress and explicitly chosen interest folders.
type OnboardingStore struct{ db *DB }
type OrganizeStore struct{ db *DB }

// CreditStore shares SQL across people and organizations, with snapshot reads and smart-folder notifications after relevant edits.
type CreditStore struct {
	sql, read *sql.DB
	db        *DB
}

// WorkInfoStore writes work details and display names in one transaction through private helpers, publishing changes after commit.
type WorkInfoStore struct{ db *DB }

// VideoEditStore applies one bulk edit across tags, work information and visibility in one transaction through
// those roles' private helpers, publishing their combined changes after commit.
type VideoEditStore struct{ db *DB }

// WorkInfoPluginStore revalidates approvals and settings inside write transactions; it emits no domain events.
type WorkInfoPluginStore struct{ sql, read *sql.DB }

// VirtualVideoStore owns virtual videos and their projected rows, publishing changes after commit.
type VirtualVideoStore struct{ db *DB }

// AssistantStore owns the assistant's proposals, each transition one UPDATE guarded by the previous status, with
// the assistant_run entry written in the same transaction and AssistantChanged published after commit.
type AssistantStore struct{ db *DB }

// ActivityStore owns reads and startup recovery; other roles write activity through private helpers in their own transactions.
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
func (db *DB) Faces() *FaceStore               { return &FaceStore{db: db} }
func (db *DB) Versions() *VersionStore         { return &VersionStore{db: db} }
func (db *DB) Groups() *GroupStore             { return &GroupStore{db: db} }
func (db *DB) Organize() *OrganizeStore        { return &OrganizeStore{db: db} }
func (db *DB) SmartFolders() *SmartFolderStore { return &SmartFolderStore{db: db} }
func (db *DB) Onboarding() *OnboardingStore    { return &OnboardingStore{db: db} }
func (db *DB) Collections() *CollectionStore   { return &CollectionStore{db: db} }
func (db *DB) Credits() *CreditStore           { return &CreditStore{sql: db.sql, read: db.read, db: db} }
func (db *DB) WorkInfo() *WorkInfoStore        { return &WorkInfoStore{db: db} }
func (db *DB) VideoEdits() *VideoEditStore     { return &VideoEditStore{db: db} }
func (db *DB) WorkInfoPlugins() *WorkInfoPluginStore {
	return &WorkInfoPluginStore{sql: db.sql, read: db.read}
}
func (db *DB) Activity() *ActivityStore   { return &ActivityStore{db: db} }
func (db *DB) Assistant() *AssistantStore { return &AssistantStore{db: db} }
func (db *DB) VirtualVideos() *VirtualVideoStore {
	return &VirtualVideoStore{db: db}
}
