package httpapi

import (
	"context"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画の追加の情報を、動画 1 件の詳細（GET /api/videos/{id}）に載せる部分
// （specs/056-media-info/contracts/screen-api.md「GET /api/videos/{id}: mediaInfo」）。

// MediaInfoReader は動画の追加の情報の読み出し先である。
type MediaInfoReader interface {
	// MediaInfo は動画 videoID の追加の情報を返す。今の版の行が無ければ nil である。
	MediaInfo(ctx context.Context, videoID int64) (*domain.MediaInfo, error)
}

// mediaInfoForDetail は動画 1 件の詳細に載せる追加の情報を video に埋める。所有者とゲストを
// 分けない（specs/056-media-info/research.md R-6）。一覧・関連動画・ライブラリの項目・所有者の
// PUT の応答は呼ばないので、そこには載らない。
func (s *server) mediaInfoForDetail(ctx context.Context, video domain.Video) (domain.Video, error) {
	if s.mediaInfo == nil {
		return video, nil
	}
	info, err := s.mediaInfo.MediaInfo(ctx, video.ID)
	if err != nil {
		return domain.Video{}, err
	}
	video.MediaInfo = info
	return video, nil
}

// toAPIMediaInfo は追加の情報を契約の MediaInfo にする。nil、または項目が 1 つも無いときは
// nil を返し、応答から丸ごと省く。
func toAPIMediaInfo(info *domain.MediaInfo) *gen.MediaInfo {
	if info == nil {
		return nil
	}
	out := gen.MediaInfo{
		Video:          toAPIMediaVideo(info.Video),
		AudioTracks:    toAPIAudioTracks(info.AudioTracks),
		SubtitleTracks: toAPISubtitleTracks(info.SubtitleTracks),
		Embedded:       toAPIMediaEmbedded(info.Embedded),
		Chapters:       toAPIChapters(info.Chapters),
	}
	if out == (gen.MediaInfo{}) {
		return nil
	}
	return &out
}

func toAPIMediaVideo(video *domain.MediaVideo) *gen.MediaInfoVideo {
	if video == nil {
		return nil
	}
	hdr := video.HDR
	if hdr == "" {
		hdr = domain.HDRKindSDR
	}
	return &gen.MediaInfoVideo{
		FrameRate:      optionalPositive(video.FrameRate),
		BitDepth:       optionalPositive(video.BitDepth),
		BitRate:        optionalPositive(video.BitRate),
		Interlaced:     video.Interlaced,
		Hdr:            gen.MediaInfoVideoHdr(hdr),
		Projection:     optionalString(video.Projection),
		Stereo3d:       optionalString(video.Stereo3D),
		ClosedCaptions: optionalTrue(video.ClosedCaptions),
	}
}

func toAPIAudioTracks(tracks []domain.MediaAudioTrack) *[]gen.MediaInfoAudioTrack {
	if len(tracks) == 0 {
		return nil
	}
	out := make([]gen.MediaInfoAudioTrack, 0, len(tracks))
	for _, track := range tracks {
		out = append(out, gen.MediaInfoAudioTrack{
			Index:         track.Index,
			Codec:         optionalString(track.Codec),
			Channels:      optionalPositive(track.Channels),
			ChannelLayout: optionalString(track.ChannelLayout),
			Language:      optionalString(track.Language),
			Title:         optionalString(track.Title),
			Default:       optionalTrue(track.Default),
		})
	}
	return &out
}

func toAPISubtitleTracks(tracks []domain.MediaSubtitleTrack) *[]gen.MediaInfoSubtitleTrack {
	if len(tracks) == 0 {
		return nil
	}
	out := make([]gen.MediaInfoSubtitleTrack, 0, len(tracks))
	for _, track := range tracks {
		out = append(out, gen.MediaInfoSubtitleTrack{
			Index:           track.Index,
			Codec:           optionalString(track.Codec),
			Language:        optionalString(track.Language),
			Title:           optionalString(track.Title),
			Default:         optionalTrue(track.Default),
			Forced:          optionalTrue(track.Forced),
			HearingImpaired: optionalTrue(track.HearingImpaired),
		})
	}
	return &out
}

func toAPIMediaEmbedded(embedded *domain.MediaEmbedded) *gen.MediaInfoEmbedded {
	if embedded == nil {
		return nil
	}
	out := gen.MediaInfoEmbedded{
		Title:       optionalString(embedded.Title),
		Author:      optionalString(embedded.Author),
		Comment:     optionalString(embedded.Comment),
		Description: optionalString(embedded.Description),
		Genre:       optionalString(embedded.Genre),
		Date:        optionalString(embedded.Date),
		RecordedAt:  recordedAt(embedded.RecordedAt),
		Device:      optionalString(embedded.Device),
		CoverArt:    optionalTrue(embedded.CoverArt),
	}
	if location := embedded.Location; location != nil {
		out.Location = &gen.MediaInfoLocation{
			Latitude: location.Latitude, Longitude: location.Longitude, Altitude: location.Altitude,
		}
	}
	if out == (gen.MediaInfoEmbedded{}) {
		return nil
	}
	return &out
}

// recordedAt は保存した RFC 3339 の撮影の日時を読む。ファイルの時差は time.Time が保ち、
// JSON にもそのまま出る。読めなければ nil（省く）。
func recordedAt(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func toAPIChapters(chapters []domain.MediaChapter) *[]gen.MediaInfoChapter {
	if len(chapters) == 0 {
		return nil
	}
	out := make([]gen.MediaInfoChapter, 0, len(chapters))
	for _, chapter := range chapters {
		out = append(out, gen.MediaInfoChapter{
			StartMs: chapter.StartMs, EndMs: chapter.EndMs, Title: optionalString(chapter.Title),
		})
	}
	return &out
}

// optionalPositive は正の数だけを返し、0 以下（無い）は省く。
func optionalPositive[T int | int64 | float64](value T) *T {
	if value <= 0 {
		return nil
	}
	return &value
}

// optionalTrue は true だけを返し、false（無い）は省く。
func optionalTrue(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}
