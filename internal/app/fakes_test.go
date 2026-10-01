package app

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// このファイルの偽物は、アプリケーション層が宣言した interface を満たすだけの
// 決め打ちである。SQLite・ffmpeg/ffprobe・HTTP サーバーのどれも起動しない。

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakePublisher は発行された変化を記録する。
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

// fakeIngestStore は取り込みのジョブが読み書きする保存先の偽物である。
type fakeIngestStore struct {
	mu sync.Mutex

	videos map[int64]domain.Video
	// stale が真なら、専有した時点から所在や内容が変わったものとして答える。
	stale bool
	// gone が真なら、結果を反映する時点で動画が消えていたものとして答える。
	gone bool
	// referenced は内容の識別子を参照する動画があるか。
	referenced map[string]bool

	appliedProbes   []domain.Probe
	thumbnailStates []domain.ThumbnailState
	seekStates      []domain.SeekThumbnailState
	// thumbnailSubstitutions と seekSubstitutions は、成功とともに渡された代用。
	thumbnailSubstitutions []domain.Substitution
	seekSubstitutions      []domain.Substitution
	previewsDone           int
	// fingerprints は ApplyFingerprintForJob に渡された指紋。
	fingerprints []domain.Fingerprint
	// positions は SetThumbnailPosition に渡された位置（nil は解除）。
	positions []*int64
	// setPositionErr があれば SetThumbnailPosition は記録せずにそれを返す。
	setPositionErr error
	// sourceChecks は ThumbnailSourceCurrent の呼び出しで、sourceLocations はそれに渡された所在。
	// staleSourceAt が正なら、その回目の呼び出しから所在が別の内容へ付け替わったと答える。
	sourceChecks    int
	sourceLocations []string
	staleSourceAt   int
	// reads は GetVideo の呼び出し回数。afterRead があれば、読んだあとに錠を持ったまま
	// 呼び出し回数で呼ぶ。
	reads     int
	afterRead func(reads int)
	// order は、nil でなければシーク用サムネイルの状態の記録を生成の呼び出しと
	// 同じ列へ書く。生成と記録の順を確かめるのに使う。
	order *fakeGenerator
}

func newFakeIngestStore(videos ...domain.Video) *fakeIngestStore {
	store := &fakeIngestStore{videos: map[int64]domain.Video{}, referenced: map[string]bool{}}
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

// SetThumbnailPosition は位置を記録し、その内容の動画を done にする。order が nil でなければ
// 記録を生成の呼び出しと同じ列へ書く。
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
		f.videos[id] = other
	}
	return f.videos[videoID], nil
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

// fakeGenerator は生成の呼び出しを記録し、決め打ちの結果を返す。生成物の置き場
// （ArtifactStore）も兼ね、公開は write を呼ぶだけにする。
type fakeGenerator struct {
	mu sync.Mutex

	sourceErr    error
	probe        domain.Probe
	probeErr     error
	previewErr   error
	thumbnailErr error
	seekErr      error
	// thumbnailAtErr は指定の位置の代表サムネイルの生成が返す誤り。
	thumbnailAtErr error
	// recordPublish が真なら、代表サムネイルの公開も呼び出しの列に書く。
	recordPublish bool
	// positions は ThumbnailAt に渡された位置。
	positions []int64
	// firstFrame と fullDecode は、代表サムネイルとシーク用サムネイルの生成が返す代用。
	firstFrame bool
	fullDecode bool
	// seekPublished は、置き場に完成したシーク用サムネイルがあり、生成しないことを表す。
	seekPublished bool
	// publishedSubstitution は、採用したシーク用サムネイルに置き場が残した代用の記録。
	publishedSubstitution domain.Substitution
	// validated はプレビューの公開の直前に確かめた元の同一性。
	validated []bool
	// outputs は生成に渡した書き出し先。置き場が渡したものと同じであること。
	outputs []string
	// seekLayouts と publishedLayouts は、シーク用サムネイルの生成と公開に渡した配置。
	seekLayouts      []domain.SeekSpriteLayout
	publishedLayouts []domain.SeekSpriteLayout

	// sprites は置き場にある完成したシーク用スプライトの配置情報、sheets はそのシートで、
	// どちらも内容の識別子ごとである。
	sprites map[string]domain.SeekSprite
	sheets  map[string][][]byte
	// fingerprintSheets は SpriteFingerprint に渡されたシート。
	fingerprintSheets [][][]byte

	// gate は生成（thumbnail・seek・preview）の途中で呼ばれる。生成を止めておく
	// テストが使う。
	gate func(call string)

	calls   []string
	removed []string
	// stashes は代表サムネイルの写し（stash）・戻し（restore）・捨て（discard）の列である。
	stashes []string
}

func (f *fakeGenerator) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
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

func (f *fakeGenerator) Thumbnail(_ context.Context, _ string, _ int64, output string) (bool, error) {
	f.record("thumbnail")
	f.recordOutput(output)
	f.pass("thumbnail")
	return f.firstFrame, f.thumbnailErr
}

func (f *fakeGenerator) ThumbnailAt(_ context.Context, _ string, positionMs int64, output string) error {
	f.record("thumbnail-at")
	f.recordOutput(output)
	f.mu.Lock()
	f.positions = append(f.positions, positionMs)
	f.mu.Unlock()
	f.pass("thumbnail-at")
	return f.thumbnailAtErr
}

func (f *fakeGenerator) SeekSprite(_ context.Context, _, outputDir string, layout domain.SeekSpriteLayout) (bool, error) {
	f.record("seek")
	f.recordOutput(outputDir)
	f.pass("seek")
	f.mu.Lock()
	f.seekLayouts = append(f.seekLayouts, layout)
	f.mu.Unlock()
	return f.fullDecode, f.seekErr
}

func (f *fakeGenerator) Preview(_ context.Context, _, output string, _ int64) error {
	f.record("preview")
	f.recordOutput(output)
	f.pass("preview")
	return f.previewErr
}

func (f *fakeGenerator) PublishThumbnail(contentKey string, write func(string) error) error {
	if err := write("tmp/thumbnail/" + contentKey); err != nil {
		return err
	}
	if f.recordPublish {
		f.record("publish-thumbnail")
	}
	return nil
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
	contentKey string, layout domain.SeekSpriteLayout, write func(string) (bool, error),
) (domain.Substitution, error) {
	f.mu.Lock()
	f.publishedLayouts = append(f.publishedLayouts, layout)
	adopted := f.seekPublished
	f.mu.Unlock()
	if adopted {
		return f.publishedSubstitution, nil
	}
	fullDecode, err := write("tmp/seek/" + contentKey)
	if err != nil {
		return domain.SubstitutionUnknown, err
	}
	return domain.SubstitutionOf(fullDecode), nil
}

func (f *fakeGenerator) PublishPreview(
	ctx context.Context, contentKey string, write func(string) error, current func(context.Context) (bool, error),
) error {
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

// fakeArtifactStore は fakeGenerator を生成物の置き場として使う。スプライトの読み出しは
// 生成（Generator.SeekSprite）と名前が重なるので、ここで置き場の側を答える。
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

// SpriteFingerprint はシートの数だけコマを持つ指紋を返す。コマのハッシュはシートの長さ。
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

// probedVideo は解析済みの動画を1件返す。
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
