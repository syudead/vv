package app

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

var thumbnailStart = domain.ThumbnailEditStart{Source: domain.ThumbnailEditFromThumbnail}

func (f *editsFixture) startFrom(t *testing.T, video domain.Video, start domain.ThumbnailEditStart) domain.ThumbnailEdit {
	t.Helper()
	edit, err := f.edits.Start(context.Background(), video, "/media/a.mp4", start)
	if err != nil {
		t.Fatal(err)
	}
	return edit
}

// A nil position identifies a saved image rather than a video frame.
func wantSource(t *testing.T, edit domain.ThumbnailEdit, source domain.ThumbnailEditSource, positionMs *int64) {
	t.Helper()
	wantState(t, edit, domain.ThumbnailEditReady)
	if edit.Source != source {
		t.Errorf("source = %q, want %q", edit.Source, source)
	}
	switch {
	case positionMs == nil && edit.PositionMs != nil:
		t.Errorf("positionMs = %d, want none", *edit.PositionMs)
	case positionMs != nil && (edit.PositionMs == nil || *edit.PositionMs != *positionMs):
		t.Errorf("positionMs = %v, want %d", edit.PositionMs, *positionMs)
	}
}

func ms(value int64) *int64 { return &value }

func TestThumbnailEditStartFromFrame(t *testing.T) {
	f := newEditsFixture(t)
	edit := f.startFrom(t, editVideo(1), frameStart(1_500))
	wantSource(t, edit, domain.ThumbnailEditFromFrame, ms(1_500))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{1_500}) {
		t.Fatalf("extracted at %v", got)
	}
}

// A missing frame position fails before extraction, like an out-of-range position.
func TestThumbnailEditStartFromFrameNeedsPosition(t *testing.T) {
	f := newEditsFixture(t)
	_, err := f.edits.Start(context.Background(), editVideo(1), "/a", domain.ThumbnailEditStart{Source: domain.ThumbnailEditFromFrame})
	if !errors.Is(err, domain.ErrThumbnailPositionOutOfRange) {
		t.Fatalf("err = %v", err)
	}
	if got := f.frames.extracted(); len(got) != 0 {
		t.Fatalf("extracted at %v", got)
	}
}

// Saved originals retain their dimensions and have no video position.
func TestThumbnailEditStartFromStoredSource(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	f.images.setPosition(video.ContentKey, ms(12_000))
	f.images.set(video.ContentKey, testPNG(t, 4, 3), testJPEG(t))

	edit := f.startFrom(t, video, thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, nil)
	if edit.FrameWidth != 64 || edit.FrameHeight != 48 {
		t.Fatalf("frame = %dx%d, want the stored source's 64x48", edit.FrameWidth, edit.FrameHeight)
	}
	if got := f.frames.extracted(); len(got) != 0 {
		t.Fatalf("extracted at %v despite the stored source", got)
	}
	frame, err := f.edits.Frame(context.Background(), edit.ID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(frame))
	if err != nil || config.Width != 64 || config.Height != 48 {
		t.Fatalf("frame route = %+v, %v; want a 64x48 PNG", config, err)
	}
}

// Legacy rows without originals fall back to the displayed thumbnail.
func TestThumbnailEditStartFromStoredImageWithoutSource(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	f.images.set(video.ContentKey, testPNG(t, 4, 3), nil)

	edit := f.startFrom(t, video, thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, nil)
	if edit.FrameWidth != 4 || edit.FrameHeight != 3 {
		t.Fatalf("frame = %dx%d, want the stored image's 4x3", edit.FrameWidth, edit.FrameHeight)
	}
}

// Unreadable images become thumbnail_frame_unavailable; storage errors propagate unchanged.
func TestThumbnailEditStartFromUnreadableStoredImage(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	f.images.set(video.ContentKey, []byte("x"), truncatedJPEG(t))
	if _, err := f.edits.Start(context.Background(), video, "/a", thumbnailStart); !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
		t.Fatalf("err = %v", err)
	}

	failure := errors.New("database is locked")
	f.images.err = failure
	if _, err := f.edits.Start(context.Background(), video, "/a", thumbnailStart); !errors.Is(err, failure) {
		t.Fatalf("err = %v", err)
	}
}

// Read the position and image from the same storage snapshot, ignoring stale video metadata.
func TestThumbnailEditStartFromRecordedPosition(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	video.ThumbnailPositionMs = ms(3_000)
	f.images.setPosition(video.ContentKey, ms(12_000))

	edit := f.startFrom(t, video, thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, ms(12_000))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{12_000}) {
		t.Fatalf("extracted at %v", got)
	}
}

// Use the thumbnail job fallback order: automatic position, then frame zero.
func TestThumbnailEditStartFromAutomaticPosition(t *testing.T) {
	f := newEditsFixture(t)
	edit := f.startFrom(t, editVideo(1), thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, ms(6_000))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{6_000}) {
		t.Fatalf("extracted at %v", got)
	}

	f = newEditsFixture(t)
	f.frames.missing = map[int64]bool{6_000: true}
	edit = f.startFrom(t, editVideo(1), thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, ms(0))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{6_000, 0}) {
		t.Fatalf("extracted at %v", got)
	}

	f = newEditsFixture(t)
	f.frames.missing = map[int64]bool{6_000: true, 0: true}
	if _, err := f.edits.Start(context.Background(), editVideo(1), "/a", thumbnailStart); !errors.Is(err, domain.ErrThumbnailFrameUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

// A session retains its source image even after the thumbnail is reverted.
func TestThumbnailEditKeepsItsImageAfterRevert(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	f.images.set(video.ContentKey, testPNG(t, 4, 3), testJPEG(t))
	edit := f.startFrom(t, video, thumbnailStart)
	before, err := f.edits.Frame(context.Background(), edit.ID)
	if err != nil {
		t.Fatal(err)
	}

	f.images.set(video.ContentKey, nil, nil)

	after, err := f.edits.Frame(context.Background(), edit.ID)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("frame changed after the revert: %v", err)
	}
	f.run(t, edit.ID)
	if len(f.editor.starts) != 1 || !bytes.Equal(f.editor.starts[0].Frame, before) {
		t.Fatal("the run did not send the image the session started with")
	}
	if got := f.frames.extracted(); len(got) != 0 {
		t.Fatalf("extracted at %v after the revert", got)
	}
}

// An image the page sends becomes the session's frame without a media file; an unreadable one starts nothing.
func TestThumbnailEditStartFromImage(t *testing.T) {
	f := newEditsFixture(t)
	ctx := context.Background()
	sent := testPNG(t, 6, 5)
	edit, err := f.edits.Start(ctx, editVideo(1), "", domain.ThumbnailEditStart{
		Source: domain.ThumbnailEditFromImage, Image: sent,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSource(t, edit, domain.ThumbnailEditFromImage, nil)
	frame, err := f.edits.Frame(ctx, edit.ID)
	config, decodeErr := png.DecodeConfig(bytes.NewReader(frame))
	if err != nil || decodeErr != nil || config.Width != 6 || config.Height != 5 {
		t.Fatalf("frame = %+v, %v, %v", config, err, decodeErr)
	}

	_, err = f.edits.Start(ctx, editVideo(1), "", domain.ThumbnailEditStart{
		Source: domain.ThumbnailEditFromImage, Image: []byte("not an image"),
	})
	if !errors.Is(err, domain.ErrThumbnailUploadInvalid) {
		t.Fatalf("unreadable image: %v", err)
	}
	if _, err := f.edits.Get(ctx, edit.ID); err != nil {
		t.Fatalf("a refused start ended the session: %v", err)
	}
}
