package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/syudead/vv/internal/domain"
)

type IngestStore interface {
	ContentIndex
	GetVideo(ctx context.Context, id int64) (domain.Video, error)
	JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error)
	ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error)
	PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error)
	// Require both unchanged video content and membership of locationPath in that content's locations.
	ThumbnailSourceCurrent(ctx context.Context, videoID int64, key, locationPath string) (bool, error)
	// Probe results and preview jobs commit together.
	ApplyProbeForJob(ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability) (bool, error)
	// Record or clear substitution issues in the same transaction as successful stage completion.
	SetThumbnailStateForJob(
		ctx context.Context, job domain.Job, state domain.ThumbnailState, substitution domain.Substitution) (bool, error)
	SetSeekThumbnailStateForJob(
		ctx context.Context, job domain.Job, state domain.SeekThumbnailState, substitution domain.Substitution) (bool, error)
	CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error)
	// Apply fingerprints only while claimed content and location still match.
	ApplyFingerprintForJob(ctx context.Context, job domain.Job, fingerprint domain.Fingerprint) (bool, error)
	// After publication, record position and done state within the generation lock; nil clears the
	// position.
	SetThumbnailPosition(ctx context.Context, videoID int64, positionMs *int64) (domain.Video, error)
	// After publication under the generation lock, atomically store image/provenance and done state;
	// missing or changed content returns ErrContentChanged without writing.
	ApplyThumbnailImage(
		ctx context.Context, videoID int64, contentKey string, image domain.AppliedThumbnailImage,
	) (domain.Video, error)
	// After frame publication under the generation lock, atomically remove the image row and mark
	// matching content done.
	RevertThumbnailImage(ctx context.Context, videoID int64) (domain.Video, error)
	// Nil means no applied image.
	ThumbnailImage(ctx context.Context, contentKey string) ([]byte, error)
	// Nil means no applied image or one applied before sources were stored.
	ThumbnailImageSource(ctx context.Context, contentKey string) ([]byte, error)
}

type ContentIndex interface {
	ContentKeyReferenced(ctx context.Context, key string) (bool, error)
}

type ArtifactRemover interface {
	RemoveContent(contentKey string) error
}

// ArtifactStore owns paths and atomic publication; app supplies generation callbacks.
type ArtifactStore interface {
	ArtifactRemover
	// write leaves poster empty when it has no larger image; the store then removes the old poster.
	PublishThumbnail(contentKey string, write func(output, poster string) error) error
	// restore removes the replacement when no prior thumbnail existed; discard drops only the backup.
	StashThumbnail(contentKey string) (restore func() error, discard func(), err error)
	// Reuse complete sprites without write, replacing incomplete legacy directories during
	// publication; return persisted substitution provenance or SubstitutionUnknown for old metadata.
	PublishSeekThumbnails(contentKey string, layout domain.SeekSpriteLayout,
		write func(outputDir string) (fullDecode bool, err error)) (domain.Substitution, error)
	// Reuse complete previews; current=false immediately before publication returns ErrPreviewStale
	// without publishing.
	PublishPreview(ctx context.Context, contentKey string, write func(output string) error,
		current func(context.Context) (bool, error)) error
	SeekSprite(contentKey string) (domain.SeekSprite, error)
	// Sheet indexes start at zero.
	SeekSpriteSheet(contentKey string, sheet int) ([]byte, error)
}

type Generator interface {
	CheckSource(path string) error
	Probe(ctx context.Context, path string) (domain.Probe, error)
	// Thumbnail reports first-frame substitution; poster receives the same frame at up to
	// domain.ThumbnailFrameMaxSide.
	Thumbnail(ctx context.Context, path string, durationMs int64, output, poster string) (firstFrame bool, err error)
	// ThumbnailAt never substitutes the first frame.
	ThumbnailAt(ctx context.Context, path string, positionMs int64, output, poster string) error
	// SeekSprite reports fallback to full decoding after segment extraction fails.
	SeekSprite(ctx context.Context, path, outputDir string, layout domain.SeekSpriteLayout) (fullDecode bool, err error)
	Preview(ctx context.Context, path, output string, durationMs int64) error
	// Build fingerprints from ordered JPEG sheets without ffmpeg.
	SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error)
	// Zero aspect means unknown; unreadable images return ErrThumbnailImageInvalid.
	FitThumbnailImage(ctx context.Context, image []byte, aspect float64) ([]byte, error)
	// Use the same crop with at most 2048px longest side; unreadable images return
	// ErrThumbnailImageInvalid.
	FitThumbnailSource(ctx context.Context, image []byte, aspect float64) ([]byte, error)
}

type IngestOptions struct {
	Store     IngestStore
	Generator Generator
	Artifacts ArtifactStore
	// Nil disables events.
	Publisher Publisher
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// Ingest owns each job's effects and artifact cleanup; jobs owns claiming and recording job
// outcomes.
type Ingest struct {
	store     IngestStore
	generator Generator
	files     ArtifactStore
	publisher Publisher
	artifacts *artifacts
	// Serialize uploaded-image decoding across both validation and fitting passes; see
	// thumbnail_upload.go.
	uploads     uploadGate
	checkUpload func(image []byte) error
}

func NewIngest(opts IngestOptions) *Ingest {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Ingest{
		store:       opts.Store,
		generator:   opts.Generator,
		files:       opts.Artifacts,
		publisher:   opts.Publisher,
		artifacts:   newArtifacts(opts.Store, opts.Artifacts, logger),
		uploads:     newUploadGate(),
		checkUpload: checkThumbnailUpload,
	}
}

// Unknown stages return nil handlers.
func (i *Ingest) Handler(kind domain.JobKind) func(context.Context, domain.Job) error {
	switch kind {
	case domain.JobProbe:
		return i.Probe
	case domain.JobThumbnail:
		return i.Thumbnail
	case domain.JobSeekThumbnail:
		return i.SeekThumbnails
	case domain.JobPreview:
		return i.Preview
	case domain.JobFingerprint:
		return i.Fingerprint
	}
	return nil
}

// Publish stage completion; cmd/mdm subscriptions wire downstream worker wakeups without direct
// stage coupling.
func (i *Ingest) JobFinished(job domain.Job) {
	if i.publisher == nil {
		return
	}
	i.publisher.Publish(
		domain.VideoIngestChanged{VideoID: job.VideoID, Stage: job.Kind},
		domain.ProcessingChanged{},
	)
}

// Recheck references immediately before deletion so reimported content retains artifacts.
func (i *Ingest) ReleaseArtifacts(event domain.ContentUnreferenced) {
	i.artifacts.release(event.ContentKeys)
}

// Wait must finish before database closure or pending cleanup could leak artifacts.
func (i *Ingest) Wait() {
	i.artifacts.wait()
}

// Probe persists domain-derived results and queues previews in the same storage transaction.
func (i *Ingest) Probe(ctx context.Context, job domain.Job) error {
	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}

	// FailClaimedJob atomically records exhausted failures on jobs and videos, retaining library rows.
	probe, err := i.generator.Probe(ctx, job.LocationPath)
	if err != nil {
		return err
	}

	playability := domain.EvaluatePlayability(domain.ContainerFromPath(job.LocationPath), probe)
	_, err = i.store.ApplyProbeForJob(ctx, job, probe, playability)
	return err
}

// Reread applied image, position, and state under the generation lock so a queued job cannot
// overwrite a newer owner change; applied images take precedence over selected or automatic frames.
func (i *Ingest) Thumbnail(ctx context.Context, job domain.Job) error {
	// The unlocked read only checks whether the video still exists.
	if _, err := i.store.GetVideo(ctx, job.VideoID); err != nil {
		return err
	}

	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	// Applied images can be republished even when source media is unreadable; defer that error until
	// the locked read confirms no image exists.
	sourceErr := i.generator.CheckSource(job.LocationPath)
	if sourceErr != nil {
		image, err := i.store.ThumbnailImage(ctx, job.ContentKey)
		if err != nil {
			return err
		}
		if image == nil {
			return sourceErr
		}
	}
	// Keep publication and completion serialized with deletion, without blocking other artifact kinds.
	return i.artifacts.generate(ctx, job.ContentKey, artifactThumbnail, func() (bool, error) {
		video, err := i.store.GetVideo(ctx, job.VideoID)
		if errors.Is(err, domain.ErrNotFound) {
			// Cleanup handles artifacts left after deletion while waiting for the lock.
			return true, nil
		}
		if err != nil {
			return false, err
		}
		// Exhausted failure is recorded with the job transaction; a missing selected frame must not
		// delete the video.
		if video.ThumbnailState != domain.ThumbnailStateDone {
			// Record first-frame substitution atomically with success; explicit positions never substitute.
			substitution := domain.SubstitutionUnknown
			image, err := i.store.ThumbnailImage(ctx, job.ContentKey)
			if err != nil {
				return false, err
			}
			source, err := i.store.ThumbnailImageSource(ctx, job.ContentKey)
			if err != nil {
				return false, err
			}
			if image == nil && sourceErr != nil {
				// The applied image was reverted while waiting; unreadable media cannot supply its replacement
				// frame.
				return false, sourceErr
			}
			if err := i.files.PublishThumbnail(job.ContentKey, func(output, poster string) error {
				if image != nil {
					substitution = domain.SubstitutionNone
					return writeThumbnailImages(output, image, poster, source)
				}
				if video.ThumbnailPositionMs != nil {
					substitution = domain.SubstitutionNone
					return i.generator.ThumbnailAt(ctx, job.LocationPath, *video.ThumbnailPositionMs, output, poster)
				}
				firstFrame, err := i.generator.Thumbnail(ctx, job.LocationPath, durationOf(video), output, poster)
				substitution = domain.SubstitutionOf(firstFrame)
				return err
			}); err != nil {
				return false, err
			}
			if _, err := i.store.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone, substitution); err != nil {
				return false, err
			}
		}
		// Recheck references after generation because scans may delete the video concurrently.
		return true, nil
	})
}

// SetThumbnailPosition validates duration/bounds and publishes under the generation lock, with nil
// restoring automatic selection and extraction failures preserving the old image. Recheck source
// identity before generation, publication, and recording; roll back published bytes on any later
// failure so file, position, and revision agree.
func (i *Ingest) SetThumbnailPosition(
	ctx context.Context, videoID int64, locationPath, path string, positionMs *int64,
) (domain.Video, error) {
	video, err := i.store.GetVideo(ctx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			return domain.Video{}, err
		}
	}
	if video.ContentKey == "" {
		// Contentless videos cannot have artifact paths or override rows.
		return domain.Video{}, domain.ErrNotFound
	}

	return i.replaceThumbnailWithFrame(ctx, video, locationPath, path,
		func() (*int64, error) { return positionMs, nil },
		func() (domain.Video, error) { return i.store.SetThumbnailPosition(ctx, videoID, positionMs) })
}

// RevertThumbnailImage rereads position under the generation lock and restores that frame or the
// automatic frame, even when no applied image exists. It shares SetThumbnailPosition identity
// checks and rollback, preserving the image row/file on extraction failure.
func (i *Ingest) RevertThumbnailImage(
	ctx context.Context, videoID int64, locationPath, path string,
) (domain.Video, error) {
	video, err := i.store.GetVideo(ctx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if video.ContentKey == "" {
		return domain.Video{}, domain.ErrNotFound
	}
	return i.replaceThumbnailWithFrame(ctx, video, locationPath, path,
		func() (*int64, error) {
			current, err := i.store.GetVideo(ctx, videoID)
			if err != nil {
				return nil, err
			}
			return current.ThumbnailPositionMs, nil
		},
		func() (domain.Video, error) { return i.store.RevertThumbnailImage(ctx, videoID) })
}

// Choose the frame position inside the generation lock; reuse SetThumbnailPosition source checks,
// backup, and rollback.
func (i *Ingest) replaceThumbnailWithFrame(
	ctx context.Context, video domain.Video, locationPath, path string,
	position func() (*int64, error), record func() (domain.Video, error),
) (domain.Video, error) {
	videoID := video.ID
	var saved domain.Video
	err := i.artifacts.generate(ctx, video.ContentKey, artifactThumbnail, func() (bool, error) {
		if err := i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath); err != nil {
			return false, err
		}
		positionMs, err := position()
		if err != nil {
			return false, err
		}
		restore, discard, err := i.files.StashThumbnail(video.ContentKey)
		if err != nil {
			return false, err
		}
		var generateErr, sourceErr error
		if err := i.files.PublishThumbnail(video.ContentKey, func(output, poster string) error {
			if positionMs != nil {
				generateErr = i.generator.ThumbnailAt(ctx, path, *positionMs, output, poster)
			} else {
				_, generateErr = i.generator.Thumbnail(ctx, path, durationOf(video), output, poster)
			}
			if generateErr != nil {
				return generateErr
			}
			// Check before rename so even concurrent readers cannot see a frame from replaced content.
			sourceErr = i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath)
			return sourceErr
		}); err != nil {
			discard()
			if generateErr != nil {
				return false, errors.Join(domain.ErrThumbnailFrameUnavailable, generateErr)
			}
			if sourceErr != nil {
				return false, sourceErr
			}
			return false, err
		}
		// Keep published bytes only after recording succeeds; restore on failure and clean up even
		// restored bytes if the video disappeared.
		if err := i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath); err != nil {
			return true, errors.Join(err, restore())
		}
		recorded, err := record()
		if err != nil {
			return true, errors.Join(err, restore())
		}
		discard()
		saved = recorded
		return true, nil
	})
	if err != nil {
		return domain.Video{}, err
	}
	return saved, nil
}

// ApplyThumbnailImage rechecks content under a cancelable generation lock, fits thumbnail/source
// JPEGs, and atomically records image, provenance, and done state. Publication failure preserves
// existing data, while recording failure restores the stashed file so the last committed edit wins.
func (i *Ingest) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte, origin domain.ThumbnailImageOrigin,
) (domain.Video, error) {
	if contentKey == "" {
		return domain.Video{}, domain.ErrContentChanged
	}
	var saved domain.Video
	err := i.artifacts.generateContext(ctx, contentKey, artifactThumbnail, func() (bool, error) {
		video, err := i.store.GetVideo(ctx, videoID)
		if errors.Is(err, domain.ErrNotFound) {
			return false, domain.ErrContentChanged
		}
		if err != nil {
			return false, err
		}
		if video.ContentKey != contentKey {
			return false, domain.ErrContentChanged
		}
		applied, err := i.fitThumbnailImage(ctx, image, video, origin)
		if err != nil {
			return false, err
		}
		restore, discard, err := i.files.StashThumbnail(contentKey)
		if err != nil {
			return false, err
		}
		if err := i.files.PublishThumbnail(contentKey, func(output, poster string) error {
			return writeThumbnailImages(output, applied.Image, poster, applied.Source)
		}); err != nil {
			discard()
			return false, err
		}
		recorded, err := i.store.ApplyThumbnailImage(ctx, videoID, contentKey, applied)
		if err != nil {
			return true, errors.Join(err, restore())
		}
		discard()
		saved = recorded
		return true, nil
	})
	if err != nil {
		return domain.Video{}, err
	}
	return saved, nil
}

// Uploaded images hold the upload gate across both fitting passes; generated images bypass it.
func (i *Ingest) fitThumbnailImage(
	ctx context.Context, image []byte, video domain.Video, origin domain.ThumbnailImageOrigin,
) (domain.AppliedThumbnailImage, error) {
	if origin == domain.ThumbnailImageUploaded {
		release, err := i.uploads.acquire(ctx)
		if err != nil {
			return domain.AppliedThumbnailImage{}, err
		}
		defer release()
	}
	aspect := domain.ThumbnailImageAspect(video)
	fitted, err := i.generator.FitThumbnailImage(ctx, image, aspect)
	if err != nil {
		return domain.AppliedThumbnailImage{}, err
	}
	source, err := i.generator.FitThumbnailSource(ctx, image, aspect)
	if err != nil {
		return domain.AppliedThumbnailImage{}, err
	}
	return domain.AppliedThumbnailImage{Image: fitted, Source: source, Origin: origin}, nil
}

// The stored source becomes the poster; a row applied before sources were stored has none.
func writeThumbnailImages(output string, image []byte, poster string, source []byte) error {
	if err := os.WriteFile(output, image, 0o644); err != nil {
		return fmt.Errorf("cannot write the thumbnail image: %w", err)
	}
	if len(source) == 0 {
		return nil
	}
	if err := os.WriteFile(poster, source, 0o644); err != nil {
		return fmt.Errorf("cannot write the thumbnail poster: %w", err)
	}
	return nil
}

// Missing videos return ErrNotFound; removed or reassigned source locations return
// ErrMediaFileUnavailable.
func (i *Ingest) checkThumbnailSource(ctx context.Context, videoID int64, key, locationPath string) error {
	current, err := i.store.ThumbnailSourceCurrent(ctx, videoID, key, locationPath)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if _, err := i.store.GetVideo(ctx, videoID); err != nil {
		return err
	}
	return domain.ErrMediaFileUnavailable
}

// Unknown duration returns zero.
func durationOf(video domain.Video) int64 {
	if video.DurationMs == nil {
		return 0
	}
	return *video.DurationMs
}

// SeekThumbnails reuses complete sprites and records completion only for unchanged claimed
// identity; exhausted failures affect seek state only, never the representative thumbnail.
func (i *Ingest) SeekThumbnails(ctx context.Context, job domain.Job) error {
	video, err := i.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
	}
	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}
	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}
	layout := domain.NewSeekSpriteLayout(durationMs)
	// Serialize generation and completion with deletion, allowing other artifact kinds concurrently.
	return i.artifacts.generate(ctx, job.ContentKey, artifactSeekThumbnails, func() (bool, error) {
		// Reuse persisted substitution provenance after restart or across duplicate content; unknown
		// legacy provenance must not alter existing issue rows.
		substitution, err := i.files.PublishSeekThumbnails(job.ContentKey, layout, func(outputDir string) (bool, error) {
			return i.generator.SeekSprite(ctx, job.LocationPath, outputDir, layout)
		})
		if err != nil {
			return false, err
		}
		if _, err := i.store.SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, substitution); err != nil {
			return false, err
		}
		// Recheck references after generation because scans may delete the video concurrently.
		return true, nil
	})
}

var errMissingDuration = errors.New("the video has no duration to generate a preview from")

func (i *Ingest) Preview(ctx context.Context, job domain.Job) error {
	video, err := i.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
	}
	if video.ProbeState != domain.ProbeStateDone {
		return nil
	}
	if video.DurationMs == nil || *video.DurationMs <= 0 {
		return errMissingDuration
	}
	current, err := i.store.ContentKeyCurrent(ctx, job.VideoID, job.ContentKey)
	if err != nil || !current {
		return err
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}
	validateContent := func(validateCtx context.Context) (bool, error) {
		return i.store.PreviewSourceCurrent(validateCtx, job)
	}
	durationMs := *video.DurationMs
	// Serialize generation and completion with deletion, allowing other artifact kinds concurrently.
	return i.artifacts.generate(ctx, job.ContentKey, artifactPreview, func() (bool, error) {
		if err := i.files.PublishPreview(ctx, job.ContentKey, func(output string) error {
			return i.generator.Preview(ctx, job.LocationPath, output, durationMs)
		}, validateContent); err != nil {
			return false, err
		}
		applied, err := i.store.CompletePreviewForContent(context.WithoutCancel(ctx), job)
		if err != nil {
			return false, err
		}
		if !applied {
			return true, domain.ErrPreviewStale
		}
		return false, nil
	})
}

// Fingerprint reads completed sprite sheets without source media or ffmpeg; failed jobs retry to
// their limit and a later scan requeues them.
func (i *Ingest) Fingerprint(ctx context.Context, job domain.Job) error {
	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	sprite, err := i.files.SeekSprite(job.ContentKey)
	if err != nil {
		return fmt.Errorf("cannot read the seek sprite for the fingerprint: %w", err)
	}
	sheets := make([][]byte, sprite.SheetCount)
	for sheet := range sheets {
		if sheets[sheet], err = i.files.SeekSpriteSheet(job.ContentKey, sheet); err != nil {
			return fmt.Errorf("cannot read seek sprite sheet %d for the fingerprint: %w", sheet, err)
		}
	}
	fingerprint, err := i.generator.SpriteFingerprint(sprite, sheets)
	if err != nil {
		return err
	}
	_, err = i.store.ApplyFingerprintForJob(ctx, job, fingerprint)
	return err
}
