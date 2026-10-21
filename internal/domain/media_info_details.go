package domain

import (
	"math"
	"strconv"
	"strings"
)

// Items the import probe reads to tell versions, waste, sync and playback problems apart; see
// docs/design-docs/media-information.md.

// MediaTiming is a stream's or the container's own start and length; a start may be negative.
type MediaTiming struct {
	StartMs    *int64 `json:",omitempty"`
	DurationMs *int64 `json:",omitempty"`
}

// MediaStreamOrigin is what the writing tool or device recorded on one stream; CreatedAt is RFC 3339.
type MediaStreamOrigin struct {
	Encoder   string `json:",omitempty"`
	Handler   string `json:",omitempty"`
	Vendor    string `json:",omitempty"`
	CreatedAt string `json:",omitempty"`
	Timecode  string `json:",omitempty"`
}

// MediaRoles are a track's flagged purposes.
type MediaRoles struct {
	Dub             bool `json:",omitempty"`
	Original        bool `json:",omitempty"`
	Comment         bool `json:",omitempty"`
	VisualImpaired  bool `json:",omitempty"`
	HearingImpaired bool `json:",omitempty"`
	Karaoke         bool `json:",omitempty"`
	Lyrics          bool `json:",omitempty"`
	CleanEffects    bool `json:",omitempty"`
	Captions        bool `json:",omitempty"`
	Descriptions    bool `json:",omitempty"`
}

// MediaRolesFrom reads ffprobe disposition names; other names are ignored.
func MediaRolesFrom(disposition map[string]int) MediaRoles {
	on := func(name string) bool { return disposition[name] != 0 }
	return MediaRoles{
		Dub: on("dub"), Original: on("original"), Comment: on("comment"), VisualImpaired: on("visual_impaired"),
		HearingImpaired: on("hearing_impaired"), Karaoke: on("karaoke"), Lyrics: on("lyrics"),
		CleanEffects: on("clean_effects"), Captions: on("captions"), Descriptions: on("descriptions"),
	}
}

// MediaReplayGain keeps the file's text, such as "-6.50 dB".
type MediaReplayGain struct {
	TrackGain string `json:",omitempty"`
	TrackPeak string `json:",omitempty"`
	AlbumGain string `json:",omitempty"`
	AlbumPeak string `json:",omitempty"`
}

// Crop is the container's cropping in pixels from each edge.
type MediaCrop struct {
	Top    int `json:",omitempty"`
	Bottom int `json:",omitempty"`
	Left   int `json:",omitempty"`
	Right  int `json:",omitempty"`
}

// MediaMasteringDisplay uses CIE 1931 xy chromaticities and cd/m²; nil is absent.
type MediaMasteringDisplay struct {
	RedX         *float64 `json:",omitempty"`
	RedY         *float64 `json:",omitempty"`
	GreenX       *float64 `json:",omitempty"`
	GreenY       *float64 `json:",omitempty"`
	BlueX        *float64 `json:",omitempty"`
	BlueY        *float64 `json:",omitempty"`
	WhiteX       *float64 `json:",omitempty"`
	WhiteY       *float64 `json:",omitempty"`
	MinLuminance *float64 `json:",omitempty"`
	MaxLuminance *float64 `json:",omitempty"`
}

// MediaContentLight is MaxCLL and MaxFALL in cd/m².
type MediaContentLight struct {
	MaxContent int `json:",omitempty"`
	MaxAverage int `json:",omitempty"`
}

type MediaDolbyVision struct {
	Profile int
	Level   int `json:",omitempty"`
	// Compatibility is dv_bl_signal_compatibility_id: what a player without Dolby Vision shows.
	Compatibility int `json:",omitempty"`
}

// MediaContainer holds what the container says about the file as a whole.
type MediaContainer struct {
	StartMs          *int64            `json:",omitempty"`
	MajorBrand       string            `json:",omitempty"`
	CompatibleBrands []string          `json:",omitempty"`
	ProbeScore       int               `json:",omitempty"`
	Encrypted        bool              `json:",omitempty"`
	Attachments      []MediaAttachment `json:",omitempty"`
	DataStreams      []MediaDataStream `json:",omitempty"`
	Sizes            *MediaSizes       `json:",omitempty"`
}

type MediaAttachment struct {
	Name     string `json:",omitempty"`
	MimeType string `json:",omitempty"`
	Bytes    int64  `json:",omitempty"`
}

// MediaDataStream is a non-media stream such as GoPro telemetry or a timecode track.
type MediaDataStream struct {
	Index   int
	Codec   string `json:",omitempty"`
	Handler string `json:",omitempty"`
}

// MediaSizes splits the file's bytes by stream kind; a kind is nil unless every stream of it is known, and
// Other (container overhead, attachments, data, cover art) only when all three are.
type MediaSizes struct {
	TotalBytes    int64  `json:",omitempty"`
	VideoBytes    *int64 `json:",omitempty"`
	AudioBytes    *int64 `json:",omitempty"`
	SubtitleBytes *int64 `json:",omitempty"`
	OtherBytes    *int64 `json:",omitempty"`
}

type MediaStreamKind int

const (
	MediaStreamVideo MediaStreamKind = iota
	MediaStreamAudio
	MediaStreamSubtitle
)

// MediaStreamSize is one stream's size evidence: exact Bytes (Matroska statistics tags), else BitRate over
// DurationMs; zero is unknown.
type MediaStreamSize struct {
	Kind       MediaStreamKind
	Bytes      int64
	BitRate    int64
	DurationMs int64
}

// MediaSizesFrom sums each kind, falling back to the file's duration for a stream without one; nil when nothing
// is known.
func MediaSizesFrom(totalBytes, fileDurationMs int64, streams []MediaStreamSize) *MediaSizes {
	var sums [3]int64
	var seen, unknown [3]bool
	for _, stream := range streams {
		seen[stream.Kind] = true
		bytes := streamBytes(stream, fileDurationMs)
		if bytes <= 0 {
			unknown[stream.Kind] = true
			continue
		}
		sums[stream.Kind] += bytes
	}
	sizes := MediaSizes{TotalBytes: max(totalBytes, 0)}
	known := 0
	parts := [3]**int64{&sizes.VideoBytes, &sizes.AudioBytes, &sizes.SubtitleBytes}
	accounted := int64(0)
	for kind := range parts {
		switch {
		case !seen[kind]:
			known++
		case !unknown[kind]:
			value := sums[kind]
			*parts[kind] = &value
			accounted += value
			known++
		}
	}
	if known == len(parts) && sizes.TotalBytes > 0 && sizes.TotalBytes >= accounted {
		other := sizes.TotalBytes - accounted
		sizes.OtherBytes = &other
	}
	if sizes == (MediaSizes{}) {
		return nil
	}
	return &sizes
}

func streamBytes(stream MediaStreamSize, fileDurationMs int64) int64 {
	if stream.Bytes > 0 {
		return stream.Bytes
	}
	duration := stream.DurationMs
	if duration <= 0 {
		duration = fileDurationMs
	}
	if stream.BitRate <= 0 || duration <= 0 {
		return 0
	}
	return int64(math.Round(float64(stream.BitRate) * float64(duration) / 8000))
}

// BitsPerPixel is the video bit rate per pixel per frame, zero when any input is unknown.
func BitsPerPixel(bitRate int64, width, height int, frameRate float64) float64 {
	if bitRate <= 0 || width <= 0 || height <= 0 || frameRate <= 0 {
		return 0
	}
	return float64(bitRate) / (float64(width) * float64(height) * frameRate)
}

// VariableFrameRate estimates from the header: the average rate differing from the base rate by more than 1%.
// It is nil when either is unknown, since only reading every frame's timestamp can tell for certain.
func VariableFrameRate(baseRate, averageRate float64) *bool {
	if baseRate <= 0 || averageRate <= 0 {
		return nil
	}
	variable := math.Abs(baseRate-averageRate)/baseRate > 0.01
	return &variable
}

// ChromaSubsampling names the pixel format's chroma sampling, empty when unknown.
func ChromaSubsampling(pixelFormat string) string {
	format := strings.ToLower(strings.TrimSpace(pixelFormat))
	switch {
	case format == "":
		return ""
	case strings.HasPrefix(format, "yuv420"), strings.HasPrefix(format, "yuvj420"), strings.HasPrefix(format, "yuva420"),
		strings.HasPrefix(format, "nv12"), strings.HasPrefix(format, "nv21"), strings.HasPrefix(format, "p010"),
		strings.HasPrefix(format, "p012"), strings.HasPrefix(format, "p016"):
		return "4:2:0"
	case strings.HasPrefix(format, "yuv422"), strings.HasPrefix(format, "yuvj422"), strings.HasPrefix(format, "yuva422"),
		strings.HasPrefix(format, "nv16"), strings.HasPrefix(format, "p210"), strings.HasPrefix(format, "p216"),
		strings.HasPrefix(format, "y210"), strings.HasPrefix(format, "yuyv422"), strings.HasPrefix(format, "uyvy422"):
		return "4:2:2"
	case strings.HasPrefix(format, "yuv444"), strings.HasPrefix(format, "yuvj444"), strings.HasPrefix(format, "yuva444"),
		strings.HasPrefix(format, "nv24"), strings.HasPrefix(format, "p410"), strings.HasPrefix(format, "p416"),
		strings.HasPrefix(format, "gbr"), strings.HasPrefix(format, "rgb"), strings.HasPrefix(format, "bgr"):
		return "4:4:4"
	case strings.HasPrefix(format, "yuv411"), strings.HasPrefix(format, "yuvj411"):
		return "4:1:1"
	case strings.HasPrefix(format, "yuv410"):
		return "4:1:0"
	case strings.HasPrefix(format, "gray"):
		return "4:0:0"
	}
	return ""
}

// MediaColorRange maps ffprobe's tv and pc to limited and full, and anything else to unknown.
func MediaColorRange(colorRange string) string {
	switch strings.ToLower(strings.TrimSpace(colorRange)) {
	case "tv", "mpeg", "limited":
		return "limited"
	case "pc", "jpeg", "full":
		return "full"
	}
	return ""
}

// MediaCodecTag drops the fourcc ffprobe prints for an absent tag, such as "[0][0][0][0]".
func MediaCodecTag(tag string) string {
	tag = CleanMediaText(tag)
	if tag == "" || strings.Contains(tag, "[") {
		return ""
	}
	return tag
}

// DisplayMatrixMirrored reports a negative determinant in ffprobe's displaymatrix text, which mirrors the picture;
// a vertical flip is a mirror plus a 180° rotation.
func DisplayMatrixMirrored(matrix string) bool {
	var values []float64
	for _, field := range strings.Fields(matrix) {
		if strings.HasSuffix(field, ":") {
			continue
		}
		value, ok := parseMediaNumber(field)
		if !ok {
			return false
		}
		values = append(values, value)
	}
	if len(values) != 9 {
		return false
	}
	return values[0]*values[4]-values[1]*values[3] < 0
}

// MediaRational reads "num/den" or a plain number; a zero denominator is unknown.
func MediaRational(value string) (float64, bool) {
	numerator, denominator, found := strings.Cut(strings.TrimSpace(value), "/")
	if !found {
		return parseMediaNumber(numerator)
	}
	num, ok := parseMediaNumber(numerator)
	den, denOK := parseMediaNumber(denominator)
	if !ok || !denOK || den == 0 {
		return 0, false
	}
	return num / den, true
}

func parseMediaNumber(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

// MediaBrands splits MP4 compatible_brands into its four-character brands.
func MediaBrands(brands string) []string {
	brands = CleanMediaText(brands)
	var out []string
	for len(brands) >= 4 {
		if brand := strings.TrimSpace(brands[:4]); brand != "" && !strings.ContainsAny(brand, "[]") {
			out = append(out, brand)
		}
		brands = brands[4:]
	}
	return out
}

// mediaAudioServiceTypes are AVAudioServiceType names, in enum order.
var mediaAudioServiceTypes = []string{
	"main", "effects", "visually_impaired", "hearing_impaired", "dialogue", "commentary", "emergency", "voice_over",
	"karaoke",
}

func MediaAudioServiceType(value int) string {
	if value < 0 || value >= len(mediaAudioServiceTypes) {
		return ""
	}
	return mediaAudioServiceTypes[value]
}

// MediaLayerCount counts HEVC view IDs ffprobe prints, as "0,1"; zero is unknown.
func MediaLayerCount(viewIDs string) int {
	count := 0
	for field := range strings.FieldsFuncSeq(viewIDs, func(r rune) bool { return r < '0' || r > '9' }) {
		if field != "" {
			count++
		}
	}
	return count
}
