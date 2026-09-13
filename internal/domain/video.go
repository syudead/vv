package domain

import (
	"path/filepath"
	"strings"
	"time"
)

// ProbeState はメタデータ取得（ffprobe）の状態である。
// 値は api/openapi.yaml の Video.probeState に対応する。
type ProbeState string

const (
	// ProbeStatePending は取得がまだ終わっていない状態。
	ProbeStatePending ProbeState = "pending"
	// ProbeStateDone は取得できた状態。
	ProbeStateDone ProbeState = "done"
	// ProbeStateFailed は取得に失敗し、再試行の上限に達した状態。
	ProbeStateFailed ProbeState = "failed"
)

// ThumbnailState はサムネイル生成の状態である。
// 値は api/openapi.yaml の Video.thumbnailState に対応する。
type ThumbnailState string

const (
	// ThumbnailStatePending は生成がまだ終わっていない状態。
	ThumbnailStatePending ThumbnailState = "pending"
	// ThumbnailStateDone は画像が置かれている状態。
	ThumbnailStateDone ThumbnailState = "done"
	// ThumbnailStateFailed は生成に失敗し、再試行の上限に達した状態。
	ThumbnailStateFailed ThumbnailState = "failed"
)

// UnplayableReason はブラウザで再生できないと判定した理由である。
// 値は api/openapi.yaml の Video.unplayableReason に対応する。
type UnplayableReason string

const (
	// ReasonContainer はコンテナ（拡張子）が許可リストに無い。
	ReasonContainer UnplayableReason = "container"
	// ReasonVideoCodec は映像コーデックが許可リストに無い。
	ReasonVideoCodec UnplayableReason = "video_codec"
	// ReasonAudioCodec は音声コーデックが許可リストに無い。
	ReasonAudioCodec UnplayableReason = "audio_codec"
)

// 許可リスト（R-103）。拒否リストにしないのは、未知の値を「再生できる」と
// 誤ると再生して初めて失敗するためである。「再生できない」と誤る方が、
// 利用者の損失が小さい（SC-007）。
//
// hevc（H.265）や mkv はブラウザによっては再生できることがあるが、確実では
// ないので許可しない。
var (
	// allowedContainers は拡張子から決めたコンテナの許可リストである。
	allowedContainers = map[string]struct{}{
		"mp4": {}, "m4v": {}, "webm": {},
	}
	// allowedVideoCodecs は映像コーデックの許可リストである。
	allowedVideoCodecs = map[string]struct{}{
		"h264": {}, "vp8": {}, "vp9": {}, "av1": {},
	}
	// allowedAudioCodecs は音声コーデックの許可リストである。
	// 音声が無い（空）場合も許可する。
	allowedAudioCodecs = map[string]struct{}{
		"aac": {}, "mp3": {}, "opus": {}, "vorbis": {},
	}
)

// Probe は ffprobe から取り出した事実である。解釈を含まず、判定は
// EvaluatePlayability が行う。外部プロセスの実行は internal/media に閉じる。
type Probe struct {
	// DurationMs は尺（ミリ秒）。取得できなかった場合は 0 で、
	// 保存側はこれを null として扱う（0 で代用しない）。
	DurationMs int64
	// Width / Height は映像の解像度。取得できなければ 0。
	Width  int
	Height int
	// VideoCodec は先頭の映像ストリームの codec_name。映像が無ければ空。
	VideoCodec string
	// AudioCodec は先頭の音声ストリームの codec_name。音声が無ければ空。
	AudioCodec string
	// FormatName は ffprobe の format.format_name。記録用に持つ。
	// mov,mp4,m4a,3gp,3g2,mj2 のようにまとめて返るため、再生可否の判定には
	// 使わない（R-103）。
	FormatName string
}

// Playability は再生可否の判定結果である。Playable が true のとき Reason は空、
// false のとき Reason は必ず埋まる。
type Playability struct {
	Playable bool
	Reason   UnplayableReason
}

// EvaluatePlayability はコンテナと ffprobe の結果から、ブラウザでそのまま
// 再生できるかを判定する（R-103）。
//
// コンテナ → 映像 → 音声 の順に見て、最初に外れたものを理由にする。判定は
// 取り込み時に1度だけ行い、一覧では列を読むだけにする（SC-007）。
// 外部プロセスには触れないので、この規則は単体テストだけで検証できる。
func EvaluatePlayability(container string, probe Probe) Playability {
	if _, ok := allowedContainers[normalizeCodecName(container)]; !ok {
		return Playability{Reason: ReasonContainer}
	}
	if _, ok := allowedVideoCodecs[normalizeCodecName(probe.VideoCodec)]; !ok {
		return Playability{Reason: ReasonVideoCodec}
	}
	// 音声が無い動画は再生できる。無いことと、対応していない形式であることは違う。
	if audio := normalizeCodecName(probe.AudioCodec); audio != "" {
		if _, ok := allowedAudioCodecs[audio]; !ok {
			return Playability{Reason: ReasonAudioCodec}
		}
	}
	return Playability{Playable: true}
}

// ContainerFromPath は拡張子からコンテナ名を決める（先頭の "." を除いた小文字）。
//
// ffprobe の format_name ではなく拡張子を見るのは、format_name が
// mov,mp4,m4a,3gp,3g2,mj2 のようにまとめて返り mp4 と mov を区別できないため
// である。ブラウザに渡す Content-Type も拡張子から決めるので、判定と配信で
// 基準が揃う（R-103）。
func ContainerFromPath(path string) string {
	return normalizeCodecName(strings.TrimPrefix(filepath.Ext(path), "."))
}

// normalizeCodecName は比較のために表記を揃える。ffprobe の出力も拡張子も
// 表記が一定とは限らないため、判定の入口で1度だけ正規化する。
func normalizeCodecName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Video は videos の行と 1 対 1 で対応する値である。永続化の手段は知らない。
//
// 取得できなかった数値は nil で持つ。0 で代用すると「尺が 0 の動画」と
// 「尺が分からない動画」を一覧で区別できなくなる（data-model.md）。
type Video struct {
	ID        int64
	Path      string
	Title     string
	SizeBytes int64
	MTime     time.Time
	AddedAt   time.Time
	UpdatedAt time.Time

	// ContentKey は内容由来の識別子。移動・改名を越えて同じ動画と判定する鍵で、
	// 再生位置とサムネイルの名前もこれで決まる（R-101）。
	ContentKey string

	DurationMs *int64
	Width      *int
	Height     *int

	Container  string
	VideoCodec string
	AudioCodec string

	Playable         bool
	UnplayableReason UnplayableReason

	ProbeState     ProbeState
	ProbeError     string
	ThumbnailState ThumbnailState
}

// PlayableInBrowser はブラウザでそのまま再生できると確定しているかを返す。
//
// playable が立っていても、解析が終わっていなければ「確定していない」ものとして
// 扱う（data-model.md: playable = 1 は probe_state = done のときだけ取り得る）。
// 解析前の動画を「再生できる」と見せると、SC-007 の「再生を試みる前に判別できる」
// が成り立たなくなる。
func (v Video) PlayableInBrowser() bool {
	return v.Playable && v.ProbeState == ProbeStateDone
}

// HasThumbnail はサムネイルが生成済みかを返す。一覧はこれが false のとき
// 枠だけを描く（FR-010）。
func (v Video) HasThumbnail() bool {
	return v.ThumbnailState == ThumbnailStateDone
}
