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

// Bump MediaInfoVersion when adding fields or changing semantics; mismatched versions read as absent.
const MediaInfoVersion = 1

const (
	MediaTextMaxRunes     = 2000
	MediaTracksMaxCount   = 64
	MediaChaptersMaxCount = 1000
)

// MediaInfo is stored as JSON in video_media_info.info using Go field names and omitting absent fields.
type MediaInfo struct {
	// Video uses the first non-attached video stream, or nil.
	Video          *MediaVideo          `json:",omitempty"`
	AudioTracks    []MediaAudioTrack    `json:",omitempty"`
	SubtitleTracks []MediaSubtitleTrack `json:",omitempty"`
	Embedded       *MediaEmbedded       `json:",omitempty"`
	Broadcast      *MediaBroadcast      `json:",omitempty"`
	Chapters       []MediaChapter       `json:",omitempty"`
}

type HDRKind string

const (
	HDRKindSDR         HDRKind = "sdr"
	HDRKindHDR10       HDRKind = "hdr10"
	HDRKindHLG         HDRKind = "hlg"
	HDRKindDolbyVision HDRKind = "dolby_vision"
)

// HDR is always populated when a video stream exists.
type MediaVideo struct {
	FrameRate float64 `json:",omitempty"`
	BitDepth  int     `json:",omitempty"`
	// BitRate is bits per second.
	BitRate int64 `json:",omitempty"`
	// Interlaced is nil unless the source explicitly identifies field order.
	Interlaced     *bool   `json:",omitempty"`
	HDR            HDRKind `json:",omitempty"`
	Projection     string  `json:",omitempty"`
	Stereo3D       string  `json:",omitempty"`
	ClosedCaptions bool    `json:",omitempty"`
}

type MediaAudioTrack struct {
	Index         int
	Codec         string `json:",omitempty"`
	Channels      int    `json:",omitempty"`
	ChannelLayout string `json:",omitempty"`
	Language      string `json:",omitempty"`
	Title         string `json:",omitempty"`
	Default       bool   `json:",omitempty"`
}

type MediaSubtitleTrack struct {
	Index           int
	Codec           string `json:",omitempty"`
	Language        string `json:",omitempty"`
	Title           string `json:",omitempty"`
	Default         bool   `json:",omitempty"`
	Forced          bool   `json:",omitempty"`
	HearingImpaired bool   `json:",omitempty"`
}

// Date retains source text; RecordedAt is RFC 3339.
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

// Coordinates use degrees and optional altitude uses meters.
type GeoLocation struct {
	Latitude  float64
	Longitude float64
	Altitude  *float64 `json:",omitempty"`
}

// Chapter times use milliseconds.
type MediaChapter struct {
	StartMs int64
	EndMs   int64
	Title   string `json:",omitempty"`
}

func (e MediaEmbedded) IsEmpty() bool {
	return e == MediaEmbedded{}
}

var dolbyVisionCodecTags = map[string]bool{"dvh1": true, "dvhe": true, "dvav": true, "dva1": true, "dav1": true}

// ClassifyHDR prioritizes Dolby Vision records/tags, then PQ, HLG, and finally SDR.
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

var pixelFormatDepth = regexp.MustCompile(`(?:p|gray)0?(\d{1,2})(?:le|be)?$`)

// VideoBitDepth prefers positive bits_per_raw_sample, then pix_fmt, and returns zero when unknown.
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

// CleanMediaText removes invalid UTF-8 and controls except newlines, trims whitespace, and truncates by code points.
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

// CleanMediaLanguage treats und as absent.
func CleanMediaLanguage(value string) string {
	value = CleanMediaText(value)
	if strings.EqualFold(value, "und") {
		return ""
	}
	return value
}

func FirstMediaText(candidates ...string) string {
	for _, candidate := range candidates {
		if cleaned := CleanMediaText(candidate); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

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

var recordedAtOffsetLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999-0700",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999-0700",
}

var recordedAtLocalLayouts = []string{
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
}

// ParseRecordedAt takes the first parseable candidate, preserving explicit offsets and interpreting unzoned values as UTC.
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

// ISO 6709 syntax: signed latitude, signed longitude, optional signed altitude, optional trailing slash.
var iso6709Pattern = regexp.MustCompile(`^([+-]\d+(?:\.\d*)?)([+-]\d+(?:\.\d*)?)([+-]\d+(?:\.\d*)?)?(?:CRS[^/]*)?/?$`)

// ParseISO6709 accepts degrees, degrees/minutes, or degrees/minutes/seconds and rejects coordinates outside ±90/±180; it returns the first valid candidate.
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

// The caller's regular expression guarantees numeric input.
func atof(digits string) float64 {
	parsed, _ := strconv.ParseFloat(digits, 64)
	return parsed
}

// NormalizeChapters stable-sorts starts, drops out-of-duration starts, clamps ends, preserves overlaps, and limits count; unknown duration yields no chapters.
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

func LimitMediaTracks[T any](tracks []T) []T {
	if len(tracks) > MediaTracksMaxCount {
		return tracks[:MediaTracksMaxCount]
	}
	return tracks
}
