package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Fakes implement application interfaces without starting SQLite, media tools, or HTTP servers.

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakePublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (f *fakePublisher) Publish(events ...domain.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
}

func (f *fakePublisher) published() []domain.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Event(nil), f.events...)
}

type fakeIngestStore struct {
	mu sync.Mutex

	videos map[int64]domain.Video
	// stale simulates location/content replacement after claiming.
	stale bool
	// gone simulates deletion before results apply.
	gone       bool
	referenced map[string]bool

	appliedProbes          []domain.Probe
	thumbnailStates        []domain.ThumbnailState
	seekStates             []domain.SeekThumbnailState
	thumbnailSubstitutions []domain.Substitution
	seekSubstitutions      []domain.Substitution
	previewsDone           int
	fingerprints           []domain.Fingerprint
	// nil position clears the override.
	positions []*int64
	// Return setPositionErr without recording changes.
	setPositionErr error
	// Injected apply/revert errors leave stored images unchanged.
	images        map[string][]byte
	sources       map[string][]byte
	applyImageErr error
	// Position changes and reset remove these source/provenance rows together with images.
	rows           map[string]domain.AppliedThumbnailImage
	revertImageErr error
	imageRevision  int64
	// From staleSourceAt onward, source checks report content replacement.
	sourceChecks    int
	sourceLocations []string
	staleSourceAt   int
	// afterRead runs with the mutex held and receives the read count.
	reads     int
	afterRead func(reads int)
	// Record generation and state writes together to assert ordering.
	order *fakeGenerator
}

func newFakeIngestStore(videos ...domain.Video) *fakeIngestStore {
	store := &fakeIngestStore{
		videos: map[int64]domain.Video{}, referenced: map[string]bool{}, images: map[string][]byte{},
		rows: map[string]domain.AppliedThumbnailImage{},
	}
	for _, video := range videos {
		store.videos[video.ID] = video
		store.referenced[video.ContentKey] = true
	}
	return store
}

func (f *fakeIngestStore) ContentKeyReferenced(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.referenced[key], nil
}

func (f *fakeIngestStore) GetVideo(_ context.Context, id int64) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	video, ok := f.videos[id]
	if f.afterRead != nil {
		f.afterRead(f.reads)
	}
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return video, nil
}

func (f *fakeIngestStore) JobIdentityCurrent(context.Context, domain.Job) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) ContentKeyCurrent(context.Context, int64, string) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) PreviewSourceCurrent(context.Context, domain.Job) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) ThumbnailSourceCurrent(_ context.Context, _ int64, _, locationPath string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sourceChecks++
	f.sourceLocations = append(f.sourceLocations, locationPath)
	if f.staleSourceAt > 0 && f.sourceChecks >= f.staleSourceAt {
		return false, nil
	}
	return !f.stale, nil
}

func (f *fakeIngestStore) ApplyProbeForJob(
	_ context.Context, _ domain.Job, probe domain.Probe, _ domain.Playability,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return false, nil
	}
	f.appliedProbes = append(f.appliedProbes, probe)
	return true, nil
}

func (f *fakeIngestStore) SetThumbnailStateForJob(
	_ context.Context, job domain.Job, state domain.ThumbnailState, substitution domain.Substitution,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		delete(f.referenced, job.ContentKey)
		return false, nil
	}
	f.thumbnailStates = append(f.thumbnailStates, state)
	f.thumbnailSubstitutions = append(f.thumbnailSubstitutions, substitution)
	return true, nil
}

func (f *fakeIngestStore) SetSeekThumbnailStateForJob(
	_ context.Context, job domain.Job, state domain.SeekThumbnailState, substitution domain.Substitution,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		delete(f.referenced, job.ContentKey)
		return false, nil
	}
	f.seekStates = append(f.seekStates, state)
	f.seekSubstitutions = append(f.seekSubstitutions, substitution)
	if f.order != nil {
		f.order.record("seek-state:" + string(state))
	}
	return true, nil
}

func (f *fakeIngestStore) ApplyFingerprintForJob(
	_ context.Context, _ domain.Job, fingerprint domain.Fingerprint,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return false, nil
	}
	f.fingerprints = append(f.fingerprints, fingerprint)
	return true, nil
}

func (f *fakeIngestStore) SetThumbnailPosition(
	_ context.Context, videoID int64, positionMs *int64,
) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	video, ok := f.videos[videoID]
	if !ok || f.gone {
		return domain.Video{}, domain.ErrNotFound
	}
	if f.setPositionErr != nil {
		return domain.Video{}, f.setPositionErr
	}
	f.positions = append(f.positions, positionMs)
	if f.order != nil {
		f.order.record("set-position")
	}
	delete(f.images, video.ContentKey)
	delete(f.rows, video.ContentKey)
	for id, other := range f.videos {
		if other.ContentKey != video.ContentKey {
			continue
		}
		other.ThumbnailState = domain.ThumbnailStateDone
		other.ThumbnailPositionMs = positionMs
		if positionMs != nil {
			other.ThumbnailRevision++
		} else {
			other.ThumbnailRevision = 0
		}
		other.ThumbnailImageRevision = 0
		other.ThumbnailImageOrigin = ""
		f.videos[id] = other
	}
	return f.videos[videoID], nil
}

func (f *fakeIngestStore) ApplyThumbnailImage(
	_ context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	video, ok := f.videos[videoID]
	if !ok || f.gone || video.ContentKey != contentKey {
		return domain.Video{}, domain.ErrContentChanged
	}
	if f.applyImageErr != nil {
		return domain.Video{}, f.applyImageErr
	}
	f.images[contentKey] = append([]byte(nil), image.Image...)
	f.rows[contentKey] = image
	f.imageRevision++
	if f.order != nil {
		f.order.record("apply-image")
	}
	f.setImageRevision(contentKey, f.imageRevision, image.Origin)
	return f.videos[videoID], nil
}

func (f *fakeIngestStore) RevertThumbnailImage(_ context.Context, videoID int64) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	video, ok := f.videos[videoID]
	if !ok || f.gone {
		return domain.Video{}, domain.ErrNotFound
	}
	if f.revertImageErr != nil {
		return domain.Video{}, f.revertImageErr
	}
	delete(f.images, video.ContentKey)
	delete(f.rows, video.ContentKey)
	if f.order != nil {
		f.order.record("revert-image")
	}
	f.setImageRevision(video.ContentKey, 0, "")
	return f.videos[videoID], nil
}

// Mirrors the store: the image row goes and the thumbnail returns to pending.
func (f *fakeIngestStore) RemoveVirtualThumbnailImage(_ context.Context, videoID int64, contentKey string) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	video, ok := f.videos[videoID]
	switch {
	case !ok:
		return domain.Video{}, domain.ErrNotFound
	case !video.Virtual():
		return domain.Video{}, domain.ErrNotVirtual
	case video.ContentKey != contentKey:
		return domain.Video{}, domain.ErrContentChanged
	}
	delete(f.images, contentKey)
	delete(f.rows, contentKey)
	video.ThumbnailState, video.ThumbnailImageRevision, video.ThumbnailImageOrigin = domain.ThumbnailStatePending, 0, ""
	f.videos[videoID] = video
	return video, nil
}

func (f *fakeIngestStore) VirtualThumbnailKeys(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var keys []string
	for key := range f.images {
		if domain.IsVirtualKey(key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys, nil
}

// Caller must hold f.mu.
func (f *fakeIngestStore) setImageRevision(key string, revision int64, origin domain.ThumbnailImageOrigin) {
	for id, other := range f.videos {
		if other.ContentKey != key {
			continue
		}
		other.ThumbnailState = domain.ThumbnailStateDone
		other.ThumbnailImageRevision = revision
		other.ThumbnailImageOrigin = origin
		f.videos[id] = other
	}
}

func (f *fakeIngestStore) ThumbnailImage(_ context.Context, contentKey string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	image, ok := f.images[contentKey]
	if !ok {
		return nil, nil
	}
	return append([]byte(nil), image...), nil
}

func (f *fakeIngestStore) ThumbnailImageSource(_ context.Context, contentKey string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.sources[contentKey]...), nil
}

func (f *fakeIngestStore) CompletePreviewForContent(_ context.Context, job domain.Job) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		delete(f.referenced, job.ContentKey)
		return false, nil
	}
	f.previewsDone++
	return true, nil
}

// The fake doubles as ArtifactStore; publication invokes write without real generation.
type fakeGenerator struct {
	mu sync.Mutex

	sourceErr      error
	probe          domain.Probe
	probeErr       error
	previewErr     error
	thumbnailErr   error
	seekErr        error
	thumbnailAtErr error
	recordPublish  bool
	positions      []int64
	fitErr         error
	aspects        []float64
	fitImages      []string
	// Return publishErr after writing to simulate publication failure.
	publishErr error
	// A nonempty thumbnailDir supplies real output paths; otherwise use unwritable fixture paths.
	thumbnailDir string
	published    map[string][]byte
	// Published posters by content key; absent when the publish wrote none.
	posters map[string][]byte
	// Poster paths passed to Thumbnail and ThumbnailAt.
	posterOutputs []string
	firstFrame    bool
	// Complete seek artifacts bypass generation.
	seekPublished         bool
	publishedSubstitution domain.Substitution
	validated             []bool
	outputs               []string
	seekLayouts           []domain.SeekSpriteLayout
	publishedLayouts      []domain.SeekSpriteLayout
	// The force flag of each seek sprite and preview publish.
	publishForces []bool

	sprites           map[string]domain.SeekSprite
	sheets            map[string][][]byte
	fingerprintSheets [][][]byte

	// The gate can pause generation to exercise interleavings.
	gate func(call string)

	calls   []string
	removed []string
	stashes []string
}

func (f *fakeGenerator) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeGenerator) recordPoster(poster string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posterOutputs = append(f.posterOutputs, poster)
}

func (f *fakeGenerator) recordOutput(output string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output)
}

func (f *fakeGenerator) pass(call string) {
	if f.gate != nil {
		f.gate(call)
	}
}

func (f *fakeGenerator) CheckSource(string) error {
	f.record("check")
	return f.sourceErr
}

func (f *fakeGenerator) Probe(context.Context, string) (domain.Probe, error) {
	f.record("probe")
	return f.probe, f.probeErr
}

func (f *fakeGenerator) Thumbnail(_ context.Context, _ string, _ int64, output, poster string) (bool, error) {
	f.record("thumbnail")
	f.recordOutput(output)
	f.recordPoster(poster)
	f.pass("thumbnail")
	return f.firstFrame, f.thumbnailErr
}

func (f *fakeGenerator) ThumbnailAt(_ context.Context, _ string, positionMs int64, output, poster string) error {
	f.record("thumbnail-at")
	f.recordOutput(output)
	f.recordPoster(poster)
	f.mu.Lock()
	f.positions = append(f.positions, positionMs)
	f.mu.Unlock()
	f.pass("thumbnail-at")
	return f.thumbnailAtErr
}

func (f *fakeGenerator) SeekSprite(_ context.Context, _, outputDir string, layout domain.SeekSpriteLayout) error {
	f.record("seek")
	f.recordOutput(outputDir)
	f.pass("seek")
	f.mu.Lock()
	f.seekLayouts = append(f.seekLayouts, layout)
	f.mu.Unlock()
	return f.seekErr
}

func (f *fakeGenerator) Preview(_ context.Context, _, output string, _ int64) error {
	f.record("preview")
	f.recordOutput(output)
	f.pass("preview")
	return f.previewErr
}

func (f *fakeGenerator) PublishThumbnail(contentKey string, write func(string, string) error) error {
	output, poster := "tmp/thumbnail/"+contentKey, "tmp/poster/"+contentKey
	if f.thumbnailDir != "" {
		output = filepath.Join(f.thumbnailDir, contentKey+".jpg")
		poster = filepath.Join(f.thumbnailDir, contentKey+".poster.jpg")
	}
	if err := write(output, poster); err != nil {
		return err
	}
	if f.publishErr != nil {
		return f.publishErr
	}
	if f.thumbnailDir != "" {
		// Fake frame generation writes nothing, so absent output is treated as empty.
		data, err := os.ReadFile(output)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		posterData, err := os.ReadFile(poster)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		f.mu.Lock()
		if f.published == nil {
			f.published, f.posters = map[string][]byte{}, map[string][]byte{}
		}
		f.published[contentKey] = data
		if posterData == nil {
			delete(f.posters, contentKey)
		} else {
			f.posters[contentKey] = posterData
		}
		f.mu.Unlock()
	}
	if f.recordPublish {
		f.record("publish-thumbnail")
	}
	return nil
}

// A thumbnail counts as available once published through thumbnailDir.
func (f *fakeGenerator) ThumbnailAvailable(contentKey string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.published[contentKey]
	return ok
}

// Prefix source bytes with source: and share fitErr with thumbnail fitting.
func (f *fakeGenerator) FitThumbnailSource(_ context.Context, image []byte, _ float64) ([]byte, error) {
	f.record("fit-source")
	if f.fitErr != nil {
		return nil, f.fitErr
	}
	return append([]byte("source:"), image...), nil
}

// Prefix fitted bytes with fitted:; the gate name includes the image.
func (f *fakeGenerator) FitThumbnailImage(_ context.Context, image []byte, aspect float64) ([]byte, error) {
	f.record("fit")
	f.mu.Lock()
	f.aspects = append(f.aspects, aspect)
	f.fitImages = append(f.fitImages, string(image))
	f.mu.Unlock()
	// Include image identity so tests can block one particular fit.
	f.pass("fit:" + string(image))
	if f.fitErr != nil {
		return nil, f.fitErr
	}
	return append([]byte("fitted:"), image...), nil
}

func (f *fakeGenerator) StashThumbnail(string) (func() error, func(), error) {
	f.recordStash("stash")
	restore := func() error {
		f.recordStash("restore")
		return nil
	}
	return restore, func() { f.recordStash("discard") }, nil
}

func (f *fakeGenerator) recordStash(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stashes = append(f.stashes, call)
}

func (f *fakeGenerator) stashCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stashes...)
}

func (f *fakeGenerator) PublishSeekThumbnails(
	contentKey string, layout domain.SeekSpriteLayout, force bool, write func(string) error,
) (domain.Substitution, error) {
	f.mu.Lock()
	f.publishedLayouts = append(f.publishedLayouts, layout)
	f.publishForces = append(f.publishForces, force)
	adopted := f.seekPublished && !force
	f.mu.Unlock()
	if adopted {
		return f.publishedSubstitution, nil
	}
	if err := write("tmp/seek/" + contentKey); err != nil {
		return domain.SubstitutionUnknown, err
	}
	return domain.SubstitutionNone, nil
}

func (f *fakeGenerator) PublishPreview(
	ctx context.Context, contentKey string, force bool, write func(string) error, current func(context.Context) (bool, error),
) error {
	f.mu.Lock()
	f.publishForces = append(f.publishForces, force)
	f.mu.Unlock()
	if err := write("tmp/preview/" + contentKey); err != nil {
		return err
	}
	ok, err := current(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.validated = append(f.validated, ok)
	f.mu.Unlock()
	if !ok {
		return domain.ErrPreviewStale
	}
	return nil
}

// Separate artifact reads because SeekSprite conflicts with the generator method name.
type fakeArtifactStore struct {
	*fakeGenerator
}

func (f fakeArtifactStore) SeekSprite(contentKey string) (domain.SeekSprite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sprite, ok := f.sprites[contentKey]
	if !ok {
		return domain.SeekSprite{}, fs.ErrNotExist
	}
	return sprite, nil
}

func (f fakeArtifactStore) SeekSpriteSheet(contentKey string, sheet int) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sheets := f.sheets[contentKey]
	if sheet < 0 || sheet >= len(sheets) {
		return nil, fs.ErrNotExist
	}
	return sheets[sheet], nil
}

// Use one fingerprint frame per sheet, with its byte length as the hash.
func (f *fakeGenerator) SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error) {
	f.record("fingerprint")
	f.mu.Lock()
	f.fingerprintSheets = append(f.fingerprintSheets, sheets)
	f.mu.Unlock()
	frames := make([]domain.FrameHash, 0, len(sheets))
	for _, sheet := range sheets {
		frames = append(frames, domain.FrameHash{Hash: uint64(len(sheet))})
	}
	return domain.Fingerprint{Version: domain.FingerprintVersion, IntervalMs: sprite.IntervalMs, Frames: frames}, nil
}

func (f *fakeGenerator) RemoveContent(contentKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, contentKey)
	return nil
}

func (f *fakeGenerator) snapshot() (calls, removed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...), append([]string(nil), f.removed...)
}

func probedVideo(id int64, contentKey string) domain.Video {
	duration := int64(60_000)
	return domain.Video{
		ID:             id,
		Path:           "/media/show/" + contentKey + ".mp4",
		Title:          contentKey,
		AddedAt:        time.Unix(1_757_000_000, 0),
		ContentKey:     contentKey,
		DurationMs:     &duration,
		Container:      "mp4",
		VideoCodec:     "h264",
		AudioCodec:     "aac",
		ProbeState:     domain.ProbeStateDone,
		ThumbnailState: domain.ThumbnailStatePending,
		PreviewState:   domain.PreviewStatePending,
	}
}

func jobFor(kind domain.JobKind, video domain.Video) domain.Job {
	return domain.Job{
		ID: 1, Kind: kind, VideoID: video.ID, LocationPath: video.Path, ContentKey: video.ContentKey,
	}
}
