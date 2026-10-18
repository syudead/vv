package domain

import "errors"

// Unavailable or still-checking encoder choices leave saved settings unchanged.
var ErrEncoderUnavailable = errors.New("video encoder is not available")

type VideoEncoder string

const (
	VideoEncoderSoftware     VideoEncoder = "software"
	VideoEncoderNVENC        VideoEncoder = "nvenc"
	VideoEncoderQSV          VideoEncoder = "qsv"
	VideoEncoderVAAPI        VideoEncoder = "vaapi"
	VideoEncoderVideoToolbox VideoEncoder = "videotoolbox"
)

// HardwareVideoEncoders defines automatic selection and display order.
var HardwareVideoEncoders = []VideoEncoder{
	VideoEncoderNVENC,
	VideoEncoderQSV,
	VideoEncoderVAAPI,
	VideoEncoderVideoToolbox,
}

type EncoderChoice string

const (
	EncoderChoiceSoftware                   = EncoderChoice(VideoEncoderSoftware)
	EncoderChoiceNVENC                      = EncoderChoice(VideoEncoderNVENC)
	EncoderChoiceQSV                        = EncoderChoice(VideoEncoderQSV)
	EncoderChoiceVAAPI                      = EncoderChoice(VideoEncoderVAAPI)
	EncoderChoiceVideoToolbox               = EncoderChoice(VideoEncoderVideoToolbox)
	EncoderChoiceAuto         EncoderChoice = "auto"
)

func (c EncoderChoice) Valid() bool {
	switch c {
	case EncoderChoiceSoftware, EncoderChoiceAuto:
		return true
	}
	_, ok := c.HardwareEncoder()
	return ok
}

func (c EncoderChoice) HardwareEncoder() (VideoEncoder, bool) {
	for _, encoder := range HardwareVideoEncoders {
		if EncoderChoice(encoder) == c {
			return encoder, true
		}
	}
	return "", false
}

// ParseEncoderChoice treats missing/unknown values as software without rewriting storage.
func ParseEncoderChoice(value string, found bool) EncoderChoice {
	choice := EncoderChoice(value)
	if !found || !choice.Valid() {
		return EncoderChoiceSoftware
	}
	return choice
}

type EncoderState string

const (
	EncoderChecking    EncoderState = "checking"
	EncoderAvailable   EncoderState = "available"
	EncoderUnavailable EncoderState = "unavailable"
)

type EncoderUnavailableReason string

const (
	EncoderReasonUnsupportedOS  EncoderUnavailableReason = "unsupported_os"
	EncoderReasonEncoderMissing EncoderUnavailableReason = "encoder_missing"
	EncoderReasonCheckFailed    EncoderUnavailableReason = "check_failed"
	EncoderReasonTimedOut       EncoderUnavailableReason = "timed_out"
)

type EncoderFallbackReason string

const (
	EncoderFallbackSelectedUnavailable EncoderFallbackReason = "selected_unavailable"
	EncoderFallbackChecking            EncoderFallbackReason = "checking"
)

// Software is omitted because it is always available.
type EncoderAvailability struct {
	Encoder VideoEncoder
	State   EncoderState
	Reason  EncoderUnavailableReason
	// Detail is diagnostic-only, potentially including an FFmpeg stderr tail, and is never exposed through the API.
	Detail string
}

type TranscodeEncoding struct {
	Choice    EncoderChoice
	Effective VideoEncoder
	// FallbackReason is set only when a selected hardware encoder falls back to software.
	FallbackReason EncoderFallbackReason
	Checking       bool
	Encoders       []EncoderAvailability
}

func NewTranscodeEncoding(choice EncoderChoice, availability []EncoderAvailability) TranscodeEncoding {
	effective, reason := ResolveVideoEncoder(choice, availability)
	checking := false
	for _, a := range availability {
		if a.State == EncoderChecking {
			checking = true
		}
	}
	return TranscodeEncoding{
		Choice:         choice,
		Effective:      effective,
		FallbackReason: reason,
		Checking:       checking,
		Encoders:       append([]EncoderAvailability(nil), availability...),
	}
}

// ResolveVideoEncoder selects the first available automatic candidate without a fallback reason; explicit unavailable/checking hardware falls back to software with a reason, and unknown choices use software.
func ResolveVideoEncoder(choice EncoderChoice, availability []EncoderAvailability) (VideoEncoder, EncoderFallbackReason) {
	if choice == EncoderChoiceAuto {
		for _, encoder := range HardwareVideoEncoders {
			if stateOf(encoder, availability) == EncoderAvailable {
				return encoder, ""
			}
		}
		return VideoEncoderSoftware, ""
	}
	encoder, ok := choice.HardwareEncoder()
	if !ok {
		return VideoEncoderSoftware, ""
	}
	switch stateOf(encoder, availability) {
	case EncoderAvailable:
		return encoder, ""
	case EncoderChecking:
		return VideoEncoderSoftware, EncoderFallbackChecking
	default:
		return VideoEncoderSoftware, EncoderFallbackSelectedUnavailable
	}
}

func stateOf(encoder VideoEncoder, availability []EncoderAvailability) EncoderState {
	for _, a := range availability {
		if a.Encoder == encoder {
			return a.State
		}
	}
	return EncoderUnavailable
}

func HardwareEncoderCandidates(goos string) []VideoEncoder {
	switch goos {
	case "linux":
		return []VideoEncoder{VideoEncoderNVENC, VideoEncoderQSV, VideoEncoderVAAPI}
	case "windows":
		return []VideoEncoder{VideoEncoderNVENC, VideoEncoderQSV}
	case "darwin":
		return []VideoEncoder{VideoEncoderVideoToolbox}
	default:
		return nil
	}
}

// InitialEncoderAvailability marks supported candidates checking and others unavailable/unsupported_os.
func InitialEncoderAvailability(goos string) []EncoderAvailability {
	candidates := HardwareEncoderCandidates(goos)
	list := make([]EncoderAvailability, 0, len(HardwareVideoEncoders))
	for _, encoder := range HardwareVideoEncoders {
		a := EncoderAvailability{Encoder: encoder, State: EncoderUnavailable, Reason: EncoderReasonUnsupportedOS}
		for _, c := range candidates {
			if c == encoder {
				a = EncoderAvailability{Encoder: encoder, State: EncoderChecking}
			}
		}
		list = append(list, a)
	}
	return list
}
