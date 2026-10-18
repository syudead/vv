package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (f *guestFixture) applyMediaInfo(t *testing.T, name string, info domain.MediaInfo) {
	t.Helper()
	probe := domain.Probe{DurationMs: 60_000, Width: 1280, Height: 720, VideoCodec: "h264", AudioCodec: "aac", Media: &info}
	if err := f.env.db.Ingest().ApplyProbe(context.Background(), f.ids[name], probe, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
}

func sampleMediaInfo() domain.MediaInfo {
	interlaced := true
	altitude := 40.0
	return domain.MediaInfo{
		Video: &domain.MediaVideo{FrameRate: 29.97, BitDepth: 10, BitRate: 18_200_000, Interlaced: &interlaced, HDR: domain.HDRKindHLG},
		AudioTracks: []domain.MediaAudioTrack{
			{Index: 1, Codec: "aac", Channels: 6, ChannelLayout: "5.1", Language: "jpn", Default: true},
			{Index: 2, Codec: "aac", Channels: 2, ChannelLayout: "stereo", Language: "eng", Title: "Commentary"},
		},
		SubtitleTracks: []domain.MediaSubtitleTrack{{Index: 3, Codec: "subrip", Language: "jpn", Forced: true}},
		Embedded: &domain.MediaEmbedded{
			Title: "埋め込みの題名", RecordedAt: "2026-09-27T15:04:05+09:00", Device: "Apple iPhone 15",
			Location: &domain.GeoLocation{Latitude: 35.6895, Longitude: 139.6917, Altitude: &altitude},
			CoverArt: true,
		},
		Chapters:  []domain.MediaChapter{{StartMs: 0, EndMs: 30_000, Title: "Intro"}, {StartMs: 30_000, EndMs: 60_000}},
		Broadcast: &domain.MediaBroadcast{Station: "テスト放送", Program: "ニュース"},
	}
}

func TestGetVideoMediaInfo(t *testing.T) {
	f := newGuestFixture(t, true)
	f.applyMediaInfo(t, "a", sampleMediaInfo())
	f.applyMediaInfo(t, "c", sampleMediaInfo())

	owner := f.env.get(f.videoPath("a", ""), f.owner)
	guest := f.env.get(f.videoPath("a", ""))
	if owner.Code != http.StatusOK || guest.Code != http.StatusOK {
		t.Fatalf("詳細: owner = %d, guest = %d", owner.Code, guest.Code)
	}
	ownerInfo, guestInfo := rawVideo(t, owner)["mediaInfo"], rawVideo(t, guest)["mediaInfo"]
	if len(ownerInfo) == 0 {
		t.Fatalf("所有者の詳細に mediaInfo が無い: %s", owner.Body)
	}
	if string(ownerInfo) != string(guestInfo) {
		t.Errorf("ゲストの mediaInfo = %s, want 所有者と同じ %s", guestInfo, ownerInfo)
	}
	var got gen.MediaInfo
	if err := json.Unmarshal(ownerInfo, &got); err != nil {
		t.Fatal(err)
	}
	sample := sampleMediaInfo()
	if want := toAPIMediaInfo(&sample); !reflect.DeepEqual(got, *want) {
		t.Errorf("mediaInfo = %s, want %+v", ownerInfo, *want)
	}
	if got.Broadcast == nil || got.Broadcast.Station == nil || *got.Broadcast.Station != "テスト放送" ||
		got.Broadcast.Program == nil || *got.Broadcast.Program != "ニュース" {
		t.Errorf("broadcast = %+v, want 局名「テスト放送」と番組名「ニュース」: %s", got.Broadcast, ownerInfo)
	}

	assertStatus(t, "ゲストの非公開動画", f.env.get(f.videoPath("c", "")), http.StatusNotFound)
	if value, ok := rawVideo(t, f.env.get(f.videoPath("b", ""), f.owner))["mediaInfo"]; ok {
		t.Errorf("行の無い動画の mediaInfo = %s", value)
	}

	for _, cookies := range [][]*http.Cookie{{f.owner}, nil} {
		rec := f.env.get("/api/videos", cookies...)
		if rec.Code != http.StatusOK {
			t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
		}
		for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
			if value, ok := item["mediaInfo"]; ok {
				t.Errorf("一覧の項目に mediaInfo = %s", value)
			}
		}
	}
	put := f.putVideo("a", "/work-info", `{}`, f.owner)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT: status = %d: %s", put.Code, put.Body)
	}
	if value, ok := rawVideo(t, put)["mediaInfo"]; ok {
		t.Errorf("所有者の PUT の応答に mediaInfo = %s", value)
	}
}

func TestToAPIMediaInfo(t *testing.T) {
	if got := toAPIMediaInfo(&domain.MediaInfo{}); got != nil {
		t.Errorf("項目の無い MediaInfo = %+v, want nil", got)
	}
	if got := toAPIMediaInfo(&domain.MediaInfo{Embedded: &domain.MediaEmbedded{RecordedAt: "読めない"}}); got != nil {
		t.Errorf("読めない日時だけの MediaInfo = %+v, want nil", got)
	}
	got := toAPIMediaInfo(&domain.MediaInfo{Video: &domain.MediaVideo{}, Embedded: &domain.MediaEmbedded{RecordedAt: "2026-09-27T15:04:05+09:00"}})
	if got == nil || got.Video == nil || got.Video.Hdr != gen.Sdr || got.Video.FrameRate != nil {
		t.Fatalf("映像 = %+v, want hdr = sdr だけ", got)
	}
	encoded, err := json.Marshal(got.Embedded.RecordedAt)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `"2026-09-27T15:04:05+09:00"` {
		t.Errorf("recordedAt = %s", encoded)
	}
	if got.AudioTracks != nil || got.SubtitleTracks != nil || got.Chapters != nil || got.Embedded.Location != nil || got.Broadcast != nil {
		t.Errorf("無い項目が入った: %+v", got)
	}
}

func TestToAPIMediaInfoBroadcast(t *testing.T) {
	if got := toAPIMediaInfo(&domain.MediaInfo{Broadcast: &domain.MediaBroadcast{}}); got != nil {
		t.Errorf("空の放送の情報だけの MediaInfo = %+v, want nil", got)
	}
	got := toAPIMediaInfo(&domain.MediaInfo{Broadcast: &domain.MediaBroadcast{Station: "テスト放送"}})
	if got == nil || got.Broadcast == nil || got.Broadcast.Station == nil || *got.Broadcast.Station != "テスト放送" {
		t.Fatalf("放送の情報だけの MediaInfo = %+v, want 局名", got)
	}
	if got.Broadcast.Program != nil {
		t.Errorf("番組名 = %q, want 省く", *got.Broadcast.Program)
	}
}
