package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

type receivedThumbnail struct {
	videoID int64
	key     string
	image   []byte
}

// A thumbnail edit of the peer's video starts from the peer's frame, runs on this node's ComfyUI, and its apply
// arrives on the peer as a thumbnail upload.
func TestRemoteThumbnailEditRunsHereAndUploadsToThePeer(t *testing.T) {
	received := map[*store.DB][]receivedThumbnail{}
	fa, fb, _, b, _ := mergedNodesWith(t, func(db *store.DB, opts *Options) {
		library := db.Library()
		opts.ThumbnailUploads = thumbnailUploadFunc(func(ctx context.Context, id int64, key string, image []byte) (domain.Video, error) {
			received[db] = append(received[db], receivedThumbnail{videoID: id, key: key, image: image})
			return library.GetVideo(ctx, domain.AudienceOwner, id)
		})
	})
	// The peer's frames are larger than this node's, so the session's frame shows which node extracted it.
	fa.edits.set(func(e *editFakes) { e.frameSize = 6 })
	fb.saveConnection(t)
	remote := domain.ToRemoteID(fa.ids["a"])

	rec := b.owner(http.MethodPost, fmt.Sprintf("/api/videos/%d/thumbnail-edits", remote), `{"source":"frame","positionMs":1000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start: status = %d: %s", rec.Code, rec.Body)
	}
	edit := decode[gen.ThumbnailEdit](t, rec)
	if edit.VideoId != remote || edit.FrameUrl != "/api/thumbnail-edits/"+edit.Id+"/frame" {
		t.Fatalf("start = video %d, frameUrl %q, want video %d and this node's frame URL", edit.VideoId, edit.FrameUrl, remote)
	}
	frame := fb.editRequest(http.MethodGet, edit.Id, "/frame", fb.owner)
	config, err := png.DecodeConfig(bytes.NewReader(frame.Body.Bytes()))
	if frame.Code != http.StatusOK || err != nil || config.Width != 6 {
		t.Fatalf("frame here: status = %d, width = %d, %v, want 200 and the peer's 6 px frame", frame.Code, config.Width, err)
	}
	var onA, onB []int64
	fa.edits.set(func(e *editFakes) { onA = slices.Clone(e.positions) })
	fb.edits.set(func(e *editFakes) { onB = slices.Clone(e.positions) })
	if !slices.Equal(onA, []int64{1000}) || len(onB) != 0 {
		t.Errorf("frames read: a = %v, b = %v, want 1000 on a only", onA, onB)
	}

	if rec := fb.runEdit(t, edit.Id, map[string][]byte{"prompt": []byte("remove the caption")}, fb.owner); rec.Code != http.StatusAccepted {
		t.Fatalf("run: status = %d: %s", rec.Code, rec.Body)
	}
	fb.edits.set(func(e *editFakes) { e.state = domain.ThumbnailEditSucceeded })
	fb.editRequest(http.MethodGet, edit.Id, "", fb.owner)
	rec = fb.editRequest(http.MethodPost, edit.Id, "/apply", fb.owner)
	if video := decode[gen.Video](t, rec); rec.Code != http.StatusOK || video.Id != remote {
		t.Fatalf("apply: status = %d, video %d: %s, want 200 and video %d", rec.Code, video.Id, rec.Body, remote)
	}

	if starts, _ := fa.edits.counts(); starts != 0 {
		t.Errorf("the peer's ComfyUI ran %d times", starts)
	}
	want := []receivedThumbnail{{videoID: fa.ids["a"], key: "content-key-a", image: editPNG(4, 4)}}
	if got := received[fa.env.db]; len(got) != 1 || got[0].videoID != want[0].videoID || got[0].key != want[0].key ||
		!bytes.Equal(got[0].image, want[0].image) || len(received[fb.env.db]) != 0 {
		t.Errorf("uploads: a = %v, b = %v, want this node's result on a's video a only", got, received[fb.env.db])
	}
}
