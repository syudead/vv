package httpapi

import (
	"context"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type MediaInfoReader interface {
	// Return nil when the current version is absent.
	MediaInfo(ctx context.Context, videoID int64) (*domain.MediaInfo, error)
	// MediaInfos leaves out videos without current information.
	MediaInfos(ctx context.Context, videoIDs []int64) (map[int64]*domain.MediaInfo, error)
}

// Media information appears only in video details, identically for owners and guests.
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

// Omit nil or empty media information.
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
		Broadcast:      toAPIMediaBroadcast(info.Broadcast),
		Container:      toAPIMediaContainer(info.Container),
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
	out := &gen.MediaInfoVideo{
		FrameRate:         optionalPositive(video.FrameRate),
		BitDepth:          optionalPositive(video.BitDepth),
		BitRate:           optionalPositive(video.BitRate),
		Interlaced:        video.Interlaced,
		Hdr:               gen.MediaInfoVideoHdr(hdr),
		Projection:        optionalString(video.Projection),
		Stereo3d:          optionalString(video.Stereo3D),
		ClosedCaptions:    optionalTrue(video.ClosedCaptions),
		Profile:           optionalString(video.Profile),
		Level:             optionalPositive(video.Level),
		PixelFormat:       optionalString(video.PixelFormat),
		ChromaSubsampling: optionalString(video.ChromaSubsampling),
		CodecTag:          optionalString(video.CodecTag),
		CodecString:       optionalString(video.CodecString),
		VariableFrameRate: video.VariableFrameRate,
		FrameCount:        optionalPositive(video.FrameCount),
		MaxBitRate:        optionalPositive(video.MaxBitRate),
		BitsPerPixel:      optionalPositive(video.BitsPerPixel),
		Mirrored:          optionalTrue(video.Mirrored),
		Hdr10Plus:         optionalTrue(video.HDR10Plus),
		FilmGrain:         video.FilmGrain,
		MultiLayer:        optionalTrue(video.MultiLayer),
		StartMs:           video.StartMs,
		DurationMs:        video.DurationMs,
		Origin:            toAPIStreamOrigin(video.MediaStreamOrigin),
	}
	if video.ColorRange != "" {
		colorRange := gen.MediaInfoVideoColorRange(video.ColorRange)
		out.ColorRange = &colorRange
	}
	if crop := video.Crop; crop != nil {
		out.Crop = &gen.MediaInfoCrop{Top: optionalPositive(crop.Top), Bottom: optionalPositive(crop.Bottom),
			Left: optionalPositive(crop.Left), Right: optionalPositive(crop.Right)}
	}
	if m := video.Mastering; m != nil {
		out.Mastering = &gen.MediaInfoMasteringDisplay{RedX: m.RedX, RedY: m.RedY, GreenX: m.GreenX, GreenY: m.GreenY,
			BlueX: m.BlueX, BlueY: m.BlueY, WhiteX: m.WhiteX, WhiteY: m.WhiteY,
			MinLuminance: m.MinLuminance, MaxLuminance: m.MaxLuminance}
	}
	if light := video.ContentLight; light != nil {
		out.ContentLight = &gen.MediaInfoContentLight{MaxContent: optionalPositive(light.MaxContent),
			MaxAverage: optionalPositive(light.MaxAverage)}
	}
	if dv := video.DolbyVision; dv != nil {
		out.DolbyVision = &gen.MediaInfoDolbyVision{Profile: dv.Profile, Level: optionalPositive(dv.Level),
			Compatibility: optionalPositive(dv.Compatibility)}
	}
	return out
}

func toAPIStreamOrigin(origin domain.MediaStreamOrigin) *gen.MediaInfoStreamOrigin {
	out := gen.MediaInfoStreamOrigin{
		Encoder: optionalString(origin.Encoder), Handler: optionalString(origin.Handler),
		Vendor: optionalString(origin.Vendor), CreatedAt: recordedAt(origin.CreatedAt),
		Timecode: optionalString(origin.Timecode),
	}
	if out == (gen.MediaInfoStreamOrigin{}) {
		return nil
	}
	return &out
}

func toAPITrackRoles(roles domain.MediaRoles) *gen.MediaInfoTrackRoles {
	if roles == (domain.MediaRoles{}) {
		return nil
	}
	return &gen.MediaInfoTrackRoles{
		Dub: optionalTrue(roles.Dub), Original: optionalTrue(roles.Original), Comment: optionalTrue(roles.Comment),
		VisualImpaired: optionalTrue(roles.VisualImpaired), HearingImpaired: optionalTrue(roles.HearingImpaired),
		Karaoke: optionalTrue(roles.Karaoke), Lyrics: optionalTrue(roles.Lyrics),
		CleanEffects: optionalTrue(roles.CleanEffects), Captions: optionalTrue(roles.Captions),
		Descriptions: optionalTrue(roles.Descriptions),
	}
}

func toAPIMediaContainer(container *domain.MediaContainer) *gen.MediaInfoContainer {
	if container == nil {
		return nil
	}
	out := &gen.MediaInfoContainer{
		StartMs: container.StartMs, MajorBrand: optionalString(container.MajorBrand),
		ProbeScore: optionalPositive(container.ProbeScore), Encrypted: optionalTrue(container.Encrypted),
	}
	if len(container.CompatibleBrands) > 0 {
		out.CompatibleBrands = &container.CompatibleBrands
	}
	if len(container.Attachments) > 0 {
		attachments := make([]gen.MediaInfoAttachment, 0, len(container.Attachments))
		for _, a := range container.Attachments {
			attachments = append(attachments, gen.MediaInfoAttachment{Name: optionalString(a.Name),
				MimeType: optionalString(a.MimeType), Bytes: optionalPositive(a.Bytes)})
		}
		out.Attachments = &attachments
	}
	if len(container.DataStreams) > 0 {
		streams := make([]gen.MediaInfoDataStream, 0, len(container.DataStreams))
		for _, d := range container.DataStreams {
			streams = append(streams, gen.MediaInfoDataStream{Index: d.Index, Codec: optionalString(d.Codec),
				Handler: optionalString(d.Handler)})
		}
		out.DataStreams = &streams
	}
	if sizes := container.Sizes; sizes != nil {
		out.Sizes = &gen.MediaInfoSizes{TotalBytes: optionalPositive(sizes.TotalBytes), VideoBytes: sizes.VideoBytes,
			AudioBytes: sizes.AudioBytes, SubtitleBytes: sizes.SubtitleBytes, OtherBytes: sizes.OtherBytes}
	}
	return out
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
			SampleRate:    optionalPositive(track.SampleRate),
			BitDepth:      optionalPositive(track.BitDepth),
			SampleFormat:  optionalString(track.SampleFormat),
			BitRate:       optionalPositive(track.BitRate),
			ReplayGain:    toAPIReplayGain(track.ReplayGain),
			Roles:         toAPITrackRoles(track.Roles),
			ServiceType:   audioServiceType(track.ServiceType),
			StartMs:       track.StartMs,
			DurationMs:    track.DurationMs,
			Origin:        toAPIStreamOrigin(track.MediaStreamOrigin),
		})
	}
	return &out
}

func toAPIReplayGain(gain *domain.MediaReplayGain) *gen.MediaInfoReplayGain {
	if gain == nil {
		return nil
	}
	return &gen.MediaInfoReplayGain{TrackGain: optionalString(gain.TrackGain), TrackPeak: optionalString(gain.TrackPeak),
		AlbumGain: optionalString(gain.AlbumGain), AlbumPeak: optionalString(gain.AlbumPeak)}
}

func audioServiceType(value string) *gen.MediaInfoAudioTrackServiceType {
	if value == "" {
		return nil
	}
	serviceType := gen.MediaInfoAudioTrackServiceType(value)
	return &serviceType
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
			Roles:           toAPITrackRoles(track.Roles),
			StartMs:         track.StartMs,
			DurationMs:      track.DurationMs,
			Origin:          toAPIStreamOrigin(track.MediaStreamOrigin),
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

		Album:            optionalString(embedded.Album),
		AlbumArtist:      optionalString(embedded.AlbumArtist),
		Composer:         optionalString(embedded.Composer),
		Show:             optionalString(embedded.Show),
		Episode:          optionalString(embedded.Episode),
		Season:           optionalString(embedded.Season),
		Network:          optionalString(embedded.Network),
		Synopsis:         optionalString(embedded.Synopsis),
		Copyright:        optionalString(embedded.Copyright),
		Publisher:        optionalString(embedded.Publisher),
		EncodedBy:        optionalString(embedded.EncodedBy),
		Encoder:          optionalString(embedded.Encoder),
		SourceUrl:        optionalString(embedded.SourceURL),
		Keywords:         optionalString(embedded.Keywords),
		Rating:           optionalString(embedded.Rating),
		CaptureSoftware:  optionalString(embedded.CaptureSoftware),
		CaptureFrameRate: optionalPositive(embedded.CaptureFrameRate),
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

func toAPIMediaBroadcast(broadcast *domain.MediaBroadcast) *gen.MediaInfoBroadcast {
	if broadcast == nil {
		return nil
	}
	out := gen.MediaInfoBroadcast{Station: optionalString(broadcast.Station), Program: optionalString(broadcast.Program)}
	if out == (gen.MediaInfoBroadcast{}) {
		return nil
	}
	return &out
}

// Preserve the RFC3339 offset in recordedAt; omit invalid values.
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

func optionalPositive[T int | int64 | float64](value T) *T {
	if value <= 0 {
		return nil
	}
	return &value
}

func optionalTrue(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}
