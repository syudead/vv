package media

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Unreadable optional media fields are omitted instead of failing probing; domain owns
// normalization rules.

// Normalize case-varying container tag keys to lowercase and discard nonstring values.
type probeTags map[string]string

func (t *probeTags) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		// Nonobject tags are treated as absent.
		*t = nil
		return nil
	}
	tags := make(probeTags, len(raw))
	for key, value := range raw {
		var text string
		if json.Unmarshal(value, &text) == nil {
			tags[strings.ToLower(key)] = text
		}
	}
	*t = tags
	return nil
}

func (t probeTags) get(key string) string {
	return t[key]
}

// Accept strings and scalar JSON representations while dropping objects/arrays, so field type
// variations do not invalidate the whole response.
type probeText string

func (p *probeText) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*p = probeText(text)
		return nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] == '{' || trimmed[0] == '[' || string(trimmed) == "null" {
		*p = ""
		return nil
	}
	*p = probeText(trimmed)
	return nil
}

type probeChapter struct {
	StartTime probeText `json:"start_time"`
	EndTime   probeText `json:"end_time"`
	Tags      probeTags `json:"tags"`
}

// Return an empty non-nil MediaInfo to mark completed probing even when no fields exist.
func parseMediaInfo(parsed probeOutput, durationMs int64) *domain.MediaInfo {
	info := &domain.MediaInfo{
		Video:          parseMediaVideo(parsed.Streams),
		AudioTracks:    domain.LimitMediaTracks(parseAudioTracks(parsed.Streams)),
		SubtitleTracks: domain.LimitMediaTracks(parseSubtitleTracks(parsed.Streams)),
		Chapters:       domain.NormalizeChapters(parseChapters(parsed.Chapters), durationMs),
	}
	embedded := parseEmbedded(parsed.Format.Tags, parsed.Streams)
	if !embedded.IsEmpty() {
		info.Embedded = &embedded
	}
	return info
}

func parseMediaVideo(streams []probeStream) *domain.MediaVideo {
	for _, stream := range streams {
		if stream.CodecType != "video" || stream.Disposition.AttachedPicture != 0 {
			continue
		}
		return mediaVideo(stream)
	}
	return nil
}

func mediaVideo(stream probeStream) *domain.MediaVideo {
	video := &domain.MediaVideo{
		FrameRate:      parseFrameRate(stream.AverageFrameRate),
		BitDepth:       domain.VideoBitDepth(parsePositiveInt(stream.BitsPerRawSample), stream.PixelFormat),
		BitRate:        videoBitRate(stream),
		Interlaced:     domain.InterlacedFromFieldOrder(stream.FieldOrder),
		ClosedCaptions: stream.ClosedCaptions != 0,
	}
	dolbyVision := false
	for _, sideData := range stream.SideDataList {
		switch strings.ToLower(sideData.SideDataType) {
		case "dovi configuration record":
			dolbyVision = true
		case "spherical mapping":
			video.Projection = domain.CleanMediaText(string(sideData.Projection))
		case "stereo 3d":
			video.Stereo3D = domain.CleanMediaText(string(sideData.Type))
		}
	}
	video.HDR = domain.ClassifyHDR(stream.ColorTransfer, stream.CodecTag, dolbyVision)
	return video
}

// Fall back from stream.bit_rate to Matroska tags.BPS.
func videoBitRate(stream probeStream) int64 {
	for _, value := range []string{stream.BitRate, stream.Tags.get("bps"), stream.Tags.get("bps-eng")} {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

func parseAudioTracks(streams []probeStream) []domain.MediaAudioTrack {
	var tracks []domain.MediaAudioTrack
	for _, stream := range streams {
		if stream.CodecType != "audio" {
			continue
		}
		tracks = append(tracks, domain.MediaAudioTrack{
			Index:         stream.Index,
			Codec:         domain.CleanMediaText(stream.CodecName),
			Channels:      max(stream.Channels, 0),
			ChannelLayout: domain.CleanMediaText(stream.ChannelLayout),
			Language:      domain.CleanMediaLanguage(stream.Tags.get("language")),
			Title:         domain.CleanMediaText(stream.Tags.get("title")),
			Default:       stream.Disposition.Default != 0,
		})
	}
	return tracks
}

func parseSubtitleTracks(streams []probeStream) []domain.MediaSubtitleTrack {
	var tracks []domain.MediaSubtitleTrack
	for _, stream := range streams {
		if stream.CodecType != "subtitle" {
			continue
		}
		tracks = append(tracks, domain.MediaSubtitleTrack{
			Index:           stream.Index,
			Codec:           domain.CleanMediaText(stream.CodecName),
			Language:        domain.CleanMediaLanguage(stream.Tags.get("language")),
			Title:           domain.CleanMediaText(stream.Tags.get("title")),
			Default:         stream.Disposition.Default != 0,
			Forced:          stream.Disposition.Forced != 0,
			HearingImpaired: stream.Disposition.HearingImpaired != 0,
		})
	}
	return tracks
}

func parseEmbedded(tags probeTags, streams []probeStream) domain.MediaEmbedded {
	return domain.MediaEmbedded{
		Title:       domain.FirstMediaText(tags.get("title")),
		Comment:     domain.FirstMediaText(tags.get("comment")),
		Description: domain.FirstMediaText(tags.get("description"), tags.get("synopsis")),
		Genre:       domain.FirstMediaText(tags.get("genre")),
		Date:        domain.FirstMediaText(tags.get("date")),
		Author:      domain.FirstMediaText(tags.get("artist"), tags.get("author"), tags.get("album_artist")),
		RecordedAt:  domain.ParseRecordedAt(tags.get("com.apple.quicktime.creationdate"), tags.get("creation_time")),
		Device: domain.MediaDevice(
			[2]string{tags.get("com.apple.quicktime.make"), tags.get("com.apple.quicktime.model")},
			[2]string{tags.get("com.android.manufacturer"), tags.get("com.android.model")},
			[2]string{tags.get("make"), tags.get("model")},
		),
		Location: domain.ParseISO6709(
			tags.get("com.apple.quicktime.location.iso6709"), tags.get("location"), tags.get("location-eng")),
		CoverArt: hasAttachedPicture(streams),
	}
}

func hasAttachedPicture(streams []probeStream) bool {
	for _, stream := range streams {
		if stream.Disposition.AttachedPicture != 0 {
			return true
		}
	}
	return false
}

// Skip unreadable chapter times; domain.NormalizeChapters sorts and bounds them.
func parseChapters(chapters []probeChapter) []domain.MediaChapter {
	var out []domain.MediaChapter
	for _, chapter := range chapters {
		start, startOK := secondsToMs(string(chapter.StartTime))
		end, endOK := secondsToMs(string(chapter.EndTime))
		if !startOK || !endOK {
			continue
		}
		out = append(out, domain.MediaChapter{StartMs: start, EndMs: end, Title: chapter.Tags.get("title")})
	}
	return out
}

// Clamp before float-to-int conversion to avoid implementation-dependent overflow turning late
// chapters into beginning chapters.
func secondsToMs(value string) (int64, bool) {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, false
	}
	ms := math.Round(seconds * 1000)
	switch {
	case ms >= math.MaxInt64: // float64(math.MaxInt64) rounds to 2^63.
		return math.MaxInt64, true
	case ms <= math.MinInt64:
		return math.MinInt64, true
	}
	return int64(ms), true
}
