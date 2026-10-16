package domain

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MediaInfoVersion は保存する MediaInfo の形の版である。項目を足す・意味を変えるときに
// 上げる。版が違う行は「無い」と読まれる（specs/056-media-info/data-model.md
// 「video_media_info」）。
const MediaInfoVersion = 1

// 保存する値の上限（specs/056-media-info/data-model.md「Rules」）。
const (
	// MediaTextMaxRunes は 1 つの文字列の上限の文字数である。超えた分は切る。
	MediaTextMaxRunes = 2000
	// MediaTracksMaxCount は音声・字幕それぞれの本数の上限である。
	MediaTracksMaxCount = 64
	// MediaChaptersMaxCount はチャプターの数の上限である。
	MediaChaptersMaxCount = 1000
)

// MediaInfo は取り込みの ffprobe 1 回で読む、動画の追加の情報である。JSON にして
// video_media_info.info に保存する。欄名は Go の欄名をそのまま使い、無い項目は JSON から
// 省く（specs/056-media-info/data-model.md「Stored value」）。
type MediaInfo struct {
	// Video は最初の非添付の映像 stream の項目。映像が無ければ nil。
	Video *MediaVideo `json:",omitempty"`
	// AudioTracks はすべての音声 stream（stream の順、MediaTracksMaxCount まで）。
	AudioTracks []MediaAudioTrack `json:",omitempty"`
	// SubtitleTracks はすべての字幕 stream（stream の順、MediaTracksMaxCount まで）。
	SubtitleTracks []MediaSubtitleTrack `json:",omitempty"`
	// Embedded はファイルに埋め込まれた情報。1 つも無ければ nil。
	Embedded *MediaEmbedded `json:",omitempty"`
	// Broadcast は TS の録画の放送局名と番組名。TS でないか、どちらも読めなければ nil。
	Broadcast *MediaBroadcast `json:",omitempty"`
	// Chapters は NormalizeChapters を通したチャプター。
	Chapters []MediaChapter `json:",omitempty"`
}

// HDRKind は映像の HDR の種類である。
type HDRKind string

const (
	HDRKindSDR         HDRKind = "sdr"
	HDRKindHDR10       HDRKind = "hdr10"
	HDRKindHLG         HDRKind = "hlg"
	HDRKindDolbyVision HDRKind = "dolby_vision"
)

// MediaVideo は映像 stream の項目である。HDR は映像があれば必ず入る。
type MediaVideo struct {
	FrameRate float64 `json:",omitempty"`
	BitDepth  int     `json:",omitempty"`
	// BitRate はビット毎秒。
	BitRate int64 `json:",omitempty"`
	// Interlaced はファイルがプログレッシブかインターレースかを示すときだけ入る。
	Interlaced     *bool   `json:",omitempty"`
	HDR            HDRKind `json:",omitempty"`
	Projection     string  `json:",omitempty"`
	Stereo3D       string  `json:",omitempty"`
	ClosedCaptions bool    `json:",omitempty"`
}

// MediaAudioTrack は音声 stream 1 本である。
type MediaAudioTrack struct {
	Index         int
	Codec         string `json:",omitempty"`
	Channels      int    `json:",omitempty"`
	ChannelLayout string `json:",omitempty"`
	Language      string `json:",omitempty"`
	Title         string `json:",omitempty"`
	Default       bool   `json:",omitempty"`
}

// MediaSubtitleTrack は字幕 stream 1 本である。
type MediaSubtitleTrack struct {
	Index           int
	Codec           string `json:",omitempty"`
	Language        string `json:",omitempty"`
	Title           string `json:",omitempty"`
	Default         bool   `json:",omitempty"`
	Forced          bool   `json:",omitempty"`
	HearingImpaired bool   `json:",omitempty"`
}

// MediaEmbedded はファイルに埋め込まれたタグの項目である。Date はファイルの文字のまま、
// RecordedAt は RFC 3339 の文字列である。
type MediaEmbedded struct {
	Title       string       `json:",omitempty"`
	Comment     string       `json:",omitempty"`
	Description string       `json:",omitempty"`
	Genre       string       `json:",omitempty"`
	Date        string       `json:",omitempty"`
	Author      string       `json:",omitempty"`
	RecordedAt  string       `json:",omitempty"`
	Device      string       `json:",omitempty"`
	Location    *GeoLocation `json:",omitempty"`
	CoverArt    bool         `json:",omitempty"`
}

// GeoLocation は撮影場所の緯度・経度（度）と、あれば高度（メートル）である。
type GeoLocation struct {
	Latitude  float64
	Longitude float64
	Altitude  *float64 `json:",omitempty"`
}

// MediaChapter はチャプター 1 つである。時刻はミリ秒。
type MediaChapter struct {
	StartMs int64
	EndMs   int64
	Title   string `json:",omitempty"`
}

// IsEmpty は埋め込みの情報が 1 つも無いかを返す。
func (e MediaEmbedded) IsEmpty() bool {
	return e == MediaEmbedded{}
}

// dolbyVisionCodecTags は Dolby Vision を示す codec_tag_string である。
var dolbyVisionCodecTags = map[string]bool{"dvh1": true, "dvhe": true, "dvav": true, "dva1": true, "dav1": true}

// ClassifyHDR は映像の HDR の種類を決める（specs/056-media-info/research.md R-3）。
// Dolby Vision の構成記録（hasDolbyVisionRecord）か codec tag、PQ（smpte2084）、
// HLG（arib-std-b67）の順に見て、どれでもなければ SDR である。
func ClassifyHDR(colorTransfer, codecTag string, hasDolbyVisionRecord bool) HDRKind {
	switch {
	case hasDolbyVisionRecord || dolbyVisionCodecTags[strings.ToLower(strings.TrimSpace(codecTag))]:
		return HDRKindDolbyVision
	case strings.EqualFold(colorTransfer, "smpte2084"):
		return HDRKindHDR10
	case strings.EqualFold(colorTransfer, "arib-std-b67"):
		return HDRKindHLG
	default:
		return HDRKindSDR
	}
}

// InterlacedFromFieldOrder は field_order からインターレースかを返す。tt・bb・tb・bt は
// インターレース、progressive はそうでない。unknown・空・その他は nil（項目なし）。
func InterlacedFromFieldOrder(fieldOrder string) *bool {
	var interlaced bool
	switch strings.ToLower(strings.TrimSpace(fieldOrder)) {
	case "tt", "bb", "tb", "bt":
		interlaced = true
	case "progressive":
		interlaced = false
	default:
		return nil
	}
	return &interlaced
}

// pixelFormatDepth は pix_fmt の末尾の深さ（yuv420p10le の 10、p010le の 10、gray12le の 12）を読む。
var pixelFormatDepth = regexp.MustCompile(`(?:p|gray)0?(\d{1,2})(?:le|be)?$`)

// VideoBitDepth はビット深度を返す。bits_per_raw_sample が正ならそれを、無ければ pix_fmt の
// 深さを採る。どちらも数を持たなければ 0（項目なし）。
func VideoBitDepth(bitsPerRawSample int, pixelFormat string) int {
	if bitsPerRawSample > 0 {
		return bitsPerRawSample
	}
	match := pixelFormatDepth.FindStringSubmatch(strings.ToLower(strings.TrimSpace(pixelFormat)))
	if match == nil {
		return 0
	}
	depth, err := strconv.Atoi(match[1])
	if err != nil || depth < 8 || depth > 16 {
		return 0
	}
	return depth
}

// CleanMediaText は保存する文字列を整える。不正な UTF-8 と改行以外の制御文字を除き、
// 前後の空白を除き、MediaTextMaxRunes 文字で切る。空になれば空（項目なし）である。
func CleanMediaText(value string) string {
	value = strings.ToValidUTF8(value, "")
	value = strings.Map(func(r rune) rune {
		if r != '\n' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > MediaTextMaxRunes {
		value = strings.TrimSpace(string([]rune(value)[:MediaTextMaxRunes]))
	}
	return value
}

// CleanMediaLanguage は言語の印を整える。und（未定義）は空（項目なし）にする。
func CleanMediaLanguage(value string) string {
	value = CleanMediaText(value)
	if strings.EqualFold(value, "und") {
		return ""
	}
	return value
}

// FirstMediaText は candidates のうち、整えて空でない最初の値を返す。
func FirstMediaText(candidates ...string) string {
	for _, candidate := range candidates {
		if cleaned := CleanMediaText(candidate); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

// MediaDevice は撮影機材の名前を返す。pairs はメーカーと機種の組で、どちらかが空でない
// 最初の組を空白でつなぐ。
func MediaDevice(pairs ...[2]string) string {
	for _, pair := range pairs {
		var parts []string
		for _, part := range pair {
			if cleaned := CleanMediaText(part); cleaned != "" {
				parts = append(parts, cleaned)
			}
		}
		if len(parts) > 0 {
			return CleanMediaText(strings.Join(parts, " "))
		}
	}
	return ""
}

// recordedAtOffsetLayouts はオフセットのある書き方で、オフセットを保つ。
var recordedAtOffsetLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999-0700",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999-0700",
}

// recordedAtLocalLayouts はオフセットの無い書き方で、UTC として読む。
var recordedAtLocalLayouts = []string{
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
}

// ParseRecordedAt は撮影日時を RFC 3339 の文字列にする。candidates を順に試し、日時として
// 読める最初の値を採る。オフセットのある値はそのオフセットを保ち、無い値は UTC とする。
// どれも読めなければ空（項目なし）である。
func ParseRecordedAt(candidates ...string) string {
	for _, candidate := range candidates {
		if parsed, ok := parseRecordedAt(strings.TrimSpace(candidate)); ok {
			return parsed.Format(time.RFC3339)
		}
	}
	return ""
}

func parseRecordedAt(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range recordedAtOffsetLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	for _, layout := range recordedAtLocalLayouts {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

// iso6709Pattern は ISO 6709 の ±緯度±経度[±高度][/] である。
var iso6709Pattern = regexp.MustCompile(`^([+-]\d+(?:\.\d*)?)([+-]\d+(?:\.\d*)?)([+-]\d+(?:\.\d*)?)?(?:CRS[^/]*)?/?$`)

// ParseISO6709 は撮影場所を読む。candidates を順に試し、ISO 6709 として読める最初の値を
// 採る。緯度は整数部 2・4・6 桁（度、度分、度分秒）、経度は 3・5・7 桁である。緯度が ±90、
// 経度が ±180 の外なら読めないものとする。どれも読めなければ nil（項目なし）。
func ParseISO6709(candidates ...string) *GeoLocation {
	for _, candidate := range candidates {
		if location := parseISO6709(strings.TrimSpace(candidate)); location != nil {
			return location
		}
	}
	return nil
}

func parseISO6709(value string) *GeoLocation {
	match := iso6709Pattern.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	latitude, ok := parseISO6709Coordinate(match[1], 2)
	if !ok || latitude < -90 || latitude > 90 {
		return nil
	}
	longitude, ok := parseISO6709Coordinate(match[2], 3)
	if !ok || longitude < -180 || longitude > 180 {
		return nil
	}
	location := &GeoLocation{Latitude: latitude, Longitude: longitude}
	if match[3] != "" {
		altitude, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			return nil
		}
		location.Altitude = &altitude
	}
	return location
}

// parseISO6709Coordinate は符号つきの座標 1 つを度にする。degreeDigits は度の桁数（緯度 2、
// 経度 3）で、整数部がその桁数なら度、+2 桁なら度分、+4 桁なら度分秒である。
func parseISO6709Coordinate(value string, degreeDigits int) (float64, bool) {
	sign := 1.0
	if value[0] == '-' {
		sign = -1
	}
	body := value[1:]
	integer, fraction, _ := strings.Cut(body, ".")
	fractionPart := 0.0
	if fraction != "" {
		parsed, err := strconv.ParseFloat("0."+fraction, 64)
		if err != nil {
			return 0, false
		}
		fractionPart = parsed
	}
	var parts []float64
	switch len(integer) {
	case degreeDigits:
		parts = []float64{atof(integer)}
	case degreeDigits + 2:
		parts = []float64{atof(integer[:degreeDigits]), atof(integer[degreeDigits:])}
	case degreeDigits + 4:
		parts = []float64{atof(integer[:degreeDigits]), atof(integer[degreeDigits : degreeDigits+2]), atof(integer[degreeDigits+2:])}
	default:
		return 0, false
	}
	parts[len(parts)-1] += fractionPart
	degrees := 0.0
	for i, part := range parts {
		if i > 0 && part >= 60 {
			return 0, false
		}
		divisor := 1.0
		for range i {
			divisor *= 60
		}
		degrees += part / divisor
	}
	return sign * degrees, true
}

// atof は数字だけの文字列を数にする（呼び手が正規表現で数字だと確かめている）。
func atof(digits string) float64 {
	parsed, _ := strconv.ParseFloat(digits, 64)
	return parsed
}

// NormalizeChapters はチャプターを整える（specs/056-media-info/data-model.md「Rules」）。
// 開始で並べ（同じ開始は元の順）、開始が尺以上のものを落とし、終わりを尺と開始の間に
// 収める。1 つだけでも残し、重なりはそのまま残す。題は CleanMediaText で整え、
// MediaChaptersMaxCount 個までにする。尺が分からない（0 以下）ときは何も残さない。
func NormalizeChapters(chapters []MediaChapter, durationMs int64) []MediaChapter {
	if durationMs <= 0 || len(chapters) == 0 {
		return nil
	}
	sorted := append([]MediaChapter(nil), chapters...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartMs < sorted[j].StartMs })
	var out []MediaChapter
	for _, chapter := range sorted {
		if chapter.StartMs >= durationMs {
			continue
		}
		chapter.StartMs = max(chapter.StartMs, 0)
		chapter.EndMs = max(min(chapter.EndMs, durationMs), chapter.StartMs)
		chapter.Title = CleanMediaText(chapter.Title)
		out = append(out, chapter)
		if len(out) == MediaChaptersMaxCount {
			break
		}
	}
	return out
}

// LimitMediaTracks は音声・字幕の本数を MediaTracksMaxCount までにする。
func LimitMediaTracks[T any](tracks []T) []T {
	if len(tracks) > MediaTracksMaxCount {
		return tracks[:MediaTracksMaxCount]
	}
	return tracks
}
