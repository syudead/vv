package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	var pairing *app.Pairing
	f = newGuestFixtureWith(t, true, func(db *store.DB, opts *Options) {
		pairing = withTestPairing(db, "node-a", nil, opts)
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
	// The vocabulary is not joined by name yet, so the peer's tag takes the offset.
	if len(video.Tags) != 1 || video.Tags[0].Id != domain.ToRemoteID(f.tagID) || video.Tags[0].Name != guestSecretTag {
		t.Errorf("tags = %+v, want id %d の %s", video.Tags, domain.ToRemoteID(f.tagID), guestSecretTag)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"progress", "favorite"} {
		if value, ok := raw[field]; ok {
			t.Errorf("リモートの動画に %s = %s", field, value)
		}
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

	for _, tc := range []struct {
		label string
		req   authRequest
	}{
		{"open", authRequest{method: http.MethodPost, target: f.remotePath("a", "/open")}},
		{"progress", authRequest{
			method: http.MethodPut, target: f.remotePath("a", "/progress"), body: `{"positionMs":1000,"durationMs":60000}`,
		}},
	} {
		rec := b.send(tc.req)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}
	if local := decode[gen.Video](t, f.env.get(f.videoPath("a", ""), f.owner)); local.Progress == nil ||
		local.Progress.PositionMs != 30_000 {
		t.Errorf("持ち主の再生位置が変わった: %+v", local.Progress)
	}

	// A remote id in the peer's own request names this node's video; it is not sent back.
	bounced := b.env.serve(authRequest{
		method: http.MethodGet, target: f.remotePath("a", ""),
		header: map[string]string{"Authorization": "VV-Peer " + a.outgoingSecret(t)},
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
