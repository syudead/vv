package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// remoteVideoNodes pairs b with a, whose catalog is the guest fixture, so b reaches a's videos by remote id.
// stopA stops a's server.
func remoteVideoNodes(t *testing.T) (f *guestFixture, a, b *peerNode, stopA func()) {
	t.Helper()
	return remoteVideoNodesWith(t, nil)
}

// remoteVideoNodesWith is remoteVideoNodes with a's options extended.
func remoteVideoNodesWith(t *testing.T, extend func(opts *Options)) (f *guestFixture, a, b *peerNode, stopA func()) {
	t.Helper()
	var pairing *app.Pairing
	f = newGuestFixtureWith(t, true, func(db *store.DB, opts *Options) {
		pairing = withTestPairing(db, "node-a", nil, opts)
		if extend != nil {
			extend(opts)
		}
	})
	server := httptest.NewServer(f.env.handler)
	t.Cleanup(server.Close)
	a = &peerNode{env: f.env, url: server.URL, cookie: f.owner, pairing: pairing}
	b = newPeerNode(t, "node-b")
	pairNodes(t, a, b)
	return f, a, b, server.Close
}

func (f *guestFixture) remotePath(name, suffix string) string {
	return "/api/videos/" + strconv.FormatInt(domain.ToRemoteID(f.ids[name]), 10) + suffix
}

// settleRecorder records the tentative tags a node confirms or rejects.
type settleRecorder struct {
	AutoTagging
	mu    sync.Mutex
	calls []string
}

func (f *settleRecorder) ConfirmTag(_ context.Context, videoID, tagID int64) error {
	return f.record("confirm", videoID, tagID)
}

func (f *settleRecorder) RejectTag(_ context.Context, videoID, tagID int64) error {
	return f.record("reject", videoID, tagID)
}

func (f *settleRecorder) record(action string, videoID, tagID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf("%s %d %d", action, videoID, tagID))
	return nil
}

func (f *settleRecorder) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// A tentative tag of a remote video is settled on the peer under the peer's tag id: a local tag by its name, a
// remote-only tag without the offset, and a tag the peer lacks not at all.
func TestRemoteTentativeTagIsSettledOnThePeerInItsIDs(t *testing.T) {
	var settled []*settleRecorder
	fa, fb, _, b, _ := mergedNodesWith(t, func(_ *store.DB, opts *Options) {
		recorder := &settleRecorder{}
		settled = append(settled, recorder)
		opts.AutoTagging = recorder
	})
	// b creates 鳥 first, so the two nodes hold 猫 under different ids.
	birdB, catB := tagOn(t, fb, "鳥"), tagOn(t, fb, "猫")
	catA, dogA := tagOn(t, fa, "猫"), tagOn(t, fa, "犬")
	if catA.ID == catB.ID {
		t.Fatalf("precondition: 猫 has id %d on both nodes", catA.ID)
	}
	tagPath := func(tagID int64, action string) string {
		return fa.remotePath("a", "/tags/"+strconv.FormatInt(tagID, 10)+"/"+action)
	}

	for _, tc := range []struct {
		path string
		want int
	}{
		{tagPath(catB.ID, "confirm"), http.StatusNoContent},
		{tagPath(domain.ToRemoteID(dogA.ID), "reject"), http.StatusNoContent},
		{tagPath(birdB.ID, "confirm"), http.StatusNotFound},
	} {
		if rec := b.owner(http.MethodPost, tc.path, ""); rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d: %s", tc.path, rec.Code, tc.want, rec.Body)
		}
	}
	video := fa.ids["a"]
	want := []string{fmt.Sprintf("confirm %d %d", video, catA.ID), fmt.Sprintf("reject %d %d", video, dogA.ID)}
	if got := settled[0].recorded(); !slices.Equal(got, want) {
		t.Errorf("settled on the peer = %v, want %v", got, want)
	}
	if got := settled[1].recorded(); len(got) != 0 {
		t.Errorf("settled on this node = %v, want none", got)
	}
}

func (n *peerNode) send(req authRequest) *httptest.ResponseRecorder {
	n.env.t.Helper()
	req.cookies = []*http.Cookie{n.cookie}
	return n.env.serve(req)
}

func TestRemoteVideoIsForwardedWithRemoteIDsAndURLs(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)
	if local := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner)); local.Progress == nil {
		t.Fatal("前提: 持ち主の詳細に再生位置が無い")
	}

	rec := b.owner(http.MethodGet, f.remotePath("a", ""), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("リモートの詳細: status = %d: %s", rec.Code, rec.Body)
	}
	video := decode[gen.Video](t, rec)
	remote := domain.ToRemoteID(f.ids["a"])
	if video.Id != remote || video.Title != "a" {
		t.Errorf("id = %d, title = %q, want %d と a", video.Id, video.Title, remote)
	}
	prefix := "/api/videos/" + strconv.FormatInt(remote, 10) + "/"
	for label, url := range map[string]*string{
		"thumbnailUrl": video.ThumbnailUrl, "previewUrl": video.PreviewUrl, "seekThumbnailUrl": video.SeekThumbnailUrl,
	} {
		if url == nil {
			t.Errorf("%s が無い", label)
		} else if !strings.HasPrefix(*url, prefix) {
			t.Errorf("%s = %s, want %s で始まる", label, *url, prefix)
		}
	}
	if video.Folder == nil || video.Folder.RootId != domain.ToRemoteID(f.rootID) {
		t.Errorf("folder = %+v, want rootId %d", video.Folder, domain.ToRemoteID(f.rootID))
	}
	// b holds no tag of that name, so the peer's tag takes the offset.
	if len(video.Tags) != 1 || video.Tags[0].Id != domain.ToRemoteID(f.tagID) || video.Tags[0].Name != guestSecretTag {
		t.Errorf("tags = %+v, want id %d の %s", video.Tags, domain.ToRemoteID(f.tagID), guestSecretTag)
	}
	// The position and favorite live on the peer, which holds the video.
	if video.Progress == nil || video.Progress.PositionMs != 30_000 || video.Favorite == nil {
		t.Errorf("progress = %+v, favorite = %v, want the peer's 30000 ms and its favorite", video.Progress, video.Favorite)
	}
}

// The people tab reads a remote video's face appearances from the peer, not from this node's store.
func TestRemoteVideoFaceAppearancesAreReadOnThePeer(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	rec := b.owner(http.MethodGet, f.remotePath("a", "/face-appearances"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoFaceAppearanceList](t, rec); got.Analysed || len(got.Items) != 0 {
		t.Errorf("body = %+v, want the peer's unanalysed video", got)
	}
}

func TestRemoteVideoMediaPassesThrough(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	stream := b.send(authRequest{
		method: http.MethodGet, target: f.remotePath("a", "/stream"), header: map[string]string{"Range": "bytes=0-99"},
	})
	if stream.Code != http.StatusPartialContent || stream.Body.String() != strings.Repeat("a", 100) {
		t.Errorf("Range 付きの stream: status = %d, body = %q", stream.Code, stream.Body.String())
	}
	if got := stream.Header().Get("Content-Range"); got != "bytes 0-99/4096" {
		t.Errorf("Content-Range = %q, want bytes 0-99/4096", got)
	}
	if got := stream.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Errorf("stream の CSP = %q, want 中継応答を閉じ込める CSP", got)
	}

	subtitles := decode[gen.SubtitleTrackList](t, b.owner(http.MethodGet, f.remotePath("a", "/subtitles"), ""))
	if len(subtitles.Subtitles) != 1 || subtitles.Subtitles[0].File != "a.srt" {
		t.Fatalf("字幕の一覧 = %+v, want a.srt", subtitles.Subtitles)
	}
	vtt := b.owner(http.MethodGet, f.remotePath("a", "/subtitles/a.srt"), "")
	if vtt.Code != http.StatusOK || vtt.Header().Get("Content-Type") != "text/vtt; charset=utf-8" ||
		!strings.Contains(vtt.Body.String(), "00:00:01.000 --> 00:00:02.000\na") {
		t.Errorf("字幕: status = %d, Content-Type = %q, body = %q", vtt.Code, vtt.Header().Get("Content-Type"), vtt.Body)
	}

	transcode := b.owner(http.MethodGet, f.remotePath("a", "/transcode.mp4"), "")
	if transcode.Code != http.StatusOK || transcode.Body.String() != "fragmented-mp4" ||
		transcode.Header().Get("Content-Type") != "video/mp4" {
		t.Errorf("transcode.mp4: status = %d, Content-Type = %q, body = %q",
			transcode.Code, transcode.Header().Get("Content-Type"), transcode.Body)
	}
}

func TestRemoteVideoFollowsTheViewersAudience(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	private := b.env.get(f.remotePath("c", ""))
	assertErrorBody(t, "ゲストの非公開のリモート動画", private.Code, private.Body.Bytes(), wantError{
		status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound,
	})
	public := b.env.get(f.remotePath("a", ""))
	assertAudience(t, "ゲストの公開のリモート動画", public, "guest")
	if public.Code != http.StatusOK {
		t.Fatalf("ゲストの公開のリモート動画: status = %d: %s", public.Code, public.Body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(public.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの公開のリモート動画", raw)
}

func TestRemoteVideoRefusesOperationsKeptOnEachNode(t *testing.T) {
	f, a, b, _ := remoteVideoNodes(t)

	rec := b.send(authRequest{method: http.MethodPost, target: f.remotePath("a", "/open")})
	assertErrorBody(t, "open", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})

	// With A reading B as well (a second pairing), a remote id in A's own request names A's video on B; it is
	// not sent back.
	pairNodes(t, b, a)
	bounced := b.env.serve(authRequest{
		method: http.MethodGet, target: f.remotePath("a", ""),
		header: map[string]string{"Authorization": "VV-Peer " + a.linkSecret(t)},
	})
	assertStatus(t, "相手の要求のリモート ID", bounced, http.StatusNotFound)
}

func TestRemoteVideoDisplayNameIsWrittenOnThePeer(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	rec := b.owner(http.MethodPut, f.remotePath("a", "/display-name"), displayNameBody("遠くの名前"))
	if rec.Code != http.StatusOK {
		t.Fatalf("表示名: status = %d: %s", rec.Code, rec.Body)
	}
	if saved := decode[gen.Video](t, rec); saved.Id != domain.ToRemoteID(f.ids["a"]) || saved.Title != "遠くの名前" {
		t.Errorf("表示名の応答: id = %d, title = %q", saved.Id, saved.Title)
	}
	if local := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner)); local.Title != "遠くの名前" {
		t.Errorf("持ち主の題名 = %q, want 遠くの名前", local.Title)
	}

	bad := b.owner(http.MethodPut, f.remotePath("a", "/display-name"), displayNameBody("a\nb"))
	assertErrorBody(t, "制御文字の表示名", bad.Code, bad.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonDisplayNameControlCharacters,
	})
}

func TestRemoteVideoWithAStoppedPeerIsUnavailable(t *testing.T) {
	f, _, b, stopA := remoteVideoNodes(t)
	stopA()

	unavailable := wantError{
		status: http.StatusServiceUnavailable, code: gen.ErrorCodeServiceUnavailable, reason: gen.ErrorReasonPeerUnreachable,
	}
	rec := b.owner(http.MethodGet, f.remotePath("a", ""), "")
	assertErrorBody(t, "止まった相手", rec.Code, rec.Body.Bytes(), unavailable)
	if state, _ := decodePeerState(t, b.owner(http.MethodGet, "/api/peer", "")); state.State != gen.PeerStateStateUnreachable {
		t.Errorf("失敗の後の状態 = %s, want unreachable", state.State)
	}
	// While unreachable, the request is refused without trying the peer.
	again := b.owner(http.MethodGet, f.remotePath("a", "/thumbnail"), "")
	assertErrorBody(t, "届かない間のサムネイル", again.Code, again.Body.Bytes(), unavailable)
}

func TestRemoteVideoHeadAnswersWithoutABody(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)

	rec := b.owner(http.MethodHead, f.remotePath("a", ""), "")
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("リモートの詳細の HEAD: status = %d, body = %q", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Content-Type"); got != contentTypeJSON {
		t.Errorf("Content-Type = %q, want %s", got, contentTypeJSON)
	}
}

// recordingPeerLink answers every forwarded request with one video and keeps whether it had a response timeout.
type recordingPeerLink struct {
	media []bool
}

func (l *recordingPeerLink) Forward(req *http.Request, _, media bool) (*http.Response, error) {
	l.media = append(l.media, media)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {contentTypeJSON}},
		Body:       io.NopCloser(strings.NewReader(`{"id":5,"title":"x"}`)),
		Request:    req,
	}, nil
}

func TestRemoteThumbnailPositionWaitsForThePeersExtraction(t *testing.T) {
	link := &recordingPeerLink{}
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options {
		return Options{Videos: &audienceLibrary{fakeLibrary: sampleLibrary()}, PeerLink: link}
	})
	cookie := env.setup()

	rec := env.serve(authRequest{
		method: http.MethodPut, target: "/api/videos/" + strconv.FormatInt(domain.ToRemoteID(5), 10) + "/thumbnail-position",
		body: `{"positionMs":1000}`, cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("サムネイル位置: status = %d: %s", rec.Code, rec.Body)
	}
	// The peer extracts the frame before it answers, which may take longer than a call's response timeout.
	if len(link.media) != 1 || !link.media[0] {
		t.Errorf("転送の media = %v, want [true]（応答の時間制限なし）", link.media)
	}
}

// The forwarding wiring is checked by the CSP assertion of TestRemoteVideoMediaPassesThrough.
func TestPeerContentCannotRunAsAPageOfThisNode(t *testing.T) {
	for _, tc := range []struct{ peerType, wantType, wantDisposition string }{
		{"text/html; charset=utf-8", "application/octet-stream", "attachment"},
		{"video/mp4", "video/mp4", ""},
	} {
		header := http.Header{"Content-Type": {tc.peerType}}
		confinePeerContent(header)
		if header.Get("Content-Type") != tc.wantType || header.Get("Content-Disposition") != tc.wantDisposition {
			t.Errorf("%s: Content-Type = %q, Content-Disposition = %q, want %s と %q",
				tc.peerType, header.Get("Content-Type"), header.Get("Content-Disposition"), tc.wantType, tc.wantDisposition)
		}
		if header.Get("X-Content-Type-Options") != "nosniff" || header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
			t.Errorf("%s: nosniff = %q, CSP = %q", tc.peerType, header.Get("X-Content-Type-Options"), header.Get("Content-Security-Policy"))
		}
	}
}

// The assistant's read of a remote video reaches the peer as the screens' request does.
func TestAssistantReadsARemoteVideoFromThePeer(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)
	remote := domain.ToRemoteID(f.ids["a"])

	result, err := b.env.runner.Read(context.Background(), domain.AssistantRead{
		Operation: "getVideo", Arguments: json.RawMessage(fmt.Sprintf(`{"id":%d}`, remote)),
	})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var video gen.Video
	if err := json.Unmarshal(result.Body, &video); err != nil || video.Id != remote || video.Title != "a" {
		t.Errorf("video = %s (%v), want id %d の a", result.Body, err, remote)
	}
}

// An operation no list names is forwarded by default and its answer rewritten by its type: switching the
// representative of the peer's bundle answers the versions in remote ids.
func TestRemoteVideoMakeRepresentativeAnswersRemoteVersions(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)
	if rec := f.bundle(bundleBody([]int64{f.ids["a"], f.ids["b"]}, f.ids["a"]), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("precondition: bundle on the peer: status = %d: %s", rec.Code, rec.Body)
	}

	rec := b.owner(http.MethodPost, f.remotePath("b", "/make-representative"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("make-representative: status = %d: %s", rec.Code, rec.Body)
	}
	versions := decode[gen.VideoVersions](t, rec)
	remoteA, remoteB := domain.ToRemoteID(f.ids["a"]), domain.ToRemoteID(f.ids["b"])
	if got := versionIDs(versions); versions.RepresentativeId != remoteB || !slices.Equal(slices.Sorted(slices.Values(got)), []int64{remoteA, remoteB}) {
		t.Errorf("versions = representative %d, items %v, want %d and [%d %d]", versions.RepresentativeId, got, remoteB, remoteA, remoteB)
	}
	if local := decode[gen.VideoVersions](t, f.env.get(f.videoPath("a", "/versions"), f.owner)); local.RepresentativeId != f.ids["b"] {
		t.Errorf("the peer's representative = %d, want %d", local.RepresentativeId, f.ids["b"])
	}
}

// Taking a version out of the peer's bundle happens on the peer, and the removed video answers in its remote id.
func TestRemoteVideoUnbundleTakesTheVersionOutOnThePeer(t *testing.T) {
	f, _, b, _ := remoteVideoNodes(t)
	if rec := f.bundle(bundleBody([]int64{f.ids["a"], f.ids["b"]}, f.ids["a"]), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("precondition: bundle on the peer: status = %d: %s", rec.Code, rec.Body)
	}

	rec := b.owner(http.MethodPost, f.remotePath("b", "/unbundle"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unbundle: status = %d: %s", rec.Code, rec.Body)
	}
	if removed := decode[gen.Video](t, rec); removed.Id != domain.ToRemoteID(f.ids["b"]) || removed.Versions != nil {
		t.Errorf("removed = id %d, versions %+v, want id %d without versions", removed.Id, removed.Versions, domain.ToRemoteID(f.ids["b"]))
	}
	if peer := decode[gen.Video](t, f.env.get(f.videoPath("b", ""), f.owner)); peer.Versions != nil {
		t.Errorf("the peer's b still has versions %+v", peer.Versions)
	}
}

func TestRemoteVideoIsDeletedOnThePeer(t *testing.T) {
	virtual := &fakeVirtualVideos{}
	f, _, b, _ := remoteVideoNodesWith(t, func(opts *Options) { opts.VirtualVideos = virtual })

	rec := b.owner(http.MethodDelete, f.remotePath("a", "?contentVersion=v1"), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d: %s", rec.Code, rec.Body)
	}
	want := []virtualCall{{op: "delete", videoID: f.ids["a"], contentVersion: "v1"}}
	if !reflect.DeepEqual(virtual.calls, want) {
		t.Errorf("the peer's deletes = %+v, want %+v", virtual.calls, want)
	}
}
