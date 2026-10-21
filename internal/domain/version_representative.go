package domain

import "cmp"

// VersionSide is one side of a pair as a representative candidate. Media is its stored media information, nil when
// unknown; OwnerChosen marks a bundle representative its owner picked.
type VersionSide struct {
	Video       Video
	Media       *MediaInfo
	OwnerChosen bool
}

// VersionRepresentativeReason names the first file property that told the two sides apart.
type VersionRepresentativeReason string

const (
	VersionPreferPlayable   VersionRepresentativeReason = "playable"
	VersionPreferAudio      VersionRepresentativeReason = "audio"
	VersionPreferAudioSync  VersionRepresentativeReason = "audio_sync"
	VersionPreferResolution VersionRepresentativeReason = "resolution"
	VersionPreferBitrate    VersionRepresentativeReason = "bitrate"
	VersionPreferOlder      VersionRepresentativeReason = "older"
)

// ChooseVersionRepresentative keeps a representative its owner picked over the files' properties, since bundling
// merges both sides' owner data and only the shown file is at stake.
func ChooseVersionRepresentative(sides [2]VersionSide) int {
	if sides[0].OwnerChosen != sides[1].OwnerChosen {
		return pickSide(sides[0].OwnerChosen)
	}
	chosen, _ := RecommendVersionRepresentative(sides)
	return chosen
}

// RecommendVersionRepresentative judges the files alone: browser playback, audio, audio whose length matches the
// video, pixels, bit rate, then the lower ID.
func RecommendVersionRepresentative(sides [2]VersionSide) (int, VersionRepresentativeReason) {
	a, b := sides[0].Video, sides[1].Video
	for _, preference := range []struct {
		order  int
		reason VersionRepresentativeReason
	}{
		{compareTrue(a.PlayableInBrowser(), b.PlayableInBrowser()), VersionPreferPlayable},
		{compareTrue(a.AudioCodec != "", b.AudioCodec != ""), VersionPreferAudio},
		{compareTrue(!AudioLengthSuspect(sides[0].Media), !AudioLengthSuspect(sides[1].Media)), VersionPreferAudioSync},
		{cmp.Compare(videoPixels(b), videoPixels(a)), VersionPreferResolution},
		{compareBitrate(sides[0], sides[1]), VersionPreferBitrate},
		{cmp.Compare(a.ID, b.ID), VersionPreferOlder},
	} {
		if preference.order != 0 {
			return pickSide(preference.order < 0), preference.reason
		}
	}
	return 0, VersionPreferOlder
}

// VersionAudioTiming compares the first audio track with the video: StartGapMs is how much later the audio starts,
// LengthGapMs how much shorter it is. A field is nil when either side is unknown.
type VersionAudioTiming struct {
	StartGapMs  *int64
	LengthGapMs *int64
}

func AudioTiming(media *MediaInfo) VersionAudioTiming {
	if media == nil || media.Video == nil || len(media.AudioTracks) == 0 {
		return VersionAudioTiming{}
	}
	video, audio := media.Video.MediaTiming, media.AudioTracks[0].MediaTiming
	var timing VersionAudioTiming
	if video.StartMs != nil && audio.StartMs != nil {
		gap := *audio.StartMs - *video.StartMs
		timing.StartGapMs = &gap
	}
	if video.DurationMs != nil && audio.DurationMs != nil {
		gap := *video.DurationMs - *audio.DurationMs
		timing.LengthGapMs = &gap
	}
	return timing
}

// audioLengthToleranceMs allows for an encoder's padding and a last frame of either stream.
const audioLengthToleranceMs = 1000

// AudioLengthSuspect reports audio longer or shorter than the video by more than a second and 1%, which a cut,
// a dropped stretch or drifting sync leaves; the start gap alone is what players align by, so it never counts.
func AudioLengthSuspect(media *MediaInfo) bool {
	timing := AudioTiming(media)
	if timing.LengthGapMs == nil || media.Video.DurationMs == nil {
		return false
	}
	return abs64(*timing.LengthGapMs) > max(audioLengthToleranceMs, *media.Video.DurationMs/100)
}

func pickSide(first bool) int {
	if first {
		return 0
	}
	return 1
}

func compareTrue(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

func videoPixels(v Video) int64 {
	if v.Width == nil || v.Height == nil {
		return 0
	}
	return int64(*v.Width) * int64(*v.Height)
}

// compareBitrate prefers the video stream's bit rate when both sides know it, so lossless audio or attachments do
// not make a file look better; otherwise it compares file bytes per second, with unknown duration ranking lower.
func compareBitrate(a, b VersionSide) int {
	if rateA, rateB := videoBitRate(a.Media), videoBitRate(b.Media); rateA > 0 && rateB > 0 {
		return cmp.Compare(rateB, rateA)
	}
	aKnown := a.Video.DurationMs != nil && *a.Video.DurationMs > 0
	bKnown := b.Video.DurationMs != nil && *b.Video.DurationMs > 0
	if !aKnown || !bKnown {
		return compareTrue(aKnown, bKnown)
	}
	// a.Size / a.Duration > b.Size / b.Duration ⇔ a.Size * b.Duration > b.Size * a.Duration.
	return cmp.Compare(float64(b.Video.SizeBytes)*float64(*a.Video.DurationMs),
		float64(a.Video.SizeBytes)*float64(*b.Video.DurationMs))
}

func videoBitRate(media *MediaInfo) int64 {
	if media == nil || media.Video == nil {
		return 0
	}
	return media.Video.BitRate
}
