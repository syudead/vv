package media

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// 取り込みの ffprobe の出力から domain.MediaInfo を組み立てる部分
// （specs/056-media-info/data-model.md「Stored value」）。映像・トラック・埋め込み・
// チャプターのまとまりごとに関数を分ける。読めない項目は省くだけで、解析を失敗にしない。
// 判定と整え方の規則は internal/domain にある。

// probeTags は ffprobe の tags である。鍵の大文字・小文字はコンテナで違う（Matroska は
// 大文字が多い）ので、小文字にそろえて持つ。値が文字列でない鍵は捨てる。
type probeTags map[string]string

// UnmarshalJSON は tags のオブジェクトを読む。文字列でない値は無視する。
func (t *probeTags) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		// tags がオブジェクトでなければ、無いものとして扱う（data-model.md「Rules」）。
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

// get は鍵 key（小文字）の値を返す。無ければ空。
func (t probeTags) get(key string) string {
	return t[key]
}

// probeText は文字列か数かが版で揺れうる値である。文字列はそのまま、数などの値は
// JSON の字面で持ち、オブジェクトと配列は空にする。型の違いで JSON 全体を読めなく
// しないためである。
type probeText string

// UnmarshalJSON は値を文字列として読む。
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

// probeChapter は ffprobe の chapters の 1 件のうち、取り出す部分だけを写す。
type probeChapter struct {
	StartTime probeText `json:"start_time"`
	EndTime   probeText `json:"end_time"`
	Tags      probeTags `json:"tags"`
}

// parseMediaInfo は ffprobe の出力から MediaInfo を組み立てる。値が 1 つも無いファイルでも
// nil にはせず、解析を済ませた印として空の MediaInfo を返す。
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

// parseMediaVideo は最初の非添付の映像 stream の項目を返す。映像が無ければ nil。
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

// videoBitRate は映像 stream の bit_rate を、無ければ Matroska の tags.BPS を返す。
func videoBitRate(stream probeStream) int64 {
	for _, value := range []string{stream.BitRate, stream.Tags.get("bps"), stream.Tags.get("bps-eng")} {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}

// parseAudioTracks はすべての音声 stream を stream の順に返す。
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

// parseSubtitleTracks はすべての字幕 stream を stream の順に返す。
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

// parseEmbedded は format.tags と添付画像から埋め込みの情報を読む。
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

// parseChapters はチャプターの開始と終わりをミリ秒にする。時刻が読めないものは捨てる。
// 並べ替えと尺に収めるのは domain.NormalizeChapters が行う。
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

func secondsToMs(value string) (int64, bool) {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0, false
	}
	return int64(math.Round(seconds * 1000)), true
}
