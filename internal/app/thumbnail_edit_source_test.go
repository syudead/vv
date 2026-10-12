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

// 編集を始める元（specs/046-thumbnail-image-upload/contracts/screen-api.md、research.md R-4）を、
// 偽のコマの取り出しと偽の適用した画像で確かめる。

var thumbnailStart = domain.ThumbnailEditStart{Source: domain.ThumbnailEditFromThumbnail}

func (f *editsFixture) startFrom(t *testing.T, video domain.Video, start domain.ThumbnailEditStart) domain.ThumbnailEdit {
	t.Helper()
	edit, err := f.edits.Start(context.Background(), video, "/media/a.mp4", start)
	if err != nil {
		t.Fatal(err)
	}
	return edit
}

// wantSource は始めたセッションの元と、コマを取った位置（nil は保存した画像）を確かめる。
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

// コマから始めると、その位置で取り出してその位置を返す。
func TestThumbnailEditStartFromFrame(t *testing.T) {
	f := newEditsFixture(t)
	edit := f.startFrom(t, editVideo(1), frameStart(1_500))
	wantSource(t, edit, domain.ThumbnailEditFromFrame, ms(1_500))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{1_500}) {
		t.Fatalf("extracted at %v", got)
	}
}

// コマから始めるのに位置が無ければ、尺の外と同じ誤りで、コマを取りに行かない。
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

// 画像を適用していれば、保存した元の画像を PNG にして、その画素の大きさで始め、位置は持たない。
// コマのルートはその PNG を返す。
func TestThumbnailEditStartFromStoredSource(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	video.ThumbnailPositionMs = ms(12_000)
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

// 元の画像を持たない古い行では、代表サムネイルから始める。
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

// 保存した画像を読めなければ thumbnail_frame_unavailable で、読み出しの誤りはそのまま返す。
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

// 画像を適用しておらず位置を指定していれば、その位置で取り出してその位置を返す。
func TestThumbnailEditStartFromRecordedPosition(t *testing.T) {
	f := newEditsFixture(t)
	video := editVideo(1)
	video.ThumbnailPositionMs = ms(12_000)

	edit := f.startFrom(t, video, thumbnailStart)
	wantSource(t, edit, domain.ThumbnailEditFromThumbnail, ms(12_000))
	if got := f.frames.extracted(); !slices.Equal(got, []int64{12_000}) {
		t.Fatalf("extracted at %v", got)
	}
}

// どちらも無ければサムネイルジョブと同じ自動の位置で取り出し、そこで取れなければ先頭のコマを
// 取り出して位置 0 を返す。どちらでも取れなければ thumbnail_frame_unavailable である。
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

// 始めたあとに元に戻しても、セッションの画像は変わらず、実行もその画像を送る。
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
