package domain

import "errors"

// ErrEncoderUnavailable は、使えない（確認中を含む）ハードウェアの方式を選ぼうとしたことを表す。
// 保存値は変えない（specs/025-hardware-encoding/contracts/transcoding-settings-api.md §3）。
var ErrEncoderUnavailable = errors.New("video encoder is not available")

// VideoEncoder はライブ変換が実際に使う映像エンコード方式である
// （specs/025-hardware-encoding/data-model.md §3）。
type VideoEncoder string

const (
	VideoEncoderSoftware     VideoEncoder = "software"
	VideoEncoderNVENC        VideoEncoder = "nvenc"
	VideoEncoderQSV          VideoEncoder = "qsv"
	VideoEncoderVAAPI        VideoEncoder = "vaapi"
	VideoEncoderVideoToolbox VideoEncoder = "videotoolbox"
)

// HardwareVideoEncoders はハードウェアの方式を、auto が選ぶ順（一覧に載せる順でもある）に並べる。
var HardwareVideoEncoders = []VideoEncoder{
	VideoEncoderNVENC,
	VideoEncoderQSV,
	VideoEncoderVAAPI,
	VideoEncoderVideoToolbox,
}

// EncoderChoice は所有者が選ぶ値で、VideoEncoder に auto を足したものである
// （data-model.md §2）。
type EncoderChoice string

const (
	EncoderChoiceSoftware                   = EncoderChoice(VideoEncoderSoftware)
	EncoderChoiceNVENC                      = EncoderChoice(VideoEncoderNVENC)
	EncoderChoiceQSV                        = EncoderChoice(VideoEncoderQSV)
	EncoderChoiceVAAPI                      = EncoderChoice(VideoEncoderVAAPI)
	EncoderChoiceVideoToolbox               = EncoderChoice(VideoEncoderVideoToolbox)
	EncoderChoiceAuto         EncoderChoice = "auto"
)

// Valid は c が知っている選択肢かを返す。
func (c EncoderChoice) Valid() bool {
	switch c {
	case EncoderChoiceSoftware, EncoderChoiceAuto:
		return true
	}
	_, ok := c.HardwareEncoder()
	return ok
}

// HardwareEncoder は c がハードウェアの方式のとき、その VideoEncoder を返す。
func (c EncoderChoice) HardwareEncoder() (VideoEncoder, bool) {
	for _, encoder := range HardwareVideoEncoders {
		if EncoderChoice(encoder) == c {
			return encoder, true
		}
	}
	return "", false
}

// ParseEncoderChoice は保存値を解釈する。行が無いときと知らない文字列は software と
// して扱う（親 Issue #370 要件 3、Edge Case「保存値が未知の値」）。保存値は変えない。
func ParseEncoderChoice(value string, found bool) EncoderChoice {
	choice := EncoderChoice(value)
	if !found || !choice.Valid() {
		return EncoderChoiceSoftware
	}
	return choice
}

// EncoderState は起動時の確認の、1 つのハードウェアの方式の状態である。
type EncoderState string

const (
	EncoderChecking    EncoderState = "checking"
	EncoderAvailable   EncoderState = "available"
	EncoderUnavailable EncoderState = "unavailable"
)

// EncoderUnavailableReason は方式が使えない理由である。State が unavailable のときだけ付く。
type EncoderUnavailableReason string

const (
	EncoderReasonUnsupportedOS  EncoderUnavailableReason = "unsupported_os"
	EncoderReasonEncoderMissing EncoderUnavailableReason = "encoder_missing"
	EncoderReasonCheckFailed    EncoderUnavailableReason = "check_failed"
	EncoderReasonTimedOut       EncoderUnavailableReason = "timed_out"
)

// EncoderFallbackReason は、選んだハードウェアの方式を使えず software にしたときの理由である。
type EncoderFallbackReason string

const (
	EncoderFallbackSelectedUnavailable EncoderFallbackReason = "selected_unavailable"
	EncoderFallbackChecking            EncoderFallbackReason = "checking"
)

// EncoderAvailability は 1 つのハードウェアの方式の確認結果である。software は載らない
// （常に使える）。
type EncoderAvailability struct {
	Encoder VideoEncoder
	State   EncoderState
	// Reason は State が EncoderUnavailable のときだけ付く。
	Reason EncoderUnavailableReason
	// Detail は確認が失敗したときの補足（FFmpeg の標準エラーの末尾など）で、ログ用である。
	// API には出さない。
	Detail string
}

// TranscodeEncoding は今のライブ変換の方式の状態である（data-model.md §3）。保存しない。
type TranscodeEncoding struct {
	// Choice は保存値の解釈である。
	Choice EncoderChoice
	// Effective は今のライブ変換の要求が使う方式である。
	Effective VideoEncoder
	// FallbackReason は選んだハードウェアの方式を使えず software にしたときだけ付く。
	FallbackReason EncoderFallbackReason
	// Checking は起動時の確認が終わっていないか。
	Checking bool
	// Encoders は HardwareVideoEncoders の順の確認結果である。
	Encoders []EncoderAvailability
}

// NewTranscodeEncoding は選択と確認結果から今の状態を組み立てる。
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

// ResolveVideoEncoder は選択と確認結果から実際に使う方式を決める
// （specs/025-hardware-encoding/research.md R-3）。
//   - software は software。
//   - auto は使えるものを HardwareVideoEncoders の順で最初の 1 つ。無ければ software で、
//     fallback ではないので理由は付けない（要件 7）。
//   - ハードウェアの方式は使えればそれ。使えなければ software で selected_unavailable、
//     確認中なら software で checking。
//
// 知らない選択は software として扱う。
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

// stateOf は encoder の状態を返す。一覧に無い方式は使えないものとする。
func stateOf(encoder VideoEncoder, availability []EncoderAvailability) EncoderState {
	for _, a := range availability {
		if a.Encoder == encoder {
			return a.State
		}
	}
	return EncoderUnavailable
}

// HardwareEncoderCandidates は OS（runtime.GOOS の値）ごとの確認対象を
// HardwareVideoEncoders の順で返す（research.md R-2）。
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

// InitialEncoderAvailability は確認を始める前の一覧を HardwareVideoEncoders の順で返す。
// 確認対象は checking、対象外の方式は unavailable／unsupported_os である。
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
