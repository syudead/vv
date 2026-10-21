package media

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// probeDisposition keeps every flag by name, since ffprobe prints one key per AV_DISPOSITION bit.
type probeDisposition struct {
	Default, Forced, HearingImpaired, AttachedPicture bool
	flags                                             map[string]int
}

func (d *probeDisposition) UnmarshalJSON(data []byte) error {
	var raw map[string]probeText
	if json.Unmarshal(data, &raw) != nil {
		*d = probeDisposition{}
		return nil
	}
	flags := make(map[string]int, len(raw))
	for name, value := range raw {
		if flag, err := strconv.Atoi(string(value)); err == nil {
			flags[name] = flag
		}
	}
	*d = probeDisposition{
		Default: flags["default"] != 0, Forced: flags["forced"] != 0, HearingImpaired: flags["hearing_impaired"] != 0,
		AttachedPicture: flags["attached_pic"] != 0, flags: flags,
	}
	return nil
}

type probeStreamDetails struct {
	ColorRange    string    `json:"color_range"`
	CodecString   string    `json:"mime_codec_string"`
	FrameCount    probeText `json:"nb_frames"`
	MaxBitRate    probeText `json:"max_bit_rate"`
	StartTime     probeText `json:"start_time"`
	Duration      probeText `json:"duration"`
	SampleFormat  string    `json:"sample_fmt"`
	BitsPerSample probeText `json:"bits_per_sample"`
	ExtradataSize probeText `json:"extradata_size"`
	// The HEVC decoder exports the view IDs its VPS declares, as "0,1" for MV-HEVC.
	ViewIDs   probeText `json:"view_ids_available"`
	FilmGrain probeText `json:"film_grain"`
}

type probeSideDataDetails struct {
	DisplayMatrix probeText `json:"displaymatrix"`
	CropTop       probeText `json:"crop_top"`
	CropBottom    probeText `json:"crop_bottom"`
	CropLeft      probeText `json:"crop_left"`
	CropRight     probeText `json:"crop_right"`
	RedX          probeText `json:"red_x"`
	RedY          probeText `json:"red_y"`
	GreenX        probeText `json:"green_x"`
	GreenY        probeText `json:"green_y"`
	BlueX         probeText `json:"blue_x"`
	BlueY         probeText `json:"blue_y"`
	WhiteX        probeText `json:"white_point_x"`
	WhiteY        probeText `json:"white_point_y"`
	MinLuminance  probeText `json:"min_luminance"`
	MaxLuminance  probeText `json:"max_luminance"`
	MaxContent    probeText `json:"max_content"`
	MaxAverage    probeText `json:"max_average"`
	DVProfile     probeText `json:"dv_profile"`
	DVLevel       probeText `json:"dv_level"`
	DVCompatible  probeText `json:"dv_bl_signal_compatibility_id"`
	ServiceType   probeText `json:"service_type"`
}

const (
	sideDataDisplayMatrix  = "display matrix"
	sideDataCropping       = "frame cropping"
	sideDataMastering      = "mastering display metadata"
	sideDataContentLight   = "content light level metadata"
	sideDataDolbyVision    = "dovi configuration record"
	sideDataHEVCLayers     = "hevc enhancement-layer decoder configuration"
	sideDataServiceType    = "audio service type"
	sideDataEncryptionInit = "encryption initialization data"
	sideDataEncryption     = "encryption info"
)

func sideDataKind(sideData probeSideData) string {
	return strings.ToLower(strings.TrimSpace(sideData.SideDataType))
}

func probeInt(value probeText) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

// probeSmallInt is probeInt for counts and codes that fit an int32; a larger value is unreadable.
func probeSmallInt(value probeText) int {
	parsed := probeInt(value)
	if parsed > math.MaxInt32 {
		return 0
	}
	return int(parsed)
}

func probeFloat(value probeText) *float64 {
	parsed, ok := domain.MediaRational(string(value))
	if !ok {
		return nil
	}
	return &parsed
}

// probeMs reads seconds, or Matroska's HH:MM:SS.fraction DURATION tag.
func probeMs(value string) *int64 {
	value = strings.TrimSpace(value)
	if hours, rest, found := strings.Cut(value, ":"); found {
		minutes, seconds, found := strings.Cut(rest, ":")
		h, hErr := strconv.Atoi(hours)
		m, mErr := strconv.Atoi(minutes)
		if !found || hErr != nil || mErr != nil || h < 0 || m < 0 {
			return nil
		}
		ms, ok := secondsToMs(seconds)
		if !ok {
			return nil
		}
		total := (int64(h)*60+int64(m))*60_000 + ms
		return &total
	}
	ms, ok := secondsToMs(value)
	if !ok {
		return nil
	}
	return &ms
}

func streamTiming(stream probeStream) domain.MediaTiming {
	duration := probeMs(string(stream.Duration))
	if duration == nil {
		duration = probeMs(stream.Tags.get("duration"))
	}
	return domain.MediaTiming{StartMs: probeMs(string(stream.StartTime)), DurationMs: duration}
}

func streamOrigin(stream probeStream) domain.MediaStreamOrigin {
	return domain.MediaStreamOrigin{
		Encoder:   domain.FirstMediaText(stream.Tags.get("encoder")),
		Handler:   domain.FirstMediaText(stream.Tags.get("handler_name")),
		Vendor:    domain.MediaCodecTag(stream.Tags.get("vendor_id")),
		CreatedAt: domain.ParseRecordedAt(stream.Tags.get("creation_time")),
		Timecode:  domain.FirstMediaText(stream.Tags.get("timecode")),
	}
}

// addVideoDetails fills what the stream itself carries; BitsPerPixel needs BitRate and FrameRate set first.
func addVideoDetails(video *domain.MediaVideo, stream probeStream) {
	video.Profile = domain.CleanMediaText(stream.Profile)
	if strings.EqualFold(video.Profile, "unknown") {
		video.Profile = ""
	}
	video.Level = max(stream.Level, 0)
	video.PixelFormat = domain.CleanMediaText(stream.PixelFormat)
	video.ChromaSubsampling = domain.ChromaSubsampling(stream.PixelFormat)
	video.ColorRange = domain.MediaColorRange(stream.ColorRange)
	video.CodecTag = domain.MediaCodecTag(stream.CodecTag)
	video.CodecString = domain.CleanMediaText(stream.CodecString)
	video.VariableFrameRate = domain.VariableFrameRate(parseFrameRate(stream.RealFrameRate), video.FrameRate)
	video.FrameCount = probeInt(stream.FrameCount)
	video.MaxBitRate = probeInt(stream.MaxBitRate)
	video.BitsPerPixel = domain.BitsPerPixel(video.BitRate, stream.Width, stream.Height, video.FrameRate)
	video.MultiLayer = stream.Disposition.flags["multilayer"] != 0 || domain.MediaLayerCount(string(stream.ViewIDs)) > 1
	video.MediaTiming = streamTiming(stream)
	video.MediaStreamOrigin = streamOrigin(stream)
	for _, sideData := range stream.SideDataList {
		addVideoSideData(video, sideData)
	}
}

func addVideoSideData(video *domain.MediaVideo, sideData probeSideData) {
	details := sideData.probeSideDataDetails
	switch kind := sideDataKind(sideData); {
	case kind == sideDataDisplayMatrix:
		video.Mirrored = domain.DisplayMatrixMirrored(string(details.DisplayMatrix))
	case kind == sideDataCropping:
		crop := domain.MediaCrop{Top: probeSmallInt(details.CropTop), Bottom: probeSmallInt(details.CropBottom),
			Left: probeSmallInt(details.CropLeft), Right: probeSmallInt(details.CropRight)}
		if crop != (domain.MediaCrop{}) {
			video.Crop = &crop
		}
	case kind == sideDataMastering:
		mastering := domain.MediaMasteringDisplay{
			RedX: probeFloat(details.RedX), RedY: probeFloat(details.RedY),
			GreenX: probeFloat(details.GreenX), GreenY: probeFloat(details.GreenY),
			BlueX: probeFloat(details.BlueX), BlueY: probeFloat(details.BlueY),
			WhiteX: probeFloat(details.WhiteX), WhiteY: probeFloat(details.WhiteY),
			MinLuminance: probeFloat(details.MinLuminance), MaxLuminance: probeFloat(details.MaxLuminance),
		}
		if mastering != (domain.MediaMasteringDisplay{}) {
			video.Mastering = &mastering
		}
	case kind == sideDataContentLight:
		light := domain.MediaContentLight{MaxContent: probeSmallInt(details.MaxContent),
			MaxAverage: probeSmallInt(details.MaxAverage)}
		if light != (domain.MediaContentLight{}) {
			video.ContentLight = &light
		}
	case kind == sideDataDolbyVision:
		if profile := probeSmallInt(details.DVProfile); profile > 0 {
			video.DolbyVision = &domain.MediaDolbyVision{Profile: profile, Level: probeSmallInt(details.DVLevel),
				Compatibility: probeSmallInt(details.DVCompatible)}
		}
	case kind == sideDataHEVCLayers:
		video.MultiLayer = true
	case strings.HasPrefix(kind, "hdr10+"):
		video.HDR10Plus = true
	}
}

func addAudioDetails(track *domain.MediaAudioTrack, stream probeStream, format probeTags) {
	track.SampleRate = parsePositiveInt(stream.SampleRate)
	track.BitDepth = parsePositiveInt(stream.BitsPerRawSample)
	if track.BitDepth == 0 {
		track.BitDepth = probeSmallInt(stream.BitsPerSample)
	}
	track.SampleFormat = domain.CleanMediaText(stream.SampleFormat)
	if strings.EqualFold(track.SampleFormat, "unknown") {
		track.SampleFormat = ""
	}
	track.BitRate = streamBitRate(stream)
	track.ReplayGain = replayGain(stream.Tags, format)
	track.Roles = domain.MediaRolesFrom(stream.Disposition.flags)
	for _, sideData := range stream.SideDataList {
		if sideDataKind(sideData) == sideDataServiceType {
			if value, err := strconv.Atoi(string(sideData.ServiceType)); err == nil {
				track.ServiceType = domain.MediaAudioServiceType(value)
			}
		}
	}
	track.MediaTiming = streamTiming(stream)
	track.MediaStreamOrigin = streamOrigin(stream)
}

// Stream tags win; container tags describe a single-track file such as FLAC in Matroska.
func replayGain(stream, format probeTags) *domain.MediaReplayGain {
	for _, tags := range []probeTags{stream, format} {
		gain := domain.MediaReplayGain{
			TrackGain: domain.FirstMediaText(tags.get("replaygain_track_gain")),
			TrackPeak: domain.FirstMediaText(tags.get("replaygain_track_peak")),
			AlbumGain: domain.FirstMediaText(tags.get("replaygain_album_gain")),
			AlbumPeak: domain.FirstMediaText(tags.get("replaygain_album_peak")),
		}
		if gain != (domain.MediaReplayGain{}) {
			return &gain
		}
	}
	return nil
}

func addSubtitleDetails(track *domain.MediaSubtitleTrack, stream probeStream) {
	track.Roles = domain.MediaRolesFrom(stream.Disposition.flags)
	track.MediaTiming = streamTiming(stream)
	track.MediaStreamOrigin = streamOrigin(stream)
}

func addEmbeddedDetails(embedded *domain.MediaEmbedded, tags probeTags) {
	text := func(keys ...string) string {
		values := make([]string, len(keys))
		for i, key := range keys {
			values[i] = tags.get(key)
		}
		return domain.FirstMediaText(values...)
	}
	embedded.Album = text("album")
	embedded.AlbumArtist = text("album_artist")
	embedded.Composer = text("composer")
	embedded.Show = text("show")
	embedded.Episode = text("episode_id", "episode_sort")
	embedded.Season = text("season_number")
	embedded.Network = text("network")
	embedded.Synopsis = text("synopsis")
	embedded.Copyright = text("copyright")
	embedded.Publisher = text("publisher")
	embedded.EncodedBy = text("encoded_by")
	embedded.Encoder = text("encoder")
	embedded.SourceURL = text("purl", "url")
	embedded.Keywords = text("keywords", "com.apple.quicktime.keywords")
	embedded.Rating = text("rating", "com.apple.quicktime.rating.user")
	embedded.CaptureSoftware = text("com.apple.quicktime.software", "com.android.version")
	if rate, ok := domain.MediaRational(tags.get("com.android.capture.fps")); ok && rate > 0 {
		embedded.CaptureFrameRate = rate
	}
}

func parseContainer(parsed probeOutput, durationMs int64) *domain.MediaContainer {
	tags := parsed.Format.Tags
	container := domain.MediaContainer{
		StartMs:          probeMs(string(parsed.Format.StartTime)),
		MajorBrand:       domain.MediaCodecTag(tags.get("major_brand")),
		CompatibleBrands: domain.MediaBrands(tags.get("compatible_brands")),
		ProbeScore:       probeSmallInt(parsed.Format.ProbeScore),
	}
	var sizes []domain.MediaStreamSize
	for _, stream := range parsed.Streams {
		container.Encrypted = container.Encrypted || streamEncrypted(stream)
		switch stream.CodecType {
		case "attachment":
			container.Attachments = append(container.Attachments, domain.MediaAttachment{
				Name: domain.FirstMediaText(stream.Tags.get("filename")), MimeType: domain.FirstMediaText(stream.Tags.get("mimetype")),
				Bytes: probeInt(stream.ExtradataSize),
			})
		case "data":
			codec := domain.MediaCodecTag(stream.CodecTag)
			if codec == "" {
				codec = domain.CleanMediaText(stream.CodecName)
			}
			container.DataStreams = append(container.DataStreams, domain.MediaDataStream{
				Index: stream.Index, Codec: codec, Handler: domain.FirstMediaText(stream.Tags.get("handler_name")),
			})
		}
		if size, ok := streamSize(stream); ok {
			sizes = append(sizes, size)
		}
	}
	container.Attachments = domain.LimitMediaTracks(container.Attachments)
	container.DataStreams = domain.LimitMediaTracks(container.DataStreams)
	container.Sizes = domain.MediaSizesFrom(probeInt(parsed.Format.Size), durationMs, sizes)
	if container.StartMs == nil && container.MajorBrand == "" && container.CompatibleBrands == nil &&
		container.ProbeScore == 0 && !container.Encrypted && container.Attachments == nil &&
		container.DataStreams == nil && container.Sizes == nil {
		return nil
	}
	return &container
}

var encryptedCodecTags = map[string]bool{"encv": true, "enca": true, "encs": true, "enct": true}

func streamEncrypted(stream probeStream) bool {
	if stream.Tags.get("enc_key_id") != "" || encryptedCodecTags[strings.ToLower(stream.CodecTag)] {
		return true
	}
	for _, sideData := range stream.SideDataList {
		if kind := sideDataKind(sideData); kind == sideDataEncryptionInit || kind == sideDataEncryption {
			return true
		}
	}
	return false
}

// Cover art counts as other bytes, like attachments and data streams.
func streamSize(stream probeStream) (domain.MediaStreamSize, bool) {
	var kind domain.MediaStreamKind
	switch {
	case stream.CodecType == "video" && !stream.Disposition.AttachedPicture:
		kind = domain.MediaStreamVideo
	case stream.CodecType == "audio":
		kind = domain.MediaStreamAudio
	case stream.CodecType == "subtitle":
		kind = domain.MediaStreamSubtitle
	default:
		return domain.MediaStreamSize{}, false
	}
	size := domain.MediaStreamSize{
		Kind:    kind,
		Bytes:   max(probeInt(probeText(stream.Tags.get("number_of_bytes"))), probeInt(probeText(stream.Tags.get("number_of_bytes-eng")))),
		BitRate: streamBitRate(stream),
	}
	if duration := streamTiming(stream).DurationMs; duration != nil {
		size.DurationMs = *duration
	}
	return size, true
}

// parseFilmGrain reads the first-frame AV1 probe; anything unreadable is unknown.
func parseFilmGrain(output []byte) *bool {
	var parsed struct {
		Streams []struct {
			FilmGrain probeText `json:"film_grain"`
		} `json:"streams"`
	}
	if json.Unmarshal(output, &parsed) != nil || len(parsed.Streams) == 0 {
		return nil
	}
	value, err := strconv.Atoi(string(parsed.Streams[0].FilmGrain))
	if err != nil {
		return nil
	}
	grain := value != 0
	return &grain
}
