package domain

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// ErrUnprocessableMedia は動画をbrowser互換streamへ変換するための情報または
// 本編映像が得られないことを表す。HTTP層は409へ写す。
var ErrUnprocessableMedia = errors.New("動画をライブ変換できません")

// ErrSeekFrameUnavailable は指定時刻から許容範囲内の本編映像を
// 取り出せないことを表す。HTTP層は409へ写す。
var ErrSeekFrameUnavailable = errors.New("シークプレビュー用の映像を取得できません")

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

// PreviewState is the lifecycle of the content-keyed hover preview asset.
type PreviewState string

const (
	PreviewStatePending PreviewState = "pending"
	PreviewStateDone    PreviewState = "done"
	PreviewStateFailed  PreviewState = "failed"
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

// 許可リスト。拒否リストにしないのは、未知の値を「再生できる」と
// 誤ると再生して初めて失敗するためである。「再生できない」と誤る方が、
// 利用者の損失が小さい。
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
	// Width / Height は表示される向きの映像の解像度（回転の印を反映済み）。取得できなければ 0。
	Width  int
	Height int
	// DisplayAspectRatio は表示される横÷縦の比率（回転と画素の縦横比を反映済み）。
	// 取得できなければ 0。
	DisplayAspectRatio float64
	// VideoCodec は先頭の映像ストリームの codec_name。映像が無ければ空。
	VideoCodec string
	// AudioCodec は先頭の音声ストリームの codec_name。音声が無ければ空。
	AudioCodec string
	// FormatName は ffprobe の format.format_name。記録用に持つ。
	// mov,mp4,m4a,3gp,3g2,mj2 のようにまとめて返るため、再生可否の判定には
	// 使わない。
	FormatName string
}

// Playability は再生可否の判定結果である。Playable が true のとき Reason は空、
// false のとき Reason は必ず埋まる。
type Playability struct {
	Playable bool
	Reason   UnplayableReason
}

// EvaluatePlayability はコンテナと ffprobe の結果から、ブラウザでそのまま
// 再生できるかを判定する。
//
// コンテナ → 映像 → 音声 の順に見て、最初に外れたものを理由にする。判定は
// 取り込み時に1度だけ行い、一覧では列を読むだけにする。
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
// 基準が揃う。
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
// 「尺が分からない動画」を一覧で区別できなくなる。
type Video struct {
	ID        int64
	Path      string
	Title     string
	SizeBytes int64
	MTime     time.Time
	AddedAt   time.Time
	UpdatedAt time.Time

	// ContentKey は内容由来の識別子。移動・改名を越えて同じ動画と判定する鍵で、
	// 再生位置とサムネイルの名前もこれで決まる。
	ContentKey string

	DurationMs *int64
	Width      *int
	Height     *int
	// DisplayAspectRatio は表示される横÷縦の比率。解析前・取得不能なら nil。
	DisplayAspectRatio *float64

	Container  string
	VideoCodec string
	AudioCodec string

	Playable         bool
	UnplayableReason UnplayableReason

	ProbeState     ProbeState
	ProbeError     string
	ThumbnailState ThumbnailState
	PreviewState   PreviewState

	// Public は公開フラグが立っているか（specs/016-single-account-auth/data-model.md §1）。
	// 保存層が public_videos から埋める。空の content_key の動画は常に false である。
	Public bool
}

// PlayableInBrowser はブラウザでそのまま再生できると確定しているかを返す。
//
// playable が立っていても、解析が終わっていなければ「確定していない」ものとして
// 扱う（playable = 1 は probe_state = done のときだけ取り得る）。
// 解析前の動画を「再生できる」と見せると、「再生を試みる前に判別できる」
// が成り立たなくなる。
func (v Video) PlayableInBrowser() bool {
	return v.Playable && v.ProbeState == ProbeStateDone
}

// HasThumbnail はサムネイルが生成済みかを返す。一覧はこれが false のとき
// 枠だけを描く。
func (v Video) HasThumbnail() bool {
	return v.ThumbnailState == ThumbnailStateDone
}

// VideoFile は走査で分かる事実である。解析（ffprobe）で分かる事実は含まない。
// 走査と保存の間でやり取りする値なので、どちらの都合も持ち込まない。
type VideoFile struct {
	Path       string
	Title      string
	ContentKey string
	SizeBytes  int64
	MTime      time.Time
	Container  string
	// AddedAt はゼロ値なら取り込み時刻を使う。新規のときだけ効く。
	AddedAt time.Time
}

// IndexedVideo は差分判定に要る最小限の値である。走査は実際のファイルと
// これを突き合わせる。
type IndexedVideo struct {
	ID              int64
	ContentKey      string
	LocationID      int64
	LocationVersion int64
	SizeBytes       int64
	MTime           time.Time
	ProbeState      ProbeState
	ThumbnailState  ThumbnailState
	PreviewState    PreviewState
}

// MediaFolder is one independently managed scan root.
type MediaFolder struct {
	ID        int64
	Path      string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// VideoLocation is one current filesystem location for a logical video.
type VideoLocation struct {
	ID        int64
	VideoID   int64
	Path      string
	Version   int64
	Title     string
	SizeBytes int64
	MTime     time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UpsertOutcome は取り込み1件の結果である。走査の集計（ScanResult）になる。
type UpsertOutcome string

const (
	// OutcomeAdded は新しく取り込んだ。
	OutcomeAdded UpsertOutcome = "added"
	// OutcomeUpdated は既存の行の内容が変わった。
	OutcomeUpdated UpsertOutcome = "updated"
	// OutcomeMoved は既知の内容を新しいpathで発見した。
	OutcomeMoved UpsertOutcome = "moved"
	// OutcomeUnchanged は何も変わらなかった。
	OutcomeUnchanged UpsertOutcome = "unchanged"
)

// UpsertResult は取り込み1件の結果である。
type UpsertResult struct {
	ID             int64
	Outcome        UpsertOutcome
	NeedsProbe     bool
	NeedsThumbnail bool
	NeedsPreview   bool
}

// HasSeekThumbnail はシーク用プレビューを持ちうる動画かを返す。応答の
// seekThumbnailUrl と seekThumbnailState は、これが true のときだけ載る。
func (v Video) HasSeekThumbnail() bool {
	return v.ProbeState == ProbeStateDone && v.DurationMs != nil &&
		*v.DurationMs > 0 && v.VideoCodec != "" && v.ContentKey != ""
}

// SeekThumbnailState はシーク用プレビューの状態である。DB 上には持たず、
// 置き場の有無とサムネイルのジョブの状態から導く。
type SeekThumbnailState string

const (
	// SeekThumbnailPending は作成中、または作成を待っている。
	SeekThumbnailPending SeekThumbnailState = "pending"
	// SeekThumbnailDone は置き場があり、読み出せる。
	SeekThumbnailDone SeekThumbnailState = "done"
	// SeekThumbnailFailed は置き場が無く、作る予定も無い。
	SeekThumbnailFailed SeekThumbnailState = "failed"
)

// VideoView は動画1件を応答に載せるときの形である。PreviewAvailable は
// 一覧用プレビューのファイルが今あるかで、true のときだけ previewUrl を出す。
// ファイルが消えていて作り直しを積んだ場合、Video.PreviewState は pending に
// 読み替えてある。
type VideoView struct {
	Video            Video
	PreviewAvailable bool
}

// DeletedVideo は行を消した動画である。生成物の片付けと、画面への知らせに使う。
type DeletedVideo struct {
	ID         int64
	ContentKey string
}

// ErrPreviewStale は、プレビューの生成中に元の動画の所在や内容が変わったことを
// 表す。生成物は公開されていないので、ジョブをやり直してよい。
var ErrPreviewStale = errors.New("preview source identity changed")

// SeekThumbnailInterval はシーク用プレビューのフレームの間隔である。生成
// （internal/media の ffmpeg の式）と読み出し（internal/artifacts の位置から
// フレームの番号への変換）が同じ値を使う。片方だけ変えると、読み出す番号が
// 別の場面を指す。
const SeekThumbnailInterval = 5 * time.Second
